"""Fleet HTTP contract tests: bounded input and no remote lease exposure."""
import tempfile
import unittest
from pathlib import Path

from fastapi import FastAPI
from fastapi.testclient import TestClient

from reqngo.fleet import FleetStore
from reqngo.fleet_api import router_for


class FleetAPITests(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.store = FleetStore(Path(self.tmp.name) / "fleet.sqlite3")
        app = FastAPI()
        app.include_router(router_for(self.store, lambda: True, lambda: True))
        self.client = TestClient(app)

    def tearDown(self):
        self.client.close()
        self.store.close()
        self.tmp.cleanup()

    def test_create_list_and_queue(self):
        r = self.client.post("/api/fleet/profiles/batch",
                             json={"prefix": "demo", "start": 1, "count": 500})
        self.assertEqual(r.status_code, 200, r.text)
        self.assertEqual(r.json()["created"], 500)
        r = self.client.get("/api/fleet/summary")
        self.assertEqual(r.json()["profiles"], 500)
        r = self.client.get("/api/fleet/profiles?offset=490&limit=25")
        self.assertEqual(len(r.json()["items"]), 10)
        r = self.client.post("/api/fleet/probes",
                             json={"profile_key": "demo-000001",
                                   "dedupe_key": "request-demo-0001"})
        self.assertEqual(r.status_code, 200, r.text)
        self.assertEqual(self.client.get("/api/fleet/summary")
                         .json()["jobs"]["queued"], 1)

    def test_guards_and_bounded_inputs(self):
        self.assertEqual(self.client.get("/api/fleet/profiles?limit=99999").status_code, 422)
        self.assertEqual(self.client.post("/api/fleet/profiles/batch",
                         json={"prefix": "demo", "start": 1, "count": 10001}).status_code, 422)
        self.assertEqual(self.client.post("/api/fleet/profiles/batch",
                         json={"prefix": "demo", "start": 1, "count": 1,
                               "password": "should-never-be-accepted"}).status_code, 422)
        self.assertEqual(self.client.post("/api/fleet/probes",
                         json={"profile_key": "demo-000001",
                               "dedupe_key": "short"}).status_code, 422)
        self.assertEqual(self.client.post("/api/fleet/claim", json={}).status_code, 404)


if __name__ == "__main__":
    unittest.main()
