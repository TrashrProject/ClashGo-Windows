package com.reqngo.manager;

/**
 * Deterministic one-action journal. A dispatched gesture is NOT a confirmed
 * game action. No automatic replay after an uncertain outcome.
 * This class has no Android dependencies so it can be tested on a JVM.
 */
public final class RequestJournal {
    public enum Phase { READY, ARMED, DISPATCHED_UNVERIFIED, CONFIRMED, BLOCKED }

    private Phase phase;

    public RequestJournal(String stored) {
        try { phase = Phase.valueOf(stored); }
        catch (RuntimeException ex) { phase = Phase.BLOCKED; }
    }

    public static RequestJournal fresh() { return new RequestJournal("READY"); }
    public Phase phase() { return phase; }

    public void arm() {
        require(Phase.READY);
        phase = Phase.ARMED;
    }

    public void dispatched() {
        require(Phase.ARMED);
        phase = Phase.DISPATCHED_UNVERIFIED;
    }

    public void confirmedByOperator() {
        require(Phase.DISPATCHED_UNVERIFIED);
        phase = Phase.CONFIRMED;
    }

    public void block() { phase = Phase.BLOCKED; }

    /** Never reset an ARMED or DISPATCHED_UNVERIFIED action blindly. */
    public void reviewAndReset() {
        if (phase != Phase.CONFIRMED && phase != Phase.BLOCKED) {
            throw new IllegalStateException("Action incertaine : examiner le jeu avant toute reprise");
        }
        phase = Phase.READY;
    }

    private void require(Phase expected) {
        if (phase != expected) throw new IllegalStateException(
            "Transition interdite : " + phase + " / attendu " + expected);
    }
}
