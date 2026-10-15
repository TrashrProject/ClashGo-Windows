"""CI smoke test for four real Docker services, no game connections.

Produces a strictly allowlisted report so a remote maintainer can inspect CI
without receiving credentials, account data, raw server logs or screenshots.
"""
from __future__ import annotations

import base64
import json
import os
import sys
import time
from pathlib import Path
from urllib.error import HTTPError, URLError
from urllib.request import Request, urlopen

URL = "http://127.0.0.1:8765"
PILOTS = ("pilot_01", "pilot_02", "pilot_03")
REPORT_PATH = Path("artifacts/guardian-smoke.json")
_REASONS = {"ready_timeout", "waiting_heartbeats", "waiting_probes",
            "open_incidents", "missing_password"}


def save_report(outcome, *, state=None, reason=None):
    """Whitelist fields only; never persist a raw HTTP response or error string."""
    if outcome not in ("success", "failed"):
        raise ValueError("Invalid report status")
    if reason is not None and reason not in _REASONS:
        reason = "ready_timeout"
    state = state if isinstance(state, dict) else {}
    agents = {item.get("agent_id"): item for item in state.get("agents", [])
              if isinstance(item, dict) and item.get("agent_id") in PILOTS}
    observed = sorted({item["profile_id"] for item in state.get("events", [])
                       if isinstance(item, dict) and item.get("code") == "probe_success"
                       and type(item.get("profile_id")) is int
                       and item["profile_id"] in (1, 2, 3)})
    stats = state.get("summary", {})
    report = {
        "schema": "guardian-smoke-v1",
        "result": outcome,
        "reason": reason,
        "pilots": [{"id": aid,
                    "status": agents.get(aid, {}).get("connection")
                    if agents.get(aid, {}).get("connection") in ("ok", "busy", "degraded", "stale")
                    else "missing"} for aid in PILOTS],
        "verified_probe_profiles": observed,
        "open_incidents": stats.get("open_cases") if type(stats.get("open_cases")) is int else None,
        "contains_credentials": False,
        "game_connected": False,
    }
    REPORT_PATH.parent.mkdir(parents=True, exist_ok=True)
    REPORT_PATH.write_text(json.dumps(report, indent=2, ensure_ascii=False), encoding="utf-8")
    return report


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
        save_report("failed", reason="missing_password")
        print("Missing test password", file=sys.stderr)
        return 2
    deadline = time.monotonic() + 90
    reason = "ready_timeout"
    last_data = None
    while time.monotonic() < deadline:
        try:
            ready = get("/health/ready")
            if ready.get("status") != "ready":
                raise ValueError("Guardian not ready")
            data = get("/api/telemetry/state", password)
            last_data = data
            agents = {a["agent_id"]: a for a in data["agents"]}
            connected = all(agents.get(aid, {}).get("connection") == "ok" for aid in PILOTS)
            if connected:
                observed = {e["profile_id"] for e in data["events"]
                            if e.get("code") == "probe_success"}
                if {1, 2, 3} <= observed:
                    if data.get("summary", {}).get("open_cases") == 0:
                        save_report("success", state=data)
                        print("OK: Guardian healthy, 3 pilots connected, 3 recoveries, no open incidents.")
                        return 0
                    reason = "open_incidents"
                else:
                    reason = "waiting_probes"
            else:
                reason = "waiting_heartbeats"
        except (HTTPError, URLError, ValueError, TimeoutError, OSError):
            reason = "ready_timeout"
        time.sleep(2)
    save_report("failed", state=last_data, reason=reason)
    print("ERROR: Docker smoke test did not meet readiness criteria:", reason, file=sys.stderr)
    return 1


if __name__ == "__main__":
    raise SystemExit(main())
