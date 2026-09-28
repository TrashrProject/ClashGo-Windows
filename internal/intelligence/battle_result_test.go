package intelligence

import "testing"

func TestReconcileBattleStarsEnforcesTownHallMinimumBelow50(t *testing.T) {
	got, overridden := ReconcileBattleStars(0, 37, true)
	if got != 1 || !overridden {
		t.Fatalf("got stars=%d overridden=%v want 1,true", got, overridden)
	}
}

func TestReconcileBattleStarsEnforcesTownHallMinimumAbove50(t *testing.T) {
	got, overridden := ReconcileBattleStars(1, 73, true)
	if got != 2 || !overridden {
		t.Fatalf("got stars=%d overridden=%v want 2,true", got, overridden)
	}
}

func TestReconcileBattleStarsKeepsPlausibleVisualTHStarWhenBannerMissed(t *testing.T) {
	got, overridden := ReconcileBattleStars(2, 73, false)
	if got != 2 || overridden {
		t.Fatalf("got stars=%d overridden=%v want 2,false", got, overridden)
	}
}

func TestReconcileBattleStarsRejectsImpossibleThreeStarBelow100(t *testing.T) {
	got, overridden := ReconcileBattleStars(3, 85, true)
	if got != 2 || !overridden {
		t.Fatalf("got stars=%d overridden=%v want 2,true", got, overridden)
	}
}

func TestReconcileBattleStarsForcesThreeAt100(t *testing.T) {
	got, overridden := ReconcileBattleStars(1, 100, false)
	if got != 3 || !overridden {
		t.Fatalf("got stars=%d overridden=%v want 3,true", got, overridden)
	}
}

func TestReconcileBattleStarsLeavesVisualWhenOutcomeUnknown(t *testing.T) {
	got, overridden := ReconcileBattleStars(2, 0, false)
	if got != 2 || overridden {
		t.Fatalf("got stars=%d overridden=%v want 2,false", got, overridden)
	}
}
