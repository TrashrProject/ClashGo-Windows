package main

import (
	"os"
	"runtime"

	"github.com/Ducky705/ClashGO/internal/adb"
	"github.com/Ducky705/ClashGO/internal/paths"
)

type SystemDiagnostics struct {
	OS            string                  `json:"os"`
	Arch          string                  `json:"arch"`
	Version       string                  `json:"version"`
	AssetsDir     string                  `json:"assets_dir"`
	ConfigDir     string                  `json:"config_dir"`
	AssetsReady   bool                    `json:"assets_ready"`
	MissingAssets []string                `json:"missing_assets"`
	Emulator      adb.PlatformDiagnostics `json:"emulator"`
}

var requiredRuntimeAssets = []string{
	"templates/btn_attack.png",
	"templates/btn_find_match.png",
	"templates/btn_next.png",
	"templates/btn_return_home.png",
	"strategies/auto_edrag_rush.yaml",
	"strategies/auto_edrag_rush_formula.json",
	"precision_config.json",
}

func collectSystemDiagnostics() SystemDiagnostics {
	d := SystemDiagnostics{
		OS:            runtime.GOOS,
		Arch:          runtime.GOARCH,
		Version:       version,
		AssetsDir:     paths.GetAssetsDir(),
		ConfigDir:     paths.GetConfigDir(),
		MissingAssets: []string{},
		Emulator:      adb.GetPlatformDiagnostics(),
	}

	for _, rel := range requiredRuntimeAssets {
		p := paths.Resolve(rel)
		info, err := os.Stat(p)
		if err != nil || info.IsDir() {
			d.MissingAssets = append(d.MissingAssets, rel)
		}
	}
	d.AssetsReady = len(d.MissingAssets) == 0
	return d
}

// GetSystemDiagnostics exposes a read-only preflight snapshot to the Wails UI.
func (a *App) GetSystemDiagnostics() SystemDiagnostics {
	return collectSystemDiagnostics()
}
