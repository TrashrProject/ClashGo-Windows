package main

import (
	"testing"
	"time"
)

func TestCancelScheduledSessionStopKeepsLootGoal(t *testing.T) {
	a := &App{
		sessionStopAt:    time.Now().Add(time.Hour),
		sessionGoldGoal:  5_000_000,
		sessionElixirGoal: 3_000_000,
		sessionDarkGoal:  50_000,
	}

	a.CancelScheduledSessionStop()

	if !a.sessionStopAt.IsZero() {
		t.Fatalf("scheduled stop time was not cleared: %v", a.sessionStopAt)
	}
	if a.sessionGoldGoal != 5_000_000 || a.sessionElixirGoal != 3_000_000 || a.sessionDarkGoal != 50_000 {
		t.Fatalf("cancelling timer changed loot goal: gold=%d elixir=%d dark=%d",
			a.sessionGoldGoal, a.sessionElixirGoal, a.sessionDarkGoal)
	}
}
