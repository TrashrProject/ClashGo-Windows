package bot

import (
	"fmt"
	"time"

	"gocv.io/x/gocv"
)

// runtimeFrame returns the next broker frame while the runtime capture loop is
// active. Before Start (e.g. QuickDeploy/tests) it falls back to one direct
// capture so those explicit single-shot paths keep working.
//
// During normal automation this function never starts a second screencap stream.
func (b *Bot) runtimeFrame(timeout, maxAge time.Duration) (gocv.Mat, error) {
	if b == nil || b.client == nil {
		return gocv.NewMat(), fmt.Errorf("bot/client unavailable")
	}
	if timeout <= 0 {
		timeout = 2 * time.Second
	}
	if maxAge <= 0 {
		maxAge = 2 * time.Second
	}

	if !b.brokerActive.Load() || b.frameBroker == nil {
		return b.client.CaptureToMat()
	}

	after := b.frameSeq.Load()
	if snap, _, seq, ok := b.frameBroker.Snapshot(maxAge); ok && seq > after {
		b.frameSeq.Store(seq)
		return snap, nil
	}

	frame, _, seq, err := b.frameBroker.WaitAfter(b.ctx, after, timeout, maxAge)
	if err != nil {
		return gocv.NewMat(), err
	}
	b.frameSeq.Store(seq)
	return frame, nil
}

// runtimeFrameFresh is the common attack/search helper: wait for a frame that
// was captured recently enough to make a UI decision.
func (b *Bot) runtimeFrameFresh(timeout time.Duration) (gocv.Mat, error) {
	maxAge := 1 * time.Second
	switch RuntimePhase(b.runtimePhase.Load()) {
	case PhaseAttackNavigation:
		maxAge = 700 * time.Millisecond
	case PhaseSearching:
		maxAge = 1200 * time.Millisecond
	case PhasePlanning:
		maxAge = 500 * time.Millisecond
	case PhaseDeploying:
		maxAge = 550 * time.Millisecond
	case PhaseBattle, PhaseParsingResult, PhaseReturningHome:
		maxAge = 1200 * time.Millisecond
	}
	return b.runtimeFrame(timeout, maxAge)
}
