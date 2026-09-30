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


func TestAdaptiveFarmLootPercentWeightsDarkElixir(t *testing.T) {
	pct := adaptiveFarmLootPercent(
		1000000, 1000000, 10000,
		500000, 500000, 5000,
	)
	if pct != 50 {
		t.Fatalf("weighted farm loot percent=%d want 50", pct)
	}
}

func TestAdaptiveFarmLootPercentClampsInvalidRemaining(t *testing.T) {
	if got := adaptiveFarmLootPercent(100, 100, 1, 200, 200, 2); got != 0 {
		t.Fatalf("remaining above initial must clamp to 0%% progress, got %d", got)
	}
	if got := adaptiveFarmLootPercent(100, 100, 1, 0, 0, 0); got != 100 {
		t.Fatalf("fully collected loot must report 100%%, got %d", got)
	}
}
