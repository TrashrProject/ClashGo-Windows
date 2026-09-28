package attack

import "image"

// windowsDeployCorridor converts the live red-zone bounding box into the
// single deployment line used by the Windows live-bar path. Keep this logic
// pure and regression-tested: it is the safety boundary that prevents troop
// taps from drifting into the red no-deploy polygon or the lower battle HUD.
//
// Bottom is deliberately excluded even when it has the most geometric free
// space because Surrender/End Battle, damage UI and the troop bar live there.
func windowsDeployCorridor(zone RedZone, w, h, uiCutoff int) (side string, p1, p2 image.Point, freeSpace int, ok bool) {
	if !zone.Valid || w <= 0 || h <= 0 || uiCutoff <= 0 {
		return "", image.Point{}, image.Point{}, 0, false
	}

	const (
		edgeMargin = 24
		outsidePad = 34
	)

	free := map[string]int{
		"left":  zone.BBox.Min.X,
		"right": w - zone.BBox.Max.X,
		"top":   zone.BBox.Min.Y,
	}

	// Preserve the existing Windows behavior exactly: widest legal side among
	// left/right/top; ties keep the earlier side (left -> right -> top).
	side = "left"
	freeSpace = free["left"]
	for _, candidate := range []string{"right", "top"} {
		if free[candidate] > freeSpace {
			side = candidate
			freeSpace = free[candidate]
		}
	}

	switch side {
	case "right":
		x := zone.BBox.Max.X + outsidePad
		if x > w-edgeMargin {
			x = w - edgeMargin
		}
		fieldTop := int(float64(h) * 0.22)
		fieldBottom := int(float64(h) * 0.58)
		y1 := clamp(zone.BBox.Min.Y+45, fieldTop, fieldBottom)
		y2 := clamp(zone.BBox.Max.Y-45, fieldTop, fieldBottom)
		if y2-y1 < int(float64(h)*0.12) {
			mid := (fieldTop + fieldBottom) / 2
			half := int(float64(h) * 0.10)
			y1, y2 = mid-half, mid+half
		}
		p1, p2 = image.Pt(x, y1), image.Pt(x, y2)

	case "top":
		y := zone.BBox.Min.Y - outsidePad
		if y < edgeMargin {
			y = edgeMargin
		}
		x1 := clamp(zone.BBox.Min.X+35, edgeMargin, w-edgeMargin)
		x2 := clamp(zone.BBox.Max.X-35, edgeMargin, w-edgeMargin)
		p1, p2 = image.Pt(x1, y), image.Pt(x2, y)

	default: // left
		x := zone.BBox.Min.X - outsidePad
		if x < edgeMargin {
			x = edgeMargin
		}
		fieldTop := int(float64(h) * 0.22)
		fieldBottom := int(float64(h) * 0.58)
		y1 := clamp(zone.BBox.Min.Y+45, fieldTop, fieldBottom)
		y2 := clamp(zone.BBox.Max.Y-45, fieldTop, fieldBottom)
		if y2-y1 < int(float64(h)*0.12) {
			mid := (fieldTop + fieldBottom) / 2
			half := int(float64(h) * 0.10)
			y1, y2 = mid-half, mid+half
		}
		p1, p2 = image.Pt(x, y1), image.Pt(x, y2)
	}

	return side, p1, p2, freeSpace, true
}
