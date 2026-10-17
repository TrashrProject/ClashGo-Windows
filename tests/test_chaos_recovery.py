"""Deterministic chaos drills: process restart, partial network outage, 50 profiles.

These tests run entirely offline, without Supercell accounts or real emulators.
"""
import sqlite3
import tempfile
import unittest
from concurrent.futures import ThreadPoolExecutor
from pathlib import Path
from unittest.mock import patch

from reqngo.pilot_worker import Outbox, Pilot
from reqngo.telemetry import TelemetryStore


class ChaosRecoveryTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.base = Path(self.temp.name)
        self.outbox = Outbox(self.base / "p1" / "outbox.sqlite3")
        self.events = []
        self.beats = []
        self.work = self.base / "p1" / "work"

    def tearDown(self):
        self.outbox.close()
        self.temp.cleanup()

    def pilot(self):
        return Pilot("pilot_01", 1, self.work, self.outbox,
                     self.beats.append, self.events.append)

    def test_failure_is_verified_after_worker_process_restart(self):
        self.work.mkdir()
        first = self.pilot()
        with patch.object(first, "_probe", side_effect=OSError("storage offline")):
            self.assertEqual(first.step()["status"], "degraded")
        self.assertEqual(self.outbox.get_state("last_issue"), "pilot_probe_failed")
        self.outbox.close()
        self.outbox = Outbox(self.base / "p1" / "outbox.sqlite3")
        restarted = self.pilot()
        self.assertEqual(restarted.last_issue, "pilot_probe_failed")
        self.assertEqual(restarted.step()["status"], "ok")
        self.assertEqual(self.events[-1]["code"], "probe_success")
        self.assertEqual(self.events[-1]["metadata"]["error_type"], "pilot_probe_failed")
        self.assertIsNone(self.outbox.get_state("last_issue"))

    def test_persistent_failure_does_not_repeat_error_at_every_tick(self):
        self.work.mkdir()
        first = self.pilot()
        with patch.object(first, "_probe", side_effect=OSError("disk offline")):
            for _ in range(15):
                self.assertEqual(first.step()["status"], "degraded")
        self.assertEqual(len([e for e in self.events if e["code"] == "pilot_probe_failed"]), 1)

    def test_non_directory_file_is_never_destroyed_and_retry_is_bounded(self):
        self.work.parent.mkdir(parents=True, exist_ok=True)
        self.work.write_text("DO_NOT_DELETE", encoding="utf-8")
        first = self.pilot()
        for _ in range(3):
            self.assertEqual(first.step()["status"], "degraded")
        self.assertEqual(self.work.read_text(encoding="utf-8"), "DO_NOT_DELETE")
        self.assertEqual([e["code"] for e in self.events], ["invalid_config"])
        self.work.unlink()  # Human intentionally removes the blocking file.
        self.assertEqual(first.step()["status"], "ok")
        self.assertEqual([e["code"] for e in self.events],
                         ["invalid_config", "probe_success"])

    def test_lost_ack_does_not_duplicate_a_report_or_lie_about_repair(self):
        central = TelemetryStore(self.base / "acked.sqlite3")
        try:
            lose_ack_once = [True]
            def report_and_drop_first_ack(message):
                central.report(**message)
                if lose_ack_once[0]:
                    lose_ack_once[0] = False
                    raise TimeoutError("ack lost after server committed")
            pilot = Pilot("pilot_01", 1, self.work, self.outbox,
                          lambda body: central.heartbeat(**body), report_and_drop_first_ack)
            self.assertFalse(pilot.step()["connected"])
            self.assertEqual(self.outbox.count(), 2)
            self.assertEqual(central.snapshot()["summary"]["open_cases"], 1)
            # On replay, the same event_key is safely de-duplicated.
            pilot.report = lambda event: central.report(**event)
            self.assertTrue(pilot.step()["connected"])
            self.assertEqual(self.outbox.count(), 0)
            snapshot = central.snapshot()
            self.assertEqual(len(snapshot["events"]), 2)
            self.assertEqual(snapshot["summary"]["open_cases"], 0)
            self.assertEqual(snapshot["cases"][0]["status"], "verified")
        finally:
            central.close()

    def test_partial_ack_keeps_only_unconfirmed_event(self):
        box = self.outbox
        for n in range(3):
            box.append({"event_key": "evt_" + str(n)})
        delivered = []
        def unreliable(event):
            if event["event_key"] == "evt_1":
                raise ConnectionError("network drop")
            delivered.append(event["event_key"])
        with self.assertRaises(ConnectionError):
            box.flush(unreliable)
        self.assertEqual(delivered, ["evt_0"])
        self.assertEqual(box.count(), 2)
        after = []
        box.flush(lambda event: after.append(event["event_key"]))
        self.assertEqual(after, ["evt_1", "evt_2"])
        self.assertEqual(box.count(), 0)

    def test_parallel_pilots_are_isolated(self):
        failures = []
        for i in range(1, 4):
            root = self.base / ("isolate_" + str(i))
            box = Outbox(root / "outbox.sqlite3")
            try:
                work = root / "work"
                if i == 2:
                    work.write_text("PRESERVE", encoding="utf-8")
                p = Pilot("pilot_0" + str(i), i, work, box,
                          lambda event: None, lambda event: None)
                result = p.step()
                failures.append(result["status"])
                if i == 2:
                    self.assertEqual(work.read_text(encoding="utf-8"), "PRESERVE")
            finally:
                box.close()
        self.assertEqual(failures, ["ok", "degraded", "ok"])

    def test_same_second_reconnection_yields_distinct_outage(self):
        now = [1000.0]
        store = TelemetryStore(self.base / "central.sqlite3", clock=lambda: now[0])
        try:
            store.heartbeat("pilot_01")
            now[0] += 200
            self.assertEqual(store.check_stale()["alerts_created"], 1)
            # A host clock correction reuses the SAME last_seen timestamp,
            # but the new heartbeat sequence must still create a new alert.
            now[0] = 1000.0
            store.heartbeat("pilot_01")
            now[0] = 1200.0
            self.assertEqual(store.check_stale()["alerts_created"], 1)
            self.assertEqual(store.check_stale()["alerts_created"], 0)
            self.assertEqual(
                [e["code"] for e in store.snapshot()["events"]],
                ["worker_unreachable", "worker_unreachable"],
            )
            self.assertEqual(store.snapshot()["summary"]["open_cases"], 1)
        finally:
            store.close()

    def test_legacy_db_migrates_without_losing_incidents(self):
        path = self.base / "legacy.sqlite3"
        db = sqlite3.connect(path)
        db.execute("CREATE TABLE agents(agent_id TEXT PRIMARY KEY, "
                   "status TEXT NOT NULL, version TEXT NOT NULL, last_seen REAL NOT NULL)")
        db.execute("INSERT INTO agents VALUES('pilot_01','ok','legacy',1000)")
        db.commit()
        db.close()
        store = TelemetryStore(path, clock=lambda: 1200.0)
        try:
            self.assertEqual(store.check_stale()["alerts_created"], 1)
            self.assertEqual(store.snapshot()["summary"]["open_cases"], 1)
            store.heartbeat("pilot_01")
            self.assertEqual(store.snapshot()["summary"]["open_cases"], 0)
        finally:
            store.close()

    def test_fifty_simultaneous_profile_incidents_do_not_block_each_other(self):
        now = [1000.0]
        store = TelemetryStore(self.base / "burst.sqlite3", clock=lambda: now[0])
        try:
            store.heartbeat("pilot_01")
            def report(i):
                return store.report("pilot_01", "ev_" + str(i), "worker_timeout",
                                    level="warning", profile_id=i)
            with ThreadPoolExecutor(max_workers=10) as pool:
                results = list(pool.map(report, range(1, 51)))
            self.assertTrue(all(r["ok"] for r in results))
            self.assertEqual(store.snapshot()["summary"]["open_cases"], 50)
            store.report("pilot_01", "probe_17", "probe_success", level="info",
                         profile_id=17, metadata={"error_type": "worker_timeout"})
            snap = store.snapshot()
            self.assertEqual(snap["summary"]["open_cases"], 49)
            self.assertEqual(
                [c["status"] for c in snap["cases"] if c["profile_id"] == 17],
                ["verified"],
            )
        finally:
            store.close()


if __name__ == "__main__":
    unittest.main()
