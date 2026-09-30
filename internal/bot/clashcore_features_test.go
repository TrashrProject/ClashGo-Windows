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
	if !collectorResourceIncreased("gold", before, after) {
		t.Fatal("gold gain should verify")
	}
	if !collectorResourceIncreased("elixir", before, after) {
		t.Fatal("elixir gain should verify")
	}
	if collectorResourceIncreased("dark_elixir", before, after) {
		t.Fatal("unchanged dark elixir must not verify")
	}
	after.GoldValid = false
	if collectorResourceIncreased("gold", before, after) {
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
