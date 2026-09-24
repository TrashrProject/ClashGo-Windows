package attack

import (
	"image"
	"math"
	"time"

	"gocv.io/x/gocv"
)

// slotVisualDelta measures how much the selected troop card changed between
// two frames. When Clash accepts a deployment, the amount badge / card shading
// changes immediately. This gives us a cheap proof-of-progress signal without
// needing OCR for every single troop tap.
func (e *Executor) slotVisualDelta(before, after gocv.Mat, slot image.Point) float64 {
	if before.Empty() || after.Empty() {
		return 0
	}
	radius := maxInt(12, int(math.Round(18*e.cal.ScaleX)))
	x0 := maxInt(0, slot.X-radius)
	y0 := maxInt(0, slot.Y-radius)
	x1 := minInt(minInt(before.Cols(), after.Cols())-1, slot.X+radius)
	y1 := minInt(minInt(before.Rows(), after.Rows())-1, slot.Y+radius)
	if x1 <= x0 || y1 <= y0 {
		return 0
	}

	var total float64
	var samples int
	for y := y0; y <= y1; y += 2 {
		for x := x0; x <= x1; x += 2 {
			for c := 0; c < 3; c++ {
				a := int(before.GetUCharAt(y, x*3+c))
				b := int(after.GetUCharAt(y, x*3+c))
				d := a - b
				if d < 0 {
					d = -d
				}
				total += float64(d)
				samples++
			}
		}
	}
	if samples == 0 {
		return 0
	}
	return total / (255.0 * float64(samples))
}

func tapDeployPoints(e *Executor, pts []image.Point) {
	switch len(pts) {
	case 0:
		return
	case 1:
		e.client.TapFast(pts[0].X, pts[0].Y, 12.0)
	case 2:
		e.client.TapDual(pts[0].X, pts[0].Y, 12.0, pts[1].X, pts[1].Y, 12.0)
	default:
		e.client.TapTriple(
			pts[0].X, pts[0].Y, 12.0,
			pts[1].X, pts[1].Y, 12.0,
			pts[2].X, pts[2].Y, 12.0,
		)
	}
}

// deployTroopBatchVerified enforces the runtime principle:
// visual proof -> action -> proof of progress.
//
// It checks the troop card before and after a deployment batch. If the card did
// not visibly change, Clash probably rejected the taps (red zone, stale
// selection or missed input). The executor then reselects the card, reacquires
// the live red overlay, moves the target slightly farther outside the village
// and retries once. We never hammer the same rejected coordinates repeatedly.
func (e *Executor) deployTroopBatchVerified(slot image.Point, pts []image.Point) bool {
	if len(pts) == 0 {
		return false
	}
	if len(pts) > 3 {
		pts = pts[:3]
	}

	before, beforeErr := e.client.CaptureToMat()
	if beforeErr != nil || before.Empty() {
		if !before.Empty() {
			before.Close()
		}
		tapDeployPoints(e, pts)
		return true
	}

	tapDeployPoints(e, pts)
	e.client.HumanSleep(135, 25)

	after, afterErr := e.client.CaptureToMat()
	if afterErr == nil && !after.Empty() {
		empty := e.isSlotEmpty(after, slot.X, slot.Y)
		delta := e.slotVisualDelta(before, after, slot)
		before.Close()

		if empty || delta >= 0.018 {
			e.logger.Debug().
				Float64("slot_delta", delta).
				Bool("slot_empty", empty).
				Msg("deployment batch visually confirmed")
			after.Close()
			return true
		}

		e.logger.Warn().
			Float64("slot_delta", delta).
			Msg("deployment taps produced no troop-card progress; relocating and retrying once")
		after.Close()
	} else {
		before.Close()
		if !after.Empty() {
			after.Close()
		}
		// No verification frame: do not duplicate the batch blindly.
		return true
	}

	// Re-select so the red no-deploy overlay is guaranteed visible for a
	// fresh relocation pass.
	e.client.TapFast(slot.X, slot.Y, 2.0)
	e.client.HumanSleep(55, 10)

	overlay, err := e.client.CaptureToMat()
	if err != nil || overlay.Empty() {
		if !overlay.Empty() {
			overlay.Close()
		}
		return false
	}
	defer overlay.Close()

	center := image.Pt(overlay.Cols()/2, int(float64(overlay.Rows())*0.43))
	retry := make([]image.Point, 0, len(pts))
	for _, p := range pts {
		dx := float64(p.X-center.X)
		dy := float64(p.Y-center.Y)
		l := math.Hypot(dx, dy)
		if l < 1 {
			l = 1
		}
		nudge := math.Max(12, 18*e.cal.ScaleX)
		candidate := image.Pt(
			p.X+int(math.Round(dx/l*nudge)),
			p.Y+int(math.Round(dy/l*nudge)),
		)
		if safe, ok := e.resolveSafeDeployPoint(overlay, candidate); ok {
			retry = append(retry, safe)
		}
	}
	if len(retry) == 0 {
		e.logger.Warn().Msg("retry suppressed: no safe alternative deployment point")
		return false
	}

	retryBefore := overlay.Clone()
	defer retryBefore.Close()
	tapDeployPoints(e, retry)
	time.Sleep(150 * time.Millisecond)

	final, err := e.client.CaptureToMat()
	if err != nil || final.Empty() {
		if !final.Empty() {
			final.Close()
		}
		return true
	}
	defer final.Close()

	empty := e.isSlotEmpty(final, slot.X, slot.Y)
	delta := e.slotVisualDelta(retryBefore, final, slot)
	ok := empty || delta >= 0.018
	e.logger.Info().
		Bool("confirmed", ok).
		Bool("slot_empty", empty).
		Float64("slot_delta", delta).
		Int("retry_points", len(retry)).
		Msg("deployment relocation retry result")
	return ok
}
