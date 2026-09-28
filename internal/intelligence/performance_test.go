package intelligence

import "testing"

func stableSample() PerformanceSample {
	return PerformanceSample{
		SearchMS: 10_000, DeployMS: 20_000, RoutineMS: 120_000,
		CaptureMS: 300, TargetScanMS: 90, DeploySuccess: true,
	}
}

func TestAnalyzePerformanceLearning(t *testing.T) {
	got := AnalyzePerformance([]PerformanceSample{stableSample(), stableSample()})
	if got.Status != "learning" {
		t.Fatalf("status=%q want learning", got.Status)
	}
}

func TestAnalyzePerformanceHealthy(t *testing.T) {
	samples := make([]PerformanceSample, 20)
	for i := range samples { samples[i] = stableSample() }
	got := AnalyzePerformance(samples)
	if got.Status != "healthy" || len(got.Regressions) != 0 {
		t.Fatalf("unexpected assessment: %+v", got)
	}
}

func TestAnalyzePerformanceDetectsRecentSlowdown(t *testing.T) {
	samples := make([]PerformanceSample, 20)
	for i := range samples { samples[i] = stableSample() }
	for i := 0; i < 5; i++ {
		samples[i].SearchMS = 15_000
		samples[i].DeployMS = 30_000
		samples[i].RoutineMS = 170_000
		samples[i].CaptureMS = 500
		samples[i].TargetScanMS = 150
	}
	got := AnalyzePerformance(samples)
	if got.Status != "watch" {
		t.Fatalf("status=%q want watch: %+v", got.Status, got)
	}
	want := map[string]bool{
		"search_ms": true, "deploy_ms": true, "routine_ms": true,
		"capture_ms": true, "target_scan_ms": true,
	}
	for _, r := range got.Regressions {
		delete(want, r.Metric)
	}
	if len(want) != 0 {
		t.Fatalf("missing regressions: %+v; got=%+v", want, got.Regressions)
	}
}

func TestAnalyzePerformanceDetectsDeployReliabilityDrop(t *testing.T) {
	samples := make([]PerformanceSample, 20)
	for i := range samples { samples[i] = stableSample() }
	for i := 0; i < 5; i++ {
		samples[i].DeploySuccess = i == 0
	}
	got := AnalyzePerformance(samples)
	found := false
	for _, r := range got.Regressions {
		if r.Metric == "deploy_success_rate" {
			found = true
			if r.Current != 20 || r.Baseline != 100 {
				t.Fatalf("unexpected deploy success regression: %+v", r)
			}
		}
	}
	if !found {
		t.Fatalf("deploy reliability regression missing: %+v", got)
	}
}
