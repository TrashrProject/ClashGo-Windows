import json
import tempfile
import unittest
from pathlib import Path
from unittest.mock import patch

from tools import compose_smoke


class SanitizedDiagnosticsTests(unittest.TestCase):
    def test_report_only_contains_safe_allowlisted_fields(self):
        with tempfile.TemporaryDirectory() as temp, patch.object(
                compose_smoke, "REPORT_PATH", Path(temp) / "diagnostics.json"):
            snapshot = {
                "agents": [{"agent_id": "pilot_01", "connection": "ok",
                            "raw_password": "TOP_SECRET_VALUE"},
                           {"agent_id": "other_service", "connection": "ok",
                            "user_email": "sensitive@example.com"}],
                "events": [{"code": "probe_success", "profile_id": 1,
                            "metadata": {"password": "TOP_SECRET_VALUE"}},
                           {"code": "some_other_code", "profile_id": 99}],
                "summary": {"open_cases": 0, "token": "TOP_SECRET_VALUE"},
            }
            result = compose_smoke.save_report("success", state=snapshot)
            content = compose_smoke.REPORT_PATH.read_text(encoding="utf-8")
            self.assertNotIn("TOP_SECRET_VALUE", content)
            self.assertNotIn("sensitive@example.com", content)
            self.assertNotIn("other_service", content)
            self.assertEqual(result["verified_probe_profiles"], [1])
            self.assertEqual(result["open_incidents"], 0)
            self.assertEqual(result["pilots"][0]["status"], "ok")
            self.assertEqual(len(result["pilots"]), 3)
            self.assertFalse(result["game_connected"])

    def test_report_on_failure_is_machine_readable(self):
        with tempfile.TemporaryDirectory() as temp, patch.object(
                compose_smoke, "REPORT_PATH", Path(temp) / "fail.json"):
            result = compose_smoke.save_report(
                "failed", reason="waiting_heartbeats",
                state={"agents": [], "events": [], "summary": {}})
            parsed = json.loads(compose_smoke.REPORT_PATH.read_text(encoding="utf-8"))
            self.assertEqual(parsed, result)
            self.assertEqual(parsed["result"], "failed")
            self.assertEqual(parsed["pilots"][2]["status"], "missing")

    def test_untrusted_failure_message_is_not_persisted(self):
        with tempfile.TemporaryDirectory() as temp, patch.object(
                compose_smoke, "REPORT_PATH", Path(temp) / "fail.json"):
            result = compose_smoke.save_report(
                "failed", reason="Authorization Bearer SOME_SECRET_KEY")
            self.assertEqual(result["reason"], "ready_timeout")
            self.assertNotIn("SOME_SECRET_KEY", compose_smoke.REPORT_PATH.read_text())


if __name__ == "__main__":
    unittest.main()
