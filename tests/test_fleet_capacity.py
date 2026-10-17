"""Capacity advice and CI benchmark safeguards; no game/device execution."""
from __future__ import annotations

import os
import tempfile
import unittest
from pathlib import Path
from unittest.mock import patch

from fastapi import FastAPI
from fastapi.testclient import TestClient

from reqngo.fleet import FleetStore
from reqngo.fleet_api import router_for
from reqngo.fleet_capacity import assess_fleet_capacity
from tools.fleet_load_benchmark import benchmark, percentile


def make_summary(ready=0, workers=1, slots=2, age=0, incidents=0):
    return {
        "ready_jobs": ready,
        "online_workers": workers,
        "online_slots": slots,
        "oldest_ready_wait_seconds": age,
        "open_worker_incidents": incidents,
        "jobs": {"leased": 0, "needs_review": 0},
    }


class CapacityAdvisorTests(unittest.TestCase):
    def test_no_workers_with_backlog(self):
        result = assess_fleet_capacity(make_summary(ready=10, workers=0, slots=0))
        self.assertEqual(result["state"], "no_capacity")
        self.assertIsNone(result["backlog_per_declared_slot"])

    def test_backpressure_by_age_and_ratio(self):
        self.assertEqual(assess_fleet_capacity(
            make_summary(ready=50, workers=1, slots=2))["state"], "backlog_pressure")
        self.assertEqual(assess_fleet_capacity(
            make_summary(ready=1, workers=1, slots=2, age=120))["state"], "backlog_pressure")
        self.assertEqual(assess_fleet_capacity(
            make_summary(ready=20, workers=1, slots=2))["state"], "nominal")

    def test_worker_alerts_and_idle(self):
        self.assertEqual(assess_fleet_capacity(
            make_summary(ready=0, workers=0, slots=0, incidents=2))["state"], "worker_incidents")
        self.assertEqual(assess_fleet_capacity(
            make_summary(ready=0, workers=0, slots=0))["state"], "idle_without_workers")
        self.assertEqual(assess_fleet_capacity(
            make_summary())["state"], "nominal")

    def test_invalid_counts_fail_closed(self):
        with self.assertRaises(ValueError):
            assess_fleet_capacity(make_summary(ready=-1))
        with self.assertRaises(ValueError):
            assess_fleet_capacity(make_summary(slots=True))
        with self.assertRaises(ValueError):
            assess_fleet_capacity({"jobs": {}})

    def test_http_is_authenticated_and_returns_no_ids(self):
        with tempfile.TemporaryDirectory() as directory:
            store = FleetStore(Path(directory) / "fleet.sqlite3")
            try:
                app = FastAPI()

                def read_guard():
                    return True

                app.include_router(router_for(store, read_guard, read_guard))
                with TestClient(app) as client:
                    r = client.get("/api/fleet/capacity")
                    self.assertEqual(r.status_code, 200)
                    self.assertEqual(r.json()["state"], "idle_without_workers")
                    self.assertNotIn("lease_token", r.text)
            finally:
                store.close()


class BenchmarkSafetyTests(unittest.TestCase):
    def test_percentiles_are_deterministic(self):
        self.assertEqual(percentile([1, 2, 3, 4, 5], .5), 3)
        self.assertEqual(percentile([1, 2, 3, 4, 5], .95), 4.8)
        self.assertEqual(percentile([], .99), 0)

    def test_benchmark_disabled_without_explicit_disposable_opt_in(self):
        with patch.dict(os.environ, {"FLEET_BENCH_DISPOSABLE": "0"}):
            with self.assertRaisesRegex(ValueError, "jetable"):
                benchmark(dsn="postgresql://unused/unused", profiles=2,
                          jobs=1, workers=1)

    def test_benchmark_parameter_bounds_without_database(self):
        with self.assertRaises(ValueError):
            benchmark(dsn="", profiles=2, jobs=3)
        with self.assertRaises(ValueError):
            benchmark(dsn="", profiles=2, jobs=1, workers=17)


if __name__ == "__main__":
    unittest.main()
