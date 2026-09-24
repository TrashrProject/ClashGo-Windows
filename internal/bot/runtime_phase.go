package bot

import "time"

// RuntimePhase describes what the automation believes it is DOING, while
// game.GameState describes what it SEES. Keeping both is important:
// "Battle" can mean searching a base, deploying troops, waiting for the fight,
// or parsing the result; the phase removes that ambiguity from logs/recovery.
type RuntimePhase int32

const (
	PhaseIdle RuntimePhase = iota
	PhaseDonation
	PhaseResourceScan
	PhaseWallUpgrade
	PhaseArmyCheck
	PhaseAttackNavigation
	PhaseSearching
	PhaseDeploying
	PhaseBattle
	PhaseParsingResult
	PhaseReturningHome
)

func (p RuntimePhase) String() string {
	switch p {
	case PhaseIdle:
		return "Idle"
	case PhaseDonation:
		return "Donation"
	case PhaseResourceScan:
		return "ResourceScan"
	case PhaseWallUpgrade:
		return "WallUpgrade"
	case PhaseArmyCheck:
		return "ArmyCheck"
	case PhaseAttackNavigation:
		return "AttackNavigation"
	case PhaseSearching:
		return "Searching"
	case PhaseDeploying:
		return "Deploying"
	case PhaseBattle:
		return "Battle"
	case PhaseParsingResult:
		return "ParsingResult"
	case PhaseReturningHome:
		return "ReturningHome"
	default:
		return "UnknownPhase"
	}
}

func (b *Bot) setRuntimePhase(phase RuntimePhase) {
	old := RuntimePhase(b.runtimePhase.Swap(int32(phase)))
	if old == phase {
		return
	}
	now := time.Now()
	b.runtimePhaseSince.Store(now.UnixNano())
	b.runtimeProgress.Store(now.UnixNano())
	b.logger.Debug().
		Str("from_phase", old.String()).
		Str("to_phase", phase.String()).
		Msg("runtime phase transition")
}

func (b *Bot) runtimePhaseAge(now time.Time) time.Duration {
	n := b.runtimePhaseSince.Load()
	if n <= 0 {
		return 0
	}
	return now.Sub(time.Unix(0, n))
}
