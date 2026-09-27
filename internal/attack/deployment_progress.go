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


// deploySpellBatchVerified verifies that a spell card changes after taps. Spells
// are intentionally NOT passed through the troop red-zone resolver because
// Clash allows spells to be placed inside the village.
func (e *Executor) deploySpellBatchVerified(slot image.Point, pts []image.Point) bool {
	if len(pts) == 0 {
		return false
	}
	if len(pts) > 3 {
		pts = pts[:3]
	}

	before, err := e.client.CaptureToMat()
	if err != nil || before.Empty() {
		if !before.Empty() {
			before.Close()
		}
		tapDeployPoints(e, pts)
		return true
	}
	defer before.Close()

	tapDeployPoints(e, pts)
	e.client.HumanSleep(150, 25)

	after, err := e.client.CaptureToMat()
	if err != nil || after.Empty() {
		if !after.Empty() {
			after.Close()
		}
		return true
	}
	empty := e.isSlotEmpty(after, slot.X, slot.Y)
	delta := e.slotVisualDelta(before, after, slot)
	after.Close()
	if empty || delta >= 0.018 {
		return true
	}

	// One controlled retry after reselecting the same card. A spell rejected
	// for transient input/focus reasons should not make us spam indefinitely.
	e.logger.Warn().
		Float64("slot_delta", delta).
		Msg("spell batch produced no card progress; reselecting and retrying once")
	e.client.TapFast(slot.X, slot.Y, 2.0)
	e.client.HumanSleep(55, 10)
	tapDeployPoints(e, pts)
	e.client.HumanSleep(150, 25)

	final, err := e.client.CaptureToMat()
	if err != nil || final.Empty() {
		if !final.Empty() {
			final.Close()
		}
		return true
	}
	defer final.Close()
	return e.isSlotEmpty(final, slot.X, slot.Y) || e.slotVisualDelta(before, final, slot) >= 0.018
}

func lineDeployPoints(p1, p2 image.Point, count int) []image.Point {
	if count <= 0 {
		return nil
	}
	if count == 1 || p1 == p2 {
		return []image.Point{p1}
	}
	out := make([]image.Point, 0, count)
	for i := 0; i < count; i++ {
		pct := float64(i) / float64(count-1)
		out = append(out, image.Pt(
			int(float64(p1.X)+float64(p2.X-p1.X)*pct),
			int(float64(p1.Y)+float64(p2.Y-p1.Y)*pct),
		))
	}
	return out
}

// recoverRemainingSlot is the common recovery path used both by the early sweep
// and the final deployment verification pass. Troops/CC use a fresh selected-
// unit red overlay and safe-point resolution; spells use their normal in-base
// line. Every batch is followed by proof that the card changed.
func (e *Executor) recoverRemainingSlot(slot TroopSlot, pCfg PrecisionConfig, targetEdge string, w, h int) bool {
	if slot.Category == "Siege" || e.isSiegeTapped(slot.X, w) {
		return true
	}

	slotPt := image.Pt(slot.X, slot.Y)
	e.client.TapFast(slot.X, slot.Y, 2.0)
	e.client.HumanSleep(55, 10)

	var p1, p2 image.Point
	if slot.Category == "Spell" {
		if edge, ok := pCfg.SpellEdgesB[targetEdge]; ok {
			p1, p2 = edge.P1, edge.P2
			centerX, centerY := w/2, h/2
			pct := 0.20
			p1 = image.Pt(
				int(float64(p1.X)+float64(centerX-p1.X)*pct),
				int(float64(p1.Y)+float64(centerY-p1.Y)*pct),
			)
			p2 = image.Pt(
				int(float64(p2.X)+float64(centerX-p2.X)*pct),
				int(float64(p2.Y)+float64(centerY-p2.Y)*pct),
			)
		}
	}
	if p1 == (image.Point{}) && p2 == (image.Point{}) {
		if edge, ok := pCfg.Edges[targetEdge]; ok {
			p1, p2 = edge.P1, edge.P2
		} else {
			p1 = image.Pt(w/2, int(float64(h)*0.65))
			p2 = p1
		}
	}

	planned := lineDeployPoints(p1, p2, 9)
	if slot.Category != "Spell" {
		overlay, err := e.client.CaptureToMat()
		if err == nil && !overlay.Empty() {
			planned = e.adaptDeployPointsToBoundary(overlay, planned)
			overlay.Close()
		} else if !overlay.Empty() {
			overlay.Close()
		}
		if len(planned) == 0 {
			e.logger.Warn().Int("x", slot.X).Msg("recovery pass found no safe troop deployment points")
			return false
		}
	}

	for i := 0; i < len(planned); i += 3 {
		end := i + 3
		if end > len(planned) {
			end = len(planned)
		}
		batch := append([]image.Point(nil), planned[i:end]...)
		ok := false
		if slot.Category == "Spell" {
			ok = e.deploySpellBatchVerified(slotPt, batch)
		} else {
			ok = e.deployTroopBatchVerified(slotPt, batch)
		}
		if !ok {
			e.logger.Warn().
				Int("x", slot.X).
				Str("category", slot.Category).
				Msg("recovery deployment batch not confirmed")
			return false
		}

		check, err := e.client.CaptureToMat()
		if err == nil && !check.Empty() {
			empty := e.isSlotEmpty(check, slot.X, slot.Y)
			check.Close()
			if empty {
				e.logger.Info().Int("x", slot.X).Str("category", slot.Category).Msg("recovery slot emptied successfully")
				return true
			}
		} else if !check.Empty() {
			check.Close()
		}
		e.client.HumanSleep(80, 15)
	}

	final, err := e.client.CaptureToMat()
	if err != nil || final.Empty() {
		if !final.Empty() {
			final.Close()
		}
		return false
	}
	defer final.Close()
	return e.isSlotEmpty(final, slot.X, slot.Y)
}
