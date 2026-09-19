//go:build windows

package adb

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDiscoverBlueStacksWindowsInstances(t *testing.T) {
	dir := t.TempDir()
	conf := filepath.Join(dir, "bluestacks.conf")
	data := []byte(
		"bst.instance.Pie64.adb_port=\"5555\"\n" +
			"bst.instance.Tiramisu64.status.adb_port=\"5562\"\n" +
			"bst.instance.Broken.adb_port=\"nope\"\n",
	)
	if err := os.WriteFile(conf, data, 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CLASHGO_BLUESTACKS_CONF", conf)
	t.Setenv("CLASHGO_BLUESTACKS_DATA", "")

	got := discoverBlueStacksWindowsInstances()
	if len(got) != 2 {
		t.Fatalf("expected 2 instances, got %d: %#v", len(got), got)
	}
	if got[0].Name != "Pie64" || got[0].ADBPort != 5555 {
		t.Fatalf("unexpected first instance: %#v", got[0])
	}
	if got[1].Name != "Tiramisu64" || got[1].ADBPort != 5562 {
		t.Fatalf("unexpected second instance: %#v", got[1])
	}
}

func TestChooseBlueStacksWindowsInstance(t *testing.T) {
	t.Setenv("CLASHGO_BLUESTACKS_INSTANCE", "")
	instances := []blueStacksWindowsInstance{
		{Name: "Pie64", ADBPort: 5555},
		{Name: "Tiramisu64", ADBPort: 5562},
	}
	if got := chooseBlueStacksWindowsInstance(instances, ""); got != "Tiramisu64" {
		t.Fatalf("expected Tiramisu64 preference, got %q", got)
	}

	t.Setenv("CLASHGO_BLUESTACKS_INSTANCE", "Custom64")
	if got := chooseBlueStacksWindowsInstance(instances, ""); got != "Custom64" {
		t.Fatalf("expected env override, got %q", got)
	}
}

func TestWindowsCandidateADBPortsDeduplicates(t *testing.T) {
	got := windowsCandidateADBPorts([]blueStacksWindowsInstance{
		{Name: "A", ADBPort: 5555},
		{Name: "B", ADBPort: 5562},
		{Name: "C", ADBPort: 5562},
	})
	seen := map[int]int{}
	for _, p := range got {
		seen[p]++
	}
	if seen[5555] != 1 || seen[5562] != 1 {
		t.Fatalf("ports were not deduplicated: %v", got)
	}
}

func TestWindowsCandidateADBPortsPreferred(t *testing.T) {
	instances := []blueStacksWindowsInstance{
		{Name: "Pie64", ADBPort: 5555},
		{Name: "Tiramisu64", ADBPort: 5562},
	}
	got := windowsCandidateADBPortsPreferred(instances, "Tiramisu64")
	if len(got) < 2 || got[0] != 5562 || got[1] != 5555 {
		t.Fatalf("preferred port ordering wrong: %v", got)
	}
}

func TestEnsureBlueStacksADBAccess(t *testing.T) {
	dir := t.TempDir()
	conf := filepath.Join(dir, "bluestacks.conf")
	original := []byte("bst.enable_adb_access=\"0\"\r\nbst.other=\"x\"\r\n")
	if err := os.WriteFile(conf, original, 0o644); err != nil {
		t.Fatal(err)
	}

	changed, err := ensureBlueStacksADBAccess(conf)
	if err != nil {
		t.Fatal(err)
	}
	if !changed {
		t.Fatal("expected ADB setting to be changed")
	}

	got, err := os.ReadFile(conf)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(got), "bst.enable_adb_access=\"1\"") {
		t.Fatalf("ADB flag not enabled: %s", got)
	}
	if _, err := os.Stat(conf + ".clashgo.bak"); err != nil {
		t.Fatalf("backup missing: %v", err)
	}
}

func TestEnsureBlueStacksADBAccessAlreadyEnabled(t *testing.T) {
	dir := t.TempDir()
	conf := filepath.Join(dir, "bluestacks.conf")
	if err := os.WriteFile(conf, []byte("bst.enable_adb_access=\"1\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	changed, err := ensureBlueStacksADBAccess(conf)
	if err != nil {
		t.Fatal(err)
	}
	if changed {
		t.Fatal("already-enabled ADB must not rewrite config")
	}
}

func TestEnsureBlueStacksADBAccessDoesNotInventMissingKey(t *testing.T) {
	dir := t.TempDir()
	conf := filepath.Join(dir, "bluestacks.conf")
	if err := os.WriteFile(conf, []byte("bst.other=\"x\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	changed, err := ensureBlueStacksADBAccess(conf)
	if err != nil {
		t.Fatal(err)
	}
	if changed {
		t.Fatal("must not add an ADB key that was absent")
	}
}


func TestChooseBlueStacksWindowsConfiguredInstance(t *testing.T) {
	t.Setenv("CLASHGO_BLUESTACKS_INSTANCE", "")
	instances := []blueStacksWindowsInstance{
		{Name: "Pie64", ADBPort: 5555},
		{Name: "Tiramisu64", ADBPort: 5562},
	}
	if got := chooseBlueStacksWindowsInstance(instances, "Pie64"); got != "Pie64" {
		t.Fatalf("expected configured Pie64 instance, got %q", got)
	}
}


func TestWindowsCandidateADBPortsConfiguredSelectionIsStrict(t *testing.T) {
	instances := []blueStacksWindowsInstance{
		{Name: "Pie64", ADBPort: 5555},
		{Name: "Tiramisu64", ADBPort: 5562},
	}
	got := windowsCandidateADBPortsForSelection(instances, "Pie64", "Pie64")
	if len(got) != 1 || got[0] != 5555 {
		t.Fatalf("configured instance must use only its own ADB port, got %v", got)
	}
}

func TestWindowsCandidateADBPortsAutoKeepsFallbacks(t *testing.T) {
	instances := []blueStacksWindowsInstance{
		{Name: "Pie64", ADBPort: 5555},
		{Name: "Tiramisu64", ADBPort: 5562},
	}
	got := windowsCandidateADBPortsForSelection(instances, "Tiramisu64", "")
	if len(got) < 2 || got[0] != 5562 || got[1] != 5555 {
		t.Fatalf("automatic mode should preserve preferred/fallback ordering, got %v", got)
	}
}
