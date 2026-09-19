package adb

import (
	"errors"
	"sync"
	"testing"
	"time"
)

func TestHealthCountersAndSnapshot(t *testing.T) {
	var h Health
	h.RecordSuccess(10 * time.Millisecond)
	h.RecordSuccess(20 * time.Millisecond)
	h.RecordFailure(errors.New("boom"))

	got := h.Snapshot()
	if got.CapturesTotal != 2 {
		t.Fatalf("captures_total=%d want 2", got.CapturesTotal)
	}
	if got.ErrorsTotal != 1 {
		t.Fatalf("errors_total=%d want 1", got.ErrorsTotal)
	}
	if got.ConsecutiveFails != 1 {
		t.Fatalf("consecutive_fails=%d want 1", got.ConsecutiveFails)
	}
	if got.LastError != "boom" {
		t.Fatalf("last_error=%q want boom", got.LastError)
	}
	if got.AvgCaptureMs <= 0 {
		t.Fatalf("avg_capture_ms=%f want >0", got.AvgCaptureMs)
	}
}

func TestHealthConcurrentSnapshots(t *testing.T) {
	var h Health
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			if i%5 == 0 {
				h.RecordFailure(errors.New("x"))
				return
			}
			h.RecordSuccess(time.Millisecond)
			_ = h.Snapshot()
		}(i)
	}
	wg.Wait()

	got := h.Snapshot()
	if got.CapturesTotal+got.ErrorsTotal != 20 {
		t.Fatalf("total events=%d want 20", got.CapturesTotal+got.ErrorsTotal)
	}
}
