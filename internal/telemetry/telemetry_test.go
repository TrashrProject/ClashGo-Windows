package telemetry

import (
	"bytes"
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
	b.Emit(EventAnomaly, map[string]any{"kind": "slow_next_transition"})
	b.Emit(EventCaptureSample, map[string]any{"duration_us": int64(1500)})
	b.Emit(EventCaptureSample, map[string]any{"duration_us": int64(2500)})

	s := b.Snapshot()
	if s.Searches != 1 || s.TargetsFound != 1 || s.TargetsSkipped != 1 || s.AttacksFinished != 1 || s.Recoveries != 1 || s.Anomalies != 1 {
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

func TestTargetQualitySeparatesAcceptedAndRejected(t *testing.T) {
	b := New(filepath.Join(t.TempDir(), "events.ndjson"))
	defer b.Close()

	b.Emit(EventTargetFound, map[string]any{
		"accept": true, "gold": 900000, "elixir": 800000, "de": 4000, "score": 88,
	})
	b.Emit(EventTargetFound, map[string]any{
		"accept": true, "gold": 1100000, "elixir": 1000000, "de": 6000, "score": 92,
	})
	b.Emit(EventTargetFound, map[string]any{
		"accept": false, "gold": 400000, "elixir": 300000, "de": 1000, "score": 51,
	})
	b.Emit(EventTargetFound, map[string]any{
		"accept": false, "gold": 600000, "elixir": 500000, "de": 2000, "score": 61,
	})

	s := b.Snapshot()
	if s.TargetsFound != 4 || s.TargetsAccepted != 2 {
		t.Fatalf("unexpected target counters: %+v", s)
	}
	if s.AvgAcceptedGE != 1_900_000 {
		t.Fatalf("avg accepted G+E=%v want 1900000", s.AvgAcceptedGE)
	}
	if s.AvgRejectedGE != 900_000 {
		t.Fatalf("avg rejected G+E=%v want 900000", s.AvgRejectedGE)
	}
	if s.AvgAcceptedDE != 5000 || s.AvgRejectedDE != 1500 {
		t.Fatalf("unexpected DE averages: accepted=%v rejected=%v", s.AvgAcceptedDE, s.AvgRejectedDE)
	}
	if s.AvgAcceptedScore != 90 || s.AvgRejectedScore != 56 {
		t.Fatalf("unexpected score averages: accepted=%v rejected=%v", s.AvgAcceptedScore, s.AvgRejectedScore)
	}
}

func TestRotateJournalIfOversizeKeepsSingleBackup(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "events.ndjson")
	if err := os.WriteFile(path, bytes.Repeat([]byte("x"), 2048), 0o644); err != nil {
		t.Fatal(err)
	}

	rotateJournalIfOversize(path, 1024)

	if _, err := os.Stat(path + ".1"); err != nil {
		t.Fatalf("expected rotated backup: %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("expected original path moved away, err=%v", err)
	}

	if err := os.WriteFile(path, bytes.Repeat([]byte("y"), 3072), 0o644); err != nil {
		t.Fatal(err)
	}
	rotateJournalIfOversize(path, 1024)
	data, err := os.ReadFile(path + ".1")
	if err != nil {
		t.Fatal(err)
	}
	if len(data) != 3072 {
		t.Fatalf("backup size=%d want newest 3072", len(data))
	}
}

func TestPruneIncidentDirKeepsNewestFiles(t *testing.T) {
	dir := t.TempDir()
	names := []string{
		"20260101T000000.000000001Z_a.json",
		"20260101T000000.000000002Z_b.json",
		"20260101T000000.000000003Z_c.json",
		"20260101T000000.000000004Z_d.json",
	}
	for _, name := range names {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(name), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	pruneIncidentDir(dir, 2)

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		t.Fatalf("remaining=%d want 2", len(entries))
	}
	if entries[0].Name() != names[2] || entries[1].Name() != names[3] {
		t.Fatalf("kept wrong incidents: %q %q", entries[0].Name(), entries[1].Name())
	}
}

func TestNextEfficiencyAggregatesRetriesAndProbes(t *testing.T) {
	b := New(filepath.Join(t.TempDir(), "events.ndjson"))
	defer b.Close()

	b.Emit(EventTargetSkipped, map[string]any{
		"transition_us": int64(1_000_000),
		"retry_used": false,
		"verify_probes": 1,
	})
	b.Emit(EventTargetSkipped, map[string]any{
		"transition_us": int64(2_000_000),
		"retry_used": false,
		"verify_probes": 2,
	})
	b.Emit(EventTargetSkipped, map[string]any{
		"transition_us": int64(4_000_000),
		"retry_used": true,
		"verify_probes": 5,
	})

	s := b.Snapshot()
	if s.NextTransitions != 3 || s.NextRetries != 1 {
		t.Fatalf("unexpected Next counters: %+v", s)
	}
	wantRate := 200.0 / 3.0
	if diff := s.NextFirstPassRate - wantRate; diff < -0.0001 || diff > 0.0001 {
		t.Fatalf("first-pass rate=%v want %v", s.NextFirstPassRate, wantRate)
	}
	wantProbes := 8.0 / 3.0
	if diff := s.AvgNextVerifyProbes - wantProbes; diff < -0.0001 || diff > 0.0001 {
		t.Fatalf("avg probes=%v want %v", s.AvgNextVerifyProbes, wantProbes)
	}
	if s.AvgNextTransitionMS != (1000.0+2000.0+4000.0)/3.0 {
		t.Fatalf("avg transition=%v", s.AvgNextTransitionMS)
	}
}

func TestRecordCaptureMicrosMatchesCaptureEventMetrics(t *testing.T) {
	b := New(filepath.Join(t.TempDir(), "events.ndjson"))
	defer b.Close()

	b.RecordCaptureMicros(1500)
	b.RecordCaptureMicros(2500)

	s := b.Snapshot()
	if s.AvgCaptureMS != 2.0 || s.LastCaptureMS != 2.5 {
		t.Fatalf("direct capture metrics mismatch: %+v", s)
	}
	if s.Events != 2 {
		t.Fatalf("events=%d want 2", s.Events)
	}
	if len(b.Recent(10)) != 0 {
		t.Fatal("direct capture samples must not enter activity feed")
	}
}

func TestRecordRejectedTargetMatchesRejectedEventMetrics(t *testing.T) {
	b := New(filepath.Join(t.TempDir(), "events.ndjson"))
	defer b.Close()

	b.RecordRejectedTarget(400000, 300000, 1200, 52, 12500, 750000, 750000, 2000)
	b.RecordRejectedTarget(600000, 500000, 1800, 62, 17500, 750000, 750000, 2000)

	s := b.Snapshot()
	if s.TargetsFound != 2 || s.TargetsAccepted != 0 {
		t.Fatalf("target counters mismatch: %+v", s)
	}
	if s.AvgRejectedGE != 900000 || s.AvgRejectedDE != 1500 || s.AvgRejectedScore != 57 {
		t.Fatalf("rejected quality mismatch: %+v", s)
	}
	if s.AvgTargetScanMS != 15 || s.LastTargetScanMS != 17.5 {
		t.Fatalf("scan metrics mismatch: %+v", s)
	}
	if len(b.Recent(10)) != 0 {
		t.Fatal("rejected target must not enter activity feed")
	}
}

func TestTopRejectedTargetsKeepsBestFiveInMemory(t *testing.T) {
	b := New(filepath.Join(t.TempDir(), "events.ndjson"))
	defer b.Close()

	cases := []struct{
		g, e, de, score int
	}{
		{300000, 300000, 1000, 50},
		{700000, 700000, 3000, 75},
		{900000, 800000, 4000, 88},
		{1000000, 900000, 5000, 92},
		{600000, 600000, 2500, 70},
		{1200000, 1100000, 6000, 95},
		{950000, 950000, 4500, 92},
	}
	for _, tc := range cases {
		b.RecordRejectedTarget(tc.g, tc.e, tc.de, tc.score, 1000, 750000, 750000, 2000)
	}

	s := b.Snapshot()
	if len(s.TopRejectedTargets) != 5 {
		t.Fatalf("top rejected len=%d want 5: %+v", len(s.TopRejectedTargets), s.TopRejectedTargets)
	}
	if s.TopRejectedTargets[0].Score != 95 {
		t.Fatalf("best rejected target not first: %+v", s.TopRejectedTargets)
	}
	if s.TopRejectedTargets[1].Score != 92 || s.TopRejectedTargets[2].Score != 92 {
		t.Fatalf("score ordering wrong: %+v", s.TopRejectedTargets)
	}
	ge1 := s.TopRejectedTargets[1].Gold + s.TopRejectedTargets[1].Elixir
	ge2 := s.TopRejectedTargets[2].Gold + s.TopRejectedTargets[2].Elixir
	if ge1 < ge2 {
		t.Fatalf("same-score targets must prefer higher G+E: %+v", s.TopRejectedTargets)
	}
}

func TestTargetThresholdGapPct(t *testing.T) {
	if got := targetThresholdGapPct(700000, 700000, 1900, 750000, 750000, 2000); got < 6.6 || got > 6.7 {
		t.Fatalf("near-miss gap=%v want ~6.67", got)
	}
	if got := targetThresholdGapPct(750000, 750000, 2000, 750000, 750000, 2000); got != 0 {
		t.Fatalf("met thresholds gap=%v want 0", got)
	}
	if got := targetThresholdGapPct(100000, 750000, 2000, 750000, 750000, 2000); got < 86 {
		t.Fatalf("large miss gap=%v want >86", got)
	}
}

func TestRejectedNearMissCounter(t *testing.T) {
	b := New(filepath.Join(t.TempDir(), "events.ndjson"))
	defer b.Close()

	// ~6.67% gap -> counts in 10% and 15%, not 5%.
	b.RecordRejectedTarget(700000, 700000, 1900, 82, 1000, 750000, 750000, 2000)
	// ~2.67% gap -> counts in all sensitivity buckets.
	b.RecordRejectedTarget(730000, 730000, 1960, 86, 1000, 750000, 750000, 2000)
	// Large miss -> none.
	b.RecordRejectedTarget(300000, 300000, 500, 40, 1000, 750000, 750000, 2000)

	s := b.Snapshot()
	if s.NearMissTargets != 2 {
		t.Fatalf("near misses=%d want 2", s.NearMissTargets)
	}
	if s.NearMiss5Targets != 1 || s.NearMiss10Targets != 2 || s.NearMiss15Targets != 2 {
		t.Fatalf("unexpected sensitivity buckets: 5=%d 10=%d 15=%d",
			s.NearMiss5Targets, s.NearMiss10Targets, s.NearMiss15Targets)
	}
	if len(s.TopRejectedTargets) == 0 || !s.TopRejectedTargets[0].NearMiss {
		t.Fatalf("expected best rejected target to expose near-miss metadata: %+v", s.TopRejectedTargets)
	}
}
