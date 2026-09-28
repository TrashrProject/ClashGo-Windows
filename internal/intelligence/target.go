package intelligence

import "math"

type TargetRules struct {
	MinGold       int
	MinElixir     int
	MinDarkElixir int
	DarkOverride  int
	SearchEnabled bool
}

type Target struct {
	Gold       int
	Elixir     int
	DarkElixir int
}

type TargetDecision struct {
	Accept bool     `json:"accept"`
	Score  int      `json:"score"`
	Reason string   `json:"reason"`
	Flags  []string `json:"flags,omitempty"`
}

func EvaluateTarget(t Target, rules TargetRules) TargetDecision {
	if !rules.SearchEnabled {
		return TargetDecision{Accept: true, Score: 100, Reason: "search filters disabled"}
	}

	goldRatio := ratio(t.Gold, rules.MinGold)
	elixirRatio := ratio(t.Elixir, rules.MinElixir)
	deRatio := ratio(t.DarkElixir, rules.MinDarkElixir)

	// Weighted for farming: gold/elixir dominate sustained throughput while
	// dark elixir receives enough weight to make a rich DE target visible.
	raw := 0.4*math.Min(goldRatio, 1.5) +
		0.4*math.Min(elixirRatio, 1.5) +
		0.2*math.Min(deRatio, 1.5)
	score := int(math.Round(math.Min(100, raw/1.5*100)))

	flags := make([]string, 0, 4)
	if t.Gold >= rules.MinGold { flags = append(flags, "gold") }
	if t.Elixir >= rules.MinElixir { flags = append(flags, "elixir") }
	if t.DarkElixir >= rules.MinDarkElixir { flags = append(flags, "dark_elixir") }

	allMinimums := t.Gold >= rules.MinGold &&
		t.Elixir >= rules.MinElixir &&
		t.DarkElixir >= rules.MinDarkElixir
	if allMinimums {
		return TargetDecision{Accept: true, Score: score, Reason: "all loot thresholds met", Flags: flags}
	}

	if rules.DarkOverride > 0 && t.DarkElixir >= rules.DarkOverride {
		flags = append(flags, "dark_override")
		return TargetDecision{Accept: true, Score: score, Reason: "dark elixir override met", Flags: flags}
	}

	return TargetDecision{Accept: false, Score: score, Reason: "loot below configured thresholds", Flags: flags}
}

func ratio(value, minimum int) float64 {
	if minimum <= 0 {
		return 1
	}
	if value <= 0 {
		return 0
	}
	return float64(value) / float64(minimum)
}
