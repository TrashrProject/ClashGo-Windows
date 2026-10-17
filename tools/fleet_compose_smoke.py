"""End-to-end local CI smoke for the opt-in PostgreSQL fleet demo.

Runs only against a disposable localhost Compose deployment. It never talks to
Clash of Clans, reads private accounts or uses any game credentials.
"""
from __future__ import annotations

import base64
import json
import os
import time
from urllib.error import HTTPError, URLError
from urllib.request import Request, urlopen

BASE = "http://127.0.0.1:8765"
PASSWORD = os.environ["GUARDIAN_PASSWORD"]


def call(path, method="GET", body=None):
    credentials = base64.b64encode(("admin:" + PASSWORD).encode()).decode()
    headers = {"Authorization": "Basic " + credentials}
    if method == "POST":
        headers["X-Guardian-Action"] = "1"
        headers["Content-Type"] = "application/json"
    req = Request(BASE + path, method=method, headers=headers,
                  data=json.dumps(body).encode("utf-8") if body is not None else None)
    with urlopen(req, timeout=5) as resp:
        if resp.status != 200:
            raise RuntimeError("Unexpected response status.")
        return json.loads(resp.read().decode("utf-8"))


def wait_ready(seconds=100):
    end = time.monotonic() + seconds
    while time.monotonic() < end:
        try:
            with urlopen(BASE + "/health/ready", timeout=3) as response:
                if response.status == 200:
                    return
        except (HTTPError, URLError, TimeoutError, OSError):
            pass
        time.sleep(2)
    raise AssertionError("Guardian did not become ready before timeout.")


def main():
    wait_ready()
    initial = call("/api/fleet/summary")
    if initial["backend"] != "distributed-postgres":
        raise AssertionError("Not using PostgreSQL backend.")
    created = call("/api/fleet/profiles/batch", "POST", {
        "prefix": "ci", "start": 1, "count": 5000
    })
    if created["created"] != 5000:
        raise AssertionError("Expected 5000 fresh logical test profiles.")
    for number in range(1, 17):
        answer = call("/api/fleet/probes", "POST", {
            "profile_key": f"ci-{number:06d}",
            "dedupe_key": f"compose:probe:{number:06d}",
        })
        if answer["duplicate"]:
            raise AssertionError("Unexpected duplicate in test database.")
    end = time.monotonic() + 100
    while time.monotonic() < end:
        summary = call("/api/fleet/summary")
        if (summary["jobs"]["done"] == 16 and
                summary["online_workers"] >= 2 and
                summary["jobs"]["needs_review"] == 0):
            agents = call("/api/fleet/workers")["items"]
            if {w["worker_id"] for w in agents if w["connected"]} < {
                    "fleet_01", "fleet_02"}:
                raise AssertionError("The two independent workers are not connected.")
            # sanitized report only: no secrets, URLs or identities of players.
            print(json.dumps({
                "backend": summary["backend"],
                "logical_profiles": summary["profiles"],
                "connected_technical_workers": summary["online_workers"],
                "diagnostics_done": summary["jobs"]["done"],
                "diagnostics_pending": summary["jobs"]["queued"],
                "diagnostics_for_review": summary["jobs"]["needs_review"],
            }, sort_keys=True))
            return 0
        time.sleep(2)
    raise AssertionError("The 16 technical probes were not completed in time.")


if __name__ == "__main__":
    raise SystemExit(main())
