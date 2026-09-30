package attack

import "testing"

func TestApplyDetectedPositionsTracksBarCompaction(t *testing.T) {
	first := &TrackedSlot{TroopSlot: TroopSlot{X: 100, Y: 680, Category: "Troop"}, UnitName: "dragon", State: SlotDeployed}
	second := &TrackedSlot{TroopSlot: TroopSlot{X: 180, Y: 680, Category: "Troop"}, UnitName: "balloon", State: SlotIdentified}
	hero := &TrackedSlot{TroopSlot: TroopSlot{X: 260, Y: 680, Category: "Hero"}, UnitName: "archer queen", State: SlotDeployed}

	sm := &SlotManager{
		slots: []*TrackedSlot{first, second, hero},
		unitIndex: map[string]*TrackedSlot{
			"dragon": first,
			"balloon": second,
			"archer queen": hero,
		},
		xIndex: map[int]*TrackedSlot{100: first, 180: second, 260: hero},
	}

	// The depleted troop card disappeared. Balloon and the persistent hero
	// moved left but kept their relative order.
	if !sm.applyDetectedPositions([]int{120, 200}) {
		t.Fatal("expected compaction refresh")
	}
	if second.X != 120 {
		t.Fatalf("balloon X=%d, want 120", second.X)
	}
	if hero.X != 200 {
		t.Fatalf("hero X=%d, want 200", hero.X)
	}
	if sm.GetSlot("balloon") != second {
		t.Fatal("unit index lost balloon pointer after position refresh")
	}
}

func TestApplyDetectedPositionsRejectsAmbiguousCount(t *testing.T) {
	a := &TrackedSlot{TroopSlot: TroopSlot{X: 100, Y: 680, Category: "Troop"}, UnitName: "dragon", State: SlotIdentified}
	b := &TrackedSlot{TroopSlot: TroopSlot{X: 180, Y: 680, Category: "Troop"}, UnitName: "balloon", State: SlotIdentified}
	sm := &SlotManager{slots: []*TrackedSlot{a, b}}

	if sm.applyDetectedPositions([]int{120}) {
		t.Fatal("ambiguous card count should not rewrite positions")
	}
	if a.X != 100 || b.X != 180 {
		t.Fatalf("positions changed on ambiguous refresh: %d %d", a.X, b.X)
	}
}
