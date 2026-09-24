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
		AttackEnabled:       true,
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
		AttackEnabled:       true,
		AttackButtonVisible: true,
	})
	if got.Action != VillageActionIdle {
		t.Fatalf("action=%v want idle", got.Action)
	}
}

func TestVillageBrainReportsActiveWork(t *testing.T) {
	now := time.Unix(2000, 0)

	attack := decideVillageAction(VillageDecisionInput{
		Now: now, VillageVerified: true, SequenceRunning: true,
		DonationEnabled: true, ResourceEnabled: true, AttackButtonVisible: true,
	})
	if attack.Action != VillageActionHold {
		t.Fatalf("attack action=%v want hold", attack.Action)
	}

	donation := decideVillageAction(VillageDecisionInput{
		Now: now, VillageVerified: true, DonationInFlight: true,
		DonationEnabled: true, ResourceEnabled: true, AttackButtonVisible: true,
	})
	if donation.Action != VillageActionDonate {
		t.Fatalf("donation action=%v want donate", donation.Action)
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
		AttackEnabled:       true,
		AttackButtonVisible: true,
	})
	if got.Action != VillageActionScanResources {
		t.Fatalf("action=%v want resource scan before army wait", got.Action)
	}
}

func TestVillageBrainHonorsDonationBackoff(t *testing.T) {
	now := time.Unix(4000, 0)
	got := decideVillageAction(VillageDecisionInput{
		Now:                 now,
		VillageVerified:     true,
		DonationEnabled:     true,
		LastDonationScan:    now.Add(-10 * time.Minute),
		DonationNextCheck:   now.Add(2 * time.Minute),
		ResourceEnabled:     false,
		AttackEnabled:       true,
		AttackButtonVisible: true,
	})
	if got.Action != VillageActionAttack {
		t.Fatalf("action=%v want attack while donation backoff is active", got.Action)
	}
}

func TestVillageBrainStopsAtSessionCap(t *testing.T) {
	now := time.Unix(5000, 0)
	got := decideVillageAction(VillageDecisionInput{
		Now: now, VillageVerified: true,
		AttackEnabled: true, AttackCapReached: true, AttackButtonVisible: true,
	})
	if got.Action != VillageActionSessionComplete {
		t.Fatalf("action=%v want session complete", got.Action)
	}
}

func TestVillageBrainOwnsInterAttackCooldown(t *testing.T) {
	now := time.Unix(6000, 0)
	got := decideVillageAction(VillageDecisionInput{
		Now: now, VillageVerified: true,
		AttackEnabled: true,
		AttackNotBefore: now.Add(20 * time.Second),
		AttackButtonVisible: true,
	})
	if got.Action != VillageActionCooldown {
		t.Fatalf("action=%v want cooldown", got.Action)
	}
}
