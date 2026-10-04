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
	if v.X != float64(c.X) {
		t.Fatalf("clamped camera has extra displacement: %+v vs %d", v, c.X)
	}
	// The free axis still holds its anchor.
	if math.Abs((4200-v.Z)*v.Factor-200) > 1e-8 {
		t.Fatal("clamping X discarded the Z anchor")
	}
}

// The overview's spare border must not re-centre its shorter axis while the
// pointer zooms into the map (DESIGN_GPU_RENDERER §16.5, §16.7).
func TestOverviewZoomKeepsPointerAnchoredOnRectangularMaps(t *testing.T) {
	for _, size := range []struct {
		name       string
		mapW, mapH int32
	}{
		{"wide", 8192, 2048},
		{"tall", 2048, 8192},
		{"square", 4096, 4096},
	} {
		for _, eased := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/eased=%t", size.name, eased), func(t *testing.T) {
				for _, position := range []float64{0.2, 0.8} {
					c := &Camera{ViewW: 1024, ViewH: 768, MapW: size.mapW, MapH: size.mapH}
					c.SetZoomAbout(OriginX, OriginY, c.MinZoom())
					start := c.PresentationView()
					ax := int32(math.Round((float64(c.MapW)*position - start.X) * start.Factor))
					ay := int32(math.Round((float64(c.MapH)*position - start.Z) * start.Factor))
					wx := start.X + float64(ax)/start.Factor
					wz := start.Z + float64(ay)/start.Factor
					check := func() {
						t.Helper()
						v := c.PresentationView()
						if dx, dy := (wx-v.X)*v.Factor-float64(ax), (wz-v.Z)*v.Factor-float64(ay); math.Abs(dx) > 1e-8 || math.Abs(dy) > 1e-8 {
							t.Fatalf("point %.1f at %s drifted by (%g,%g) screen pixels", position, c.EffectiveZoom(), dx, dy)
						}
					}
					for _, target := range []Zoom{ZoomUnit / 4, ZoomUnit, ZoomMax, c.MinZoom(), ZoomUnit} {
						if eased {
							var control ZoomController
							control.SetTarget(c, ax+OriginX, ay+OriginY, target)
							for control.Step(c) {
								check()
							}
						} else {
							c.SetZoomAbout(ax+OriginX, ay+OriginY, target)
							check()
						}
					}
				}
			})
		}
	}
}
