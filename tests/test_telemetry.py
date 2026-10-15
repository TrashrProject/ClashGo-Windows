import tempfile
import unittest
from pathlib import Path

from reqngo.telemetry import TelemetryStore


class TelemetryTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.path = Path(self.temp.name) / "telemetry.sqlite3"
        self.now = [1000.0]
        self.db = TelemetryStore(self.path, clock=lambda: self.now[0])

    def tearDown(self):
        self.db.close()
        self.temp.cleanup()

    def test_heartbeat_and_stale_detection(self):
        self.db.heartbeat("pilot_01", version="0_1")
        self.assertEqual(self.db.snapshot()["summary"]["connected"], 1)
        self.now[0] += 181
        self.assertEqual(self.db.snapshot()["agents"][0]["connection"], "stale")
        self.db.heartbeat("pilot_01")
        self.assertEqual(self.db.snapshot()["summary"]["stale"], 0)

    def test_report_idempotent(self):
        params = ("pilot_01", "event_123", "worker_timeout")
        self.assertFalse(self.db.report(*params, profile_id=2, metadata={"retry_count": 1})["duplicate"])
        self.assertTrue(self.db.report(*params)["duplicate"])
        self.assertEqual(len(self.db.snapshot()["events"]), 1)

    def test_only_structured_diagnostics_accepted(self):
        for value in ({"password": "secret"}, {"trace_id": "john@example.com"},
                      {"component": "a" * 81}):
            with self.assertRaises(ValueError):
                self.db.report("pilot_01", "evt_123", "worker_timeout", metadata=value)
        self.assertEqual(self.db.snapshot()["events"], [])

    def test_invalid_agent_and_payload(self):
        with self.assertRaises(ValueError):
            self.db.heartbeat("somebody@gmail.com")
        with self.assertRaises(ValueError):
            self.db.report("pilot_01", "event_01", "invalid code")
        with self.assertRaises(ValueError):
            self.db.report("pilot_01", "event_01", "valid_code", profile_id=-1)

    def test_persistence_after_restart(self):
        self.db.heartbeat("pilot_01")
        self.db.report("pilot_01", "event_01", "worker_timeout", profile_id=17)
        self.db.close()
        self.db = TelemetryStore(self.path, clock=lambda: self.now[0])
        snap = self.db.snapshot()
        self.assertEqual(snap["summary"]["connected"], 1)
        self.assertEqual(snap["events"][0]["profile_id"], 17)

    def test_separate_worker_incidents(self):
        self.db.heartbeat("pilot_01")
        self.db.heartbeat("pilot_02")
        self.db.report("pilot_01", "event_1", "worker_timeout")
        self.assertEqual(self.db.snapshot()["summary"]["connected"], 2)
        self.assertEqual(self.db.snapshot()["events"][0]["agent_id"], "pilot_01")

    def test_reject_event_key_collision_between_agents(self):
        self.db.report("pilot_01", "event_same", "worker_timeout")
        with self.assertRaises(ValueError):
            self.db.report("pilot_02", "event_same", "worker_timeout")


if __name__ == "__main__":
    unittest.main()
