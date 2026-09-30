package attack

import (
	"testing"

	"github.com/rs/zerolog"
)

func TestPickSideFallsBackWhenPreferredSpaceIsCramped(t *testing.T) {
	calc := NewDeployLineCalculator(zerolog.Nop())
	free := map[string]int{
		"left":  24,
		"right": 150,
		"top":   90,
		"bottom": 80,
	}
	if got := calc.pickSide(free, "left"); got != "right" {
		t.Fatalf("pickSide=%q, want right when preferred left is cramped", got)
	}
}

func TestPickSideKeepsPreferredWhenSafe(t *testing.T) {
	calc := NewDeployLineCalculator(zerolog.Nop())
	free := map[string]int{
		"left":  80,
		"right": 150,
		"top":   90,
		"bottom": 70,
	}
	if got := calc.pickSide(free, "left"); got != "left" {
		t.Fatalf("pickSide=%q, want safe preferred left", got)
	}
}
