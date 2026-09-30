package attack

import (
	"image"
	"sort"
	"sync"

	"github.com/Ducky705/ClashGO/internal/vision"
	"github.com/rs/zerolog"
	"gocv.io/x/gocv"
)

// RedZone represents detected deployment boundary.
type RedZone struct {
	BBox     image.Rectangle
	Valid    bool
	Contours int
	// Boundary contains a detached copy of the detected red-line contour.
	// It survives after OpenCV contour objects are released and lets the
	// deploy-line calculator follow irregular bases instead of using only BBox.
	Boundary []image.Point
}


// boundarySamples extracts sparse boundary points from the accepted red-zone
// rectangle. Scanning only inside that rectangle avoids unrelated red UI
// elements while preserving irregular village geometry.
func boundarySamples(mask gocv.Mat, rect image.Rectangle) (left, right, top, bottom []image.Point) {
	if mask.Empty() || rect.Empty() {
		return nil, nil, nil, nil
	}
	w, h := mask.Cols(), mask.Rows()
	x0, x1 := rect.Min.X, rect.Max.X
	y0, y1 := rect.Min.Y, rect.Max.Y
	if x0 < 0 { x0 = 0 }
	if y0 < 0 { y0 = 0 }
	if x1 > w { x1 = w }
	if y1 > h { y1 = h }
	if x1 <= x0 || y1 <= y0 {
		return nil, nil, nil, nil
	}

	const sampleStep = 8
	for y := y0; y < y1; y += sampleStep {
		lx, rx := -1, -1
		for x := x0; x < x1; x++ {
			if mask.GetUCharAt(y, x) == 0 {
				continue
			}
			if lx < 0 { lx = x }
			rx = x
		}
		if lx >= 0 {
			left = append(left, image.Pt(lx, y))
			right = append(right, image.Pt(rx, y))
		}
	}
	for x := x0; x < x1; x += sampleStep {
		ty, by := -1, -1
		for y := y0; y < y1; y++ {
			if mask.GetUCharAt(y, x) == 0 {
				continue
			}
			if ty < 0 { ty = y }
			by = y
		}
		if ty >= 0 {
			top = append(top, image.Pt(x, ty))
			bottom = append(bottom, image.Pt(x, by))
		}
	}
	return left, right, top, bottom
}

func redZoneFromRect(mask gocv.Mat, rect image.Rectangle, contours int) RedZone {
	left, right, top, bottom := boundarySamples(mask, rect)
	return RedZone{
		BBox:           rect,
		Valid:          true,
		Contours:       contours,
		LeftBoundary:   left,
		RightBoundary:  right,
		TopBoundary:    top,
		BottomBoundary: bottom,
	}
}

// RedLineDetector finds the red deployment boundary on screen.
type RedLineDetector struct {
	logger zerolog.Logger
}

// NewRedLineDetector creates detector.
func NewRedLineDetector(logger zerolog.Logger) *RedLineDetector {
	return &RedLineDetector{logger: logger.With().Str("component", "redline").Logger()}
}

// Detect finds the red deployment boundary in the screenshot.
// Returns bounding box of the red zone (the no-deploy area).
// Troops must deploy OUTSIDE this box.
func (r *RedLineDetector) Detect(screen gocv.Mat, uiCutoff int) RedZone {
	h, w := screen.Rows(), screen.Cols()

	roi := screen
	if uiCutoff < h {
		roi = screen.Region(image.Rect(0, 0, w, uiCutoff))
	}
	defer roi.Close()

	mask := r.detectRedMask(roi)
	defer mask.Close()

	contours := gocv.FindContours(mask, gocv.RetrievalExternal, gocv.ChainApproxSimple)
	if contours.Size() == 0 {
		r.logger.Debug().Msg("no red zone contours found")
		return RedZone{Valid: false}
	}

	type bbox struct {
		rect   image.Rectangle
		area   float64
		points []image.Point
	}
	var boxes []bbox

	for i := 0; i < contours.Size(); i++ {
		cnt := contours.At(i)
		area := gocv.ContourArea(cnt)
		if area < 400 {
			continue
		}
		rect := gocv.BoundingRect(cnt)
		pts := make([]image.Point, 0, cnt.Size())
		for j := 0; j < cnt.Size(); j++ {
			pts = append(pts, cnt.At(j))
		}
		boxes = append(boxes, bbox{rect: rect, area: area, points: pts})
	}

	if len(boxes) == 0 {
		r.logger.Debug().Msg("no valid red zone contours (all too small)")
		return RedZone{Valid: false}
	}

	sort.Slice(boxes, func(i, j int) bool {
		return boxes[i].area > boxes[j].area
	})

	minW := int(float64(w) * 0.55)
	minH := int(float64(uiCutoff) * 0.55)

	for _, b := range boxes {
		rect := b.rect

		if rect.Dx() < minW || rect.Dy() < minH {
			continue
		}

		if rect.Dx() > int(float64(w)*0.97) && rect.Dy() > int(float64(uiCutoff)*0.97) {
			continue
		}

		r.logger.Debug().
			Int("x", rect.Min.X).
			Int("y", rect.Min.Y).
			Int("w", rect.Dx()).
			Int("h", rect.Dy()).
			Float64("area", b.area).
			Msg("red zone detected")

		return RedZone{
			BBox:     rect,
			Valid:    true,
			Contours: contours.Size(),
			Boundary: append([]image.Point(nil), b.points...),
		}
	}

	xMin, yMin := w, uiCutoff
	xMax, yMax := 0, 0
	for _, b := range boxes {
		if b.rect.Min.X < xMin {
			xMin = b.rect.Min.X
		}
		if b.rect.Min.Y < yMin {
			yMin = b.rect.Min.Y
		}
		if b.rect.Max.X > xMax {
			xMax = b.rect.Max.X
		}
		if b.rect.Max.Y > yMax {
			yMax = b.rect.Max.Y
		}
	}

	combined := image.Rect(xMin, yMin, xMax, yMax)
	if combined.Dx() >= minW && combined.Dy() >= minH {
		r.logger.Debug().
			Int("x", xMin).Int("y", yMin).
			Int("w", combined.Dx()).Int("h", combined.Dy()).
			Msg("red zone detected (combined contours)")

		boundary := make([]image.Point, 0)
		for _, b := range boxes {
			boundary = append(boundary, b.points...)
		}
		return RedZone{
			BBox:     combined,
			Valid:    true,
			Contours: contours.Size(),
			Boundary: boundary,
		}
	}

	r.logger.Warn().Msg("red zone detection failed: no contour spans 55% of playfield")
	return RedZone{Valid: false}
}

// redlineKernels are static structuring elements reused across every
// detectRedMask call. Building them once avoids repeatedly allocating +
// freeing kernel Mats on the per-deploy hot path.
var (
	redlineKernelsOnce              sync.Once
	redlineKh, redlineKv, redlineKs gocv.Mat
)

func redlineKernels() (gocv.Mat, gocv.Mat, gocv.Mat) {
	redlineKernelsOnce.Do(func() {
		redlineKh = gocv.GetStructuringElement(gocv.MorphRect, image.Pt(35, 3))
		redlineKv = gocv.GetStructuringElement(gocv.MorphRect, image.Pt(3, 35))
		redlineKs = gocv.GetStructuringElement(gocv.MorphRect, image.Pt(9, 9))
	})
	return redlineKh, redlineKv, redlineKs
}

// detectRedMask creates binary mask of red/orange/pink/magenta pixels.
// These colors form the deployment boundary dashed line.
func (r *RedLineDetector) detectRedMask(roi gocv.Mat) gocv.Mat {
	hsv := vision.GetMat(roi.Rows(), roi.Cols(), gocv.MatTypeCV8UC3)
	defer vision.PutMat(hsv)
	gocv.CvtColor(roi, &hsv, gocv.ColorBGRToHSV)

	m1 := vision.GetMatFrom(hsv)
	defer vision.PutMat(m1)
	m2 := vision.GetMatFrom(hsv)
	defer vision.PutMat(m2)
	m3 := vision.GetMatFrom(hsv)
	defer vision.PutMat(m3)
	m4 := vision.GetMatFrom(hsv)
	defer vision.PutMat(m4)
	m5 := vision.GetMatFrom(hsv)
	defer vision.PutMat(m5)

	gocv.InRangeWithScalar(hsv, gocv.NewScalar(0, 100, 100, 0), gocv.NewScalar(12, 255, 255, 0), &m1)
	gocv.InRangeWithScalar(hsv, gocv.NewScalar(168, 100, 100, 0), gocv.NewScalar(180, 255, 255, 0), &m2)
	gocv.InRangeWithScalar(hsv, gocv.NewScalar(10, 120, 120, 0), gocv.NewScalar(24, 255, 255, 0), &m3)
	gocv.InRangeWithScalar(hsv, gocv.NewScalar(140, 60, 160, 0), gocv.NewScalar(170, 220, 255, 0), &m4)
	gocv.InRangeWithScalar(hsv, gocv.NewScalar(150, 80, 140, 0), gocv.NewScalar(175, 255, 255, 0), &m5)

	mask := gocv.NewMat()

	gocv.BitwiseOr(m1, m2, &mask)

	tmp := vision.GetMatFrom(hsv)
	defer vision.PutMat(tmp)
	gocv.BitwiseOr(mask, m3, &tmp)
	tmp.CopyTo(&mask)
	gocv.BitwiseOr(mask, m4, &tmp)
	tmp.CopyTo(&mask)
	gocv.BitwiseOr(mask, m5, &tmp)
	tmp.CopyTo(&mask)

	kh, kv, ks := redlineKernels()
	gocv.MorphologyEx(mask, &tmp, gocv.MorphClose, kh)
	tmp.CopyTo(&mask)
	gocv.MorphologyEx(mask, &tmp, gocv.MorphClose, kv)
	tmp.CopyTo(&mask)
	gocv.MorphologyEx(mask, &tmp, gocv.MorphClose, ks)
	tmp.CopyTo(&mask)

	return mask
}
