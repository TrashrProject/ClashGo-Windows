package bot

import (
	"time"

	"github.com/Ducky705/ClashGO/internal/attack"
	"github.com/Ducky705/ClashGO/internal/config"
	"github.com/Ducky705/ClashGO/internal/game"
)

// reapplyActiveArmyRecipe performs one bounded repair attempt using the saved
// recipe already selected by ClashGO. Since modern Clash army recipes apply
// instantly, "waiting for training" is not a repair strategy anymore.
//
// The helper proves the army menu state, reopens the recipe list only when
// necessary, selects the configured recipe with visual verification, then
// requires a fresh multi-frame army consensus.
func (b *Bot) reapplyActiveArmyRecipe(profile config.FarmProfile) (attack.ArmyGuardResult, bool) {
	screen, err := b.client.CaptureToMat()
	if err != nil || screen.Empty() {
		if !screen.Empty() { screen.Close() }
		return attack.ArmyGuardResult{Decision: attack.ArmyGuardUncertain}, false
	}
	state, _ := b.classify(screen)
	screen.Close()

	if state != game.StateArmySelection && state != game.StateArmyCamp {
		// The recipe list may have collapsed after the first selection. Only
		// reopen it through a visually verified arrow button.
		if !b.findAndClick("btn_army_arrow", "Army Arrow", 2) {
			b.logger.Warn().Str("state", state.String()).Msg("cannot reopen army recipes for repair")
			return attack.ArmyGuardResult{Decision: attack.ArmyGuardUncertain}, false
		}
		if !b.sleepResponsive(180 * time.Millisecond) {
			return attack.ArmyGuardResult{Decision: attack.ArmyGuardUncertain}, false
		}
	}

	if !b.selectArmySlot() {
		b.logger.Warn().Int("army_slot", b.armySlot).Msg("army recipe repair could not verify recipe selection")
		return attack.ArmyGuardResult{Decision: attack.ArmyGuardUncertain}, false
	}

	if !b.sleepResponsive(240 * time.Millisecond) {
		return attack.ArmyGuardResult{Decision: attack.ArmyGuardUncertain}, false
	}
	guard := b.inspectArmyConsensus(profile, 3)
	return guard, guard.Decision == attack.ArmyGuardReady
}
