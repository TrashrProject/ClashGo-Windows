package attack

import (
	"testing"

	"github.com/Ducky705/ClashGO/pkg/strategy"
	"gocv.io/x/gocv"
)

func TestStrategyTemplateSubsetUsesOnlyStrategyTemplates(t *testing.T) {
	dragon := gocv.NewMatWithSize(4, 4, gocv.MatTypeCV8UC3)
	balloon := gocv.NewMatWithSize(4, 4, gocv.MatTypeCV8UC3)
	rage := gocv.NewMatWithSize(4, 4, gocv.MatTypeCV8UC3)
	defer dragon.Close()
	defer balloon.Close()
	defer rage.Close()

	all := map[string]gocv.Mat{
		"dragon": dragon,
		"balloon": balloon,
		"rage": rage,
	}
	s := &strategy.DynamicStrategy{Phases: []strategy.Phase{{
		Units: []strategy.Unit{{Name: "Dragon"}, {Name: "Balloon"}},
	}}}

	got, fast := strategyTemplateSubset(all, s)
	if !fast {
		t.Fatal("expected portrait-template fast path")
	}
	if len(got) != 2 {
		t.Fatalf("subset size=%d, want 2", len(got))
	}
	if _, ok := got["rage"]; ok {
		t.Fatal("unneeded rage template leaked into strategy subset")
	}
}

func TestStrategyTemplateSubsetFallsBackIfRequiredTemplateMissing(t *testing.T) {
	dragon := gocv.NewMatWithSize(4, 4, gocv.MatTypeCV8UC3)
	defer dragon.Close()

	all := map[string]gocv.Mat{"dragon": dragon}
	s := &strategy.DynamicStrategy{Phases: []strategy.Phase{{
		Units: []strategy.Unit{{Name: "Dragon"}, {Name: "Balloon"}},
	}}}

	got, fast := strategyTemplateSubset(all, s)
	if fast {
		t.Fatal("unexpected fast path with missing required template")
	}
	if len(got) != len(all) {
		t.Fatalf("fallback size=%d, want %d", len(got), len(all))
	}
}
