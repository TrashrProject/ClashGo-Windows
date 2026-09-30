package bot

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/Ducky705/ClashGO/internal/paths"
)

type SessionBestAttack struct {
	Timestamp string `json:"timestamp,omitempty"`
	Strategy  string `json:"strategy,omitempty"`
	Side      string `json:"side,omitempty"`
	Stars     int    `json:"stars"`
	Gold      int    `json:"gold"`
	Elixir    int    `json:"elixir"`
	DarkElixir int   `json:"dark_elixir"`
	GE        int    `json:"gold_plus_elixir"`
	TargetScore int  `json:"target_score"`
}

type SessionReport struct {
	SessionID string    `json:"session_id"`
	GeneratedAt time.Time `json:"generated_at"`

	Attacks int `json:"attacks"`
	TotalGold int64 `json:"total_gold"`
	TotalElixir int64 `json:"total_elixir"`
	TotalDE int64 `json:"total_de"`

	GoldPerHour float64 `json:"gold_per_hour"`
	ElixirPerHour float64 `json:"elixir_per_hour"`
	DEPerHour float64 `json:"de_per_hour"`

	AverageStars float64 `json:"average_stars"`
	ThreeStarRate float64 `json:"three_star_rate"`
	AverageDestruction float64 `json:"average_destruction"`
	AverageTargetScore float64 `json:"average_target_score"`

	FullDeployRate float64 `json:"full_deploy_rate"`
	ReturnHomeRate float64 `json:"return_home_rate"`
	SafeCorridorRate float64 `json:"safe_corridor_rate"`
	ZeroTouchRate float64 `json:"zero_touch_rate"`
	CurrentZeroTouchStreak int `json:"current_zero_touch_streak"`
	BestZeroTouchStreak int `json:"best_zero_touch_streak"`

	AverageCooldownMS float64 `json:"average_cooldown_ms"`
	AveragePreparationMS float64 `json:"average_preparation_ms"`
	AverageSearchMS float64 `json:"average_search_ms"`
	AverageDeployMS float64 `json:"average_deploy_ms"`
	AverageCombatMS float64 `json:"average_combat_ms"`
	AverageReturnHomeMS float64 `json:"average_return_home_ms"`
	AverageRoutineMS float64 `json:"average_routine_ms"`
	AverageBattleEndWaitMS float64 `json:"average_battle_end_wait_ms"`
	NaturalBattleEndWaitMS float64 `json:"natural_battle_end_wait_ms"`
	EarlyBattleEndWaitMS float64 `json:"early_battle_end_wait_ms"`
	EarlyExitRate float64 `json:"early_exit_rate"`
	AverageLootExitPercent float64 `json:"average_loot_exit_percent"`
	Bottleneck string `json:"bottleneck"`
	OptimizationTarget string `json:"optimization_target"`

	TopStrategy string `json:"top_strategy,omitempty"`
	TopDeploySide string `json:"top_deploy_side,omitempty"`
	RuntimeModes map[string]int `json:"runtime_modes,omitempty"`
	EndReasons map[string]int `json:"end_reasons,omitempty"`

	BestAttack SessionBestAttack `json:"best_attack"`

	HealthScore int `json:"health_score"`
	SpeedProfile string `json:"speed_profile"`
	Anomalies int64 `json:"anomalies"`
	RecoveryAttempts int32 `json:"recovery_attempts"`
	RecoverySuccesses int32 `json:"recovery_successes"`
	RecoverySuccessRate float64 `json:"recovery_success_rate"`
	BlueStacksRestarts int32 `json:"bluestacks_restarts"`
	PreferredScaleHitRate float64 `json:"preferred_scale_hit_rate"`
	PreferredScaleEnabled bool `json:"preferred_scale_enabled"`
	Recommendations []string `json:"recommendations,omitempty"`
}

func BuildSessionReport(sessionID string, history []AttackReport, stats BotStats, now time.Time) SessionReport {
	report := SessionReport{
		SessionID: sessionID,
		GeneratedAt: now.UTC(),
		RuntimeModes: map[string]int{},
		EndReasons: map[string]int{},
		HealthScore: stats.HealthScore,
		SpeedProfile: stats.SpeedProfile,
		Anomalies: stats.Anomalies,
		RecoveryAttempts: stats.RecoveryAttempts,
		RecoverySuccesses: stats.RecoverySuccesses,
		RecoverySuccessRate: stats.RecoverySuccessRate,
		BlueStacksRestarts: stats.BlueStacksRestarts,
		PreferredScaleHitRate: stats.PreferredScaleHitRate,
		PreferredScaleEnabled: stats.PreferredScaleEnabled,
	}

	rows := make([]AttackReport, 0, len(history))
	for _, rep := range history {
		if sessionID != "" && rep.SessionID != sessionID {
			continue
		}
		rows = append(rows, rep)
	}
	if len(rows) == 0 {
		return report
	}

	report.Attacks = len(rows)
	strategyCounts := map[string]int{}
	sideCounts := map[string]int{}

	var stars, destruction, targetScore float64
	var triples, fullDeploy, returned, safe, zeroTouch int
	var cooldownMS, prepMS, searchMS, deployMS, combatMS, homeMS, routineMS float64
	var battleEndWaitMS, naturalBattleEndWaitMS, earlyBattleEndWaitMS, lootExitPct float64
	var naturalBattleEnds, earlyBattleEnds, lootExitCount int
	var measuredTargetScore int
	var currentStreak, bestStreak, runningStreak int

	bestValue := -1

	for index, rep := range rows {
		gold := rep.GoldStolen + rep.BonusGold
		elixir := rep.ElixirStolen + rep.BonusElixir
		de := rep.DarkElixirStolen + rep.BonusDE
		ge := gold + elixir

		report.TotalGold += int64(gold)
		report.TotalElixir += int64(elixir)
		report.TotalDE += int64(de)

		stars += float64(rep.Stars)
		destruction += float64(rep.DestructionPct)
		if rep.Stars == 3 {
			triples++
		}
		if rep.TargetScore > 0 {
			targetScore += float64(rep.TargetScore)
			measuredTargetScore++
		}
		if rep.DeploySuccess {
			fullDeploy++
		}
		if rep.ReturnHomeSuccess {
			returned++
		}
		if rep.RedZoneValid && rep.CorridorVerified && rep.HUDSafe {
			safe++
		}
		clean := rep.DeploySuccess && rep.ReturnHomeSuccess && rep.CorridorVerified && rep.HUDSafe
		if clean {
			zeroTouch++
			runningStreak++
			if runningStreak > bestStreak {
				bestStreak = runningStreak
			}
			if index == currentStreak {
				currentStreak++
			}
		} else {
			runningStreak = 0
		}

		cooldownMS += float64(rep.CooldownDurationMS)
		prepMS += float64(rep.PreparationDurationMS)
		searchMS += float64(rep.SearchDurationMS)
		deployMS += float64(rep.DeployDurationMS)
		battle := float64(rep.BattleDurationMS)
		if battle > float64(rep.DeployDurationMS) {
			combatMS += battle - float64(rep.DeployDurationMS)
		}
		homeMS += float64(rep.ReturnHomeDurationMS)
		routine := rep.FullRoutineDurationMS
		if routine <= 0 {
			routine = rep.CycleDurationMS
		}
		routineMS += float64(routine)

		if s := strings.TrimSpace(rep.Strategy); s != "" && s != "Unknown" {
			strategyCounts[s]++
		}
		side := strings.TrimSpace(rep.DeploySide)
		if side == "" || side == "Unknown" {
			side = strings.TrimSpace(rep.TargetEdge)
		}
		if side != "" && side != "Unknown" {
			sideCounts[side]++
		}
		if mode := strings.TrimSpace(rep.RuntimeMode); mode != "" {
			report.RuntimeModes[mode]++
		}
		if reason := strings.TrimSpace(rep.BattleEndReason); reason != "" {
			report.EndReasons[reason]++
		}
		waitMS := float64(rep.BattleEndWaitMS)
		if waitMS > 0 {
			battleEndWaitMS += waitMS
			switch rep.BattleEndReason {
			case "loot_threshold", "destruction_threshold", "stall":
				earlyBattleEnds++
				earlyBattleEndWaitMS += waitMS
			case "natural_result":
				naturalBattleEnds++
				naturalBattleEndWaitMS += waitMS
			}
		}
		if rep.LootExitPercent > 0 {
			lootExitPct += float64(rep.LootExitPercent)
			lootExitCount++
		}

		if ge > bestValue {
			bestValue = ge
			report.BestAttack = SessionBestAttack{
				Timestamp: rep.Timestamp,
				Strategy: rep.Strategy,
				Side: side,
				Stars: rep.Stars,
				Gold: gold,
				Elixir: elixir,
				DarkElixir: de,
				GE: ge,
				TargetScore: rep.TargetScore,
			}
		}
	}

	n := float64(len(rows))
	report.AverageStars = stars / n
	report.ThreeStarRate = float64(triples) * 100 / n
	report.AverageDestruction = destruction / n
	if measuredTargetScore > 0 {
		report.AverageTargetScore = targetScore / float64(measuredTargetScore)
	}
	report.FullDeployRate = float64(fullDeploy) * 100 / n
	report.ReturnHomeRate = float64(returned) * 100 / n
	report.SafeCorridorRate = float64(safe) * 100 / n
	report.ZeroTouchRate = float64(zeroTouch) * 100 / n
	report.CurrentZeroTouchStreak = currentStreak
	report.BestZeroTouchStreak = bestStreak

	report.AverageCooldownMS = cooldownMS / n
	report.AveragePreparationMS = prepMS / n
	report.AverageSearchMS = searchMS / n
	report.AverageDeployMS = deployMS / n
	report.AverageCombatMS = combatMS / n
	report.AverageReturnHomeMS = homeMS / n
	report.AverageRoutineMS = routineMS / n
	report.AverageBattleEndWaitMS = battleEndWaitMS / n
	if naturalBattleEnds > 0 {
		report.NaturalBattleEndWaitMS = naturalBattleEndWaitMS / float64(naturalBattleEnds)
	}
	if earlyBattleEnds > 0 {
		report.EarlyBattleEndWaitMS = earlyBattleEndWaitMS / float64(earlyBattleEnds)
	}
	report.EarlyExitRate = float64(earlyBattleEnds) * 100 / n
	if lootExitCount > 0 {
		report.AverageLootExitPercent = lootExitPct / float64(lootExitCount)
	}

	type stage struct {
		name string
		ms float64
		tunable bool
	}
	stages := []stage{
		{"cooldown_intentional", report.AverageCooldownMS, false},
		{"preparation", report.AveragePreparationMS, true},
		{"search", report.AverageSearchMS, true},
		// Deployment is deliberately protected: red-zone safety, live card
		// re-indexing and reliable tap cadence take priority over shaving time.
		{"deployment_protected", report.AverageDeployMS, false},
		{"combat", report.AverageCombatMS, true},
		{"return_home", report.AverageReturnHomeMS, true},
	}
	sort.Slice(stages, func(i, j int) bool { return stages[i].ms > stages[j].ms })
	if len(stages) > 0 {
		report.Bottleneck = stages[0].name
	}
	for _, s := range stages {
		if s.tunable {
			report.OptimizationTarget = s.name
			break
		}
	}

	if routineMS > 0 {
		hours := routineMS / 3_600_000
		report.GoldPerHour = float64(report.TotalGold) / hours
		report.ElixirPerHour = float64(report.TotalElixir) / hours
		report.DEPerHour = float64(report.TotalDE) / hours
	}

	report.TopStrategy = mostCommonKey(strategyCounts)
	report.TopDeploySide = mostCommonKey(sideCounts)

	// Recommendations are descriptive and based only on measured session
	// behavior. They never change config automatically.
	if report.FullDeployRate < 95 {
		report.Recommendations = append(report.Recommendations,
			"La fiabilité du déploiement est sous 95 % ; conserve les protections de sécurité et de timing avant d’accélérer davantage la cadence.")
	}
	if report.SafeCorridorRate < 98 {
		report.Recommendations = append(report.Recommendations,
			"La validation du corridor sûr est sous 98 % ; vérifie les diagnostics zone rouge/HUD avant de modifier la géométrie de déploiement.")
	}
	if report.OptimizationTarget != "" {
		report.Recommendations = append(report.Recommendations,
			fmt.Sprintf("Étape optimisable la plus longue de cette session : %s.", strings.ReplaceAll(report.OptimizationTarget, "_", " ")))
	}
	if report.EarlyExitRate > 0 && report.EarlyBattleEndWaitMS > 0 && report.NaturalBattleEndWaitMS > 0 {
		delta := report.NaturalBattleEndWaitMS - report.EarlyBattleEndWaitMS
		if delta > 5000 {
			report.Recommendations = append(report.Recommendations,
				fmt.Sprintf("Les sorties anticipées ont réduit l’attente de fin de combat de %.1fs en moyenne ; vérifie le compromis butin/étoiles avant de réduire les seuils.", delta/1000))
		}
	}
	if report.ZeroTouchRate >= 98 && report.Attacks >= 10 {
		report.Recommendations = append(report.Recommendations,
			"L’autonomie est stable à ≥98 % sans intervention ; cette session est adaptée à un test d’endurance plus long sans surveillance.")
	}
	return report
}

func mostCommonKey(values map[string]int) string {
	best := ""
	bestCount := -1
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		if values[key] > bestCount {
			best = key
			bestCount = values[key]
		}
	}
	return best
}

func (b *Bot) CurrentSessionReport() SessionReport {
	sessionID := ""
	if b.telemetry != nil {
		sessionID = b.telemetry.SessionID()
	}
	return BuildSessionReport(sessionID, b.HistorySnapshot(), b.Stats(), time.Now())
}

func writeSessionReportCSV(path string, report SessionReport) error {
	f, err := os.Create(path)
	if err != nil { return err }
	defer f.Close()
	w := csv.NewWriter(f)
	defer w.Flush()

	header := []string{
		"session_id", "generated_at", "attacks",
		"total_gold", "total_elixir", "total_de",
		"gold_per_hour", "elixir_per_hour", "de_per_hour",
		"average_stars", "three_star_rate", "average_destruction",
		"full_deploy_rate", "return_home_rate", "safe_corridor_rate", "zero_touch_rate",
		"best_zero_touch_streak", "average_routine_ms", "average_battle_end_wait_ms", "early_exit_rate", "average_loot_exit_percent", "bottleneck", "optimization_target",
		"top_strategy", "top_deploy_side", "health_score", "speed_profile",
		"anomalies", "recovery_attempts", "recovery_success_rate", "bluestacks_restarts",
		"preferred_scale_hit_rate", "preferred_scale_enabled",
	}
	row := []string{
		report.SessionID, report.GeneratedAt.Format(time.RFC3339Nano), fmt.Sprint(report.Attacks),
		fmt.Sprint(report.TotalGold), fmt.Sprint(report.TotalElixir), fmt.Sprint(report.TotalDE),
		fmt.Sprintf("%.2f", report.GoldPerHour), fmt.Sprintf("%.2f", report.ElixirPerHour), fmt.Sprintf("%.2f", report.DEPerHour),
		fmt.Sprintf("%.3f", report.AverageStars), fmt.Sprintf("%.2f", report.ThreeStarRate), fmt.Sprintf("%.2f", report.AverageDestruction),
		fmt.Sprintf("%.2f", report.FullDeployRate), fmt.Sprintf("%.2f", report.ReturnHomeRate), fmt.Sprintf("%.2f", report.SafeCorridorRate), fmt.Sprintf("%.2f", report.ZeroTouchRate),
		fmt.Sprint(report.BestZeroTouchStreak), fmt.Sprintf("%.0f", report.AverageRoutineMS), fmt.Sprintf("%.0f", report.AverageBattleEndWaitMS), fmt.Sprintf("%.2f", report.EarlyExitRate), fmt.Sprintf("%.2f", report.AverageLootExitPercent), report.Bottleneck, report.OptimizationTarget,
		report.TopStrategy, report.TopDeploySide, fmt.Sprint(report.HealthScore), report.SpeedProfile,
		fmt.Sprint(report.Anomalies), fmt.Sprint(report.RecoveryAttempts), fmt.Sprintf("%.2f", report.RecoverySuccessRate), fmt.Sprint(report.BlueStacksRestarts),
		fmt.Sprintf("%.2f", report.PreferredScaleHitRate), fmt.Sprint(report.PreferredScaleEnabled),
	}
	if err := w.Write(header); err != nil { return err }
	if err := w.Write(row); err != nil { return err }
	w.Flush()
	return w.Error()
}

func saveSessionReport(report SessionReport) error {
	if report.Attacks == 0 {
		return nil
	}
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return err
	}
	dir := paths.ResolveConfig("output/session_reports")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	name := report.SessionID
	if name == "" {
		name = report.GeneratedAt.Format("20060102T150405.000000000Z")
	}
	if err := os.WriteFile(filepath.Join(dir, name+".json"), data, 0o644); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, "latest.json"), data, 0o644); err != nil {
		return err
	}

	// CSV companion for spreadsheet analysis. One row per session file keeps
	// exports deterministic/idempotent and avoids duplicate append rows when a
	// Stop path is invoked twice.
	if err := writeSessionReportCSV(filepath.Join(dir, name+".csv"), report); err != nil {
		return err
	}
	return writeSessionReportCSV(filepath.Join(dir, "latest.csv"), report)
}
