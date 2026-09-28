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
	if h.FastCaptureMs <= 0 {
		t.Fatalf("fast_capture_ms=%f want >0", h.FastCaptureMs)
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

func TestHealthFastCaptureReactsFasterThanStableAverage(t *testing.T) {
	var h Health
	for i := 0; i < 10; i++ {
		h.RecordSuccess(300 * time.Millisecond)
	}
	beforeAvg := h.AvgCaptureMs
	beforeFast := h.FastCaptureMs

	h.RecordSuccess(1500 * time.Millisecond)

	if h.FastCaptureMs <= h.AvgCaptureMs {
		t.Fatalf("fast EWMA=%f should react above stable average=%f", h.FastCaptureMs, h.AvgCaptureMs)
	}
	if h.FastCaptureMs-beforeFast <= h.AvgCaptureMs-beforeAvg {
		t.Fatalf("fast EWMA did not react more strongly: fast delta=%f avg delta=%f",
			h.FastCaptureMs-beforeFast, h.AvgCaptureMs-beforeAvg)
	}
}

func TestHealthTapLatencyEWMA(t *testing.T) {
	var h Health
	h.RecordTap(200 * time.Millisecond)
	h.RecordTap(100 * time.Millisecond)

	if h.TapsTotal != 2 {
		t.Fatalf("taps_total=%d want 2", h.TapsTotal)
	}
	if h.AvgTapMs <= 0 || h.FastTapMs <= 0 {
		t.Fatalf("tap latency metrics must be >0: avg=%f fast=%f", h.AvgTapMs, h.FastTapMs)
	}
}

func TestHealthFastTapReactsFasterThanStableAverage(t *testing.T) {
	var h Health
	for i := 0; i < 10; i++ {
		h.RecordTap(80 * time.Millisecond)
	}
	beforeAvg := h.AvgTapMs
	beforeFast := h.FastTapMs

	h.RecordTap(400 * time.Millisecond)

	if h.FastTapMs <= h.AvgTapMs {
		t.Fatalf("fast tap EWMA=%f should react above stable avg=%f", h.FastTapMs, h.AvgTapMs)
	}
	if h.FastTapMs-beforeFast <= h.AvgTapMs-beforeAvg {
		t.Fatalf("fast tap EWMA did not react more strongly: fast delta=%f avg delta=%f",
			h.FastTapMs-beforeFast, h.AvgTapMs-beforeAvg)
	}
}

func TestHealthTapRouteCounters(t *testing.T) {
	var h Health
	h.RecordTapRoute(true)
	h.RecordTapRoute(false)
	h.RecordTapRoute(false)

	if h.PipeTapsTotal != 1 {
		t.Fatalf("pipe_taps_total=%d want 1", h.PipeTapsTotal)
	}
	if h.LegacyTapsTotal != 2 {
		t.Fatalf("legacy_taps_total=%d want 2", h.LegacyTapsTotal)
	}
}
