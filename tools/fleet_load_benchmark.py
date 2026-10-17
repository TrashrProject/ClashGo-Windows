"""Reproducible V13 PostgreSQL fleet benchmark (synthetic diagnostics only).

This writes disposable logical test profiles and READ-ONLY technical jobs.
It never controls Android, game accounts, devices or external services.

Must explicitly opt in on a database whose name ends in _ci or _test.
The benchmark prints a sanitized JSON result without DSNs or secrets.
"""
from __future__ import annotations

import argparse
import json
import os
import re
import secrets
import statistics
import threading
import time
from concurrent.futures import ThreadPoolExecutor
from pathlib import Path

from reqngo.fleet_postgres import PostgresFleetStore


def percentile(values, fraction):
    if not values:
        return 0.0
    ordered = sorted(values)
    index = (len(ordered) - 1) * fraction
    low = int(index)
    high = min(len(ordered) - 1, low + 1)
    return ordered[low] + (ordered[high] - ordered[low]) * (index - low)


def _milliseconds(values):
    return {label: round(percentile(values, quantile), 3)
            for label, quantile in (("p50", 0.5), ("p95", 0.95), ("p99", 0.99))}


def benchmark(*, dsn: str, profiles: int = 3000, jobs: int = 1000,
              workers: int = 6, batch_size: int = 8, max_seconds: int = 90):
    if (type(profiles) is not int or type(jobs) is not int
            or not 1 <= jobs <= profiles <= 10000):
        raise ValueError("Valeurs de profils ou de taches incorrectes.")
    if (type(workers) is not int or not 1 <= workers <= 16
            or type(batch_size) is not int or not 1 <= batch_size <= 8
            or type(max_seconds) is not int or not 10 <= max_seconds <= 300):
        raise ValueError("Configuration de benchmark incorrecte.")
    if os.environ.get("FLEET_BENCH_DISPOSABLE") != "1":
        raise ValueError("Benchmark refuse : une base jetable explicite est requise.")
    # Check the CURRENT database name, not merely the DSN name, before writing.
    setup = PostgresFleetStore(dsn)
    worker_stores = []
    prefix = "b" + secrets.token_hex(6)
    try:
        with setup.pool.connection() as conn:
            database = conn.execute("SELECT current_database() AS db").fetchone()["db"]
        if not re.fullmatch(r"[a-zA-Z0-9_]+_(?:ci|test)", database):
            raise ValueError("Le benchmark necessite une base de donnees jetable *_ci ou *_test.")
        setup.register_batch(prefix, 1, profiles)
        enqueue_timings = []
        for n in range(1, jobs + 1):
            start = time.perf_counter()
            setup.enqueue_probe(f"{prefix}-{n:06d}", f"load:{prefix}:{n:06d}")
            enqueue_timings.append((time.perf_counter() - start) * 1000)
        worker_stores = [PostgresFleetStore(dsn) for _ in range(workers)]
        latch = threading.Barrier(workers)
        timings_lock = threading.Lock()
        claim_latencies = []
        ack_latencies = []
        observed = []
        errors = []
        started = time.perf_counter()
        deadline = started + max_seconds

        def consume(index, store):
            claims = []
            acks = []
            processed = []
            try:
                latch.wait(timeout=20)
                while time.perf_counter() < deadline:
                    call_start = time.perf_counter()
                    leases = store.claim(f"bench_{index:02d}", limit=batch_size,
                                         capacity=batch_size)
                    claims.append((time.perf_counter() - call_start) * 1000)
                    if not leases:
                        break
                    for lease in leases:
                        ack_start = time.perf_counter()
                        store.complete(lease["job_id"], f"bench_{index:02d}",
                                       lease["lease_token"], True)
                        acks.append((time.perf_counter() - ack_start) * 1000)
                        processed.append(lease["job_id"])
                if time.perf_counter() >= deadline:
                    raise TimeoutError("Benchmark deadline exceeded")
            except Exception as exc:
                with timings_lock:
                    errors.append(type(exc).__name__)
            with timings_lock:
                claim_latencies.extend(claims)
                ack_latencies.extend(acks)
                observed.extend(processed)

        with ThreadPoolExecutor(max_workers=workers) as executor:
            futures = [executor.submit(consume, n, store)
                       for n, store in enumerate(worker_stores)]
            for future in futures:
                future.result()
        elapsed = time.perf_counter() - started
        with setup.pool.connection() as conn:
            row = conn.execute("""
                SELECT COUNT(*) AS jobs, COUNT(*) FILTER (WHERE state='done') AS done,
                       COUNT(*) FILTER (WHERE state='needs_review') AS review
                FROM fleet_jobs WHERE profile_key LIKE %s
            """, (prefix + "-%",)).fetchone()
        if errors or len(observed) != jobs or len(set(observed)) != jobs or row["done"] != jobs:
            raise AssertionError(
                f"Benchmark incomplet: erreurs={len(errors)}, "
                f"traites={len(observed)}, uniques={len(set(observed))}, termines={row['done']}"
            )
        return {
            "scope": "disposable-postgresql-synthetic-health-probes",
            "profiles": profiles,
            "jobs": jobs,
            "workers": workers,
            "worker_batch_size": batch_size,
            "completed": row["done"],
            "needs_review": row["review"],
            "duplicate_claims": len(observed) - len(set(observed)),
            "worker_processing_seconds": round(elapsed, 3),
            "throughput_jobs_per_second": round(jobs / max(elapsed, 1e-6), 2),
            "enqueue_ms": _milliseconds(enqueue_timings),
            "claim_ms": _milliseconds(claim_latencies),
            "ack_ms": _milliseconds(ack_latencies),
            "caveat": "CI measures synthetic job coordination, not Android/game capacity",
        }
    finally:
        for store in worker_stores:
            store.close()
        setup.close()


def main(argv=None):
    parser = argparse.ArgumentParser(description="PostgreSQL test fleet benchmark")
    parser.add_argument("--profiles", type=int, default=3000)
    parser.add_argument("--jobs", type=int, default=1000)
    parser.add_argument("--workers", type=int, default=6)
    parser.add_argument("--output", default="artifacts/fleet-benchmark.json")
    options = parser.parse_args(argv)
    dsn = os.environ.get("FLEET_TEST_PG_DSN")
    if not dsn:
        parser.error("FLEET_TEST_PG_DSN requis.")
    report = benchmark(dsn=dsn, profiles=options.profiles,
                       jobs=options.jobs, workers=options.workers)
    destination = Path(options.output)
    if not destination.is_relative_to(Path("artifacts")):
        parser.error("Le resultat doit rester dans artifacts/.")
    destination.parent.mkdir(parents=True, exist_ok=True)
    destination.write_text(json.dumps(report, ensure_ascii=False,
                                      indent=2) + "\n", encoding="utf-8")
    print(json.dumps(report, sort_keys=True))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
