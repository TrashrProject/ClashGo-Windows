package com.reqngo.manager;

/**
 * Pure deterministic policy for one locally authorized test route.
 * A gesture is never retried, including after a crash or ambiguous screenshot.
 * The final result is VISUAL_CONFIRMATION_ONLY, not a verified server receipt.
 */
public final class AutoRoutePolicy {
    public enum State { IDLE, CHECKING, IN_FLIGHT, VERIFYING, COMPLETE_VISUAL, BLOCKED }
    private int index;
    private State state;

    public AutoRoutePolicy(int index, String state) {
        if (index < 0 || index > GuidedRoute.SIZE) {
            this.index = 0; this.state = State.BLOCKED; return;
        }
        this.index = index;
        try { this.state = State.valueOf(state); }
        catch (RuntimeException e) { this.state = State.BLOCKED; }
        if ((index == GuidedRoute.SIZE) != (this.state == State.COMPLETE_VISUAL)
                && (index == GuidedRoute.SIZE || this.state == State.COMPLETE_VISUAL)) {
            this.state = State.BLOCKED;
        }
    }

    public static AutoRoutePolicy fresh() { return new AutoRoutePolicy(0, "IDLE"); }
    public int index() { return index; }
    public State state() { return state; }
    public String before() { return index < GuidedRoute.SIZE ? GuidedRoute.FROM[index] : "complete"; }
    public String after() { return index < GuidedRoute.SIZE ? GuidedRoute.TO[index] : "complete"; }

    public void start(boolean explicitConsent, boolean allReferencesPresent) {
        if (state != State.IDLE || index != 0 || !explicitConsent || !allReferencesPresent)
            throw new IllegalStateException("Consentement et calibration nécessaires");
        state = State.CHECKING;
    }

    public void verifiedBefore(String screen, boolean foreground, boolean calibratedButton) {
        if (state != State.CHECKING || !foreground || !calibratedButton
                || !before().equals(screen))
            throw new IllegalStateException("Écran préalable non vérifié");
        state = State.IN_FLIGHT;
    }

    public void dispatched() {
        if (state != State.IN_FLIGHT) throw new IllegalStateException("Geste non armé");
        state = State.VERIFYING;
    }

    public void verifiedAfter(String screen) {
        if (state != State.VERIFYING || !after().equals(screen))
            throw new IllegalStateException("Résultat visuel incertain");
        index++;
        state = index == GuidedRoute.SIZE ? State.COMPLETE_VISUAL : State.CHECKING;
    }

    public void block() { state = State.BLOCKED; }

    /** An interrupted operation must never auto-resume after process restart. */
    public static AutoRoutePolicy recover(int index, String phase) {
        AutoRoutePolicy saved = new AutoRoutePolicy(index, phase);
        if (saved.state != State.COMPLETE_VISUAL) saved.block();
        return saved;
    }
}
