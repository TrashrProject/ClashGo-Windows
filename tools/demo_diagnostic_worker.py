"""Demo diagnostic worker: exercises secure telemetry without touching any game."""
from __future__ import annotations

import json
import os
import sys
import time
import uuid
from urllib.parse import urlparse
from urllib.request import Request, urlopen


def post(url, token, endpoint, data):
    payload = json.dumps(data).encode("utf-8")
    request = Request(
        url.rstrip("/") + endpoint, data=payload, method="POST",
        headers={"Authorization": "Bearer " + token, "Content-Type": "application/json"},
    )
    with urlopen(request, timeout=10) as response:
        return json.loads(response.read().decode("utf-8"))


def main():
    url = os.environ.get("GUARDIAN_URL", "http://127.0.0.1:8765")
    token = os.environ.get("GUARDIAN_AGENT_TOKEN", "")
    agent = os.environ.get("GUARDIAN_AGENT_ID", "pilot_01")
    parsed = urlparse(url)
    if parsed.scheme != "https" and not (parsed.scheme == "http" and parsed.hostname in ("127.0.0.1", "localhost")):
        sys.exit("Utilisez HTTPS pour un serveur distant ou HTTP sur localhost.")
    if len(token) < 32:
        sys.exit("GUARDIAN_AGENT_TOKEN manquant (32 caracteres minimum).")
    once = "--once" in sys.argv
    demo_error = "--demo-error" in sys.argv
    while True:
        try:
            post(url, token, "/api/telemetry/heartbeat",
                 {"agent_id": agent, "status": "ok", "version": "demo_1"})
            if demo_error:
                post(url, token, "/api/telemetry/report",
                     {"agent_id": agent, "event_key": uuid.uuid4().hex,
                      "code": "demo_timeout", "level": "warning", "profile_id": 1,
                      "metadata": {"component": "mock_session", "retry_count": 1}})
                demo_error = False
            print("Diagnostic transmis :", agent, flush=True)
        except Exception as exc:
            print("Transmission echouee :", type(exc).__name__, flush=True)
            if once:
                raise
        if once:
            break
        time.sleep(30)


if __name__ == "__main__":
    main()
