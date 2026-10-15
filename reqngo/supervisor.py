"""Independent periodic checks for local simulations and remote technical pilots.

A failure in either check never blocks the other. All actions are limited to
existing deterministic simulated recovery and monitoring: no remote execution.
"""
from __future__ import annotations

import time


class Supervisor:
    def __init__(self, engine, telemetry, *, wall_clock=None, monotonic_clock=None,
                 telemetry_interval=30.0, on_error=None):
        if not 5 <= telemetry_interval <= 3600:
            raise ValueError("Intervalle de supervision invalide")
        self.engine = engine
        self.telemetry = telemetry
        self.wall_clock = wall_clock or time.time
        self.monotonic_clock = monotonic_clock or time.monotonic
        self.telemetry_interval = telemetry_interval
        self.last_telemetry_check = None
        self.on_error = on_error or (lambda component, error: None)

    def tick(self):
        """Process a single scheduler iteration; safe to call every two seconds."""
        status = {"full_scan": False, "remote_check": False, "alerts_created": 0,
                  "failed_components": []}
        try:
            cfg = self.engine.settings()
            elapsed = self.wall_clock() - cfg["last_full_scan"]
            due = elapsed >= cfg["scan_interval_seconds"] or elapsed < 0
            self.engine.scan(full=due)
            status["full_scan"] = due
        except Exception as error:
            # No secrets, arbitrary error strings or tracebacks are returned.
            status["failed_components"].append("local")
            self.on_error("local", type(error).__name__)

        current = self.monotonic_clock()
        if (self.last_telemetry_check is None or
                current < self.last_telemetry_check or
                current - self.last_telemetry_check >= self.telemetry_interval):
            # Rate-limit repeated failures and avoid flooding the event log.
            self.last_telemetry_check = current
            status["remote_check"] = True
            try:
                result = self.telemetry.check_stale()
                status["alerts_created"] = result["alerts_created"]
            except Exception as error:
                status["failed_components"].append("remote")
                self.on_error("remote", type(error).__name__)
        return status
