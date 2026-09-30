package bot

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"time"

	"gocv.io/x/gocv"
)

// FrameBroker is the single runtime fan-out point for screenshots.
//
// Only captureLoop talks to ADB for runtime screenshots. Consumers clone the
// latest broker frame instead of starting their own screencap stream. That
// removes concurrent framebuffer pressure from navigation/search/deploy code
// while keeping every consumer on a recent image.
type FrameBroker struct {
	mu      sync.RWMutex
	frame   gocv.Mat
	at      time.Time
	seq     uint64
	notify  chan struct{}
	closed  bool

	published atomic.Uint64
	waiters   atomic.Int64
}

func NewFrameBroker() *FrameBroker {
	return &FrameBroker{notify: make(chan struct{})}
}

func (b *FrameBroker) Publish(frame gocv.Mat, at time.Time) {
	if frame.Empty() {
		return
	}
	clone := frame.Clone()
	if clone.Empty() {
		clone.Close()
		return
	}

	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		clone.Close()
		return
	}
	old := b.frame
	b.frame = clone
	b.at = at
	b.seq++
	oldNotify := b.notify
	b.notify = make(chan struct{})
	close(oldNotify)
	b.mu.Unlock()

	if !old.Empty() {
		old.Close()
	}
	b.published.Add(1)
}

func (b *FrameBroker) Snapshot(maxAge time.Duration) (gocv.Mat, time.Time, uint64, bool) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	if b.closed || b.frame.Empty() {
		return gocv.NewMat(), time.Time{}, 0, false
	}
	if maxAge > 0 && time.Since(b.at) > maxAge {
		return gocv.NewMat(), b.at, b.seq, false
	}
	return b.frame.Clone(), b.at, b.seq, true
}

// WaitAfter returns a frame newer than afterSeq. maxAge is applied after wake
// so callers never consume a frame that became stale while they were waiting.
func (b *FrameBroker) WaitAfter(ctx context.Context, afterSeq uint64, timeout, maxAge time.Duration) (gocv.Mat, time.Time, uint64, error) {
	if ctx == nil {
		return gocv.NewMat(), time.Time{}, 0, errors.New("nil frame context")
	}

	deadline := time.NewTimer(timeout)
	defer deadline.Stop()

	b.waiters.Add(1)
	defer b.waiters.Add(-1)

	for {
		b.mu.RLock()
		if b.closed {
			b.mu.RUnlock()
			return gocv.NewMat(), time.Time{}, 0, errors.New("frame broker closed")
		}
		if b.seq > afterSeq && !b.frame.Empty() && (maxAge <= 0 || time.Since(b.at) <= maxAge) {
			mat := b.frame.Clone()
			at := b.at
			seq := b.seq
			b.mu.RUnlock()
			return mat, at, seq, nil
		}
		notify := b.notify
		b.mu.RUnlock()

		select {
		case <-ctx.Done():
			return gocv.NewMat(), time.Time{}, 0, ctx.Err()
		case <-deadline.C:
			return gocv.NewMat(), time.Time{}, 0, errors.New("frame wait timeout")
		case <-notify:
		}
	}
}

func (b *FrameBroker) Stats() (published uint64, waiters int64) {
	return b.published.Load(), b.waiters.Load()
}

func (b *FrameBroker) Close() {
	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		return
	}
	b.closed = true
	old := b.frame
	b.frame = gocv.NewMat()
	close(b.notify)
	b.notify = make(chan struct{})
	b.mu.Unlock()

	if !old.Empty() {
		old.Close()
	}
}
