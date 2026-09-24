package bot

import (
	"testing"
	"time"

	"github.com/Ducky705/ClashGO/internal/config"
	"github.com/rs/zerolog"
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
	base.ArmyCheckEnabled = true
	base.ArmyCheckDue = true
	if got := decideVillageAction(base).Action; got != VillageActionCheckArmy {
		t.Fatalf("fourth action=%v want army preflight", got)
	}

	base.ArmyCheckDue = false
	if got := decideVillageAction(base).Action; got != VillageActionAttack {
		t.Fatalf("fifth action=%v want attack", got)
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


func TestVillageBrainSchedulesQueuedWallsBeforeAttack(t *testing.T) {
	now := time.Unix(7000, 0)
	got := decideVillageAction(VillageDecisionInput{
		Now:                 now,
		VillageVerified:     true,
		WallsEnabled:        true,
		WallsDue:            true,
		AttackEnabled:       true,
		AttackButtonVisible: true,
	})
	if got.Action != VillageActionUpgradeWalls {
		t.Fatalf("action=%v want wall upgrade", got.Action)
	}
}

func TestVillageBrainDoesNotRunWallsUnlessQueued(t *testing.T) {
	now := time.Unix(7100, 0)
	got := decideVillageAction(VillageDecisionInput{
		Now:                 now,
		VillageVerified:     true,
		WallsEnabled:        true,
		WallsDue:            false,
		AttackEnabled:       true,
		AttackButtonVisible: true,
	})
	if got.Action != VillageActionAttack {
		t.Fatalf("action=%v want attack when walls are enabled but not queued", got.Action)
	}
}


func TestAutomationTaskLeaseAllowsOnlyOneOwner(t *testing.T) {
	b := &Bot{}
	if !b.tryBeginAutomationTask("attack") {
		t.Fatal("first task should acquire lease")
	}
	if b.tryBeginAutomationTask("donation") {
		t.Fatal("second task must not acquire lease while attack owns it")
	}
	if got := b.currentAutomationTask(); got != "attack" {
		t.Fatalf("current task=%q want attack", got)
	}
	b.endAutomationTask("attack")
	if b.automationTaskInFlight.Load() {
		t.Fatal("lease should be released")
	}
	if !b.tryBeginAutomationTask("donation") {
		t.Fatal("next task should acquire released lease")
	}
	b.endAutomationTask("donation")
}

func TestAutomationTaskLeaseRejectsWrongRelease(t *testing.T) {
	b := &Bot{}
	if !b.tryBeginAutomationTask("wall upgrades") {
		t.Fatal("wall task should acquire lease")
	}
	b.endAutomationTask("attack")
	if !b.automationTaskInFlight.Load() {
		t.Fatal("wrong owner must not release active lease")
	}
	if got := b.currentAutomationTask(); got != "wall upgrades" {
		t.Fatalf("current task=%q want wall upgrades", got)
	}
	b.endAutomationTask("wall upgrades")
}


func TestVillageBrainExplainsActiveExclusiveTask(t *testing.T) {
	got := decideVillageAction(VillageDecisionInput{
		Now: time.Unix(8000, 0),
		VillageVerified: true,
		SequenceRunning: true,
		ActiveTaskName: "wall upgrades",
	})
	if got.Action != VillageActionHold {
		t.Fatalf("action=%v want hold", got.Action)
	}
	if got.Reason != "exclusive task active: wall upgrades" {
		t.Fatalf("reason=%q", got.Reason)
	}
}


func TestVillageBrainArmyPreflightWaitsForBackoff(t *testing.T) {
	now := time.Unix(9000, 0)
	got := decideVillageAction(VillageDecisionInput{
		Now: now,
		VillageVerified: true,
		ArmyCheckEnabled: true,
		ArmyCheckDue: true,
		ArmyWaitUntil: now.Add(25 * time.Second),
		AttackEnabled: true,
		AttackButtonVisible: true,
	})
	if got.Action != VillageActionWaitArmy {
		t.Fatalf("action=%v want wait army before retrying preflight", got.Action)
	}
}

func TestVillageBrainArmyPreflightRunsBeforeAttack(t *testing.T) {
	now := time.Unix(9100, 0)
	got := decideVillageAction(VillageDecisionInput{
		Now: now,
		VillageVerified: true,
		ArmyCheckEnabled: true,
		ArmyCheckDue: true,
		AttackEnabled: true,
		AttackButtonVisible: true,
	})
	if got.Action != VillageActionCheckArmy {
		t.Fatalf("action=%v want army preflight", got.Action)
	}
}


func TestRuntimeConfigIsDeferredUntilTaskBoundary(t *testing.T) {
	oldCfg := config.DefaultConfig()
	newCfg := config.DefaultConfig()
	newCfg.Attack.Enabled = false
	newCfg.Automation.AutoArmyGuard = false

	b := &Bot{
		cfg: oldCfg,
		logger: zerolog.Nop(),
		armySlot: 1,
	}
	if !b.tryBeginAutomationTask("attack") {
		t.Fatal("attack should acquire task lease")
	}

	b.UpdateConfig(newCfg)
	if b.cfg != oldCfg {
		t.Fatal("active task must keep original config until it finishes")
	}
	if b.pendingConfig != newCfg {
		t.Fatal("newest config should be staged while a task owns the UI")
	}

	b.endAutomationTask("attack")
	if b.cfg != newCfg {
		t.Fatal("staged config was not applied at task boundary")
	}
	if b.pendingConfig != nil {
		t.Fatal("pending config should be cleared after boundary apply")
	}
	if b.automationTaskInFlight.Load() {
		t.Fatal("task lease should be released after config boundary apply")
	}
}


func TestApplyConfigQueuesAndCancelsWallMaintenance(t *testing.T) {
	base := config.DefaultConfig()
	base.Upgrade.UpgradeWalls = false
	b := &Bot{cfg: base, logger: zerolog.Nop(), armySlot: 1}

	enabled := config.DefaultConfig()
	enabled.Upgrade.UpgradeWalls = true
	b.applyConfigNow(enabled)
	if !b.wallUpgradePending.Load() {
		t.Fatal("enabling wall automation should queue one scheduler pass")
	}

	disabled := config.DefaultConfig()
	disabled.Upgrade.UpgradeWalls = false
	b.applyConfigNow(disabled)
	if b.wallUpgradePending.Load() {
		t.Fatal("disabling wall automation should cancel queued wall maintenance")
	}
}
