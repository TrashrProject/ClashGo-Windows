package main

import (
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/Ducky705/ClashGO/internal/bot"
	"github.com/Ducky705/ClashGO/internal/config"
	"github.com/Ducky705/ClashGO/internal/paths"
	"github.com/Ducky705/ClashGO/internal/telemetry"
)

func TestApp_GetConfig(t *testing.T) {
	a := NewApp()
	cfg := a.GetConfig()
	if cfg == nil {
		t.Error("GetConfig returned nil")
	}
}

func TestApp_GetStats(t *testing.T) {
	a := NewApp()
	stats := a.GetStats()
	if stats.AttacksCompleted != 0 {
		t.Errorf("Expected 0 attacks, got %d", stats.AttacksCompleted)
	}
}

func TestApp_GetAttackHistory(t *testing.T) {
	a := NewApp()
	history := a.GetAttackHistory()
	if history == nil {
		t.Error("GetAttackHistory returned nil")
	}
}

func TestApp_GetActivityUsesStoppedSessionCache(t *testing.T) {
	a := &App{
		lastActivity: []telemetry.Event{
			{Type: telemetry.EventSessionComplete},
		},
	}

	got := a.GetActivity()
	if len(got) != 1 || got[0].Type != telemetry.EventSessionComplete {
		t.Fatalf("cached stopped activity=%+v", got)
	}

	a.clearInMemoryMemberRuntimeState()
	if got := a.GetActivity(); len(got) != 0 {
		t.Fatalf("member switch should clear cached activity, got=%+v", got)
	}
}

func TestApp_GetStrategies(t *testing.T) {
	a := NewApp()
	strats := a.GetStrategies()
	if strats == nil {
		t.Error("GetStrategies returned nil")
	}
}

// TestApp_RefreshHistoryReReadsWarmCache is the regression test for the
// "latest attack never shows in history" bug. The App's cachedHistory
// is warmed by React's very first GetAttackHistory poll; after that,
// refreshHistory (fired per-attack from OnStatsUpdate) must STILL
// re-read attack_history.json from disk. Previously it delegated to
// ensureHistoryLoadedLocked, which no-ops on a warm cache — freezing
// the UI on the launch-time snapshot while loot totals kept climbing.
//
// It also guards the ordering contract: the bot writes the report to
// disk before firing OnStatsUpdate, so the re-read must see it.
func TestApp_RefreshHistoryReReadsWarmCache(t *testing.T) {
	// Redirect all state writes to a throwaway dir so the test never
	// touches the user's real config dir (or the wails dev watcher).
	t.Setenv("CLASHGO_CONFIG_DIR", t.TempDir())

	a := NewApp()
	histPath := paths.ResolveConfig("attack_history.json")

	writeHist := func(reps []bot.AttackReport) {
		t.Helper()
		data, err := json.Marshal(reps)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(histPath, data, 0644); err != nil {
			t.Fatal(err)
		}
	}

	// Prime the disk with one attack, then warm the App's cache via a
	// GetAttackHistory poll (same cold-start poll React fires at launch).
	writeHist([]bot.AttackReport{{Timestamp: "first", Stars: 3}})
	if got := a.GetAttackHistory(); len(got) != 1 {
		t.Fatalf("warm-up: want 1 history entry, got %d", len(got))
	}

	// Simulate a second attack completing: the bot appends the new
	// report to disk and fires OnStatsUpdate → refreshHistory.
	writeHist([]bot.AttackReport{
		{Timestamp: "second", Stars: 2},
		{Timestamp: "first", Stars: 3},
	})
	a.refreshHistory()

	got := a.GetAttackHistory()
	if len(got) != 2 {
		t.Fatalf("refreshHistory did not re-read disk: want 2 entries, got %d (cache is stale)", len(got))
	}
	if got[0].Timestamp != "second" {
		t.Errorf("latest attack missing from history head: got %+v", got)
	}
}


func TestClashAccountServiceURLPrecedence(t *testing.T) {
	oldEmbedded := accountServiceURL
	defer func() { accountServiceURL = oldEmbedded }()

	cfg := config.DefaultConfig()
	cfg.Account.ProxyURL = "https://config.example/"

	accountServiceURL = "https://embedded.example/"
	t.Setenv("CLASHGO_ACCOUNT_API_URL", "")
	if got := clashAccountServiceURL(cfg); got != "https://embedded.example" {
		t.Fatalf("embedded service URL=%q", got)
	}

	t.Setenv("CLASHGO_ACCOUNT_API_URL", "https://env.example/")
	if got := clashAccountServiceURL(cfg); got != "https://env.example" {
		t.Fatalf("env service URL should win, got %q", got)
	}

	t.Setenv("CLASHGO_ACCOUNT_API_URL", "")
	accountServiceURL = ""
	if got := clashAccountServiceURL(cfg); got != "https://config.example" {
		t.Fatalf("config proxy URL fallback=%q", got)
	}
}

func TestMergeStatsPreservesIntelligenceV2Metrics(t *testing.T) {
	acc := bot.BotStats{
		AttacksCompleted: 2,
		TotalGold:        2_000_000,
		TotalElixir:      1_000_000,
		TotalDE:          10_000,
		Stars3:           1,
		Stars2:           1,
		Uptime:           30 * time.Minute,
		RecoveryAttempts: 1,
		RecoverySuccesses: 1,
	}
	current := bot.BotStats{
		AttacksCompleted:        2,
		TotalGold:               1_000_000,
		TotalElixir:             1_000_000,
		TotalDE:                 5_000,
		Stars3:                  2,
		Uptime:                  30 * time.Minute,
		RecoveryAttempts:        1,
		RecoverySuccesses:       0,
		AverageCaptureMS:        321,
		AverageTargetScanMS:     42,
		AverageReturnHomeMS:     880,
		AverageNextTransitionMS: 735,
		HealthScore:             94,
		SpeedProfile:            "Fast",
		Anomalies:               2,
		TargetsSeen:             12,
		TargetsAccepted:         2,
		TargetAcceptanceRate:    16.7,
		AvgSkipsPerAttack:       5,
		AvgAcceptedGE:           1_900_000,
		AvgRejectedGE:           850_000,
		AvgAcceptedDE:           4_500,
		AvgRejectedDE:           1_200,
		AvgAcceptedScore:        91,
		AvgRejectedScore:        57,
		PreferredScaleAttempts:  20,
		PreferredScaleHits:      15,
		PreferredScaleFallbacks: 5,
		PreferredScaleHitRate:   75,
		PreferredScaleEnabled:   true,
		UIAnchorAttempts:        24,
		UIAnchorHits:            20,
		UIAnchorFallbacks:       4,
		UIAnchorHitRate:         83.33,
		UIAnchorEnabled:         true,
		NearMissTargets:         7,
		NearMiss5Targets:        2,
		NearMiss10Targets:       5,
		NearMiss15Targets:       7,
	}

	got := mergeStats(acc, current)

	if got.AttacksCompleted != 4 || got.TotalGold != 3_000_000 || got.TotalElixir != 2_000_000 {
		t.Fatalf("additive counters not merged: %+v", got)
	}
	if got.SessionAttacks != 2 || got.SessionAttackCap != 10 {
		t.Fatalf("session progress must remain live-only, got attacks=%d cap=%d", got.SessionAttacks, got.SessionAttackCap)
	}
	if got.SpeedProfile != "Fast" || got.HealthScore != 94 || got.Anomalies != 2 {
		t.Fatalf("runtime intelligence metrics lost: %+v", got)
	}
	if got.AverageCaptureMS != 321 || got.AverageTargetScanMS != 42 ||
		got.AverageReturnHomeMS != 880 || got.AverageNextTransitionMS != 735 {
		t.Fatalf("latency metrics lost: %+v", got)
	}
	if got.TargetsAccepted != 2 || got.AvgAcceptedGE != 1_900_000 || got.AvgRejectedGE != 850_000 ||
		got.AvgAcceptedDE != 4_500 || got.AvgRejectedDE != 1_200 ||
		got.AvgAcceptedScore != 91 || got.AvgRejectedScore != 57 {
		t.Fatalf("search intelligence metrics lost: %+v", got)
	}
	if got.PreferredScaleAttempts != 20 || got.PreferredScaleHits != 15 ||
		got.PreferredScaleFallbacks != 5 || got.PreferredScaleHitRate != 75 || !got.PreferredScaleEnabled {
		t.Fatalf("preferred-scale metrics lost: %+v", got)
	}
	if got.UIAnchorAttempts != 24 || got.UIAnchorHits != 20 || got.UIAnchorFallbacks != 4 ||
		got.UIAnchorHitRate != 83.33 || !got.UIAnchorEnabled {
		t.Fatalf("UI-anchor metrics lost: %+v", got)
	}
	if got.NearMissTargets != 7 || got.NearMiss5Targets != 2 ||
		got.NearMiss10Targets != 5 || got.NearMiss15Targets != 7 {
		t.Fatalf("threshold-sensitivity metrics lost: %+v", got)
	}
	if got.GoldPerHour != 3_000_000 {
		t.Fatalf("gold/hour=%v want 3000000", got.GoldPerHour)
	}
	if got.RecoverySuccessRate != 50 {
		t.Fatalf("recovery success rate=%v want 50", got.RecoverySuccessRate)
	}
	if got.ThreeStarRate != 75 {
		t.Fatalf("3-star rate=%v want 75", got.ThreeStarRate)
	}
	if got.AverageStars != 2.75 {
		t.Fatalf("average stars=%v want 2.75", got.AverageStars)
	}
}


func TestMergeStatsSeparatesLifetimeAndCurrentSessionAttacks(t *testing.T) {
	acc := bot.BotStats{AttacksCompleted: 120}
	current := bot.BotStats{
		AttacksCompleted: 3,
		SessionAttacks:   3,
		SessionAttackCap: 10,
	}

	got := mergeStats(acc, current)
	if got.AttacksCompleted != 123 {
		t.Fatalf("lifetime attacks=%d want 123", got.AttacksCompleted)
	}
	if got.SessionAttacks != 3 {
		t.Fatalf("session attacks=%d want 3", got.SessionAttacks)
	}
	if got.SessionAttackCap != 10 {
		t.Fatalf("session cap=%d want 10", got.SessionAttackCap)
	}
}

func TestStartupReadinessRequiresLicenseWhenEnforced(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("CLASHGO_CONFIG_DIR", dir)
	t.Setenv("CLASHGO_CONTROL_API_URL", "https://control.example.test")

	a := &App{}
	readiness := a.GetStartupReadiness()

	found := false
	for _, check := range readiness.Checks {
		if check.ID != "license" {
			continue
		}
		found = true
		if check.OK {
			t.Fatal("license readiness should fail when enforcement is enabled and no license is active")
		}
	}
	if !found {
		t.Fatal("startup readiness did not include license check")
	}
	if readiness.Ready {
		t.Fatal("startup readiness should not be ready without required license")
	}
}
