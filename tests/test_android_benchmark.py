"""Read-only ADB performance tests with fake screenshots and monotonic clock."""
import unittest

from reqngo.android.adb import Screen
from reqngo.android.benchmark import measure_capture_latency


class FakeDevice:
    def __init__(self, ticks=None):
        self.running = True
        self.focused = True
        self.shots = 0
        self.ticks = ticks or [0.0]

    def game_running(self):
        return self.running

    def game_foreground(self):
        return self.focused

    def screenshot(self):
        self.shots += 1
        return Screen(b"", 240, 120)


class CaptureLatencyTests(unittest.TestCase):
    def test_benchmark_has_no_touches_and_returns_aggregates(self):
        d = FakeDevice()
        points = iter([0, .1, .2, .3, .4, .5])
        result = measure_capture_latency(d, samples=3, clock=lambda: next(points))
        self.assertEqual(d.shots, 3)
        self.assertEqual(result["samples"], 3)
        self.assertEqual(result["median_ms"], 100)
        self.assertEqual(result["resolution"], [240, 120])
        self.assertNotIn("image", result)
        self.assertNotIn("png", result)

    def test_requires_foreground_and_bounded_samples(self):
        d = FakeDevice()
        for count in (0, 1, 13, "5", True):
            with self.subTest(samples=count), self.assertRaises(ValueError):
                measure_capture_latency(d, samples=count)
        self.assertEqual(d.shots, 0)
        d.focused = False
        with self.assertRaises(ValueError):
            measure_capture_latency(d, samples=2)
        self.assertEqual(d.shots, 0)

    def test_detects_resolution_change(self):
        class Device(FakeDevice):
            def screenshot(self):
                self.shots += 1
                return Screen(b"", 240 if self.shots == 1 else 320, 120)
        with self.assertRaises(ValueError):
            measure_capture_latency(Device(), samples=2)


if __name__ == "__main__":
    unittest.main()
