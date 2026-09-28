package bot

import (
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
			TargetScore: 90, BattleEndReason: "natural",
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
			TargetScore: 80, BattleEndReason: "natural",
		},
		{Timestamp: "2026-09-28T09:00:00Z", SessionID: "other", GoldStolen: 99_000_000},
	}
	stats := BotStats{
		HealthScore: 97, SpeedProfile: "Fast", Anomalies: 1,
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
