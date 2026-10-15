import tempfile
import unittest
from pathlib import Path

from reqngo.core import Guardian


class GuardianTests(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.now = [10000.0]
        self.path = Path(self.tmp.name) / "guardian.sqlite3"
        self.engine = Guardian(self.path, clock=lambda: self.now[0])

    def tearDown(self):
        self.engine.close()
        self.tmp.cleanup()

    def test_fifty_profiles_and_three_pilots(self):
        value = self.engine.overview()
        self.assertEqual(value["total"], 50)
        self.assertEqual(value["pilots"], 3)
        self.assertEqual(value["monitored"], 3)

    def test_repair_does_not_stop_other_profiles(self):
        self.engine.inject(2, "session_frozen")
        self.assertEqual(self.engine.scan()["repaired"], 1)
        self.assertTrue(all(x["status"] == "ok" for x in self.engine.accounts()))

    def test_unknown_fault_escalated_and_isolated(self):
        self.engine.inject(1, "unknown_failure")
        self.engine.inject(2, "worker_crash")
        result = self.engine.scan()
        self.assertEqual(result, {"repaired": 1, "escalated": 1})
        self.assertEqual(self.engine.accounts()[0]["status"], "error")
        self.assertEqual(self.engine.accounts()[1]["status"], "ok")
        self.assertEqual(len(self.engine.incidents()), 2)

    def test_network_backoff(self):
        self.engine.inject(3, "network_timeout")
        self.assertEqual(self.engine.scan()["repaired"], 0)
        self.assertEqual(self.engine.scan()["repaired"], 0)
        self.now[0] += 9
        self.assertEqual(self.engine.scan()["repaired"], 1)

    def test_changes_hot_reload_and_validation(self):
        self.assertEqual(self.engine.configure({"scan_interval_seconds": 90})[
            "scan_interval_seconds"], 90)
        with self.assertRaises(ValueError):
            self.engine.configure({"auto_repair": 1})
        with self.assertRaises(ValueError):
            self.engine.configure({"max_attempts": 99})
        with self.assertRaises(ValueError):
            self.engine.configure({"password": "bad"})

    def test_pilot_selection(self):
        self.engine.set_pilots([10, 11, 12])
        self.assertEqual(self.engine.overview()["pilots"], 3)
        with self.assertRaises(ValueError):
            self.engine.inject(1, "session_frozen")
        with self.assertRaises(ValueError):
            self.engine.set_pilots([10, 10, 11])

    def test_50_profiles_independent(self):
        self.engine.configure({"pilot_mode": False})
        for i in range(1, 51):
            self.engine.inject(i, "worker_crash")
        result = self.engine.scan()
        self.assertEqual(result["repaired"], 50)
        self.assertEqual(self.engine.overview()["healthy"], 50)

    def test_persistence(self):
        self.engine.inject(1, "unknown_failure")
        self.engine.scan(full=True)
        self.engine.close()
        other = Guardian(self.path)
        self.assertEqual(other.overview()["total"], 50)
        self.assertEqual(other.incidents()[0]["status"], "escalated")
        other.close()
        self.engine = Guardian(self.path, clock=lambda: self.now[0])

    def test_disabled_account(self):
        self.engine.enable(1, False)
        with self.assertRaises(ValueError):
            self.engine.inject(1, "worker_crash")
        self.engine.inject(2, "worker_crash")
        self.assertEqual(self.engine.scan()["repaired"], 1)

    def test_no_game_credentials_in_export(self):
        raw = str(self.engine.diagnostics()).lower()
        self.assertNotIn("supercell_id", raw)
        self.assertNotIn("password", raw)


if __name__ == "__main__":
    unittest.main()
