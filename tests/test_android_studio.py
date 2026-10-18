"""End-to-end local Android Studio tests with synthetic PNGs and mocked ADB.

No real game account and no Android environment are required in CI. The tests
assert that only an explicitly confirmed call may cause a real adapter tap.
"""
from __future__ import annotations

import json
import tempfile
import threading
import unittest
from pathlib import Path
from urllib.error import HTTPError
from urllib.request import Request, urlopen

from reqngo.android.studio import AndroidStudio, StudioError, StudioHTTPServer

try:
    import cv2
    import numpy as np
    HAS_VISION = True
except ImportError:
    HAS_VISION = False


class FakeADB:
    active = None

    def __init__(self, serial, *, adb_binary="adb"):
        self.serial = serial
        self.stage = 0
        self.focused = True
        self.tap_count = 0
        self.frames = None
        FakeADB.active = self

    def game_running(self):
        return True

    def game_foreground(self):
        return self.focused

    def screenshot(self):
        return self.frames[self.stage]

    def tap(self, x, y, screen):
        self.tap_count += 1
        self.stage = 1

    def swipe(self, *args):
        raise AssertionError("Studio does not expose swipes")

    def back(self):
        raise AssertionError("Studio does not expose back")


@unittest.skipUnless(HAS_VISION, "Optional Android vision dependencies missing")
class StudioTests(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.path = Path(self.tmp.name)
        before = np.random.default_rng(83).integers(0, 256, (120, 160), dtype=np.uint8)
        after = np.random.default_rng(97).integers(0, 256, (120, 160), dtype=np.uint8)
        from reqngo.android.adb import Screen
        def shot(frame):
            ok, encoded = cv2.imencode(".png", frame)
            assert ok
            return Screen(encoded.tobytes(), 160, 120)
        self.studio = AndroidStudio("emulator-5554", self.path,
                                    client_class=FakeADB, actions_enabled=True)
        self.device = FakeADB.active
        self.device.frames = [shot(before), shot(after)]
        self.region = [.25, .33, .5, .6]

    def tearDown(self):
        self.tmp.cleanup()

    def calibrate(self):
        before, _ = self.studio.capture()
        self.studio.save_state({"name": "village", "frame_id": before,
                                "rect": self.region})
        self.device.stage = 1
        after, _ = self.studio.capture()
        self.studio.save_state({"name": "clan_window", "frame_id": after,
                                "rect": self.region})
        self.device.stage = 0
        self.studio.save_step({"name": "open_clan", "before": "village",
                               "after": "clan_window", "position": [.3, .4]})

    def test_real_png_calibration_and_exactly_one_tap(self):
        self.calibrate()
        self.assertEqual(len(self.studio.status()["states"]), 2)
        dry = self.studio.run_once()
        self.assertEqual(dry["status"], "dry_run")
        self.assertEqual(self.device.tap_count, 0)
        with self.assertRaises(StudioError):
            self.studio.run_once(execute=True, confirmation="no")
        executed = self.studio.run_once(execute=True,
                                         confirmation="EXECUTER UNE ACTION")
        self.assertEqual(executed["status"], "verified")
        self.assertEqual(self.device.tap_count, 1)
        self.assertEqual(self.studio.status()["next_step"], 1)
        self.assertEqual(self.studio.run_once(execute=True,
                          confirmation="EXECUTER UNE ACTION")["status"], "complete")
        self.assertEqual(self.device.tap_count, 1)

    def test_restarted_studio_uses_persisted_calibration_and_journal(self):
        self.calibrate()
        self.studio.run_once(execute=True, confirmation="EXECUTER UNE ACTION")
        restarted = AndroidStudio("emulator-5554", self.path, client_class=FakeADB,
                                   actions_enabled=True)
        self.assertEqual(restarted.status()["next_step"], 1)
        self.assertEqual(len(restarted.status()["steps"]), 1)

    def test_no_tap_when_frame_ambiguous_or_unfocused(self):
        self.calibrate()
        self.device.focused = False
        with self.assertRaises(StudioError):
            self.studio.capture()
        with self.assertRaises(Exception):
            self.studio.run_once(execute=True, confirmation="EXECUTER UNE ACTION")
        self.assertEqual(self.device.tap_count, 0)
        self.device.focused = True
        self.device.stage = 1  # after state, not before
        with self.assertRaises(Exception):
            self.studio.run_once(execute=True, confirmation="EXECUTER UNE ACTION")
        self.assertEqual(self.device.tap_count, 0)

    def test_rejects_stale_frame_invalid_paths_and_duplicate_state(self):
        captured, _ = self.studio.capture()
        with self.assertRaises(StudioError):
            self.studio.save_state({"name": "../bad", "frame_id": captured,
                                    "rect": self.region})
        with self.assertRaises(StudioError):
            self.studio.save_state({"name": "village", "frame_id": "other",
                                    "rect": self.region})
        self.studio.save_state({"name": "village", "frame_id": captured,
                                "rect": self.region})
        with self.assertRaises(StudioError):
            self.studio.save_state({"name": "village", "frame_id": captured,
                                    "rect": self.region})
        self.assertEqual(len(list(self.studio.folder.glob("village.png"))), 1)
        with self.assertRaises(StudioError):
            self.studio.save_step({"name": "missing", "before": "village",
                                   "after": "unknown", "position": [.4, .5]})

    def test_http_rejects_origin_and_missing_token(self):
        self.calibrate()
        server = StudioHTTPServer(("127.0.0.1", 0), self.studio)
        thread = threading.Thread(target=server.serve_forever, daemon=True)
        thread.start()
        try:
            base = f"http://127.0.0.1:{server.server_port}"
            with urlopen(base + "/", timeout=5) as response:
                text = response.read().decode()
            self.assertIn(server.secret, text)
            with urlopen(base + "/api/status", timeout=5) as response:
                data = json.loads(response.read())
            self.assertEqual(len(data["steps"]), 1)
            payload = json.dumps({"execute": True,
                                  "confirmation": "EXECUTER UNE ACTION"}).encode()
            for origin, token in [("https://evil.invalid", server.secret),
                                  (base, "invalid")]:
                req = Request(base + "/api/run", data=payload,
                              headers={"Content-Type": "application/json",
                                       "Origin": origin,
                                       "X-REQNGO-Local-Token": token},
                              method="POST")
                with self.assertRaises(HTTPError) as err:
                    urlopen(req, timeout=5)
                self.assertEqual(err.exception.code, 403)
            self.assertEqual(self.device.tap_count, 0)
            req = Request(base + "/api/run", data=payload,
                          headers={"Content-Type": "application/json",
                                   "Origin": base,
                                   "X-REQNGO-Local-Token": server.secret},
                          method="POST")
            with urlopen(req, timeout=10) as response:
                self.assertEqual(json.loads(response.read())["status"], "verified")
            self.assertEqual(self.device.tap_count, 1)
        finally:
            server.shutdown()
            thread.join(timeout=3)
            server.server_close()


if __name__ == "__main__":
    unittest.main()
