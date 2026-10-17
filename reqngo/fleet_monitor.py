"""Independent fleet health checks; never invokes a game, device or worker command.

The monitor runs a bounded database reconciliation. Heartbeats close incidents;
a worker that disappears causes a single durable incident per outage.
"""
from __future__ import annotations

import time


class FleetHealthSupervisor:
    def __init__(self, fleet, *, interval=30, monotonic_clock=None, on_error=None):
        if type(interval) not in (int, float) or not 5 <= interval <= 3600:
            raise ValueError("Intervalle de supervision invalide.")
        self.fleet = fleet
        self.interval = interval
        self.clock = monotonic_clock or time.monotonic
        self.on_error = on_error or (lambda component, kind: None)
        self.last_scan = None

    def tick(self):
        now = self.clock()
        due = (self.last_scan is None or now < self.last_scan or
               now - self.last_scan >= self.interval)
        if not due:
            return {"checked": False, "new_incidents": 0, "failed": False}
        self.last_scan = now
        try:
            result = self.fleet.scan_worker_health()
            return {"checked": True, "new_incidents": result["new_incidents"],
                    "open_incidents": result["open_incidents"], "failed": False}
        except Exception as exc:
            # No SQL strings, credentials or connection URLs in diagnostics.
            self.on_error("fleet", type(exc).__name__)
            return {"checked": True, "new_incidents": 0, "failed": True}
