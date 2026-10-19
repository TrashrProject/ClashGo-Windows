package com.reqngo.manager;

/** Recognized visual obstacles. Stop instead of retrying a request. */
public final class GameBlocker {
    private GameBlocker() { }

    public static final String[] IDS = {
        "clan_closed", "clan_invitation", "join_pending",
        "request_cooldown", "castle_full", "connection_error"
    };
    public static final String[] LABELS = {
        "Clan fermé / adhésion impossible", "Invitation ou autorisation requise",
        "Demande d'adhésion en attente", "Délai de demande de renforts actif",
        "Château de clan plein", "Erreur de connexion au jeu"
    };

    public static String reason(String screenId) {
        if (screenId == null) return null;
        switch (screenId) {
            case "clan_closed": return "Clan fermé : aucune adhésion automatique";
            case "clan_invitation": return "Accord d'un responsable du clan nécessaire";
            case "join_pending": return "Adhésion encore en attente : ne pas réessayer";
            case "request_cooldown": return "Délai de renforts actif : ne pas cliquer en boucle";
            case "castle_full": return "Château plein : demande bloquée";
            case "connection_error": return "Connexion au jeu interrompue";
            default: return null;
        }
    }

    public static DiagnosticTrail.Event diagnosticEvent(String screenId) {
        if (screenId == null) return null;
        switch (screenId) {
            case "clan_closed": return DiagnosticTrail.Event.GAME_CLAN_CLOSED;
            case "clan_invitation": return DiagnosticTrail.Event.GAME_INVITATION;
            case "join_pending": return DiagnosticTrail.Event.GAME_JOIN_PENDING;
            case "request_cooldown": return DiagnosticTrail.Event.GAME_REQUEST_COOLDOWN;
            case "castle_full": return DiagnosticTrail.Event.GAME_CASTLE_FULL;
            case "connection_error": return DiagnosticTrail.Event.GAME_CONNECTION_ERROR;
            default: return null;
        }
    }

    public static boolean isBlocker(String screenId) { return reason(screenId) != null; }
}
