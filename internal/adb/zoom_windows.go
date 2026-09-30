//go:build windows

package adb

import (
	"fmt"
	"strings"
)

// ZoomOutSafe asks the BlueStacks host window to execute its configured
// zoom-out key. This avoids Android sendevent multi-touch, which has proven
// unstable on some BlueStacks 5 Pie64 installations.
func (c *Client) ZoomOutSafe() error {
	key := strings.TrimSpace(c.zoomOutKey)
	if key == "" {
		key = "i"
	}
	sendKey := key
	switch key {
	case "+":
		sendKey = "{+}"
	case "%":
		sendKey = "{%}"
	case "^":
		sendKey = "{^}"
	case "~":
		sendKey = "{~}"
	}

	script := fmt.Sprintf(
		"$p=Get-Process HD-Player -ErrorAction SilentlyContinue | Where-Object {$_.MainWindowHandle -ne 0} | Select-Object -First 1; "+
			"if(-not $p){exit 2}; "+
			"$ws=New-Object -ComObject WScript.Shell; "+
			"if(-not $ws.AppActivate($p.Id)){exit 3}; "+
			"Start-Sleep -Milliseconds 80; "+
			"$ws.SendKeys('%s')",
		strings.ReplaceAll(sendKey, "'", "''"),
	)
	if err := hiddenCommand("powershell.exe", "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-Command", script).Run(); err != nil {
		return fmt.Errorf("send BlueStacks zoom-out key %q: %w", key, err)
	}
	return nil
}
