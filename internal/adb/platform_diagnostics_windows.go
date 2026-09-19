//go:build windows

package adb

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

func getPlatformDiagnostics() PlatformDiagnostics {
	d := PlatformDiagnostics{
		OS:        runtime.GOOS,
		Supported: true,
		Instances: []PlatformInstanceDiagnostic{},
	}

	d.ADBExecutable = ADBExecutable()
	d.ADBFound = executablePathExists(d.ADBExecutable)

	if p, err := findBlueStacksWindowsPlayer(); err == nil {
		d.BlueStacksPlayer = p
		d.BlueStacksPlayerFound = true
	}

	d.BlueStacksConfig = findBlueStacksWindowsConfig()
	d.BlueStacksConfigFound = d.BlueStacksConfig != ""
	if d.BlueStacksConfigFound {
		if data, err := os.ReadFile(d.BlueStacksConfig); err == nil {
			m := windowsADBAccessRE.FindSubmatch(data)
			if len(m) == 2 {
				d.ADBSettingPresent = true
				d.ADBEnabled = string(m[1]) == "1"
			}
		}
	}

	instances := discoverBlueStacksWindowsInstances()
	d.PreferredInstance = chooseBlueStacksWindowsInstance(instances, "")
	for _, inst := range instances {
		d.Instances = append(d.Instances, PlatformInstanceDiagnostic{
			Name:      inst.Name,
			ADBPort:   inst.ADBPort,
			Preferred: strings.EqualFold(inst.Name, d.PreferredInstance),
		})
	}

	d.BlueStacksRunning = blueStacksWindowsProcessRunning()
	return d
}

func executablePathExists(path string) bool {
	if strings.TrimSpace(path) == "" {
		return false
	}
	if filepath.IsAbs(path) || strings.ContainsAny(path, `/\`) {
		info, err := os.Stat(path)
		return err == nil && !info.IsDir()
	}
	_, err := exec.LookPath(path)
	return err == nil
}

func blueStacksWindowsProcessRunning() bool {
	out, err := exec.Command(
		"tasklist",
		"/FI", "IMAGENAME eq HD-Player.exe",
		"/FO", "CSV",
		"/NH",
	).Output()
	return err == nil && strings.Contains(strings.ToLower(string(out)), "hd-player.exe")
}
