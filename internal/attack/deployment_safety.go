package attack

import (
	"image"
	"math"

	"gocv.io/x/gocv"
)

// deploymentRedRatio estimates whether a small area is covered by Clash of
// Clans' red "cannot deploy here" tint. It deliberately uses a cheap local
// color signature instead of a full-frame OpenCV pass: mature mobile bots do
// the fast pixel/color check first and reserve heavier vision for ambiguous
// cases.
func deploymentRedRatio(screen gocv.Mat, pt image.Point, radius int) float64 {
	if screen.Empty() || screen.Cols() < 2 || screen.Rows() < 2 {
		return 0
	}
	if radius < 2 {
		radius = 2
	}

	x0 := maxInt(0, pt.X-radius)
	y0 := maxInt(0, pt.Y-radius)
	x1 := minInt(screen.Cols()-1, pt.X+radius)
	y1 := minInt(screen.Rows()-1, pt.Y+radius)
	if x1 <= x0 || y1 <= y0 {
		return 1
	}

	red, sampled := 0, 0
	// Sample every other pixel. A 17x17 ROI therefore costs ~80 reads instead
	// of ~300 and is still more than enough to detect the translucent red mask.
	for y := y0; y <= y1; y += 2 {
		for x := x0; x <= x1; x += 2 {
			b := int(screen.GetUCharAt(y, x*3))
			g := int(screen.GetUCharAt(y, x*3+1))
			r := int(screen.GetUCharAt(y, x*3+2))
			sampled++

			// The no-deploy overlay is translucent, so absolute RGB varies with
			// terrain. Red dominance is much more stable than one exact color.
			if isDeploymentRedRGB(r, g, b) {
				red++
			}
		}
	}
	if sampled == 0 {
		return 1
	}
	return float64(red) / float64(sampled)
}

// resolveSafeDeployPoint keeps a planned deployment point when it is already
// valid. If it lies in the red no-deploy overlay, it walks OUTWARD from the
// village centre until the tint clears. Small tangential probes handle corners
// and irregular red boundaries. It never silently returns a point that is still
// red: callers can skip that tap instead of wasting troops/actions.
func (e *Executor) resolveSafeDeployPoint(screen gocv.Mat, planned image.Point) (image.Point, bool) {
	if screen.Empty() {
		return planned, true
	}

	w, h := screen.Cols(), screen.Rows()
	margin := maxInt(6, int(math.Round(6*e.cal.ScaleX)))
	maxBattleY := minInt(h-8, int(float64(h)*0.79))
	inBounds := func(p image.Point) bool {
		return p.X >= margin && p.X < w-margin && p.Y >= margin && p.Y < maxBattleY
	}
	isSafe := func(p image.Point) bool {
		if !inBounds(p) {
			return false
		}
		return deploymentRedRatio(screen, p, maxInt(5, int(7*e.cal.ScaleX))) < 0.16
	}

	if isSafe(planned) {
		return planned, true
	}

	center := image.Pt(w/2, int(float64(h)*0.43))
	dx := float64(planned.X - center.X)
	dy := float64(planned.Y - center.Y)
	length := math.Hypot(dx, dy)
	if length < 1 {
		dx, dy, length = 0, -1, 1
	}
	ux, uy := dx/length, dy/length
	// Tangent to the outward vector, used at corners/diagonal boundaries.
	tx, ty := -uy, ux

	step := math.Max(5, 6*e.cal.ScaleX)
	maxDist := math.Max(54, 72*e.cal.ScaleX)
	lateral := []float64{0, -10, 10, -20, 20}

	for dist := step; dist <= maxDist; dist += step {
		for _, lat := range lateral {
			p := image.Pt(
				planned.X+int(math.Round(ux*dist+tx*lat*e.cal.ScaleX)),
				planned.Y+int(math.Round(uy*dist+ty*lat*e.cal.ScaleY)),
			)
			if isSafe(p) {
				e.logger.Debug().
					Interface("planned", planned).
					Interface("resolved", p).
					Float64("red_ratio_before", deploymentRedRatio(screen, planned, maxInt(5, int(7*e.cal.ScaleX)))).
					Float64("red_ratio_after", deploymentRedRatio(screen, p, maxInt(5, int(7*e.cal.ScaleX)))).
					Msg("moved deployment point out of red no-deploy zone")
				return p, true
			}
		}
	}

	e.logger.Warn().
		Interface("planned", planned).
		Float64("red_ratio", deploymentRedRatio(screen, planned, maxInt(5, int(7*e.cal.ScaleX)))).
		Msg("no safe deployment point found near planned coordinate; suppressing tap")
	return image.Point{}, false
}

func (e *Executor) resolveSafeDeployPoints(screen gocv.Mat, planned []image.Point) []image.Point {
	if screen.Empty() {
		return planned
	}
	out := make([]image.Point, 0, len(planned))
	for _, p := range planned {
		if safe, ok := e.resolveSafeDeployPoint(screen, p); ok {
			out = append(out, safe)
		}
	}
	return out
}

func isDeploymentRedRGB(r, g, b int) bool {
	return r >= 105 && r-g >= 24 && r-b >= 18 && r*100 >= g*125
}

func minInt(a, b int) int {
	if a < b { return a }
	return b
}

func maxInt(a, b int) int {
	if a > b { return a }
	return b
}
