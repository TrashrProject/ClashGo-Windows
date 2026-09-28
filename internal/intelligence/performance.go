package intelligence

type PerformanceSample struct {
	SearchMS       int64
	DeployMS       int64
	RoutineMS      int64
	CaptureMS      float64
	TargetScanMS   float64
	DeploySuccess  bool
}

type PerformanceRegression struct {
	Metric   string  `json:"metric"`
	Current  float64 `json:"current"`
	Baseline float64 `json:"baseline"`
	DeltaPct float64 `json:"delta_pct"`
}

type PerformanceAssessment struct {
	Status      string                  `json:"status"`
	RecentCount int                     `json:"recent_count"`
	BaseCount   int                     `json:"baseline_count"`
	Regressions []PerformanceRegression `json:"regressions,omitempty"`
}

func AnalyzePerformance(samples []PerformanceSample) PerformanceAssessment {
	const recentN = 5
	if len(samples) < 8 {
		return PerformanceAssessment{
			Status:      "learning",
			RecentCount: minInt(len(samples), recentN),
			BaseCount:   maxInt(0, len(samples)-minInt(len(samples), recentN)),
		}
	}

	rn := recentN
	if len(samples) < recentN {
		rn = len(samples)
	}
	baseEnd := len(samples)
	if baseEnd > 20 {
		baseEnd = 20
	}
	recent := summarizePerformance(samples[:rn])
	base := summarizePerformance(samples[rn:baseEnd])

	out := PerformanceAssessment{
		Status:      "healthy",
		RecentCount: rn,
		BaseCount:   baseEnd - rn,
	}

	addSlower := func(metric string, current, baseline, thresholdPct float64) {
		if baseline <= 0 || current <= baseline*(1+thresholdPct/100) {
			return
		}
		delta := (current - baseline) * 100 / baseline
		out.Regressions = append(out.Regressions, PerformanceRegression{
			Metric: metric, Current: current, Baseline: baseline, DeltaPct: delta,
		})
	}

	addSlower("search_ms", recent.searchMS, base.searchMS, 35)
	addSlower("deploy_ms", recent.deployMS, base.deployMS, 35)
	addSlower("routine_ms", recent.routineMS, base.routineMS, 30)
	addSlower("capture_ms", recent.captureMS, base.captureMS, 40)
	addSlower("target_scan_ms", recent.scanMS, base.scanMS, 50)

	if base.deploySuccessRate-recent.deploySuccessRate >= 20 {
		out.Regressions = append(out.Regressions, PerformanceRegression{
			Metric:   "deploy_success_rate",
			Current:  recent.deploySuccessRate,
			Baseline: base.deploySuccessRate,
			DeltaPct: recent.deploySuccessRate - base.deploySuccessRate,
		})
	}

	if len(out.Regressions) > 0 {
		out.Status = "watch"
	}
	return out
}

type performanceSummary struct {
	searchMS          float64
	deployMS          float64
	routineMS         float64
	captureMS         float64
	scanMS            float64
	deploySuccessRate float64
}

func summarizePerformance(samples []PerformanceSample) performanceSummary {
	if len(samples) == 0 {
		return performanceSummary{}
	}
	var out performanceSummary
	var success int
	var searchN, deployN, routineN, captureN, scanN int
	for _, s := range samples {
		if s.SearchMS > 0 {
			out.searchMS += float64(s.SearchMS)
			searchN++
		}
		if s.DeployMS > 0 {
			out.deployMS += float64(s.DeployMS)
			deployN++
		}
		if s.RoutineMS > 0 {
			out.routineMS += float64(s.RoutineMS)
			routineN++
		}
		if s.CaptureMS > 0 {
			out.captureMS += s.CaptureMS
			captureN++
		}
		if s.TargetScanMS > 0 {
			out.scanMS += s.TargetScanMS
			scanN++
		}
		if s.DeploySuccess {
			success++
		}
	}
	if searchN > 0 { out.searchMS /= float64(searchN) }
	if deployN > 0 { out.deployMS /= float64(deployN) }
	if routineN > 0 { out.routineMS /= float64(routineN) }
	if captureN > 0 { out.captureMS /= float64(captureN) }
	if scanN > 0 { out.scanMS /= float64(scanN) }
	out.deploySuccessRate = float64(success) * 100 / float64(len(samples))
	return out
}

func minInt(a, b int) int {
	if a < b { return a }
	return b
}

func maxInt(a, b int) int {
	if a > b { return a }
	return b
}
