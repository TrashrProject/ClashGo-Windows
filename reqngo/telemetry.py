"""Persistent structured diagnostics for trusted test workers; never stores raw logs or secrets."""
from __future__ import annotations

import json
import re
import sqlite3
import threading
import time
from pathlib import Path

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
                    version TEXT NOT NULL, last_seen REAL NOT NULL
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
            """)

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
                INSERT INTO agents(agent_id,status,version,last_seen) VALUES(?,?,?,?)
                ON CONFLICT(agent_id) DO UPDATE SET
                    status=excluded.status, version=excluded.version,
                    last_seen=excluded.last_seen
            """, (agent_id, status, version, at))
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
            self.db.execute("""
                DELETE FROM diagnostic_events WHERE id NOT IN (
                    SELECT id FROM diagnostic_events ORDER BY id DESC LIMIT 2000
                )
            """)
        return {"ok": True, "duplicate": False}

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
        for agent in agents:
            agent["connection"] = "stale" if now - agent["last_seen"] > stale_after else agent["status"]
        for event in events:
            event["metadata"] = json.loads(event["metadata"])
        return {"agents": agents, "events": events,
                "summary": {"connected": sum(a["connection"] != "stale" for a in agents),
                            "stale": sum(a["connection"] == "stale" for a in agents),
                            "recorded_events": len(events), "server_time": now},
                "scope": "test-workers-only"}
