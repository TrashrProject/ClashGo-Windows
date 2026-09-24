package bot

import (
	"fmt"
	"time"

	"github.com/Ducky705/ClashGO/internal/attack"
	"github.com/Ducky705/ClashGO/internal/config"
)

// inspectArmyConsensus samples multiple fresh frames and requires two matching
// definitive verdicts before reporting Ready/NotReady. One noisy OCR frame can
// therefore neither start a bad attack nor generate a bogus training deficit.
func (b *Bot) inspectArmyConsensus(profile config.FarmProfile, attempts int) attack.ArmyGuardResult {
	if attempts < 2 {
		attempts = 2
	}
	if attempts > 4 {
		attempts = 4
	}

	var readyVotes, notReadyVotes int
	var readyResult, notReadyResult attack.ArmyGuardResult
	var warnings []string

	for i := 0; i < attempts; i++ {
		screen, err := b.client.CaptureToMat()
		if err != nil || screen.Empty() {
			if !screen.Empty() {
				screen.Close()
			}
			warnings = append(warnings, fmt.Sprintf("army sample %d capture failed", i+1))
		} else {
			res := b.attackExec.InspectPreBattleArmy(screen, profile)
			screen.Close()
			warnings = append(warnings, res.Warnings...)

			switch res.Decision {
			case attack.ArmyGuardReady:
				readyVotes++
				readyResult = res
				if readyVotes >= 2 {
					readyResult.Warnings = append(readyResult.Warnings, warnings...)
					return readyResult
				}
			case attack.ArmyGuardNotReady:
				notReadyVotes++
				notReadyResult = res
				if notReadyVotes >= 2 {
					notReadyResult.Warnings = append(notReadyResult.Warnings, warnings...)
					return notReadyResult
				}
			}
		}

		if i+1 < attempts && !b.sleepResponsive(140*time.Millisecond) {
			break
		}
	}

	return attack.ArmyGuardResult{
		Decision: attack.ArmyGuardUncertain,
		Warnings: append([]string{"army readiness did not reach two-frame consensus"}, warnings...),
		Observed: map[string]int{},
	}
}
