package bot

import (
	"testing"
	"time"
)

func TestVillageBrainPriorities(t *testing.T) {
	now := time.Unix(1000, 0)
	base := VillageDecisionInput{
		Now: now,
		VillageVerified: true,
		DonationEnabled: true,
		LastDonationScan: now.Add(-2*time.Minute),
		ResourceEnabled: true,
		LastResourceScan: now.Add(-time.Minute),
		AttackButtonVisible: true,
	}

	if got := decideVillageAction(base).Action; got != VillageActionDonate {
		t.Fatalf("first action=%v want donate", got)
	}

	base.LastDonationScan = now
	if got := decideVillageAction(base).Action; got != VillageActionScanResources {
		t.Fatalf("second action=%v want resource scan", got)
	}

	base.LastResourceScan = now
	base.ArmyWaitUntil = now.Add(time.Minute)
	if got := decideVillageAction(base).Action; got != VillageActionWaitArmy {
		t.Fatalf("third action=%v want army wait", got)
	}

	base.ArmyWaitUntil = time.Time{}
	if got := decideVillageAction(base).Action; got != VillageActionAttack {
		t.Fatalf("fourth action=%v want attack", got)
	}
}

func TestVillageBrainNeverActsWithoutVerifiedVillage(t *testing.T) {
	got := decideVillageAction(VillageDecisionInput{
		Now: time.Now(),
		DonationEnabled: true,
		ResourceEnabled: true,
		AttackButtonVisible: true,
	})
	if got.Action != VillageActionIdle {
		t.Fatalf("action=%v want idle", got.Action)
	}
}

func TestVillageBrainHoldsDuringDonationOrAttack(t *testing.T) {
	now := time.Unix(2000, 0)
	for _, tc := range []struct {
		name string
		seq  bool
		don  bool
	}{
		{name: "attack running", seq: true},
		{name: "donation running", don: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := decideVillageAction(VillageDecisionInput{
				Now:                 now,
				VillageVerified:     true,
				SequenceRunning:     tc.seq,
				DonationInFlight:    tc.don,
				DonationEnabled:     true,
				ResourceEnabled:     true,
				AttackButtonVisible: true,
			})
			if got.Action != VillageActionHold {
				t.Fatalf("action=%v want hold", got.Action)
			}
		})
	}
}

func TestVillageBrainUsesHousekeepingWhileArmyWaits(t *testing.T) {
	now := time.Unix(3000, 0)
	got := decideVillageAction(VillageDecisionInput{
		Now:                 now,
		VillageVerified:     true,
		ResourceEnabled:     true,
		LastResourceScan:    now.Add(-time.Minute),
		ArmyWaitUntil:       now.Add(5 * time.Minute),
		AttackButtonVisible: true,
	})
	if got.Action != VillageActionScanResources {
		t.Fatalf("action=%v want resource scan before army wait", got.Action)
	}
}
