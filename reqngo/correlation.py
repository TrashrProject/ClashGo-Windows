"""Context-aware but deterministic, read-only correlation of pilot diagnostics.

This does NOT call a language model and never issues actions against a machine.
It distinguishes likely shared failures from isolated pilot incidents.
"""
from __future__ import annotations

from collections import defaultdict

_SEVERITY = {"warning": 1, "error": 2, "critical": 3}


def correlate(snapshot):
    agents = snapshot.get("agents", [])
    cases = [c for c in snapshot.get("cases", []) if c.get("status") == "open"]
    stale = [a for a in agents if a.get("connection") == "stale"]
    insights = []

    def add(code, severity, title, recommendation, affected=()):
        # Machine-safe output: no raw log excerpts or arbitrary input strings.
        insights.append({
            "code": code,
            "severity": severity,
            "title": title,
            "recommendation": recommendation,
            "affected_count": len(set(affected)),
            "automatic_action": None,
        })

    if not agents:
        add("no_pilots_connected", "warning", "Aucun pilote enregistré",
            "Vérifier le déploiement des services pilotes et leurs signaux de vie.")
    elif len(stale) >= 2 and len(stale) * 2 >= len(agents):
        add("shared_connectivity_suspected", "critical",
            "Plusieurs pilotes sans nouvelles",
            "Vérifier le réseau ou le serveur central avant de relancer les pilotes.",
            [a["agent_id"] for a in stale])
    elif len(stale) == 1:
        add("isolated_connectivity_loss", "warning",
            "Un pilote sans nouvelles",
            "Examiner uniquement le pilote concerné avant toute intervention.",
            [stale[0]["agent_id"]])

    grouped = defaultdict(set)
    for item in cases:
        code = item.get("code", "")
        if code != "worker_unreachable" and item.get("agent_id"):
            grouped[code].add(item["agent_id"])
    if any(len(agents_with_fault) >= 2 for agents_with_fault in grouped.values()):
        affected = set().union(*(members for members in grouped.values() if len(members) >= 2))
        add("shared_error_signature", "error", "Même erreur sur plusieurs pilotes",
            "Comparer les changements communs avant d'appliquer une réparation locale.",
            affected)

    if any(item.get("severity") == "critical" for item in cases):
        add("critical_case_unresolved", "critical", "Incident critique non vérifié",
            "Isoler le composant concerné et examiner ses diagnostics ; aucune action risquée automatique.")

    repeated = [item for item in cases if isinstance(item.get("occurrences"), int)
                and item["occurrences"] >= 3]
    if repeated:
        add("repeating_fault", "warning", "Une erreur se répète",
            "Éviter les redémarrages en boucle ; analyser la cause avant de réessayer.",
            [c["agent_id"] for c in repeated if c.get("agent_id")])

    if not insights:
        add("no_active_warnings", "warning", "Aucune alerte détectée",
            "Poursuivre la surveillance : cela ne garantit pas l'absence de panne.")

    return {
        "mode": "deterministic",
        "automatic_actions_executed": 0,
        "insights": sorted(insights, key=lambda x: -_SEVERITY.get(x["severity"], 0))[:5],
    }
