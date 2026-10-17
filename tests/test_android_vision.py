"""Offline visual-matcher tests using synthetic PNGs, never game assets."""
import json
import tempfile
import unittest
from pathlib import Path

from reqngo.android.adb import Screen
from reqngo.android.vision import TemplateVision, VisionError


try:
    import cv2
    import numpy as np
    HAS_CV = True
except ImportError:
    HAS_CV = False


@unittest.skipUnless(HAS_CV, "Optional Android computer-vision dependencies missing")
class TemplateVisionTests(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.base = Path(self.tmp.name)
        random = np.random.default_rng(123)
        self.base_img = random.integers(0, 256, (120, 160), dtype=np.uint8)
        self.match = self.base_img[40:65, 50:85]
        cv2.imwrite(str(self.base / "known.png"), self.match)
        cv2.imwrite(str(self.base / "unrelated.png"),
                    random.integers(0, 256, self.match.shape, dtype=np.uint8))
        self.path = self.base / "states.json"

    def tearDown(self):
        self.tmp.cleanup()

    def manifest(self, rows):
        self.path.write_text(json.dumps({"states": rows}), encoding="utf-8")
        return TemplateVision(self.path)

    def screenshot(self, data):
        ok, png = cv2.imencode(".png", data)
        self.assertTrue(ok)
        return Screen(png.tobytes(), data.shape[1], data.shape[0])

    def test_detects_exact_template_in_real_png(self):
        vision = self.manifest([
            {"name": "village", "template": "known.png", "threshold": 0.9},
            {"name": "clan_window", "template": "unrelated.png", "threshold": 0.9},
        ])
        found = vision.detect(self.screenshot(self.base_img))
        self.assertEqual(found.state, "village")
        self.assertGreater(found.confidence, 0.99)

    def test_random_other_screen_is_unknown(self):
        vision = self.manifest([{"name": "village", "template": "known.png"}])
        random = np.random.default_rng(456)
        frame = random.integers(0, 256, (120, 160), dtype=np.uint8)
        self.assertEqual(vision.detect(self.screenshot(frame)).state, "unknown")

    def test_ambiguous_equal_templates_refuse_to_choose(self):
        cv2.imwrite(str(self.base / "same.png"), self.match)
        vision = self.manifest([
            {"name": "village", "template": "known.png"},
            {"name": "clan_window", "template": "same.png"},
        ])
        self.assertEqual(vision.detect(self.screenshot(self.base_img)).state, "unknown")

    def test_path_traversal_and_uniform_templates_rejected(self):
        with self.assertRaises(VisionError):
            self.manifest([{"name": "x", "template": "../outside.png"}])
        cv2.imwrite(str(self.base / "blank.png"), np.zeros((10, 10), dtype=np.uint8))
        vision = self.manifest([{"name": "x", "template": "blank.png"}])
        with self.assertRaises(VisionError):
            vision.detect(self.screenshot(self.base_img))


if __name__ == "__main__":
    unittest.main()
