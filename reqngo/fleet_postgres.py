"""PostgreSQL-backed, multi-server logical fleet scheduler.

The scope is read-only technical health probes, NOT external gameplay actions.
Every claim is fenced and workers never supply their own identity to the API.
Database timestamps avoid clock skew between application hosts.
"""
from __future__ import annotations

import re
import secrets

from psycopg.rows import dict_row
from psycopg_pool import ConnectionPool

from reqngo.fleet import _PREFIX, _KEY, _WORKER, _DEDUPE
from reqngo.fleet_metrics import clean_metrics


class PostgresFleetStore:
    def __init__(self, dsn: str, *, min_size: int = 1, max_size: int = 10):
        if not isinstance(dsn, str) or not dsn.startswith(("postgresql://", "postgres://")):
            raise ValueError("Une URL PostgreSQL est requise.")
        self.pool = ConnectionPool(conninfo=dsn, min_size=min_size, max_size=max_size,
                                   kwargs={"row_factory": dict_row}, open=True)
        try:
            with self.pool.connection() as conn:
                conn.execute("""
                    CREATE TABLE IF NOT EXISTS fleet_profiles (
                        profile_key text PRIMARY KEY,
                        enabled boolean NOT NULL DEFAULT true,
                        created_at timestamptz NOT NULL DEFAULT clock_timestamp()
                    )
                """)
                conn.execute("""
                    CREATE TABLE IF NOT EXISTS fleet_jobs (
                        id bigserial PRIMARY KEY,
                        profile_key text NOT NULL REFERENCES fleet_profiles(profile_key),
                        kind text NOT NULL CHECK(kind='health_probe'),
                        dedupe_key text NOT NULL UNIQUE,
                        state text NOT NULL DEFAULT 'queued'
                          CHECK(state IN ('queued','leased','done','needs_review')),
                        attempts integer NOT NULL DEFAULT 0,
                        available_at timestamptz NOT NULL DEFAULT clock_timestamp(),
                        lease_until timestamptz,
                        lease_owner text,
                        lease_token text,
                        updated_at timestamptz NOT NULL DEFAULT clock_timestamp()
                    )
                """)
                # Existing installations acquire additive columns without data loss.
                conn.execute("ALTER TABLE fleet_jobs ADD COLUMN IF NOT EXISTS target_worker text")
                conn.execute("ALTER TABLE fleet_jobs ADD COLUMN IF NOT EXISTS "
                             "probe_mode text NOT NULL DEFAULT 'storage'")
                conn.execute("""
                    CREATE INDEX IF NOT EXISTS fleet_target_ready
                    ON fleet_jobs(state,target_worker,available_at,id)
                """)
                conn.execute("""
                    CREATE UNIQUE INDEX IF NOT EXISTS fleet_one_open_job
                    ON fleet_jobs(profile_key) WHERE state IN ('queued','leased')
                """)
                conn.execute("""
                    CREATE INDEX IF NOT EXISTS fleet_ready_jobs
                    ON fleet_jobs(state, available_at, id)
                """)
                conn.execute("""
                    CREATE TABLE IF NOT EXISTS fleet_workers (
                        worker_id text PRIMARY KEY,
                        status text NOT NULL CHECK(status IN ('ok','busy','degraded')),
                        capacity integer NOT NULL CHECK(capacity BETWEEN 1 AND 100),
                        last_seen timestamptz NOT NULL DEFAULT clock_timestamp()
                    )
                """)
                for field, sql_type in (
                    ("cpu_percent", "double precision"),
                    ("memory_percent", "double precision"),
                    ("process_rss_mb", "double precision"),
                    ("android_online_devices", "integer"),
                    ("android_bound_devices", "integer")):
                    conn.execute("ALTER TABLE fleet_workers ADD COLUMN IF NOT EXISTS "
                                 + field + " " + sql_type)
                conn.execute("""
                    CREATE TABLE IF NOT EXISTS fleet_worker_incidents (
                        worker_id text PRIMARY KEY REFERENCES fleet_workers(worker_id),
                        status text NOT NULL CHECK(status IN ('open','resolved')),
                        occurrences integer NOT NULL DEFAULT 1,
                        first_seen timestamptz NOT NULL DEFAULT clock_timestamp(),
                        last_seen timestamptz NOT NULL,
                        resolved_at timestamptz
                    )
                """)
        except BaseException:
            self.pool.close()
            raise

    def register_batch(self, prefix: str, start: int, count: int):
        if not isinstance(prefix, str) or not _PREFIX.fullmatch(prefix):
            raise ValueError("Prefixe de flotte invalide.")
        if type(start) is not int or type(count) is not int or not (
                1 <= start <= 9_999_999 and 1 <= count <= 10_000
                and start + count - 1 <= 9_999_999):
            raise ValueError("Plage de profils invalide.")
        with self.pool.connection() as conn:
            inserted = conn.execute("""
                WITH added AS (
                  INSERT INTO fleet_profiles (profile_key)
                  SELECT %s || '-' || lpad(n::text,6,'0')
                  FROM generate_series(%s::int,%s::int) n
                  ON CONFLICT (profile_key) DO NOTHING
                  RETURNING 1
                )
                SELECT COUNT(*) AS n FROM added
            """, (prefix, start, start + count - 1)).fetchone()["n"]
        return {"requested": count, "created": inserted, "existing": count - inserted}

    def set_enabled(self, profile_key: str, enabled: bool):
        if type(enabled) is not bool:
            raise ValueError("Etat invalide.")
        with self.pool.connection() as conn:
            result = conn.execute(
                "UPDATE fleet_profiles SET enabled=%s WHERE profile_key=%s",
                (enabled, profile_key))
            if result.rowcount != 1:
                raise ValueError("Profil introuvable.")

    def enqueue_probe(self, profile_key: str, dedupe_key: str,
                      target_worker: str | None = None, probe_mode: str = "storage"):
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
        with self.pool.connection() as conn:
            old = conn.execute(
                "SELECT id,profile_key,target_worker,probe_mode FROM fleet_jobs WHERE dedupe_key=%s",
                (dedupe_key,)).fetchone()
            if old:
                if (old["profile_key"] != profile_key
                        or old["target_worker"] != target_worker
                        or old["probe_mode"] != probe_mode):
                    raise ValueError("Cle d'idempotence deja utilisee.")
                return {"job_id": old["id"], "duplicate": True}
            profile = conn.execute(
                "SELECT enabled FROM fleet_profiles WHERE profile_key=%s",
                (profile_key,)).fetchone()
            if not profile or not profile["enabled"]:
                raise ValueError("Profil absent ou desactive.")
            # ON CONFLICT avoids aborting the transaction on a racing insert.
            job = conn.execute("""
                INSERT INTO fleet_jobs(profile_key,kind,dedupe_key,target_worker,probe_mode)
                VALUES(%s,'health_probe',%s,%s,%s)
                ON CONFLICT DO NOTHING RETURNING id
            """, (profile_key, dedupe_key, target_worker, probe_mode)).fetchone()
            if job:
                return {"job_id": job["id"], "duplicate": False}
            old = conn.execute(
                "SELECT id,profile_key,target_worker,probe_mode FROM fleet_jobs WHERE dedupe_key=%s",
                (dedupe_key,)).fetchone()
            if (old and old["profile_key"] == profile_key
                    and old["target_worker"] == target_worker
                    and old["probe_mode"] == probe_mode):
                return {"job_id": old["id"], "duplicate": True}
            if old:
                raise ValueError("Cle d'idempotence deja utilisee.")
            raise ValueError("Une tache est deja en attente pour ce profil.")

    @staticmethod
    def _reclaim(conn):
        # Short batches prevent one transaction from locking the entire stale
        # queue. Fail closed when all safe retries are exhausted.
        conn.execute("""
            WITH expired AS (
                SELECT id FROM fleet_jobs
                WHERE state='leased' AND lease_until<=clock_timestamp()
                      AND kind='health_probe'
                ORDER BY lease_until, id
                LIMIT 200 FOR UPDATE SKIP LOCKED
            )
            UPDATE fleet_jobs j SET
                state=CASE WHEN j.attempts>=3 THEN 'needs_review' ELSE 'queued' END,
                available_at=clock_timestamp() + (j.attempts * INTERVAL '5 seconds'),
                lease_until=NULL,lease_owner=NULL,lease_token=NULL,
                updated_at=clock_timestamp()
            FROM expired WHERE j.id=expired.id
        """)

    def heartbeat(self, worker_id: str, status: str = "ok", capacity: int = 1, *,
                  metrics=None):
        if not isinstance(worker_id, str) or not _WORKER.fullmatch(worker_id):
            raise ValueError("Worker invalide.")
        if status not in ("ok", "busy", "degraded") or type(capacity) is not int or not 1 <= capacity <= 100:
            raise ValueError("Etat ou capacite invalide.")
        metric = clean_metrics(metrics)
        with self.pool.connection() as conn:
            conn.execute("""
                INSERT INTO fleet_workers(
                    worker_id,status,capacity,cpu_percent,memory_percent,
                    process_rss_mb,android_online_devices,android_bound_devices)
                VALUES(%s,%s,%s,%s,%s,%s,%s,%s)
                ON CONFLICT (worker_id) DO UPDATE SET
                    status=excluded.status,capacity=excluded.capacity,
                    last_seen=clock_timestamp(),
                    cpu_percent=excluded.cpu_percent,
                    memory_percent=excluded.memory_percent,
                    process_rss_mb=excluded.process_rss_mb,
                    android_online_devices=excluded.android_online_devices,
                    android_bound_devices=excluded.android_bound_devices
            """, (worker_id, status, capacity,
                  *(metric[name] for name in (
                      "cpu_percent", "memory_percent", "process_rss_mb",
                      "android_online_devices", "android_bound_devices"))))
            conn.execute("""
                UPDATE fleet_worker_incidents
                SET status='resolved', resolved_at=clock_timestamp()
                WHERE worker_id=%s AND status='open'
            """, (worker_id,))
        return {"ok": True}

    def scan_worker_health(self, stale_seconds: int = 180):
        """Idempotent cross-replica worker outage detection."""
        if type(stale_seconds) is not int or not 30 <= stale_seconds <= 3600:
            raise ValueError("Delai de detection invalide.")
        with self.pool.connection() as conn:
            created = conn.execute("""
                WITH stale AS (
                    SELECT worker_id,last_seen FROM fleet_workers
                    WHERE last_seen < clock_timestamp() - %s * INTERVAL '1 second'
                    FOR UPDATE
                )
                INSERT INTO fleet_worker_incidents
                    (worker_id,status,occurrences,first_seen,last_seen,resolved_at)
                SELECT worker_id,'open',1,clock_timestamp(),last_seen,NULL
                FROM stale WHERE TRUE
                ON CONFLICT(worker_id) DO UPDATE SET
                    status='open',
                    occurrences=fleet_worker_incidents.occurrences+1,
                    first_seen=excluded.first_seen,
                    last_seen=excluded.last_seen,
                    resolved_at=NULL
                WHERE fleet_worker_incidents.status='resolved'
                RETURNING worker_id
            """, (stale_seconds,)).fetchall()
            total = conn.execute(
                "SELECT COUNT(*) AS n FROM fleet_worker_incidents WHERE status='open'"
            ).fetchone()["n"]
        return {"new_incidents": len(created), "open_incidents": total}

    def worker_incidents(self, limit: int = 100):
        if type(limit) is not int or not 1 <= limit <= 200:
            raise ValueError("Limite invalide.")
        with self.pool.connection() as conn:
            rows = conn.execute("""
                SELECT worker_id,status,occurrences,
                    EXTRACT(EPOCH FROM first_seen) AS first_seen,
                    EXTRACT(EPOCH FROM last_seen) AS last_seen,
                    EXTRACT(EPOCH FROM resolved_at) AS resolved_at
                FROM fleet_worker_incidents
                ORDER BY CASE WHEN status='open' THEN 0 ELSE 1 END,
                         first_seen DESC LIMIT %s
            """, (limit,)).fetchall()
        return [dict(r, first_seen=float(r["first_seen"]),
                     last_seen=float(r["last_seen"]),
                     resolved_at=float(r["resolved_at"]) if r["resolved_at"] is not None else None)
                for r in rows]

    def workers(self, limit: int = 100):
        if type(limit) is not int or not 1 <= limit <= 200:
            raise ValueError("Limite invalide.")
        with self.pool.connection() as conn:
            rows = conn.execute("""
                SELECT worker_id,status,capacity,cpu_percent,memory_percent,
                       process_rss_mb,android_online_devices,android_bound_devices,
                       EXTRACT(EPOCH FROM last_seen) AS last_seen,
                       last_seen >= clock_timestamp()- INTERVAL '180 seconds' AS connected
                FROM fleet_workers ORDER BY last_seen DESC LIMIT %s
            """, (limit,)).fetchall()
        return [dict(r, last_seen=float(r["last_seen"])) for r in rows]

    def claim(self, worker_id: str, limit: int = 1, lease_seconds: int = 90,
              capacity: int = 100):
        if not isinstance(worker_id, str) or not _WORKER.fullmatch(worker_id):
            raise ValueError("Worker invalide.")
        if type(limit) is not int or not 1 <= limit <= 100:
            raise ValueError("Taille de lot invalide.")
        if type(lease_seconds) is not int or not 15 <= lease_seconds <= 3600:
            raise ValueError("Duree du bail invalide.")
        if type(capacity) is not int or not 1 <= capacity <= 100:
            raise ValueError("Capacite invalide.")
        leased = []
        with self.pool.connection() as conn:
            # Serialize concurrent claims by the SAME worker across API replicas,
            # while other worker IDs can claim independent rows concurrently.
            conn.execute("SELECT pg_advisory_xact_lock(hashtext(%s)::bigint)",
                         (worker_id,))
            self._reclaim(conn)
            active = conn.execute("""
                SELECT COUNT(*) AS n FROM fleet_jobs
                WHERE state='leased' AND lease_owner=%s
                      AND lease_until>clock_timestamp()
            """, (worker_id,)).fetchone()["n"]
            available = min(limit, max(0, capacity - active))
            if not available:
                return []
            rows = conn.execute("""
                SELECT j.id,j.profile_key,j.kind,j.attempts,
                       j.target_worker,j.probe_mode
                FROM fleet_jobs j
                JOIN fleet_profiles p ON p.profile_key=j.profile_key
                WHERE j.state='queued' AND j.available_at<=clock_timestamp()
                      AND p.enabled=true
                      AND (j.target_worker IS NULL OR j.target_worker=%s)
                ORDER BY j.available_at,j.id
                LIMIT %s FOR UPDATE OF j SKIP LOCKED
            """, (worker_id, available)).fetchall()
            for job in rows:
                token = secrets.token_urlsafe(32)
                result = conn.execute("""
                    UPDATE fleet_jobs
                    SET state='leased',attempts=attempts+1,lease_owner=%s,
                        lease_token=%s,
                        lease_until=clock_timestamp() + %s * INTERVAL '1 second',
                        updated_at=clock_timestamp()
                    WHERE id=%s RETURNING EXTRACT(EPOCH FROM lease_until) AS until
                """, (worker_id, token, lease_seconds, job["id"])).fetchone()
                leased.append({"job_id": job["id"], "profile_key": job["profile_key"],
                               "kind": job["kind"], "attempt": job["attempts"] + 1,
                               "target_worker": job["target_worker"],
                               "probe_mode": job["probe_mode"],
                               "lease_token": token, "lease_until": float(result["until"])})
        return leased

    def renew(self, job_id: int, worker_id: str, lease_token: str,
              lease_seconds: int = 90):
        if type(lease_seconds) is not int or not 15 <= lease_seconds <= 3600:
            raise ValueError("Duree du bail invalide.")
        with self.pool.connection() as conn:
            row = conn.execute("""
                UPDATE fleet_jobs SET
                    lease_until=clock_timestamp() + %s * INTERVAL '1 second',
                    updated_at=clock_timestamp()
                WHERE id=%s AND state='leased' AND lease_owner=%s
                      AND lease_token=%s AND lease_until>clock_timestamp()
                RETURNING EXTRACT(EPOCH FROM lease_until) AS until
            """, (lease_seconds, job_id, worker_id, lease_token)).fetchone()
            if not row:
                raise ValueError("Bail expire ou ne vous appartenant pas.")
        return {"lease_until": float(row["until"])}

    def complete(self, job_id: int, worker_id: str, lease_token: str,
                 success: bool):
        if type(success) is not bool:
            raise ValueError("Resultat invalide.")
        state = "done" if success else "needs_review"
        with self.pool.connection() as conn:
            row = conn.execute("""
                UPDATE fleet_jobs SET state=%s, lease_owner=NULL,lease_token=NULL,
                                      lease_until=NULL,updated_at=clock_timestamp()
                WHERE id=%s AND state='leased' AND lease_owner=%s
                      AND lease_token=%s AND lease_until>clock_timestamp()
                RETURNING id
            """, (state, job_id, worker_id, lease_token)).fetchone()
            if not row:
                raise ValueError("Accuse refuse : bail expire ou deja termine.")
        return {"state": state}

    def summary(self):
        with self.pool.connection() as conn:
            n = conn.execute("""
                SELECT COUNT(*) AS profiles,COUNT(*) FILTER (WHERE enabled) AS enabled
                FROM fleet_profiles
            """).fetchone()
            rows = conn.execute(
                "SELECT state,COUNT(*) AS n FROM fleet_jobs GROUP BY state").fetchall()
            ready = conn.execute("""
                SELECT COUNT(*) AS n FROM fleet_jobs WHERE state='queued'
                      AND available_at<=clock_timestamp()
            """).fetchone()["n"]
            work = conn.execute("""
                SELECT COUNT(*) FILTER (
                    WHERE last_seen >= clock_timestamp()-INTERVAL '180 seconds'
                ) AS online,
                COUNT(*) AS total,
                COALESCE(SUM(capacity) FILTER (
                    WHERE last_seen >= clock_timestamp()-INTERVAL '180 seconds'
                ), 0) AS slots,
                COUNT(*) FILTER (
                    WHERE last_seen >= clock_timestamp()-INTERVAL '180 seconds'
                      AND (cpu_percent>=95 OR memory_percent>=95)
                ) AS overloaded FROM fleet_workers
            """).fetchone()
            incidents = conn.execute(
                "SELECT COUNT(*) AS n FROM fleet_worker_incidents WHERE status='open'"
            ).fetchone()["n"]
            oldest = conn.execute("""
                SELECT COALESCE(GREATEST(0,EXTRACT(EPOCH FROM
                    (clock_timestamp()-MIN(available_at)))),0) AS seconds
                FROM fleet_jobs
                WHERE state='queued' AND available_at<=clock_timestamp()
            """).fetchone()["seconds"]
        counts = {r["state"]: r["n"] for r in rows}
        return {"profiles": n["profiles"], "enabled": n["enabled"],
                "total_workers": work["total"], "online_slots": work["slots"],
                "overloaded_workers": work["overloaded"],
                "open_worker_incidents": incidents,
                "oldest_ready_wait_seconds": int(oldest),
                "jobs": {k: counts.get(k, 0) for k in
                         ("queued", "leased", "done", "needs_review")},
                "ready_jobs": ready, "online_workers": work["online"],
                "scope": "logical-test-profiles-only",
                "backend": "distributed-postgres"}

    def profiles(self, offset: int = 0, limit: int = 100):
        if type(offset) is not int or type(limit) is not int or not (
                0 <= offset <= 10_000_000 and 1 <= limit <= 200):
            raise ValueError("Pagination invalide.")
        with self.pool.connection() as conn:
            rows = conn.execute("""
                SELECT profile_key, enabled,
                       EXTRACT(EPOCH FROM created_at) AS created_at
                FROM fleet_profiles ORDER BY profile_key LIMIT %s OFFSET %s
            """, (limit, offset)).fetchall()
        return [dict(r, created_at=float(r["created_at"])) for r in rows]

    def ping(self):
        try:
            with self.pool.connection(timeout=3) as conn:
                return conn.execute("SELECT 1 AS ok").fetchone()["ok"] == 1
        except Exception:
            return False

    def close(self):
        self.pool.close()
