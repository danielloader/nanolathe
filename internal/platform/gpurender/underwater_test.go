package gpurender

import (
	"bytes"
	"fmt"
	"math"
	"testing"

	"github.com/nanolathe-gg/nanolathe/internal/camera"
	"github.com/nanolathe-gg/nanolathe/internal/drawlist"
)

// underwaterMaxOffset is the analytic bound on waterOffset; the commit pads its
// quad by it, so it has to be the real maximum of the field's arithmetic.
func TestUnderwaterMaxOffsetBoundsTheField(t *testing.T) {
	const surfaceEnergy = 0.5
	got := 0.5 * (3.2 + surfaceEnergy*2.4) * (0.7 + 0.5)
	if math.Abs(got-underwaterMaxOffset) > 1e-6 {
		t.Fatalf("waterOffset bound %v, underwaterMaxOffset %v", got, underwaterMaxOffset)
	}
}

// These relationships lock the Enhanced underwater refraction (§26.5), not a
// retail contract: the blue-tinted part of a hull moves with the water, the
// part above the waterline is byte-for-byte the ordinary commit, an erased
// hull is untouched, displacement stays inside its bound, and the commit adds
// no device draw.
func checkUnderwaterDevicePixels() error {
	pal := fixturePalette()
	const w, h, bg = 160, 120, 200
	r, err := NewChecked(&pal, w, h)
	if err != nil {
		return err
	}
	terrain := waterFixtureTerrain()
	// Key rises 5 → 40 left to right; at and below key 20 the hull is under
	// the waterline, so its left part is submerged and its right part is not.
	// It sits well inside the fixture's water, away from the coast at x 128.
	hull := directSubject(24, 40, 44, 18, directFace(0, 0, 40, 16, 100, 5, 40))
	type frame struct {
		pix   []byte
		stats ModelStats
	}
	render := func(water bool, tick uint32, mode drawlist.ModelWaterline) frame {
		hull.Waterline, hull.WaterlineKey = mode, 20
		var l drawlist.List
		l.RecordClear()
		l.RecordTerrain(drawlist.Terrain{Terrain: terrain, DstW: w, DstH: h, Scale: camera.ViewScaleNative, Water: drawlist.WaterSurface{Enabled: water, Tick: tick}})
		// Stable background isolates the hull from the animated water itself.
		l.RecordFill(drawlist.Fill{Rect: drawlist.Rect{W: w, H: h}, Index: bg})
		l.RecordModel(drawlist.Model{Geometry: hull})
		l.RecordExpand()
		img := r.Execute(&l, w, h)
		p := make([]byte, w*h*4)
		img.ReadPixels(p)
		return frame{p, r.ModelStats()}
	}
	off := render(false, 30, drawlist.ModelWaterlineBlue)
	a := render(true, 30, drawlist.ModelWaterlineBlue)
	b := render(true, 90, drawlist.ModelWaterlineBlue)
	if a.stats.UnderwaterCommits != 1 || off.stats.UnderwaterCommits != 0 {
		return fmt.Errorf("underwater commits on/off = %d/%d, want 1/0", a.stats.UnderwaterCommits, off.stats.UnderwaterCommits)
	}
	// The same water frame with the hull erased takes the ordinary commit; the
	// underwater op joins the open model run, so the draw count is the same.
	erased := render(true, 30, drawlist.ModelWaterlineErase)
	if a.stats.DeviceDraws != erased.stats.DeviceDraws {
		return fmt.Errorf("underwater commit changed device draws %d → %d", erased.stats.DeviceDraws, a.stats.DeviceDraws)
	}
	// The hull's own pixels without the treatment, and its horizontal extent.
	x0, x1, y0, y1 := w, -1, h, -1
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			if off.pix[(y*w+x)*4] != bg {
				x0, x1, y0, y1 = min(x0, x), max(x1, x), min(y0, y), max(y1, y)
			}
		}
	}
	if x1 < 0 {
		return fmt.Errorf("underwater fixture drew no hull")
	}
	if bytes.Equal(a.pix, off.pix) {
		return fmt.Errorf("underwater refraction left the submerged hull unchanged")
	}
	if bytes.Equal(a.pix, b.pix) {
		return fmt.Errorf("underwater refraction did not move with the water phase")
	}
	pad := int(math.Ceil(underwaterMaxOffset*modelRefraction)) + 1
	right := x0 + (x1-x0)*3/4
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			i := (y*w + x) * 4
			differs := !bytes.Equal(a.pix[i:i+4], off.pix[i:i+4]) || !bytes.Equal(b.pix[i:i+4], off.pix[i:i+4])
			if !differs {
				continue
			}
			if x < x0-pad || x > x1+pad || y < y0-pad || y > y1+pad {
				return fmt.Errorf("underwater refraction changed (%d,%d), outside the hull %d..%d×%d..%d padded by %d", x, y, x0, x1, y0, y1, pad)
			}
			// The last quarter of the hull is above the waterline: the
			// ordinary commit's pixels exactly.
			if x >= right+pad {
				return fmt.Errorf("underwater refraction changed above-water pixel (%d,%d)", x, y)
			}
		}
	}
	// An erased hull (no sonar contact) has nothing below the line to refract.
	if eo := render(false, 30, drawlist.ModelWaterlineErase); !bytes.Equal(eo.pix, erased.pix) || erased.stats.UnderwaterCommits != 0 {
		return fmt.Errorf("underwater refraction touched an erased hull")
	}
	// The refraction is the water motion switch (§30): off, the hull takes
	// the ordinary commit on a live water frame.
	r.setWaterMotion(false)
	still := render(true, 30, drawlist.ModelWaterlineBlue)
	r.setWaterMotion(true)
	if still.stats.UnderwaterCommits != 0 || !bytes.Equal(still.pix, off.pix) {
		return fmt.Errorf("water motion off still refracted the hull (%d commits)", still.stats.UnderwaterCommits)
	}
	// The water's shade over the refracted hull is the surface switch's: off,
	// the hull still moves with the water but takes no ripple shade, so the
	// frame differs from both the shaded refraction and the ordinary commit,
	// and still moves with the phase.
	r.setWaterSurface(false)
	plain, plainLater := render(true, 30, drawlist.ModelWaterlineBlue), render(true, 90, drawlist.ModelWaterlineBlue)
	r.setWaterSurface(true)
	if plain.stats.UnderwaterCommits != 1 {
		return fmt.Errorf("water surface off stopped the refraction (%d commits)", plain.stats.UnderwaterCommits)
	}
	if bytes.Equal(plain.pix, a.pix) || bytes.Equal(plain.pix, off.pix) || bytes.Equal(plain.pix, plainLater.pix) {
		return fmt.Errorf("water surface off: the hull kept the shade, lost the refraction, or stopped moving")
	}
	// Submerged alpha is a tag, not partial opacity. With the fixture's
	// identity Blue table and refraction off, filtered covered pixels and
	// their silhouette must match the ordinary hull exactly (§16.3, §26.5).
	filtered := directSubject(24, 40, 44, 18, directFace(0, 0, 40, 16, 100, 10, 10))
	filtered.Supersample = directSubject(0, 0, 88, 36, directFace(0, 0, 80, 32, 100, 10, 10))
	filtered.Supersample.Scale = 2
	r.setWaterMotion(false)
	fractional := func(mode drawlist.ModelWaterline) []byte {
		filtered.Waterline, filtered.WaterlineKey = mode, 20
		filtered.Supersample.Waterline, filtered.Supersample.WaterlineKey = mode, 20
		var l drawlist.List
		l.RecordClear()
		l.RecordFill(drawlist.Fill{Rect: drawlist.Rect{W: w, H: h}, Index: bg})
		l.RecordWorld(drawlist.WorldSpace{Begin: true, Zoom: camera.ZoomUnit * 3 / 2, Step: camera.ViewScaleDetail,
			RecordW: w * 4 / 3, RecordH: h * 4 / 3, Viewport: drawlist.Rect{W: w, H: h}})
		l.RecordModel(drawlist.Model{Geometry: filtered})
		l.RecordWorld(drawlist.WorldSpace{})
		l.RecordExpand()
		img := r.Execute(&l, w, h)
		p := make([]byte, w*h*4)
		img.ReadPixels(p)
		return p
	}
	ordinary := fractional(drawlist.ModelWaterlineNone)
	submerged := fractional(drawlist.ModelWaterlineBlue)
	if !bytes.Equal(ordinary, submerged) {
		return fmt.Errorf("fractional filtered hull treated submerged alpha metadata as partial coverage")
	}
	covered := 0
	for i := 0; i < len(ordinary); i += 4 {
		if ordinary[i] == 100 {
			covered++
		}
	}
	if covered == 0 {
		return fmt.Errorf("fractional submerged fixture drew no fully covered hull pixels")
	}
	return nil
}
