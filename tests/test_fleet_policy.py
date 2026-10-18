"""V16 inherited worker settings and read-only profile diagnostic verification."""
import tempfile
import unittest
from pathlib import Path

from fastapi import FastAPI
from fastapi.testclient import TestClient

from reqngo.fleet import FleetStore
from reqngo.fleet_console_api import router_for
from reqngo.fleet_policy import FleetPolicyConsole


class FleetPolicyTests(unittest.TestCase):
    def setUp(self):
        self.directory = tempfile.TemporaryDirectory()
        self.database = Path(self.directory.name) / "fleet.sqlite3"
        self.store = FleetStore(self.database)
        self.store.register_batch("demo", 1, 5)
        self.store.heartbeat("worker_01", "ok", 2)
        self.store.heartbeat("worker_02", "ok", 2)
        self.policy = FleetPolicyConsole(self.store)
        self.policy.create_group("alpha", "Pool Alpha")
        self.policy.assign(["demo-000001", "demo-000002"], "alpha")

    def tearDown(self):
        self.store.close()
        self.directory.cleanup()

    def test_inheritance_override_and_reassignment(self):
        self.assertEqual(self.policy.profile_detail("demo-000001")["effective_interval_seconds"], 600)
        self.policy.set_group_policy("alpha", "worker_01", 300)
        first = self.policy.profile_detail("demo-000001")
        self.assertEqual(first["effective_worker"], "worker_01")
        self.assertEqual(first["effective_interval_seconds"], 300)
        self.assertEqual(first["worker_source"], "group")
        self.policy.set_profile_override("demo-000001", "worker_02", 75)
        first = self.policy.profile_detail("demo-000001")
        second = self.policy.profile_detail("demo-000002")
        self.assertEqual(first["effective_worker"], "worker_02")
        self.assertEqual(first["interval_source"], "profile")
        self.assertEqual(second["effective_worker"], "worker_01")
        self.assertEqual(second["effective_interval_seconds"], 300)
        self.policy.set_profile_override("demo-000001", None, None)
        self.assertEqual(self.policy.profile_detail("demo-000001")["effective_worker"],
                         "worker_01")
        self.policy.assign(["demo-000001"], None)
        self.assertIsNone(self.policy.profile_detail("demo-000001")["effective_worker"])
        self.assertEqual(self.policy.profile_detail("demo-000001")["effective_interval_seconds"], 600)

    def test_unknown_workers_and_invalid_config_refused(self):
        with self.assertRaises(ValueError):
            self.policy.set_group_policy("alpha", "not_known", 60)
        with self.assertRaises(ValueError):
            self.policy.set_group_policy("alpha", "worker_01", 5)
        with self.assertRaises(ValueError):
            self.policy.set_group_policy("missing", None, 600)
        with self.assertRaises(ValueError):
            self.policy.set_profile_override("demo-999999", None, None)
        with self.assertRaises(ValueError):
            self.policy.set_profile_override("demo-000001", "worker_01", 3601)
        self.assertEqual(self.policy.group_policy("alpha")["source"], "defaults")

    def test_diagnostic_uses_preferred_worker_and_sanitizes_history(self):
        self.policy.set_group_policy("alpha", "worker_01", 90)
        result = self.policy.enqueue_profile_diagnostic("demo-000001", "storage")
        self.assertEqual(result["target_worker"], "worker_01")
        self.assertEqual(self.store.claim("worker_02"), [])
        lease = self.store.claim("worker_01")[0]
        self.assertEqual(lease["job_id"], result["job_id"])
        self.store.complete(lease["job_id"], "worker_01", lease["lease_token"], True)
        detail = self.policy.profile_detail("demo-000001")
        self.assertEqual(detail["recent_jobs"][0]["state"], "done")
        self.assertNotIn("lease_token", str(detail))
        self.assertNotIn("dedupe_key", str(detail))

    def test_diagnostic_unknown_offline_or_disabled(self):
        with self.assertRaises(ValueError):
            self.policy.enqueue_profile_diagnostic("demo-000003", "android")
        self.policy.set_group_policy("alpha", "worker_01", 300)
        self.store.set_enabled("demo-000001", False)
        with self.assertRaises(ValueError):
            self.policy.enqueue_profile_diagnostic("demo-000001", "storage")
        with self.assertRaises(ValueError):
            self.policy.enqueue_profile_diagnostic("demo-000002", "wrong")
        self.store.set_enabled("demo-000001", True)
        self.assertEqual(self.policy.enqueue_profile_diagnostic(
            "demo-000001", "android")["target_worker"], "worker_01")
        # Existing leases are unaffected by later group settings.
        lease = self.store.claim("worker_01")[0]
        self.policy.set_group_policy("alpha", "worker_02", 90)
        self.assertEqual(lease["target_worker"], "worker_01")

    def test_configuration_survives_restart(self):
        self.policy.set_group_policy("alpha", "worker_01", 150)
        self.policy.set_profile_override("demo-000001", None, 180)
        self.policy = FleetPolicyConsole(self.store)
        detail = self.policy.profile_detail("demo-000001")
        self.assertEqual(detail["effective_interval_seconds"], 180)
        self.assertEqual(detail["effective_worker"], "worker_01")
        self.assertTrue(any(item["action"] == "profile_override"
                            for item in self.policy.audit()["items"]))

    def test_http_endpoints_and_rejected_extra_fields(self):
        app = FastAPI()
        app.include_router(router_for(self.policy, lambda: True, lambda: True))
        with TestClient(app) as client:
            endpoint = "/api/fleet/console/groups/alpha/policy"
            self.assertEqual(client.get(endpoint).json()["source"], "defaults")
            response = client.put(endpoint, json={
                "preferred_worker": "worker_01", "probe_interval_seconds": 120})
            self.assertEqual(response.status_code, 200, response.text)
            self.assertEqual(client.put(endpoint, json={
                "preferred_worker": "worker_02",
                "probe_interval_seconds": 100, "credentials": "secret"
            }).status_code, 422)
            profile = "/api/fleet/console/profiles/demo-000001"
            self.assertEqual(client.put(profile + "/override",
                json={"preferred_worker": "worker_02",
                      "probe_interval_seconds": 60}).status_code, 200)
            self.assertEqual(client.get(profile + "/detail").json()
                             ["effective_worker"], "worker_02")
            diagnostic = client.post(profile + "/diagnostic",
                                     json={"probe_mode": "storage"})
            self.assertEqual(diagnostic.status_code, 200, diagnostic.text)
            self.assertEqual(diagnostic.json()["target_worker"], "worker_02")
            self.assertEqual(client.post(profile + "/diagnostic",
                                         json={"probe_mode": "swipe"}).status_code, 422)
            self.assertEqual(client.get(profile + "/detail").status_code, 200)
            self.assertNotIn("lease_token", client.get(profile + "/detail").text)


if __name__ == "__main__":
    unittest.main()
