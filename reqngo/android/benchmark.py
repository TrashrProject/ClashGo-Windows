"""Read-only performance measurements of a selected Android emulator.

No account data is accessed, screenshots are not persisted, and no touches
are sent. This only measures the ADB screenshot roundtrip, not CoC load time.
"""
from __future__ import annotations

import argparse
import json
import os
import statistics
import time

from reqngo.android.adb import AdbClient


def measure_capture_latency(device, *, samples=5, clock=None):
    """Return aggregate screen capture timings without storing image contents."""
    if type(samples) is not int or not 2 <= samples <= 12:
        raise ValueError("Choisir entre 2 et 12 captures de mesure.")
    if not device.game_running() or not device.game_foreground():
        raise ValueError("Ouvrir Clash of Clans au premier plan avant le test.")
    timer = clock or time.perf_counter
    timings = []
    dimensions = None
    for _ in range(samples):
        if not device.game_foreground():
            raise ValueError("Le jeu a perdu le premier plan pendant la mesure.")
        start = timer()
        frame = device.screenshot()
        milliseconds = (timer() - start) * 1000
        if dimensions is not None and dimensions != (frame.width, frame.height):
            raise ValueError("La resolution Android a change pendant le test.")
        dimensions = (frame.width, frame.height)
        timings.append(round(max(0.0, milliseconds), 2))
    return {
        "samples": samples,
        "median_ms": round(statistics.median(timings), 2),
        "min_ms": round(min(timings), 2),
        "max_ms": round(max(timings), 2),
        "mean_ms": round(statistics.mean(timings), 2),
        "resolution": list(dimensions),
        "note": "Latence ADB capture uniquement ; ne mesure pas les changements de compte."
    }


def main(argv=None):
    parser = argparse.ArgumentParser(description="Diagnostic local de latence ADB de MuMu.")
    parser.add_argument("--serial", required=True)
    parser.add_argument("--adb", default=os.environ.get("ADB_BINARY", "adb"))
    parser.add_argument("--samples", type=int, default=5)
    args = parser.parse_args(argv)
    if args.serial not in AdbClient.devices(adb_binary=args.adb):
        parser.error("Appareil Android introuvable dans ADB.")
    device = AdbClient(args.serial, adb_binary=args.adb)
    print(json.dumps(measure_capture_latency(device, samples=args.samples),
                     indent=2, ensure_ascii=False))


if __name__ == "__main__":
    main()
