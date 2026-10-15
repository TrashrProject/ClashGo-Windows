"""Read-only incident triage rules. No shell commands or game automation.

The classifications are technical suggestions, not AI-generated diagnoses.
"""
from __future__ import annotations

RULES = {
    "worker_unreachable": ("Connexion perdue", "Le service ne répond plus.",
                           "Vérifier la disponibilité du serveur et les derniers signaux."),
    "network_timeout": ("Réseau", "La communication a expiré.",
                        "Vérifier la connexion puis retester uniquement le service touché."),
    "worker_timeout": ("Service bloqué", "Une opération n'a pas répondu à temps.",
                       "Contrôler le service concerné et refaire une vérification de santé."),
    "demo_timeout": ("Test de délai", "Un délai simulé a été signalé.",
                     "Vérifier le scénario pilote avant d'étendre le test."),
    "session_frozen": ("Session figée", "Le service pilote semble figé.",
                       "Vérifier son état avant un redémarrage ciblé."),
    "worker_crash": ("Service arrêté", "Un composant s'est arrêté.",
                     "Consulter sa sortie et vérifier les dépendances avant la relance."),
    "invalid_config": ("Configuration", "Un paramètre est invalide.",
                       "Comparer les paramètres avec les valeurs attendues."),
    "unknown_failure": ("Inconnu", "La cause n'est pas identifiée.",
                        "Conserver les diagnostics et demander une analyse humaine."),
}
_SEVERITIES = {"warning": 1, "error": 2, "critical": 3}


def classify(code: str, level: str) -> dict:
    """Return a bounded human-readable advisory without claiming a root cause."""
    category, summary, action = RULES.get(
        code, ("À investiguer", "Incident technique non reconnu.",
               "Consulter les diagnostics structurés sans exécuter de commande arbitraire.")
    )
    severity = level if level in _SEVERITIES else "warning"
    return {"category": category, "summary": summary, "recommended_action": action,
            "severity": severity, "automatic_action": None}


def more_severe(current: str, incoming: str) -> str:
    return incoming if _SEVERITIES.get(incoming, 0) > _SEVERITIES.get(current, 0) else current
