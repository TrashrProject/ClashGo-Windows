package adb

import (
	"errors"
	"sync"
	"testing"
	"time"
)

func TestHealthCounters(t *testing.T) {
	var h Health
	h.RecordSuccess(10 * time.Millisecond)
	h.RecordSuccess(20 * time.Millisecond)
	h.RecordFailure(errors.New("boom"))

	if h.CapturesTotal != 2 {
		t.Fatalf("captures_total=%d want 2", h.CapturesTotal)
	}
	if h.ErrorsTotal != 1 {
		t.Fatalf("errors_total=%d want 1", h.ErrorsTotal)
	}
	if h.ConsecutiveFails != 1 {
		t.Fatalf("consecutive_fails=%d want 1", h.ConsecutiveFails)
	}
	if h.LastError != "boom" {
		t.Fatalf("last_error=%q want boom", h.LastError)
	}
	if h.AvgCaptureMs <= 0 {
		t.Fatalf("avg_capture_ms=%f want >0", h.AvgCaptureMs)
	}
}

func TestClientHealthConcurrentSnapshots(t *testing.T) {
	c := NewClient()
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			if i%5 == 0 {
				c.recordHealthFailure(errors.New("x"))
				return
			}
			c.recordHealthSuccess(time.Millisecond)
			_ = c.Health()
		}(i)
	}
	wg.Wait()

	got := c.Health()
	if got.CapturesTotal+got.ErrorsTotal != 20 {
		t.Fatalf("total events=%d want 20", got.CapturesTotal+got.ErrorsTotal)
	}
}


func TestCaptureGapForFailures(t *testing.T) {
	base := 180 * time.Millisecond
	cases := []struct {
		fails int
		want  time.Duration
	}{
		{fails: 0, want: 180 * time.Millisecond},
		{fails: 1, want: 360 * time.Millisecond},
		{fails: 2, want: 720 * time.Millisecond},
		{fails: 3, want: 1440 * time.Millisecond},
		{fails: 4, want: 1440 * time.Millisecond},
		{fails: 20, want: 1440 * time.Millisecond},
		{fails: -1, want: 180 * time.Millisecond},
	}
	for _, tc := range cases {
		if got := captureGapForFailures(base, tc.fails); got != tc.want {
			t.Fatalf("fails=%d gap=%v want %v", tc.fails, got, tc.want)
		}
	}
	if got := captureGapForFailures(0, 3); got != 0 {
		t.Fatalf("zero base gap=%v want 0", got)
	}
}
