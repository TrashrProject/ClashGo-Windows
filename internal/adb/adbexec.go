package adb

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
)

var (
	adbExecutableOnce sync.Once
	adbExecutablePath string
)

// ADBExecutable returns the adb-compatible executable used for lifecycle
// commands (connect, devices, start-server, kill-server). The bot's hot path
// still speaks the ADB wire protocol directly; this executable is only needed
// for server/device registration and recovery.
//
// Resolution order:
//   1. CLASHGO_ADB_PATH
//   2. adb / adb.exe from PATH
//   3. BlueStacks 5 HD-Adb.exe on Windows
//   4. plain "adb" as a final error-producing fallback
func ADBExecutable() string {
	adbExecutableOnce.Do(func() {
		if p := strings.TrimSpace(os.Getenv("CLASHGO_ADB_PATH")); p != "" {
			adbExecutablePath = p
			return
		}

		names := []string{"adb"}
		if runtime.GOOS == "windows" {
			names = []string{"adb.exe", "adb"}
		}
		for _, name := range names {
			if p, err := exec.LookPath(name); err == nil {
				adbExecutablePath = p
				return
			}
		}

		if runtime.GOOS == "windows" {
			// Standard Android SDK locations first. Some Windows setups have
			// platform-tools installed but do not add it to PATH.
			for _, envName := range []string{"ANDROID_HOME", "ANDROID_SDK_ROOT"} {
				if sdk := strings.TrimSpace(os.Getenv(envName)); sdk != "" {
					p := filepath.Join(sdk, "platform-tools", "adb.exe")
					if info, err := os.Stat(p); err == nil && !info.IsDir() {
						adbExecutablePath = p
						return
					}
				}
			}
			if local := strings.TrimSpace(os.Getenv("LOCALAPPDATA")); local != "" {
				p := filepath.Join(local, "Android", "Sdk", "platform-tools", "adb.exe")
				if info, err := os.Stat(p); err == nil && !info.IsDir() {
					adbExecutablePath = p
					return
				}
			}

			var roots []string
			if home := strings.TrimSpace(os.Getenv("CLASHGO_BLUESTACKS_HOME")); home != "" {
				roots = append(roots, home)
			}
			for _, envName := range []string{"ProgramFiles", "ProgramFiles(x86)"} {
				if root := strings.TrimSpace(os.Getenv(envName)); root != "" {
					roots = append(roots,
						filepath.Join(root, "BlueStacks_nxt"),
						filepath.Join(root, "BlueStacks"),
					)
				}
			}
			roots = append(roots,
				`C:\Program Files\BlueStacks_nxt`,
				`C:\Program Files\BlueStacks`,
				`C:\Program Files (x86)\BlueStacks_nxt`,
			)
			for _, root := range roots {
				for _, name := range []string{"HD-Adb.exe", "adb.exe"} {
					p := filepath.Join(root, name)
					if info, err := os.Stat(p); err == nil && !info.IsDir() {
						adbExecutablePath = p
						return
					}
				}
			}
		}

		adbExecutablePath = "adb"
	})
	return adbExecutablePath
}

// ResetADBExecutableCache is intended for tests that change environment
// overrides. Production code should never need it.
func ResetADBExecutableCache() {
	adbExecutableOnce = sync.Once{}
	adbExecutablePath = ""
}
