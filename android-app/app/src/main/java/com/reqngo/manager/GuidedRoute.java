package com.reqngo.manager;

/**
 * An explicitly supervised route through four observable Clash of Clans screens.
 *
 * No automatic account access, clan entry, request emission or claims about
 * game success. Each transition needs a captured before-state, one authorized
 * gesture and an independently reviewed after-state. An uncertain transition
 * NEVER advances or retries on its own.
 */
public final class GuidedRoute {
    public static final String[] FROM = {
        "village", "clan_profile", "clan_chat", "request_dialog"
    };
    public static final String[] TO = {
        "clan_profile", "clan_chat", "request_dialog", "request_posted"
    };
    public static final String[] TITLE = {
        "Ouvrir la fiche du clan",
        "Accéder au chat du clan (après adhésion)",
        "Ouvrir la demande de renforts",
        "Envoyer une demande de renforts"
    };
    public static final int SIZE = TITLE.length;

    public enum Phase { READY, AWAITING_REVIEW, BLOCKED, COMPLETE }

    private int index;
    private Phase phase;

    /** Invalid persistence always fails closed. */
    public GuidedRoute(int index, String phase) {
        if (index < 0 || index > SIZE) {
            this.index = 0;
            this.phase = Phase.BLOCKED;
            return;
        }
        this.index = index;
        try { this.phase = Phase.valueOf(phase); }
        catch (RuntimeException e) { this.phase = Phase.BLOCKED; }
        if (this.index == SIZE && this.phase != Phase.COMPLETE) {
            this.phase = Phase.BLOCKED;
        }
        if (this.index < SIZE && this.phase == Phase.COMPLETE) {
            this.phase = Phase.BLOCKED;
        }
    }

    public static GuidedRoute fresh() { return new GuidedRoute(0, "READY"); }

    public int index() { return index; }
    public Phase phase() { return phase; }

    public String expectedBefore() {
        if (index >= SIZE) return "complete";
        return FROM[index];
    }

    public String expectedAfter() {
        if (index >= SIZE) return "complete";
        return TO[index];
    }

    /**
     * Precondition: the operator selected a point on a NEW screenshot, and
     * an unambiguous classifier matched the before-state.
     */
    public void start(String identifiedBefore, boolean captureFresh) {
        if (phase != Phase.READY || index >= SIZE) throw new IllegalStateException("Parcours non prêt");
        if (!captureFresh || !expectedBefore().equals(identifiedBefore)) {
            throw new IllegalStateException("Écran de départ non vérifié");
        }
        phase = Phase.AWAITING_REVIEW; // persisted before gesture dispatch
    }

    /**
     * Completion requires game screenshot evidence AND explicit confirmation.
     * No screenshot alone is treated as proof of real clan membership/request.
     */
    public void accept(String identifiedAfter, boolean freshPostCapture, boolean operatorConfirmed) {
        if (phase != Phase.AWAITING_REVIEW) throw new IllegalStateException("Aucune action à revoir");
        if (!freshPostCapture || !operatorConfirmed || !expectedAfter().equals(identifiedAfter)) {
            throw new IllegalStateException("Résultat non confirmé : vérifier le jeu");
        }
        index++;
        phase = index == SIZE ? Phase.COMPLETE : Phase.READY;
    }

    /** Failed/ambiguous actions require explicit operator intervention. */
    public void block() { phase = Phase.BLOCKED; }

    public void restartAfterReview() {
        if (phase != Phase.BLOCKED && phase != Phase.COMPLETE)
            throw new IllegalStateException("Revue requise avant réinitialisation");
        index = 0;
        phase = Phase.READY;
    }

    public String summary() {
        if (phase == Phase.COMPLETE) return "Parcours terminé (revu par l'opérateur)";
        if (phase == Phase.BLOCKED) return "Parcours bloqué : vérification manuelle";
        return "Étape " + (index + 1) + "/" + SIZE + " : " + TITLE[index]
                + " [" + phase + "]";
    }
}
