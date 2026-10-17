"""Durable single-node fleet queue for logical test profiles.

This module never logs in to a game, controls Android, or stores credentials.
SQLite is intentional for the pilot: it serializes writers and must be replaced
by a shared transactional database before horizontal cloud deployment.
"""
from __future__ import annotations

import re
import secrets
import sqlite3
import threading
import time
from contextlib import contextmanager
from pathlib import Path

_PREFIX = re.compile(r"[A-Za-z][A-Za-z0-9_-]{0,23}\Z")
_KEY = re.compile(r"[A-Za-z][A-Za-z0-9_-]{0,23}-[0-9]{6,7}\Z")
_WORKER = re.compile(r"[A-Za-z][A-Za-z0-9_-]{1,63}\Z")
_DEDUPE = re.compile(r"[A-Za-z0-9][A-Za-z0-9_.:-]{7,79}\Z")


class FleetStore:
    """Atomic leases, fenced acknowledgements and bounded safe probe retries."""

    def __init__(self, path, clock=None):
        self.clock = clock or time.time
        self.lock = threading.RLock()
        path = Path(path)
        path.parent.mkdir(parents=True, exist_ok=True)
        self.db = sqlite3.connect(str(path), timeout=15, isolation_level=None,
                                  check_same_thread=False)
        self.db.row_factory = sqlite3.Row
        self.db.execute("PRAGMA busy_timeout=15000")
        self.db.execute("PRAGMA journal_mode=WAL")
        self.db.execute("PRAGMA foreign_keys=ON")
        with self.lock:
            self.db.executescript("""
                CREATE TABLE IF NOT EXISTS fleet_profiles (
                    profile_key TEXT PRIMARY KEY,
                    enabled INTEGER NOT NULL DEFAULT 1 CHECK(enabled IN (0,1)),
                    created_at REAL NOT NULL
                );
                CREATE TABLE IF NOT EXISTS fleet_jobs (
                    id INTEGER PRIMARY KEY AUTOINCREMENT,
                    profile_key TEXT NOT NULL REFERENCES fleet_profiles(profile_key),
                    kind TEXT NOT NULL CHECK(kind='health_probe'),
                    dedupe_key TEXT NOT NULL UNIQUE,
                    state TEXT NOT NULL DEFAULT 'queued'
                        CHECK(state IN ('queued','leased','done','needs_review')),
                    attempts INTEGER NOT NULL DEFAULT 0,
                    available_at REAL NOT NULL,
                    lease_until REAL,
                    lease_owner TEXT,
                    lease_token TEXT,
                    updated_at REAL NOT NULL
                );
                CREATE TABLE IF NOT EXISTS fleet_workers (
                    worker_id TEXT PRIMARY KEY,
                    status TEXT NOT NULL CHECK(status IN ('ok','busy','degraded')),
                    capacity INTEGER NOT NULL CHECK(capacity BETWEEN 1 AND 100),
                    last_seen REAL NOT NULL
                );
                CREATE UNIQUE INDEX IF NOT EXISTS fleet_one_open_job
                  ON fleet_jobs(profile_key) WHERE state IN ('queued','leased');
                CREATE INDEX IF NOT EXISTS fleet_ready_jobs
                  ON fleet_jobs(state,available_at,id);
                CREATE INDEX IF NOT EXISTS fleet_profile_jobs
                  ON fleet_jobs(profile_key,id);
            """)
            # Non-destructive schema evolution for installed V9/V10 databases.
            cols = {row["name"] for row in self.db.execute("PRAGMA table_info(fleet_jobs)")}
            if "target_worker" not in cols:
                self.db.execute("ALTER TABLE fleet_jobs ADD COLUMN target_worker TEXT")
            if "probe_mode" not in cols:
                self.db.execute(
                    "ALTER TABLE fleet_jobs ADD COLUMN probe_mode TEXT NOT NULL "
                    "DEFAULT 'storage' CHECK(probe_mode IN ('storage','android'))"
                )
            self.db.execute(
                "CREATE INDEX IF NOT EXISTS fleet_target_ready "
                "ON fleet_jobs(state,target_worker,available_at,id)"
            )

    @contextmanager
    def _transaction(self):
        with self.lock:
            self.db.execute("BEGIN IMMEDIATE")
            try:
                yield
                self.db.execute("COMMIT")
            except BaseException:
                self.db.execute("ROLLBACK")
                raise

    def register_batch(self, prefix: str, start: int, count: int):
        """Create up to 10k logical profiles in one transaction; idempotent."""
        if not isinstance(prefix, str) or not _PREFIX.fullmatch(prefix):
            raise ValueError("Prefixe de flotte invalide.")
        if type(start) is not int or type(count) is not int or not (
                1 <= start <= 9_999_999 and 1 <= count <= 10_000 and
                start + count - 1 <= 9_999_999):
            raise ValueError("Plage de profils invalide.")
        now = self.clock()
        with self._transaction():
            before = self.db.total_changes
            self.db.executemany(
                "INSERT OR IGNORE INTO fleet_profiles(profile_key,created_at) VALUES (?,?)",
                ((f"{prefix}-{i:06d}", now) for i in range(start, start + count))
            )
            added = self.db.total_changes - before
        return {"requested": count, "created": added, "existing": count - added}

    def set_enabled(self, profile_key: str, enabled: bool):
        if type(enabled) is not bool:
            raise ValueError("Etat invalide.")
        with self._transaction():
            cursor = self.db.execute(
                "UPDATE fleet_profiles SET enabled=? WHERE profile_key=?",
                (int(enabled), profile_key))
            if cursor.rowcount != 1:
                raise ValueError("Profil introuvable.")
        # Disabling prevents new leases; a lease already issued stays fenced.

    def enqueue_probe(self, profile_key: str, dedupe_key: str,
                      target_worker: str | None = None, probe_mode: str = "storage"):
        """Only a read-only diagnostic probe is allowed in the pilot queue."""
        if not isinstance(profile_key, str) or not _KEY.fullmatch(profile_key):
            raise ValueError("Identifiant de profil invalide.")
        if not isinstance(dedupe_key, str) or not _DEDUPE.fullmatch(dedupe_key):
            raise ValueError("Cle d'idempotence invalide.")
        if probe_mode not in ("storage", "android"):
            raise ValueError("Mode de sonde invalide.")
        if target_worker is not None and (
                not isinstance(target_worker, str) or not _WORKER.fullmatch(target_worker)):
            raise ValueError("Worker cible invalide.")
        if probe_mode == "android" and target_worker is None:
            raise ValueError("Une sonde Android doit cibler un worker precis.")
        now = self.clock()
        with self._transaction():
            old = self.db.execute(
                "SELECT id,profile_key,target_worker,probe_mode FROM fleet_jobs WHERE dedupe_key=?",
                (dedupe_key,)).fetchone()
            if old:
                if (old["profile_key"] != profile_key
                        or old["target_worker"] != target_worker
                        or old["probe_mode"] != probe_mode):
                    raise ValueError("Cle d'idempotence deja utilisee.")
                return {"job_id": old["id"], "duplicate": True}
            profile = self.db.execute(
                "SELECT enabled FROM fleet_profiles WHERE profile_key=?",
                (profile_key,)).fetchone()
            if not profile or not profile["enabled"]:
                raise ValueError("Profil absent ou desactive.")
            try:
                cursor = self.db.execute(
                    "INSERT INTO fleet_jobs(profile_key,kind,dedupe_key,state,"
                    "available_at,updated_at,target_worker,probe_mode) "
                    "VALUES (?,'health_probe',?,'queued',?,?,?,?)",
                    (profile_key, dedupe_key, now, now, target_worker, probe_mode))
            except sqlite3.IntegrityError as exc:
                raise ValueError("Une tache est deja en attente pour ce profil.") from exc
            return {"job_id": cursor.lastrowid, "duplicate": False}

    def _reclaim_expired(self, now):
        # A health probe is read-only and may be safely repeated after lease loss.
        # Any future side-effecting job type MUST fail closed to needs_review.
        self.db.execute("""
            UPDATE fleet_jobs
            SET state=CASE WHEN attempts>=3 THEN 'needs_review' ELSE 'queued' END,
                available_at=? + 5*attempts,
                lease_until=NULL, lease_owner=NULL, lease_token=NULL,
                updated_at=?
            WHERE state='leased' AND lease_until<=? AND kind='health_probe'
        """, (now, now, now))

    def heartbeat(self, worker_id: str, status: str = "ok", capacity: int = 1):
        if not isinstance(worker_id, str) or not _WORKER.fullmatch(worker_id):
            raise ValueError("Worker invalide.")
        if status not in ("ok", "busy", "degraded") or type(capacity) is not int or not 1 <= capacity <= 100:
            raise ValueError("Etat ou capacite invalide.")
        with self._transaction():
            self.db.execute("""
                INSERT INTO fleet_workers(worker_id,status,capacity,last_seen)
                VALUES(?,?,?,?)
                ON CONFLICT(worker_id) DO UPDATE SET
                    status=excluded.status,capacity=excluded.capacity,
                    last_seen=excluded.last_seen
            """, (worker_id, status, capacity, self.clock()))
        return {"ok": True}

    def workers(self, limit: int = 100):
        if type(limit) is not int or not 1 <= limit <= 200:
            raise ValueError("Limite invalide.")
        with self.lock:
            rows = [dict(r) for r in self.db.execute(
                "SELECT worker_id,status,capacity,last_seen FROM fleet_workers "
                "ORDER BY last_seen DESC LIMIT ?", (limit,))]
        cutoff = self.clock() - 180
        for row in rows:
            row["connected"] = row["last_seen"] >= cutoff
        return rows

    def claim(self, worker_id: str, limit: int = 1, lease_seconds: int = 90,
              capacity: int = 100):
        """Lease distinct profiles atomically, safe across SQLite connections."""
        if not isinstance(worker_id, str) or not _WORKER.fullmatch(worker_id):
            raise ValueError("Worker invalide.")
        if type(limit) is not int or not 1 <= limit <= 100:
            raise ValueError("Taille de lot invalide.")
        if type(lease_seconds) is not int or not 15 <= lease_seconds <= 3600:
            raise ValueError("Duree du bail invalide.")
        if type(capacity) is not int or not 1 <= capacity <= 100:
            raise ValueError("Capacite invalide.")
        now = self.clock()
        leased = []
        with self._transaction():
            self._reclaim_expired(now)
            active = self.db.execute(
                "SELECT COUNT(*) FROM fleet_jobs WHERE state='leased' "
                "AND lease_owner=? AND lease_until>?", (worker_id, now)).fetchone()[0]
            available = min(limit, max(0, capacity - active))
            if not available:
                return []
            rows = self.db.execute("""
                SELECT j.id,j.profile_key,j.kind,j.attempts,
                       j.target_worker,j.probe_mode
                FROM fleet_jobs j
                JOIN fleet_profiles p ON p.profile_key=j.profile_key
                WHERE j.state='queued' AND j.available_at<=? AND p.enabled=1
                  AND (j.target_worker IS NULL OR j.target_worker=?)
                ORDER BY j.available_at,j.id LIMIT ?
            """, (now, worker_id, available)).fetchall()
            for job in rows:
                token = secrets.token_urlsafe(32)
                until = now + lease_seconds
                self.db.execute("""
                    UPDATE fleet_jobs SET state='leased', attempts=attempts+1,
                    lease_owner=?, lease_token=?, lease_until=?, updated_at=?
                    WHERE id=? AND state='queued'
                """, (worker_id, token, until, now, job["id"]))
                leased.append({"job_id": job["id"], "profile_key": job["profile_key"],
                               "kind": job["kind"], "attempt": job["attempts"] + 1,
                               "target_worker": job["target_worker"],
                               "probe_mode": job["probe_mode"],
                               "lease_token": token, "lease_until": until})
        return leased

    def renew(self, job_id: int, worker_id: str, lease_token: str,
              lease_seconds: int = 90):
        if type(lease_seconds) is not int or not 15 <= lease_seconds <= 3600:
            raise ValueError("Duree du bail invalide.")
        now = self.clock()
        with self._transaction():
            cursor = self.db.execute("""
                UPDATE fleet_jobs SET lease_until=?, updated_at=?
                WHERE id=? AND state='leased' AND lease_owner=? AND lease_token=?
                      AND lease_until>?
            """, (now + lease_seconds, now, job_id, worker_id, lease_token, now))
            if cursor.rowcount != 1:
                raise ValueError("Bail expire ou ne vous appartenant pas.")
        return {"lease_until": now + lease_seconds}

    def complete(self, job_id: int, worker_id: str, lease_token: str,
                 success: bool):
        if type(success) is not bool:
            raise ValueError("Resultat invalide.")
        now = self.clock()
        with self._transaction():
            cursor = self.db.execute("""
                UPDATE fleet_jobs SET state=?, lease_owner=NULL,
                    lease_token=NULL,lease_until=NULL,updated_at=?
                WHERE id=? AND state='leased' AND lease_owner=?
                      AND lease_token=? AND lease_until>?
            """, ("done" if success else "needs_review", now,
                  job_id, worker_id, lease_token, now))
            if cursor.rowcount != 1:
                raise ValueError("Accuse refuse : bail expire ou deja termine.")
        return {"state": "done" if success else "needs_review"}

    def summary(self):
        with self.lock:
            total = self.db.execute(
                "SELECT COUNT(*) FROM fleet_profiles").fetchone()[0]
            enabled = self.db.execute(
                "SELECT COUNT(*) FROM fleet_profiles WHERE enabled=1").fetchone()[0]
            counts = {r["state"]: r["n"] for r in self.db.execute(
                "SELECT state,COUNT(*) AS n FROM fleet_jobs GROUP BY state")}
            ready = self.db.execute(
                "SELECT COUNT(*) FROM fleet_jobs WHERE state='queued' "
                "AND available_at<=?", (self.clock(),)).fetchone()[0]
        with self.lock:
            workers = self.db.execute(
                "SELECT COUNT(*) FROM fleet_workers WHERE last_seen>=?",
                (self.clock() - 180,)).fetchone()[0]
        return {"profiles": total, "enabled": enabled, "online_workers": workers,
                "jobs": {key: counts.get(key, 0) for key in
                         ("queued", "leased", "done", "needs_review")},
                "ready_jobs": ready, "scope": "logical-test-profiles-only",
                "backend": "single-node-sqlite"}

    def profiles(self, offset: int = 0, limit: int = 100):
        if type(offset) is not int or type(limit) is not int or not (
                0 <= offset <= 10_000_000 and 1 <= limit <= 200):
            raise ValueError("Pagination invalide.")
        with self.lock:
            return [dict(row) for row in self.db.execute(
                "SELECT profile_key,enabled,created_at FROM fleet_profiles "
                "ORDER BY profile_key LIMIT ? OFFSET ?", (limit, offset))]

    def ping(self):
        try:
            with self.lock:
                return self.db.execute("SELECT 1").fetchone()[0] == 1
        except sqlite3.Error:
            return False

    def close(self):
        with self.lock:
            self.db.close()
