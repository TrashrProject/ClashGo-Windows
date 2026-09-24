package bot

import (
	"time"
)

const (
	runtimeSupervisorTick         = 10 * time.Second
	runtimeCaptureStaleThreshold  = 45 * time.Second
)

// runtimeSupervisorLoop is deliberately independent from captureLoop.
//
// The in-loop stuck watchdog only runs when frames are still arriving.
// If CaptureToMat, the transport, or the capture goroutine wedges hard,
// that watchdog never gets another chance to execute. This supervisor
// watches an atomic heartbeat updated by the capture goroutine and starts
// the existing device recovery ladder when the heartbeat goes stale.
//
// recoveryInFlight lives on Bot and is shared with recoverEmulator so the
// supervisor and frame watchdog cannot launch overlapping ADB/BlueStacks
// recovery sequences.
func (b *Bot) runtimeSupervisorLoop() {
	ticker := time.NewTicker(runtimeSupervisorTick)
	defer ticker.Stop()

	for {
		select {
		case <-b.ctx.Done():
			return
		case <-ticker.C:
			lastNano := b.captureHeartbeat.Load()
			if lastNano <= 0 {
				continue
			}

			staleFor := time.Since(time.Unix(0, lastNano))
			if staleFor < runtimeCaptureStaleThreshold {
				continue
			}

			b.logger.Error().
				Dur("capture_stale_for", staleFor).
				Dur("threshold", runtimeCaptureStaleThreshold).
				Msg("runtime supervisor detected stalled capture heartbeat; starting recovery")

			b.recoverEmulator()

			// Give the recovered capture loop a fresh grace window. If the
			// recovery failed, another supervisor pass will escalate again
			// after the threshold instead of hammering ADB every tick.
			b.captureHeartbeat.Store(time.Now().UnixNano())
		}
	}
}
