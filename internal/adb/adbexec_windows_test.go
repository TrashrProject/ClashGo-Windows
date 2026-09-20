//go:build windows

package adb

import (
	"os"
	"path/filepath"
	"testing"
)

func TestADBExecutableFindsLocalAndroidSDK(t *testing.T) {
	root := t.TempDir()
	adbPath := filepath.Join(root, "Android", "Sdk", "platform-tools", "adb.exe")
	if err := os.MkdirAll(filepath.Dir(adbPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(adbPath, []byte("stub"), 0o755); err != nil {
		t.Fatal(err)
	}

	t.Setenv("PATH", "")
	t.Setenv("CLASHGO_ADB_PATH", "")
	t.Setenv("ANDROID_HOME", "")
	t.Setenv("ANDROID_SDK_ROOT", "")
	t.Setenv("LOCALAPPDATA", root)
	ResetADBExecutableCache()
	t.Cleanup(ResetADBExecutableCache)

	if got := ADBExecutable(); got != adbPath {
		t.Fatalf("ADBExecutable()=%q want %q", got, adbPath)
	}
}
