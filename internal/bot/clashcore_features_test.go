package bot

import (
	"testing"

	"github.com/Ducky705/ClashGO/internal/game"
)

func TestCollectorResourceIncreased(t *testing.T) {
	before := game.VillageResourceSnapshot{
		Gold: 100, Elixir: 200, DarkElixir: 300,
		GoldValid: true, ElixirValid: true, DarkValid: true,
	}
	after := game.VillageResourceSnapshot{
		Gold: 150, Elixir: 210, DarkElixir: 300,
		GoldValid: true, ElixirValid: true, DarkValid: true,
	}
	if !func() bool { ok, _ := collectorResourceVerification("gold", before, after); return ok }() {
		t.Fatal("gold gain should verify")
	}
	if !func() bool { ok, _ := collectorResourceVerification("elixir", before, after); return ok }() {
		t.Fatal("elixir gain should verify")
	}
	if func() bool { ok, _ := collectorResourceVerification("dark_elixir", before, after); return ok }() {
		t.Fatal("unchanged dark elixir must not verify")
	}
	after.GoldValid = false
	if func() bool { ok, _ := collectorResourceVerification("gold", before, after); return ok }() {
		t.Fatal("invalid OCR must not verify")
	}
}

func TestNearLootThreshold(t *testing.T) {
	if !nearLootThreshold(950, 1000, 10) {
		t.Fatal("95% of threshold should be a near miss")
	}
	if nearLootThreshold(899, 1000, 10) {
		t.Fatal("value below configured window should not be sampled")
	}
	if nearLootThreshold(1000, 1000, 10) {
		t.Fatal("meeting the threshold is not a rejected near miss")
	}
}


func TestCollectorResourceVerificationRequiresComparableOCR(t *testing.T) {
	before := game.VillageResourceSnapshot{Gold: 100, GoldValid: true}
	after := game.VillageResourceSnapshot{Gold: 200, GoldValid: false}
	increased, comparable := collectorResourceVerification("gold", before, after)
	if increased || comparable {
		t.Fatalf("invalid post-tap OCR must be inconclusive, got increased=%v comparable=%v", increased, comparable)
	}
}
