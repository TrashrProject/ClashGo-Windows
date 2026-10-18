"""Group-oriented management of logical fleet profiles for REQ N GO.

Keeps the V9–V14 queue untouched. Uses the existing SQLite connection or
PostgreSQL pool. Bulk changes affect logical enable flags only: never Android
devices, game accounts, jobs in progress or external services.
"""
from __future__ import annotations

import re
import sqlite3
import time
from contextlib import contextmanager

from reqngo.fleet import _KEY

_GROUP = re.compile(r"[a-z][a-z0-9_-]{1,39}\Z")
_MAX_GROUPS = 300
_MAX_BULK = 200


class FleetConsole:
    def __init__(self, fleet):
        self.fleet = fleet
        self.pg = hasattr(fleet, "pool")
        self.p = "%s" if self.pg else "?"
        with self._connection(write=True) as db:
            if self.pg:
                db.execute("""
                    CREATE TABLE IF NOT EXISTS fleet_console_groups (
                        group_key text PRIMARY KEY,
                        label text NOT NULL,
                        description text NOT NULL DEFAULT '',
                        created_at double precision NOT NULL
                    )""")
                db.execute("ALTER TABLE fleet_profiles "
                           "ADD COLUMN IF NOT EXISTS group_key text")
                db.execute("""
                    CREATE TABLE IF NOT EXISTS fleet_console_audit (
                        id bigserial PRIMARY KEY, group_key text NOT NULL,
                        action text NOT NULL, affected integer NOT NULL,
                        recorded_at double precision NOT NULL
                    )""")
            else:
                db.execute("""
                    CREATE TABLE IF NOT EXISTS fleet_console_groups (
                        group_key TEXT PRIMARY KEY,
                        label TEXT NOT NULL,
                        description TEXT NOT NULL DEFAULT '',
                        created_at REAL NOT NULL
                    )""")
                columns = {r["name"] for r in db.execute(
                    "PRAGMA table_info(fleet_profiles)")}
                if "group_key" not in columns:
                    db.execute("ALTER TABLE fleet_profiles ADD COLUMN group_key TEXT")
                db.execute("""
                    CREATE TABLE IF NOT EXISTS fleet_console_audit (
                        id INTEGER PRIMARY KEY AUTOINCREMENT,
                        group_key TEXT NOT NULL, action TEXT NOT NULL,
                        affected INTEGER NOT NULL, recorded_at REAL NOT NULL
                    )""")
            db.execute("CREATE INDEX IF NOT EXISTS fleet_console_group_profiles "
                       "ON fleet_profiles(group_key,profile_key)")
            db.execute("CREATE INDEX IF NOT EXISTS fleet_console_group_enabled "
                       "ON fleet_profiles(group_key,enabled,profile_key)")

    @contextmanager
    def _connection(self, *, write=False):
        if self.pg:
            with self.fleet.pool.connection() as db:
                yield db
        elif write:
            with self.fleet._transaction():
                yield self.fleet.db
        else:
            with self.fleet.lock:
                yield self.fleet.db

    def _first(self, row):
        return next(iter(row.values())) if self.pg else row[0]

    def _lock_group(self, db, key):
        if self.pg:
            db.execute("SELECT pg_advisory_xact_lock(hashtext(%s)::bigint)", (key,))

    @staticmethod
    def _valid_key(key):
        return isinstance(key, str) and bool(_GROUP.fullmatch(key))

    def _exists(self, db, key):
        row = db.execute("SELECT group_key FROM fleet_console_groups "
                         "WHERE group_key=" + self.p, (key,)).fetchone()
        return row is not None

    def create_group(self, key, label, description=""):
        if not self._valid_key(key):
            raise ValueError("Identifiant de groupe invalide.")
        if (not isinstance(label, str) or not 2 <= len(label.strip()) <= 60
                or not isinstance(description, str) or len(description) > 240):
            raise ValueError("Nom ou description de groupe invalide.")
        with self._connection(write=True) as db:
            if self._first(db.execute("SELECT COUNT(*) FROM fleet_console_groups").fetchone()) >= _MAX_GROUPS:
                raise ValueError("Limite de 300 groupes atteinte.")
            if self._exists(db, key):
                raise ValueError("Ce groupe existe deja.")
            db.execute("INSERT INTO fleet_console_groups "
                       "(group_key,label,description,created_at) VALUES "
                       f"({self.p},{self.p},{self.p},{self.p})",
                       (key, label.strip(), description.strip(), time.time()))
        return {"group_key": key, "label": label.strip()}

    def groups(self):
        with self._connection() as db:
            rows = db.execute("""
                SELECT g.group_key,g.label,g.description,g.created_at,
                    COUNT(p.profile_key) AS profiles,
                    COALESCE(SUM(CASE WHEN p.enabled THEN 1 ELSE 0 END),0) AS enabled
                FROM fleet_console_groups g
                LEFT JOIN fleet_profiles p ON p.group_key=g.group_key
                GROUP BY g.group_key,g.label,g.description,g.created_at
                ORDER BY g.label,g.group_key LIMIT 301
            """).fetchall()
        return [dict(r) if self.pg else dict(r) for r in rows]

    def profiles(self, q="", group="all", enabled="all", offset=0, limit=50):
        if (not isinstance(q, str) or len(q) > 40
                or not isinstance(group, str)
                or group not in ("all", "ungrouped") and not self._valid_key(group)
                or enabled not in ("all", "yes", "no")
                or type(offset) is not int or not 0 <= offset <= 10_000_000
                or type(limit) is not int or not 1 <= limit <= 100):
            raise ValueError("Filtre ou pagination invalide.")
        predicates, args = [], []
        if q:
            # Key-prefix search: indexed and scalable, never arbitrary SQL.
            if not re.fullmatch(r"[A-Za-z0-9_-]{1,40}", q):
                raise ValueError("Recherche de profil invalide.")
            predicates.append("profile_key LIKE " + self.p)
            args.append(q + "%")
        if group == "ungrouped":
            predicates.append("group_key IS NULL")
        elif group != "all":
            predicates.append("group_key=" + self.p)
            args.append(group)
        if enabled != "all":
            predicates.append("enabled=" + self.p)
            args.append(enabled == "yes" if self.pg else int(enabled == "yes"))
        where = " WHERE " + " AND ".join(predicates) if predicates else ""
        with self._connection() as db:
            total = self._first(db.execute("SELECT COUNT(*) FROM fleet_profiles" + where,
                               tuple(args)).fetchone())
            rows = db.execute(
                ("SELECT profile_key,enabled,group_key,"
                 + ("EXTRACT(EPOCH FROM created_at)" if self.pg else "created_at")
                 + " AS created_at FROM fleet_profiles")
                + where + " ORDER BY profile_key LIMIT " + self.p + " OFFSET " + self.p,
                tuple(args + [limit, offset])).fetchall()
        return {"total": total, "offset": offset, "limit": limit,
                "items": [dict(r) for r in rows]}

    def assign(self, profile_keys, group_key):
        if (not isinstance(profile_keys, list) or not 1 <= len(profile_keys) <= _MAX_BULK
                or len(set(profile_keys)) != len(profile_keys)
                or any(not isinstance(k, str) or not _KEY.fullmatch(k)
                       for k in profile_keys)
                or group_key is not None and not self._valid_key(group_key)):
            raise ValueError("Selection de profils invalide.")
        # Serialize concurrent group-wide suspension and assignment using
        # an advisory lock for PostgreSQL. In SQLite BEGIN IMMEDIATE suffices.
        with self._connection(write=True) as db:
            if group_key is not None:
                self._lock_group(db, group_key)
                if not self._exists(db, group_key):
                    raise ValueError("Groupe inconnu.")
            # Explicit manual selections only; never a page-wide implicit all.
            placeholders = ",".join([self.p] * len(profile_keys))
            found = db.execute("SELECT profile_key FROM fleet_profiles "
                               "WHERE profile_key IN (" + placeholders + ")",
                               tuple(profile_keys)).fetchall()
            if len(found) != len(profile_keys):
                raise ValueError("Au moins un profil est introuvable.")
            # Lock previous groups too, in deterministic order, to serialize
            # bulk mutations with group reassignments.
            previous = db.execute("SELECT DISTINCT group_key FROM fleet_profiles "
                                  "WHERE profile_key IN (" + placeholders + ")",
                                  tuple(profile_keys)).fetchall()
            old_keys = sorted({(r["group_key"] if self.pg else r[0]) for r in previous
                               if (r["group_key"] if self.pg else r[0])
                               and (r["group_key"] if self.pg else r[0]) != group_key})
            for old in old_keys:
                self._lock_group(db, old)
            db.execute("UPDATE fleet_profiles SET group_key=" + self.p
                       + " WHERE profile_key IN (" + placeholders + ")",
                       tuple([group_key] + profile_keys))
            self._audit(db, group_key or "ungrouped", "assign", len(profile_keys))
        return {"updated": len(profile_keys), "group_key": group_key}

    def _audit(self, db, key, action, count):
        db.execute("INSERT INTO fleet_console_audit "
                   "(group_key,action,affected,recorded_at) VALUES "
                   f"({self.p},{self.p},{self.p},{self.p})",
                   (key, action, count, time.time()))

    def preview(self, group_key, enabled):
        if not self._valid_key(group_key) or type(enabled) is not bool:
            raise ValueError("Groupe ou action invalide.")
        with self._connection() as db:
            if not self._exists(db, group_key):
                raise ValueError("Groupe inconnu.")
            row = db.execute("SELECT COUNT(*) AS n FROM fleet_profiles "
                             "WHERE group_key=" + self.p + " AND enabled=" + self.p,
                             (group_key, not enabled if self.pg else int(not enabled))
                             ).fetchone()
        return {"group_key": group_key, "action": "activate" if enabled else "suspend",
                "would_change": self._first(row),
                "confirmation": ("ACTIVER " if enabled else "SUSPENDRE ") + group_key}

    def bulk_set_enabled(self, group_key, enabled, confirmation, expected_count):
        if (not self._valid_key(group_key) or type(enabled) is not bool
                or type(expected_count) is not int or not 0 <= expected_count <= 10_000_000
                or confirmation != ("ACTIVER " if enabled else "SUSPENDRE ") + group_key):
            raise ValueError("Confirmation de l'action groupee incorrecte.")
        with self._connection(write=True) as db:
            self._lock_group(db, group_key)
            if not self._exists(db, group_key):
                raise ValueError("Groupe inconnu.")
            old = not enabled if self.pg else int(not enabled)
            count = self._first(db.execute("SELECT COUNT(*) FROM fleet_profiles "
                               "WHERE group_key=" + self.p + " AND enabled=" + self.p,
                               (group_key, old)).fetchone())
            if count != expected_count:
                raise ValueError("Le groupe a change depuis la previsualisation.")
            if count:
                db.execute("UPDATE fleet_profiles SET enabled=" + self.p +
                           " WHERE group_key=" + self.p + " AND enabled=" + self.p,
                           (enabled if self.pg else int(enabled), group_key, old))
            self._audit(db, group_key, "activate" if enabled else "suspend", count)
        return {"group_key": group_key, "updated": count}

    def audit(self, limit=25):
        if type(limit) is not int or not 1 <= limit <= 100:
            raise ValueError("Limite invalide.")
        with self._connection() as db:
            rows = db.execute(
                "SELECT group_key,action,affected,recorded_at "
                "FROM fleet_console_audit ORDER BY id DESC LIMIT " + self.p,
                (limit,)).fetchall()
        return {"items": [dict(r) for r in rows]}
