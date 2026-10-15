import tempfile
import unittest
from pathlib import Path
from unittest.mock import Mock

from reqngo.core import Guardian
from reqngo.supervisor import Supervisor
from reqngo.telemetry import TelemetryStore


class SupervisorReliabilityTests(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.now = [1000.0]
        self.monotonic = [0.0]
        clock = lambda: self.now[0]
        self.core = Guardian(Path(self.tmp.name) / "guardian.sqlite3", clock=clock)
        self.remote = TelemetryStore(Path(self.tmp.name) / "telemetry.sqlite3", clock=clock)
        self.errors = []
        self.svc = Supervisor(self.core, self.remote, wall_clock=clock,
                              monotonic_clock=lambda: self.monotonic[0],
                              telemetry_interval=30, on_error=lambda *v: self.errors.append(v))

    def tearDown(self):
        self.core.close()
        self.remote.close()
        self.tmp.cleanup()

    def test_remote_failure_detected_without_waiting_full_ten_minutes(self):
        self.remote.heartbeat("pilot_01")
        first = self.svc.tick()
        self.assertTrue(first["full_scan"])
        self.assertTrue(first["remote_check"])
        self.assertEqual(first["alerts_created"], 0)
        self.now[0] += 181
        self.monotonic[0] += 31
        result = self.svc.tick()
        self.assertFalse(result["full_scan"])
        self.assertEqual(result["alerts_created"], 1)
        self.assertEqual(self.remote.snapshot()["summary"]["open_cases"], 1)

    def test_deduplicates_alerts_while_worker_disconnected(self):
        self.remote.heartbeat("pilot_01")
        self.svc.tick()
        self.now[0] += 210
        self.monotonic[0] += 31
        self.assertEqual(self.svc.tick()["alerts_created"], 1)
        self.monotonic[0] += 31
        self.assertEqual(self.svc.tick()["alerts_created"], 0)
        self.assertEqual(self.remote.snapshot()["summary"]["open_cases"], 1)
        self.remote.heartbeat("pilot_01")
        self.assertEqual(self.remote.snapshot()["summary"]["open_cases"], 0)

    def test_remote_failure_does_not_stop_local_scans(self):
        failing = Mock(side_effect=OSError("offline connection"))
        self.remote.check_stale = failing
        self.core.inject(1, "worker_crash")
        result = self.svc.tick()
        self.assertTrue(result["full_scan"])
        self.assertEqual(result["failed_components"], ["remote"])
        self.assertEqual(self.core.accounts()[0]["status"], "ok")
        self.assertEqual(self.errors, [("remote", "OSError")])

    def test_local_failure_does_not_stop_remote_monitoring(self):
        self.remote.heartbeat("pilot_01")
        self.core.settings = Mock(side_effect=RuntimeError("db failure secret"))
        result = self.svc.tick()
        self.assertEqual(result["failed_components"], ["local"])
        self.assertTrue(result["remote_check"])
        self.assertEqual(self.errors, [("local", "RuntimeError")])

    def test_checks_are_rate_limited_by_monotonic_time(self):
        remote = Mock(return_value={"alerts_created": 0})
        self.remote.check_stale = remote
        self.svc.tick()
        self.monotonic[0] += 29
        self.svc.tick()
        self.assertEqual(remote.call_count, 1)
        self.monotonic[0] += 1
        self.svc.tick()
        self.assertEqual(remote.call_count, 2)

    def test_bad_system_clock_does_not_skip_full_scan(self):
        self.svc.tick()
        self.now[0] -= 300
        result = self.svc.tick()
        self.assertTrue(result["full_scan"])


if __name__ == "__main__":
    unittest.main()
