import os
import tempfile
import unittest
from pathlib import Path
from unittest.mock import patch

from reqngo import pilot_worker
from reqngo.health import worker_healthy


class WorkerHeartbeatTests(unittest.TestCase):
    def test_process_writes_single_fresh_stamp_after_successful_tick(self):
        with tempfile.TemporaryDirectory() as temp:
            dest = Path(temp)
            events = []
            with patch.dict(os.environ, {
                "GUARDIAN_URL": "http://127.0.0.1:8765",
                "GUARDIAN_AGENT_TOKEN": "t" * 40,
                "PILOT_DATA_DIR": str(dest)
            }), patch.object(pilot_worker, "send", side_effect=lambda *args: events.append(args) or {"ok": True}), \
                    patch.object(pilot_worker, "update_worker_heartbeat",
                                 wraps=pilot_worker.update_worker_heartbeat) as beat:
                self.assertEqual(pilot_worker.main(["--id", "pilot_01", "--profile", "1", "--once"]), 0)
                self.assertEqual(beat.call_count, 1)
            self.assertTrue(worker_healthy(dest / "worker_heartbeat.txt"))
            self.assertEqual(len(events), 3)

    def test_offline_tick_stays_alive_and_outbox_survives(self):
        with tempfile.TemporaryDirectory() as temp:
            dest = Path(temp)
            env = {
                "GUARDIAN_URL": "http://127.0.0.1:8765",
                "GUARDIAN_AGENT_TOKEN": "t" * 40,
                "PILOT_DATA_DIR": str(dest)
            }
            with patch.dict(os.environ, env), patch.object(pilot_worker, "send",
                    side_effect=ConnectionError("network down")):
                self.assertEqual(pilot_worker.main(["--id", "pilot_01", "--profile", "1", "--once"]), 1)
            self.assertTrue(worker_healthy(dest / "worker_heartbeat.txt"))
            self.assertTrue((dest / "outbox.sqlite3").exists())

            delivered = []
            with patch.dict(os.environ, env), patch.object(pilot_worker, "send",
                    side_effect=lambda *args: delivered.append(args) or {"ok": True}):
                self.assertEqual(pilot_worker.main(["--id", "pilot_01", "--profile", "1", "--once"]), 0)
            self.assertEqual(len(delivered), 3)


if __name__ == "__main__":
    unittest.main()
