package attack

import (
	"image"
	"math/rand"
	"runtime"
	"strings"
	"time"

	"github.com/Ducky705/ClashGO/internal/adb"
	"github.com/Ducky705/ClashGO/internal/game"
	"github.com/rs/zerolog"
	"gocv.io/x/gocv"
)

// DeployBudget bounds the entire deploy phase (strategy phases + spell /
// hero reconcile + sweep + verify) from the moment DeployDynamicV2 starts.
// A CoC battle lasts 3 minutes from the first drop; every reconcile/sweep
// loop below consults DeployBudgetExhausted so a slot whose "empty" check
// keeps failing (observed live: a spent earthquake/spell card stuck in its
// cooldown read as visually non-empty forever) can never keep firing taps
// past the battle timer — the stuck-sweep symptom that burned minutes and
// hundreds of taps after the battle was already over. The budget is ~2x the
// longest legitimately-observed full deployment (~100s) so real attacks are
// never cut short.
const DeployBudget = 170 * time.Second

// TapExecutor handles all tap operations, screen capture, and timing.
type TapExecutor struct {
	client      *adb.Client
	cal         *game.Calibration
	logger      zerolog.Logger
	lineForward bool

	// deployDeadline is when this battle's deploy budget runs out. Zero
	// when no deploy is in progress (or the budget was never started).
	// Set by StartDeployBudget on the attack goroutine; read by the same
	// goroutine's reconcile loops, so no locking is needed.
	deployDeadline time.Time

	// slotResolver maps a lower-cased unit name to its TrackedSlot.
	// The orchestrator wires it to SlotManager.GetSlot after the slot
	// manager is built; deployers use it to pair a unit with its OCR'd
	// count. nil is legal (lookup just returns no slot).
	slotResolver func(unitName string) *TrackedSlot
}

// SetSlotResolver wires the unit-name -> slot lookup used by deployers
// that need to correlate a unit with its slot (e.g. live OCR counts).
func (t *TapExecutor) SetSlotResolver(fn func(unitName string) *TrackedSlot) {
	t.slotResolver = fn
}

// NewTapExecutor creates a new tap executor.
func NewTapExecutor(client *adb.Client, cal *game.Calibration, logger zerolog.Logger) *TapExecutor {
	return &TapExecutor{
		client: client,
		cal:    cal,
		logger: logger.With().Str("component", "tap_executor").Logger(),
	}
}

// CaptureFresh captures a fresh screen from the device.
func (t *TapExecutor) CaptureFresh() (gocv.Mat, error) {
	return t.client.CaptureToMat()
}

// StartDeployBudget arms the deploy deadline at DeployBudget from now.
// Call once per battle before any phase runs (DeployDynamicV2).
func (t *TapExecutor) StartDeployBudget() {
	t.deployDeadline = time.Now().Add(DeployBudget)
}

// StartDeployBudgetAt arms the deploy deadline at an explicit instant
// (used by tests to simulate an already-expired budget).
func (t *TapExecutor) StartDeployBudgetAt(deadline time.Time) {
	t.deployDeadline = deadline
}

// DeployBudgetExhausted reports whether the deploy phase may keep firing
// taps. It is the single abort check every reconcile/sweep/verify loop
// consults between rounds: once true, remaining units are reported
// undeployed and the bot moves on to the battle-end wait instead of
// tapping into (or past) the battle timer.
func (t *TapExecutor) DeployBudgetExhausted() bool {
	return !t.deployDeadline.IsZero() && time.Now().After(t.deployDeadline)
}

// DeployBudgetRemaining returns the time left in the deploy budget (0
// when exhausted or never armed).
func (t *TapExecutor) DeployBudgetRemaining() time.Duration {
	if t.deployDeadline.IsZero() {
		return 0
	}
	rem := time.Until(t.deployDeadline)
	if rem < 0 {
		return 0
	}
	return rem
}

// TapSlot selects a slot with jitter for human-like behavior.
func (t *TapExecutor) TapSlot(slot *TrackedSlot, jitterPx int) {

	ptY := slot.Y
	if strings.Contains(strings.ToLower(slot.UnitName), "warden") {
		ptY -= int(25.0 * t.cal.ScaleY)
	}

	// Hard Windows safety rail: a troop-card selection tap is NEVER allowed
	// outside the bottom battle bar. This protects against stale/manual slot
	// coordinates accidentally landing on HUD buttons such as Surrender.
	if runtime.GOOS == "windows" {
		h := t.cal.PhysicalH
		if h <= 0 { h = 732 }
		minY := int(float64(h) * 0.84)
		maxY := int(float64(h) * 0.975)
		if ptY < minY || ptY > maxY {
			safeY := int(float64(h) * 0.925)
			t.logger.Warn().
				Int("requested_y", ptY).
				Int("safe_y", safeY).
				Str("unit", slot.UnitName).
				Msg("blocked unsafe troop-slot Y outside bottom battle bar")
			ptY = safeY
		}
	}

	jPt := t.addJitter(image.Pt(slot.X, ptY), jitterPx)
	t.logger.Debug().
		Int("x", jPt.X).
		Int("y", jPt.Y).
		Str("unit", slot.UnitName).
		Msg("tapping slot")
	t.client.TapFast(jPt.X, jPt.Y, 0.8)
}

// sanitizeDeployPoint prevents deployment taps from ever landing on the
// lower battle HUD under Windows/BlueStacks. In particular, the Surrender
// button occupies the lower-left HUD and the Overall Damage panel the
// lower-right. If a computed point enters that band, move it upward while
// preserving X so the troop still lands on the same outside edge.
func (t *TapExecutor) sanitizeDeployPoint(pt image.Point) image.Point {
	if runtime.GOOS != "windows" {
		return pt
	}
	w := t.cal.PhysicalW
	h := t.cal.PhysicalH
	if w <= 0 { w = 860 }
	if h <= 0 { h = 732 }

	maxBattleY := int(float64(h) * 0.70)
	if pt.Y > maxBattleY {
		old := pt
		pt.Y = maxBattleY
		t.logger.Warn().
			Int("old_x", old.X).
			Int("old_y", old.Y).
			Int("safe_x", pt.X).
			Int("safe_y", pt.Y).
			Msg("blocked deploy tap in lower HUD; moved above Surrender/damage controls")
	}

	// Extra hard rail for the exact Surrender/End-Battle region on the left.
	if pt.X < int(float64(w)*0.22) && pt.Y > int(float64(h)*0.64) {
		old := pt
		pt.Y = int(float64(h) * 0.62)
		t.logger.Warn().
			Int("old_x", old.X).
			Int("old_y", old.Y).
			Int("safe_x", pt.X).
			Int("safe_y", pt.Y).
			Msg("blocked deploy tap over Surrender button region")
	}
	return pt
}

// TapDeployLine distributes taps along a line from p1 to p2.
// Deployment taps deliberately use sub-2px transport jitter. The previous
// 12-15px Gaussian jitter was large enough to throw otherwise-correct
// red-boundary points back inside the forbidden zone on BlueStacks.
// Direction alternates per call (boustrophedon): down the line, then
// back up the next call, so consecutive passes never restart at the top.
func (t *TapExecutor) TapDeployLine(p1, p2 image.Point, count int, jitterPx int) {
	points := t.calculateLinePoints(p1, p2, count)
	for i := range points {
		points[i] = t.sanitizeDeployPoint(points[i])
	}

	t.lineForward = !t.lineForward
	if !t.lineForward {
		for i, j := 0, len(points)-1; i < j; i, j = i+1, j-1 {
			points[i], points[j] = points[j], points[i]
		}
	}

	for i := 0; i < len(points); {
		rem := len(points) - i
		if rem >= 3 {
			j1 := t.addJitter(points[i], jitterPx)
			j2 := t.addJitter(points[i+1], jitterPx)
			j3 := t.addJitter(points[i+2], jitterPx)
			t.client.TapTriple(j1.X, j1.Y, 1.2, j2.X, j2.Y, 1.2, j3.X, j3.Y, 1.2)
			i += 3
		} else if rem == 2 {
			j1 := t.addJitter(points[i], jitterPx)
			j2 := t.addJitter(points[i+1], jitterPx)
			t.client.TapDual(j1.X, j1.Y, 1.2, j2.X, j2.Y, 1.2)
			i += 2
		} else {
			j1 := t.addJitter(points[i], jitterPx)
			t.client.TapFast(j1.X, j1.Y, 1.0)
			i += 1
		}
		t.sleepBetweenBatches()
	}
}

// TapDeployLineReliable uses individual taps with a larger settle on Windows.
// It is intentionally slower than TapDeployLine, but far more reliable for
// expensive multi-capacity troops (e.g. EDrags) where losing 1-2 gestures is
// worse than spending a few hundred extra milliseconds.
func (t *TapExecutor) TapDeployLineReliable(p1, p2 image.Point, count int, jitterPx int) {
	// Do not use the exact line endpoints. On live bases the endpoint pixels
	// are the first ones to fall outside the legal deployment contour; this
	// produced the repeatable 7/9 EDrag symptom (two endpoint taps rejected).
	// Spread troops over the inner 12%-88% of the verified line instead.
	points := make([]image.Point, 0, count)
	for i := 0; i < count; i++ {
		pct := 0.50
		if count > 1 {
			pct = 0.12 + 0.76*(float64(i)/float64(count-1))
		}
		x, y := intLerp(p1, p2, pct)
		points = append(points, t.sanitizeDeployPoint(image.Pt(x, y)))
	}
	t.lineForward = !t.lineForward
	if !t.lineForward {
		for i, j := 0, len(points)-1; i < j; i, j = i+1, j-1 {
			points[i], points[j] = points[j], points[i]
		}
	}
	for _, pt := range points {
		j := t.addJitter(pt, jitterPx)
		_ = t.client.TapFast(j.X, j.Y, 0.8)
		// 60-70ms is enough separation for CoC while avoiding the visibly
		// sluggish 95ms cadence on 8-10 heavy troops.
		t.client.HumanSleep(65, 10)
	}
}

// TapDeployPoint clusters taps around a single point.
func (t *TapExecutor) TapDeployPoint(pt image.Point, count int, jitterPx int) {
	pt = t.sanitizeDeployPoint(pt)
	for i := 0; i < count; {
		rem := count - i
		if rem >= 3 {
			j1 := t.addJitter(pt, jitterPx)
			j2 := t.addJitter(pt, jitterPx)
			j3 := t.addJitter(pt, jitterPx)
			t.client.TapTriple(j1.X, j1.Y, 1.2, j2.X, j2.Y, 1.2, j3.X, j3.Y, 1.2)
			i += 3
		} else if rem == 2 {
			j1 := t.addJitter(pt, jitterPx)
			j2 := t.addJitter(pt, jitterPx)
			t.client.TapDual(j1.X, j1.Y, 1.2, j2.X, j2.Y, 1.2)
			i += 2
		} else {
			j1 := t.addJitter(pt, jitterPx)
			t.client.TapFast(j1.X, j1.Y, 1.0)
			i += 1
		}
		t.sleepBetweenBatches()
	}
}

// TapDeployFourSides performs rapid 4-side spam deployment.
func (t *TapExecutor) TapDeployFourSides(pCfg PrecisionConfig, targetEdge string, countPerSide int, jitterPx int) {
	edges := []string{"TopRight", "BottomRight", "BottomLeft", "TopLeft"}
	for _, edgeName := range edges {
		edge, ok := pCfg.Edges[edgeName]
		if !ok {
			continue
		}
		p1, p2 := edge.P1, edge.P2

		t.logger.Info().Str("edge", edgeName).Msg("FourSides rapid spam")
		steps := countPerSide
		if steps < 4 {
			steps = 4
		}
		for i := 0; i < steps; i += 3 {
			rem := steps - i
			if rem >= 3 {
				pct1 := float64(i) / float64(steps-1)
				pct2 := float64(i+1) / float64(steps-1)
				pct3 := float64(i+2) / float64(steps-1)
				tx1, ty1 := intLerp(p1, p2, pct1)
				tx2, ty2 := intLerp(p1, p2, pct2)
				tx3, ty3 := intLerp(p1, p2, pct3)
				j1 := t.addJitter(image.Pt(tx1, ty1), jitterPx)
				j2 := t.addJitter(image.Pt(tx2, ty2), jitterPx)
				j3 := t.addJitter(image.Pt(tx3, ty3), jitterPx)
				t.client.TapTriple(j1.X, j1.Y, 1.2, j2.X, j2.Y, 1.2, j3.X, j3.Y, 1.2)
			} else if rem == 2 {
				pct1 := float64(i) / float64(steps-1)
				pct2 := float64(i+1) / float64(steps-1)
				tx1, ty1 := intLerp(p1, p2, pct1)
				tx2, ty2 := intLerp(p1, p2, pct2)
				j1 := t.addJitter(image.Pt(tx1, ty1), jitterPx)
				j2 := t.addJitter(image.Pt(tx2, ty2), jitterPx)
				t.client.TapDual(j1.X, j1.Y, 1.2, j2.X, j2.Y, 1.2)
			} else {
				pct := float64(i) / float64(steps-1)
				tx, ty := intLerp(p1, p2, pct)
				j1 := t.addJitter(image.Pt(tx, ty), jitterPx)
				t.client.TapFast(j1.X, j1.Y, 1.0)
			}
			time.Sleep(45 * time.Millisecond)
		}
	}
	time.Sleep(120 * time.Millisecond)
}

// TapHeroAbility taps a hero slot for ability activation.
func (t *TapExecutor) TapHeroAbility(slot *TrackedSlot) {

	ptY := slot.Y
	if strings.Contains(strings.ToLower(slot.UnitName), "warden") {
		ptY -= int(25.0 * t.cal.ScaleY)
	}
	t.logger.Info().
		Int("x", slot.X).
		Int("y", ptY).
		Str("unit", slot.UnitName).
		Msg("tapping hero ability")
	t.client.TapFast(slot.X, ptY, 4.0)
}

// WaitForSettle waits for deployment to settle.
func (t *TapExecutor) WaitForSettle(duration time.Duration) {
	time.Sleep(duration)
}

// sleepBetweenBatches waits between tap batches.
// Tightened for speed: 60±20ms is the empirical floor below which CoC
// stacks consecutive tap triples as a single deploy gesture. The 10%
// "long pause" branch preserves a touch of variability for the anti-cheat
// heuristic but stays under 100ms.
func (t *TapExecutor) sleepBetweenBatches() {
	sleepBase := 60
	sleepDev := 20
	if rand.Float64() < 0.10 {
		sleepBase = 90
		sleepDev = 25
	}
	t.client.HumanSleep(sleepBase, sleepDev)
}

// HumanSleep wraps client HumanSleep.
func (t *TapExecutor) HumanSleep(baseMs, stdDevMs int) {
	t.client.HumanSleep(baseMs, stdDevMs)
}

// addJitter adds random pixel offset for human-like tap positions.
func (t *TapExecutor) addJitter(pt image.Point, maxPixels int) image.Point {
	if maxPixels <= 0 {
		return pt
	}
	jx := int(float64(maxPixels) * t.cal.ScaleX)
	jy := int(float64(maxPixels) * t.cal.ScaleY)
	if jx <= 0 {
		jx = 1
	}
	if jy <= 0 {
		jy = 1
	}
	return image.Pt(
		pt.X+rand.Intn(jx*2+1)-jx,
		pt.Y+rand.Intn(jy*2+1)-jy,
	)
}

// calculateLinePoints distributes points along a line.
func (t *TapExecutor) calculateLinePoints(p1, p2 image.Point, count int) []image.Point {
	points := make([]image.Point, 0, count)
	for i := 0; i < count; i++ {
		pct := 0.5
		if count > 1 {
			pct = float64(i) / float64(count-1)
		}
		tx, ty := intLerp(p1, p2, pct)
		points = append(points, image.Pt(tx, ty))
	}
	return points
}

// intLerp interpolates between two points.
func intLerp(p1, p2 image.Point, pct float64) (int, int) {
	return int(float64(p1.X) + float64(p2.X-p1.X)*pct),
		int(float64(p1.Y) + float64(p2.Y-p1.Y)*pct)
}
