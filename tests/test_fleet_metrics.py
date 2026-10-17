"""V14: safe resource metrics collection, migrations and operator visibility."""
import math
import sqlite3
import tempfile
import unittest
from pathlib import Path
from types import SimpleNamespace
from unittest.mock import Mock

from fastapi import FastAPI
from fastapi.testclient import TestClient

from reqngo.fleet import FleetStore
from reqngo.fleet_api import router_for
from reqngo.fleet_capacity import assess_fleet_capacity
from reqngo.fleet_metrics import WorkerResourceSampler, clean_metrics
from reqngo.fleet_worker import FleetDiagnosticWorker


class ResourceMetricsTests(unittest.TestCase):
    def test_metrics_bounds_and_nullable_defaults(self):
        blank = clean_metrics({})
        self.assertEqual(set(blank), {
            "cpu_percent", "memory_percent", "process_rss_mb",
            "android_online_devices", "android_bound_devices"
        })
        self.assertTrue(all(v is None for v in blank.values()))
        for bad in [
            {"cpu_percent": True}, {"cpu_percent": 101},
            {"cpu_percent": math.nan}, {"memory_percent": float("inf")},
            {"process_rss_mb": -10}, {"android_online_devices": 1.2},
            {"android_online_devices": 3, "android_bound_devices": 2},
            {"device_serial": "emulator-5554"}, {"memory_percent": "70"}
        ]:
            with self.subTest(bad=bad), self.assertRaises(ValueError):
                clean_metrics(bad)

    def test_sampler_reports_only_numeric_data_without_paths_or_serials(self):
        psutil = Mock()
        psutil.cpu_percent.return_value = 41.2
        psutil.virtual_memory.return_value = SimpleNamespace(percent=61.4)
        psutil.Process.return_value.memory_info.return_value = SimpleNamespace(
            rss=104857600)
        adb = Mock()
        adb.bindings = {
            "demo-000001": "emulator-5554",
            "demo-000002": "emulator-5554",
            "demo-000003": "emulator-5556"
        }
        adb.client_class.devices.return_value = ["emulator-5554", "external-device"]
        adb.adb_binary = "adb"
        sampler = WorkerResourceSampler(
            android_preflight=adb, enable_adb=True, psutil_module=psutil)
        sample = sampler.sample()
        self.assertEqual(sample["cpu_percent"], 41.2)
        self.assertEqual(sample["memory_percent"], 61.4)
        self.assertEqual(sample["process_rss_mb"], 100)
        self.assertEqual(sample["android_bound_devices"], 2)
        self.assertEqual(sample["android_online_devices"], 1)
        self.assertNotIn("emulator-", str(sample))
        self.assertNotIn("serial", str(sample))
        adb.client_class.devices.assert_called_once_with(adb_binary="adb")

    def test_adb_inventory_is_opt_in_and_offline_is_unknown(self):
        adb = Mock()
        adb.bindings = {"demo-000001": "emulator-5554"}
        adb.adb_binary = "adb"
        sampler = WorkerResourceSampler(
            android_preflight=adb, enable_adb=False, psutil_module=Mock())
        sampler.sample()
        adb.client_class.devices.assert_not_called()
        adb.client_class.devices.side_effect = OSError("ADB unavailable")
        offline = WorkerResourceSampler(
            android_preflight=adb, enable_adb=True, psutil_module=Mock())
        # Configure valid psutil results so only the ADB check fails.
        offline.psutil.cpu_percent.return_value = 3
        offline.psutil.virtual_memory.return_value.percent = 8
        offline.psutil.Process.return_value.memory_info.return_value.rss = 10
        output = offline.sample()
        self.assertIsNone(output["android_online_devices"])
        self.assertEqual(output["android_bound_devices"], 1)

    def test_high_load_is_advisory_not_an_automated_game_command(self):
        summary = {"ready_jobs": 0, "online_slots": 1, "online_workers": 1,
                   "open_worker_incidents": 0, "oldest_ready_wait_seconds": 0,
                   "overloaded_workers": 1,
                   "jobs": {"leased": 0, "needs_review": 0}}
        self.assertEqual(assess_fleet_capacity(summary)["state"], "worker_overload")


class ResourcePersistenceTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.path = Path(self.temp.name) / "fleet.sqlite3"
        self.store = FleetStore(self.path)

    def tearDown(self):
        self.store.close()
        self.temp.cleanup()

    def test_heartbeat_metrics_and_legacy_null(self):
        self.store.heartbeat("fleet_01", "busy", 2, metrics={
            "cpu_percent": 97.1, "memory_percent": 72.5,
            "process_rss_mb": 123.1, "android_online_devices": 1,
            "android_bound_devices": 2
        })
        self.store.heartbeat("fleet_02", "ok", 1)
        rows = {x["worker_id"]: x for x in self.store.workers()}
        self.assertEqual(rows["fleet_01"]["android_bound_devices"], 2)
        self.assertEqual(rows["fleet_01"]["process_rss_mb"], 123.1)
        self.assertEqual(rows["fleet_02"]["cpu_percent"], None)
        self.assertEqual(self.store.summary()["overloaded_workers"], 1)
        self.store.heartbeat("fleet_01", "ok", 2)
        self.assertIsNone(self.store.workers()[0]["cpu_percent"])
        self.assertEqual(self.store.summary()["overloaded_workers"], 0)

    def test_authenticated_http_rejects_untrusted_samples(self):
        app = FastAPI()
        app.include_router(router_for(self.store, lambda: True, lambda: True))
        # Uses only in-process test authentication logic, no live credentials.
        import json
        import os
        from unittest.mock import patch
        with patch.dict(os.environ, {"FLEET_WORKER_TOKENS_JSON":
                                     json.dumps({"fleet_01": "a" * 48})}):
            with TestClient(app) as client:
                headers = {"Authorization": "Bearer " + "a" * 48,
                           "X-Fleet-Worker-Id": "fleet_01"}
                payload = {"status": "ok", "capacity": 2,
                           "cpu_percent": 12.5, "android_online_devices": 1,
                           "android_bound_devices": 1}
                r = client.post("/api/fleet/worker/heartbeat", headers=headers,
                                json=payload)
                self.assertEqual(r.status_code, 200, r.text)
                self.assertEqual(self.store.workers()[0]["android_online_devices"], 1)
                r = client.post("/api/fleet/worker/heartbeat", headers=headers,
                                json=dict(payload, cpu_percent=101))
                self.assertEqual(r.status_code, 422)
                r = client.post("/api/fleet/worker/heartbeat", headers=headers,
                                json=dict(payload, device_serial="private"))
                self.assertEqual(r.status_code, 422)
                self.assertNotIn("private", str(self.store.workers()))

    def test_worker_transmits_sample_in_authenticated_heartbeat(self):
        events = []
        worker = FleetDiagnosticWorker(
            lambda route, body: events.append((route, body)) or {"items": []},
            Path(self.temp.name) / "work", capacity=1,
            resource_sampler=SimpleNamespace(sample=lambda: {
                "cpu_percent": 15.0, "android_bound_devices": None}))
        self.assertEqual(worker.step()["received"], 0)
        self.assertEqual(events[0][1]["cpu_percent"], 15)
        self.assertIsNone(events[0][1]["android_bound_devices"])
        self.assertEqual(events[0][0], "/api/fleet/worker/heartbeat")


if __name__ == "__main__":
    unittest.main()
