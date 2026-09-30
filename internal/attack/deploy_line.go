package attack

import (
	"image"

	"github.com/rs/zerolog"
)

const (
	standoff    = 80
	margin      = 30
	yTopMin     = 110
	yBotPad     = 80
	xMinPad     = 60
	linePoints  = 15
	lineSpacing = 35
)

// DeployLine represents a calculated deployment line.
type DeployLine struct {
	Points  []image.Point
	Side    string
	Anchor  image.Point
	Outside bool
}

// DeployLineCalculator computes deployment lines dynamically.
type DeployLineCalculator struct {
	logger zerolog.Logger
}

// NewDeployLineCalculator creates calculator.
func NewDeployLineCalculator(logger zerolog.Logger) *DeployLineCalculator {
	return &DeployLineCalculator{logger: logger.With().Str("component", "deploy_line").Logger()}
}

// Calculate returns a deployment line outside the red zone.
// Picks edge with most free space, places line 80px outside red zone.
func (d *DeployLineCalculator) Calculate(
	zone RedZone,
	screenW, screenH, uiCutoff int,
	preferSide string,
	count int,
) DeployLine {
	if count < 2 {
		count = linePoints
	}

	if !zone.Valid {
		return d.fallbackLine(screenW, screenH, uiCutoff, count)
	}

	freeSpace := map[string]int{
		"left":   zone.BBox.Min.X,
		"right":  screenW - zone.BBox.Max.X,
		"top":    zone.BBox.Min.Y,
		"bottom": uiCutoff - zone.BBox.Max.Y,
	}

	side := d.pickSide(freeSpace, preferSide)

	if freeSpace[side] <= 0 {
		d.logger.Warn().Str("side", side).Msg("no space on preferred side, using fallback")
		return d.fallbackLine(screenW, screenH, uiCutoff, count)
	}

	xLo := xMinPad
	xHi := screenW - xMinPad
	yTop := yTopMin
	yBot := uiCutoff - yBotPad

	// Prefer the real red-line contour when available. The old BBox-only
	// geometry turns every base into a rectangle; following the contour keeps
	// deployment points consistently outside irregular/diamond-shaped bases.
	if contourPoints := d.contourAwarePoints(zone, side, xLo, xHi, yTop, yBot, count); len(contourPoints) == count {
		anchor := contourPoints[len(contourPoints)/2]
		d.logger.Info().
			Str("side", side).
			Int("points", len(contourPoints)).
			Interface("anchor", anchor).
			Msg("calculated contour-aware deployment line")
		return DeployLine{Points: contourPoints, Side: side, Anchor: anchor, Outside: true}
	}

	var points []image.Point

	switch side {
	case "left":
		x := zone.BBox.Min.X - standoff
		if x < xLo {
			x = xLo
		}
		yStart := zone.BBox.Min.Y + 30
		yEnd := zone.BBox.Max.Y - 30
		if yStart < yTop {
			yStart = yTop
		}
		if yEnd > yBot {
			yEnd = yBot
		}
		points = d.linspaceY(x, yStart, yEnd, count)

	case "right":
		x := zone.BBox.Max.X + standoff
		if x > xHi {
			x = xHi
		}
		yStart := zone.BBox.Min.Y + 30
		yEnd := zone.BBox.Max.Y - 30
		if yStart < yTop {
			yStart = yTop
		}
		if yEnd > yBot {
			yEnd = yBot
		}
		points = d.linspaceY(x, yStart, yEnd, count)

	case "top":
		y := zone.BBox.Min.Y - standoff
		if y < yTop {
			y = yTop
		}
		xStart := zone.BBox.Min.X + 30
		xEnd := zone.BBox.Max.X - 30
		if xStart < xLo {
			xStart = xLo
		}
		if xEnd > xHi {
			xEnd = xHi
		}
		points = d.linspaceX(xStart, xEnd, y, count)

	case "bottom":
		y := zone.BBox.Max.Y + standoff
		if y > yBot {
			y = yBot
		}
		xStart := zone.BBox.Min.X + 30
		xEnd := zone.BBox.Max.X - 30
		if xStart < xLo {
			xStart = xLo
		}
		if xEnd > xHi {
			xEnd = xHi
		}
		points = d.linspaceX(xStart, xEnd, y, count)
	}

	for i := range points {
		points[i].X = clamp(points[i].X, xLo, xHi)
		points[i].Y = clamp(points[i].Y, yTop, yBot)
	}

	anchor := points[len(points)/2]

	d.logger.Info().
		Str("side", side).
		Int("points", len(points)).
		Interface("anchor", anchor).
		Msg("calculated deployment line")

	return DeployLine{
		Points:  points,
		Side:    side,
		Anchor:  anchor,
		Outside: true,
	}
}

// pickSide selects edge with most free space.
func (d *DeployLineCalculator) pickSide(freeSpace map[string]int, prefer string) string {
	const preferredSafeSpace = 60
	if prefer != "" && freeSpace[prefer] >= preferredSafeSpace {
		return prefer
	}

	best := "left"
	bestSpace := -1
	for side, space := range freeSpace {
		if space > bestSpace {
			bestSpace = space
			best = side
		}
	}
	if prefer != "" && best != prefer {
		d.logger.Info().
			Str("preferred_side", prefer).
			Int("preferred_space", freeSpace[prefer]).
			Str("selected_side", best).
			Int("selected_space", bestSpace).
			Msg("preferred attack side too cramped; using safer red-zone side")
	}
	return best
}

// fallbackLine creates a line when no red zone is detected.
// Uses fixed positions near screen edges.
func (d *DeployLineCalculator) fallbackLine(screenW, screenH, uiCutoff, count int) DeployLine {

	x := xMinPad
	yStart := yTopMin
	yEnd := uiCutoff - yBotPad

	points := d.linspaceY(x, yStart, yEnd, count)
	anchor := points[len(points)/2]

	d.logger.Warn().Msg("using fallback deployment line (no red zone)")

	return DeployLine{
		Points:  points,
		Side:    "left",
		Anchor:  anchor,
		Outside: false,
	}
}

// linspaceY generates points with varying Y, fixed X.
func (d *DeployLineCalculator) linspaceY(x, yStart, yEnd, count int) []image.Point {
	points := make([]image.Point, count)
	for i := 0; i < count; i++ {
		t := float64(i) / float64(count-1)
		y := yStart + int(float64(yEnd-yStart)*t)
		points[i] = image.Pt(x, y)
	}
	return points
}

// linspaceX generates points with varying X, fixed Y.
func (d *DeployLineCalculator) linspaceX(xStart, xEnd, y, count int) []image.Point {
	points := make([]image.Point, count)
	for i := 0; i < count; i++ {
		t := float64(i) / float64(count-1)
		x := xStart + int(float64(xEnd-xStart)*t)
		points[i] = image.Pt(x, y)
	}
	return points
}


func (d *DeployLineCalculator) contourAwarePoints(zone RedZone, side string, xLo, xHi, yTop, yBot, count int) []image.Point {
	if count < 2 {
		return nil
	}

	var profile []image.Point
	switch side {
	case "left":
		profile = zone.LeftBoundary
	case "right":
		profile = zone.RightBoundary
	case "top":
		profile = zone.TopBoundary
	case "bottom":
		profile = zone.BottomBoundary
	default:
		return nil
	}
	if len(profile) < 4 {
		return nil
	}

	points := make([]image.Point, 0, count)
	for i := 0; i < count; i++ {
		t := float64(i) / float64(count-1)
		idx := int(t * float64(len(profile)-1))
		p := profile[idx]

		switch side {
		case "left":
			p.X -= standoff
		case "right":
			p.X += standoff
		case "top":
			p.Y -= standoff
		case "bottom":
			p.Y += standoff
		}

		p.X = clamp(p.X, xLo, xHi)
		p.Y = clamp(p.Y, yTop, yBot)
		points = append(points, p)
	}
	return points
}

func clamp(v, min, max int) int {
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}
