package attack

import (
	"sync"
	"sync/atomic"
	"time"

	"github.com/rs/zerolog"
)

// DeployScheduler is the single execution gate for deployment actions.
//
// The attack planner may prepare many unit operations up front, but only one
// deployment transaction is allowed to own input at a time. A transaction is
// intentionally larger than a single tap: select-card + settle + troop/spell
// placement + one bounded verification all stay together, so no other action
// can steal the selected card between those steps.
type DeployScheduler struct {
	mu       sync.Mutex
	logger   zerolog.Logger
	sequence atomic.Uint64
	totalUS  atomic.Int64
}

func NewDeployScheduler(logger zerolog.Logger) *DeployScheduler {
	return &DeployScheduler{logger: logger.With().Str("component", "deploy_scheduler").Logger()}
}

func (s *DeployScheduler) Run(label string, fn func()) {
	if s == nil || fn == nil {
		if fn != nil {
			fn()
		}
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	id := s.sequence.Add(1)
	started := time.Now()
	s.logger.Debug().Uint64("op", id).Str("action", label).Msg("deployment action start")
	fn()
	dur := time.Since(started)
	s.totalUS.Add(dur.Microseconds())
	s.logger.Debug().
		Uint64("op", id).
		Str("action", label).
		Dur("duration", dur).
		Msg("deployment action complete")
}

func (s *DeployScheduler) Stats() (operations uint64, total time.Duration) {
	if s == nil {
		return 0, 0
	}
	return s.sequence.Load(), time.Duration(s.totalUS.Load()) * time.Microsecond
}
