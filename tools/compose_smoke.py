"""CI smoke test for four real Docker services, no game connections."""
from __future__ import annotations

import base64
import json
import os
import sys
import time
from urllib.error import HTTPError, URLError
from urllib.request import Request, urlopen

URL = "http://127.0.0.1:8765"


def get(path, password=None):
    headers = {}
    if password is not None:
        encoded = base64.b64encode(("admin:" + password).encode("utf-8")).decode("ascii")
        headers["Authorization"] = "Basic " + encoded
    with urlopen(Request(URL + path, headers=headers), timeout=3) as response:
        return json.loads(response.read().decode("utf-8"))


def main():
    password = os.environ.get("GUARDIAN_PASSWORD", "")
    if len(password) < 16:
        print("Missing test password", file=sys.stderr)
        return 2
    deadline = time.monotonic() + 90
    last_problem = "Service en attente"
    while time.monotonic() < deadline:
        try:
            ready = get("/health/ready")
            if ready.get("status") != "ready":
                raise ValueError("Guardian not ready")
            data = get("/api/telemetry/state", password)
            agents = {a["agent_id"]: a for a in data["agents"]}
            if set(agents) >= {"pilot_01", "pilot_02", "pilot_03"} and all(
                agents[x]["connection"] == "ok" for x in ("pilot_01", "pilot_02", "pilot_03")
            ):
                seen = {e["profile_id"] for e in data["events"] if e.get("code") == "probe_success"}
                if {1, 2, 3} <= seen:
                    print("OK: Guardian healthy, 3 independent workers connected, 3 recoveries reported.")
                    return 0
                last_problem = "Workers connected; expected initial recovery events"
            else:
                last_problem = "Still waiting for three worker heartbeats"
        except (HTTPError, URLError, ValueError, TimeoutError) as exc:
            last_problem = type(exc).__name__
        time.sleep(2)
    print("ERROR: Docker smoke test timeout:", last_problem, file=sys.stderr)
    return 1


if __name__ == "__main__":
    raise SystemExit(main())
