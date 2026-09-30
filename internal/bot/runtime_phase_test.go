package bot

import "testing"

func TestRuntimePhaseHappyPath(t *testing.T) {
	path := []RuntimePhase{
		PhaseIdle,
		PhaseAttackNavigation,
		PhaseSearching,
		PhasePlanning,
		PhaseDeploying,
		PhaseBattle,
		PhaseParsingResult,
		PhaseReturningHome,
		PhaseIdle,
	}
	for i := 0; i < len(path)-1; i++ {
		if !validRuntimePhaseTransition(path[i], path[i+1]) {
			t.Fatalf("expected valid transition %s -> %s", path[i], path[i+1])
		}
	}
}

func TestRuntimePhaseRejectsPlanningBackToSearch(t *testing.T) {
	if validRuntimePhaseTransition(PhasePlanning, PhaseSearching) {
		t.Fatal("planning -> searching should be flagged unexpected")
	}
}
