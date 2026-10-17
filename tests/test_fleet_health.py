"""V12: durable worker-outage incidents, dedupe, recovery and periodic scans."""
from __future__ import annotations

import tempfile
import unittest
from pathlib import Path
from fastapi import FastAPI
from fastapi.testclient import TestClient

from reqngo.fleet import FleetStore
from reqngo.fleet_api import router_for
from reqngo.fleet_monitor import FleetHealthSupervisor


class WorkerHealthTests(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.now = [1000.0]
        self.store = FleetStore(Path(self.tmp.name) / "fleet.sqlite3",
                                clock=lambda: self.now[0])

    def tearDown(self):
        self.store.close()
        self.tmp.cleanup()

    def test_outage_is_logged_once_then_automatically_resolved(self):
        self.store.heartbeat("fleet_01", "ok", 2)
        self.store.heartbeat("fleet_02", "ok", 1)
        self.now[0] += 100
        self.store.heartbeat("fleet_02")
        self.now[0] += 81
        first = self.store.scan_worker_health()
        self.assertEqual(first, {"new_incidents": 1, "open_incidents": 1})
        self.assertEqual(self.store.scan_worker_health()["new_incidents"], 0)
        incident = self.store.worker_incidents()[0]
        self.assertEqual(incident["worker_id"], "fleet_01")
        self.assertEqual(incident["occurrences"], 1)
        self.assertEqual(self.store.summary()["online_workers"], 1)
        self.assertEqual(self.store.summary()["online_slots"], 1)
        self.assertEqual(self.store.summary()["open_worker_incidents"], 1)
        self.store.heartbeat("fleet_01", "degraded", 2)
        self.assertEqual(self.store.scan_worker_health()["open_incidents"], 0)
        self.assertEqual(self.store.worker_incidents()[0]["status"], "resolved")
        self.now[0] += 181
        self.assertEqual(self.store.scan_worker_health()["new_incidents"], 2)
        resumed = {r["worker_id"]: r for r in self.store.worker_incidents()}
        self.assertEqual(resumed["fleet_01"]["occurrences"], 2)

    def test_queue_pressure_is_bounded_summary_not_raw_jobs(self):
        self.store.register_batch("demo", 1, 2)
        self.store.enqueue_probe("demo-000001", "probe:health:0001")
        self.now[0] += 45
        stats = self.store.summary()
        self.assertEqual(stats["oldest_ready_wait_seconds"], 45)
        self.assertEqual(stats["ready_jobs"], 1)
        lease = self.store.claim("worker_01")[0]
        self.assertEqual(self.store.summary()["oldest_ready_wait_seconds"], 0)
        self.store.complete(lease["job_id"], "worker_01", lease["lease_token"], True)
        self.assertEqual(self.store.summary()["jobs"]["done"], 1)

    def test_periodic_scan_failure_does_not_crash_other_supervisors(self):
        monotonic = [0.0]
        errors = []
        supervisor = FleetHealthSupervisor(
            self.store, interval=30, monotonic_clock=lambda: monotonic[0],
            on_error=lambda component, kind: errors.append((component, kind)))
        self.store.heartbeat("fleet_01")
        self.assertTrue(supervisor.tick()["checked"])
        self.assertFalse(supervisor.tick()["checked"])
        monotonic[0] = 30
        self.assertTrue(supervisor.tick()["checked"])
        broken = FleetHealthSupervisor(
            object(), interval=30, on_error=lambda a, b: errors.append((a, b)))
        self.assertTrue(broken.tick()["failed"])
        self.assertEqual(errors, [("fleet", "AttributeError")])
        with self.assertRaises(ValueError):
            FleetHealthSupervisor(self.store, interval=2)

    def test_incidents_api_is_paginated_and_does_not_expose_tokens(self):
        self.store.heartbeat("fleet_01")
        self.now[0] += 181
        self.store.scan_worker_health()
        app = FastAPI()
        app.include_router(router_for(self.store, lambda: True, lambda: True))
        with TestClient(app) as client:
            response = client.get("/api/fleet/incidents?limit=1")
            self.assertEqual(response.status_code, 200, response.text)
            self.assertEqual(response.json()["items"][0]["worker_id"], "fleet_01")
            self.assertNotIn("lease_token", response.text)
            self.assertEqual(client.get("/api/fleet/incidents?limit=1000").status_code, 422)


if __name__ == "__main__":
    unittest.main()
