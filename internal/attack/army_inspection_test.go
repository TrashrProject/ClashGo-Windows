package attack

import (
	"testing"

	"github.com/Ducky705/ClashGO/internal/config"
)

func inspectionSlot(name, category string, x int) *TrackedSlot {
	s := &TrackedSlot{
		UnitName:   name,
		Category:   category,
		Confidence: 0.95,
	}
	s.X = x
	return s
}

func TestBuildArmyInspectionReadyWhenCompositionMatches(t *testing.T) {
	profile := config.FarmProfile{
		TownHall: 18,
		Label: "TH18 Farm",
		Troops: []config.FarmUnit{{Name: "Electro Dragon", Count: 9}},
		Spells: []config.FarmUnit{{Name: "Rage Spell", Count: 5}},
	}
	slots := []*TrackedSlot{
		inspectionSlot("Electro Dragon", "Troop", 100),
		inspectionSlot("Rage Spell", "Spell", 200),
	}
	counts := []TroopCount{
		{X: 100, Count: 9, Confidence: 0.91},
		{X: 200, Count: 5, Confidence: 0.88},
	}

	got := buildArmyInspection(slots, counts, &profile)
	if !got.Ready || got.Uncertain {
		t.Fatalf("matching army should be ready and certain: %+v", got)
	}
	if len(got.Warnings) != 0 {
		t.Fatalf("matching army produced warnings: %+v", got.Warnings)
	}
}

func TestBuildArmyInspectionRejectsProvenShortage(t *testing.T) {
	profile := config.FarmProfile{
		TownHall: 18,
		Troops: []config.FarmUnit{{Name: "Electro Dragon", Count: 9}},
	}
	slots := []*TrackedSlot{
		inspectionSlot("Electro Dragon", "Troop", 100),
	}
	counts := []TroopCount{
		{X: 100, Count: 6, Confidence: 0.93},
	}

	got := buildArmyInspection(slots, counts, &profile)
	if got.Ready {
		t.Fatalf("proven shortage must not be ready: %+v", got)
	}
	if got.Uncertain {
		t.Fatalf("high-confidence shortage should be certain: %+v", got)
	}
	if len(got.Warnings) == 0 {
		t.Fatal("proven shortage should explain the mismatch")
	}
}

func TestBuildArmyInspectionFailsOpenOnUnreadableQuantity(t *testing.T) {
	profile := config.FarmProfile{
		TownHall: 18,
		Troops: []config.FarmUnit{{Name: "Electro Dragon", Count: 9}},
	}
	slots := []*TrackedSlot{
		inspectionSlot("Electro Dragon", "Troop", 100),
	}
	// Count=0 with no OCR confidence means "quantity unreadable", not
	// "zero troops". AutoArmyGuard must never skip a good target from that.
	counts := []TroopCount{
		{X: 100, Count: 0, Confidence: 0},
	}

	got := buildArmyInspection(slots, counts, &profile)
	if !got.Ready {
		t.Fatalf("uncertain OCR should fail open, got not-ready: %+v", got)
	}
	if !got.Uncertain {
		t.Fatalf("unreadable quantity should be marked uncertain: %+v", got)
	}
}

func TestBuildArmyInspectionFailsOpenWhenUnknownSlotCouldExplainMismatch(t *testing.T) {
	profile := config.FarmProfile{
		TownHall: 18,
		Troops: []config.FarmUnit{{Name: "Electro Dragon", Count: 9}},
	}
	slots := []*TrackedSlot{
		inspectionSlot("", "Troop", 100),
	}
	counts := []TroopCount{
		{X: 100, Count: 9, Confidence: 0.9},
	}

	got := buildArmyInspection(slots, counts, &profile)
	if !got.Ready || !got.Uncertain {
		t.Fatalf("unknown portrait should fail open as uncertain: %+v", got)
	}
}
