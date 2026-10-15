import os
import tempfile
import unittest
from pathlib import Path
from unittest.mock import Mock

from reqngo.pilot_worker import Outbox, Pilot, validate_endpoint


class PilotWorkerTests(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.root = Path(self.tmp.name)
        self.outbox = Outbox(self.root / "queue.sqlite3")
        self.reports = []
        self.beats = []
        self.pilot = Pilot("pilot_01", 1, self.root / "pilotwork",
                           self.outbox, self.beats.append, self.reports.append)

    def tearDown(self):
        self.outbox.close()
        self.tmp.cleanup()

    def test_missing_directory_is_repaired_then_verified(self):
        result = self.pilot.step()
        self.assertEqual(result["status"], "ok")
        self.assertTrue(result["connected"])
        self.assertTrue(self.pilot.workdir.is_dir())
        self.assertEqual([event["code"] for event in self.reports],
                         ["invalid_config", "probe_success"])
        self.assertEqual(self.reports[1]["metadata"]["error_type"], "invalid_config")
        self.assertEqual(self.beats[0]["agent_id"], "pilot_01")

    def test_outbox_survives_failed_transport_and_restart(self):
        failing = Mock(side_effect=ConnectionError("test network outage"))
        self.pilot.report = failing
        self.assertFalse(self.pilot.step()["connected"])
        self.assertEqual(self.outbox.count(), 2)
        self.outbox.close()
        self.outbox = Outbox(self.root / "queue.sqlite3")
        self.pilot.outbox = self.outbox
        self.pilot.report = self.reports.append
        self.assertTrue(self.pilot.step()["connected"])
        self.assertEqual(self.outbox.count(), 0)
        self.assertEqual(len(self.reports), 2)

    def test_persistent_probe_failure_does_not_stop_other_pilot(self):
        # A file cannot be used as a directory; do not delete it to recover.
        bad = self.root / "not_a_directory"
        bad.write_text("KEEP ME", encoding="utf-8")
        other = Pilot("pilot_02", 2, bad, self.outbox,
                      self.beats.append, self.reports.append)
        result = other.step()
        self.assertEqual(result["status"], "degraded")
        self.assertEqual(bad.read_text(encoding="utf-8"), "KEEP ME")
        self.assertEqual(self.pilot.step()["status"], "ok")

    def test_unknown_storage_error_escalates_without_unsafe_command(self):
        self.pilot.workdir.mkdir()
        with unittest.mock.patch.object(self.pilot, "_probe", side_effect=OSError("disk error")):
            self.assertEqual(self.pilot.step()["status"], "degraded")
            self.assertEqual(self.reports[0]["code"], "pilot_probe_failed")
            self.assertEqual(self.pilot.step()["status"], "degraded")
            self.assertEqual(len(self.reports), 1)

    def test_recovery_event_after_prior_failure(self):
        self.pilot.workdir.mkdir()
        with unittest.mock.patch.object(self.pilot, "_probe", side_effect=OSError("offline")):
            self.pilot.step()
        self.pilot.step()
        self.assertEqual(self.reports[-1]["code"], "probe_success")
        self.assertEqual(self.reports[-1]["metadata"]["error_type"], "pilot_probe_failed")

    def test_endpoint_policy(self):
        self.assertEqual(validate_endpoint("http://localhost:8765"), "http://localhost:8765")
        self.assertEqual(validate_endpoint("https://guardian.example.com"), "https://guardian.example.com")
        with self.assertRaises(ValueError):
            validate_endpoint("http://guardian:8765")
        self.assertEqual(validate_endpoint("http://guardian:8765", True), "http://guardian:8765")
        for url in ("http://evil.example.com", "ftp://guardian", "https://a.example.com/secret",
                    "http://attacker:pass@localhost:8765"):
            with self.assertRaises(ValueError, msg=url):
                validate_endpoint(url)


if __name__ == "__main__":
    unittest.main()
