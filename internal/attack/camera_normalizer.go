package attack

import (
	"image"
	"runtime"
	"time"

	"gocv.io/x/gocv"
)

const (
	cameraNormalizeMaxZooms = 3
	cameraNormalizeMaxPans  = 2
	// Hard SLA for camera preparation. The whole accept->plan path should stay
	// in the "few seconds" range even on a base that needs multiple adjustments.
	cameraNormalizeBudget   = 3 * time.Second
)

// deploymentMargins describes the legal screen space outside the detected
// red no-deploy bounding box. Bottom is clipped to uiCutoff so the troop bar
// is never treated as usable battlefield space.
type deploymentMargins struct {
	Left   int
	Right  int
	Top    int
	Bottom int
}

func marginsForRedZone(zone RedZone, w, uiCutoff int) deploymentMargins {
	if !zone.Valid {
		return deploymentMargins{}
	}
	return deploymentMargins{
		Left:   zone.BBox.Min.X,
		Right:  w - zone.BBox.Max.X,
		Top:    zone.BBox.Min.Y,
		Bottom: uiCutoff - zone.BBox.Max.Y,
	}
}

func marginForSide(m deploymentMargins, side string) int {
	switch side {
	case "left":
		return m.Left
	case "right":
		return m.Right
	case "top":
		return m.Top
	case "bottom":
		return m.Bottom
	default:
		best := m.Left
		if m.Right > best {
			best = m.Right
		}
		if m.Top > best {
			best = m.Top
		}
		if m.Bottom > best {
			best = m.Bottom
		}
		return best
	}
}

func cameraSafeMargin(w int) int {
	margin := int(82.0 * float64(w) / 860.0)
	if margin < 58 {
		margin = 58
	}
	return margin
}

// preferredPan moves the village away from the intended deploy side. The
// gesture remains in the playable region and never enters the lower troop HUD.
func preferredPan(side string, w, uiCutoff int) (from, to image.Point, ok bool) {
	cx := w / 2
	cy := int(float64(uiCutoff) * 0.48)
	dx := int(float64(w) * 0.16)
	dy := int(float64(uiCutoff) * 0.13)

	from = image.Pt(cx, cy)
	to = from
	switch side {
	case "left":
		to.X += dx
	case "right":
		to.X -= dx
	case "top":
		to.Y += dy
	case "bottom":
		to.Y -= dy
	default:
		return image.Point{}, image.Point{}, false
	}
	return from, to, true
}

// normalizeBattlefieldCamera establishes a repeatable pre-deploy camera state.
//
// It deliberately does NOT own troop placement. Its only responsibilities are:
//   1. inspect the live red deployment boundary;
//   2. zoom out in bounded host-safe steps when the intended side is cramped;
//   3. pan the village away from the intended deploy side only if needed;
//   4. re-capture and re-detect after every camera movement.
//
// This keeps the proven generic planner responsible for red-zone geometry,
// slot resolution and unit placement while making its input view deterministic.
//
// returnedOwned is true when returnedScreen is a fresh capture owned by the
// caller. The original screen is never closed here.
func (e *Executor) normalizeBattlefieldCamera(
	screen gocv.Mat,
	targetEdge string,
	uiCutoff int,
) (returnedScreen gocv.Mat, returnedOwned bool, zone RedZone) {
	w, h := screen.Cols(), screen.Rows()
	detector := NewRedLineDetector(e.logger)
	preferredSide := cornerToSide(targetEdge)
	if preferredSide == "" {
		preferredSide = targetEdge
	}

	current := screen
	owned := false
	zone = detector.Detect(current, uiCutoff)
	required := cameraSafeMargin(w)
	started := time.Now()
	budgetExpired := func() bool { return time.Since(started) >= cameraNormalizeBudget }

	logState := func(stage string, attempt int) {
		margins := marginsForRedZone(zone, w, uiCutoff)
		e.logger.Info().
			Str("stage", stage).
			Int("attempt", attempt).
			Str("target_edge", targetEdge).
			Str("preferred_side", preferredSide).
			Bool("red_zone_valid", zone.Valid).
			Interface("red_bbox", zone.BBox).
			Int("preferred_free_space", marginForSide(margins, preferredSide)).
			Int("required_free_space", required).
			Msg("battlefield camera normalization")
	}

	replaceWithFresh := func(stage string, attempt int) bool {
		fresh, err := e.captureFrame(2 * time.Second)
		if err != nil || fresh.Empty() {
			if !fresh.Empty() {
				fresh.Close()
			}
			e.logger.Warn().
				Err(err).
				Str("stage", stage).
				Int("attempt", attempt).
				Msg("battlefield camera recapture failed")
			return false
		}
		if owned && !current.Empty() {
			current.Close()
		}
		current = fresh
		owned = true
		zone = detector.Detect(current, uiCutoff)
		logState(stage, attempt)
		return true
	}

	logState("initial", 0)

	if runtime.GOOS == "windows" {
		for attempt := 1; attempt <= cameraNormalizeMaxZooms; attempt++ {
			if budgetExpired() {
				e.logger.Warn().Dur("elapsed", time.Since(started)).Msg("camera normalization budget reached during zoom stage")
				break
			}
			margins := marginsForRedZone(zone, w, uiCutoff)
			if zone.Valid && marginForSide(margins, preferredSide) >= required {
				break
			}

			e.logger.Info().
				Int("attempt", attempt).
				Str("preferred_side", preferredSide).
				Msg("camera normalization: safe zoom-out")
			if err := e.client.ZoomOutSafe(); err != nil {
				e.logger.Warn().Err(err).Msg("camera normalization: zoom-out failed")
				break
			}
			// One calm render window: do not poll/capture through the zoom
			// animation, which previously created extra BlueStacks pressure.
			time.Sleep(350 * time.Millisecond)
			if !replaceWithFresh("zoom_out", attempt) {
				break
			}
		}

		// Zoom establishes scale first. Pan only when that scale still leaves
		// the intended side cramped. Each pan is followed by exactly one fresh
		// capture and one red-zone pass.
		for attempt := 1; attempt <= cameraNormalizeMaxPans; attempt++ {
			if budgetExpired() {
				e.logger.Warn().Dur("elapsed", time.Since(started)).Msg("camera normalization budget reached during pan stage")
				break
			}
			margins := marginsForRedZone(zone, w, uiCutoff)
			if zone.Valid && marginForSide(margins, preferredSide) >= required {
				break
			}
			from, to, ok := preferredPan(preferredSide, w, uiCutoff)
			if !ok {
				break
			}
			e.logger.Info().
				Int("attempt", attempt).
				Str("preferred_side", preferredSide).
				Interface("from", from).
				Interface("to", to).
				Msg("camera normalization: exposing deployment side")
			if err := e.client.Swipe(from.X, from.Y, to.X, to.Y, 280); err != nil {
				e.logger.Warn().Err(err).Msg("camera normalization: pan failed")
				break
			}
			time.Sleep(325 * time.Millisecond)
			if !replaceWithFresh("pan", attempt) {
				break
			}
		}
	}

	margins := marginsForRedZone(zone, w, uiCutoff)
	e.logger.Info().
		Str("target_edge", targetEdge).
		Str("preferred_side", preferredSide).
		Bool("red_zone_valid", zone.Valid).
		Interface("red_bbox", zone.BBox).
		Int("preferred_free_space", marginForSide(margins, preferredSide)).
		Int("required_free_space", required).
		Int("screen_w", w).
		Int("screen_h", h).
		Dur("camera_ms", time.Since(started)).
		Msg("battlefield camera normalized; handing frame to deploy planner")

	return current, owned, zone
}
