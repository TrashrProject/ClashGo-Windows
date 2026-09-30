package bot

import (
	"fmt"
	"image"
	"math"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/Ducky705/ClashGO/internal/game"
	"github.com/Ducky705/ClashGO/internal/paths"
	"github.com/Ducky705/ClashGO/internal/vision"
	"gocv.io/x/gocv"
)

// screenshotForPersistence clones a runtime frame before writing it to disk.
// Vision always continues to use the untouched source frame. When privacy mode
// is enabled only the persisted copy receives a strong blur over the player
// identity area, so masking can never influence state classification/OCR.
func (b *Bot) screenshotForPersistence(screen gocv.Mat) gocv.Mat {
	if screen.Empty() {
		return gocv.NewMat()
	}
	out := screen.Clone()
	if out.Empty() || b == nil || b.cfg == nil || !b.cfg.Automation.PrivacyMaskUsername {
		return out
	}

	x0, y0 := b.cal.ScaleRef(12, 8)
	x1, y1 := b.cal.ScaleRef(315, 92)
	if x0 < 0 { x0 = 0 }
	if y0 < 0 { y0 = 0 }
	if x1 > out.Cols() { x1 = out.Cols() }
	if y1 > out.Rows() { y1 = out.Rows() }
	if x1-x0 < 4 || y1-y0 < 4 {
		return out
	}

	region := out.Region(image.Rect(x0, y0, x1, y1))
	defer region.Close()

	k := int(math.Round(31 * math.Max(b.cal.ScaleX, b.cal.ScaleY)))
	if k < 15 { k = 15 }
	if k%2 == 0 { k++ }
	maxK := region.Cols()
	if region.Rows() < maxK { maxK = region.Rows() }
	if maxK%2 == 0 { maxK-- }
	if k > maxK { k = maxK }
	if k < 3 {
		return out
	}

	blurred := gocv.NewMat()
	defer blurred.Close()
	gocv.GaussianBlur(region, &blurred, image.Pt(k, k), 0, 0, gocv.BorderDefault)
	blurred.CopyTo(&region)
	return out
}

// saveAcceptedBaseScreenshot stores the exact opponent frame that passed the
// target decision, before deploy planning/taps mutate the screen.
func (b *Bot) saveAcceptedBaseScreenshot(screen gocv.Mat, gold, elixir, darkElixir, score int) {
	if b == nil || b.cfg == nil || !b.cfg.Search.SaveAcceptedBaseScreenshots || screen.Empty() {
		return
	}

	dir := paths.ResolveConfig("output/accepted_bases")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		b.logger.Warn().Err(err).Str("dir", dir).Msg("could not create accepted-base screenshot directory")
		return
	}

	// Never put the player tag in the filename: privacy mode must protect both
	// pixels and filesystem metadata.
	name := fmt.Sprintf(
		"accepted_%s_G%d_E%d_DE%d_S%d.png",
		time.Now().Format("20060102_150405.000"),
		gold,
		elixir,
		darkElixir,
		score,
	)
	path := filepath.Join(dir, name)

	persisted := b.screenshotForPersistence(screen)
	if persisted.Empty() {
		persisted.Close()
		return
	}
	defer persisted.Close()

	if ok := gocv.IMWrite(path, persisted); !ok {
		b.logger.Warn().Str("path", path).Msg("failed to save accepted-base screenshot")
		return
	}
	b.logger.Info().
		Str("path", path).
		Int("gold", gold).
		Int("elixir", elixir).
		Int("de", darkElixir).
		Int("score", score).
		Msg("accepted-base screenshot saved")

	// Unattended sessions must not grow the output directory without bound.
	// Keep the newest 200 accepted targets; failures to prune are non-fatal.
	if entries, err := os.ReadDir(dir); err == nil {
		type agedFile struct {
			path string
			when time.Time
		}
		files := make([]agedFile, 0, len(entries))
		for _, entry := range entries {
			if entry.IsDir() || filepath.Ext(entry.Name()) != ".png" {
				continue
			}
			if info, statErr := entry.Info(); statErr == nil {
				files = append(files, agedFile{path: filepath.Join(dir, entry.Name()), when: info.ModTime()})
			}
		}
		if len(files) > 200 {
			sort.Slice(files, func(i, j int) bool { return files[i].when.Before(files[j].when) })
			for _, old := range files[:len(files)-200] {
				_ = os.Remove(old.path)
			}
		}
	}
}

func prunePNGDir(dir string, keep int) {
	if keep <= 0 {
		return
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	type agedFile struct {
		path string
		when time.Time
	}
	files := make([]agedFile, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".png" {
			continue
		}
		if info, statErr := entry.Info(); statErr == nil {
			files = append(files, agedFile{path: filepath.Join(dir, entry.Name()), when: info.ModTime()})
		}
	}
	if len(files) <= keep {
		return
	}
	sort.Slice(files, func(i, j int) bool { return files[i].when.Before(files[j].when) })
	for _, old := range files[:len(files)-keep] {
		_ = os.Remove(old.path)
	}
}

func nearLootThreshold(value, threshold, withinPercent int) bool {
	if threshold <= 0 || value >= threshold {
		return false
	}
	if withinPercent <= 0 {
		withinPercent = 10
	}
	if withinPercent > 50 {
		withinPercent = 50
	}
	floor := threshold * (100 - withinPercent) / 100
	return value >= floor
}

// maybeSaveNearMissBaseScreenshot samples only rejected targets that were
// genuinely close to at least one configured threshold. This gives debugging
// evidence without turning every matchmaking skip into disk I/O.
func (b *Bot) maybeSaveNearMissBaseScreenshot(screen gocv.Mat, gold, elixir, darkElixir, score, ordinal int) {
	if b == nil || b.cfg == nil || screen.Empty() || !b.cfg.Search.SaveNearMissBaseScreenshots {
		return
	}
	every := b.cfg.Search.NearMissSampleEvery
	if every <= 0 {
		every = 20
	}
	if ordinal <= 0 || ordinal%every != 0 {
		return
	}
	within := b.cfg.Search.NearMissWithinPercent
	if !nearLootThreshold(gold, b.cfg.Search.MinLootGold, within) &&
		!nearLootThreshold(elixir, b.cfg.Search.MinLootElixir, within) &&
		!nearLootThreshold(darkElixir, b.cfg.Search.MinLootDarkElixir, within) {
		return
	}

	dir := paths.ResolveConfig("output/near_miss_bases")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return
	}
	name := fmt.Sprintf(
		"near_miss_%s_G%d_E%d_DE%d_S%d.png",
		time.Now().Format("20060102_150405.000"),
		gold, elixir, darkElixir, score,
	)
	path := filepath.Join(dir, name)
	persisted := b.screenshotForPersistence(screen)
	if persisted.Empty() {
		persisted.Close()
		return
	}
	defer persisted.Close()
	if ok := gocv.IMWrite(path, persisted); !ok {
		return
	}
	prunePNGDir(dir, 100)
	b.logger.Debug().
		Str("path", path).
		Int("gold", gold).
		Int("elixir", elixir).
		Int("de", darkElixir).
		Int("score", score).
		Msg("saved sampled near-miss target")
}

type collectorTarget struct {
	kind string
	point image.Point
	score float64
}

type collectorColorRule struct {
	kind string
	lower gocv.Scalar
	upper gocv.Scalar
	minArea float64
	maxArea float64
	minAspect float64
	maxAspect float64
}

// maybeCollectVillageResources uses only the already-owned runtime frame.
// It never asks ADB for another screenshot and only schedules taps after the
// caller has confirmed MainVillage and seqRunning=false.
func (b *Bot) maybeCollectVillageResources(screen gocv.Mat) {
	if b == nil || b.cfg == nil || !b.cfg.Automation.AutoCollectors || screen.Empty() {
		return
	}
	if b.seqRunning.Load() || b.paused.Load() || b.ctx.Err() != nil {
		return
	}

	interval := b.cfg.Automation.CollectorInterval.Duration
	if interval <= 0 {
		interval = 10 * time.Minute
	}
	if !b.lastCollectorSweep.IsZero() && time.Since(b.lastCollectorSweep) < interval {
		return
	}
	if !b.collectorSweepInFlight.CompareAndSwap(false, true) {
		return
	}

	targets := b.findCollectorTargets(screen)
	b.lastCollectorSweep = time.Now()
	if len(targets) == 0 {
		b.collectorSweepInFlight.Store(false)
		b.logger.Debug().Msg("collector sweep found no high-confidence resource bubbles")
		return
	}

	go func(targets []collectorTarget) {
		defer b.collectorSweepInFlight.Store(false)
		for _, target := range targets {
			if b.ctx.Err() != nil || b.seqRunning.Load() || b.paused.Load() {
				return
			}
			confirmed, before, ok := b.confirmCollectorTarget(target)
			if !ok {
				b.logger.Debug().Str("collector", target.kind).Msg("collector candidate did not survive two-frame verification")
				continue
			}
			target = confirmed
			if err := b.client.TapFast(target.point.X, target.point.Y, 0.8); err != nil {
				b.logger.Debug().Err(err).Str("collector", target.kind).Msg("collector tap failed")
				continue
			}
			b.recordActivity()

			verifiedGain := false
			select {
			case <-b.ctx.Done():
				return
			case <-time.After(450 * time.Millisecond):
			}
			if fresh, err := b.runtimeFrameFresh(1500 * time.Millisecond); err == nil && !fresh.Empty() {
				after := b.readCollectorResourceSnapshot(fresh)
				verifiedGain = collectorResourceIncreased(target.kind, before, after)
				fresh.Close()
			}

			logEvent := b.logger.Info().
				Str("collector", target.kind).
				Int("x", target.point.X).
				Int("y", target.point.Y).
				Float64("confidence_score", target.score).
				Bool("resource_gain_verified", verifiedGain)
			logEvent.Msg("collector resource bubble tapped")
		}
	}(targets)
}

func (b *Bot) confirmCollectorTarget(original collectorTarget) (collectorTarget, game.VillageResourceSnapshot, bool) {
	fresh, err := b.runtimeFrameFresh(1500 * time.Millisecond)
	if err != nil || fresh.Empty() {
		if err == nil {
			fresh.Close()
		}
		return collectorTarget{}, game.VillageResourceSnapshot{}, false
	}
	defer fresh.Close()

	state, _ := b.classify(fresh)
	if state != game.StateMainVillage {
		return collectorTarget{}, game.VillageResourceSnapshot{}, false
	}

	candidates := b.findCollectorTargets(fresh)
	maxDX := 34.0 * math.Max(b.cal.ScaleX, 0.1)
	maxDY := 34.0 * math.Max(b.cal.ScaleY, 0.1)
	for _, candidate := range candidates {
		if candidate.kind != original.kind {
			continue
		}
		dx := math.Abs(float64(candidate.point.X - original.point.X))
		dy := math.Abs(float64(candidate.point.Y - original.point.Y))
		if dx <= maxDX && dy <= maxDY {
			return candidate, b.readCollectorResourceSnapshot(fresh), true
		}
	}
	return collectorTarget{}, game.VillageResourceSnapshot{}, false
}

func (b *Bot) readCollectorResourceSnapshot(screen gocv.Mat) game.VillageResourceSnapshot {
	if b == nil || b.cal == nil || b.templates == nil || screen.Empty() {
		return game.VillageResourceSnapshot{}
	}
	// Use a short-lived reader so the collector verification never races the
	// background resource tracker through the recognizer's internal buffers.
	reader := game.NewVillageResourceReader(b.cal, b.templates, b.logger)
	defer reader.Close()
	return reader.Read(screen)
}

func collectorResourceIncreased(kind string, before, after game.VillageResourceSnapshot) bool {
	switch kind {
	case "gold":
		return before.GoldValid && after.GoldValid && after.Gold > before.Gold
	case "elixir":
		return before.ElixirValid && after.ElixirValid && after.Elixir > before.Elixir
	case "dark_elixir":
		return before.DarkValid && after.DarkValid && after.DarkElixir > before.DarkElixir
	default:
		return false
	}
}

// findCollectorTargets finds at most one compact, high-saturation resource
// bubble for each resource family. The ROI excludes the fixed top/bottom HUD,
// and the geometry gates deliberately reject large buildings/buttons.
// One tap per detected family matches current Clash collection behaviour while
// keeping the automation bounded to at most three taps per interval.
func (b *Bot) findCollectorTargets(screen gocv.Mat) []collectorTarget {
	x0, y0 := b.cal.ScaleRef(80, 95)
	x1, y1 := b.cal.ScaleRef(790, 565)
	if x0 < 0 { x0 = 0 }
	if y0 < 0 { y0 = 0 }
	if x1 > screen.Cols() { x1 = screen.Cols() }
	if y1 > screen.Rows() { y1 = screen.Rows() }
	if x1-x0 < 20 || y1-y0 < 20 {
		return nil
	}

	roi := screen.Region(image.Rect(x0, y0, x1, y1))
	defer roi.Close()
	hsv := vision.GetMat(roi.Rows(), roi.Cols(), gocv.MatTypeCV8UC3)
	defer vision.PutMat(hsv)
	gocv.CvtColor(roi, &hsv, gocv.ColorBGRToHSV)

	rules := []collectorColorRule{
		{
			kind: "gold",
			lower: gocv.NewScalar(16, 150, 175, 0),
			upper: gocv.NewScalar(39, 255, 255, 0),
			minArea: 70, maxArea: 650, minAspect: 0.68, maxAspect: 1.45,
		},
		{
			kind: "elixir",
			lower: gocv.NewScalar(135, 105, 135, 0),
			upper: gocv.NewScalar(174, 255, 255, 0),
			minArea: 45, maxArea: 900, minAspect: 0.55, maxAspect: 1.85,
		},
		{
			kind: "dark_elixir",
			lower: gocv.NewScalar(125, 85, 45, 0),
			upper: gocv.NewScalar(174, 255, 135, 0),
			minArea: 65, maxArea: 460, minAspect: 0.75, maxAspect: 1.35,
		},
	}

	out := make([]collectorTarget, 0, len(rules))
	scaleArea := b.cal.ScaleX * b.cal.ScaleY
	if scaleArea <= 0 { scaleArea = 1 }

	for _, rule := range rules {
		mask := vision.GetMat(roi.Rows(), roi.Cols(), gocv.MatTypeCV8UC1)
		gocv.InRangeWithScalar(hsv, rule.lower, rule.upper, &mask)

		kernel := gocv.GetStructuringElement(gocv.MorphEllipse, image.Pt(3, 3))
		gocv.MorphologyEx(mask, &mask, gocv.MorphOpen, kernel)
		kernel.Close()

		contours := gocv.FindContours(mask, gocv.RetrievalExternal, gocv.ChainApproxSimple)
		best := collectorTarget{kind: rule.kind}
		for i := 0; i < contours.Size(); i++ {
			c := contours.At(i)
			area := gocv.ContourArea(c)
			refArea := area / scaleArea
			if refArea < rule.minArea || refArea > rule.maxArea {
				continue
			}
			rect := gocv.BoundingRect(c)
			if rect.Dx() < 3 || rect.Dy() < 3 {
				continue
			}
			refW := float64(rect.Dx()) / math.Max(b.cal.ScaleX, 0.1)
			refH := float64(rect.Dy()) / math.Max(b.cal.ScaleY, 0.1)
			if refW < 10 || refW > 38 || refH < 10 || refH > 38 {
				continue
			}
			aspect := refW / refH
			if aspect < rule.minAspect || aspect > rule.maxAspect {
				continue
			}
			fill := area / float64(rect.Dx()*rect.Dy())
			if fill < 0.44 {
				continue
			}

			// Compact + filled + moderate area wins over broad scenery blobs.
			score := fill*100 + math.Min(refArea, 300)/10
			if score <= best.score {
				continue
			}
			best.score = score
			best.point = image.Pt(
				x0+rect.Min.X+rect.Dx()/2,
				y0+rect.Min.Y+rect.Dy()/2,
			)
		}
		contours.Close()
		vision.PutMat(mask)

		if best.score > 0 {
			duplicate := false
			for _, prior := range out {
				dx := float64(best.point.X-prior.point.X) / math.Max(b.cal.ScaleX, 0.1)
				dy := float64(best.point.Y-prior.point.Y) / math.Max(b.cal.ScaleY, 0.1)
				if dx*dx+dy*dy < 18*18 {
					duplicate = true
					break
				}
			}
			if !duplicate {
				out = append(out, best)
			}
		}
	}
	return out
}
