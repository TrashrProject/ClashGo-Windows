package adb

import (
	"os"
	"path/filepath"
	"testing"
)

func TestADBExecutableHonorsOverride(t *testing.T) {
	dir := t.TempDir()
	name := "fake-adb"
	if os.PathSeparator == '\\' {
		name += ".exe"
	}
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte("stub"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CLASHGO_ADB_PATH", p)
	ResetADBExecutableCache()
	t.Cleanup(ResetADBExecutableCache)

	if got := ADBExecutable(); got != p {
		t.Fatalf("ADBExecutable()=%q want %q", got, p)
	}
}
