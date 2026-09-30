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

// xingchenCompatibleAttackFlow keeps Windows on the same generic deployment
// planner as the proven Xingchen-style path. The experimental Windows-only
// adaptive-camera/live-slot engine remains in source for diagnostics, but is
// deliberately bypassed until it can survive repeated soak tests.
const xingchenCompatibleAttackFlow = true

// DeployDynamicV2 deploys troops using dynamic red line detection.
// No hardcoded precision_config.json needed - detects deployment boundary live.
//
// strategyPath is the on-disk YAML path. The orchestrator uses it to find
// the matching formula.json (loaded as <stem>_formula.json next to the
// YAML). Pass "" to skip formula lookup entirely.
func (e *Executor) DeployDynamicV2(s *strategy.DynamicStrategy, screen gocv.Mat, strategyPath string) (int, error) {
	analysisStarted := time.Now()
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
		e.logger.Debug().Str("edge", targetEdge).Msg("rotated to next edge")
	case strings.EqualFold(targetEdge, "Random"):
		edges := []string{"TopLeft", "TopRight", "BottomLeft", "BottomRight"}
		targetEdge = edges[rand.Intn(len(edges))]
		e.logger.Debug().Str("edge", targetEdge).Msg("random edge selected")
	}

	e.lastResolvedEdge = targetEdge
	e.lastDeploySide = cornerToSide(targetEdge)
	e.lastSafetyMode = "not_evaluated"
	e.lastRedZoneValid = false
	e.lastCorridorVerified = false
	e.lastHUDSafe = false
	e.lastRedZoneBBox = image.Rectangle{}
	e.lastDeployP1 = image.Point{}
	e.lastDeployP2 = image.Point{}
	e.lastDeployFreeSpace = 0
	e.lastLiveBarRescans = 0
	e.lastLiveBarRescanMicros = 0
	e.lastSlotDetectMicros = 0
	e.lastSlotClassifyMicros = 0
	e.lastTemplatesTried = 0
	e.lastTemplatesMatched = 0
	e.lastSelectedCardOCRCount = 0
	e.lastSelectedCardOCRMicros = 0

	// 1. Normalize the battlefield camera BEFORE red-zone geometry, slot
	// detection or troop planning. This is a bounded Xingchen-style stage:
	// establish scale, re-capture, re-detect the live red boundary, expose the
	// intended attack side if needed, then freeze that fresh frame for the
	// generic deployment planner.
	uiCutoff := int(float64(h) * 0.85) // above troop bar
	cameraStarted := time.Now()
	deployScreen, cameraFrameOwned, redZone := e.normalizeBattlefieldCamera(screen, targetEdge, uiCutoff)
	cameraMS := time.Since(cameraStarted).Milliseconds()
	defer func() {
		if cameraFrameOwned && !deployScreen.Empty() {
			deployScreen.Close()
		}
	}()

	e.lastRedZoneValid = redZone.Valid
	if redZone.Valid {
		e.lastRedZoneBBox = redZone.BBox
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
		e.logger.Debug().
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

	// 4. Initialize SlotManager — exactly once on the normalized frame.
	slotStarted := time.Now()
	slotMgr := NewSlotManager(deployScreen, pCfg, w, h, mBarY, e.templates, e.classify, e.logger)
	slotMS := time.Since(slotStarted).Milliseconds()
	if len(slotMgr.GetAllSlots()) == 0 {
		return 0, fmt.Errorf("no active slots detected")
	}

	// 5. Detect troop counts once. No pre-deploy rescan loop.
	countStarted := time.Now()
	troopCounter := NewTroopCounter(pCfg.Width, pCfg.Height, e.logger)
	defer troopCounter.Close()
	troopCounts := troopCounter.DetectCounts(deployScreen, slotMgr.GetAllSlots(), mBarY)
	countMS := time.Since(countStarted).Milliseconds()
	countMap := GetAllCounts(troopCounts)
	farmProfile, farmControlled := e.cfg.Farm.ActiveProfile()
	if farmControlled {
		writeArmyInspection(slotMgr.GetAllSlots(), troopCounts, &farmProfile)
	} else {
		writeArmyInspection(slotMgr.GetAllSlots(), troopCounts, nil)
	}
	e.logger.Debug().Interface("counts", countMap).Msg("detected troop counts")

	// Windows-safe deployment path.
	//
	// The legacy dynamic planner depends on historical manual_slots /
	// manual_labels / formula mappings that were calibrated on another
	// layout. On BlueStacks Windows the bot can reach Battle correctly but
	// then repeatedly select/reconcile the wrong cards, which is exactly what
	// the live logs show ("unit not found in bar", repeated top-up taps, heroes
	// never transitioning). For Windows, prefer the slots we just detected on
	// THIS live 860x732 battle frame and deploy them directly along a safe edge.
	if runtime.GOOS == "windows" && !xingchenCompatibleAttackFlow {
		e.logger.Debug().Int("slots", len(slotMgr.GetAllSlots())).Msg("using Windows live-slot deployment path")

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

		// A real user pin is an explicit calibration made against this player's
		// current BlueStacks layout. Honor it before the coarse red-zone BBox.
		// The BBox can legitimately span almost the whole frame when disconnected
		// red/orange UI contours are merged, which previously discarded a valid
		// user line and aborted the attack with "no safe Windows deploy corridor".
		if userPinnedForTarget && len(deployLine.Points) >= 2 {
			p1 = sanitizeWindowsDeployPoint(deployLine.Points[0], w, h)
			p2 = sanitizeWindowsDeployPoint(deployLine.Points[len(deployLine.Points)-1], w, h)
			if p1.Y >= uiCutoff || p2.Y >= uiCutoff {
				return len(slotMgr.GetAllSlots()), fmt.Errorf("user-pinned Windows deploy line intersects lower battle HUD")
			}
			deploySide = targetEdge
			e.lastDeploySide = deploySide
			e.lastRedZoneBBox = redZone.BBox
			e.lastDeployP1 = p1
			e.lastDeployP2 = p2
			e.lastDeployFreeSpace = 0
			e.lastSafetyMode = "user_pinned"
			e.lastRedZoneValid = redZone.Valid
			e.lastCorridorVerified = true
			e.lastHUDSafe = true
			e.logger.Info().
				Str("target", targetEdge).
				Interface("p1", p1).
				Interface("p2", p2).
				Msg("Windows user-pinned deploy line locked")
		} else if side, rp1, rp2, freeSpace, ok := windowsDeployCorridor(redZone, w, h, uiCutoff); ok {
			deploySide, p1, p2 = side, rp1, rp2
			e.lastDeploySide = deploySide
			e.lastRedZoneBBox = redZone.BBox
			e.lastDeployP1 = p1
			e.lastDeployP2 = p2
			e.lastDeployFreeSpace = freeSpace
			e.lastSafetyMode = "live_red_zone"
			e.lastRedZoneValid = true
			e.lastCorridorVerified = true
			e.lastHUDSafe = sanitizeWindowsDeployPoint(p1, w, h) == p1 && sanitizeWindowsDeployPoint(p2, w, h) == p2
			e.logger.Info().
				Str("side", deploySide).
				Interface("red_bbox", redZone.BBox).
				Interface("p1", p1).
				Interface("p2", p2).
				Int("free_space", freeSpace).
				Msg("Windows deploy corridor locked outside live red zone")
		} else if redZone.Valid {
			// A live red zone was detected but no mathematically safe line could
			// be constructed. Do not silently fall back to historical/manual
			// coordinates: refusing to tap is safer than deploying into the red
			// polygon or lower HUD.
			return len(slotMgr.GetAllSlots()), fmt.Errorf("live red zone detected but no safe Windows deploy corridor exists")
		} else if len(deployLine.Points) >= 2 {
			// Existing calculator already keeps these points near the outer
			// edge; use them if red-line detection itself was unavailable.
			p1 = deployLine.Points[0]
			p2 = deployLine.Points[len(deployLine.Points)-1]
			e.lastDeployP1 = p1
			e.lastDeployP2 = p2
			e.lastSafetyMode = "pinned_or_calculated"
			e.lastRedZoneValid = false
			e.lastCorridorVerified = false
			e.lastHUDSafe = sanitizeWindowsDeployPoint(p1, w, h) == p1 && sanitizeWindowsDeployPoint(p2, w, h) == p2
			e.logger.Warn().
				Interface("p1", p1).
				Interface("p2", p2).
				Msg("Windows red zone unavailable; using calculated outer deploy line")
		} else {
			// Last-resort line hugs the LEFT border rather than the middle.
			p1 = image.Pt(edgeMargin, int(float64(h)*0.25))
			p2 = image.Pt(edgeMargin, int(float64(h)*0.68))
			e.lastDeployP1 = p1
			e.lastDeployP2 = p2
			e.lastSafetyMode = "fallback_outer_edge"
			e.lastRedZoneValid = false
			e.lastCorridorVerified = false
			e.lastHUDSafe = sanitizeWindowsDeployPoint(p1, w, h) == p1 && sanitizeWindowsDeployPoint(p2, w, h) == p2
			e.logger.Warn().Msg("Windows deploy line fallback: hugging outer left border")
		}

		tapExec := NewTapExecutor(e.client, e.cal, e.logger)
	tapExec.SetFrameProvider(e.frameProvider)
		tapExec.StartDeployBudget()
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

		// One stable line only on Windows. Repeatedly nudging farther outward
		// eventually pushed taps into screen chrome / HUD. The base line is
		// already outside the detected red boundary by outsidePad.
		e.logger.Debug().
			Str("side", deploySide).
			Int("safe_lines", len(safeLines)).
			Interface("closest", safeLines[0]).
			Interface("furthest", safeLines[len(safeLines)-1]).
			Msg("Windows safe deploy corridor locked behind red boundary")

		var armyState *ArmyStateManager

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
				if armyState != nil {
					armyState.RecordDeploy(slot.UnitName, slot.Category, n, slot.X, slot.Y, deploySide, spellPoint, spellPoint)
				}
				tapExec.TapDeployPoint(spellPoint, n, 2)
			} else {
				// Keep the whole army on one coherent deployment line. The old
				// code advanced to another parallel band for every retry/card,
				// which made the attack look like scattered "balls" and could
				// waste taps near the red boundary. Use the furthest verified
				// safe line consistently for normal troops.
				line := safeLines[len(safeLines)-1]
				e.logger.Debug().
					Str("unit", slot.UnitName).
					Str("category", slot.Category).
					Interface("p1", line[0]).
					Interface("p2", line[1]).
					Msg("Windows deploy: using outer-edge line")

				if slot.Category == "Hero" || slot.Category == "Siege" || slot.Category == "CC" {
					pt := image.Pt((line[0].X+line[1].X)/2, (line[0].Y+line[1].Y)/2)
					if armyState != nil {
						armyState.RecordDeploy(slot.UnitName, slot.Category, 1, slot.X, slot.Y, deploySide, pt, pt)
					}
					tapExec.TapDeployPoint(pt, 1, 2)
				} else {
					if armyState != nil {
						armyState.RecordDeploy(slot.UnitName, slot.Category, n, slot.X, slot.Y, deploySide, line[0], line[1])
					}
					// Windows/BlueStacks can drop rapid tap triples under load.
					// Use paced one-by-one line deployment so a 9-count EDrag
					// card does not end with 1-2 troops still sitting in the bar.
					tapExec.TapDeployLineReliable(line[0], line[1], n, 2)
				}
			}
		}

		// Windows live deployment MUST re-read the troop bar after every card.
		// CoC compacts the bar when a troop/siege/spell card is emptied. Keeping
		// the initial X positions therefore makes every later tap drift onto the
		// next card (and eventually onto hero ability buttons). This is exactly
		// the observed "select ED -> jump to siege -> hammer last hero" failure.
		// Always create a replay recorder. When no farm profile is active the
		// manager simply has no expected-unit inventory, but RecordDeploy still
		// captures the real live-bar actions and geometry for Attack Replay.
		armyState = NewArmyStateManager(farmProfile)
		defer writeAttackTrace(s.Name, armyState)
		if farmControlled {
			e.logger.Debug().
				Int("town_hall", farmProfile.TownHall).
				Str("profile", farmProfile.Label).
				Int("troop_capacity", farmProfile.TroopCapacity).
				Int("spell_capacity", farmProfile.SpellCapacity).
				Msg("Windows deployment controlled by farm composition profile")
		}
		oneShotDone := make(map[string]bool)
		// Anonymous Siege/CC cards must be blacklisted by CATEGORY after their
		// first deployment, not by X. The live bar compacts after cards empty;
		// an unnamed siege can therefore move to a new X and otherwise look like
		// a fresh one-shot card on the next scan.
		anonymousOneShotDone := make(map[string]bool)
		// Structurally detected heroes may not have a portrait-template name.
		// Their relative left-to-right order remains stable even as troop cards
		// disappear, so remember how many anonymous hero cards were already
		// deployed and skip exactly that many on subsequent rescans.
		unknownHeroesDeployed := 0
		cardAttempts := make(map[string]int)
		profileFirstDeploy := make(map[string]bool)
		liveRemaining := 0

		oneShotKey := func(slot *TrackedSlot) string {
			name := strings.ToLower(strings.TrimSpace(slot.UnitName))
			if name == "" {
				name = fmt.Sprintf("x%d", slot.X)
			}
			return slot.Category + ":" + name
		}

		for liveRound := 1; liveRound <= 36 && !tapExec.DeployBudgetExhausted(); liveRound++ {
			fresh, capErr := tapExec.CaptureFresh()
			if capErr != nil || fresh.Empty() {
				if !fresh.Empty() { fresh.Close() }
				e.logger.Warn().Int("round", liveRound).Msg("Windows live deployment capture failed")
				tapExec.HumanSleep(140, 20)
				continue
			}

			rescanStarted := time.Now()
			liveMgr := NewSlotManagerLiveRescan(fresh, pCfg, w, h, mBarY, e.templates, e.classify, e.logger)
			detectMS, classifyMS := liveMgr.Timing()
			tried, matched := liveMgr.TemplateWork()
			e.lastSlotDetectMicros += int64(detectMS * 1000)
			e.lastSlotClassifyMicros += int64(classifyMS * 1000)
			e.lastTemplatesTried += tried
			e.lastTemplatesMatched += matched
			liveSlots := append([]*TrackedSlot(nil), liveMgr.GetAllSlots()...)
			if len(liveSlots) == 0 {
				fresh.Close()
				liveRemaining = 0
				e.logger.Debug().Int("round", liveRound).Msg("Windows live deployment: no active cards remain")
				break
			}

			sort.SliceStable(liveSlots, func(i, j int) bool {
				pi := windowsCategoryPriority(liveSlots[i].Category)
				pj := windowsCategoryPriority(liveSlots[j].Category)
				if pi != pj { return pi < pj }
				return liveSlots[i].X < liveSlots[j].X
			})

			var chosen *TrackedSlot
			chosenCount := 0
			chosenActivity := 0.0

			anonymousHeroIndex := 0
			for _, slot := range liveSlots {
				key := oneShotKey(slot)
				// Skip every identity explicitly blacklisted for this battle.
				if oneShotDone[key] {
					continue
				}
				if strings.TrimSpace(slot.UnitName) == "" &&
					windowsAnonymousOneShotCategory(slot.Category) &&
					anonymousOneShotDone[slot.Category] {
					continue
				}
				if slot.Category == "Hero" && strings.TrimSpace(slot.UnitName) == "" {
					if anonymousHeroIndex < unknownHeroesDeployed {
						anonymousHeroIndex++
						continue
					}
					anonymousHeroIndex++
				}

				// The farm profile is a TARGET, never a reason to strand a live
				// card. Actual battle-bar contents are authoritative: seasonal
				// troops, a different siege, or a user's chosen hero lineup
				// must still be deployed. Profile counts are used below when
				// the unit is known; deviations are diagnostic only.
				if farmControlled && strings.TrimSpace(slot.UnitName) != "" {
					switch slot.Category {
					case "Hero":
						if !farmProfile.UsesHero(slot.UnitName) {
							e.logger.Debug().Str("unit", slot.UnitName).Msg("live hero differs from farm target; deploying live hero")
						}
					case "Siege", "CC":
						if strings.TrimSpace(farmProfile.Siege) == "" || !strings.EqualFold(strings.TrimSpace(farmProfile.Siege), strings.TrimSpace(slot.UnitName)) {
							e.logger.Debug().Str("unit", slot.UnitName).Msg("live siege/CC differs from farm target; deploying live card")
						}
					case "Troop", "Spell":
						if farmProfile.DesiredCount(slot.UnitName) <= 0 {
							e.logger.Debug().Str("unit", slot.UnitName).Str("category", slot.Category).Msg("live card not in farm target; deploying observed amount")
						}
					}
				}

				activity := GetSlotActivityRatioStatic(fresh, slot.X, slot.Y, w)
				if activity < 0.08 {
					continue
				}

				// Slot ordering/activity decides which card is next. OCR only that
				// selected card instead of every visible card on every rescan.
				// This preserves the live re-indexing safety while removing
				// repeated digit-template work from the Windows hot path.
				chosen = slot
				chosenActivity = activity
				break
			}
			e.lastLiveBarRescans++
			e.lastLiveBarRescanMicros += time.Since(rescanStarted).Microseconds()

			if chosen == nil {
				fresh.Close()
				liveRemaining = 0
				e.logger.Debug().Int("round", liveRound).Msg("Windows live deployment: only spent/ability cards remain")
				break
			}

			ocrStarted := time.Now()
			chosenCount = troopCounter.DetectCount(fresh, chosen, liveMgr.GetBarY())
			e.lastSelectedCardOCRCount++
			e.lastSelectedCardOCRMicros += time.Since(ocrStarted).Microseconds()
			if chosenCount > 50 { chosenCount = 0 }
			if armyState != nil && chosenCount > 0 && strings.TrimSpace(chosen.UnitName) != "" {
				armyState.ObserveRemaining(chosen.UnitName, chosenCount)
			}

			key := oneShotKey(chosen)
			cardAttempts[key]++
			if cardAttempts[key] == 1 {
				e.logger.Info().
					Str("unit", chosen.UnitName).
					Str("category", chosen.Category).
					Int("count", chosenCount).
					Msg("deploying card")
			} else {
				e.logger.Debug().
					Int("round", liveRound).
					Str("unit", chosen.UnitName).
					Str("category", chosen.Category).
					Int("slot_x", chosen.X).
					Int("slot_y", chosen.Y).
					Int("ocr_count", chosenCount).
					Float64("activity", chosenActivity).
					Int("attempt", cardAttempts[key]).
					Msg("reacquired card for deployment reconciliation")
			}

			// Use the current fresh coordinates only. Close the frame before
			// sending ADB input; the card will be reacquired again afterwards.
			fresh.Close()

			if chosen.Category == "Hero" || chosen.Category == "Siege" || chosen.Category == "CC" {
				line := safeLines[len(safeLines)-1]
				pt := image.Pt((line[0].X+line[1].X)/2, (line[0].Y+line[1].Y)/2)
				if armyState != nil {
					armyState.RecordDeploy(chosen.UnitName, chosen.Category, 1, chosen.X, chosen.Y, deploySide, pt, pt)
				}
				tapExec.TapSlot(chosen, 1)
				tapExec.HumanSleep(130, 15)
				tapExec.TapDeployPoint(pt, 1, 1)
				oneShotDone[key] = true
				if strings.TrimSpace(chosen.UnitName) == "" &&
					windowsAnonymousOneShotCategory(chosen.Category) {
					anonymousOneShotDone[chosen.Category] = true
				}
				if chosen.Category == "Hero" && strings.TrimSpace(chosen.UnitName) == "" {
					unknownHeroesDeployed++
					e.logger.Debug().
						Int("anonymous_heroes_deployed", unknownHeroesDeployed).
						Msg("Windows anonymous hero deployed once; advancing structural hero cursor")
				}
				if armyState != nil && strings.TrimSpace(chosen.UnitName) != "" {
					armyState.CompleteOneShot(chosen.UnitName)
				}
				e.logger.Debug().
					Str("unit", chosen.UnitName).
					Str("category", chosen.Category).
					Interface("deploy_point", pt).
					Msg("Windows one-shot card deployed once and permanently blacklisted from re-selection")
				tapExec.HumanSleep(180, 20)
				continue
			}

			count := chosenCount
			desired := 0
			if farmControlled && strings.TrimSpace(chosen.UnitName) != "" {
				desired = farmProfile.DesiredCount(chosen.UnitName)
				// Live OCR is authoritative whenever it produced a sane positive
				// count. Using the profile amount over a smaller live amount can
				// keep firing battlefield taps after the selected card empties,
				// at which point CoC may compact/select another card. That is a
				// direct path to "EDrags skipped, then siege/spells dumped".
				//
				// The profile is only a fallback when OCR genuinely failed.
				if desired > 0 && !profileFirstDeploy[key] {
					profileFirstDeploy[key] = true
					if chosenCount <= 0 {
						count = desired
						e.logger.Debug().
							Str("unit", chosen.UnitName).
							Int("profile_count", desired).
							Msg("farm profile: OCR unavailable; using configured count as guarded fallback")
					} else {
						e.logger.Debug().
							Str("unit", chosen.UnitName).
							Int("profile_count", desired).
							Int("ocr_count", chosenCount).
							Msg("farm profile: live OCR count wins over configured target")
					}
				}
			}
			if count <= 0 {
				// Unknown-count cards use a deliberately bounded burst, then
				// the next loop recaptures/reacquires the whole bar. Never send
				// a full profile count blindly through a shifting troop bar.
				if chosen.Category == "Spell" {
					count = 2
				} else {
					count = 6
				}
			}
			if count > 40 { count = 40 }

			deploySlot(chosen, count)
			if armyState != nil && strings.TrimSpace(chosen.UnitName) != "" {
				armyState.Attempt(chosen.UnitName, count)
			}
			tapExec.HumanSleep(150, 20)

			// Do not trust old coordinates after this point. On the next loop
			// the whole bar is captured and re-indexed from scratch.
			if cardAttempts[key] >= 8 && chosen.UnitName != "" && chosenCount <= 0 {
				if armyState != nil {
					armyState.RecordReplayEvent("failed", chosen.UnitName, chosen.Category)
				}
				e.logger.Warn().
					Str("unit", chosen.UnitName).
					Str("category", chosen.Category).
					Msg("Windows live deployment card persisted with unknown count after 8 bursts; blacklisting to avoid a stuck loop")
				oneShotDone["Troop:"+strings.ToLower(strings.TrimSpace(chosen.UnitName))] = true
				if armyState != nil {
					armyState.Fail(chosen.UnitName)
				}
			}
		}

		// One final read decides whether genuinely deployable normal/spell cards
		// remain. Deployed hero ability cards are intentionally ignored.
		finalFrame, finalErr := tapExec.CaptureFresh()
		if finalErr == nil && !finalFrame.Empty() {
			finalMgr := NewSlotManagerLiveRescan(finalFrame, pCfg, w, h, mBarY, e.templates, e.classify, e.logger)
			finalCounts := troopCounter.DetectCounts(finalFrame, finalMgr.GetAllSlots(), finalMgr.GetBarY())
			liveRemaining = 0
			for _, slot := range finalMgr.GetAllSlots() {
				if slot.Category == "Hero" || slot.Category == "Siege" || slot.Category == "CC" {
					continue
				}
				count := GetCountForSlot(finalCounts, slot.X)
				activity := GetSlotActivityRatioStatic(finalFrame, slot.X, slot.Y, w)
				if count > 0 || activity >= 0.12 {
					liveRemaining++
				}
			}
			finalFrame.Close()
		}

		profileIncomplete := 0
		if armyState != nil {
			profileIncomplete = armyState.IncompleteCount()
			if profileIncomplete > 0 {
				e.logger.Warn().
					Int("profile_incomplete", profileIncomplete).
					Interface("units", armyState.IncompleteUnits()).
					Msg("farm profile still has undeployed expected units")
			}
		}

		totalRemaining := liveRemaining
		if profileIncomplete > totalRemaining {
			totalRemaining = profileIncomplete
		}
		if armyState != nil {
			if totalRemaining > 0 {
				armyState.RecordReplayEvent("deployment_incomplete", "", "")
			} else {
				armyState.RecordReplayEvent("deployment_complete", "", "")
			}
		}

		e.logger.Info().
			Int("visible_remaining", liveRemaining).
			Int("profile_incomplete", profileIncomplete).
			Int("remaining", totalRemaining).
			Msg("Windows dynamic live-bar deployment complete")
		if totalRemaining > 0 {
			return totalRemaining, fmt.Errorf("%d expected/deployable card(s) still incomplete after dynamic live deployment", totalRemaining)
		}
		return 0, nil
	}
	// troopCounter is threaded below to NewHeroManager / NewSweeper /
	// NewVerifier so they can live-OCR per-slot counts at deploy time
	// and reconcile until the slot is truly empty (fixes the
	// "balloons/EDs sometimes don't all get placed" bug).

	// 6. Initialize tap executor
	tapExec := NewTapExecutor(e.client, e.cal, e.logger)
	tapExec.SetFrameProvider(e.frameProvider)
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

	// 7. Build the complete immutable plan before the first deploy tap.
	planner := NewDeployPlanner(slotMgr, pCfg, targetEdge, w, h, e.logger)
	prepared := planner.Prepare(s)
	plans := prepared.Phases
	analysisMS := time.Since(analysisStarted).Milliseconds()

	planLog := e.logger.Info().
		Int64("analysis_ms", analysisMS).
		Int64("camera_ms", cameraMS).
		Int64("slot_ms", slotMS).
		Int64("count_ms", countMS).
		Int64("planner_ms", prepared.BuiltIn.Milliseconds()).
		Int("slots", len(slotMgr.GetAllSlots())).
		Int("resolved_units", prepared.ResolvedUnits).
		Str("target_edge", targetEdge).
		Bool("red_zone_valid", redZone.Valid)
	if len(prepared.MissingUnits) > 0 {
		planLog = planLog.Strs("missing_units", prepared.MissingUnits)
	}
	planLog.Msg("attack plan ready")

	// Planning should normally be only a few seconds. Do not abort a valid
	// attack solely because a slow machine exceeded the target; surface the
	// exact timing instead so the hot path can be tuned without blind delays.
	if analysisMS > 5000 {
		e.logger.Warn().
			Int64("analysis_ms", analysisMS).
			Int64("camera_ms", cameraMS).
			Int64("slot_ms", slotMS).
			Int64("count_ms", countMS).
			Msg("attack preparation exceeded 5s target")
	}
	if e.OnPlanReady != nil {
		e.OnPlanReady(time.Since(analysisStarted), targetEdge)
	}

	// 8. Collect strategy unit names
	strategyNames := GetStrategyUnitNames(s)

	// 9. Execute the prebuilt plan through one deployment scheduler. A whole
	// card transaction (select -> settle -> deploy -> one bounded verify) is
	// atomic from the scheduler's point of view, so no second action can steal
	// the selected troop/spell between taps.
	scheduler := NewDeployScheduler(e.logger)
	for _, plan := range plans {
		if tapExec.DeployBudgetExhausted() {
			remaining := len(slotMgr.GetUndeployedSlots())
			e.logger.Warn().
				Str("phase", plan.Phase.Name).
				Dur("budget", DeployBudget).
				Int("undeployed", remaining).
				Msg("deploy budget exhausted before phases completed; stopping deploy")
			return remaining, fmt.Errorf("deploy budget exhausted (%s); %d slots undeployed", DeployBudget, remaining)
		}

		e.logger.Debug().Str("phase", plan.Phase.Name).Msg("attack phase")
		if e.OnPhaseStart != nil {
			e.OnPhaseStart(plan.Phase.Name, targetEdge)
		}

		spellDeployer := NewSpellDeployerWithCounts(tapExec, pCfg, formulaPtr, w, h, countMap, e.logger)
		spellDeployer.SetCounter(troopCounter, slotMgr.GetBarY())
		for _, planned := range ResolveSpellTargets(plan) {
			if planned.Slot == nil {
				continue
			}
			up := planned
			scheduler.Run("spell:"+up.Unit.Name, func() {
				if e.OnUnitDeploy != nil {
					e.OnUnitDeploy(up.Unit.Name, up.Slot.X, slotMgr.GetSlotY())
				}
				tapExec.TapSlot(up.Slot, 8)
				// Keep the empirically safe card-selection settle; removing this
				// saves little but can deploy the previously-selected card.
				tapExec.HumanSleep(150, 30)
				if spellDeployer.DeploySpell(up.Unit, up.Slot, targetEdge, plan.Phase.Pattern) {
					slotMgr.MarkDeployed(strings.ToLower(up.Unit.Name))
					// Exactly one targeted confirmation. No multi-round OCR loop on
					// the normal path.
					if extra, confirmed := spellDeployer.VerifyAndReconcile(up.Unit, up.Slot, targetEdge, plan.Phase.Pattern, 1); extra > 0 {
						e.logger.Debug().
							Str("unit", up.Unit.Name).
							Int("extra_fired", extra).
							Bool("confirmed_empty", confirmed).
							Msg("spell recovery top-up")
					}
				}
			})
		}

		heroMgr := NewHeroManager(tapExec, slotMgr, pCfg, targetEdge, w, h, formulaPtr, troopCounter, e.logger)
		heroMgr.OnDukeDeployed = func(target string) {
			if e.OnDukePick != nil {
				e.OnDukePick(target, target)
			}
		}

		for _, planned := range ResolveTroopTargets(plan) {
			if planned.Slot == nil {
				continue
			}
			up := planned
			scheduler.Run("troop:"+up.Unit.Name, func() {
				if e.OnUnitDeploy != nil {
					e.OnUnitDeploy(up.Unit.Name, up.Slot.X, slotMgr.GetSlotY())
				}
				tapExec.TapSlot(up.Slot, 8)
				tapExec.HumanSleep(150, 30)
				detectedCount := GetCountForSlot(troopCounts, up.Slot.X)
				heroMgr.DeployTroops(
					up.Unit,
					up.Slot,
					plan.Phase.Pattern,
					plan.Phase.Offset,
					plan.Phase.Pattern,
					deployScreen,
					detectedCount,
				)
			})
		}

		for _, planned := range ResolveSiegeTargets(plan) {
			if planned.Slot == nil {
				continue
			}
			up := planned
			scheduler.Run("siege:"+up.Unit.Name, func() {
				if e.OnUnitDeploy != nil {
					e.OnUnitDeploy(up.Unit.Name, up.Slot.X, slotMgr.GetSlotY())
				}
				tapExec.TapSlot(up.Slot, 8)
				tapExec.HumanSleep(150, 30)
				heroMgr.DeploySiege(up.Unit, up.Slot)
			})
		}

		if strings.Contains(plan.Phase.Name, "Heroes") {
			heroUnits := make([]strategy.Unit, 0)
			for _, up := range ResolveHeroTargets(plan) {
				heroUnits = append(heroUnits, up.Unit)
			}
			if len(heroUnits) > 0 {
				scheduler.Run("heroes:"+plan.Phase.Name, func() {
					heroMgr.DeployHeroes(heroUnits, deployScreen)
				})
			}
		}

		const heroSiegeDefault = 50 * time.Millisecond
		const interPhaseMin = 50 * time.Millisecond
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
				Msg("phase delay exceeds cap; clamping")
			pDelay = maxPhaseDelay
		}
		if pDelay > 0 {
			time.Sleep(pDelay)
		}
	}

	if ops, scheduledFor := scheduler.Stats(); ops > 0 {
		e.logger.Info().
			Uint64("scheduled_actions", ops).
			Dur("scheduler_total", scheduledFor).
			Msg("deployment scheduler completed prepared actions")
	}

	// 10. Recovery-only sweep. On the normal path every planned card has
	// already been marked deployed, so skip the expensive fresh-frame sweep.
	remainingBeforeSweep := len(slotMgr.GetUndeployedSlots())
	if remainingBeforeSweep > 0 {
		e.logger.Debug().Int("remaining", remainingBeforeSweep).Msg("running recovery sweep for unresolved cards")
		sweeper := NewSweeper(tapExec, slotMgr, pCfg, deployLine, w, h, formulaPtr, troopCounter, s.EventTroopsAutoDeployEnabled(), e.logger)
		sweeper.Sweep(strategyNames, countMap)
	}

	// 11. One checkpoint only when something still appears unresolved.
	remainingAfterSweep := len(slotMgr.GetUndeployedSlots())
	if remainingAfterSweep == 0 {
		e.logger.Info().Dur("deploy_total", time.Since(analysisStarted)).Msg("deployment complete without recovery scan")
		return 0, nil
	}

	verifier := NewVerifier(tapExec, slotMgr, pCfg, targetEdge, w, h, FastVerifyConfig(), troopCounter, e.logger)
	remainingCount := verifier.VerifyAll()
	e.logger.Info().
		Int("remaining", remainingCount).
		Dur("deploy_total", time.Since(analysisStarted)).
		Msg("deployment recovery checkpoint complete")
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
