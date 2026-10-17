"""Persistent structured diagnostics for trusted test workers; never stores raw logs or secrets."""
from __future__ import annotations

import json
import re
import sqlite3
import threading
import time
from pathlib import Path

from reqngo.triage import classify, more_severe

_AGENT = re.compile(r"^[A-Za-z0-9_-]{1,40}$")
_CODE = re.compile(r"^[a-z][a-z0-9_]{2,63}$")
_TRACE = re.compile(r"^[A-Za-z0-9_-]{1,80}$")
_STATES = {"ok", "busy", "degraded"}
_LEVELS = {"info", "warning", "error", "critical"}
_ALLOWED_META = {"component", "phase", "error_type", "trace_id", "duration_ms", "retry_count", "version"}


class TelemetryStore:
    def __init__(self, path, clock=None):
        self.clock = clock or time.time
        self.lock = threading.RLock()
        path = Path(path)
        path.parent.mkdir(parents=True, exist_ok=True)
        self.db = sqlite3.connect(str(path), check_same_thread=False, timeout=5)
        self.db.row_factory = sqlite3.Row
        with self.lock, self.db:
            self.db.execute("PRAGMA journal_mode=WAL")
            self.db.executescript("""
                CREATE TABLE IF NOT EXISTS agents (
                    agent_id TEXT PRIMARY KEY, status TEXT NOT NULL,
                    version TEXT NOT NULL, last_seen REAL NOT NULL,
                    heartbeat_seq INTEGER NOT NULL DEFAULT 0
                );
                CREATE TABLE IF NOT EXISTS diagnostic_events (
                    id INTEGER PRIMARY KEY AUTOINCREMENT,
                    event_key TEXT NOT NULL UNIQUE,
                    agent_id TEXT NOT NULL, profile_id INTEGER,
                    code TEXT NOT NULL, level TEXT NOT NULL,
                    metadata TEXT NOT NULL, recorded_at REAL NOT NULL
                );
                CREATE INDEX IF NOT EXISTS idx_diagnostic_events_time
                    ON diagnostic_events(recorded_at DESC);
                CREATE TABLE IF NOT EXISTS diagnostic_cases (
                    id INTEGER PRIMARY KEY AUTOINCREMENT,
                    agent_id TEXT NOT NULL, profile_id INTEGER,
                    code TEXT NOT NULL, severity TEXT NOT NULL,
                    category TEXT NOT NULL, summary TEXT NOT NULL,
                    recommended_action TEXT NOT NULL,
                    status TEXT NOT NULL, occurrences INTEGER NOT NULL DEFAULT 1,
                    first_seen REAL NOT NULL, last_seen REAL NOT NULL, verified_at REAL
                );
                CREATE INDEX IF NOT EXISTS idx_cases_status
                    ON diagnostic_cases(status, last_seen DESC);
            """)
            # Older SQLite databases predate heartbeat_seq. Preserve every
            # existing event and case when upgrading the schema.
            columns = {row["name"] for row in self.db.execute("PRAGMA table_info(agents)")}
            if "heartbeat_seq" not in columns:
                self.db.execute(
                    "ALTER TABLE agents ADD COLUMN heartbeat_seq INTEGER NOT NULL DEFAULT 0"
                )

    def close(self):
        with self.lock:
            self.db.close()

    @staticmethod
    def _name(value):
        if not isinstance(value, str) or not _AGENT.fullmatch(value):
            raise ValueError("Identifiant d'agent invalide.")
        return value

    @staticmethod
    def _profile(value):
        if value is None:
            return None
        if type(value) is not int or not 1 <= value <= 10000:
            raise ValueError("Identifiant de profil invalide.")
        return value

    @staticmethod
    def _metadata(value):
        if value is None:
            return {}
        if not isinstance(value, dict) or len(value) > 7 or set(value) - _ALLOWED_META:
            raise ValueError("Metadonnees hors liste autorisee.")
        result = {}
        for key, val in value.items():
            if key in ("duration_ms", "retry_count"):
                if type(val) is not int or not 0 <= val <= 10_000_000:
                    raise ValueError("Mesure invalide : " + key)
            elif not isinstance(val, str) or not _TRACE.fullmatch(val):
                raise ValueError("Valeur non structuree interdite : " + key)
            result[key] = val
        return result

    def heartbeat(self, agent_id, status="ok", version="unknown"):
        agent_id = self._name(agent_id)
        if status not in _STATES:
            raise ValueError("Etat inconnu.")
        if not isinstance(version, str) or not _TRACE.fullmatch(version):
            raise ValueError("Version invalide.")
        at = self.clock()
        with self.lock, self.db:
            self.db.execute("""
                INSERT INTO agents(agent_id,status,version,last_seen,heartbeat_seq)
                VALUES(?,?,?,?,1)
                ON CONFLICT(agent_id) DO UPDATE SET
                    status=excluded.status, version=excluded.version,
                    last_seen=excluded.last_seen,
                    heartbeat_seq=agents.heartbeat_seq+1
            """, (agent_id, status, version, at))
            # A new heartbeat proves connectivity, not the health of other components.
            self._verify_case(agent_id, None, "worker_unreachable", at)
        return {"ok": True, "agent_id": agent_id, "server_time": at}

    def report(self, agent_id, event_key, code, level="error", profile_id=None, metadata=None):
        agent_id = self._name(agent_id)
        if not isinstance(event_key, str) or not _TRACE.fullmatch(event_key):
            raise ValueError("Identifiant unique d'evenement invalide.")
        if not isinstance(code, str) or not _CODE.fullmatch(code):
            raise ValueError("Code d'erreur invalide.")
        if level not in _LEVELS:
            raise ValueError("Niveau inconnu.")
        profile_id = self._profile(profile_id)
        metadata = self._metadata(metadata)
        if code == "probe_success":
            restored = metadata.get("error_type")
            if level != "info" or not isinstance(restored, str) or not _CODE.fullmatch(restored) or restored == "probe_success":
                raise ValueError("Une verification exige un code source structure et un niveau info.")
        at = self.clock()
        with self.lock, self.db:
            old = self.db.execute("SELECT agent_id FROM diagnostic_events WHERE event_key=?", (event_key,)).fetchone()
            if old is not None:
                if old["agent_id"] != agent_id:
                    raise ValueError("Identifiant deja utilise par un autre agent.")
                return {"ok": True, "duplicate": True}
            self.db.execute("""
                INSERT INTO diagnostic_events(event_key,agent_id,profile_id,code,level,metadata,recorded_at)
                VALUES (?,?,?,?,?,?,?)
            """, (event_key, agent_id, profile_id, code, level,
                  json.dumps(metadata, ensure_ascii=False, sort_keys=True), at))
            if code == "probe_success":
                self._verify_case(agent_id, profile_id, metadata["error_type"], at)
            elif level in ("warning", "error", "critical"):
                self._record_case(agent_id, profile_id, code, level, at)
            self.db.execute("""
                DELETE FROM diagnostic_events WHERE id NOT IN (
                    SELECT id FROM diagnostic_events ORDER BY id DESC LIMIT 2000
                )
            """)
        return {"ok": True, "duplicate": False}

    def _record_case(self, agent_id, profile_id, code, level, at):
        diagnosis = classify(code, level)
        existing = self.db.execute(
            "SELECT id,severity FROM diagnostic_cases "
            "WHERE agent_id=? AND profile_id IS ? AND code=? AND status='open' "
            "ORDER BY id DESC LIMIT 1", (agent_id, profile_id, code)
        ).fetchone()
        if existing:
            self.db.execute(
                "UPDATE diagnostic_cases SET occurrences=occurrences+1, "
                "last_seen=?,severity=? WHERE id=?",
                (at, more_severe(existing["severity"], diagnosis["severity"]), existing["id"])
            )
        else:
            self.db.execute(
                "INSERT INTO diagnostic_cases "
                "(agent_id,profile_id,code,severity,category,summary,recommended_action,"
                "status,first_seen,last_seen) VALUES (?,?,?,?,?,?,?,'open',?,?)",
                (agent_id, profile_id, code, diagnosis["severity"], diagnosis["category"],
                 diagnosis["summary"], diagnosis["recommended_action"], at, at)
            )

    def _verify_case(self, agent_id, profile_id, code, at):
        """Close only a specifically observed recovery, never guess based on elapsed time."""
        self.db.execute(
            "UPDATE diagnostic_cases SET status='verified',verified_at=? "
            "WHERE agent_id=? AND profile_id IS ? AND code=? AND status='open'",
            (at, agent_id, profile_id, code)
        )

    def check_stale(self, stale_after=180):
        """Record at most one alert per missed heartbeat window; never touch a worker."""
        if type(stale_after) is not int or not 30 <= stale_after <= 3600:
            raise ValueError("Seuil de surveillance invalide.")
        with self.lock:
            stale = [(row["agent_id"], row["heartbeat_seq"]) for row in self.db.execute(
                "SELECT agent_id,heartbeat_seq FROM agents WHERE last_seen<?",
                (self.clock() - stale_after,))]
        created = 0
        for agent_id, sequence in stale:
            # Sequence changes on EVERY accepted heartbeat, even if the
            # timestamp is unchanged: distinct outages cannot collide.
            event_key = "stale_" + agent_id + "_" + str(sequence)
            result = self.report(agent_id, event_key, "worker_unreachable", "warning",
                                 metadata={"component": "heartbeat_monitor"})
            created += int(not result["duplicate"])
        return {"alerts_created": created}

    def snapshot(self, stale_after=180, limit=100):
        if type(stale_after) is not int or not 30 <= stale_after <= 3600:
            raise ValueError("Seuil de surveillance invalide.")
        if type(limit) is not int or not 1 <= limit <= 200:
            raise ValueError("Limite de resultats invalide.")
        now = self.clock()
        with self.lock:
            agents = [dict(row) for row in self.db.execute("SELECT * FROM agents ORDER BY agent_id")]
            events = [dict(row) for row in self.db.execute(
                "SELECT id,agent_id,profile_id,code,level,metadata,recorded_at "
                "FROM diagnostic_events ORDER BY id DESC LIMIT ?", (limit,))]
            cases = [dict(row) for row in self.db.execute(
                "SELECT * FROM diagnostic_cases ORDER BY "
                "CASE WHEN status='open' THEN 0 ELSE 1 END, last_seen DESC LIMIT ?",
                (limit,)
            )]
            open_cases = self.db.execute(
                "SELECT COUNT(*) FROM diagnostic_cases WHERE status='open'"
            ).fetchone()[0]
        for agent in agents:
            agent["connection"] = "stale" if now - agent["last_seen"] > stale_after else agent["status"]
        for event in events:
            event["metadata"] = json.loads(event["metadata"])
        return {"agents": agents, "events": events, "cases": cases,
                "summary": {"connected": sum(a["connection"] != "stale" for a in agents),
                            "stale": sum(a["connection"] == "stale" for a in agents),
                            "recorded_events": len(events), "open_cases": open_cases,
                            "server_time": now},
                "scope": "test-workers-only"}
