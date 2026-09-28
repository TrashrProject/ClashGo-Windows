package intelligence

// AdaptiveSearchPolicy bounds how far target thresholds may relax during a
// long matchmaking streak. It never changes the user's base config on disk.
type AdaptiveSearchPolicy struct {
	Enabled          bool
	StartAfterSkips  int
	StepEverySkips   int
	StepPercent      int
	FloorPercent     int
}

// AdaptTargetRules returns an in-memory copy of base with progressively
// relaxed loot thresholds. The floor is expressed as a percentage of the
// configured values (70 means thresholds can never fall below 70%).
func AdaptTargetRules(base TargetRules, skips int, p AdaptiveSearchPolicy) (TargetRules, int) {
	if !p.Enabled || skips < p.StartAfterSkips {
		return base, 100
	}
	if p.StepEverySkips <= 0 {
		p.StepEverySkips = 4
	}
	if p.StepPercent <= 0 {
		p.StepPercent = 5
	}
	if p.FloorPercent <= 0 || p.FloorPercent > 100 {
		p.FloorPercent = 70
	}

	steps := ((skips - p.StartAfterSkips) / p.StepEverySkips) + 1
	percent := 100 - steps*p.StepPercent
	if percent < p.FloorPercent {
		percent = p.FloorPercent
	}
	scale := func(v int) int {
		if v <= 0 {
			return v
		}
		return v * percent / 100
	}

	out := base
	out.MinGold = scale(base.MinGold)
	out.MinElixir = scale(base.MinElixir)
	out.MinDarkElixir = scale(base.MinDarkElixir)
	// DarkOverride is an explicit user override and must remain exact.
	return out, percent
}
