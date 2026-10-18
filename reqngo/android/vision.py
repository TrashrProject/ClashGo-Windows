"""Opt-in template recognition for captured Android PNG frames.

Requires user-supplied, locally verified reference screenshots. When evidence
is missing or ambiguous, return UNKNOWN and never authorize any touch.
"""
from __future__ import annotations

import json
from dataclasses import dataclass
from pathlib import Path

from reqngo.android.adb import AdbError, Screen


class VisionError(ValueError):
    pass


@dataclass(frozen=True)
class Observation:
    state: str
    confidence: float


class TemplateVision:
    def __init__(self, manifest_path):
        self.manifest_path = Path(manifest_path)
        if not self.manifest_path.is_file():
            raise VisionError("Fichier de calibration visuelle manquant.")
        config = json.loads(self.manifest_path.read_text(encoding="utf-8"))
        if not isinstance(config, dict) or not isinstance(config.get("states"), list):
            raise VisionError("Configuration de detection invalide.")
        specs = config["states"]
        if not 1 <= len(specs) <= 30:
            raise VisionError("Entre 1 et 30 etats visuels requis.")
        names = set()
        parsed = []
        base = self.manifest_path.parent.resolve()
        for row in specs:
            if not isinstance(row, dict):
                raise VisionError("Description d'etat incorrecte.")
            name = row.get("name")
            template = row.get("template")
            region = row.get("region", [0, 0, 1, 1])
            minimum = row.get("threshold", 0.87)
            if (not isinstance(name, str) or not name.isidentifier()
                    or name in names or name == "unknown"):
                raise VisionError("Nom d'etat invalide ou duplique.")
            if (not isinstance(template, str) or not template.endswith(".png")
                    or Path(template).name != template):
                raise VisionError("Modele PNG local obligatoire.")
            path = (base / template).resolve()
            if path.parent != base or not path.is_file() or path.stat().st_size > 5_000_000:
                raise VisionError("Modele de reference absent ou trop volumineux.")
            if (not isinstance(region, list) or len(region) != 4
                    or any(type(v) not in (float, int) or not 0 <= v <= 1 for v in region)
                    or region[2] <= region[0] or region[3] <= region[1]):
                raise VisionError("Region de detection incorrecte.")
            if (type(minimum) not in (float, int) or not 0.6 <= minimum <= 1):
                raise VisionError("Seuil de confiance invalide.")
            names.add(name)
            parsed.append((name, path, tuple(region), float(minimum)))
        self.specs = parsed
        self.states = frozenset(names)
        # A fast route checks the same reference several times per gesture.
        # Load it once per vision instance rather than reading disk for every frame.
        self._template_cache = {}

    @staticmethod
    def _libraries():
        try:
            import cv2
            import numpy as np
        except ImportError as exc:
            raise VisionError("Installer les dependances de requirements-android.txt.") from exc
        return cv2, np

    def detect(self, screen: Screen) -> Observation:
        cv2, np = self._libraries()
        frame = cv2.imdecode(np.frombuffer(screen.png, dtype=np.uint8), cv2.IMREAD_GRAYSCALE)
        if frame is None or frame.shape[:2] != (screen.height, screen.width):
            raise AdbError("Capture Android non decodable.")
        scores = []
        for name, path, region, threshold in self.specs:
            template = self._template_cache.get(path)
            if template is None:
                template = cv2.imread(str(path), cv2.IMREAD_GRAYSCALE)
                if template is None or template.size == 0:
                    raise VisionError("Modele de reference illisible.")
                if template.std() < 8:
                    raise VisionError("Modele de reference presque uniforme : risque de faux positif.")
                self._template_cache[path] = template
            x1, y1, x2, y2 = region
            left = int(x1 * screen.width)
            top = int(y1 * screen.height)
            right = max(left + 1, int(x2 * screen.width))
            bottom = max(top + 1, int(y2 * screen.height))
            patch = frame[top:bottom, left:right]
            if template.shape[0] > patch.shape[0] or template.shape[1] > patch.shape[1]:
                continue
            # TM_CCOEFF_NORMED is intentionally conservative. Do not infer a
            # visual state from OCR strings or imagined image descriptions.
            match = cv2.matchTemplate(patch, template, cv2.TM_CCOEFF_NORMED)
            score = float(match.max())
            scores.append((score, name, threshold))
        scores.sort(reverse=True)
        if not scores or scores[0][0] < scores[0][2]:
            return Observation("unknown", 0.0 if not scores else scores[0][0])
        if len(scores) >= 2 and scores[0][0] - scores[1][0] < 0.035:
            return Observation("unknown", scores[0][0])
        return Observation(scores[0][1], scores[0][0])
