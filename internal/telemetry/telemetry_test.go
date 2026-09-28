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
