package bot

import "time"

type RuntimePhase int32

const (
	PhaseIdle RuntimePhase = iota
	PhaseAttackNavigation
	PhaseSearching
	PhasePlanning
	PhaseDeploying
	PhaseBattle
	PhaseParsingResult
	PhaseReturningHome
)

func (p RuntimePhase) String() string {
	switch p {
	case PhaseIdle:
		return "Idle"
	case PhaseAttackNavigation:
		return "AttackNavigation"
	case PhaseSearching:
		return "Searching"
	case PhasePlanning:
		return "Planning"
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
	b.logger.Info().
		Str("from_phase", old.String()).
		Str("to_phase", phase.String()).
		Msg("runtime phase transition")
}
