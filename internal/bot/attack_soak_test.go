package bot

import (
	"testing"

	"github.com/rs/zerolog"
)

func TestAttackSoakRequiresThreeCleanAttacks(t *testing.T) {
	b := &Bot{logger: zerolog.Nop()}
	clean := AttackReport{DeploySuccess: true, ReturnHomeSuccess: true, ParsedResults: true}

	b.recordAttackSoak(clean, 0)
	b.recordAttackSoak(clean, 0)
	if b.soakValidated.Load() {
		t.Fatal("soak validated before third clean attack")
	}
	if got := b.cleanAttackStreak.Load(); got != 2 {
		t.Fatalf("streak=%d, want 2", got)
	}

	b.recordAttackSoak(clean, 0)
	if !b.soakValidated.Load() {
		t.Fatal("soak not validated on third clean attack")
	}
}

func TestAttackSoakResetsOnRecovery(t *testing.T) {
	b := &Bot{logger: zerolog.Nop()}
	clean := AttackReport{DeploySuccess: true, ReturnHomeSuccess: true, ParsedResults: true}
	b.recordAttackSoak(clean, 0)
	b.recordAttackSoak(clean, 0)
	b.recordAttackSoak(clean, 1)
	if got := b.cleanAttackStreak.Load(); got != 0 {
		t.Fatalf("streak=%d, want reset to 0", got)
	}
}
