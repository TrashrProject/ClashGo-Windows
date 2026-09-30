package bot

import (
	"os"
	"path/filepath"
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


func TestResolveMultiAccountActivePlayerTagUsesVerifiedSchedulerState(t *testing.T) {
	t.Setenv("CLASHGO_CONFIG_DIR", t.TempDir())
	cfg := config.DefaultConfig()
	cfg.Account.PlayerTag = "#OLD111"
	cfg.Account.MultiAccount = config.MultiAccountConfig{
		Enabled: true,
		ActiveAccountID: "old",
		Accounts: []config.ManagedAccount{
			{ID: "old", PlayerTag: "#OLD111", Enabled: true, SwitchSlot: 1},
			{ID: "new", PlayerTag: "#NEW222", Enabled: true, SwitchSlot: 2},
		},
	}

	path := multiAccountStatePath(cfg)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(`{"active_account_id":"new"}`), 0o600); err != nil {
		t.Fatal(err)
	}

	tag, ok := ResolveMultiAccountActivePlayerTag(cfg)
	if !ok || tag != "#NEW222" {
		t.Fatalf("resolved tag=%q ok=%v want #NEW222/true", tag, ok)
	}
}

func TestResolveMultiAccountActivePlayerTagRejectsDisabledStateAccount(t *testing.T) {
	t.Setenv("CLASHGO_CONFIG_DIR", t.TempDir())
	cfg := config.DefaultConfig()
	cfg.Account.MultiAccount = config.MultiAccountConfig{
		Enabled: true,
		Accounts: []config.ManagedAccount{
			{ID: "a", PlayerTag: "#AAA111", Enabled: true, SwitchSlot: 1},
			{ID: "b", PlayerTag: "#BBB222", Enabled: false, SwitchSlot: 2},
		},
	}
	path := multiAccountStatePath(cfg)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(`{"active_account_id":"b"}`), 0o600); err != nil {
		t.Fatal(err)
	}

	if tag, ok := ResolveMultiAccountActivePlayerTag(cfg); ok {
		t.Fatalf("disabled scheduler account unexpectedly resolved to %q", tag)
	}
}
