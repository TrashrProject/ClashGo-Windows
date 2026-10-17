"""Crash-safe, visually verified action runner for one expressly selected device.

No blind tap loops: a confirmed BEFORE state and two confirming AFTER frames
are required. A crash during input leaves an unresolved transaction, never a
silent retry. Each profile has its own SQLite journal.
"""
from __future__ import annotations

import json
import sqlite3
import time
from dataclasses import dataclass
from pathlib import Path

from reqngo.android.adb import AdbError


class ExecutionBlocked(RuntimeError):
    pass


@dataclass(frozen=True)
class Step:
    name: str
    before: str
    after: str
    action: str
    points: tuple
    duration_ms: int
    timeout_seconds: int


class Workflow:
    def __init__(self, path, states):
        data = json.loads(Path(path).read_text(encoding="utf-8"))
        steps = data.get("steps") if isinstance(data, dict) else None
        if not isinstance(steps, list) or not 1 <= len(steps) <= 30:
            raise ValueError("Workflow de 1 a 30 etapes requis.")
        checked = []
        for row in steps:
            if not isinstance(row, dict) or set(row) - {
                "name", "before", "after", "action", "points", "duration_ms", "timeout_seconds"
            }:
                raise ValueError("Etape inconnue ou champ non autorise.")
            name = row.get("name")
            before, after = row.get("before"), row.get("after")
            action = row.get("action")
            points = row.get("points", [])
            duration = row.get("duration_ms", 350)
            timeout = row.get("timeout_seconds", 12)
            if (not isinstance(name, str) or not name.isidentifier()
                    or before not in states or after not in states or before == after):
                raise ValueError("Etat de depart ou d'arrivee non confirme par la calibration.")
            if action not in ("tap", "swipe", "back"):
                raise ValueError("Action Android non autorisee.")
            expected = {"tap": 2, "swipe": 4, "back": 0}[action]
            if (not isinstance(points, list) or len(points) != expected
                    or any(type(v) not in (int, float) or not 0 <= v <= 1 for v in points)):
                raise ValueError("Coordonnees d'action invalides.")
            if type(timeout) is not int or not 2 <= timeout <= 90:
                raise ValueError("Timeout d'action non autorise.")
            if type(duration) is not int or not 100 <= duration <= 1500:
                raise ValueError("Duree de geste invalide.")
            checked.append(Step(name, before, after, action, tuple(points), duration, timeout))
        self.steps = tuple(checked)


class Journal:
    def __init__(self, path):
        path = Path(path)
        path.parent.mkdir(parents=True, exist_ok=True)
        self.db = sqlite3.connect(path)
        with self.db:
            self.db.execute("CREATE TABLE IF NOT EXISTS execution ("
                            "id INTEGER PRIMARY KEY CHECK(id=1), "
                            "step_index INTEGER NOT NULL, phase TEXT NOT NULL)")
            self.db.execute("INSERT OR IGNORE INTO execution VALUES(1,0,'ready')")

    def state(self):
        idx, phase = self.db.execute(
            "SELECT step_index,phase FROM execution WHERE id=1"
        ).fetchone()
        return idx, phase

    def move(self, idx, phase):
        if type(idx) is not int or idx < 0 or phase not in (
                "ready", "armed", "blocked", "awaiting_verification"):
            raise ValueError("Transaction Android incorrecte.")
        with self.db:
            self.db.execute("UPDATE execution SET step_index=?,phase=? WHERE id=1",
                            (idx, phase))

    def close(self):
        self.db.close()


class ActionRunner:
    def __init__(self, device, vision, workflow: Workflow, journal: Journal,
                 *, clock=None, sleep=None):
        self.device = device
        self.vision = vision
        self.workflow = workflow
        self.journal = journal
        self.clock = clock or time.monotonic
        self.sleep = sleep or time.sleep

    def _observe(self, expected, deadline):
        """Return last fresh screen only after two consecutive matches."""
        same = 0
        while self.clock() < deadline:
            shot = self.device.screenshot()
            seen = self.vision.detect(shot)
            same = same + 1 if seen.state == expected else 0
            if same >= 2:
                return shot
            self.sleep(0.3)
        raise ExecutionBlocked("Etat visuel non confirme : aucune action repetee.")

    def _execute(self, step, screen):
        if step.action == "tap":
            self.device.tap(*step.points, screen)
        elif step.action == "swipe":
            self.device.swipe(*step.points, step.duration_ms, screen)
        else:
            self.device.back()

    def step(self, *, dry_run=True):
        idx, phase = self.journal.state()
        if idx >= len(self.workflow.steps):
            return {"status": "complete", "step_index": idx}
        current = self.workflow.steps[idx]
        if not self.device.game_running() or not self.device.game_foreground():
            raise ExecutionBlocked("Jeu absent ou non confirme au premier plan : aucun geste.")
        if phase != "ready":
            # We cannot know whether a previous tap was delivered. Never replay it.
            try:
                self._observe(current.after, self.clock() + 5)
            except (ExecutionBlocked, AdbError) as exc:
                self.journal.move(idx, "blocked")
                raise ExecutionBlocked(
                    "Action interrompue : verifier manuellement l'ecran, aucune repetition."
                ) from exc
            self.journal.move(idx + 1, "ready")
            return {"status": "verified_after_restart", "step_index": idx + 1}
        screen = self._observe(current.before, self.clock() + 5)
        if dry_run:
            return {"status": "dry_run", "step_index": idx, "action": current.action,
                    "before": current.before, "after": current.after}
        # Reconfirm focus immediately before input, not only before vision
        # matching: a user can switch apps while frames are being checked.
        if not self.device.game_foreground():
            raise ExecutionBlocked("Le jeu a perdu le premier plan : geste refuse.")
        # Mark possible execution BEFORE touching Android, so a crash cannot
        # trigger an automatic second request/join/leave action.
        self.journal.move(idx, "armed")
        self._execute(current, screen)
        self.journal.move(idx, "awaiting_verification")
        try:
            self._observe(current.after, self.clock() + current.timeout_seconds)
        except (ExecutionBlocked, AdbError) as exc:
            self.journal.move(idx, "blocked")
            raise ExecutionBlocked(
                "Transition non verifiee, pilote bloque sans retenter l'action."
            ) from exc
        self.journal.move(idx + 1, "ready")
        return {"status": "verified", "step_index": idx + 1, "action": current.action}
