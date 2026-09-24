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
