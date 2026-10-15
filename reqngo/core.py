"""Isolated monitoring of fictional profiles, with durable incident history.

No game connection, emulator control, account credentials or remote execution.
Repairs change only the local simulated profile state.
"""
from __future__ import annotations

import json
import sqlite3
import threading
import time
from pathlib import Path

KNOWN = {
    "session_frozen": "Session pilote figée",
    "network_timeout": "Connexion pilote interrompue",
    "worker_crash": "Service pilote arrêté",
    "invalid_config": "Configuration invalide",
    "unknown_failure": "Erreur inconnue",
}
AUTO = {"session_frozen", "network_timeout", "worker_crash"}


class Guardian:
    def __init__(self, path, clock=None):
        self.clock = clock or time.time
        self.lock = threading.RLock()
        path = Path(path)
        path.parent.mkdir(parents=True, exist_ok=True)
        self.db = sqlite3.connect(str(path), check_same_thread=False, timeout=5)
        self.db.row_factory = sqlite3.Row
        self.db.execute("PRAGMA journal_mode=WAL")
        with self.lock, self.db:
            self.db.executescript("""
              CREATE TABLE IF NOT EXISTS accounts (
                id INTEGER PRIMARY KEY, name TEXT NOT NULL,
                pilot INTEGER NOT NULL DEFAULT 0,
                enabled INTEGER NOT NULL DEFAULT 1,
                status TEXT NOT NULL DEFAULT 'ok',
                fault TEXT, retry_at REAL NOT NULL DEFAULT 0,
                updated_at REAL NOT NULL
              );
              CREATE TABLE IF NOT EXISTS incidents (
                id INTEGER PRIMARY KEY AUTOINCREMENT,
                account_id INTEGER NOT NULL, code TEXT NOT NULL,
                status TEXT NOT NULL, attempts INTEGER NOT NULL DEFAULT 0,
                details TEXT NOT NULL, opened_at REAL NOT NULL, closed_at REAL
              );
              CREATE TABLE IF NOT EXISTS events (
                id INTEGER PRIMARY KEY AUTOINCREMENT, at REAL NOT NULL,
                account_id INTEGER, level TEXT NOT NULL, message TEXT NOT NULL
              );
              CREATE TABLE IF NOT EXISTS config (
                key TEXT PRIMARY KEY, value TEXT NOT NULL
              );
            """)
            defaults = {
                "scan_interval_seconds": 600,
                "auto_repair": True,
                "retry_delay_seconds": 8,
                "max_attempts": 2,
                "pilot_mode": True,
                "last_full_scan": 0,
            }
            for key, value in defaults.items():
                self.db.execute("INSERT OR IGNORE INTO config VALUES (?,?)",
                                (key, json.dumps(value)))
            if not self.db.execute("SELECT 1 FROM accounts LIMIT 1").fetchone():
                now = self.clock()
                self.db.executemany(
                    "INSERT INTO accounts(id,name,pilot,updated_at) VALUES (?,?,?,?)",
                    [(i, "ACC_%02d" % i, int(i <= 3), now) for i in range(1, 51)]
                )
                self._event(None, "info", "50 profils fictifs initialisés ; 3 pilotes actifs.")

    def close(self):
        with self.lock:
            self.db.close()

    def _event(self, account, level, message):
        self.db.execute(
            "INSERT INTO events(at,account_id,level,message) VALUES (?,?,?,?)",
            (self.clock(), account, level, message)
        )
        self.db.execute(
            "DELETE FROM events WHERE id NOT IN "
            "(SELECT id FROM events ORDER BY id DESC LIMIT 500)"
        )

    def _settings(self):
        return {r["key"]: json.loads(r["value"])
                for r in self.db.execute("SELECT * FROM config")}

    def settings(self):
        with self.lock:
            return self._settings()

    def configure(self, changes):
        allowed = {
            "scan_interval_seconds": (int, 30, 3600),
            "retry_delay_seconds": (int, 1, 3600),
            "max_attempts": (int, 0, 5),
        }
        for key, value in changes.items():
            if key in allowed:
                typ, lo, hi = allowed[key]
                if type(value) is not typ or not lo <= value <= hi:
                    raise ValueError("Valeur incorrecte : " + key)
            elif key in ("auto_repair", "pilot_mode"):
                if type(value) is not bool:
                    raise ValueError("Booléen requis : " + key)
            else:
                raise ValueError("Réglage inconnu : " + key)
        with self.lock, self.db:
            for key, value in changes.items():
                self.db.execute("UPDATE config SET value=? WHERE key=?",
                                (json.dumps(value), key))
            self._event(None, "info", "Configuration modifiée sans arrêt général.")
        return self.settings()

    def accounts(self):
        with self.lock:
            return [dict(r) for r in self.db.execute("SELECT * FROM accounts ORDER BY id")]

    def incidents(self, limit=50):
        with self.lock:
            return [dict(r) for r in self.db.execute(
                "SELECT * FROM incidents ORDER BY id DESC LIMIT ?", (min(int(limit), 500),)
            )]

    def events(self, limit=60):
        with self.lock:
            return [dict(r) for r in self.db.execute(
                "SELECT * FROM events ORDER BY id DESC LIMIT ?", (min(int(limit), 500),)
            )]

    def overview(self):
        with self.lock:
            cfg = self._settings()
            accounts = self.accounts()
            selected = [a for a in accounts if a["enabled"] and
                        (not cfg["pilot_mode"] or a["pilot"])]
            open_count = self.db.execute(
                "SELECT COUNT(*) FROM incidents WHERE status!='resolved'"
            ).fetchone()[0]
            return {
                "total": len(accounts),
                "pilots": sum(a["pilot"] for a in accounts),
                "monitored": len(selected),
                "healthy": sum(a["status"] == "ok" for a in selected),
                "errors": sum(a["status"] == "error" for a in selected),
                "open_incidents": open_count,
                "last_scan": cfg["last_full_scan"],
                "simulation": True,
            }

    def set_pilots(self, ids):
        if (not isinstance(ids, list) or not 1 <= len(ids) <= 3 or
                any(type(i) is not int for i in ids) or len(ids) != len(set(ids))):
            raise ValueError("Sélectionner 1 à 3 pilotes distincts.")
        with self.lock, self.db:
            known = {r["id"] for r in self.db.execute("SELECT id FROM accounts")}
            if not set(ids) <= known:
                raise ValueError("Profil inexistant.")
            self.db.execute("UPDATE accounts SET pilot=0")
            self.db.executemany("UPDATE accounts SET pilot=1 WHERE id=?",
                                [(i,) for i in ids])
            self._event(None, "info", "Pilotes modifiés.")
        return ids

    def enable(self, account_id, enabled):
        if type(enabled) is not bool:
            raise ValueError("Activation invalide.")
        with self.lock, self.db:
            cur = self.db.execute("UPDATE accounts SET enabled=?,updated_at=? WHERE id=?",
                                  (int(enabled), self.clock(), account_id))
            if cur.rowcount != 1:
                raise ValueError("Profil introuvable.")
            self._event(account_id, "info", "Profil activé." if enabled else "Profil suspendu.")

    def inject(self, account_id, code):
        if code not in KNOWN:
            raise ValueError("Code d'incident inconnu.")
        with self.lock, self.db:
            acc = self.db.execute("SELECT * FROM accounts WHERE id=?", (account_id,)).fetchone()
            if not acc or not acc["enabled"]:
                raise ValueError("Compte absent ou suspendu.")
            if self._settings()["pilot_mode"] and not acc["pilot"]:
                raise ValueError("Hors des pilotes actifs.")
            if acc["fault"]:
                raise ValueError("Incident déjà ouvert sur ce compte.")
            self.db.execute("UPDATE accounts SET status='error',fault=?,retry_at=0,updated_at=? WHERE id=?",
                            (code, self.clock(), account_id))
            self._event(account_id, "error", "Incident fictif : " + KNOWN[code])
        return {"ok": True}

    def scan(self, full=False):
        """Repair only known simulation faults. Other accounts remain unaffected."""
        now = self.clock()
        result = {"repaired": 0, "escalated": 0}
        with self.lock, self.db:
            cfg = self._settings()
            if full:
                self.db.execute("UPDATE config SET value=? WHERE key='last_full_scan'",
                                (json.dumps(now),))
                self._event(None, "info", "Contrôle de santé terminé.")
            accounts = list(self.db.execute(
                "SELECT * FROM accounts WHERE enabled=1 AND fault IS NOT NULL ORDER BY id"
            ))
            for account in accounts:
                aid, code = account["id"], account["fault"]
                if cfg["pilot_mode"] and not account["pilot"]:
                    continue
                if account["retry_at"] > now:
                    continue
                incident = self.db.execute(
                    "SELECT * FROM incidents WHERE account_id=? AND status!='resolved' "
                    "ORDER BY id DESC LIMIT 1", (aid,)
                ).fetchone()
                if incident is None:
                    self.db.execute(
                        "INSERT INTO incidents(account_id,code,status,details,opened_at) "
                        "VALUES (?,?,?,?,?)",
                        (aid, code, "open", KNOWN[code], now)
                    )
                    incident = self.db.execute(
                        "SELECT * FROM incidents WHERE id=last_insert_rowid()"
                    ).fetchone()
                if incident["status"] == "escalated":
                    continue
                tries = incident["attempts"] + 1
                if not cfg["auto_repair"] or code not in AUTO or tries > cfg["max_attempts"]:
                    self.db.execute(
                        "UPDATE incidents SET status='escalated',details=? WHERE id=?",
                        ("Correction non autorisée ou limite atteinte.", incident["id"])
                    )
                    self._event(aid, "error", "Compte isolé : intervention nécessaire.")
                    result["escalated"] += 1
                    continue
                self.db.execute("UPDATE incidents SET status='recovering',attempts=? WHERE id=?",
                                (tries, incident["id"]))
                if code == "network_timeout" and tries == 1:
                    self.db.execute("UPDATE accounts SET retry_at=? WHERE id=?",
                                    (now + cfg["retry_delay_seconds"], aid))
                    self._event(aid, "warning", "Connexion pilote : nouvelle tentative prévue.")
                else:
                    self.db.execute(
                        "UPDATE accounts SET status='ok',fault=NULL,retry_at=0,updated_at=? WHERE id=?",
                        (now, aid)
                    )
                    self.db.execute(
                        "UPDATE incidents SET status='resolved',closed_at=?,details=? WHERE id=?",
                        (now, "Récupération simulée réussie.", incident["id"])
                    )
                    self._event(aid, "ok", "Profil récupéré (simulation).")
                    result["repaired"] += 1
        return result

    def resolve(self, incident_id):
        with self.lock, self.db:
            row = self.db.execute("SELECT * FROM incidents WHERE id=?",
                                  (incident_id,)).fetchone()
            if not row:
                raise ValueError("Incident introuvable.")
            if row["status"] == "resolved":
                return
            self.db.execute(
                "UPDATE accounts SET status='ok',fault=NULL,retry_at=0 WHERE id=?",
                (row["account_id"],)
            )
            self.db.execute(
                "UPDATE incidents SET status='resolved',closed_at=?,details=? WHERE id=?",
                (self.clock(), "Clôturé manuellement en simulation.", incident_id)
            )
            self._event(row["account_id"], "info", "Incident clôturé.")

    def diagnostics(self):
        # No secrets exist in profile records or incident payloads.
        return {"overview": self.overview(), "settings": self.settings(),
                "accounts": self.accounts(), "incidents": self.incidents(200),
                "events": self.events(200)}
