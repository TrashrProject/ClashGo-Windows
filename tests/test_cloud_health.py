import tempfile
import unittest
from pathlib import Path
from unittest.mock import Mock
from fastapi.testclient import TestClient
from fastapi import FastAPI, HTTPException

from reqngo.core import Guardian
from reqngo.health import guardian_ready, update_worker_heartbeat, worker_healthy
from reqngo.telemetry import TelemetryStore


class CloudHealthTests(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.db = Guardian(Path(self.tmp.name) / "guardian.sqlite3")
        self.telemetry = TelemetryStore(Path(self.tmp.name) / "telemetry.sqlite3")

    def tearDown(self):
        self.db.close()
        self.telemetry.close()
        self.tmp.cleanup()

    def test_readiness_requires_both_databases(self):
        self.assertTrue(guardian_ready(self.db, self.telemetry))
        self.telemetry.close()
        self.assertFalse(guardian_ready(self.db, self.telemetry))

    def test_worker_heartbeat_is_fresh_then_stale(self):
        dest = Path(self.tmp.name) / "worker_heartbeat.txt"
        self.assertFalse(worker_healthy(dest, now=1000))
        update_worker_heartbeat(dest, now=1000)
        self.assertTrue(worker_healthy(dest, now=1050))
        self.assertFalse(worker_healthy(dest, now=1100))
        self.assertFalse(worker_healthy(dest, now=900))

    def test_malformed_local_heartbeat_fails_closed(self):
        dest = Path(self.tmp.name) / "worker_heartbeat.txt"
        dest.write_text("invalid string", encoding="ascii")
        self.assertFalse(worker_healthy(dest))
        dest.write_text("nan", encoding="ascii")
        self.assertFalse(worker_healthy(dest))


if __name__ == "__main__":
    unittest.main()
