package bot

import (
	"image"
	"time"

	"github.com/Ducky705/ClashGO/internal/game"
	"gocv.io/x/gocv"
)

// localVisualDelta compares only a small rectangle around the action target.
// It is used as proof that a UI action actually changed the intended area,
// rather than accepting an ADB tap as success.
func localVisualDelta(before, after gocv.Mat, pt image.Point, radiusX, radiusY int) float64 {
	if before.Empty() || after.Empty() {
		return 0
	}
	if radiusX < 8 { radiusX = 8 }
	if radiusY < 8 { radiusY = 8 }

	maxW := minBotInt(before.Cols(), after.Cols())
	maxH := minBotInt(before.Rows(), after.Rows())
	x0 := maxBotInt(0, pt.X-radiusX)
	y0 := maxBotInt(0, pt.Y-radiusY)
	x1 := minBotInt(maxW-1, pt.X+radiusX)
	y1 := minBotInt(maxH-1, pt.Y+radiusY)
	if x1 <= x0 || y1 <= y0 {
		return 0
	}

	var sum float64
	var n int
	for y := y0; y <= y1; y += 2 {
		for x := x0; x <= x1; x += 2 {
			for c := 0; c < 3; c++ {
				d := int(before.GetUCharAt(y, x*3+c)) - int(after.GetUCharAt(y, x*3+c))
				if d < 0 { d = -d }
				sum += float64(d)
				n++
			}
		}
	}
	if n == 0 {
		return 0
	}
	return sum / (255 * float64(n))
}

// returnToVillageVerified issues at most maxBacks Back presses and verifies the
// screen after every transition. If Clash's quit confirmation ever appears,
// cancel it immediately rather than risking an application exit.
func (b *Bot) returnToVillageVerified(maxBacks int, source string) bool {
	if b == nil || (b.ctx != nil && b.ctx.Err() != nil) {
		return false
	}
	if maxBacks < 1 {
		maxBacks = 1
	}
	for attempt := 0; attempt <= maxBacks; attempt++ {
		if b.ctx != nil && b.ctx.Err() != nil {
			return false
		}
		screen, err := b.client.CaptureToMat()
		if err == nil && !screen.Empty() {
			state, _ := b.classify(screen)
			atVillage := state == game.StateMainVillage || b.findAttackButton(screen, 0.30)
			if state == game.StateConfirmExit {
				screen.Close()
				x, y := b.cal.ScaleRef(279, 429)
				if tapErr := b.client.TapFast(x, y, 0.5); tapErr == nil {
					b.logger.Warn().Str("source", source).Msg("verified-return reached quit-confirm; cancelled safely")
				}
				return true
			}
			screen.Close()
			if atVillage {
				return true
			}
		} else if !screen.Empty() {
			screen.Close()
		}

		if attempt == maxBacks {
			break
		}
		if err := b.client.Back(); err != nil {
			b.logger.Warn().Err(err).Str("source", source).Msg("verified-return Back failed")
			return false
		}
		if !b.sleepResponsive(160 * time.Millisecond) {
			return false
		}
	}

	b.logger.Warn().Str("source", source).Msg("could not positively confirm return to village")
	return false
}

func minBotInt(a,b int) int { if a < b { return a }; return b }
func maxBotInt(a,b int) int { if a > b { return a }; return b }
