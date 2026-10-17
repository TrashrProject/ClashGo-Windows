"""Standalone, authorized multi-slot technical diagnostic worker.

No credentials for game accounts, device control, UI taps, or arbitrary scripts.
A worker only performs a short test in its OWN work directory and reports it.
"""
from __future__ import annotations

import argparse
import json
import os
import tempfile
import time
from concurrent.futures import ThreadPoolExecutor
from pathlib import Path
from urllib.request import Request, urlopen

from reqngo.fleet import _WORKER
from reqngo.health import update_worker_heartbeat
from reqngo.pilot_worker import validate_endpoint


class FleetClient:
    def __init__(self, url: str, worker_id: str, token: str):
        if not _WORKER.fullmatch(worker_id):
            raise ValueError("Identifiant de worker invalide.")
        if not isinstance(token, str) or len(token) < 32:
            raise ValueError("Jeton de worker manquant ou trop court.")
        self.url = url
        self.worker_id = worker_id
        self.token = token

    def send(self, endpoint: str, body: dict):
        if endpoint not in ("/api/fleet/worker/heartbeat",
                            "/api/fleet/worker/claim",
                            "/api/fleet/worker/renew",
                            "/api/fleet/worker/complete"):
            raise ValueError("Route worker non autorisee.")
        request = Request(
            self.url + endpoint, data=json.dumps(body).encode("utf-8"),
            method="POST", headers={
                "Authorization": "Bearer " + self.token,
                "X-Fleet-Worker-Id": self.worker_id,
                "Content-Type": "application/json",
            })
        with urlopen(request, timeout=10) as response:
            return json.loads(response.read(32_768).decode("utf-8"))


class FleetDiagnosticWorker:
    def __init__(self, send, workdir, capacity: int = 1):
        if type(capacity) is not int or not 1 <= capacity <= 8:
            raise ValueError("Capacite de worker invalide.")
        self.send = send
        self.capacity = capacity
        self.workdir = Path(workdir)

    def _probe(self):
        """A real IO check within the dedicated worker directory, no game."""
        self.workdir.mkdir(parents=True, exist_ok=True)
        with tempfile.NamedTemporaryFile(mode="w+b", prefix=".fleet_probe_",
                                         dir=self.workdir, delete=True) as check:
            check.write(b"probe_ok")
            check.flush()
            check.seek(0)
            if check.read() != b"probe_ok":
                raise OSError("Sonde stockage invalide.")

    def _process(self, job):
        # Fail closed on any unexpected job type: never dispatch commands.
        healthy = False
        if job.get("kind") == "health_probe":
            try:
                self._probe()
                healthy = True
            except OSError:
                healthy = False
        # If acknowledgement is lost, the server lease expires. Health probes
        # are specifically designed to be safe to repeat.
        self.send("/api/fleet/worker/complete", {
            "job_id": job["job_id"], "lease_token": job["lease_token"],
            "success": healthy
        })
        return healthy

    def step(self):
        self.send("/api/fleet/worker/heartbeat",
                  {"status": "ok", "capacity": self.capacity})
        answer = self.send("/api/fleet/worker/claim",
                           {"limit": self.capacity, "capacity": self.capacity,
                            "lease_seconds": 90})
        jobs = answer["items"]
        if not isinstance(jobs, list) or len(jobs) > self.capacity:
            raise ValueError("Lot de taches distant invalide.")
        if not jobs:
            return {"received": 0, "completed": 0, "failed": 0}
        # These are IO-only diagnostic probes. Do not reuse this parallel runner
        # for account/game actions without state reconciliation and fencing.
        with ThreadPoolExecutor(max_workers=self.capacity) as executor:
            results = list(executor.map(self._process, jobs))
        return {"received": len(jobs), "completed": sum(results),
                "failed": len(jobs) - sum(results)}


def main(argv=None):
    parser = argparse.ArgumentParser(description="REQ N GO V10 test worker")
    parser.add_argument("--once", action="store_true")
    args = parser.parse_args(argv)
    worker_id = os.environ.get("FLEET_WORKER_ID", "")
    token = os.environ.get("FLEET_WORKER_TOKEN", "")
    try:
        url = validate_endpoint(
            os.environ.get("GUARDIAN_URL", "http://127.0.0.1:8765"),
            os.environ.get("GUARDIAN_PRIVATE_DOCKER") == "1",
            os.environ.get("GUARDIAN_PRIVATE_RAILWAY") == "1")
        capacity = int(os.environ.get("FLEET_WORKER_CAPACITY", "1"))
        interval = int(os.environ.get("FLEET_POLL_SECONDS", "10"))
        if not 2 <= interval <= 300:
            raise ValueError("Intervalle de polling invalide.")
        client = FleetClient(url, worker_id, token)
        worker = FleetDiagnosticWorker(
            client.send,
            os.environ.get("FLEET_WORKDIR", "./data/fleet-" + worker_id),
            capacity=capacity)
    except (ValueError, TypeError) as error:
        parser.error(str(error))
    heartbeat_file = worker.workdir / "worker_heartbeat.txt"
    while True:
        connected = True
        try:
            outcome = worker.step()
            print("worker", worker_id, "recues", outcome["received"],
                  "terminees", outcome["completed"], "echecs", outcome["failed"],
                  flush=True)
        except (OSError, ValueError, KeyError, TypeError) as error:
            connected = False
            # Never print tokens, credentials, request contents, or remote text.
            print("worker", worker_id, "indisponible", type(error).__name__, flush=True)
        try:
            update_worker_heartbeat(heartbeat_file)
        except OSError:
            pass
        if args.once:
            return 0 if connected else 1
        time.sleep(interval)


if __name__ == "__main__":
    raise SystemExit(main())
