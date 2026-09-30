package bot

import (
	"fmt"
	"image"
	"math"
	"os"
	"path/filepath"
	"strings"
	"time"

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

	account := strings.TrimSpace(b.cfg.Account.PlayerTag)
	account = strings.TrimPrefix(account, "#")
	account = strings.NewReplacer("/", "_", "\\", "_", ":", "_", " ", "_").Replace(account)
	if account == "" {
		account = "account"
	}

	name := fmt.Sprintf(
		"accepted_%s_%s_G%d_E%d_DE%d_S%d.png",
		account,
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
			if err := b.client.TapFast(target.point.X, target.point.Y, 0.8); err != nil {
				b.logger.Debug().Err(err).Str("collector", target.kind).Msg("collector tap failed")
				continue
			}
			b.recordActivity()
			b.logger.Info().
				Str("collector", target.kind).
				Int("x", target.point.X).
				Int("y", target.point.Y).
				Float64("confidence_score", target.score).
				Msg("collector resource bubble tapped")
			select {
			case <-b.ctx.Done():
				return
			case <-time.After(260 * time.Millisecond):
			}
		}
	}(targets)
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
			minArea: 45, maxArea: 900, minAspect: 0.55, maxAspect: 1.85,
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
			minArea: 55, maxArea: 520, minAspect: 0.72, maxAspect: 1.40,
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
			if refW < 6 || refW > 42 || refH < 6 || refH > 42 {
				continue
			}
			aspect := refW / refH
			if aspect < rule.minAspect || aspect > rule.maxAspect {
				continue
			}
			fill := area / float64(rect.Dx()*rect.Dy())
			if fill < 0.38 {
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
