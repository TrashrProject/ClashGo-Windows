package attack

import (
	"fmt"
	"strings"

	"github.com/Ducky705/ClashGO/internal/config"
	"gocv.io/x/gocv"
)

type ArmyGuardDecision int

const (
	ArmyGuardUncertain ArmyGuardDecision = iota
	ArmyGuardReady
	ArmyGuardNotReady
)

type ArmyDeficit struct {
	Name     string `json:"name"`
	Category string `json:"category"`
	Have     int    `json:"have"`
	Need     int    `json:"need"`
	Missing  int    `json:"missing"`
	Housing  int    `json:"housing"`
	Confident bool  `json:"confident"`
}

type ArmyGuardResult struct {
	Decision ArmyGuardDecision
	Warnings []string
	Observed map[string]int
	Deficits []ArmyDeficit
}

// InspectPreBattleArmy reads the live troop bar immediately before the final
// Battle click. It is intentionally conservative: only a positive, confident
// count below the configured target may block the attack. Missing templates or
// OCR uncertainty never create a false "not ready" verdict.
//
// This gives the orchestrator a safe full-army gate now, while leaving actual
// barracks/training UI automation to a later calibrated step.
func (e *Executor) InspectPreBattleArmy(screen gocv.Mat, profile config.FarmProfile) ArmyGuardResult {
	res := ArmyGuardResult{
		Decision: ArmyGuardUncertain,
		Observed: make(map[string]int),
	}
	if screen.Empty() {
		res.Warnings = append(res.Warnings, "empty pre-battle capture")
		return res
	}

	w, h := screen.Cols(), screen.Rows()
	if w < 200 || h < 200 {
		res.Warnings = append(res.Warnings, "pre-battle capture too small")
		return res
	}

	// SlotManager owns the current Windows live-bar geometry and deliberately
	// ignores stale manual slot coordinates on Windows.
	pCfg := PrecisionConfig{Width: 860, Height: 732}
	barY := int(float64(h) * 0.82)
	sm := NewSlotManager(screen, pCfg, w, h, barY, e.templates, e.classify, e.logger)
	slots := sm.GetAllSlots()
	if len(slots) == 0 {
		res.Warnings = append(res.Warnings, "no active army cards detected")
		return res
	}

	counter := NewTroopCounter(860, 732, e.logger)
	defer counter.Close()
	counts := counter.DetectCounts(screen, slots, sm.GetBarY())

	identified := 0
	confidentCounts := 0
	for _, slot := range slots {
		if slot == nil {
			continue
		}
		name := strings.ToLower(strings.TrimSpace(slot.UnitName))
		if name == "" {
			continue
		}
		identified++

		countRead, hasRead := GetTroopCountForSlot(counts, slot.X)
		count := 0
		confident := hasRead && countRead.Count > 0 && countRead.Confidence >= 0.58
		if confident {
			count = countRead.Count
		}

		switch slot.Category {
		case "Hero", "Siege", "CC":
			// One-shot cards do not need quantity OCR. Their visible,
			// non-empty card is stronger evidence than a missing digit.
			if count <= 0 && !slot.IsEmpty {
				count = 1
				confident = true
			}
		}

		if count > 0 && confident {
			confidentCounts++
			res.Observed[name] += count
		}
	}

	if identified == 0 {
		res.Warnings = append(res.Warnings, "army cards visible but none identified confidently")
		return res
	}

	hardMissing := 0
	uncertain := 0
	check := func(name string, expected, housing int, category string) {
		if expected <= 0 || strings.TrimSpace(name) == "" {
			return
		}
		key := strings.ToLower(strings.TrimSpace(name))
		got, seen := res.Observed[key]
		if !seen {
			// No confident positive count: could be OCR/template uncertainty.
			// Do not block the attack solely on missing evidence.
			uncertain++
			res.Deficits = append(res.Deficits, ArmyDeficit{
				Name: name, Category: category, Have: 0, Need: expected,
				Missing: expected, Housing: housing, Confident: false,
			})
			res.Warnings = append(res.Warnings,
				fmt.Sprintf("%s %s: target %d, count not confidently readable", category, name, expected))
			return
		}
		if got < expected {
			hardMissing++
			res.Deficits = append(res.Deficits, ArmyDeficit{
				Name: name, Category: category, Have: got, Need: expected,
				Missing: expected-got, Housing: housing, Confident: true,
			})
			res.Warnings = append(res.Warnings,
				fmt.Sprintf("%s %s: detected %d, target %d", category, name, got, expected))
		}
	}

	for _, u := range profile.Troops {
		check(u.Name, u.Count, u.Housing, "troop")
	}
	for _, u := range profile.Spells {
		check(u.Name, u.Count, u.Housing, "spell")
	}

	if hardMissing > 0 {
		res.Decision = ArmyGuardNotReady
		return res
	}
	if uncertain > 0 || confidentCounts == 0 {
		res.Decision = ArmyGuardUncertain
		return res
	}

	res.Decision = ArmyGuardReady
	return res
}
