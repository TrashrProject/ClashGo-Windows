package attack

import (
	"testing"

	"github.com/Ducky705/ClashGO/pkg/strategy"
	"github.com/rs/zerolog"
)

func TestStrategyCountSlotsUsesOnlyRequiredCardsWhenComplete(t *testing.T) {
	a := &TrackedSlot{UnitName: "dragon"}
	b := &TrackedSlot{UnitName: "balloon"}
	extra := &TrackedSlot{UnitName: "rage"}
	sm := &SlotManager{
		slots: []*TrackedSlot{a, b, extra},
		unitIndex: map[string]*TrackedSlot{
			"dragon": a,
			"balloon": b,
			"rage": extra,
		},
		logger: zerolog.Nop(),
	}
	s := &strategy.DynamicStrategy{Phases: []strategy.Phase{{
		Units: []strategy.Unit{{Name: "Dragon"}, {Name: "Balloon"}},
	}}}

	got, fast := strategyCountSlots(sm, s)
	if !fast {
		t.Fatal("expected fast path")
	}
	if len(got) != 2 {
		t.Fatalf("got %d slots, want 2", len(got))
	}
}

func TestStrategyCountSlotsFallsBackWhenIdentityMissing(t *testing.T) {
	a := &TrackedSlot{UnitName: "dragon"}
	unknown := &TrackedSlot{}
	sm := &SlotManager{
		slots: []*TrackedSlot{a, unknown},
		unitIndex: map[string]*TrackedSlot{"dragon": a},
		logger: zerolog.Nop(),
	}
	s := &strategy.DynamicStrategy{Phases: []strategy.Phase{{
		Units: []strategy.Unit{{Name: "Dragon"}, {Name: "Balloon"}},
	}}}

	got, fast := strategyCountSlots(sm, s)
	if fast {
		t.Fatal("unexpected fast path with missing identity")
	}
	if len(got) != 2 {
		t.Fatalf("fallback got %d slots, want full bar 2", len(got))
	}
}
