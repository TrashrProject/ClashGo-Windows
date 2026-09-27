package attack

import (
	"image"
	"math"
	"sort"

	"gocv.io/x/gocv"
)

type deploymentBoundaryPoint struct {
	Point    image.Point
	Angle    float64
	RedRatio float64
}

// buildDeploymentBoundary samples the live no-deploy overlay radially around
// the village and records the first consistently-safe point after the red
// region on each ray. This gives us a lightweight dynamic outline of the
// current base instead of trusting only a pre-calibrated straight edge.
func (e *Executor) buildDeploymentBoundary(screen gocv.Mat) []deploymentBoundaryPoint {
	if screen.Empty() {
		return nil
	}
	w, h := screen.Cols(), screen.Rows()
	center := image.Pt(w/2, int(float64(h)*0.43))
	maxBattleY := int(float64(h) * 0.79)
	margin := maxInt(8, int(7*e.cal.ScaleX))
	radiusStep := math.Max(5, 7*e.cal.ScaleX)
	roiRadius := maxInt(4, int(6*e.cal.ScaleX))
	angleSteps := 72 // 5-degree rays: detailed enough without being expensive.

	maxRadius := math.Hypot(float64(w), float64(h))
	out := make([]deploymentBoundaryPoint, 0, angleSteps)

	for i := 0; i < angleSteps; i++ {
		angle := (2 * math.Pi * float64(i)) / float64(angleSteps)
		cosA, sinA := math.Cos(angle), math.Sin(angle)
		sawRed := false
		safeRun := 0
		var candidate image.Point
		var candidateRatio float64

		for r := 12.0; r <= maxRadius; r += radiusStep {
			p := image.Pt(
				center.X+int(math.Round(cosA*r)),
				center.Y+int(math.Round(sinA*r)),
			)
			if p.X < margin || p.X >= w-margin || p.Y < margin || p.Y >= maxBattleY {
				break
			}

			ratio := deploymentRedRatio(screen, p, roiRadius)
			if ratio >= 0.16 {
				sawRed = true
				safeRun = 0
				continue
			}

			// Require two safe samples in a row after having traversed red.
			// This suppresses tiny holes/gaps inside the translucent overlay.
			if sawRed {
				safeRun++
				if safeRun == 1 {
					candidate = p
					candidateRatio = ratio
				}
				if safeRun >= 2 {
					out = append(out, deploymentBoundaryPoint{
						Point: candidate, Angle: angle, RedRatio: candidateRatio,
					})
					break
				}
			}
		}
	}

	e.logger.Debug().
		Int("boundary_points", len(out)).
		Msg("built dynamic deployment boundary from live red overlay")
	return out
}

func nearestBoundaryPoint(points []deploymentBoundaryPoint, target image.Point, maxDistance float64) (image.Point, bool) {
	best := image.Point{}
	bestDist := math.MaxFloat64
	for _, bp := range points {
		dx := float64(bp.Point.X - target.X)
		dy := float64(bp.Point.Y - target.Y)
		d := math.Hypot(dx, dy)
		if d < bestDist {
			bestDist = d
			best = bp.Point
		}
	}
	if bestDist == math.MaxFloat64 || bestDist > maxDistance {
		return image.Point{}, false
	}
	return best, true
}

// adaptDeployPointsToBoundary snaps planned points to the live deployment
// frontier when a matching boundary point is nearby. Points already outside
// the red overlay still go through the standard local verifier afterwards.
func (e *Executor) adaptDeployPointsToBoundary(screen gocv.Mat, planned []image.Point) []image.Point {
	if screen.Empty() || len(planned) == 0 {
		return planned
	}

	boundary := e.buildDeploymentBoundary(screen)
	if len(boundary) < 8 {
		// The overlay may be weak/hidden on this frame. Fall back to the
		// existing local red-zone resolver rather than inventing geometry.
		return e.resolveSafeDeployPoints(screen, planned)
	}

	maxSnap := math.Max(60, 95*e.cal.ScaleX)
	out := make([]image.Point, 0, len(planned))
	for _, p := range planned {
		candidate := p
		if snap, ok := nearestBoundaryPoint(boundary, p, maxSnap); ok {
			candidate = snap
		}
		if safe, ok := e.resolveSafeDeployPoint(screen, candidate); ok {
			out = append(out, safe)
		}
	}

	// Keep the attack's intended ordering along the line. Snapping to a polar
	// boundary can otherwise slightly reorder neighboring samples around a
	// corner and make the deployment look erratic.
	if len(out) > 1 {
		dx := float64(planned[len(planned)-1].X - planned[0].X)
		dy := float64(planned[len(planned)-1].Y - planned[0].Y)
		sort.SliceStable(out, func(i, j int) bool {
			pi := float64(out[i].X-planned[0].X)*dx + float64(out[i].Y-planned[0].Y)*dy
			pj := float64(out[j].X-planned[0].X)*dx + float64(out[j].Y-planned[0].Y)*dy
			return pi < pj
		})
	}
	return out
}
