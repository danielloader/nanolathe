package camera

import "testing"

func TestLegacyViewportFloorKeepsBothAxesFilled(t *testing.T) {
	c := &Camera{ViewW: 640, ViewH: 480, MapW: 8192, MapH: 8192}
	full := c.MinZoom()
	c.ViewportZoomFloor = true
	legacy := c.MinZoom()
	if full != 52 || legacy != 64 {
		t.Fatalf("map fit / viewport fill = %d/%d, want 52/64", full, legacy)
	}
	// Round upward at the legacy floor; neither axis may expose spare space.
	c.MapW = 1000
	c.SetZoomAbout(400, 250, ZoomUnit/4)
	w, h := c.BattleView()
	if w > c.MapW || h > c.MapH {
		t.Fatalf("legacy floor leaves an uncovered axis: %dx%d", w, h)
	}
}
