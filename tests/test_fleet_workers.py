"""Worker-level identity and live HTTP tests for the V10 diagnostic queue."""
import json
import os
import tempfile
import unittest
from pathlib import Path
from unittest.mock import patch

from fastapi import FastAPI
from fastapi.testclient import TestClient

from reqngo.fleet import FleetStore
from reqngo.fleet_api import router_for
from reqngo.fleet_worker import FleetDiagnosticWorker


class DistributedHTTPTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.store = FleetStore(Path(self.temp.name) / "fleet.sqlite3")
        self.store.register_batch("demo", 1, 3)
        self.keys = {
            "fleet_01": "a" * 48,
            "fleet_02": "b" * 48
        }
        self.env = patch.dict(os.environ, {
            "FLEET_WORKER_TOKENS_JSON": json.dumps(self.keys)
        })
        self.env.start()
        app = FastAPI()
        app.include_router(router_for(self.store, lambda: True, lambda: True))
        self.client = TestClient(app)

    def tearDown(self):
        self.client.close()
        self.env.stop()
        self.store.close()
        self.temp.cleanup()

    def req(self, name="fleet_01", token=None):
        return {"Authorization": "Bearer " + (token or self.keys[name]),
                "X-Fleet-Worker-Id": name}

    def test_auth_required_and_impersonation_forbidden(self):
        url = "/api/fleet/worker/claim"
        self.assertEqual(self.client.post(url, json={}).status_code, 401)
        self.assertEqual(self.client.post(url, json={}, headers=self.req(
            "fleet_02", self.keys["fleet_01"])).status_code, 401)
        with patch.dict(os.environ, {"FLEET_WORKER_TOKENS_JSON": ""}):
            self.assertEqual(self.client.post(url, json={}, headers=self.req()).status_code, 503)
        self.assertEqual(self.client.post(url,
                         json={"limit": 100}, headers=self.req()).status_code, 422)

    def test_authenticated_worker_can_claim_and_ack_only_own_lease(self):
        self.store.enqueue_probe("demo-000001", "probe:worker01:2026")
        token = self.client.post("/api/fleet/worker/claim", json={"capacity": 1},
                                 headers=self.req()).json()["items"][0]
        self.assertEqual(self.client.post("/api/fleet/worker/complete",
            json={"job_id": token["job_id"], "lease_token": token["lease_token"],
                  "success": True}, headers=self.req("fleet_02")).status_code, 409)
        self.assertEqual(self.client.post("/api/fleet/worker/complete",
            json={"job_id": token["job_id"], "lease_token": token["lease_token"],
                  "success": True}, headers=self.req()).status_code, 200)
        self.assertEqual(self.client.post("/api/fleet/worker/complete",
            json={"job_id": token["job_id"], "lease_token": token["lease_token"],
                  "success": True}, headers=self.req()).status_code, 409)

    def test_two_slot_worker_and_metrics(self):
        for number in range(1, 4):
            self.store.enqueue_probe(f"demo-{number:06d}",
                                     f"probe:multi:{number:06d}")

        def send(endpoint, payload):
            reply = self.client.post(endpoint, json=payload, headers=self.req())
            self.assertEqual(reply.status_code, 200, reply.text)
            return reply.json()

        worker = FleetDiagnosticWorker(send, Path(self.temp.name) / "own-work",
                                       capacity=2)
        output = worker.step()
        self.assertEqual(output, {"received": 2, "completed": 2, "failed": 0})
        self.assertEqual(self.store.summary()["jobs"]["done"], 2)
        self.assertEqual(self.store.summary()["online_workers"], 1)
        self.assertEqual(len(self.client.get("/api/fleet/workers").json()["items"]), 1)
        self.assertEqual(worker.step()["received"], 1)
        self.assertEqual(self.store.summary()["jobs"]["done"], 3)

    def test_unknown_job_never_executes_and_fails_closed(self):
        sent = []
        worker = FleetDiagnosticWorker(
            lambda endpoint, body: sent.append((endpoint, body)) or {"state": "needs_review"},
            Path(self.temp.name) / "no-execution")
        self.assertFalse(worker._process({
            "kind": "unknown", "job_id": 1, "lease_token": "x" * 48
        }))
        self.assertEqual(sent[0][0], "/api/fleet/worker/complete")
        self.assertFalse(sent[0][1]["success"])
        self.assertFalse((Path(self.temp.name) / "no-execution").exists())


if __name__ == "__main__":
    unittest.main()
