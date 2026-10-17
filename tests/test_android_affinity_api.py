"""V11 HTTP: targeted ADB queue must be opt-in and fully validated."""
import tempfile
import unittest
from pathlib import Path

from fastapi import FastAPI
from fastapi.testclient import TestClient

from reqngo.fleet import FleetStore
from reqngo.fleet_api import router_for


class AndroidAPIContractTests(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.store = FleetStore(Path(self.tmp.name) / "fleet.sqlite3")
        self.store.register_batch("demo", 1, 2)
        app = FastAPI()
        app.include_router(router_for(self.store, lambda: True, lambda: True))
        self.http = TestClient(app)

    def tearDown(self):
        self.http.close()
        self.store.close()
        self.tmp.cleanup()

    def test_android_requires_target_in_admin_request(self):
        body = {"profile_key": "demo-000001", "dedupe_key": "android:req:00001",
                "probe_mode": "android"}
        response = self.http.post("/api/fleet/probes", json=body)
        self.assertEqual(response.status_code, 422)
        self.assertEqual(self.store.summary()["jobs"]["queued"], 0)
        response = self.http.post("/api/fleet/probes",
                                  json=dict(body, target_worker="worker_adb"))
        self.assertEqual(response.status_code, 200)
        self.assertEqual(self.store.claim("worker_other"), [])
        job = self.store.claim("worker_adb")[0]
        self.assertEqual(job["probe_mode"], "android")

    def test_default_storage_still_compatible(self):
        response = self.http.post("/api/fleet/probes", json={
            "profile_key": "demo-000002", "dedupe_key": "storage:req:0001"
        })
        self.assertEqual(response.status_code, 200)
        job = self.store.claim("worker_other")[0]
        self.assertEqual(job["probe_mode"], "storage")
        self.assertIsNone(job["target_worker"])

    def test_unknown_mode_rejected(self):
        response = self.http.post("/api/fleet/probes", json={
            "profile_key": "demo-000001", "dedupe_key": "android:req:00002",
            "probe_mode": "execute_game", "target_worker": "worker_adb"
        })
        self.assertEqual(response.status_code, 422)
        self.assertEqual(self.store.summary()["jobs"]["queued"], 0)


if __name__ == "__main__":
    unittest.main()
