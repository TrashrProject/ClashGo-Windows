package intelligence

// ReconcileBattleStars combines result-screen OCR with battle facts that are
// already known from the live fight. It returns the reconciled star count and
// whether the visual OCR had to be overridden.
//
// finalPct <= 0 means destruction was not measured reliably, so the visual
// result remains the only available source.
func ReconcileBattleStars(visualStars, finalPct int, townHallDestroyed bool) (stars int, overridden bool) {
	if visualStars < 0 {
		visualStars = 0
	}
	if visualStars > 3 {
		visualStars = 3
	}

	if finalPct <= 0 {
		return visualStars, false
	}
	if finalPct >= 100 {
		return 3, visualStars != 3
	}

	minStars := 0
	maxStars := 1
	if finalPct >= 50 {
		minStars = 1
		maxStars = 2
	}
	if townHallDestroyed {
		minStars++
	}
	if minStars > maxStars {
		minStars = maxStars
	}

	// A visual star above minStars can legitimately reveal a TH destruction
	// that the optional banner detector missed. Keep it as long as it remains
	// possible for the measured destruction percentage.
	if visualStars >= minStars && visualStars <= maxStars {
		return visualStars, false
	}
	return minStars, true
}
