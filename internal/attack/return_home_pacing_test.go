package attack

import (
	"testing"
	"time"

	"github.com/Ducky705/ClashGO/internal/adb"
)

func TestChooseReturnHomePacingFast(t *testing.T) {
	p := chooseReturnHomePacing(adb.Health{FastCaptureMs: 350})
	if p.InitialWait != 250*time.Millisecond || p.PollWait != 150*time.Millisecond || p.Window != 1300*time.Millisecond {
		t.Fatalf("unexpected fast return-home pacing: %+v", p)
	}
}

func TestChooseReturnHomePacingBalanced(t *testing.T) {
	p := chooseReturnHomePacing(adb.Health{FastCaptureMs: 650})
	if p.InitialWait != 350*time.Millisecond || p.PollWait != 200*time.Millisecond || p.Window != 1500*time.Millisecond {
		t.Fatalf("unexpected balanced return-home pacing: %+v", p)
	}
}

func TestChooseReturnHomePacingSafe(t *testing.T) {
	p := chooseReturnHomePacing(adb.Health{FastCaptureMs: 1000})
	if p.InitialWait != 450*time.Millisecond || p.PollWait != 240*time.Millisecond || p.Window != 1700*time.Millisecond {
		t.Fatalf("unexpected safe return-home pacing: %+v", p)
	}
}

func TestChooseReturnHomePacingFailureForcesSafe(t *testing.T) {
	p := chooseReturnHomePacing(adb.Health{FastCaptureMs: 300, ConsecutiveFails: 1})
	if p.InitialWait != 450*time.Millisecond || p.PollWait != 240*time.Millisecond {
		t.Fatalf("transport failure must force safe return-home pacing: %+v", p)
	}
}
