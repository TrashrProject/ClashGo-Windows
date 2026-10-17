"""Real PostgreSQL concurrency and recovery tests for V10.

Run with FLEET_TEST_PG_DSN pointed at a DISPOSABLE test database only.
WARNING: this test truncates fleet tables in that database.
"""
import os
import threading
import unittest

from reqngo.fleet_postgres import PostgresFleetStore

DSN = os.environ.get("FLEET_TEST_PG_DSN")


@unittest.skipUnless(DSN, "PostgreSQL test database not configured")
class PostgreSQLFleetTests(unittest.TestCase):
    def setUp(self):
        self.store = PostgresFleetStore(DSN)
        self.second = PostgresFleetStore(DSN)
        with self.store.pool.connection() as conn:
            # CI starts a dedicated fresh database; never run against production.
            conn.execute("TRUNCATE fleet_worker_incidents, fleet_jobs, "
                         "fleet_profiles, fleet_workers RESTART IDENTITY")

    def tearDown(self):
        self.second.close()
        self.store.close()

    def test_five_thousand_profiles_and_idempotency(self):
        self.assertTrue(self.store.ping())
        self.assertEqual(self.store.register_batch("test", 1, 5000)["created"], 5000)
        self.assertEqual(self.second.register_batch("test", 1, 5000)["existing"], 5000)
        self.assertEqual(self.store.summary()["profiles"], 5000)
        self.assertEqual(len(self.second.profiles(4900, 200)), 100)
        original = self.store.enqueue_probe("test-000001", "pg:dedupe:000001")
        repeat = self.second.enqueue_probe("test-000001", "pg:dedupe:000001")
        self.assertEqual(original["job_id"], repeat["job_id"])
        self.assertTrue(repeat["duplicate"])
        with self.assertRaises(ValueError):
            self.second.enqueue_probe("test-000002", "pg:dedupe:000001")

    def test_atomic_claims_across_distinct_stores(self):
        self.store.register_batch("test", 1, 100)
        for number in range(1, 101):
            self.store.enqueue_probe(f"test-{number:06d}", f"pg:atomic:{number:06d}")
        barrier = threading.Barrier(2)
        results = []
        errors = []

        def work(store, worker_id):
            try:
                barrier.wait(timeout=5)
                results.extend(store.claim(worker_id, limit=100, capacity=100))
            except Exception as exc:
                errors.append(exc)

        a = threading.Thread(target=work, args=(self.store, "worker_01"))
        b = threading.Thread(target=work, args=(self.second, "worker_02"))
        a.start(); b.start()
        a.join(timeout=15); b.join(timeout=15)
        self.assertFalse(a.is_alive() or b.is_alive())
        self.assertEqual(errors, [])
        self.assertEqual(len(results), 100)
        self.assertEqual(len({r["job_id"] for r in results}), 100)

    def test_cross_worker_fencing_and_expiration(self):
        self.store.register_batch("test", 1, 1)
        job = self.store.enqueue_probe("test-000001", "pg:fencing:000001")
        first = self.store.claim("worker_01", lease_seconds=15)[0]
        with self.assertRaises(ValueError):
            self.second.complete(job["job_id"], "worker_02",
                                 first["lease_token"], True)
        with self.store.pool.connection() as conn:
            conn.execute("UPDATE fleet_jobs SET lease_until=clock_timestamp() "
                         "- INTERVAL '1 second' WHERE id=%s", (job["job_id"],))
        self.assertEqual(self.second.claim("worker_02"), [])
        with self.store.pool.connection() as conn:
            conn.execute("UPDATE fleet_jobs SET available_at=clock_timestamp() "
                         "- INTERVAL '1 second' WHERE id=%s", (job["job_id"],))
        second = self.second.claim("worker_02")[0]
        self.assertNotEqual(first["lease_token"], second["lease_token"])
        with self.assertRaises(ValueError):
            self.store.complete(job["job_id"], "worker_01",
                                first["lease_token"], True)
        self.assertEqual(self.second.complete(job["job_id"], "worker_02",
                         second["lease_token"], True)["state"], "done")
        self.assertEqual(self.store.summary()["jobs"]["done"], 1)

    def test_fifty_thousand_logical_profiles_paginate_without_full_fetch(self):
        for start in range(1, 50001, 10000):
            batch = self.store.register_batch("scale", start, 10000)
            self.assertEqual(batch["created"], 10000)
        self.assertEqual(self.store.summary()["profiles"], 50000)
        sample = self.second.profiles(49000, 200)
        self.assertEqual(len(sample), 200)
        self.assertEqual(sample[0]["profile_key"], "scale-049001")
        self.assertEqual(sample[-1]["profile_key"], "scale-049200")
        self.assertEqual(self.second.register_batch("scale", 49001, 1000)["existing"], 1000)

    def test_targeted_android_probe_skips_other_workers(self):
        self.store.register_batch("test", 1, 3)
        device_job = self.store.enqueue_probe(
            "test-000001", "pg:android:000001",
            target_worker="worker_adb", probe_mode="android")
        storage_job = self.second.enqueue_probe(
            "test-000002", "pg:storage:000002")
        self.assertEqual(self.store.claim("worker_plain", limit=5)[0]["job_id"],
                         storage_job["job_id"])
        claimed = self.second.claim("worker_adb", limit=5)
        self.assertEqual(len(claimed), 1)
        self.assertEqual(claimed[0]["job_id"], device_job["job_id"])
        self.assertEqual(claimed[0]["probe_mode"], "android")
        self.assertEqual(claimed[0]["target_worker"], "worker_adb")
        with self.assertRaises(ValueError):
            self.second.enqueue_probe("test-000001", "pg:android:000001",
                                      target_worker="worker_other", probe_mode="android")
        self.assertEqual(self.second.complete(
            claimed[0]["job_id"], "worker_adb",
            claimed[0]["lease_token"], True)["state"], "done")

    def test_stale_worker_incidents_across_multiple_postgres_replicas(self):
        self.store.heartbeat("worker_01", "ok", 2)
        with self.store.pool.connection() as conn:
            conn.execute("""
                UPDATE fleet_workers SET last_seen=clock_timestamp() -
                    INTERVAL '181 seconds' WHERE worker_id='worker_01'
            """)
        first = self.store.scan_worker_health()
        self.assertEqual(first, {"new_incidents": 1, "open_incidents": 1})
        second = self.second.scan_worker_health()
        self.assertEqual(second, {"new_incidents": 0, "open_incidents": 1})
        self.assertEqual(self.second.summary()["open_worker_incidents"], 1)
        self.assertEqual(self.second.summary()["online_slots"], 0)
        self.second.heartbeat("worker_01", "degraded", 2)
        self.assertEqual(self.store.scan_worker_health()["open_incidents"], 0)
        self.assertEqual(self.store.worker_incidents()[0]["status"], "resolved")
        with self.second.pool.connection() as conn:
            conn.execute("""
                UPDATE fleet_workers SET last_seen=clock_timestamp() -
                    INTERVAL '181 seconds' WHERE worker_id='worker_01'
            """)
        reopened = self.store.scan_worker_health()
        self.assertEqual(reopened, {"new_incidents": 1, "open_incidents": 1})
        history = self.second.worker_incidents()
        self.assertEqual(history[0]["occurrences"], 2)
        self.assertIsNone(history[0]["resolved_at"])

    def test_capacity_and_monitoring(self):
        self.store.register_batch("test", 1, 3)
        for num in range(1, 4):
            self.store.enqueue_probe(f"test-{num:06d}", f"pg:capacity:{num:06d}")
        self.assertEqual(len(self.store.claim("worker_01", limit=3, capacity=1)), 1)
        self.assertEqual(self.second.claim("worker_01", limit=3, capacity=1), [])
        self.assertEqual(len(self.second.claim("worker_02", limit=3, capacity=2)), 2)
        self.store.heartbeat("worker_01", "busy", 1)
        self.second.heartbeat("worker_02", "ok", 2)
        self.assertEqual(self.store.summary()["online_workers"], 2)
        self.assertEqual(len(self.second.workers()), 2)


if __name__ == "__main__":
    unittest.main()
