"""Workflow tests: no gesture without stable before/after state and crash-safe journal."""
import json
import tempfile
import unittest
from pathlib import Path
from unittest.mock import Mock

from reqngo.android.adb import AdbError, Screen
from reqngo.android.runner import ActionRunner, ExecutionBlocked, Journal, Workflow
from reqngo.android.vision import Observation


class FakeDevice:
    def __init__(self, running=True, tap_error=None):
        self.running = running
        self.tap_error = tap_error
        self.taps = []
        self.shots = 0

    def game_running(self):
        return self.running

    def game_foreground(self):
        return self.running

    def screenshot(self):
        self.shots += 1
        return Screen(b"", 100, 200)

    def tap(self, x, y, screen):
        self.taps.append((x, y))
        if self.tap_error:
            raise self.tap_error

    def swipe(self, *args):
        self.taps.append(args)

    def back(self):
        self.taps.append(("back",))


class FakeVision:
    def __init__(self, values):
        self.values = iter(values)
        self.states = {"village", "clan_window"}

    def detect(self, screen):
        try:
            state = next(self.values)
        except StopIteration:
            state = "unknown"
        return Observation(state, 0.99 if state != "unknown" else 0)


class RunnerTests(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.base = Path(self.tmp.name)
        self.flow_path = self.base / "flow.json"
        self.flow_path.write_text(json.dumps({"steps": [{
            "name": "open_clan", "before": "village", "after": "clan_window",
            "action": "tap", "points": [0.25, 0.7], "timeout_seconds": 3
        }]}))
        self.workflow = Workflow(self.flow_path, {"village", "clan_window"})
        self.journal = Journal(self.base / "state.sqlite3")
        self.now = [0.]
        self.sleep = lambda delta: self.now.__setitem__(0, self.now[0] + delta)

    def tearDown(self):
        self.journal.close()
        self.tmp.cleanup()

    def runner(self, device, states):
        return ActionRunner(device, FakeVision(states), self.workflow, self.journal,
                            clock=lambda: self.now[0], sleep=self.sleep)

    def test_dry_run_is_default_and_never_touches_game(self):
        device = FakeDevice()
        result = self.runner(device, ["village", "village"]).step()
        self.assertEqual(result["status"], "dry_run")
        self.assertEqual(device.taps, [])
        self.assertEqual(self.journal.state(), (0, "ready"))

    def test_execute_waits_for_two_matching_pre_and_post_frames(self):
        device = FakeDevice()
        result = self.runner(device, [
            "unknown", "village", "village",
            "clan_window", "unknown", "clan_window", "clan_window",
        ]).step(dry_run=False)
        self.assertEqual(result["status"], "verified")
        self.assertEqual(device.taps, [(0.25, 0.7)])
        self.assertEqual(self.journal.state(), (1, "ready"))

    def test_unknown_before_never_touches_screen(self):
        device = FakeDevice()
        with self.assertRaises(ExecutionBlocked):
            self.runner(device, ["unknown"] * 30).step(dry_run=False)
        self.assertEqual(device.taps, [])
        self.assertEqual(self.journal.state(), (0, "ready"))

    def test_unknown_after_blocks_without_replaying_tap(self):
        device = FakeDevice()
        with self.assertRaises(ExecutionBlocked):
            self.runner(device, ["village", "village"] + ["unknown"] * 20).step(dry_run=False)
        self.assertEqual(device.taps, [(0.25, 0.7)])
        self.assertEqual(self.journal.state(), (0, "blocked"))
        with self.assertRaises(ExecutionBlocked):
            self.runner(device, ["unknown"] * 30).step(dry_run=False)
        self.assertEqual(len(device.taps), 1)

    def test_crash_during_android_input_never_repeats_touch(self):
        device = FakeDevice(tap_error=AdbError("transport died"))
        with self.assertRaises(AdbError):
            self.runner(device, ["village", "village"]).step(dry_run=False)
        self.assertEqual(self.journal.state(), (0, "armed"))
        device.tap_error = None
        resumed = self.runner(device, ["clan_window", "clan_window"]).step(dry_run=False)
        self.assertEqual(resumed["status"], "verified_after_restart")
        self.assertEqual(len(device.taps), 1)
        self.assertEqual(self.journal.state(), (1, "ready"))

    def test_app_switch_during_visual_check_prevents_touch(self):
        class SwitchingDevice(FakeDevice):
            checks = 0

            def game_foreground(self):
                self.checks += 1
                return self.checks == 1

        device = SwitchingDevice()
        with self.assertRaises(ExecutionBlocked):
            self.runner(device, ["village", "village"]).step(dry_run=False)
        self.assertEqual(device.taps, [])
        self.assertEqual(self.journal.state(), (0, "ready"))

    def test_game_must_already_be_running(self):
        device = FakeDevice(running=False)
        with self.assertRaises(ExecutionBlocked):
            self.runner(device, ["village", "village"]).step(dry_run=False)
        self.assertFalse(device.taps)

    def test_workflow_rejects_unvalidated_and_unsupported_actions(self):
        bads = [
            {"name": "x", "before": "unknown", "after": "clan_window",
             "action": "tap", "points": [0.3, 0.3]},
            {"name": "x", "before": "village", "after": "clan_window",
             "action": "shell", "points": []},
            {"name": "x", "before": "village", "after": "clan_window",
             "action": "tap", "points": [1.2, 0.3]},
            {"name": "x", "before": "village", "after": "village",
             "action": "tap", "points": [0.3, 0.3]},
        ]
        for bad in bads:
            with self.subTest(bad=bad), self.assertRaises(ValueError):
                self.flow_path.write_text(json.dumps({"steps": [bad]}))
                Workflow(self.flow_path, {"village", "clan_window"})

    def test_state_journal_persists_across_process_restart(self):
        self.journal.move(0, "armed")
        self.journal.close()
        self.journal = Journal(self.base / "state.sqlite3")
        self.assertEqual(self.journal.state(), (0, "armed"))


if __name__ == "__main__":
    unittest.main()
