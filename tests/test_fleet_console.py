"""New console: persistent groups, bounded selections and audited confirmations."""
import tempfile
import unittest
from pathlib import Path

from fastapi import FastAPI
from fastapi.testclient import TestClient

from reqngo.fleet import FleetStore
from reqngo.fleet_console import FleetConsole
from reqngo.fleet_console_api import router_for


class FleetConsoleTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.path = Path(self.temp.name) / "fleet.sqlite3"
        self.store = FleetStore(self.path)
        self.store.register_batch("demo", 1, 650)
        self.console = FleetConsole(self.store)

    def tearDown(self):
        self.store.close()
        self.temp.cleanup()

    def test_search_pagination_and_persistent_groups(self):
        self.assertEqual(self.console.create_group("pool-alpha", "Pool Alpha")["group_key"],
                         "pool-alpha")
        self.assertEqual(self.console.groups()[0]["profiles"], 0)
        self.console.assign(["demo-000001", "demo-000002"], "pool-alpha")
        results = self.console.profiles(q="demo-00000", group="pool-alpha", limit=50)
        self.assertEqual(results["total"], 2)
        self.assertEqual(results["items"][0]["group_key"], "pool-alpha")
        self.assertEqual(self.console.profiles(group="ungrouped")["total"], 648)
        self.assertEqual(len(self.console.profiles(offset=600, limit=100)["items"]), 50)
        self.assertEqual(self.console.groups()[0]["enabled"], 2)
        self.console = FleetConsole(self.store)
        self.assertEqual(self.console.groups()[0]["profiles"], 2)
        self.assertEqual(self.console.audit()["items"][0]["affected"], 2)

    def test_exact_confirmation_and_stale_preview_do_not_mutate(self):
        self.console.create_group("pool-alpha", "Pool Alpha")
        self.console.assign(["demo-000001", "demo-000002"], "pool-alpha")
        sample = self.console.preview("pool-alpha", False)
        self.assertEqual(sample["would_change"], 2)
        with self.assertRaises(ValueError):
            self.console.bulk_set_enabled("pool-alpha", False, "SUSPENDRE pool", 2)
        with self.assertRaises(ValueError):
            self.console.bulk_set_enabled("pool-alpha", False,
                                          sample["confirmation"], 1)
        self.assertEqual(self.console.groups()[0]["enabled"], 2)
        applied = self.console.bulk_set_enabled("pool-alpha", False,
                                                sample["confirmation"], 2)
        self.assertEqual(applied["updated"], 2)
        self.assertEqual(self.console.profiles(group="pool-alpha", enabled="no")["total"], 2)
        enabled = self.console.preview("pool-alpha", True)
        self.console.bulk_set_enabled("pool-alpha", True,
                                      enabled["confirmation"], enabled["would_change"])
        self.assertEqual(self.console.groups()[0]["enabled"], 2)
        audit = self.console.audit()["items"]
        self.assertEqual([a["action"] for a in audit[:3]],
                         ["activate", "suspend", "assign"])

    def test_validate_selection_atomically(self):
        self.console.create_group("pool-alpha", "Pool Alpha")
        with self.assertRaises(ValueError):
            self.console.assign(["demo-000001", "demo-999999"], "pool-alpha")
        self.assertEqual(self.console.profiles(group="pool-alpha")["total"], 0)
        with self.assertRaises(ValueError):
            self.console.assign(["demo-000001"] * 2, "pool-alpha")
        with self.assertRaises(ValueError):
            self.console.assign([f"demo-{i:06d}" for i in range(1, 202)],
                                "pool-alpha")
        with self.assertRaises(ValueError):
            self.console.create_group("pool-alpha", "Duplicate")
        with self.assertRaises(ValueError):
            self.console.profiles(q="%")
        with self.assertRaises(ValueError):
            self.console.preview("missing", False)

    def test_bulk_suspend_does_not_cancel_existing_leases(self):
        self.console.create_group("pool-alpha", "Pool Alpha")
        self.console.assign(["demo-000001"], "pool-alpha")
        self.store.enqueue_probe("demo-000001", "probe:already:0001")
        lease = self.store.claim("worker_01")[0]
        preview = self.console.preview("pool-alpha", False)
        self.console.bulk_set_enabled("pool-alpha", False,
                                      preview["confirmation"], preview["would_change"])
        self.assertEqual(self.store.complete(lease["job_id"], "worker_01",
                         lease["lease_token"], True)["state"], "done")

    def test_http_filter_guard_limits_and_scope(self):
        app = FastAPI()
        app.include_router(router_for(self.console, lambda: True, lambda: True))
        with TestClient(app) as client:
            self.assertEqual(client.post("/api/fleet/console/groups",
                json={"group_key": "test-pool", "label": "Test Pool"}).status_code, 200)
            self.assertEqual(client.get("/api/fleet/console/profiles?limit=101").status_code, 422)
            self.assertEqual(client.get("/api/fleet/console/groups").json()
                             ["items"][0]["group_key"], "test-pool")
            self.assertEqual(client.post("/api/fleet/console/assign",
                json={"profile_keys": ["demo-000001"], "group_key": "test-pool"}
            ).status_code, 200)
            preview = client.get(
                "/api/fleet/console/groups/test-pool/preview?enabled=false").json()
            self.assertEqual(preview["would_change"], 1)
            self.assertEqual(client.post(
                "/api/fleet/console/groups/test-pool/enabled",
                json={"enabled": False, "confirmation": "SUSPENDRE test-pool",
                      "expected_count": 1}).status_code, 200)
            self.assertEqual(client.get("/api/fleet/console/audit").status_code, 200)
            self.assertEqual(client.post("/api/fleet/console/assign",
                json={"profile_keys": ["demo-000002"], "group_key": "other"}
            ).status_code, 422)


if __name__ == "__main__":
    unittest.main()
