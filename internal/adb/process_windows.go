//go:build windows

package adb

import (
	"context"
	"os/exec"
	"syscall"
)

// hiddenCommand runs short-lived console utilities without flashing a CMD
// window in the Wails GUI application. Do not use this for HD-Player.exe:
// BlueStacks itself must stay visible.
func hiddenCommand(name string, args ...string) *exec.Cmd {
	cmd := exec.Command(name, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	return cmd
}

func hiddenCommandContext(ctx context.Context, name string, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	return cmd
}
