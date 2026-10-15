"""Minimal cloud health probes: no secrets or remote code execution."""
from __future__ import annotations

import argparse
import os
import time
from pathlib import Path
from urllib.error import HTTPError, URLError
from urllib.request import urlopen


def worker_healthy(path, now=None, max_age=95):
    now = time.time() if now is None else now
    try:
        # Heartbeat is just a timestamp written by this process after each tick.
        stamp = float(Path(path).read_text(encoding="ascii").strip())
    except (OSError, ValueError, OverflowError):
        return False
    return 0 <= now - stamp <= max_age


def update_worker_heartbeat(path, now=None):
    now = time.time() if now is None else now
    dest = Path(path)
    dest.parent.mkdir(parents=True, exist_ok=True)
    temp = dest.with_suffix(".pending")
    temp.write_text(str(now), encoding="ascii")
    os.replace(temp, dest)


def guardian_ready(engine, telemetry):
    """Test both SQLite handles. No sensitive diagnostic data is returned."""
    try:
        with engine.lock:
            ok1 = engine.db.execute("SELECT 1").fetchone()[0] == 1
        with telemetry.lock:
            ok2 = telemetry.db.execute("SELECT 1").fetchone()[0] == 1
        return ok1 and ok2
    except Exception:
        return False


def main(argv=None):
    parser = argparse.ArgumentParser()
    parser.add_argument("mode", choices=("worker", "guardian"))
    parser.add_argument("--stamp", default="/data/worker_heartbeat.txt")
    parser.add_argument("--url", default="http://127.0.0.1:8765/health/ready")
    args = parser.parse_args(argv)
    if args.mode == "worker":
        return 0 if worker_healthy(args.stamp) else 1
    # Fixed loopback-only health endpoint: do not forward tokens or secrets.
    if args.url != "http://127.0.0.1:8765/health/ready":
        parser.error("Seule l'adresse localhost est acceptee pour la sonde Docker.")
    try:
        with urlopen(args.url, timeout=3) as response:
            return 0 if response.status == 200 else 1
    except (HTTPError, URLError, TimeoutError, OSError):
        return 1


if __name__ == "__main__":
    raise SystemExit(main())
