"""Fleet V9: isolation, fencing, crash recovery and volume tests (no game)."""
import tempfile
import threading
import unittest
from pathlib import Path

from reqngo.fleet import FleetStore


class FleetTests(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.path = Path(self.tmp.name) / "fleet.sqlite3"
        self.now = [1000.0]
        self.store = FleetStore(self.path, clock=lambda: self.now[0])

    def tearDown(self):
        self.store.close()
        self.tmp.cleanup()

    def test_five_thousand_profiles_and_bounded_pagination(self):
        self.assertEqual(self.store.register_batch("fleet", 1, 5000)["created"], 5000)
        self.assertEqual(self.store.register_batch("fleet", 1, 5000)["existing"], 5000)
        self.assertEqual(self.store.summary()["profiles"], 5000)
        self.assertEqual(len(self.store.profiles(4900, 200)), 100)
        with self.assertRaises(ValueError):
            self.store.profiles(0, 10000)
        with self.assertRaises(ValueError):
            self.store.register_batch("bad prefix", 1, 5)

    def test_queue_idempotency_and_fencing(self):
        self.store.register_batch("acct", 1, 2)
        job = self.store.enqueue_probe("acct-000001", "probe:first:2026")
        self.assertFalse(job["duplicate"])
        self.assertTrue(self.store.enqueue_probe("acct-000001", "probe:first:2026")["duplicate"])
        with self.assertRaises(ValueError):
            self.store.enqueue_probe("acct-000002", "probe:first:2026")
        with self.assertRaises(ValueError):
            self.store.enqueue_probe("acct-000001", "probe:second:2026")
        claimed = self.store.claim("worker_01")
        self.assertEqual(claimed[0]["job_id"], job["job_id"])
        self.assertEqual(self.store.claim("worker_02"), [])
        with self.assertRaises(ValueError):
            self.store.complete(job["job_id"], "worker_02",
                                claimed[0]["lease_token"], True)
        self.store.renew(job["job_id"], "worker_01", claimed[0]["lease_token"])
        self.assertEqual(self.store.complete(job["job_id"], "worker_01",
                         claimed[0]["lease_token"], True)["state"], "done")
        with self.assertRaises(ValueError):
            self.store.complete(job["job_id"], "worker_01",
                                claimed[0]["lease_token"], True)
        self.assertEqual(self.store.summary()["jobs"]["done"], 1)

    def test_stale_lease_retries_only_safe_probe_with_fence(self):
        self.store.register_batch("acct", 1, 1)
        job = self.store.enqueue_probe("acct-000001", "probe:retry:2026")
        first = self.store.claim("worker_01", lease_seconds=15)[0]
        self.now[0] += 16
        self.assertEqual(self.store.claim("worker_02"), [])  # cooldown
        with self.assertRaises(ValueError):
            self.store.complete(job["job_id"], "worker_01", first["lease_token"], True)
        self.now[0] += 5
        second = self.store.claim("worker_02", lease_seconds=15)[0]
        self.assertNotEqual(first["lease_token"], second["lease_token"])
        self.assertEqual(second["attempt"], 2)
        self.now[0] += 16
        self.assertEqual(self.store.claim("worker_03"), [])
        self.now[0] += 10
        third = self.store.claim("worker_03", lease_seconds=15)[0]
        self.assertEqual(third["attempt"], 3)
        self.now[0] += 16
        self.assertEqual(self.store.claim("worker_01"), [])
        self.assertEqual(self.store.summary()["jobs"]["needs_review"], 1)

    def test_failure_does_not_auto_retry(self):
        self.store.register_batch("acct", 1, 1)
        self.store.enqueue_probe("acct-000001", "probe:failed:2026")
        leased = self.store.claim("worker_01")[0]
        self.store.complete(leased["job_id"], "worker_01",
                            leased["lease_token"], False)
        self.assertEqual(self.store.claim("worker_02"), [])
        self.assertEqual(self.store.summary()["jobs"]["needs_review"], 1)

    def test_disabled_profiles_are_not_claimed(self):
        self.store.register_batch("acct", 1, 1)
        self.store.enqueue_probe("acct-000001", "probe:disabled:2026")
        self.store.set_enabled("acct-000001", False)
        self.assertEqual(self.store.claim("worker_01"), [])
        self.store.set_enabled("acct-000001", True)
        self.assertEqual(len(self.store.claim("worker_01")), 1)

    def test_two_independent_connections_never_double_lease(self):
        self.store.register_batch("acct", 1, 100)
        for number in range(1, 101):
            key = f"acct-{number:06d}"
            self.store.enqueue_probe(key, f"probe:concurrent:{number:06d}")
        second = FleetStore(self.path, clock=lambda: self.now[0])
        barrier = threading.Barrier(2)
        results = []
        failures = []

        def run(store, name):
            try:
                barrier.wait(timeout=5)
                results.append(store.claim(name, limit=100))
            except Exception as error:
                failures.append(error)

        try:
            a = threading.Thread(target=run, args=(self.store, "worker_01"))
            b = threading.Thread(target=run, args=(second, "worker_02"))
            a.start()
            b.start()
            a.join(timeout=10)
            b.join(timeout=10)
            self.assertFalse(a.is_alive() or b.is_alive())
            self.assertEqual(failures, [])
            all_jobs = [job["job_id"] for batch in results for job in batch]
            self.assertEqual(len(all_jobs), 100)
            self.assertEqual(len(set(all_jobs)), 100)
        finally:
            second.close()


if __name__ == "__main__":
    unittest.main()
