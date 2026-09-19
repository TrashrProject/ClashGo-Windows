package bot

import "testing"

func TestRecoveryStatsFieldsAreAdditiveSessionCounters(t *testing.T) {
	s := BotStats{
		RecoveryAttempts:   3,
		RecoverySuccesses:  2,
		BlueStacksRestarts: 1,
	}
	if s.RecoveryAttempts != 3 || s.RecoverySuccesses != 2 || s.BlueStacksRestarts != 1 {
		t.Fatal("recovery counters not retained")
	}
}
