package attack

import "testing"

func TestBattleLootSampleDueWhenLootExitEnabled(t *testing.T) {
	for tick := 1; tick <= 8; tick++ {
		if !battleLootSampleDue(true, tick) {
			t.Fatalf("loot-exit enabled must sample every tick; tick=%d", tick)
		}
	}
}

func TestBattleLootSampleDueThrottlesStatsOnlySampling(t *testing.T) {
	want := map[int]bool{1: true, 4: true, 7: true, 10: true}
	for tick := 0; tick <= 10; tick++ {
		got := battleLootSampleDue(false, tick)
		if got != want[tick] {
			t.Fatalf("tick=%d got=%v want=%v", tick, got, want[tick])
		}
	}
}
