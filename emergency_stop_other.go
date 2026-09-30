//go:build !windows

package main

import (
	"context"

	"github.com/Ducky705/ClashGO/internal/bot"
)

// Non-Windows builds keep the same call site without importing Win32.
func (a *App) watchEmergencyStopKey(ctx context.Context, b *bot.Bot) {}
