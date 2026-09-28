package client

import (
	"testing"

	"github.com/nanolathe-gg/nanolathe/internal/camera"
	"github.com/nanolathe-gg/nanolathe/internal/sim/numeric"
)

// The lit-disc gate is measured in RECORD pixels, the space the discs are
// recorded in (DESIGN_GPU_RENDERER §13.11, §14.1). At the detail step a world
// pixel is a 2x2 block, so the map's edges sit at twice their world distance
// from the camera origin. Measured in world pixels, the gate cut every disc
// along a straight line halfway to the map's right or bottom edge whenever a 2x
// view looked there. This locks both readings of the gate — the predicate and
// the rectangle — to the camera's own projection at both record steps, at the
// near map corner and at the far one.
func TestTerrainGateFollowsTheRecordStep(t *testing.T) {
	for _, scale := range []camera.ViewScale{camera.ViewScaleNative, camera.ViewScaleDetail} {
		// Off the near corner, as litDiscClient scrolls it, and short of the far
		// corner of its 64x48 map.
		for _, origin := range [][2]int32{{-12, -8}, {40, 20}} {
			c := litDiscClient(scale)
			c.cam.X, c.cam.Z = origin[0], origin[1]
			mapW, mapH := int32(c.terrain.CellW)*16, int32(c.terrain.CellH)*16
			// The record pixel a world pixel's top-left corner lands on, through
			// the projection the recorded disc centres use.
			record := func(wx, wz int32) (int, int) {
				sx, sy := c.cam.WorldToScreen(numeric.Fixed(int64(wx)<<16), 0, numeric.Fixed(int64(wz)<<16))
				return int(sx - camera.OriginX), int(sy - camera.OriginY)
			}
			x0, y0 := record(0, 0)
			x1, y1 := record(mapW, mapH)
			inside := func(x, y int) bool { return x >= x0 && y >= y0 && x < x1 && y < y1 }
			rect := c.terrainScreenRect()
			for y := y0 - 4; y < y1+4; y++ {
				for x := x0 - 4; x < x1+4; x++ {
					want := inside(x, y)
					if got := c.terrainScreenCoverage(x, y); got != want {
						t.Fatalf("scale %v camera %v: record pixel (%d,%d) covered=%v, the projection says %v", scale, origin, x, y, got, want)
					}
					if got := rect.Contains(int32(x), int32(y)); got != want {
						t.Fatalf("scale %v camera %v: record pixel (%d,%d) in rectangle %+v=%v, the projection says %v", scale, origin, x, y, rect, got, want)
					}
				}
			}
		}
	}
}
