package bot

import (
	"time"

	"github.com/Ducky705/ClashGO/internal/game"
)

const (
	runtimeSupervisorTick        = 5 * time.Second
	runtimeCaptureStaleThreshold = 45 * time.Second
)

func automationTaskTimeout(name string) time.Duration {
	switch name {
	case "chest reward":
		return 25 * time.Second
	case "resource scan":
		return 20 * time.Second
	case "donation":
		return 75 * time.Second
	case "village navigation":
		return 45 * time.Second
	case "return home recovery":
		return 75 * time.Second
	case "army check":
		return 90 * time.Second
	case "wall upgrades":
		return 2 * time.Minute
	case "attack":
		return 12 * time.Minute
	default:
		return 2 * time.Minute
	}
}

// observeRuntimeState stores only CONFIRMED classifier states. Raw one-frame
// guesses never reach this supervisor, which prevents recovery decisions from
// being driven by transient misclassification.
func (b *Bot) observeRuntimeState(state game.GameState, at time.Time) {
	previous := game.GameState(b.runtimeState.Load())
	if previous == state {
		return
	}

	b.runtimeState.Store(int32(state))
	b.runtimeStateSince.Store(at.UnixNano())
	b.runtimeProgress.Store(at.UnixNano())

	b.logger.Debug().
		Str("from", previous.String()).
		Str("to", state.String()).
		Msg("runtime supervisor observed confirmed state transition")
}

// runtimeStateTimeout returns how long a confirmed state may remain unchanged
// while no attack sequence is actively owning the UI.
//
// A zero duration means the state can be legitimately idle and must not cause
// a restart. MainVillage is intentionally unbounded: training/cooldowns may
// keep a healthy bot there for a long time.
func runtimeStateTimeout(state game.GameState) time.Duration {
	switch state {
	case game.StateMainVillage:
		return 0
	case game.StateLogo:
		// Cold game/network startup can legitimately sit on the logo for
		// minutes. This is the one deliberately generous UI timeout.
		return 3 * time.Minute
	case game.StateTapToContinue, game.StateNewsSplash:
		return 20 * time.Second
	case game.StateConnectionLost:
		return 15 * time.Second
	case game.StateConfirmExit:
		return 12 * time.Second
	case game.StateChestReward, game.StateWelcomeBack:
		return 30 * time.Second
	case game.StateObstacleDialog, game.StateGemDialog, game.StateChatOpen, game.StateShieldInfo:
		return 20 * time.Second
	case game.StateArmyCamp, game.StateArmySelection, game.StateFindMatch, game.StateSettings:
		return 25 * time.Second
	case game.StateSearchMap, game.StateLoading:
		// These are only supervised when seqRunning=false. During a real
		// search the attack sequence owns its own timeout and this path is
		// skipped.
		return 40 * time.Second
	case game.StateBattle, game.StateBattleEnd, game.StateReturnHome:
		return 30 * time.Second
	case game.StateBuilderBase:
		return 45 * time.Second
	case game.StateUnknown:
		return 25 * time.Second
	default:
		return 30 * time.Second
	}
}

// runtimeSupervisorLoop is deliberately independent from captureLoop.
//
// It supervises two different failure classes:
//   1. capture heartbeat stalls -> device/ADB recovery ladder;
//   2. captures continue, but the confirmed game state stops progressing ->
//      bounded game restart.
//
// Lease-owning automation tasks are excluded from generic state-stall
// recovery because each flow owns its own bounded checks. Capture-heartbeat
// recovery still runs above this gate, so a dead emulator can always recover.
func (b *Bot) runtimeSupervisorLoop() {
	ticker := time.NewTicker(runtimeSupervisorTick)
	defer ticker.Stop()

	for {
		select {
		case <-b.ctx.Done():
			return
		case <-ticker.C:
			now := time.Now()

			if b.repairAutomationLeaseInvariant() {
				b.recordActivity()
			}

			lastCaptureNano := b.captureHeartbeat.Load()
			if lastCaptureNano > 0 {
				staleFor := now.Sub(time.Unix(0, lastCaptureNano))
				if staleFor >= runtimeCaptureStaleThreshold {
					b.logger.Error().
						Dur("capture_stale_for", staleFor).
						Dur("threshold", runtimeCaptureStaleThreshold).
						Msg("runtime supervisor detected stalled capture heartbeat; starting device recovery")

					b.recoverEmulator()

					// Recovery may take time. Reset the heartbeat grace so
					// we do not immediately re-enter recovery on the next
					// supervisor tick.
					b.captureHeartbeat.Store(time.Now().UnixNano())
					continue
				}
			}

			// Lease-owning tasks have task-specific budgets. They are excluded
			// from the generic state watchdog, but are not allowed to hold the
			// UI forever. A timed-out task triggers one controlled game restart;
			// the task keeps its lease until its own flow exits, so we never
			// create overlapping clickers by force-releasing ownership.
			if b.automationTaskInFlight.Load() {
				task := b.currentAutomationTask()
				if started := b.automationTaskStarted.Load(); started > 0 {
					age := now.Sub(time.Unix(started, 0))
					timeout := automationTaskTimeout(task)
					if timeout > 0 && age >= timeout && !b.recoveryInFlight.Load() && !b.restartInFlight.Load() {
						b.automationTaskTimeouts.Add(1)
						b.logger.Error().
							Str("task", task).
							Dur("task_age", age).
							Dur("timeout", timeout).
							Msg("automation task exceeded bounded runtime; restarting game without releasing task lease")
						b.automationTaskStarted.Store(now.Unix())
						b.restartGame()
					}
				}
				continue
			}

			if b.seqRunning.Load() || b.recoveryInFlight.Load() || b.restartInFlight.Load() {
				continue
			}

			state := game.GameState(b.runtimeState.Load())

			// Windows/BlueStacks may classify the first visually usable frames
			// as Unknown while localized HUD assets and template caches settle.
			// Preserve the proven startup grace from the Windows branch, but do
			// not give Unknown the same generosity later in the session.
			if state == game.StateUnknown && time.Since(b.startedAt) < 2*time.Minute {
				continue
			}

			timeout := runtimeStateTimeout(state)
			if timeout <= 0 {
				continue
			}

			sinceNano := b.runtimeStateSince.Load()
			if sinceNano <= 0 {
				continue
			}
			stateAge := now.Sub(time.Unix(0, sinceNano))
			if stateAge < timeout {
				continue
			}

			progressAge := time.Duration(0)
			if p := b.runtimeProgress.Load(); p > 0 {
				progressAge = now.Sub(time.Unix(0, p))
			}

			b.logger.Warn().
				Str("state", state.String()).
				Dur("state_age", stateAge).
				Dur("progress_age", progressAge).
				Dur("timeout", timeout).
				Msg("runtime state stopped progressing; restarting game")

			// Throttle before invoking restart: even if the classifier keeps
			// reporting the stale pre-restart state for a few frames, the
			// supervisor will not fire again immediately.
			b.runtimeStateSince.Store(now.UnixNano())
			b.restartGame()
		}
	}
}
