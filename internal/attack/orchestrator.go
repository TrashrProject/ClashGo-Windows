package attack

import (
	"encoding/json"
	"fmt"
	"image"
	"math/rand"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/Ducky705/ClashGO/pkg/formula"
	"github.com/Ducky705/ClashGO/pkg/strategy"
	"gocv.io/x/gocv"
)

// DeployDynamicV2 deploys troops using dynamic red line detection.
// No hardcoded precision_config.json needed - detects deployment boundary live.
//
// strategyPath is the on-disk YAML path. The orchestrator uses it to find
// the matching formula.json (loaded as <stem>_formula.json next to the
// YAML). Pass "" to skip formula lookup entirely.
func (e *Executor) DeployDynamicV2(s *strategy.DynamicStrategy, screen gocv.Mat, strategyPath string) (int, error) {
	w, h := screen.Cols(), screen.Rows()
	targetEdge := s.TargetEdge

	// Record the active strategy so the battle-end wait can honor
	// per-strategy knobs (e.g. end_at_percent auto-end threshold).
	e.activeStrategy = s

	// Pre-flight validation
	if err := e.Validate(s); err != nil {
		e.logger.Error().Err(err).Msg("pre-flight validation failed")
		return 0, err
	}

	// 0. Resolve "Rotate" / "Random" targetEdge BEFORE we read configuration
	//    that depends on it. Previously this happened AFTER the red-zone
	//    pass, so heroes/sweep saw a different edge than troops — a silent
	//    coordinate-mismatch bug.
	switch {
	case strings.EqualFold(targetEdge, "Rotate"):
		// EqualFold is intentional: YAML authors may type "rotate" or
		// "ROTATE" or "Rotate" and the bot should not silently fall
		// through to the per-corner default. Parity with the "Random"
		// branch below, which uses the same case-insensitive match.
		//
		// Cycle through the 4 corners in a top→right→bottom→left
		// pattern via a persistent on-disk counter. The bot distributes
		// attacks evenly across sides over multiple runs instead of
		// re-picking TopLeft every process restart. See
		// rotation_state.go for the failure-mode / concurrency story.
		targetEdge = NextEdgeIndex()
		e.logger.Info().Str("edge", targetEdge).Msg("rotated to next edge")
	case strings.EqualFold(targetEdge, "Random"):
		edges := []string{"TopLeft", "TopRight", "BottomLeft", "BottomRight"}
		targetEdge = edges[rand.Intn(len(edges))]
		e.logger.Info().Str("edge", targetEdge).Msg("random edge selected")
	}

	// 1. Detect red zone (deployment boundary)
	redDetector := NewRedLineDetector(e.logger)
	uiCutoff := int(float64(h) * 0.85) // above troop bar
	redZone := redDetector.Detect(screen, uiCutoff)

	// Windows adaptive camera search.
	//
	// A static screenshot is not enough when the village is zoomed-in or
	// shifted against an edge: there may literally be no safe strip behind
	// the red deployment boundary. Before choosing any troop-drop line, let
	// the bot manipulate the map like a player would: zoom OUT, re-detect,
	// then pan the map to expose more legal terrain. Every gesture is followed
	// by a fresh capture + fresh red-line detection. We never deploy from stale
	// pre-gesture coordinates.
	deployScreen := screen
	var cameraFrame gocv.Mat
	cameraFrameOwned := false
	defer func() {
		if cameraFrameOwned && !cameraFrame.Empty() {
			cameraFrame.Close()
		}
	}()

	if runtime.GOOS == "windows" {
		freeSpace := func(z RedZone) (string, int) {
			if !z.Valid {
				return "", 0
			}
			free := map[string]int{
				"left":   z.BBox.Min.X,
				"right":  w - z.BBox.Max.X,
				"top":    z.BBox.Min.Y,
				"bottom": uiCutoff - z.BBox.Max.Y,
			}
			side := "left"
			best := free[side]
			for _, s := range []string{"right", "top", "bottom"} {
				if free[s] > best {
					side, best = s, free[s]
				}
			}
			return side, best
		}

		refreshCamera := func(reason string) bool {
			fresh, err := e.client.CaptureToMat()
			if err != nil || fresh.Empty() {
				if !fresh.Empty() {
					fresh.Close()
				}
				e.logger.Warn().Err(err).Str("reason", reason).Msg("adaptive camera capture failed")
				return false
			}
			if cameraFrameOwned && !cameraFrame.Empty() {
				cameraFrame.Close()
			}
			cameraFrame = fresh
			cameraFrameOwned = true
			deployScreen = cameraFrame
			redZone = redDetector.Detect(deployScreen, uiCutoff)
			side, free := freeSpace(redZone)
			e.logger.Info().
				Str("reason", reason).
				Bool("red_zone_valid", redZone.Valid).
				Str("best_side", side).
				Int("free_space", free).
				Msg("adaptive camera re-evaluated deployment space")
			return true
		}

		// Aim for a meaningful strip outside the red line, not merely a few
		// pixels. ~90px on the 860-wide reference frame leaves enough room for
		// the line itself, contour error and multiple troop taps.
		minSafeFree := int(90.0 * float64(w) / 860.0)
		if minSafeFree < 64 {
			minSafeFree = 64
		}

		side, free := freeSpace(redZone)
		e.logger.Info().
			Bool("red_zone_valid", redZone.Valid).
			Str("best_side", side).
			Int("free_space", free).
			Int("required_free_space", minSafeFree).
			Msg("adaptive camera evaluating battlefield")

		// IMPORTANT (Windows / BlueStacks):
		// Do NOT use Client.ZoomOut()/ZoomIn() here. Those methods inject
		// low-level multi-touch sendevent batches. BlueStacks 5 on the user's
		// Pie64 instance can terminate the emulator process under repeated
		// synthetic multi-touch. Startup already had a Windows-safe path that
		// deliberately skipped native zoom for this reason.
		//
		// Keep adaptive camera movement to single-pointer map pans only. They
		// are handled by Android's normal input swipe path and are much more
		// stable on BlueStacks.
		if !redZone.Valid || free < minSafeFree {
			e.logger.Info().
				Int("free_space", free).
				Int("required_free_space", minSafeFree).
				Msg("adaptive camera: native pinch zoom disabled on Windows-safe path; using map pan only")
		}

		// Drag the MAP toward the opposite
		// direction so the already-best legal side gains even more empty land.
		// Gestures stay in the playfield, well above the troop bar.
		for panTry := 1; panTry <= 2 && redZone.Valid && free < minSafeFree; panTry++ {
			cx := w / 2
			cy := int(float64(uiCutoff) * 0.52)
			dx := int(float64(w) * 0.20)
			dy := int(float64(uiCutoff) * 0.18)
			x2, y2 := cx, cy

			switch side {
			case "left":
				// Move village right -> expose more legal space on left.
				x2 = cx + dx
			case "right":
				x2 = cx - dx
			case "top":
				y2 = cy + dy
			case "bottom":
				y2 = cy - dy
			}

			e.logger.Info().
				Int("attempt", panTry).
				Str("target_safe_side", side).
				Int("from_x", cx).Int("from_y", cy).
				Int("to_x", x2).Int("to_y", y2).
				Msg("adaptive camera: panning map to expose legal deployment area")

			if err := e.client.Swipe(cx, cy, x2, y2, 260); err != nil {
				e.logger.Warn().Err(err).Msg("adaptive camera map pan failed")
				break
			}
			time.Sleep(360 * time.Millisecond)
			if !refreshCamera("map_pan") {
				break
			}
			side, free = freeSpace(redZone)
		}

		// No native pinch-zoom recovery on Windows. If panning lost the red
		// boundary, keep the failure visible to the caller rather than sending
		// an unsafe multi-touch gesture that can crash BlueStacks.
		if !redZone.Valid && cameraFrameOwned {
			e.logger.Warn().Msg("adaptive camera lost red boundary after pan; refusing unsafe Windows pinch-zoom recovery")
		}

		side, free = freeSpace(redZone)
		e.logger.Info().
			Bool("red_zone_valid", redZone.Valid).
			Str("selected_side", side).
			Int("free_space", free).
			Msg("adaptive camera search complete")
	}

	// 2. Load precision config FIRST so we can detect user-pinned coords
	//    before computing the deploy line. "Pinned" = user-authored non-zero
	//    entries for the chosen target. If the user pinned something we
	//    respect it; otherwise the dynamic red-zone line takes over.
	var pCfg PrecisionConfig
	mBarY := int(float64(h) * 0.78)
	pData, ok := readConfigJSON("precision_config.json")
	if ok && json.Unmarshal(pData, &pCfg) == nil {
		scaleX, scaleY := float64(w)/float64(pCfg.Width), float64(h)/float64(pCfg.Height)
		for k, v := range pCfg.Edges {
			pCfg.Edges[k] = ManualEdge{
				P1: image.Pt(int(float64(v.P1.X)*scaleX), int(float64(v.P1.Y)*scaleY)),
				P2: image.Pt(int(float64(v.P2.X)*scaleX), int(float64(v.P2.Y)*scaleY)),
			}
		}
		for k, v := range pCfg.SpellEdgesA {
			pCfg.SpellEdgesA[k] = ManualEdge{
				P1: image.Pt(int(float64(v.P1.X)*scaleX), int(float64(v.P1.Y)*scaleY)),
				P2: image.Pt(int(float64(v.P2.X)*scaleX), int(float64(v.P2.Y)*scaleY)),
			}
		}
		for k, v := range pCfg.SpellEdgesB {
			pCfg.SpellEdgesB[k] = ManualEdge{
				P1: image.Pt(int(float64(v.P1.X)*scaleX), int(float64(v.P1.Y)*scaleY)),
				P2: image.Pt(int(float64(v.P2.X)*scaleX), int(float64(v.P2.Y)*scaleY)),
			}
		}
		for k, v := range pCfg.HeroTargets {
			pCfg.HeroTargets[k] = image.Pt(int(float64(v.X)*scaleX), int(float64(v.Y)*scaleY))
		}
		for k, v := range pCfg.SpellTargets {
			pCfg.SpellTargets[k] = image.Pt(int(float64(v.X)*scaleX), int(float64(v.Y)*scaleY))
		}
		if pCfg.Sides != nil {
			for k, v := range pCfg.Sides {
				pCfg.Sides[k] = ManualEdge{
					P1: image.Pt(int(float64(v.P1.X)*scaleX), int(float64(v.P1.Y)*scaleY)),
					P2: image.Pt(int(float64(v.P2.X)*scaleX), int(float64(v.P2.Y)*scaleY)),
				}
			}
		}
		mBarY = int(float64(pCfg.BarY) * scaleY)
		if mBarY > int(float64(h)*0.92) {
			mBarY = int(float64(h) * 0.92)
		}
	}

	// 2a. Detect user-pinned coords for the SPECIFIC chosen target.
	// A pin "exists" when targetEdge has non-zero coords in Edges, or a
	// matching side has non-zero coords in Sides. We deliberately avoid
	// falling back to phantom (0,0)→(0,0) entries (Go zero-default for
	// missing JSON keys) which would otherwise look "pinned".
	userPinnedForTarget := hasPinnedForTarget(pCfg, targetEdge)

	// 2b. If we have neither a red zone nor a pin, don't guess. The original
	// fall-through path deployed at x=60 regardless of base orientation —
	// that's how the legacy "scatters across the corner" symptom started.
	if !redZone.Valid && !userPinnedForTarget {
		return 0, fmt.Errorf("no red zone detected AND no user-pinned sides/edges in precision_config.json for target=%q — re-pin via `cmd/pick_coords -mode=strict` or capture a battle shot", targetEdge)
	}

	// 2c. If user pinned the chosen target, build the deploy line directly
	// from those pinned coords using a real linspace. We bypass textual
	// side mapping (BottomLeft → "bottom" / "left") to preserve the path
	// the user actually drew. Side is set to targetEdge as a stable identifier
	// for downstream consumers (Sweep, SpellLine).
	var pinnedLine DeployLine
	if userPinnedForTarget {
		if e2, ok := pCfg.Edges[targetEdge]; ok && !isZeroManualEdge(e2) {
			pinnedLine = manualEdgeToDeployLine(e2, targetEdge, linePoints)
		} else if side := cornerToSide(targetEdge); side != "" {
			if e2, ok := pCfg.Sides[side]; ok && !isZeroManualEdge(e2) {
				pinnedLine = manualEdgeToDeployLine(e2, targetEdge, linePoints)
			}
		}
		e.logger.Info().
			Str("target", targetEdge).
			Bool("red_zone_valid", redZone.Valid).
			Int("pinned_points", len(pinnedLine.Points)).
			Msg("using user-pinned deploy line; skipping dynamic override")
	}

	// 0a. Load formula.json (if present). When the user authored one via
	//     `cmd/design_attack`, it overrides the dynamic red-zone / corner-
	//     based deploy path so every unit drops on the chosen SIDE with
	//     coordinates the user actually pinned, not the legacy corner
	//     heuristics that kept attacking in the corner.
	//
	// strategyPath is the *YAML* path (passed by bot.go / debug_attack);
	// candidatePaths inside the formula loader computes the parallel
	// "<stem>_formula.json" location, which is the same directory the
	// user creates formula files in via cmd/design_attack.
	formulaPtr, hasFormula, ferr := formula.Load(strategyPath)
	if ferr != nil {
		e.logger.Warn().Err(ferr).Str("strategy_path", strategyPath).Msg("formula found but failed to parse")
	}
	if hasFormula {
		// Per-corner override (if present) wins over the mirror. The
		// user may have authored explicit coords for this corner via
		// `cmd/design_attack -corner BL` (which writes to
		// formula.corner_overrides.BL). This is more accurate than
		// reflecting the BR default — different base geometries have
		// different red-line positions on each side, and a mirror
		// across a non-symmetric base puts the line either too close
		// to the new side's red line (overlap) or too far from it.
		usedOverride := false
		if formulaPtr.CornerOverrides != nil {
			if cornerUnits, ok := formulaPtr.CornerOverrides[targetEdge]; ok {
				// Merge: per-corner overrides win per-unit. Typical
				// override is PARTIAL — only the units that differ
				// from the mirrored BR default. Units not in the
				// override fall through to formula.units so the
				// user only re-pins the units that actually need it.
				merged := make(map[string]formula.UnitEntry, len(formulaPtr.Units)+len(cornerUnits))
				for k, v := range formulaPtr.Units {
					merged[k] = v
				}
				for k, v := range cornerUnits {
					merged[k] = v
				}
				formulaPtr.Units = merged
				usedOverride = true
			}
		}
		if !usedOverride {
			// No explicit override for this corner: mirror the BR
			// default (formula.units) around the formula's authored
			// 860×732 reference frame. This is the simpler
			// replacement for the previous FourSides pattern: the
			// user authors ONE attack in cmd/design_attack, the
			// orchestrator mirrors it per-corner.
			formulaPtr.MirrorForCorner(targetEdge)
		}
		// Scale (the merged or mirrored units) to the live screen.
		// Doing mirror-before-scale keeps formula.Screen.W/H
		// meaningful as the formula's intent reference and avoids
		// "which center do we reflect around" ambiguity.
		formulaPtr.ApplyScreenScale(formulaPtr.Screen.W, formulaPtr.Screen.H, w, h)
		e.logger.Info().
			Str("strategy", s.Name).
			Int("units", len(formulaPtr.Units)).
			Int("formula_w", formulaPtr.Screen.W).
			Int("formula_h", formulaPtr.Screen.H).
			Int("screen_w", w).
			Int("screen_h", h).
			Msg("formula.json loaded; per-unit explicit coordinates will override edge-based deploy")
	}

	// 3. Calculate dynamic deployment line. We always compute it for the
	//    use-case where target is NOT user-pinned (the live red-zone path)
	//    OR for when red zone is valid even if pinned (so the dynamic line
	//    can serve as the deployLine passed to Sweep / SpellLine).
	deployCalc := NewDeployLineCalculator(e.logger)
	deployLine := deployCalc.Calculate(redZone, w, h, uiCutoff, cornerToSide(targetEdge), linePoints)

	// 3a. Apply corner override. When the user did NOT pin the target,
	//    all four corners get the dynamic red-zone line (original Duke-
	//    coherence fix preserved). When the user DID pin the target,
	//    ONLY the unpinned adjacent corners get overridden — so Duke's
	//    random adjacent-corner pick still lands on the actual chosen
	//    side, while the user's pinned target stays intact.
	//
	//    Sides (top/right/bottom/left) gets mirrored in both cases so
	//    SpotsForSide / future side-aware readers see the dynamic line.
	applyCornerOverride(&pCfg, deployLine, redZone.Valid, targetEdge)

	// 3b. Once the override is decided, the active deployLine is what
	//     Sweep and SpellLine read. If the user pinned (and forces — or
	//     chose — a real line), use it as the active deployLine so
	//     every consumer (troops, spells, sweep) hits the pin; else the
	//     dynamic line stands. This eliminates the "troops obey pin but
	//     spells/sweep scatter off-pin" partial-inconsistency bug.
	if userPinnedForTarget && len(pinnedLine.Points) >= 2 {
		deployLine = pinnedLine
	}

	// 4. Initialize SlotManager
	slotMgr := NewSlotManager(deployScreen, pCfg, w, h, mBarY, e.templates, e.classify, e.logger)
	if len(slotMgr.GetAllSlots()) == 0 {
		return 0, fmt.Errorf("no active slots detected")
	}

	// 5. Detect troop counts
	troopCounter := NewTroopCounter(pCfg.Width, pCfg.Height, e.logger)
	troopCounts := troopCounter.DetectCounts(deployScreen, slotMgr.GetAllSlots(), mBarY)
	countMap := GetAllCounts(troopCounts)
	e.logger.Info().Interface("counts", countMap).Msg("detected troop counts")

	// Windows-safe deployment path.
	//
	// The legacy dynamic planner depends on historical manual_slots /
	// manual_labels / formula mappings that were calibrated on another
	// layout. On BlueStacks Windows the bot can reach Battle correctly but
	// then repeatedly select/reconcile the wrong cards, which is exactly what
	// the live logs show ("unit not found in bar", repeated top-up taps, heroes
	// never transitioning). For Windows, prefer the slots we just detected on
	// THIS live 860x732 battle frame and deploy them directly along a safe edge.
	if runtime.GOOS == "windows" {
		e.logger.Info().Int("slots", len(slotMgr.GetAllSlots())).Msg("using Windows live-slot deployment path")

		// Build the Windows deployment line from the LIVE red deployment
		// boundary, not from fixed percentages. Clash of Clans only accepts
		// troop drops OUTSIDE the red no-deploy polygon; the previous fixed
		// 28%-72% / 64%-height line frequently landed inside the village and
		// produced "You cannot deploy troops on the red area!".
		//
		// RedZone is represented as a bounding box, so choose a side that has
		// actual free screen space and place the line just OUTSIDE that box.
		// If the strategy's preferred side has no room, use the side with the
		// most room instead of falling back toward the middle of the base.
		var p1, p2 image.Point
		const edgeMargin = 24
		deploySide := strings.ToLower(targetEdge)
		if strings.Contains(deploySide, "top") {
			deploySide = "top"
		} else if strings.Contains(deploySide, "bottom") {
			deploySide = "bottom"
		} else if strings.Contains(deploySide, "left") {
			deploySide = "left"
		} else if strings.Contains(deploySide, "right") {
			deploySide = "right"
		}

		if redZone.Valid {
			const outsidePad = 34
			free := map[string]int{
				"left":   redZone.BBox.Min.X,
				"right":  w - redZone.BBox.Max.X,
				"top":    redZone.BBox.Min.Y,
				"bottom": uiCutoff - redZone.BBox.Max.Y,
			}

			// Reliability first on Windows: use the side with the most real
			// free space outside the detected red box, regardless of the
			// strategy's preferred corner. The old preference could pick a
			// very narrow strip and force taps back toward the village.
			// Never choose the bottom side on Windows. The lower battle HUD
			// contains Surrender/End Battle, Overall Damage and the troop bar.
			// A geometrically "free" strip there is not a safe tap region.
			deploySide = "left"
			best := free["left"]
			for _, side := range []string{"right", "top"} {
				if free[side] > best {
					deploySide = side
					best = free[side]
				}
			}

			switch deploySide {
			case "right":
				x := redZone.BBox.Max.X + outsidePad
				if x > w-edgeMargin { x = w-edgeMargin }
				hudSafeBottom := int(float64(h) * 0.70)
				y1 := clamp(redZone.BBox.Min.Y+35, edgeMargin, hudSafeBottom)
				y2 := clamp(redZone.BBox.Max.Y-35, edgeMargin, hudSafeBottom)
				if y2 < y1 { y1, y2 = y2, y1 }
				p1, p2 = image.Pt(x, y1), image.Pt(x, y2)
			case "top":
				y := redZone.BBox.Min.Y - outsidePad
				if y < edgeMargin { y = edgeMargin }
				x1 := clamp(redZone.BBox.Min.X+35, edgeMargin, w-edgeMargin)
				x2 := clamp(redZone.BBox.Max.X-35, edgeMargin, w-edgeMargin)
				p1, p2 = image.Pt(x1, y), image.Pt(x2, y)
			case "bottom":
				y := redZone.BBox.Max.Y + outsidePad
				if y > uiCutoff-edgeMargin { y = uiCutoff-edgeMargin }
				x1 := clamp(redZone.BBox.Min.X+35, edgeMargin, w-edgeMargin)
				x2 := clamp(redZone.BBox.Max.X-35, edgeMargin, w-edgeMargin)
				p1, p2 = image.Pt(x1, y), image.Pt(x2, y)
			default: // left
				x := redZone.BBox.Min.X - outsidePad
				if x < edgeMargin { x = edgeMargin }
				hudSafeBottom := int(float64(h) * 0.70)
				y1 := clamp(redZone.BBox.Min.Y+35, edgeMargin, hudSafeBottom)
				y2 := clamp(redZone.BBox.Max.Y-35, edgeMargin, hudSafeBottom)
				if y2 < y1 { y1, y2 = y2, y1 }
				p1, p2 = image.Pt(x, y1), image.Pt(x, y2)
			}

			e.logger.Info().
				Str("side", deploySide).
				Interface("red_bbox", redZone.BBox).
				Interface("p1", p1).
				Interface("p2", p2).
				Int("free_space", free[deploySide]).
				Msg("Windows deploy line locked to widest free side outside live red zone")
		} else if len(deployLine.Points) >= 2 {
			// Existing calculator already keeps these points near the outer
			// edge; use them if red-line detection itself was unavailable.
			p1 = deployLine.Points[0]
			p2 = deployLine.Points[len(deployLine.Points)-1]
			e.logger.Warn().
				Interface("p1", p1).
				Interface("p2", p2).
				Msg("Windows red zone unavailable; using calculated outer deploy line")
		} else {
			// Last-resort line hugs the LEFT border rather than the middle.
			p1 = image.Pt(edgeMargin, int(float64(h)*0.25))
			p2 = image.Pt(edgeMargin, int(float64(h)*0.68))
			e.logger.Warn().Msg("Windows deploy line fallback: hugging outer left border")
		}

		tapExec := NewTapExecutor(e.client, e.cal, e.logger)
		tapExec.StartDeployBudget()
		unverifiedSlots := 0

		// Candidate deploy lines MUST all stay on the SAME verified outside
		// side of the live red boundary. The previous implementation rotated
		// retries through hard-coded left/right/top/bottom lines; on irregular
		// bases those fallback lines could be INSIDE the red no-deploy polygon,
		// which is exactly why Clash displayed "You cannot deploy troops on the
		// red area!" even though the first line was correct.
		//
		// Build progressively-more-outward variants instead. If the first line
		// is rejected, every retry moves AWAY from the village / red boundary,
		// never across it.
		safeLines := make([][2]image.Point, 0, 4)
		baseLine := [2]image.Point{p1, p2}
		safeLines = append(safeLines, baseLine)

		nudgeOutside := func(line [2]image.Point, pixels int) [2]image.Point {
			out := line
			switch deploySide {
			case "right":
				out[0].X = clamp(out[0].X+pixels, edgeMargin, w-edgeMargin)
				out[1].X = clamp(out[1].X+pixels, edgeMargin, w-edgeMargin)
			case "top":
				out[0].Y = clamp(out[0].Y-pixels, edgeMargin, uiCutoff-edgeMargin)
				out[1].Y = clamp(out[1].Y-pixels, edgeMargin, uiCutoff-edgeMargin)
			case "bottom":
				out[0].Y = clamp(out[0].Y+pixels, edgeMargin, uiCutoff-edgeMargin)
				out[1].Y = clamp(out[1].Y+pixels, edgeMargin, uiCutoff-edgeMargin)
			default: // left
				out[0].X = clamp(out[0].X-pixels, edgeMargin, w-edgeMargin)
				out[1].X = clamp(out[1].X-pixels, edgeMargin, w-edgeMargin)
			}
			return out
		}

		// Keep three retry bands behind the same red line. This gives enough
		// clearance for the line thickness, tap jitter and contour error.
		for _, extra := range []int{16, 32, 48} {
			line := nudgeOutside(baseLine, extra)
			if line != safeLines[len(safeLines)-1] {
				safeLines = append(safeLines, line)
			}
		}
		e.logger.Info().
			Str("side", deploySide).
			Int("safe_lines", len(safeLines)).
			Interface("closest", safeLines[0]).
			Interface("furthest", safeLines[len(safeLines)-1]).
			Msg("Windows safe deploy corridor locked behind red boundary")

		// One helper for initial deploy + reconciliation. Every troop-like card
		// uses this verified outside corridor. Retries only move farther OUT.
		// Spells intentionally target inside.
		deploySlot := func(slot *TrackedSlot, n int) {
			if n <= 0 {
				n = 1
			}
			if n > 40 {
				n = 40
			}

			tapExec.TapSlot(slot, 3)
			tapExec.HumanSleep(110, 20)

			if slot.Category == "Spell" {
				spellPoint := image.Pt(w/2, int(float64(uiCutoff)*0.50))
				if redZone.Valid {
					spellPoint = image.Pt(
						(redZone.BBox.Min.X+redZone.BBox.Max.X)/2,
						(redZone.BBox.Min.Y+redZone.BBox.Max.Y)/2,
					)
				}
				tapExec.TapDeployPoint(spellPoint, n, 2)
			} else {
				// Keep the whole army on one coherent deployment line. The old
				// code advanced to another parallel band for every retry/card,
				// which made the attack look like scattered "balls" and could
				// waste taps near the red boundary. Use the furthest verified
				// safe line consistently for normal troops.
				line := safeLines[len(safeLines)-1]
				e.logger.Info().
					Str("unit", slot.UnitName).
					Str("category", slot.Category).
					Interface("p1", line[0]).
					Interface("p2", line[1]).
					Msg("Windows deploy: using outer-edge line")

				if slot.Category == "Hero" || slot.Category == "Siege" || slot.Category == "CC" {
					tapExec.TapDeployPoint(image.Pt((line[0].X+line[1].X)/2, (line[0].Y+line[1].Y)/2), 1, 2)
				} else {
					tapExec.TapDeployLine(line[0], line[1], n, 2)
				}
			}
		}

		windowsSlots := append([]*TrackedSlot(nil), slotMgr.GetAllSlots()...)
		categoryPriority := func(cat string) int {
			switch cat {
			case "Troop":
				return 0
			case "Hero":
				return 1
			case "Siege", "CC":
				return 2
			case "Spell":
				return 3
			default:
				return 0
			}
		}
		sort.SliceStable(windowsSlots, func(i, j int) bool {
			pi := categoryPriority(windowsSlots[i].Category)
			pj := categoryPriority(windowsSlots[j].Category)
			if pi != pj { return pi < pj }
			return windowsSlots[i].X < windowsSlots[j].X
		})

		for _, slot := range windowsSlots {
			if tapExec.DeployBudgetExhausted() {
				unverifiedSlots++
				continue
			}

			count := GetCountForSlot(troopCounts, slot.X)
			if count <= 0 {
				// OCR failure must NOT turn a troop card into a one-tap deploy.
				// That was the exact live bug: a card with several troops was
				// read as 0, we substituted 1, then immediately moved on.
				// Use a bounded burst for ordinary cards; one-shot categories
				// are handled separately below.
				switch slot.Category {
				case "Spell":
					count = 3
				default:
					count = 8
				}
				e.logger.Info().
					Str("unit", slot.UnitName).
					Str("category", slot.Category).
					Int("fallback_burst", count).
					Msg("troop count OCR unavailable; using bounded multi-unit burst instead of single tap")
			}
			if count > 40 {
				count = 40
			}

			beforeActivity := GetSlotActivityRatioStatic(deployScreen, slot.X, slot.Y, w)

			e.logger.Info().
				Str("unit", slot.UnitName).
				Str("category", slot.Category).
				Int("slot_x", slot.X).
				Int("slot_y", slot.Y).
				Int("count", count).
				Msg("Windows deploy: selecting live slot")

			// One-shot cards (heroes / siege / clan castle) must NEVER enter the
			// generic reconciliation loop. After a hero is deployed its card
			// remains visible as the hero ability button, so "still active"
			// does not mean "not deployed". Re-selecting it repeatedly wastes
			// time and can fire abilities instead of placing troops.
			if slot.Category == "Hero" || slot.Category == "Siege" || slot.Category == "CC" {
				// Use the furthest known-safe corridor point immediately for
				// one-shot units. This maximizes legal-placement margin and
				// avoids spending retries close to the red boundary.
				safeIdx := len(safeLines) - 1
				if safeIdx < 0 { safeIdx = 0 }
				line := safeLines[safeIdx]
				tapExec.TapSlot(slot, 1)
				tapExec.HumanSleep(120, 15)
				pt := image.Pt((line[0].X+line[1].X)/2, (line[0].Y+line[1].Y)/2)
				tapExec.TapDeployPoint(pt, 1, 1)
				tapExec.HumanSleep(180, 20)
				slotMgr.MarkSlotDeployed(slot)
				e.logger.Info().
					Str("unit", slot.UnitName).
					Str("category", slot.Category).
					Int("slot_x", slot.X).
					Interface("deploy_point", pt).
					Msg("Windows one-shot unit placed once on furthest safe edge; retries disabled")
				continue
			}

			deploySlot(slot, count)

			// FAST VERIFY: one fresh read, at most one top-up. Spending four
			// reconciliation rounds on every card was the main reason attacks
			// stalled for tens of seconds while troops were visibly left.
			tapExec.HumanSleep(120, 15)
			verified := false
			fresh, capErr := tapExec.CaptureFresh()
			if capErr == nil && !fresh.Empty() {
				liveCount := troopCounter.DetectCount(fresh, slot, mBarY)
				empty := isSlotEmptyStatic(fresh, slot.X, slot.Y, w, h)
				activity := GetSlotActivityRatioStatic(fresh, slot.X, slot.Y, w)
				fresh.Close()

				if empty || (liveCount == 0 && beforeActivity > 0 && activity < beforeActivity*0.60) {
					verified = true
					e.logger.Info().
						Str("unit", slot.UnitName).
						Int("live_count", liveCount).
						Msg("Windows fast deploy verified slot drained")
				} else if liveCount > 0 {
					if liveCount > 40 { liveCount = 40 }
					e.logger.Warn().
						Str("unit", slot.UnitName).
						Int("remaining", liveCount).
						Msg("Windows fast deploy: one immediate remainder top-up")
					deploySlot(slot, liveCount)
					verified = true // final sweep will catch any true remainder
				} else {
					// OCR unknown but the card still looks active. Fire one
					// small remainder burst now instead of abandoning the card
					// after a single unit. This keeps deployment fast while
					// making progress even when digit OCR misses the xN label.
					if !empty && activity >= 0.10 {
						const unknownRemainderBurst = 6
						e.logger.Warn().
							Str("unit", slot.UnitName).
							Float64("activity", activity).
							Int("burst", unknownRemainderBurst).
							Msg("Windows fast deploy: active card with unknown count; firing bounded remainder burst")
						deploySlot(slot, unknownRemainderBurst)
						verified = true // final live sweep still checks leftovers
					} else {
						e.logger.Warn().
							Str("unit", slot.UnitName).
							Msg("Windows fast deploy: verification inconclusive; moving on")
					}
				}
			} else if !fresh.Empty() {
				fresh.Close()
			}

			if verified {
				slotMgr.MarkSlotDeployed(slot)
			} else {
				slotMgr.MarkSlotFailed(slot)
				unverifiedSlots++
			}
		}

		// FINAL LIVE SWEEP
		// ----------------
		// Do not trust the initial slot states as the completion criterion.
		// Hero ability cards can stay visible after a successful deployment,
		// while ordinary troop cards can remain with a numeric xN count even
		// after earlier verification failed. Rebuild the slot map from fresh
		// battle frames and keep draining every card with an actual positive
		// live count. This is the authoritative "are troops still left?" pass.
		//
		// One-shot cards (Hero/Siege/CC) get one rescue attempt per X position
		// if they were never visibly transitioned; they are never spammed,
		// because a deployed hero card becomes its ability button.
		oneShotRescue := make(map[int]bool)
		for _, slot := range slotMgr.GetAllSlots() {
			if slot.Category == "Hero" || slot.Category == "Siege" || slot.Category == "CC" {
				if slot.State == SlotDeployed {
					oneShotRescue[slot.X] = true
				}
			}
		}

		liveRemaining := 0
		for sweepRound := 1; sweepRound <= 2 && !tapExec.DeployBudgetExhausted(); sweepRound++ {
			fresh, capErr := tapExec.CaptureFresh()
			if capErr != nil || fresh.Empty() {
				if !fresh.Empty() { fresh.Close() }
				e.logger.Warn().Int("round", sweepRound).Msg("final live sweep capture failed")
				tapExec.HumanSleep(180, 25)
				continue
			}

			liveMgr := NewSlotManager(fresh, pCfg, w, h, mBarY, e.templates, e.classify, e.logger)
			liveSlots := liveMgr.GetAllSlots()
			if len(liveSlots) == 0 {
				fresh.Close()
				e.logger.Info().Int("round", sweepRound).Msg("final live sweep: no active troop-bar cards detected")
				liveRemaining = 0
				break
			}

			liveCounts := troopCounter.DetectCounts(fresh, liveSlots, mBarY)
			acted := 0
			liveRemaining = 0

			for _, liveSlot := range liveSlots {
				count := GetCountForSlot(liveCounts, liveSlot.X)

				// Positive OCR count is the strongest possible evidence that
				// deployable troops/spells are still sitting in the bar.
				if count > 0 {
					// Never treat a hero/siege/CC card with a positive OCR
					// read as an undeployed multi-count card. After placement,
					// hero ability art/labels can look like a numeric count.
					if liveSlot.Category == "Hero" || liveSlot.Category == "Siege" || liveSlot.Category == "CC" {
						e.logger.Debug().
							Int("round", sweepRound).
							Int("slot_x", liveSlot.X).
							Str("unit", liveSlot.UnitName).
							Str("category", liveSlot.Category).
							Int("ocr_count", count).
							Msg("final sweep ignoring one-shot card after initial placement")
						continue
					}
					if count > 40 { count = 40 }
					liveRemaining++
					e.logger.Warn().
						Int("round", sweepRound).
						Int("slot_x", liveSlot.X).
						Str("unit", liveSlot.UnitName).
						Str("category", liveSlot.Category).
						Int("count", count).
						Msg("final live sweep found remaining deployable units")

					deploySlot(liveSlot, count)
					acted++
					continue
				}

				// One-shot cards are intentionally never rescued here. They were
				// already given exactly one placement attempt on the furthest
				// safe edge during the initial pass. A visible hero card after
				// that is normally its ability button, not an undeployed hero.
				if liveSlot.Category == "Hero" || liveSlot.Category == "Siege" || liveSlot.Category == "CC" {
					oneShotRescue[liveSlot.X] = true
					continue
				}

				// OCR can return 0 for a perfectly live troop card. Use the
				// card's visual activity as a second signal and give it one
				// bounded burst per sweep. This is what prevents visible x5/x10
				// cards from being silently left behind.
				activity := GetSlotActivityRatioStatic(fresh, liveSlot.X, liveSlot.Y, w)
				empty := isSlotEmptyStatic(fresh, liveSlot.X, liveSlot.Y, w, h)
				if !empty && activity >= 0.12 {
					burst := 6
					if liveSlot.Category == "Spell" {
						burst = 2
					}
					liveRemaining++
					e.logger.Warn().
						Int("round", sweepRound).
						Int("slot_x", liveSlot.X).
						Str("unit", liveSlot.UnitName).
						Str("category", liveSlot.Category).
						Float64("activity", activity).
						Int("burst", burst).
						Msg("final live sweep found visually active card with unknown count")
					deploySlot(liveSlot, burst)
					acted++
				}
			}
			fresh.Close()

			if acted == 0 {
				// There may still be hero ability cards visible, but no
				// positively-counted deployable troops remain and no untried
				// one-shot card was found.
				liveRemaining = 0
				e.logger.Info().Int("round", sweepRound).Msg("final live sweep confirms no deployable units remain")
				break
			}

			e.logger.Info().
				Int("round", sweepRound).
				Int("actions", acted).
				Msg("final live sweep fired remaining units; rechecking bar")
			tapExec.HumanSleep(140, 20)
		}

		remaining := liveRemaining
		e.logger.Info().
			Int("remaining", remaining).
			Int("initial_unverified", unverifiedSlots).
			Int("slots", len(slotMgr.GetAllSlots())).
			Msg("Windows deployment final live verification complete")
		if remaining > 0 {
			return remaining, fmt.Errorf("%d live deployable slot(s) still remain after final sweep", remaining)
		}
		return 0, nil
	}
	// troopCounter is threaded below to NewHeroManager / NewSweeper /
	// NewVerifier so they can live-OCR per-slot counts at deploy time
	// and reconcile until the slot is truly empty (fixes the
	// "balloons/EDs sometimes don't all get placed" bug).

	// 6. Initialize tap executor
	tapExec := NewTapExecutor(e.client, e.cal, e.logger)
	// Give deployers a unit-name -> slot lookup (used to pair Amount:"All"
	// spells with their live OCR counts).
	tapExec.SetSlotResolver(slotMgr.GetSlot)

	// Arm the deploy budget. Every phase/reconcile/sweep loop below
	// consults tapExec.DeployBudgetExhausted() between rounds so a stuck
	// slot (spent spell card still reading visually non-empty, broken OCR,
	// etc.) can never fire taps past the 3-minute battle timer. Observed
	// live: the EQ-spell sweep kept re-firing for ~2 minutes after the
	// battle had already ended because no wall-clock bound existed.
	tapExec.StartDeployBudget()

	// 7. Plan phases
	planner := NewDeployPlanner(slotMgr, pCfg, targetEdge, w, h, e.logger)
	plans := planner.PlanDeployment(s)

	// 8. Collect strategy unit names
	strategyNames := GetStrategyUnitNames(s)

	// 9. Execute each phase
	for _, plan := range plans {
		// Hard deploy-time stop: if the budget ran out mid-plan, abandon
		// the remaining phases instead of tapping into the battle timer.
		// The leftover slots are reported as undeployed so the attack
		// report shows the partial failure instead of a phantom success.
		if tapExec.DeployBudgetExhausted() {
			remaining := len(slotMgr.GetUndeployedSlots())
			e.logger.Warn().
				Str("phase", plan.Phase.Name).
				Dur("budget", DeployBudget).
				Int("undeployed", remaining).
				Msg("deploy budget exhausted before phases completed; stopping deploy (battle-timer guard)")
			return remaining, fmt.Errorf("deploy budget exhausted (%s); %d slots undeployed", DeployBudget, remaining)
		}

		e.logger.Info().Str("phase", plan.Phase.Name).Msg("attack phase")
		if e.OnPhaseStart != nil {
			e.OnPhaseStart(plan.Phase.Name, targetEdge)
		}

		// Deploy spells. Thread the live OCR counts + slot resolver so
		// Amount:"All" spells tap exactly the count the army carries
		// instead of a hardcoded 5 (the valk EQ army carries e.g. 4 EQs
		// on one card; the edrag rush carries 11 rage on another).
		spellDeployer := NewSpellDeployerWithCounts(tapExec, pCfg, formulaPtr, w, h, countMap, e.logger)
		spellDeployer.SetCounter(troopCounter, slotMgr.GetBarY())
		for _, up := range ResolveSpellTargets(plan) {
			if up.Slot == nil {
				continue
			}
			e.logger.Info().
				Str("unit", up.Unit.Name).
				Int("x", up.Slot.X).
				Msg("deploying spell")

			if e.OnUnitDeploy != nil {
				e.OnUnitDeploy(up.Unit.Name, up.Slot.X, slotMgr.GetSlotY())
			}

			tapExec.TapSlot(up.Slot, 8)
			// 150ms is the empirically-required CoC slot-selection
			// animation floor (matches deploySingleHero's settle on
			// heroes). The previous 35ms was too fast: the bot
			// frequently "selected" the ED slot but CoC was still
			// mid-animation from the prior phase, so taps fired on
			// the OLD unit type instead of ED. Live data:
			// auto_edrag_rush showed zero EDs on the field after
			// the deploy despite the bot reporting "all units
			// successfully deployed". Same fix applies to Spells +
			// Siege (they share the 35ms gap and the same bug).
			tapExec.HumanSleep(150, 30)

			success := spellDeployer.DeploySpell(up.Unit, up.Slot, targetEdge, plan.Phase.Pattern)
			if success {
				slotMgr.MarkDeployed(strings.ToLower(up.Unit.Name))
				// Post-deploy verify: live-OCR the card count and re-fire
				// any spells that didn't drop (same reconcile philosophy as
				// the troop path). Best-effort — a failed OCR just logs.
				// 4 rounds: each round fires only the unconfirmed remainder,
				// and the internal no-progress stop aborts early when OCR
				// reads the same count twice — so more headroom only helps
				// genuinely draining multi-charge cards, never the spent-
				// card loop (live: 1-charge rage re-fired for the old 2-
				// round budget while OCR read "1" every time).
				if extra, confirmed := spellDeployer.VerifyAndReconcile(up.Unit, up.Slot, targetEdge, plan.Phase.Pattern, 4); extra > 0 {
					e.logger.Info().
						Str("unit", up.Unit.Name).
						Int("extra_fired", extra).
						Bool("confirmed_empty", confirmed).
						Msg("spell reconcile fired extra spells")
				}
			}
		}

		// Deploy troops
		heroMgr := NewHeroManager(tapExec, slotMgr, pCfg, targetEdge, w, h, formulaPtr, troopCounter, e.logger)
		// Bridge: when HeroManager's resolveHeroTarget fires for the
		// Dragon Duke, route the event through Executor.OnDukePick so a
		// single observer (live bot's NDJSON writer, debug_test's
		// recorder) sees BOTH the legacy adjacent-corner random pick and
		// the new "follow the chosen edge" behavior. chosen == target in
		// the new path — Duke falls through to the chosen edge with a
		// random point along it.
		heroMgr.OnDukeDeployed = func(target string) {
			if e.OnDukePick != nil {
				e.OnDukePick(target, target)
			}
		}
		for _, up := range ResolveTroopTargets(plan) {
			if up.Slot == nil {
				continue
			}
			e.logger.Info().
				Str("unit", up.Unit.Name).
				Int("x", up.Slot.X).
				Msg("deploying troop")

			if e.OnUnitDeploy != nil {
				e.OnUnitDeploy(up.Unit.Name, up.Slot.X, slotMgr.GetSlotY())
			}

			tapExec.TapSlot(up.Slot, 8)
			// 150ms is the empirically-required CoC slot-selection
			// animation floor (matches deploySingleHero's settle on
			// heroes). The previous 35ms was too fast: the bot
			// frequently "selected" the ED slot but CoC was still
			// mid-animation from the prior phase, so taps fired on
			// the OLD unit type instead of ED. Live data:
			// auto_edrag_rush showed zero EDs on the field after
			// the deploy despite the bot reporting "all units
			// successfully deployed". Same fix applies to Spells +
			// Siege (they share the 35ms gap and the same bug).
			tapExec.HumanSleep(150, 30)

			detectedCount := GetCountForSlot(troopCounts, up.Slot.X)
			heroMgr.DeployTroops(up.Unit, up.Slot, plan.Phase.Pattern, plan.Phase.Offset, plan.Phase.Pattern, screen, detectedCount)
		}

		// Deploy siege
		for _, up := range ResolveSiegeTargets(plan) {
			if up.Slot == nil {
				continue
			}
			e.logger.Info().
				Str("unit", up.Unit.Name).
				Int("x", up.Slot.X).
				Msg("deploying siege")

			if e.OnUnitDeploy != nil {
				e.OnUnitDeploy(up.Unit.Name, up.Slot.X, slotMgr.GetSlotY())
			}

			tapExec.TapSlot(up.Slot, 8)
			// 150ms is the empirically-required CoC slot-selection
			// animation floor (matches deploySingleHero's settle on
			// heroes). The previous 35ms was too fast: the bot
			// frequently "selected" the ED slot but CoC was still
			// mid-animation from the prior phase, so taps fired on
			// the OLD unit type instead of ED. Live data:
			// auto_edrag_rush showed zero EDs on the field after
			// the deploy despite the bot reporting "all units
			// successfully deployed". Same fix applies to Spells +
			// Siege (they share the 35ms gap and the same bug).
			tapExec.HumanSleep(150, 30)

			heroMgr.DeploySiege(up.Unit, up.Slot)
		}

		// Deploy heroes
		if strings.Contains(plan.Phase.Name, "Heroes") {
			heroUnits := make([]strategy.Unit, 0)
			for _, up := range ResolveHeroTargets(plan) {
				heroUnits = append(heroUnits, up.Unit)
			}
			if len(heroUnits) > 0 {
				heroMgr.DeployHeroes(heroUnits, screen)
			}
		}

		// Phase delay defaults tightened: Heroes/Siege used to sit at
		// 500ms post-phase, which compounded with each hero's 800ms settle
		// inside hero_manager.go to produce a >1.5s wall-clock gap between
		// heroes (visibly bot-paced).
		//
		// USER YAML WINS. The previous override unconditionally clamped
		// to 100ms regardless of the strategy's `delay_after_ms`, which
		// made the field useless for Heroes/Siege. New rule: only fall
		// back when the YAML value is unset (0); any explicit YAML value
		// (including small ones like 50ms) is preserved.
		//
		// Overall cap is maxPhaseDelay. Anything above gets WARN-logged
		// so authors can spot unintentional bloat. The cap is high
		// enough for spells (which legitimately need a long settle for
		// the multi-tap flow to register).
		const heroSiegeDefault = 50 * time.Millisecond
		const interPhaseMin = 50 * time.Millisecond // Safety floor so YAML values like delay_after_ms: 10 do not bypass the inter-phase settle window — CoC needs ~50ms minimum between phases to register the new troop bar state.
		const maxPhaseDelay = 200 * time.Millisecond
		pDelay := time.Duration(plan.Phase.DelayAfterMS) * time.Millisecond
		isHeroOrSiege := strings.Contains(plan.Phase.Name, "Heroes") || strings.Contains(plan.Phase.Name, "Siege")
		if isHeroOrSiege && pDelay <= 0 {
			pDelay = heroSiegeDefault
		}
		if pDelay < interPhaseMin {
			pDelay = interPhaseMin
		}
		if pDelay > maxPhaseDelay {
			e.logger.Warn().
				Str("phase", plan.Phase.Name).
				Dur("requested", pDelay).
				Dur("clamped_to", maxPhaseDelay).
				Msg("phase delay_after_ms exceeds cap; clamping to keep attack human-paced")
			pDelay = maxPhaseDelay
		}
		if pDelay > 0 {
			time.Sleep(pDelay)
		}
	}

	// 10. Sweep remaining. Pass formulaPtr so the sweep path honors
	// user-pinned _event_troop / _event_spell coords the same way
	// DeployHeroes / DeployTroops already do. Without this the bot
	// silently dropped event troops on the dynamically-detected
	// red-zone line, ignoring the user's pin entirely.
	sweeper := NewSweeper(tapExec, slotMgr, pCfg, deployLine, w, h, formulaPtr, troopCounter, s.EventTroopsAutoDeployEnabled(), e.logger)
	sweeper.Sweep(strategyNames, countMap)

	// 11. Verify
	verifier := NewVerifier(tapExec, slotMgr, pCfg, targetEdge, w, h, DefaultVerifyConfig(), troopCounter, e.logger)
	remainingCount := verifier.VerifyAll()

	return remainingCount, nil
}

// ---- pinned-line helpers --------------------------------------------------
//
// These helpers were added together with the Path A orchestrator fix that
// respects user-pinned coords in precision_config.json. The bug being
// addressed: the orchestrator's "override all 4 corners" block in
// DeployDynamicV2 was clobbering every corner with the dynamic red-zone
// line, so users who pinned a corner in cmd/pick_coords saw all their
// lines vanish at runtime. These helpers detect a "real" pin and route a
// linspace DeployLine through it.

// hasPinnedForTarget returns true when the user authored a non-zero edge
// for `targetEdge`, OR a matching side has non-zero coords in Sides.
// We deliberately exclude Go's (0,0)→(0,0) zero-default (the result of an
// unmarshaled missing JSON key) so an empty config doesn't look "pinned".
func hasPinnedForTarget(pCfg PrecisionConfig, targetEdge string) bool {
	if pCfg.Edges != nil {
		if e, ok := pCfg.Edges[targetEdge]; ok && !isZeroManualEdge(e) {
			return true
		}
	}
	if side := cornerToSide(targetEdge); side != "" && pCfg.Sides != nil {
		if s, ok := pCfg.Sides[side]; ok && !isZeroManualEdge(s) {
			return true
		}
	}
	return false
}

// isZeroManualEdge is true for the (0,0)→(0,0) zero-default Go produces
// when a JSON key is absent. Such entries are NOT considered user-pinned.
func isZeroManualEdge(e ManualEdge) bool {
	return e.P1.X == 0 && e.P1.Y == 0 && e.P2.X == 0 && e.P2.Y == 0
}

// cornerToSide maps the four legacy corner keys to a physical side name.
// Returns "" for inputs the orchestrator doesn't know how to classify.
// This is used both for matching pinned Sides entries and for selecting
// which physical side the DeployLineCalculator should prefer.
func cornerToSide(targetEdge string) string {
	switch strings.ToLower(targetEdge) {
	case "topleft", "topright":
		return "top"
	case "bottomleft", "bottomright":
		return "bottom"
	case "left":
		return "left"
	case "right":
		return "right"
	default:
		return ""
	}
}

// manualEdgeToDeployLine produces a DeployLine by direct linspace between
// P1 and P2 of the given manual edge. We bypass textual side mapping so
// a diagonal pinned line is preserved verbatim — that's the contract
// the new orchestrator override-skip path relies on. Side is set to
// `targetEdge` so downstream consumers (Sweep, SpellLine) see a stable
// identifier instead of an inferred compass direction.
func manualEdgeToDeployLine(edge ManualEdge, targetEdge string, n int) DeployLine {
	if n < 2 {
		n = 15
	}
	pts := make([]image.Point, n)
	for i := 0; i < n; i++ {
		t := float64(i) / float64(n-1)
		pts[i] = image.Pt(
			edge.P1.X+int(t*float64(edge.P2.X-edge.P1.X)),
			edge.P1.Y+int(t*float64(edge.P2.Y-edge.P1.Y)),
		)
	}
	return DeployLine{
		Points:  pts,
		Side:    targetEdge,
		Anchor:  pts[len(pts)/2],
		Outside: true,
	}
}

// applyCornerOverride is the orchestrator corner-mirror step. It exists
// for two reasons:
//
//  1. Duke-coherence: HeroManager.resolveHeroTarget picks the Dragon
//     Duke's deploy line from ONE OF THE LEGACY CORNER KEYS
//     (`pCfg.Edges[adjacentCorner]`). Without mirroring, an attacker
//     that pinned BottomLeft but not TopLeft/BottomRight would let Duke
//     scatter to whatever default was at those unpinned corners —
//     chasing the formula drift symptom this whole change set is
//     fighting.
//
//  2. Sides feed SpotsForSide: subsequent strict-side consumers read
//     from pCfg.Sides, so we keep that map populated.
//
// When the user explicitly pinned the chosen target, the override ONLY
// touches UNPINNED corners and writes the SAME PINNED LINE into them
// — Duke's adjacent-corner random pick then drops on the user's
// pin, eliminating the cross-side scatter. The pinned target itself
// is left untouched.
//
// When the user did NOT pin the target, the legacy "clobber all 4
// corners with the dynamic red-zone line" path is preserved so an
// unpinned attack still has a sensible fallback.
func applyCornerOverride(pCfg *PrecisionConfig, deployLine DeployLine, redZoneValid bool, targetEdge string) {
	if !redZoneValid || len(deployLine.Points) < 2 {
		return
	}
	if pCfg.Edges == nil {
		pCfg.Edges = make(map[string]ManualEdge)
	}
	if pCfg.Sides == nil {
		pCfg.Sides = make(map[string]ManualEdge)
	}

	// Resolve the user's pin (if any) for the chosen target. Used as
	// the override source so Duke's adjacent-corner pick lands on the
	// user's pinned line, not the dynamic red-zone default.
	var pinnedOverride ManualEdge
	hasPinned := false
	if e, ok := pCfg.Edges[targetEdge]; ok && !isZeroManualEdge(e) {
		pinnedOverride = e
		hasPinned = true
	} else if side := cornerToSide(targetEdge); side != "" {
		if s, ok := pCfg.Sides[side]; ok && !isZeroManualEdge(s) {
			pinnedOverride = s
			hasPinned = true
		}
	}

	targets := []string{"TopLeft", "TopRight", "BottomLeft", "BottomRight"}
	sideNames := []string{"top", "right", "bottom", "left"}

	if hasPinned {
		// Pinned target path: leave target alone, mirror the user's pin
		// into the 3 unpinned corners + all 4 side names. This is the
		// fix for "attacks in corner on 2 sides" — Duke's random pick
		// can no longer scatter to garbage default coords.
		for _, c := range targets {
			if c == targetEdge {
				continue
			}
			pCfg.Edges[c] = pinnedOverride
		}
		for _, s := range sideNames {
			pCfg.Sides[s] = pinnedOverride
		}
		return
	}

	// Unpinned path: clobber all 4 corners + sides with the dynamic
	// red-zone deploy line. Preserves the original behavior so a
	// fresh-config run still finds SOMETHING to attack on.
	lineStart := deployLine.Points[0]
	lineEnd := deployLine.Points[len(deployLine.Points)-1]
	dyn := ManualEdge{P1: lineStart, P2: lineEnd}
	for _, c := range targets {
		pCfg.Edges[c] = dyn
	}
	for _, s := range sideNames {
		pCfg.Sides[s] = dyn
	}
}
