"""Strict, anonymous worker resource measurements for the V14 fleet.

All fields are optional, nullable and bounded. Never send hostnames, PIDs,
device serials, screenshots, accounts or raw paths to the control plane.
"""
from __future__ import annotations

import math

_FIELDS = ("cpu_percent", "memory_percent", "process_rss_mb",
           "android_online_devices", "android_bound_devices")
_LIMITS = {"cpu_percent": 100.0, "memory_percent": 100.0,
           "process_rss_mb": 10_000_000.0, "android_online_devices": 10_000,
           "android_bound_devices": 10_000}


def clean_metrics(metrics=None):
    """None means unmeasured, *not* zero; rejects booleans, NaN and infinity."""
    metrics = {} if metrics is None else metrics
    if not isinstance(metrics, dict) or set(metrics) - set(_FIELDS):
        raise ValueError("Champs de telemetrie invalides.")
    result = {}
    for name in _FIELDS:
        value = metrics.get(name)
        if value is None:
            result[name] = None
            continue
        if name.endswith("_devices"):
            if type(value) is not int or not 0 <= value <= _LIMITS[name]:
                raise ValueError("Nombre d'appareils invalide.")
        else:
            if type(value) not in (int, float) or not math.isfinite(value) or not (
                    0 <= value <= _LIMITS[name]):
                raise ValueError("Mesure materielle invalide.")
            value = round(float(value), 2)
        result[name] = value
    if (result["android_bound_devices"] is not None and
            result["android_online_devices"] is not None and
            result["android_online_devices"] > result["android_bound_devices"]):
        raise ValueError("Mesure Android contradictoire.")
    return result


class WorkerResourceSampler:
    """Host-observed system load and worker RSS; read-only ADB when opted in.

    psutil may be absent: unknown values stay None. System-wide percentages
    inside containers are not guaranteed to represent container limits.
    """

    def __init__(self, *, android_preflight=None, enable_adb=False,
                 psutil_module=None):
        self.android_preflight = android_preflight
        self.enable_adb = enable_adb is True
        if psutil_module is None:
            try:
                import psutil
                psutil_module = psutil
            except ImportError:
                psutil_module = None
        self.psutil = psutil_module

    def sample(self):
        values = {}
        if self.psutil is not None:
            try:
                values["cpu_percent"] = self.psutil.cpu_percent(interval=0.05)
                values["memory_percent"] = self.psutil.virtual_memory().percent
                values["process_rss_mb"] = (
                    self.psutil.Process().memory_info().rss / (1024 ** 2))
            except (OSError, ValueError, AttributeError, TypeError):
                pass
        if self.enable_adb and self.android_preflight is not None:
            configured = set(self.android_preflight.bindings.values())
            try:
                discovered = set(self.android_preflight.client_class.devices(
                    adb_binary=self.android_preflight.adb_binary))
                values["android_online_devices"] = len(configured & discovered)
                values["android_bound_devices"] = len(configured)
            except (OSError, RuntimeError, ValueError):
                # ADB offline or unavailable is NOT a claim of zero devices.
                values["android_bound_devices"] = len(configured)
        return clean_metrics(values)
