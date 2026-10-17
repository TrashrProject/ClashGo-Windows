import tempfile
import unittest
from pathlib import Path

from reqngo.correlation import correlate
from reqngo.telemetry import TelemetryStore


class CorrelationTests(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.now = [1000.0]
        self.db = TelemetryStore(Path(self.tmp.name) / "telemetry.sqlite3",
                                 clock=lambda: self.now[0])

    def tearDown(self):
        self.db.close()
        self.tmp.cleanup()

    def codes(self):
        return [i["code"] for i in correlate(self.db.snapshot())["insights"]]

    def test_no_agents_is_explicit(self):
        self.assertEqual(self.codes(), ["no_pilots_connected"])

    def test_single_lost_pilot_recommends_isolation(self):
        for i in range(1, 4):
            self.db.heartbeat("pilot_0" + str(i))
        self.now[0] += 180
        self.db.heartbeat("pilot_01")
        self.db.heartbeat("pilot_03")
        self.assertEqual(self.codes(), ["isolated_connectivity_loss"])

    def test_shared_outage_does_not_trigger_mass_restart(self):
        for i in range(1, 4):
            self.db.heartbeat("pilot_0" + str(i))
        self.now[0] += 190
        self.db.heartbeat("pilot_03")
        output = correlate(self.db.snapshot())
        self.assertEqual(output["insights"][0]["code"], "shared_connectivity_suspected")
        self.assertEqual(output["insights"][0]["affected_count"], 2)
        self.assertIsNone(output["insights"][0]["automatic_action"])
        self.assertEqual(output["automatic_actions_executed"], 0)

    def test_shared_error_on_different_workers(self):
        for i in (1, 2):
            aid = "pilot_0" + str(i)
            self.db.heartbeat(aid)
            self.db.report(aid, "ev_" + str(i), "worker_timeout",
                           profile_id=i, level="error")
        self.assertIn("shared_error_signature", self.codes())

    def test_one_worker_repeating_fault_is_not_shared_outage(self):
        self.db.heartbeat("pilot_01")
        for i in range(4):
            self.db.report("pilot_01", "e_" + str(i), "pilot_probe_failed",
                           profile_id=1, level="error")
        codes = self.codes()
        self.assertIn("repeating_fault", codes)
        self.assertNotIn("shared_error_signature", codes)

    def test_unknown_critical_cannot_request_an_automatic_command(self):
        self.db.heartbeat("pilot_01")
        self.db.report("pilot_01", "ev_1", "unexpected_failure",
                       profile_id=1, level="critical")
        output = correlate(self.db.snapshot())
        self.assertIn("critical_case_unresolved", [i["code"] for i in output["insights"]])
        self.assertTrue(all(x["automatic_action"] is None for x in output["insights"]))

    def test_all_healthy_is_not_misrepresented_as_guaranteed(self):
        self.db.heartbeat("pilot_01")
        item = correlate(self.db.snapshot())["insights"][0]
        self.assertEqual(item["code"], "no_active_warnings")
        self.assertIn("ne garantit", item["recommendation"])

    def test_untrusted_cases_cannot_leak_content(self):
        snapshot = {
            "agents": [{"agent_id": "pilot_01", "connection": "ok"}],
            "cases": [{"agent_id": "secret@user.invalid", "code": "xyz",
                       "status": "open", "severity": "critical",
                       "details": "TOKEN_DONT_SHOW", "occurrences": 99}],
        }
        output = correlate(snapshot)
        self.assertNotIn("TOKEN_DONT_SHOW", str(output))
        self.assertNotIn("secret@user.invalid", str(output))


if __name__ == "__main__":
    unittest.main()
