package bot

import (
	"image"
	"math"
	"time"

	"github.com/Ducky705/ClashGO/internal/game"
	"github.com/Ducky705/ClashGO/internal/paths"
	"github.com/Ducky705/ClashGO/internal/vision"
	"gocv.io/x/gocv"
)

// sleepResponsive waits without making Stop feel frozen.
func (b *Bot) sleepResponsive(d time.Duration) bool {
	if d <= 0 {
		return b.ctx.Err() == nil
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-b.ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

// captureFailureDiagnostic takes a fresh frame only when a flow step failed.
// Keeping this out of the happy path avoids extra ADB captures during normal
// operation while preserving useful evidence when recovery is needed.
func (b *Bot) captureFailureDiagnostic(name string, extra map[string]interface{}) {
	screen, err := b.runtimeFrameFresh(2 * time.Second)
	if err != nil || screen.Empty() {
		if !screen.Empty() {
			screen.Close()
		}
		return
	}
	defer screen.Close()
	b.DumpDiagnostics(name, screen, extra)
}

func (b *Bot) findLocalizedFindMatchButton(screen gocv.Mat, roi image.Rectangle) (int, int, bool) {
	if screen.Empty() || roi.Dx() < 10 || roi.Dy() < 10 {
		return 0, 0, false
	}
	bounds := image.Rect(0, 0, screen.Cols(), screen.Rows())
	roi = roi.Intersect(bounds)
	if roi.Dx() < 10 || roi.Dy() < 10 {
		return 0, 0, false
	}

	sub := screen.Region(roi)
	defer sub.Close()
	hsv := vision.GetMat(sub.Rows(), sub.Cols(), gocv.MatTypeCV8UC3)
	defer vision.PutMat(hsv)
	mask := vision.GetMat(sub.Rows(), sub.Cols(), gocv.MatTypeCV8UC1)
	defer vision.PutMat(mask)
	gocv.CvtColor(sub, &hsv, gocv.ColorBGRToHSV)

	// CoC's Find Match action is a large saturated yellow/orange button.
	// Detecting the button body instead of its text keeps this path language
	// independent (French/English/etc.).
	gocv.InRangeWithScalar(
		hsv,
		gocv.NewScalar(7, 105, 120, 0),
		gocv.NewScalar(48, 255, 255, 0),
		&mask,
	)

	contours := gocv.FindContours(mask, gocv.RetrievalExternal, gocv.ChainApproxSimple)
	defer contours.Close()

	bestArea := 0.0
	best := image.Rectangle{}
	scaleArea := math.Max(0.1, b.cal.ScaleX*b.cal.ScaleY)
	for i := 0; i < contours.Size(); i++ {
		c := contours.At(i)
		area := gocv.ContourArea(c)
		refArea := area / scaleArea
		if refArea < 900 || refArea > 45000 {
			continue
		}
		r := gocv.BoundingRect(c)
		if r.Dx() < 20 || r.Dy() < 16 {
			continue
		}
		aspect := float64(r.Dx()) / float64(r.Dy())
		if aspect < 1.35 || aspect > 7.5 {
			continue
		}
		fill := area / float64(r.Dx()*r.Dy())
		if fill < 0.32 {
			continue
		}
		if area > bestArea {
			bestArea = area
			best = r
		}
	}
	if bestArea <= 0 {
		return 0, 0, false
	}
	return roi.Min.X + best.Min.X + best.Dx()/2,
		roi.Min.Y + best.Min.Y + best.Dy()/2,
		true
}

// waitAndClickButton waits for visual evidence of a button and clicks it as
// soon as it is visible. It replaces fixed post-click sleeps in the hot
// village -> attack -> army -> battle path.
//
// Fast path:
//   - poll every 120ms;
//   - use a strong color check for the Attack button / secondary Battle;
//   - otherwise use the existing ROI-scoped template matcher.
//
// Safety path:
//   - transient dialogs are dismissed while waiting;
//   - missing template assets fail closed instead of clicking blindly;
//   - if visual evidence never appears before timeout, the caller recovers
//     from the failed step rather than firing a stale coordinate.
func (b *Bot) waitAndClickButton(templateName, stepName string, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)

	var tpl gocv.Mat
	hasTemplate := false
	if b.templates != nil {
		if t, ok := b.templates.Get(templateName); ok {
			tpl = t
			hasTemplate = true
		}
	}

	// If there is no template, do not blindly tap an assumed coordinate.
	// A missing asset is a configuration/runtime-integrity problem; failing
	// closed is safer than turning a fast path into a random UI action.
	if !hasTemplate &&
		templateName != "btn_attack" &&
		templateName != "btn_battle" &&
		templateName != "btn_find_match" {
		b.logger.Error().
			Str("step", stepName).
			Str("template", templateName).
			Msg("required UI template unavailable; refusing blind action")
		return false
	}

	roi := b.buttonROI(templateName)
	physROI := image.Rect(
		int(float64(roi.Min.X)*b.cal.ScaleX),
		int(float64(roi.Min.Y)*b.cal.ScaleY),
		int(float64(roi.Max.X)*b.cal.ScaleX),
		int(float64(roi.Max.Y)*b.cal.ScaleY),
	)

	var lastInterruptAttempt time.Time
	for time.Now().Before(deadline) {
		if b.ctx.Err() != nil {
			return false
		}

		screen, err := b.runtimeFrameFresh(2 * time.Second)
		if err != nil {
			if !b.sleepResponsive(120 * time.Millisecond) {
				return false
			}
			continue
		}
		if screen.Empty() {
			screen.Close()
			if !b.sleepResponsive(120 * time.Millisecond) {
				return false
			}
			continue
		}

		clickX, clickY := 0, 0
		matched := false
		confidence := 0.0

		// Cheap, high-confidence color evidence before the more expensive
		// multiscale template pass.
		if templateName == "btn_find_match" {
			if x, y, ok := b.locateFindMatchButtonColor(screen); ok {
				clickX, clickY = x, y
				matched = true
				confidence = 1.0
			}
		}
		if templateName == "btn_attack" {
			if pp, ok := villagePinpoints[templateName]; ok {
				px, py := b.cal.ScaleRef(pp.X, pp.Y)
				if b.isOrange(screen, px, py) {
					clickX, clickY = px, py
					matched = true
					confidence = 1.0
				}
			}
		}
		if !matched && templateName == "btn_battle" {
			altX, altY := b.cal.ScaleRef(525, 247)
			if b.isGreen(screen, altX, altY) {
				clickX, clickY = altX, altY
				matched = true
				confidence = 1.0
			}
		}

		if !matched && templateName == "btn_find_match" {
			if x, y, ok := b.findLocalizedFindMatchButton(screen, physROI); ok {
				clickX, clickY = x, y
				confidence = 0.93
				matched = true
			}
		}

		if !matched && hasTemplate {
			matches, matchErr := vision.MatchMultiScaleROICached(
				screen, tpl, templateName, 0.2, 2.0, 5, 0.45, physROI,
			)
			if matchErr == nil && len(matches) > 0 {
				clickX, clickY = matches[0].Point.X, matches[0].Point.Y
				confidence = matches[0].Confidence
				matched = true
			}
		}

		if matched {
			if b.cfg.Debug.SaveScreenshots {
				_ = gocv.IMWrite(paths.ResolveConfig("diag_flow_"+templateName+".png"), screen)
			}
			screen.Close()

			b.logger.Info().
				Str("step", stepName).
				Float64("confidence", confidence).
				Int("x", clickX).
				Int("y", clickY).
				Msg("visual target ready; clicking " + stepName)
			if err := b.client.TapFast(clickX, clickY, 1.2); err != nil {
				b.logger.Warn().Err(err).Str("step", stepName).Msg("visual target tap failed")
				return false
			}
			b.recordActivity()
			return true
		}

		state, _ := b.classify(screen)
		screen.Close()

		if isTransientRuntimeState(state) && time.Since(lastInterruptAttempt) >= 500*time.Millisecond {
			lastInterruptAttempt = time.Now()
			b.logger.Debug().Str("state", state.String()).Str("step", stepName).Msg("transient UI while waiting for target; clearing")
			b.dismissInterruptions()
		}

		if !b.sleepResponsive(120 * time.Millisecond) {
			return false
		}
	}

	b.logger.Warn().
		Str("step", stepName).
		Dur("visual_timeout", timeout).
		Msg("visual target not confirmed for " + stepName + "; refusing blind fallback tap")
	return false
}

// waitForUIEvidence waits until either the expected state or the supplied
// template is visible. It is used for geometry-only actions (saved army slots
// >1) so coordinates are never fired before the menu is actually open.
func (b *Bot) waitForUIEvidence(templateName string, expected game.GameState, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)

	var tpl gocv.Mat
	hasTemplate := false
	if b.templates != nil {
		if t, ok := b.templates.Get(templateName); ok {
			tpl = t
			hasTemplate = true
		}
	}

	for time.Now().Before(deadline) {
		if b.ctx.Err() != nil {
			return false
		}
		screen, err := b.runtimeFrameFresh(2 * time.Second)
		if err != nil {
			if !b.sleepResponsive(120 * time.Millisecond) {
				return false
			}
			continue
		}
		if screen.Empty() {
			screen.Close()
			if !b.sleepResponsive(120 * time.Millisecond) {
				return false
			}
			continue
		}

		state, _ := b.classify(screen)
		if state == expected {
			screen.Close()
			return true
		}

		if hasTemplate {
			roi := b.buttonROI(templateName)
			physROI := image.Rect(
				int(float64(roi.Min.X)*b.cal.ScaleX),
				int(float64(roi.Min.Y)*b.cal.ScaleY),
				int(float64(roi.Max.X)*b.cal.ScaleX),
				int(float64(roi.Max.Y)*b.cal.ScaleY),
			)
			matches, matchErr := vision.MatchMultiScaleROICached(
				screen, tpl, templateName, 0.2, 2.0, 5, 0.45, physROI,
			)
			if matchErr == nil && len(matches) > 0 {
				screen.Close()
				return true
			}
		}
		screen.Close()

		if !b.sleepResponsive(120 * time.Millisecond) {
			return false
		}
	}
	return false
}

func isTransientRuntimeState(state game.GameState) bool {
	switch state {
	case game.StateObstacleDialog,
		game.StateGemDialog,
		game.StateChatOpen,
		game.StateShieldInfo,
		game.StateWelcomeBack,
		game.StateChestReward,
		game.StateTapToContinue,
		game.StateNewsSplash,
		game.StateConnectionLost,
		game.StateConfirmExit:
		return true
	default:
		return false
	}
}
