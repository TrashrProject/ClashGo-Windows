package intelligence

import "testing"

func stableSample() PerformanceSample {
	return PerformanceSample{
		SearchMS: 10_000, DeployMS: 20_000, RoutineMS: 120_000,
		CaptureMS: 300, TargetScanMS: 90, DeploySuccess: true, ReturnHomeOK: true, SafeDeployment: true,
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

func TestAnalyzePerformanceIgnoresUnavailableLegacyRoutineMetrics(t *testing.T) {
	samples := make([]PerformanceSample, 20)
	for i := range samples {
		samples[i] = stableSample()
	}
	// Simulate older history rows written before routine/capture/OCR timing
	// existed. Missing telemetry is unknown, not "zero milliseconds".
	for i := 5; i < 20; i++ {
		samples[i].RoutineMS = 0
		samples[i].CaptureMS = 0
		samples[i].TargetScanMS = 0
	}

	got := AnalyzePerformance(samples)
	for _, regression := range got.Regressions {
		if regression.Metric == "routine_ms" || regression.Metric == "capture_ms" || regression.Metric == "target_scan_ms" {
			t.Fatalf("legacy missing metric created false regression: %+v", regression)
		}
	}
}

func TestAnalyzePerformanceDetectsReturnHomeRegression(t *testing.T) {
	samples := make([]PerformanceSample, 20)
	for i := range samples { samples[i] = stableSample() }
	for i := 0; i < 5; i++ {
		samples[i].ReturnHomeOK = i == 0
	}
	got := AnalyzePerformance(samples)
	found := false
	for _, r := range got.Regressions {
		if r.Metric == "return_home_rate" {
			found = true
		}
	}
	if !found {
		t.Fatalf("return-home reliability regression missing: %+v", got)
	}
}

func TestAnalyzePerformanceDetectsSafeDeploymentRegression(t *testing.T) {
	samples := make([]PerformanceSample, 20)
	for i := range samples { samples[i] = stableSample() }
	for i := 0; i < 5; i++ {
		samples[i].SafeDeployment = i < 2
	}
	got := AnalyzePerformance(samples)
	found := false
	for _, r := range got.Regressions {
		if r.Metric == "safe_deployment_rate" {
			found = true
		}
	}
	if !found {
		t.Fatalf("safe-deployment reliability regression missing: %+v", got)
	}
}
