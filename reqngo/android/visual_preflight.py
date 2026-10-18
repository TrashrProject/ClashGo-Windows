"""Read-only visual state verification for authorized, locally bound Android devices.

Inspired by conditional visual automation, but intentionally executes no inputs.
Screenshots are processed in memory and never uploaded to the control plane.
An observation only succeeds after two fresh consistent frames.
"""
from __future__ import annotations

import json
import re
import time
from pathlib import Path

from reqngo.android.adb import AdbClient, AdbError
from reqngo.android.vision import TemplateVision, VisionError
from reqngo.fleet import _KEY

_FILENAME = re.compile(r"[A-Za-z][A-Za-z0-9_-]{0,63}\.json\Z")
_STATE = re.compile(r"[A-Za-z][A-Za-z0-9_]{0,63}\Z")


class VisualReadOnlyPreflight:
    def __init__(self, device_preflight, config_dir, rules, *,
                 vision_class=TemplateVision, device_class=AdbClient,
                 delay=None):
        """rules maps logical profiles to local manifest basenames and states.

        Never accept filenames or a device serial from the central job payload.
        """
        if not isinstance(rules, dict) or len(rules) > 10_000:
            raise ValueError("Configuration visuelle invalide.")
        self.preflight = device_preflight
        self.directory = Path(config_dir).resolve()
        self.vision_class = vision_class
        self.device_class = device_class
        self.delay = delay or time.sleep
        checked = {}
        for key, record in rules.items():
            if (not isinstance(key, str) or not _KEY.fullmatch(key)
                    or not isinstance(record, dict)
                    or set(record) != {"manifest", "expected"}):
                raise ValueError("Regle de reconnaissance invalide.")
            filename, expected = record["manifest"], record["expected"]
            if (not isinstance(filename, str) or not _FILENAME.fullmatch(filename)
                    or not isinstance(expected, str) or not _STATE.fullmatch(expected)
                    or expected == "unknown"):
                raise ValueError("Nom de calibration ou d'etat invalide.")
            checked[key] = (filename, expected)
        self.rules = checked
        if self.rules and not self.directory.is_dir():
            raise ValueError("Repertoire local des calibrations introuvable.")

    @classmethod
    def from_json(cls, device_preflight, config_dir, raw, **kwargs):
        if not isinstance(raw, str) or len(raw) > 1_000_000:
            raise ValueError("Configuration visuelle trop volumineuse.")
        try:
            config = json.loads(raw) if raw else {}
        except (ValueError, TypeError) as exc:
            raise ValueError("Configuration visuelle JSON invalide.") from exc
        return cls(device_preflight, config_dir, config, **kwargs)

    def check(self, profile_key):
        rule = self.rules.get(profile_key)
        serial = self.preflight.bindings.get(profile_key)
        if rule is None or serial is None:
            return False
        filename, expected = rule
        calibration = (self.directory / filename).resolve()
        if calibration.parent != self.directory or not calibration.is_file():
            return False
        if calibration.stat().st_size > 60_000:
            raise VisionError("Manifeste visuel trop volumineux.")
        vision = self.vision_class(calibration)
        if expected not in vision.states:
            raise VisionError("Etat non defini dans la calibration locale.")
        # Use the same per-device lock as ordinary ADB diagnostics, so one
        # worker process cannot inspect a device in conflicting threads.
        lock = self.preflight.serial_locks[serial]
        with lock:
            available = self.device_class.devices(
                adb_binary=self.preflight.adb_binary)
            if serial not in available:
                return False
            device = self.device_class(serial, adb_binary=self.preflight.adb_binary,
                                       timeout=15)
            if not device.game_running() or not device.game_foreground():
                return False
            for frame_idx in range(2):
                # Re-check before EACH frame; no input, no retry loop.
                if not device.game_foreground():
                    return False
                screen = device.screenshot()
                observation = vision.detect(screen)
                if observation.state != expected:
                    return False
                if frame_idx == 0:
                    self.delay(0.25)
            return True
