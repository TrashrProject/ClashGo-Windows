package bot

import (
	"time"

	"github.com/Ducky705/ClashGO/internal/game"
)

const (
	runtimeSupervisorTick        = 3 * time.Second
	runtimeCaptureStaleThreshold = 18 * time.Second
)

func (b *Bot) observeRuntimeState(state game.GameState, at time.Time) {
	previous := game.GameState(b.runtimeState.Load())
	if previous == state {
		return
	}
	b.runtimeState.Store(int32(state))
	b.runtimeStateSince.Store(at.UnixNano())
	b.runtimeProgress.Store(at.UnixNano())
	b.logger.Info().
		Str("from", previous.String()).
		Str("to", state.String()).
		Msg("runtime state transition")
}

func runtimeStateTimeout(state game.GameState) time.Duration {
	switch state {
	case game.StateMainVillage:
		return 0
	case game.StateLogo:
		return 3 * time.Minute
	case game.StateTapToContinue, game.StateNewsSplash:
		return 25 * time.Second
	case game.StateConnectionLost, game.StateConfirmExit:
		return 15 * time.Second
	case game.StateChestReward, game.StateWelcomeBack:
		return 35 * time.Second
	case game.StateObstacleDialog, game.StateGemDialog, game.StateChatOpen, game.StateShieldInfo:
		return 25 * time.Second
	case game.StateArmyCamp, game.StateArmySelection, game.StateFindMatch, game.StateSettings:
		return 30 * time.Second
	case game.StateSearchMap, game.StateLoading:
		return 45 * time.Second
	case game.StateBattle, game.StateBattleEnd, game.StateReturnHome:
		return 40 * time.Second
	case game.StateBuilderBase:
		return 45 * time.Second
	case game.StateUnknown:
		return 30 * time.Second
	default:
		return 35 * time.Second
	}
}

func (b *Bot) runtimeSupervisorLoop() {
	ticker := time.NewTicker(runtimeSupervisorTick)
	defer ticker.Stop()

	b.logger.Info().Msg("runtime supervisor active")

	for {
		select {
		case <-b.ctx.Done():
			return
		case <-ticker.C:
			now := time.Now()

			if last := b.captureHeartbeat.Load(); last > 0 {
				staleFor := now.Sub(time.Unix(0, last))
				if staleFor >= runtimeCaptureStaleThreshold && !b.recoveryInFlight.Load() {
					b.logger.Error().
						Dur("capture_stale_for", staleFor).
						Str("phase", RuntimePhase(b.runtimePhase.Load()).String()).
						Str("state", game.GameState(b.runtimeState.Load()).String()).
						Msg("runtime supervisor: capture heartbeat stalled; escalating recovery")
					go b.recoverEmulator()
					b.captureHeartbeat.Store(now.UnixNano())
					continue
				}
			}

			if b.seqRunning.Load() || b.recoveryInFlight.Load() || b.restartInFlight.Load() {
				continue
			}

			state := game.GameState(b.runtimeState.Load())
			timeout := runtimeStateTimeout(state)
			if timeout <= 0 {
				continue
			}
			since := b.runtimeStateSince.Load()
			if since <= 0 || now.Sub(time.Unix(0, since)) < timeout {
				continue
			}

			b.logger.Warn().
				Str("state", state.String()).
				Str("phase", RuntimePhase(b.runtimePhase.Load()).String()).
				Dur("state_age", now.Sub(time.Unix(0, since))).
				Msg("runtime supervisor: UI stopped progressing; restarting game")
			b.runtimeStateSince.Store(now.UnixNano())
			go b.restartGame()
		}
	}
}
