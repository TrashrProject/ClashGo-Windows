"""Select a shared PostgreSQL scheduler only when explicitly configured.

The existing V9 SQLite store remains unchanged by default.
"""
from __future__ import annotations

import os

from reqngo.fleet import FleetStore


def make_fleet(path):
    dsn = os.getenv("FLEET_DATABASE_URL", "")
    if not dsn:
        return FleetStore(path)
    from reqngo.fleet_postgres import PostgresFleetStore
    return PostgresFleetStore(dsn)
