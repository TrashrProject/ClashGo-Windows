//go:build windows

package main

import (
	"context"
	"time"

	"github.com/Ducky705/ClashGO/internal/bot"
	"github.com/rs/zerolog/log"
	"golang.org/x/sys/windows"
)

var (
	emergencyUser32 = windows.NewLazySystemDLL("user32.dll")
	emergencyGetAsyncKeyState = emergencyUser32.NewProc("GetAsyncKeyState")
)

const emergencyVKEnd = 0x23

func emergencyEndPressed() bool {
	state, _, _ := emergencyGetAsyncKeyState.Call(emergencyVKEnd)
	return state&0x8000 != 0
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

	wasPressed := emergencyEndPressed()
	for {
		select {
		case <-ctx.Done():
			return
		case <-b.Done():
			return
		case <-ticker.C:
			pressed := emergencyEndPressed()
			if pressed && !wasPressed {
				a.mu.Lock()
				active := a.bot == b
				a.mu.Unlock()
				if active {
					log.Warn().Msg("End key pressed; stopping ClashGO immediately")
					_ = a.StopBot()
				}
				return
			}
			wasPressed = pressed
		}
	}
}
