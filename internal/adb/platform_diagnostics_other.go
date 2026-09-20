//go:build !windows

package adb

import (
	"os/exec"
	"runtime"
)

func getPlatformDiagnostics() PlatformDiagnostics {
	adbPath := ADBExecutable()
	_, err := exec.LookPath(adbPath)
	return PlatformDiagnostics{
		OS:            runtime.GOOS,
		Supported:     runtime.GOOS == "darwin",
		ADBExecutable: adbPath,
		ADBFound:      err == nil,
		Instances:     []PlatformInstanceDiagnostic{},
	}
}
