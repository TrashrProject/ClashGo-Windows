"""V16 inherited diagnostic preferences and per-profile read-only diagnostics.

Technical metadata only: an assigned worker becomes the target for diagnostic
jobs created through this console, not a game session or remote control.
"""
from __future__ import annotations

import secrets
import time

from reqngo.fleet import _KEY, _WORKER
from reqngo.fleet_console import FleetConsole


class FleetPolicyConsole(FleetConsole):
    DEFAULT_INTERVAL = 600

    def __init__(self, fleet):
        super().__init__(fleet)
        with self._connection(write=True) as db:
            db.execute("""
                CREATE TABLE IF NOT EXISTS fleet_console_group_policy (
                    group_key TEXT PRIMARY KEY,
                    preferred_worker TEXT,
                    probe_interval_seconds INTEGER NOT NULL DEFAULT 600,
                    updated_at DOUBLE PRECISION NOT NULL
                )
            """)
            db.execute("""
                CREATE TABLE IF NOT EXISTS fleet_console_profile_override (
                    profile_key TEXT PRIMARY KEY,
                    preferred_worker TEXT,
                    probe_interval_seconds INTEGER,
                    updated_at DOUBLE PRECISION NOT NULL
                )
            """)

    def _worker_exists(self, db, worker):
        if worker is None:
            return
        if not isinstance(worker, str) or not _WORKER.fullmatch(worker):
            raise ValueError("Worker de diagnostic invalide.")
        if db.execute("SELECT 1 FROM fleet_workers WHERE worker_id=" + self.p,
                      (worker,)).fetchone() is None:
            raise ValueError("Worker inconnu : enregistrer un heartbeat avant l'affectation.")

    @staticmethod
    def _interval(seconds, *, inherit=False):
        if seconds is None and inherit:
            return
        if type(seconds) is not int or not 30 <= seconds <= 3600:
            raise ValueError("Intervalle de diagnostic entre 30 et 3600 secondes.")

    def group_policy(self, group_key):
        if not self._valid_key(group_key):
            raise ValueError("Groupe invalide.")
        with self._connection() as db:
            if not self._exists(db, group_key):
                raise ValueError("Groupe inconnu.")
            row = db.execute(
                "SELECT preferred_worker,probe_interval_seconds,updated_at "
                "FROM fleet_console_group_policy WHERE group_key=" + self.p,
                (group_key,)).fetchone()
        if row is None:
            return {"group_key": group_key, "preferred_worker": None,
                    "probe_interval_seconds": self.DEFAULT_INTERVAL,
                    "source": "defaults"}
        return {"group_key": group_key, "preferred_worker": row["preferred_worker"],
                "probe_interval_seconds": row["probe_interval_seconds"],
                "source": "group"}

    def set_group_policy(self, group_key, preferred_worker, seconds):
        if not self._valid_key(group_key):
            raise ValueError("Groupe invalide.")
        self._interval(seconds)
        with self._connection(write=True) as db:
            if not self._exists(db, group_key):
                raise ValueError("Groupe inconnu.")
            self._worker_exists(db, preferred_worker)
            db.execute(
                "INSERT INTO fleet_console_group_policy "
                "(group_key,preferred_worker,probe_interval_seconds,updated_at) "
                f"VALUES ({self.p},{self.p},{self.p},{self.p}) "
                "ON CONFLICT(group_key) DO UPDATE SET "
                "preferred_worker=excluded.preferred_worker,"
                "probe_interval_seconds=excluded.probe_interval_seconds,"
                "updated_at=excluded.updated_at",
                (group_key, preferred_worker, seconds, time.time()))
            self._audit(db, group_key, "group_policy", 1)
        return self.group_policy(group_key)

    def _detail(self, db, profile_key):
        ts = ("EXTRACT(EPOCH FROM p.created_at)"
              if self.pg else "p.created_at")
        sql = (f"SELECT p.profile_key,p.enabled,p.group_key,{ts} AS created_at,"
               "o.preferred_worker AS worker_override,"
               "o.probe_interval_seconds AS interval_override,"
               "g.preferred_worker AS group_worker,"
               "g.probe_interval_seconds AS group_interval "
               "FROM fleet_profiles p "
               "LEFT JOIN fleet_console_profile_override o "
               "ON o.profile_key=p.profile_key "
               "LEFT JOIN fleet_console_group_policy g ON g.group_key=p.group_key "
               "WHERE p.profile_key=" + self.p)
        row = db.execute(sql, (profile_key,)).fetchone()
        if row is None:
            raise ValueError("Profil introuvable.")
        values = dict(row)
        override_worker = values["worker_override"]
        group_worker = values["group_worker"]
        values["effective_worker"] = override_worker if override_worker is not None else group_worker
        override_interval = values["interval_override"]
        group_interval = values["group_interval"]
        values["effective_interval_seconds"] = (
            override_interval if override_interval is not None
            else group_interval if group_interval is not None
            else self.DEFAULT_INTERVAL)
        values["worker_source"] = (
            "profile" if override_worker is not None else
            "group" if group_worker is not None else "unassigned")
        values["interval_source"] = (
            "profile" if override_interval is not None else
            "group" if group_interval is not None else "defaults")
        return values

    def profile_detail(self, profile_key):
        if not isinstance(profile_key, str) or not _KEY.fullmatch(profile_key):
            raise ValueError("Profil invalide.")
        with self._connection() as db:
            detail = self._detail(db, profile_key)
            ts = ("EXTRACT(EPOCH FROM updated_at)" if self.pg else "updated_at")
            rows = db.execute(
                "SELECT id AS job_id, state, kind, probe_mode, target_worker, attempts,"
                + ts + " AS updated_at FROM fleet_jobs WHERE profile_key=" + self.p
                + " ORDER BY id DESC LIMIT 10",
                (profile_key,)).fetchall()
            worker = detail["effective_worker"]
            if worker is not None:
                ts_worker = ("EXTRACT(EPOCH FROM last_seen)" if self.pg else "last_seen")
                status = db.execute(
                    "SELECT status,capacity," + ts_worker + " AS last_seen "
                    "FROM fleet_workers WHERE worker_id=" + self.p, (worker,)).fetchone()
            else:
                status = None
        detail["recent_jobs"] = [dict(r) for r in rows]
        detail["worker_health"] = dict(status) if status is not None else None
        return detail

    def set_profile_override(self, profile_key, preferred_worker, seconds):
        if not isinstance(profile_key, str) or not _KEY.fullmatch(profile_key):
            raise ValueError("Profil invalide.")
        self._interval(seconds, inherit=True)
        with self._connection(write=True) as db:
            self._detail(db, profile_key)
            self._worker_exists(db, preferred_worker)
            db.execute(
                "INSERT INTO fleet_console_profile_override "
                "(profile_key,preferred_worker,probe_interval_seconds,updated_at) "
                f"VALUES ({self.p},{self.p},{self.p},{self.p}) "
                "ON CONFLICT(profile_key) DO UPDATE SET "
                "preferred_worker=excluded.preferred_worker,"
                "probe_interval_seconds=excluded.probe_interval_seconds,"
                "updated_at=excluded.updated_at",
                (profile_key, preferred_worker, seconds, time.time()))
            self._audit(db, profile_key, "profile_override", 1)
        return self.profile_detail(profile_key)

    def enqueue_profile_diagnostic(self, profile_key, probe_mode):
        if probe_mode not in ("storage", "android"):
            raise ValueError("Diagnostic invalide.")
        detail = self.profile_detail(profile_key)
        if not detail["enabled"]:
            raise ValueError("Le profil est suspendu.")
        worker = detail["effective_worker"]
        if probe_mode == "android" and worker is None:
            raise ValueError("Une sonde Android exige un worker explicitement affecte.")
        if worker is not None:
            # An offline worker can be retained in configuration, but do not
            # silently create a job that will wait indefinitely for it.
            status = detail["worker_health"]
            if status is None or time.time() - float(status["last_seen"]) > 180:
                raise ValueError("Worker affecte sans heartbeat recent.")
        # No gameplay and no screenshots are returned to the console.
        result = self.fleet.enqueue_probe(
            profile_key, "console:probe:" + secrets.token_hex(15),
            target_worker=worker, probe_mode=probe_mode)
        return {"job_id": result["job_id"], "probe_mode": probe_mode,
                "target_worker": worker, "queued": not result["duplicate"]}
