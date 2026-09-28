package bot

import (
	"strings"
	"encoding/csv"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestBuildSessionReportUsesTrueRoutineTimeAndReliability(t *testing.T) {
	session := "session-a"
	rows := []AttackReport{
		{
			Timestamp: "2026-09-28T10:02:00Z", SessionID: session,
			Strategy: "EDrag", DeploySide: "left", RuntimeMode: "Fast",
			DeploySuccess: true, ReturnHomeSuccess: true, RedZoneValid: true,
			CorridorVerified: true, HUDSafe: true, Stars: 3, DestructionPct: 100,
			GoldStolen: 1_000_000, ElixirStolen: 900_000, DarkElixirStolen: 8_000,
			CooldownDurationMS: 30_000, PreparationDurationMS: 5_000, SearchDurationMS: 10_000,
			DeployDurationMS: 20_000, BattleDurationMS: 120_000,
			ReturnHomeDurationMS: 1_000, FullRoutineDurationMS: 150_000,
			BattleEndWaitMS: 60_000, TargetScore: 90, BattleEndReason: "natural_result",
		},
		{
			Timestamp: "2026-09-28T10:05:00Z", SessionID: session,
			Strategy: "EDrag", DeploySide: "left", RuntimeMode: "Balanced",
			DeploySuccess: true, ReturnHomeSuccess: true, RedZoneValid: true,
			CorridorVerified: true, HUDSafe: true, Stars: 2, DestructionPct: 80,
			GoldStolen: 800_000, ElixirStolen: 700_000, DarkElixirStolen: 5_000,
			CooldownDurationMS: 20_000, PreparationDurationMS: 6_000, SearchDurationMS: 12_000,
			DeployDurationMS: 18_000, BattleDurationMS: 110_000,
			ReturnHomeDurationMS: 1_200, FullRoutineDurationMS: 140_000,
			BattleEndWaitMS: 45_000, LootExitPercent: 90,
			TargetScore: 80, BattleEndReason: "loot_threshold",
		},
		{Timestamp: "2026-09-28T09:00:00Z", SessionID: "other", GoldStolen: 99_000_000},
	}
	stats := BotStats{
		HealthScore: 97, SpeedProfile: "Fast", Anomalies: 1,
		RecoveryAttempts: 2, RecoverySuccesses: 2, RecoverySuccessRate: 100,
		BlueStacksRestarts: 1,
		PreferredScaleHitRate: 55, PreferredScaleEnabled: true,
	}
	report := BuildSessionReport(session, rows, stats, time.Date(2026,9,28,10,10,0,0,time.UTC))

	if report.Attacks != 2 {
		t.Fatalf("attacks=%d want 2", report.Attacks)
	}
	if report.TotalGold != 1_800_000 || report.TotalElixir != 1_600_000 || report.TotalDE != 13_000 {
		t.Fatalf("unexpected totals: %+v", report)
	}
	if report.FullDeployRate != 100 || report.ReturnHomeRate != 100 || report.ZeroTouchRate != 100 {
		t.Fatalf("unexpected reliability rates: %+v", report)
	}
	if report.CurrentZeroTouchStreak != 2 || report.BestZeroTouchStreak != 2 {
		t.Fatalf("unexpected streaks: current=%d best=%d", report.CurrentZeroTouchStreak, report.BestZeroTouchStreak)
	}
	if report.TopStrategy != "EDrag" || report.TopDeploySide != "left" {
		t.Fatalf("unexpected leaders: strategy=%q side=%q", report.TopStrategy, report.TopDeploySide)
	}
	if report.BestAttack.GE != 1_900_000 || report.BestAttack.Stars != 3 {
		t.Fatalf("unexpected best attack: %+v", report.BestAttack)
	}
	if report.AverageRoutineMS != 145_000 {
		t.Fatalf("avg routine=%v want 145000", report.AverageRoutineMS)
	}
	if report.AverageCooldownMS != 25_000 {
		t.Fatalf("avg cooldown=%v want 25000", report.AverageCooldownMS)
	}
	if report.AverageBattleEndWaitMS != 52_500 {
		t.Fatalf("avg battle-end wait=%v want 52500", report.AverageBattleEndWaitMS)
	}
	if report.NaturalBattleEndWaitMS != 60_000 || report.EarlyBattleEndWaitMS != 45_000 {
		t.Fatalf("unexpected natural/early wait: %v/%v", report.NaturalBattleEndWaitMS, report.EarlyBattleEndWaitMS)
	}
	if report.EarlyExitRate != 50 || report.AverageLootExitPercent != 90 {
		t.Fatalf("unexpected early-exit stats: rate=%v loot=%v", report.EarlyExitRate, report.AverageLootExitPercent)
	}
	if len(report.Recommendations) == 0 {
		t.Fatal("expected descriptive session recommendations")
	}
	if report.OptimizationTarget == "deployment_protected" || report.OptimizationTarget == "cooldown_intentional" {
		t.Fatalf("protected/intentional stage chosen as optimization target: %q", report.OptimizationTarget)
	}
	wantGoldPerHour := 1_800_000.0 / (290_000.0 / 3_600_000.0)
	if diff := report.GoldPerHour - wantGoldPerHour; diff < -0.01 || diff > 0.01 {
		t.Fatalf("gold/hour=%v want %v", report.GoldPerHour, wantGoldPerHour)
	}
	if report.HealthScore != 97 || !report.PreferredScaleEnabled {
		t.Fatalf("runtime intelligence fields missing: %+v", report)
	}
	if report.RecoveryAttempts != 2 || report.RecoverySuccesses != 2 ||
		report.RecoverySuccessRate != 100 || report.BlueStacksRestarts != 1 {
		t.Fatalf("runtime stability fields missing: %+v", report)
	}
}

func TestBuildSessionReportZeroTouchStreakStopsAtNewestFailure(t *testing.T) {
	rows := []AttackReport{
		{SessionID: "s", DeploySuccess: false, ReturnHomeSuccess: true, CorridorVerified: true, HUDSafe: true},
		{SessionID: "s", DeploySuccess: true, ReturnHomeSuccess: true, CorridorVerified: true, HUDSafe: true},
		{SessionID: "s", DeploySuccess: true, ReturnHomeSuccess: true, CorridorVerified: true, HUDSafe: true},
	}
	report := BuildSessionReport("s", rows, BotStats{}, time.Now())
	if report.CurrentZeroTouchStreak != 0 {
		t.Fatalf("current streak=%d want 0", report.CurrentZeroTouchStreak)
	}
	if report.BestZeroTouchStreak != 2 {
		t.Fatalf("best streak=%d want 2", report.BestZeroTouchStreak)
	}
}

func TestWriteSessionReportCSVWritesHeaderAndSingleRow(t *testing.T) {
	path := filepath.Join(t.TempDir(), "session.csv")
	report := SessionReport{
		SessionID: "session-csv",
		GeneratedAt: time.Date(2026, 9, 28, 12, 0, 0, 0, time.UTC),
		Attacks: 3,
		TotalGold: 3_000_000,
		GoldPerHour: 7_500_000,
		AverageStars: 2.5,
		ZeroTouchRate: 100,
		Bottleneck: "combat",
		OptimizationTarget: "search",
		HealthScore: 98,
		SpeedProfile: "Fast",
		PreferredScaleHitRate: 60,
		PreferredScaleEnabled: true,
	}
	if err := writeSessionReportCSV(path, report); err != nil {
		t.Fatalf("write CSV: %v", err)
	}
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open CSV: %v", err)
	}
	defer f.Close()

	rows, err := csv.NewReader(f).ReadAll()
	if err != nil {
		t.Fatalf("read CSV: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("rows=%d want header+1 data row", len(rows))
	}
	if rows[0][0] != "session_id" || rows[1][0] != "session-csv" {
		t.Fatalf("unexpected CSV first column: header=%q value=%q", rows[0][0], rows[1][0])
	}
	if len(rows[0]) != len(rows[1]) {
		t.Fatalf("CSV header/data column mismatch: %d vs %d", len(rows[0]), len(rows[1]))
	}
}

func TestBuildSessionReportRecommendationsProtectDeploymentSafety(t *testing.T) {
	rows := []AttackReport{
		{
			SessionID: "s", DeploySuccess: false, ReturnHomeSuccess: true,
			CorridorVerified: false, HUDSafe: false,
			FullRoutineDurationMS: 100_000, SearchDurationMS: 25_000,
		},
		{
			SessionID: "s", DeploySuccess: true, ReturnHomeSuccess: true,
			CorridorVerified: true, HUDSafe: true, RedZoneValid: true,
			FullRoutineDurationMS: 100_000, SearchDurationMS: 20_000,
		},
	}
	report := BuildSessionReport("s", rows, BotStats{}, time.Now())
	joined := strings.Join(report.Recommendations, " | ")
	if !strings.Contains(joined, "Deployment reliability is below 95%") {
		t.Fatalf("missing deployment safety recommendation: %v", report.Recommendations)
	}
	if !strings.Contains(joined, "Safe corridor certification is below 98%") {
		t.Fatalf("missing corridor safety recommendation: %v", report.Recommendations)
	}
}
