package attack

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/rs/zerolog"
)

func TestDeploySchedulerSerializesTransactions(t *testing.T) {
	s := NewDeployScheduler(zerolog.Nop())
	var active atomic.Int32
	var maxActive atomic.Int32
	var wg sync.WaitGroup

	for i := 0; i < 6; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			s.Run("test", func() {
				cur := active.Add(1)
				for {
					prev := maxActive.Load()
					if cur <= prev || maxActive.CompareAndSwap(prev, cur) {
						break
					}
				}
				time.Sleep(5 * time.Millisecond)
				active.Add(-1)
			})
		}()
	}
	wg.Wait()

	if got := maxActive.Load(); got != 1 {
		t.Fatalf("scheduler allowed %d concurrent transactions, want 1", got)
	}
	if ops, _ := s.Stats(); ops != 6 {
		t.Fatalf("operations=%d, want 6", ops)
	}
}
