"""Independent diagnostic pilot process with bounded local recovery and durable outbox.

No Clash of Clans access, emulator control, account credentials or remote commands.
"""
from __future__ import annotations

import argparse
import json
import os
import sqlite3
import tempfile
import time
import uuid
from pathlib import Path
from urllib.parse import urlparse
from urllib.request import Request, urlopen

from reqngo.health import update_worker_heartbeat

from reqngo.health import update_worker_heartbeat


def validate_endpoint(url: str, private_docker: bool = False) -> str:
    parsed = urlparse(url)
    local = parsed.scheme == "http" and parsed.hostname in ("127.0.0.1", "localhost")
    docker = (private_docker and parsed.scheme == "http" and parsed.hostname == "guardian"
              and parsed.port == 8765)
    secure = parsed.scheme == "https" and bool(parsed.hostname)
    if not (local or docker or secure) or parsed.username or parsed.password or parsed.path not in ("", "/"):
        raise ValueError("Adresse Guardian invalide : utiliser HTTPS, localhost ou reseau Docker prive.")
    return url.rstrip("/")


def send(url, token, endpoint, data):
    request = Request(
        url + endpoint, data=json.dumps(data).encode("utf-8"), method="POST",
        headers={"Authorization": "Bearer " + token,
                 "Content-Type": "application/json"},
    )
    with urlopen(request, timeout=8) as response:
        return json.loads(response.read().decode("utf-8"))


class Outbox:
    """At-least-once delivery; server event keys make report retries idempotent."""
    def __init__(self, path):
        path = Path(path)
        path.parent.mkdir(parents=True, exist_ok=True)
        self.db = sqlite3.connect(str(path))
        self.db.execute("CREATE TABLE IF NOT EXISTS pending ("
                        "id INTEGER PRIMARY KEY AUTOINCREMENT, payload TEXT NOT NULL)")
        self.db.commit()

    def append(self, event):
        if self.count() >= 1000:
            raise RuntimeError("Outbox pleine : intervention necessaire.")
        with self.db:
            self.db.execute("INSERT INTO pending(payload) VALUES (?)",
                            (json.dumps(event, sort_keys=True),))

    def count(self):
        return self.db.execute("SELECT COUNT(*) FROM pending").fetchone()[0]

    def flush(self, send_report):
        delivered = 0
        for row_id, payload in self.db.execute(
                "SELECT id,payload FROM pending ORDER BY id LIMIT 100").fetchall():
            send_report(json.loads(payload))
            with self.db:
                self.db.execute("DELETE FROM pending WHERE id=?", (row_id,))
            delivered += 1
        return delivered

    def close(self):
        self.db.close()


class Pilot:
    def __init__(self, agent_id, profile_id, workdir, outbox, heartbeat, report):
        if not agent_id.startswith("pilot_") or not agent_id[6:].isdigit():
            raise ValueError("Identifiant pilote invalide.")
        if type(profile_id) is not int or not 1 <= profile_id <= 10000:
            raise ValueError("Profil pilote invalide.")
        self.agent_id = agent_id
        self.profile_id = profile_id
        self.workdir = Path(workdir)
        self.outbox = outbox
        self.heartbeat = heartbeat
        self.report = report
        self.last_issue = None

    def enqueue(self, code, level, **metadata):
        self.outbox.append({
            "agent_id": self.agent_id, "event_key": uuid.uuid4().hex,
            "code": code, "level": level, "profile_id": self.profile_id,
            "metadata": {"component": "pilot_probe", **metadata},
        })

    def _probe(self):
        """Check real local process storage, without reading or deleting user files."""
        with tempfile.NamedTemporaryFile(mode="w+b", dir=self.workdir,
                                         prefix=".health_", delete=True) as tmp:
            tmp.write(b"healthy")
            tmp.flush()
            tmp.seek(0)
            if tmp.read() != b"healthy":
                raise OSError("Test de lecture/ecriture echoue.")

    def check(self):
        if not self.workdir.is_dir():
            # Only recreate the worker's own dedicated directory.
            self.enqueue("invalid_config", "warning", phase="missing_workdir")
            try:
                self.workdir.mkdir(parents=True, exist_ok=True)
                self._probe()
                self.enqueue("probe_success", "info", error_type="invalid_config")
                self.last_issue = None
                return "ok"
            except OSError:
                self.last_issue = "invalid_config"
                return "degraded"
        try:
            self._probe()
        except OSError:
            if self.last_issue != "pilot_probe_failed":
                self.enqueue("pilot_probe_failed", "error", phase="storage")
            self.last_issue = "pilot_probe_failed"
            return "degraded"
        if self.last_issue:
            self.enqueue("probe_success", "info", error_type=self.last_issue)
        self.last_issue = None
        return "ok"

    def step(self):
        status = self.check()
        update_worker_heartbeat(self.workdir.parent / "worker_heartbeat.txt")
        # Reports are durable even when the central service is temporarily unreachable.
        try:
            self.outbox.flush(self.report)
            self.heartbeat({"agent_id": self.agent_id, "status": status,
                            "version": "pilot_v4"})
            return {"status": status, "queued": self.outbox.count(), "connected": True}
        except (OSError, TimeoutError, ValueError) as exc:
            # Only structured diagnostics go to the console, never credentials or payloads.
            print("Connexion Guardian indisponible :", type(exc).__name__, flush=True)
            return {"status": status, "queued": self.outbox.count(), "connected": False}


def main(argv=None):
    parser = argparse.ArgumentParser(description="REQ N GO independent test pilot")
    parser.add_argument("--id", required=True)
    parser.add_argument("--profile", type=int, required=True)
    parser.add_argument("--once", action="store_true")
    args = parser.parse_args(argv)
    token = os.environ.get("GUARDIAN_AGENT_TOKEN", "")
    if len(token) < 32:
        parser.error("GUARDIAN_AGENT_TOKEN requis (32 caracteres minimum)")
    url = validate_endpoint(os.environ.get("GUARDIAN_URL", "http://127.0.0.1:8765"),
                            os.environ.get("GUARDIAN_PRIVATE_DOCKER") == "1")
    base = Path(os.environ.get("PILOT_DATA_DIR", "./data/" + args.id))
    base.mkdir(parents=True, exist_ok=True)
    outbox = Outbox(base / "outbox.sqlite3")
    worker = Pilot(
        args.id, args.profile, base / "work", outbox,
        heartbeat=lambda body: send(url, token, "/api/telemetry/heartbeat", body),
        report=lambda body: send(url, token, "/api/telemetry/report", body)
    )
    try:
        while True:
            state = worker.step()
            # Shows that this worker's own event loop remains live, independently of Guardian.
            update_worker_heartbeat(base / "worker_heartbeat.txt")
            print(args.id, "etat", state["status"], "connecte", state["connected"],
                  "evenements en attente", state["queued"], flush=True)
            if args.once:
                return 0 if state["connected"] else 1
            time.sleep(30)
    finally:
        outbox.close()


if __name__ == "__main__":
    raise SystemExit(main())
