package bot

import (
	"strings"
	"testing"

	"github.com/Ducky705/ClashGO/internal/config"
)

func TestLearningScopeSeparatesClashAccounts(t *testing.T) {
	a := config.DefaultConfig()
	b := config.DefaultConfig()
	a.Account.PlayerTag = "#AAA111"
	b.Account.PlayerTag = "#BBB222"

	ka := learningScopeKey(a)
	kb := learningScopeKey(b)
	if ka == kb {
		t.Fatalf("different Clash accounts share learning scope %q", ka)
	}
	if strings.Contains(ka, "AAA111") || strings.Contains(kb, "BBB222") {
		t.Fatal("raw player tag leaked into learning scope")
	}
}

func TestLearningScopeStableForSameAccountAcrossDeviceDetails(t *testing.T) {
	a := config.DefaultConfig()
	b := config.DefaultConfig()
	a.Account.PlayerTag = "#SAME1"
	b.Account.PlayerTag = "#SAME1"
	b.Device.DeviceID = "127.0.0.1:7777"
	b.Device.Width = 1920
	b.Device.Height = 1080

	if got, want := learningScopeKey(b), learningScopeKey(a); got != want {
		t.Fatalf("same linked account should keep its learning namespace: got=%q want=%q", got, want)
	}
}

func TestVillageEnvironmentScopeSeparatesGeometryForSameAccount(t *testing.T) {
	a := config.DefaultConfig()
	b := config.DefaultConfig()
	a.Account.PlayerTag = "#SAME1"
	b.Account.PlayerTag = "#SAME1"
	b.Device.Width = 1920
	b.Device.Height = 1080
	b.Device.DPI = 240

	if learningScopeKey(a) != learningScopeKey(b) {
		t.Fatal("same account should share account learning namespace")
	}
	if learningEnvironmentScopeKey(a) == learningEnvironmentScopeKey(b) {
		t.Fatal("different geometry should use different village-memory environment scope")
	}
}

func TestAnonymousLearningScopeSeparatesDifferentEnvironments(t *testing.T) {
	a := config.DefaultConfig()
	b := config.DefaultConfig()
	a.Account.PlayerTag = ""
	b.Account.PlayerTag = ""
	a.Device.DeviceID = "localhost:5555"
	b.Device.DeviceID = "localhost:6666"

	if learningScopeKey(a) == learningScopeKey(b) {
		t.Fatal("different anonymous emulator environments share learning scope")
	}
}
