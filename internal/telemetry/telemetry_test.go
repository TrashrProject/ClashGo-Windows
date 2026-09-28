package telemetry

import (
	"path/filepath"
	"testing"
)

func TestSnapshotCounters(t *testing.T) {
	b := New(filepath.Join(t.TempDir(), "events.ndjson"))
	defer b.Close()

	b.Emit(EventSearchStarted, nil)
	b.Emit(EventTargetFound, nil)
	b.Emit(EventTargetSkipped, nil)
	b.Emit(EventAttackFinished, nil)
	b.Emit(EventRecovery, nil)
	b.Emit(EventCaptureSample, map[string]any{"duration_us": int64(1500)})
	b.Emit(EventCaptureSample, map[string]any{"duration_us": int64(2500)})

	s := b.Snapshot()
	if s.Searches != 1 || s.TargetsFound != 1 || s.TargetsSkipped != 1 || s.AttacksFinished != 1 || s.Recoveries != 1 {
		t.Fatalf("unexpected counters: %+v", s)
	}
	if s.AvgCaptureMS != 2.0 {
		t.Fatalf("avg capture ms = %v, want 2.0", s.AvgCaptureMS)
	}
	if s.LastCaptureMS != 2.5 {
		t.Fatalf("last capture ms = %v, want 2.5", s.LastCaptureMS)
	}
}

func TestRecentExcludesCaptureSamplesAndReturnsNewestFirst(t *testing.T) {
	b := New(filepath.Join(t.TempDir(), "events.ndjson"))
	defer b.Close()

	b.Emit(EventSearchStarted, map[string]any{"step": 1})
	b.Emit(EventCaptureSample, map[string]any{"duration_us": int64(900)})
	b.Emit(EventTargetFound, map[string]any{"step": 2})
	b.Emit(EventAttackStarted, map[string]any{"step": 3})

	recent := b.Recent(2)
	if len(recent) != 2 {
		t.Fatalf("recent len=%d, want 2", len(recent))
	}
	if recent[0].Type != EventAttackStarted || recent[1].Type != EventTargetFound {
		t.Fatalf("unexpected recent order: %+v", recent)
	}
	for _, ev := range recent {
		if ev.Type == EventCaptureSample {
			t.Fatal("capture samples must never enter activity feed")
		}
	}
}
