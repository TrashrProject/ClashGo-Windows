package attack

import (
	"sort"

	"gocv.io/x/gocv"
)

// RefreshPositions updates the X coordinates of cards after CoC compacts the
// battle bar. It deliberately performs only the cheap structural active-card
// scan; identities and counts are preserved from the initial attack plan.
//
// To avoid corrupting the immutable plan when the visual evidence is
// ambiguous, positions are updated only when the number of detected cards
// exactly matches the number of cards expected to remain visible.
//
// Hero cards remain visible after their first deployment because the same
// button later owns the hero ability, so deployed heroes stay in the mapping.
func (sm *SlotManager) RefreshPositions(screen gocv.Mat) bool {
	if sm == nil || screen.Empty() {
		return false
	}

	activeXs := sm.detectActiveSlots(screen)
	return sm.applyDetectedPositions(activeXs)
}

// applyDetectedPositions is the pure remapping half of RefreshPositions. It is
// split out so compaction semantics are testable without constructing a full
// OpenCV battle frame.
func (sm *SlotManager) applyDetectedPositions(activeXs []int) bool {
	if sm == nil || len(activeXs) == 0 {
		return false
	}

	visible := make([]*TrackedSlot, 0, len(sm.slots))
	for _, slot := range sm.slots {
		if slot == nil {
			continue
		}
		if slot.State == SlotDeployed && slot.Category != "Hero" {
			continue
		}
		if slot.State == SlotFailed {
			continue
		}
		visible = append(visible, slot)
	}

	if len(activeXs) != len(visible) {
		sm.logger.Debug().
			Int("detected_cards", len(activeXs)).
			Int("expected_cards", len(visible)).
			Msg("battle-bar compaction refresh skipped: ambiguous card count")
		return false
	}

	sort.SliceStable(visible, func(i, j int) bool { return visible[i].X < visible[j].X })

	moved := 0
	sm.xIndex = make(map[int]*TrackedSlot, len(sm.slots))
	for i, slot := range visible {
		oldX := slot.X
		slot.X = activeXs[i]
		if slot.X != oldX {
			moved++
		}
		sm.xIndex[slot.X] = slot
	}

	// Keep terminal cards in xIndex as well for diagnostics; unitIndex points
	// to slot objects, so it remains valid after X updates.
	for _, slot := range sm.slots {
		if slot == nil {
			continue
		}
		if _, ok := sm.xIndex[slot.X]; !ok {
			sm.xIndex[slot.X] = slot
		}
	}

	if moved > 0 {
		sm.logger.Debug().
			Int("moved_cards", moved).
			Ints("slot_xs", activeXs).
			Msg("battle-bar positions refreshed after compaction")
	}
	return true
}
