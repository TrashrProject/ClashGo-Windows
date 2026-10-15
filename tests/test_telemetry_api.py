import os
import tempfile
import unittest
from pathlib import Path
from unittest.mock import patch

from fastapi import FastAPI, HTTPException
from fastapi.testclient import TestClient
from reqngo.telemetry import TelemetryStore
from reqngo.telemetry_api import router_for


class ApiTests(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.store = TelemetryStore(Path(self.tmp.name) / "telemetry.sqlite3")
        self.app = FastAPI()

        def require_admin():
            raise HTTPException(401)

        self.app.include_router(router_for(self.store, require_admin))
        self.client = TestClient(self.app)
        self.patch = patch.dict(os.environ, {"GUARDIAN_AGENT_TOKEN": "x" * 40})
        self.patch.start()

    def tearDown(self):
        self.patch.stop()
        self.client.close()
        self.store.close()
        self.tmp.cleanup()

    def test_auth_and_valid_ingestion(self):
        endpoint = "/api/telemetry/heartbeat"
        data = {"agent_id": "pilot_01", "version": "0_1"}
        self.assertEqual(self.client.post(endpoint, json=data).status_code, 401)
        self.assertEqual(self.client.post(endpoint, json=data,
                         headers={"Authorization": "Bearer bad"}).status_code, 401)
        r = self.client.post(endpoint, json=data,
                             headers={"Authorization": "Bearer " + "x" * 40})
        self.assertEqual(r.status_code, 200)
        self.assertTrue(r.json()["ok"])

    def test_reject_sensitive_payload(self):
        r = self.client.post("/api/telemetry/report", json={
            "agent_id": "pilot_01", "event_key": "evt_1", "code": "worker_timeout",
            "metadata": {"password": "secret"}},
            headers={"Authorization": "Bearer " + "x" * 40})
        self.assertEqual(r.status_code, 422)

    def test_diagnostics_not_public(self):
        self.assertEqual(self.client.get("/api/telemetry/state").status_code, 401)
        self.assertEqual(self.client.get("/api/telemetry/export").status_code, 401)
