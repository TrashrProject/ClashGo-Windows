import tempfile
import unittest
from pathlib import Path

from reqngo.telemetry import TelemetryStore
from reqngo.triage import classify


class DiagnosticCaseTests(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.now = [1000.]
        self.path = Path(self.tmp.name) / "telemetry.sqlite3"
        self.store = TelemetryStore(self.path, clock=lambda: self.now[0])

    def tearDown(self):
        self.store.close()
        self.tmp.cleanup()

    def test_incident_is_grouped_by_agent_profile_and_code(self):
        self.store.report("pilot_1", "evt_001", "worker_timeout", "warning", 1)
        self.store.report("pilot_1", "evt_002", "worker_timeout", "error", 1)
        self.store.report("pilot_2", "evt_003", "worker_timeout", "warning", 1)
        snap = self.store.snapshot()
        self.assertEqual(snap["summary"]["open_cases"], 2)
        first = next(c for c in snap["cases"] if c["agent_id"] == "pilot_1")
        self.assertEqual(first["occurrences"], 2)
        self.assertEqual(first["severity"], "error")

    def test_probe_closes_only_matching_case(self):
        self.store.report("pilot_1", "evt_001", "worker_timeout", profile_id=2)
        self.store.report("pilot_1", "evt_002", "invalid_config", profile_id=2)
        self.store.report("pilot_1", "evt_003", "probe_success", level="info",
                          profile_id=2, metadata={"error_type": "worker_timeout"})
        snap = self.store.snapshot()
        self.assertEqual(snap["summary"]["open_cases"], 1)
        statuses = {c["code"]: c["status"] for c in snap["cases"]}
        self.assertEqual(statuses["worker_timeout"], "verified")
        self.assertEqual(statuses["invalid_config"], "open")

    def test_invalid_probe_does_not_close_incident(self):
        self.store.report("pilot_1", "evt_001", "worker_timeout", profile_id=2)
        with self.assertRaises(ValueError):
            self.store.report("pilot_1", "evt_002", "probe_success", level="info",
                              profile_id=2)
        self.assertEqual(self.store.snapshot()["summary"]["open_cases"], 1)

    def test_stale_recovery_is_verified_only_by_new_heartbeat(self):
        self.store.heartbeat("pilot_1")
        self.store.report("pilot_1", "evt_001", "invalid_config", profile_id=1)
        self.now[0] += 200
        self.store.check_stale()
        self.assertEqual(self.store.snapshot()["summary"]["open_cases"], 2)
        self.store.heartbeat("pilot_1", status="degraded")
        snapshot = self.store.snapshot()
        self.assertEqual(snapshot["summary"]["open_cases"], 1)
        cases = {c["code"]: c for c in snapshot["cases"]}
        self.assertEqual(cases["worker_unreachable"]["status"], "verified")
        self.assertEqual(cases["invalid_config"]["status"], "open")

    def test_unknown_incident_requires_review(self):
        self.store.report("pilot_1", "evt_001", "not_classified", "critical")
        item = self.store.snapshot()["cases"][0]
        self.assertEqual(item["category"], "À investiguer")
        self.assertEqual(item["severity"], "critical")
        self.assertNotIn("powershell", item["recommended_action"].lower())
        self.assertIsNone(classify("not_classified", "critical")["automatic_action"])

    def test_reopened_incident_creates_new_case_after_verified(self):
        self.store.report("pilot_1", "evt_001", "worker_crash", profile_id=1)
        self.store.report("pilot_1", "evt_002", "probe_success", "info", 1,
                          {"error_type": "worker_crash"})
        self.store.report("pilot_1", "evt_003", "worker_crash", profile_id=1)
        snap = self.store.snapshot()
        self.assertEqual(len(snap["cases"]), 2)
        self.assertEqual(snap["summary"]["open_cases"], 1)


if __name__ == "__main__":
    unittest.main()
