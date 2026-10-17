"""V11 Android affinity: routing, migration and fail-closed read-only ADB probes."""
import sqlite3
import tempfile
import unittest
from pathlib import Path
from unittest.mock import Mock

from reqngo.android.preflight import AndroidReadOnlyPreflight
from reqngo.fleet import FleetStore
from reqngo.fleet_worker import FleetDiagnosticWorker


class AffinitySQLiteTests(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.path = Path(self.tmp.name) / "fleet.sqlite3"
        self.store = FleetStore(self.path)
        self.store.register_batch("demo", 1, 3)

    def tearDown(self):
        self.store.close()
        self.tmp.cleanup()

    def test_android_probe_only_claimable_by_target_and_independent_storage(self):
        job = self.store.enqueue_probe("demo-000001", "probe:android:0001",
                                       target_worker="worker_adb", probe_mode="android")
        self.store.enqueue_probe("demo-000002", "probe:storage:0002")
        generic = self.store.claim("worker_plain", limit=8)
        self.assertEqual(len(generic), 1)
        self.assertEqual(generic[0]["profile_key"], "demo-000002")
        self.assertEqual(generic[0]["probe_mode"], "storage")
        android = self.store.claim("worker_adb", limit=8)
        self.assertEqual(len(android), 1)
        self.assertEqual(android[0]["job_id"], job["job_id"])
        self.assertEqual(android[0]["probe_mode"], "android")
        self.assertEqual(android[0]["target_worker"], "worker_adb")
        with self.assertRaises(ValueError):
            self.store.complete(android[0]["job_id"], "worker_plain",
                                android[0]["lease_token"], True)
        self.store.complete(android[0]["job_id"], "worker_adb",
                            android[0]["lease_token"], True)

    def test_mocked_adb_preflight_completes_targeted_fleet_job(self):
        class FakeADB:
            @classmethod
            def devices(cls, *, adb_binary="adb"):
                return ["emulator-5554"]

        self.store.enqueue_probe("demo-000001", "probe:end-to-end:0001",
                                 target_worker="worker_adb", probe_mode="android")
        lease = self.store.claim("worker_adb")[0]

        def send(route, payload):
            self.assertEqual(route, "/api/fleet/worker/complete")
            return self.store.complete(payload["job_id"], "worker_adb",
                                       payload["lease_token"], payload["success"])

        preflight = AndroidReadOnlyPreflight({"demo-000001": "emulator-5554"},
                                             client_class=FakeADB)
        worker = FleetDiagnosticWorker(send, Path(self.tmp.name) / "work",
                                       worker_id="worker_adb", android_preflight=preflight)
        self.assertTrue(worker._process(lease))
        self.assertEqual(self.store.summary()["jobs"]["done"], 1)
        self.assertFalse((Path(self.tmp.name) / "work").exists())

    def test_android_requires_target_and_rejects_dedupe_mismatch(self):
        with self.assertRaises(ValueError):
            self.store.enqueue_probe("demo-000001", "probe:android:0001",
                                     probe_mode="android")
        self.store.enqueue_probe("demo-000001", "probe:android:0001",
                                 probe_mode="android", target_worker="worker_adb")
        with self.assertRaises(ValueError):
            self.store.enqueue_probe("demo-000001", "probe:android:0001",
                                     probe_mode="storage")
        with self.assertRaises(ValueError):
            self.store.enqueue_probe("demo-000001", "probe:android:0001",
                                     probe_mode="android", target_worker="worker_other")
        with self.assertRaises(ValueError):
            self.store.enqueue_probe("demo-000002", "probe:android:0002",
                                     probe_mode="android", target_worker="not a worker")

    def test_additive_schema_migration_preserves_jobs(self):
        self.store.close()
        self.store = None
        with sqlite3.connect(self.path) as db:
            db.execute("DROP TABLE fleet_jobs")
            db.execute("""CREATE TABLE fleet_jobs (
                id INTEGER PRIMARY KEY AUTOINCREMENT,
                profile_key TEXT NOT NULL,
                kind TEXT NOT NULL CHECK(kind='health_probe'),
                dedupe_key TEXT NOT NULL UNIQUE,
                state TEXT NOT NULL DEFAULT 'queued',
                attempts INTEGER NOT NULL DEFAULT 0,
                available_at REAL NOT NULL,
                lease_until REAL,
                lease_owner TEXT,
                lease_token TEXT,
                updated_at REAL NOT NULL)""")
            db.execute("INSERT INTO fleet_jobs(profile_key,kind,dedupe_key,"
                       "state,available_at,updated_at) VALUES "
                       "('demo-000001','health_probe','probe:old:000001','queued',1,1)")
        self.store = FleetStore(self.path)
        legacy = self.store.claim("worker_plain")
        self.assertEqual(len(legacy), 1)
        self.assertEqual(legacy[0]["probe_mode"], "storage")
        self.assertIsNone(legacy[0]["target_worker"])


class AndroidLocalTests(unittest.TestCase):
    def test_serial_must_be_explicit_and_safe(self):
        with self.assertRaises(ValueError):
            AndroidReadOnlyPreflight.from_json('{"demo-000001":"../../bad"}')
        with self.assertRaises(ValueError):
            AndroidReadOnlyPreflight.from_json('{"demo-000001":42}')
        with self.assertRaises(ValueError):
            AndroidReadOnlyPreflight.from_json('not json')
        self.assertFalse(AndroidReadOnlyPreflight({}, client_class=Mock()).check("demo-000001"))

    def test_no_game_commands_when_only_adb_presence_requested(self):
        client = Mock()
        client.devices.return_value = ["emulator-5554"]
        preflight = AndroidReadOnlyPreflight({"demo-000001": "emulator-5554"},
                                             client_class=client)
        self.assertTrue(preflight.check("demo-000001"))
        client.assert_not_called()  # no game process, capture or touch
        client.devices.assert_called_once_with(adb_binary="adb")
        self.assertFalse(preflight.check("demo-000002"))

    def test_game_process_probe_is_read_only_opt_in(self):
        client = Mock()
        client.devices.return_value = ["emulator-5554"]
        client.return_value.game_running.return_value = False
        preflight = AndroidReadOnlyPreflight({"demo-000001": "emulator-5554"},
                                             require_game=True, client_class=client)
        self.assertFalse(preflight.check("demo-000001"))
        client.return_value.game_running.assert_called_once()
        client.return_value.screenshot.assert_not_called()
        client.return_value.tap.assert_not_called()
        client.return_value.swipe.assert_not_called()

    def test_worker_never_claims_android_success_without_local_binding(self):
        sent = []
        worker = FleetDiagnosticWorker(
            lambda endpoint, data: sent.append((endpoint, data)) or {},
            tempfile.gettempdir(), worker_id="worker_adb")
        result = worker._process({"kind": "health_probe", "probe_mode": "android",
                                  "target_worker": "worker_adb",
                                  "profile_key": "demo-000001",
                                  "job_id": 1, "lease_token": "x" * 48})
        self.assertFalse(result)
        self.assertFalse(sent[-1][1]["success"])


if __name__ == "__main__":
    unittest.main()
