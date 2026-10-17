"""Bounded, read-only capacity diagnostics for logical diagnostic workers.

These are operational heuristics, not automatic scaling or a prediction of
how many Android instances/game accounts can run on a machine.
"""
from __future__ import annotations


def assess_fleet_capacity(summary: dict) -> dict:
    """Return a stable classification without exposing profiles or credentials."""
    if not isinstance(summary, dict):
        raise ValueError("Resume de flotte invalide.")
    jobs = summary.get("jobs")
    if not isinstance(jobs, dict):
        raise ValueError("Resume de taches invalide.")
    raw = {
        "ready": summary.get("ready_jobs"),
        "slots": summary.get("online_slots"),
        "workers": summary.get("online_workers"),
        "incidents": summary.get("open_worker_incidents"),
        "oldest_seconds": summary.get("oldest_ready_wait_seconds"),
        "leased": jobs.get("leased"),
        "review": jobs.get("needs_review"),
    }
    if any(type(value) is not int or value < 0 for value in raw.values()):
        raise ValueError("Compteurs de capacite invalides.")

    overload = summary.get("overloaded_workers", 0)
    if type(overload) is not int or overload < 0:
        raise ValueError("Saturation invalide.")
    ready = raw["ready"]
    slots = raw["slots"]
    age = raw["oldest_seconds"]
    missing = raw["incidents"]
    # An upper bound on work-to-declared-slot ratio; it is NOT measured device load.
    backlog_per_slot = round(ready / slots, 2) if slots else None
    if ready and slots == 0:
        state = "no_capacity"
        message = "Des sondes attendent, mais aucun slot de worker n'est signale comme disponible."
        action = "Verifier les heartbeats, le reseau et le deploiement des workers."
    elif ready and (age >= 120 or ready > 20 * slots):
        state = "backlog_pressure"
        message = "La file de diagnostic indique une pression ou une attente prolongee."
        action = "Mesurer le debit et la charge reelle des workers avant d'ajuster leur nombre."
    elif overload:
        state = "worker_overload"
        message = "Une ou plusieurs machines ont signale une saturation CPU ou memoire."
        action = "Verifier les charges et reduire les taches si necessaire avant d'augmenter les workers."
    elif missing:
        state = "worker_incidents"
        message = "Un ou plusieurs workers ont un incident de connexion ouvert."
        action = "Examiner les incidents et confirmer la reprise par un nouveau heartbeat."
    elif raw["workers"] == 0:
        state = "idle_without_workers"
        message = "Aucun worker recent n'est observe."
        action = "Demarrer un worker technique si des diagnostics doivent etre traites."
    else:
        state = "nominal"
        message = "Aucune pression de file n'est detectee avec les seuils actuels."
        action = "Continuer a surveiller les latences et les incidents."

    return {
        "state": state,
        "message": message,
        "suggestion": action,
        "ready_jobs": ready,
        "online_workers": raw["workers"],
        "declared_slots": slots,
        "backlog_per_declared_slot": backlog_per_slot,
        "oldest_ready_wait_seconds": age,
        "open_worker_incidents": missing,
        "overloaded_workers": overload,
        "thresholds": {"ready_jobs_per_declared_slot": 20,
                       "oldest_ready_seconds": 120, "worker_cpu_or_memory_percent": 95},
        "scope": "technical-diagnostics-only",
    }
