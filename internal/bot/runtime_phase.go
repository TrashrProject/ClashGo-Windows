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

func validRuntimePhaseTransition(from, to RuntimePhase) bool {
	if from == to || to == PhaseIdle {
		return true
	}
	switch from {
	case PhaseIdle:
		return to == PhaseAttackNavigation
	case PhaseAttackNavigation:
		return to == PhaseSearching
	case PhaseSearching:
		return to == PhasePlanning
	case PhasePlanning:
		// Planning may fall through to battle monitoring even when deployment
		// partially fails; recovery happens from the battle checkpoint.
		return to == PhaseDeploying || to == PhaseBattle
	case PhaseDeploying:
		return to == PhaseBattle
	case PhaseBattle:
		return to == PhaseParsingResult || to == PhaseReturningHome
	case PhaseParsingResult:
		return to == PhaseReturningHome
	case PhaseReturningHome:
		return to == PhaseAttackNavigation
	default:
		return false
	}
}

func (b *Bot) setRuntimePhase(phase RuntimePhase) {
	old := RuntimePhase(b.runtimePhase.Swap(int32(phase)))
	if old == phase {
		return
	}
	now := time.Now()
	previousSince := b.runtimePhaseSince.Swap(now.UnixNano())
	b.runtimeProgress.Store(now.UnixNano())

	event := b.logger.Info()
	if !validRuntimePhaseTransition(old, phase) {
		event = b.logger.Warn().Bool("unexpected_transition", true)
	}
	if previousSince > 0 {
		event = event.Dur("previous_phase_duration", now.Sub(time.Unix(0, previousSince)))
	}
	event.
		Str("from_phase", old.String()).
		Str("to_phase", phase.String()).
		Msg("runtime phase transition")
}
