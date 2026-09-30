//go:build windows

package main

import (
	"context"
	"strings"
	"time"

	"github.com/Ducky705/ClashGO/internal/bot"
	"github.com/Ducky705/ClashGO/internal/config"
	"github.com/rs/zerolog/log"
	"golang.org/x/sys/windows"
)

var (
	emergencyUser32 = windows.NewLazySystemDLL("user32.dll")
	emergencyGetAsyncKeyState = emergencyUser32.NewProc("GetAsyncKeyState")
)

const (
	emergencyVKEnd     = 0x23
	emergencyVKControl = 0x11
	emergencyVKShift   = 0x10
)

func emergencyKeyDown(vk uintptr) bool {
	state, _, _ := emergencyGetAsyncKeyState.Call(vk)
	return state&0x8000 != 0
}

func emergencyStopPressed(binding string) bool {
	switch strings.ToLower(strings.TrimSpace(binding)) {
	case "off", "disabled", "none":
		return false
	case "end":
		return emergencyKeyDown(emergencyVKEnd)
	default:
		return emergencyKeyDown(emergencyVKControl) &&
			emergencyKeyDown(emergencyVKShift) &&
			emergencyKeyDown(emergencyVKEnd)
	}
}

// watchEmergencyStopKey gives unattended Windows sessions a physical kill
// switch independent from the webview. It is edge-triggered and bound to the
// exact Bot instance that started it, so an old watcher can never stop a newer
// session after a quick restart.
func (a *App) watchEmergencyStopKey(ctx context.Context, b *bot.Bot) {
	if a == nil || b == nil {
		return
	}
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	binding := config.LoadOrDefault("config.json").Automation.EmergencyStopHotkey
	wasPressed := emergencyStopPressed(binding)
	for {
		select {
		case <-ctx.Done():
			return
		case <-b.Done():
			return
		case <-ticker.C:
			pressed := emergencyStopPressed(binding)
			if pressed && !wasPressed {
				a.mu.Lock()
				active := a.bot == b
				a.mu.Unlock()
				if active {
					log.Warn().Str("hotkey", binding).Msg("emergency stop hotkey pressed; stopping ClashGO immediately")
					_ = a.StopBot()
				}
				return
			}
			wasPressed = pressed
		}
	}
}
