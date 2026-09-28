package telemetry

import (
	"encoding/json"
	"os"
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

func TestWriteIncidentCreatesCompactSnapshot(t *testing.T) {
	dir := t.TempDir()
	b := New(filepath.Join(dir, "events.ndjson"))

	b.Emit(EventSearchStarted, nil)
	b.Emit(EventTargetFound, map[string]any{"gold": 900000, "accept": true})
	b.Emit(EventCaptureSample, map[string]any{"duration_us": int64(1200)})

	path := b.WriteIncident("deployment failed")
	b.Close()

	if path == "" {
		t.Fatal("expected incident path")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read incident: %v", err)
	}
	var incident Incident
	if err := json.Unmarshal(data, &incident); err != nil {
		t.Fatalf("parse incident: %v", err)
	}
	if incident.Reason != "deployment failed" {
		t.Fatalf("reason=%q", incident.Reason)
	}
	if len(incident.Recent) != 2 {
		t.Fatalf("recent events=%d, want 2 high-level events", len(incident.Recent))
	}
	if incident.Metrics.LastCaptureMS != 1.2 {
		t.Fatalf("last capture=%vms, want 1.2", incident.Metrics.LastCaptureMS)
	}
}

func TestRecentExcludesRejectedTargetsAndSkips(t *testing.T) {
	b := New(filepath.Join(t.TempDir(), "events.ndjson"))
	defer b.Close()

	b.Emit(EventTargetFound, map[string]any{"accept": false, "gold": 100000})
	b.Emit(EventTargetSkipped, map[string]any{"sequence_skips": 1})
	b.Emit(EventTargetFound, map[string]any{"accept": true, "gold": 900000})

	recent := b.Recent(10)
	if len(recent) != 1 {
		t.Fatalf("recent len=%d, want only accepted target", len(recent))
	}
	if recent[0].Type != EventTargetFound || recent[0].Fields["accept"] != true {
		t.Fatalf("unexpected recent event: %+v", recent[0])
	}
	s := b.Snapshot()
	if s.TargetsFound != 2 || s.TargetsSkipped != 1 {
		t.Fatalf("memory counters lost rejected/skip samples: %+v", s)
	}
}

func TestTargetScanLatencyAggregatesInMemory(t *testing.T) {
	b := New(filepath.Join(t.TempDir(), "events.ndjson"))
	defer b.Close()

	b.Emit(EventTargetFound, map[string]any{"accept": false, "scan_us": int64(12000)})
	b.Emit(EventTargetFound, map[string]any{"accept": true, "scan_us": int64(18000)})

	s := b.Snapshot()
	if s.AvgTargetScanMS != 15 {
		t.Fatalf("avg target scan=%vms, want 15ms", s.AvgTargetScanMS)
	}
	if s.LastTargetScanMS != 18 {
		t.Fatalf("last target scan=%vms, want 18ms", s.LastTargetScanMS)
	}
}
