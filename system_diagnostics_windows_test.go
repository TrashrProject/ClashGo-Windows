package main

import (
	"runtime"
	"testing"
)

func TestWindowsPreflightRuntimeGuardDocumented(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows preflight behavior is exercised by Windows CI/live tests")
	}
	// This smoke test intentionally only pins that diagnostics remain callable
	// on Windows without requiring BlueStacks to be installed on CI.
	d := collectSystemDiagnostics()
	if d.OS != "windows" {
		t.Fatalf("diagnostics OS=%q want windows", d.OS)
	}
}
