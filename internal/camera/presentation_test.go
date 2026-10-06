package camera

import (
	"fmt"
	"math"
	"testing"
)

func TestPreciseZoomAnchorSurvivesCyclesAndExternalPan(t *testing.T) {
	c := &Camera{X: 4000, Z: 4000, ViewW: 1920, ViewH: 1080, MapW: 16384, MapH: 16384}
	const ax, ay = 813, 487
	wx, wz := float64(c.X)+ax, float64(c.Z)+ay
	var control ZoomController
	for _, target := range []Zoom{ZoomMax, ZoomUnit / 4, ZoomUnit, ZoomMax, ZoomUnit} {
		control.SetTarget(c, ax+OriginX, ay+OriginY, target)
		for control.Step(c) {
			v := c.PresentationView()
			if math.Abs((wx-v.X)*v.Factor-ax) > 1e-8 || math.Abs((wz-v.Z)*v.Factor-ay) > 1e-8 {
				t.Fatalf("anchor drift: %+v", v)
			}
		}
	}
	c.X += 100
	v := c.PresentationView()
	if v.X != float64(c.X) || v.Z != float64(c.Z) {
		t.Fatal("external camera move kept stale fractional placement")
	}
}

func TestPreciseZoomRespectsClampedAxis(t *testing.T) {
	c := &Camera{X: 0, Z: 4000, ViewW: 640, ViewH: 480, MapW: 16384, MapH: 16384}
	c.SetZoomAbout(500+OriginX, 200+OriginY, ZoomUnit/4)
	v := c.PresentationView()
	if math.Abs(v.X*v.Factor+float64(OriginX)) > 1e-8 {
		t.Fatalf("clamped camera has extra displacement: %+v vs %d", v, c.X)
	}
	// The free axis still holds its anchor.
	if math.Abs((4200-v.Z)*v.Factor-200) > 1e-8 {
		t.Fatal("clamping X discarded the Z anchor")
	}
}

// Hard edges beat the pointer anchor; an axis that fits remains centred.
// Check the projected edges at every sample, including fractional factors.
func checkPresentedBounds(t *testing.T, c *Camera) {
	t.Helper()
	v := c.PresentationView()
	for _, axis := range []struct {
		origin                  float64
		size, leading, trailing int32
	}{
		{v.X, c.MapW, OriginX, c.ViewW},
		{v.Z, c.MapH, OriginY, c.ViewH - OriginY},
	} {
		first, last := -axis.origin*v.Factor, (float64(axis.size)-axis.origin)*v.Factor
		if float64(axis.size)*v.Factor <= float64(axis.trailing-axis.leading) {
			if math.Abs((first-float64(axis.leading))-(float64(axis.trailing)-last)) > 1e-8 {
				t.Fatalf("fitted axis not centred: first=%g last=%g view=%+v", first, last, v)
			}
		} else if first > float64(axis.leading)+1e-8 || last < float64(axis.trailing)-1e-8 {
			t.Fatalf("map edge exposed: first=%g last=%g view=%+v", first, last, v)
		}
	}
}

func TestOverviewZoomRespectsEdgesAndKeepsFreeAxesAnchored(t *testing.T) {
	for _, size := range []struct {
		name       string
		mapW, mapH int32
	}{
		{"wide", 8192, 2048}, {"tall", 2048, 8192}, {"square", 4096, 4096},
	} {
		for _, eased := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/eased=%t", size.name, eased), func(t *testing.T) {
				for _, position := range []float64{0.2, 0.8} {
					c := &Camera{ViewW: 1024, ViewH: 768, MapW: size.mapW, MapH: size.mapH}
					c.SetZoomAbout(OriginX, OriginY, c.MinZoom())
					start := c.PresentationView()
					ax := int32(math.Round((float64(c.MapW)*position - start.X) * start.Factor))
					ay := int32(math.Round((float64(c.MapH)*position - start.Z) * start.Factor))
					checkStep := func(before PresentationView) {
						t.Helper()
						checkPresentedBounds(t, c)
						after := c.PresentationView()
						for _, axis := range []struct {
							before, after                   float64
							anchor, size, leading, trailing int32
						}{
							{before.X, after.X, ax, c.MapW, OriginX, c.ViewW},
							{before.Z, after.Z, ay, c.MapH, OriginY, c.ViewH - OriginY},
						} {
							first := -axis.after * after.Factor
							last := (float64(axis.size) - axis.after) * after.Factor
							if first < float64(axis.leading)-1e-8 && last > float64(axis.trailing)+1e-8 {
								world := axis.before + float64(axis.anchor)/before.Factor
								if math.Abs((world-axis.after)*after.Factor-float64(axis.anchor)) > 1e-8 {
									t.Fatal("unclamped axis lost its pointer anchor")
								}
							}
						}
					}
					for _, target := range []Zoom{ZoomUnit / 4, ZoomUnit, ZoomMax, c.MinZoom(), ZoomUnit} {
						if eased {
							var control ZoomController
							control.SetTarget(c, ax+OriginX, ay+OriginY, target)
							for before := c.PresentationView(); control.Step(c); before = c.PresentationView() {
								checkStep(before)
							}
						} else {
							before := c.PresentationView()
							c.SetZoomAbout(ax+OriginX, ay+OriginY, target)
							checkStep(before)
						}
					}
				}
			})
		}
	}
}

func TestFractionalZoomEdgesSurvivePanAndRepeatedClamp(t *testing.T) {
	for _, factor := range []float64{0.312345, 0.687123, 1, 1.333333, 2} {
		for _, direction := range []float64{-1, 1} {
			c := &Camera{ViewW: 1025, ViewH: 769, MapW: 4096, MapH: 2048}
			c.SetPresentationView(PresentationView{X: direction * 100000, Z: direction * 100000, Factor: factor})
			checkPresentedBounds(t, c)
			before := c.PresentationView()
			c.Clamp()
			if c.PresentationView() != before {
				t.Fatal("repeated clamp moved precise boundary")
			}
			c.Pan(int32(direction)*10000, int32(direction)*10000)
			checkPresentedBounds(t, c)
		}
	}
}

func TestFittedAxisReleasesContinuously(t *testing.T) {
	// A 704px viewport exactly fits the 2048px axis at 0.34375x.
	// Samples either side must meet at that same centred edge, not jump.
	c := &Camera{ViewW: 1024, ViewH: 768, MapW: 8192, MapH: 2048}
	const fit = 704.0 / 2048
	for _, direction := range []float64{-1, 1} {
		var positions [3]float64
		for i, factor := range []float64{fit - 1e-7, fit, fit + 1e-7} {
			c.SetPresentationView(PresentationView{X: 1000, Z: direction * 100000, Factor: factor})
			v := c.PresentationView()
			positions[i] = -v.Z * v.Factor
			checkPresentedBounds(t, c)
		}
		if math.Abs(positions[0]-positions[1]) > 0.001 || math.Abs(positions[2]-positions[1]) > 0.001 {
			t.Fatalf("fit transition jumped: %v", positions)
		}
	}
}
