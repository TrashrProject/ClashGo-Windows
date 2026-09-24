package bot

import "time"

// VillageAction is the single decision emitted by the village automation
// coordinator. Keeping arbitration in one place prevents donations, resource
// scans and matchmaking from racing each other.
type VillageAction int

const (
	VillageActionIdle VillageAction = iota
	VillageActionHold
	VillageActionDonate
	VillageActionScanResources
	VillageActionWaitArmy
	VillageActionCooldown
	VillageActionSessionComplete
	VillageActionAttack
)

func (a VillageAction) String() string {
	switch a {
	case VillageActionHold:
		return "holding"
	case VillageActionDonate:
		return "donating"
	case VillageActionScanResources:
		return "reading resources"
	case VillageActionWaitArmy:
		return "waiting for army"
	case VillageActionCooldown:
		return "cooldown"
	case VillageActionSessionComplete:
		return "session complete"
	case VillageActionAttack:
		return "starting attack"
	default:
		return "idle"
	}
}

type VillageDecisionInput struct {
	Now                 time.Time
	VillageVerified     bool
	SequenceRunning     bool
	DonationInFlight    bool
	DonationEnabled     bool
	LastDonationScan    time.Time
	DonationNextCheck   time.Time
	DonationInterval    time.Duration
	ResourceEnabled     bool
	LastResourceScan    time.Time
	ResourceInterval    time.Duration
	ArmyWaitUntil       time.Time
	AttackEnabled       bool
	AttackCapReached    bool
	AttackNotBefore     time.Time
	AttackButtonVisible bool
}

type VillageDecision struct {
	Action VillageAction
	Reason string
	NextAt time.Time
}

// decideVillageAction is deliberately pure so priority rules are testable.
//
// Priority:
//   1. never overlap an existing sequence/action;
//   2. donation opportunity (short and bounded);
//   3. periodic resource read;
//   4. explicit army-ready gate;
//   5. matchmaking.
//
// This means useful village housekeeping can happen while troops are training,
// but nothing competes with an active donation or attack.
func decideVillageAction(in VillageDecisionInput) VillageDecision {
	if !in.VillageVerified {
		return VillageDecision{Action: VillageActionIdle, Reason: "village not positively verified"}
	}
	if in.SequenceRunning {
		return VillageDecision{Action: VillageActionHold, Reason: "attack or long-running sequence is active"}
	}
	if in.DonationInFlight {
		return VillageDecision{Action: VillageActionDonate, Reason: "donation cycle is already running"}
	}

	donationInterval := in.DonationInterval
	if donationInterval <= 0 {
		donationInterval = 90 * time.Second
	}
	donationDue := in.LastDonationScan.IsZero() || in.Now.Sub(in.LastDonationScan) >= donationInterval
	if !in.DonationNextCheck.IsZero() && in.Now.Before(in.DonationNextCheck) {
		donationDue = false
	}
	if in.DonationEnabled && donationDue {
		return VillageDecision{Action: VillageActionDonate, Reason: "clan donation check is due"}
	}

	resourceInterval := in.ResourceInterval
	if resourceInterval <= 0 {
		resourceInterval = 15 * time.Second
	}
	if in.ResourceEnabled && (in.LastResourceScan.IsZero() || in.Now.Sub(in.LastResourceScan) >= resourceInterval) {
		return VillageDecision{Action: VillageActionScanResources, Reason: "resource snapshot is due"}
	}

	if !in.ArmyWaitUntil.IsZero() && in.Now.Before(in.ArmyWaitUntil) {
		return VillageDecision{Action: VillageActionWaitArmy, Reason: "army readiness gate is active", NextAt: in.ArmyWaitUntil}
	}

	if !in.AttackEnabled {
		return VillageDecision{Action: VillageActionIdle, Reason: "automatic attacks are disabled"}
	}
	if in.AttackCapReached {
		return VillageDecision{Action: VillageActionSessionComplete, Reason: "session attack limit reached"}
	}
	if !in.AttackNotBefore.IsZero() && in.Now.Before(in.AttackNotBefore) {
		return VillageDecision{Action: VillageActionCooldown, Reason: "waiting between attacks", NextAt: in.AttackNotBefore}
	}

	if in.AttackButtonVisible {
		return VillageDecision{Action: VillageActionAttack, Reason: "village ready and attack entry is available"}
	}

	return VillageDecision{Action: VillageActionIdle, Reason: "no action due"}
}
