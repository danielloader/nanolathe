package gpurender

import (
	"fmt"

	"github.com/nanolathe-gg/nanolathe/internal/drawlist"
)

// doubledFixture is a recorder-built doubled lane for a one-pixel subject: the
// 2×2 raster of GPU design §17.2 holding the given faces in doubled local
// coordinates.
func doubledFixture(faces ...drawlist.ModelFace) *drawlist.ModelGeometry {
	g := fixtureGeometry(0, true, faces...)
	g.Scale, g.Width, g.Height = 2, 2, 2
	return g
}

// onePixelSubject is a native one-pixel packet at a framebuffer anchor whose
// doubled lane carries the faces given.
func onePixelSubject(ax, ay int32, color uint8, doubled ...drawlist.ModelFace) *drawlist.ModelGeometry {
	g := fixtureGeometry(ax, true, fixtureFace(0, 0, 1, 1, 50, color))
	g.AnchorY, g.Width, g.Height = ay, 1, 1
	g.Supersample = doubledFixture(doubled...)
	return g
}

// checkModelSingleSampleDevicePixels locks the Supersample switch's executor
// half (GPU design §17.5), a Nanolathe presentation choice with no retail
// counterpart. On, a pixel resolves its four doubled texels by coverage, so an
// edge pixel is fractional; off, the block's top-left texel — the native
// raster's own sample — stands for the pixel, so every model pixel is whole:
//
//   - A covers only its block's top-left texel: a quarter over the field on,
//     the face's full colour off.
//   - B covers only its bottom-right texel: a quarter on, nothing off.
//   - C splits its block between two colours: their mean on, the top-left
//     colour off.
//   - D's projected shadow covers one texel: an eighth of the shadow index on
//     (the ALP half at a quarter's coverage), the whole half-blend off.
//   - E's silhouette shadow reads its body's one covered texel the same way,
//     and E's body resolves as A's does.
func checkModelSingleSampleDevicePixels() error {
	const w, h, field = 40, 8, 100
	pal := fixturePalette()
	r, err := NewChecked(&pal, w, h)
	if err != nil {
		return err
	}
	a := onePixelSubject(2, 2, 200, fixtureFace(0, 0, 1, 1, 50, 200))
	b := onePixelSubject(6, 2, 200, fixtureFace(1, 1, 1, 1, 50, 200))
	c := onePixelSubject(10, 2, 200,
		fixtureFace(0, 0, 1, 1, 60, 200), fixtureFace(1, 0, 1, 1, 60, 40), fixtureFace(0, 1, 2, 1, 60, 40))
	d := onePixelSubject(14, 2, 150, fixtureFace(0, 0, 2, 2, 50, 150))
	d.Shadow = fixtureGeometry(20, true, fixtureFace(0, 0, 1, 1, 25, fixtureShadowIndex))
	d.Shadow.AnchorY, d.Shadow.Width, d.Shadow.Height = 2, 1, 1
	d.Shadow.Supersample = doubledFixture(fixtureFace(0, 0, 1, 1, 25, fixtureShadowIndex))
	e := onePixelSubject(26, 2, 200, fixtureFace(0, 0, 1, 1, 50, 200))
	e.Shadow = &drawlist.ModelGeometry{Eligible: true, KeyPlane: true, Silhouette: true, Width: 1, Height: 1, AnchorX: 32, AnchorY: 2, Scale: 1}
	var list drawlist.List
	list.RecordClear()
	list.RecordFill(drawlist.Fill{Rect: drawlist.Rect{X: 0, Y: 0, W: w, H: h}, Index: field, Style: drawlist.FillSolid})
	for _, g := range []*drawlist.ModelGeometry{a, b, c, d, e} {
		list.RecordModel(drawlist.Model{Geometry: g})
	}
	list.RecordExpand()
	read := func(single bool) ([]byte, error) {
		r.setSupersample(!single)
		img := r.Execute(&list, w, h)
		if img == nil {
			return nil, fmt.Errorf("single-sample fixture returned no image")
		}
		p := make([]byte, w*h*4)
		img.ReadPixels(p)
		return p, nil
	}
	on, err := read(false)
	if err != nil {
		return err
	}
	off, err := read(true)
	r.setSupersample(true)
	if err != nil {
		return err
	}
	about := func(name string, pixels []byte, x int, want float64) error {
		got := float64(pixels[(2*w+x)*4])
		if got < want-2 || got > want+2 {
			return fmt.Errorf("%s at x=%d reads %v, want about %v", name, x, got, want)
		}
		return nil
	}
	for _, tc := range []struct {
		name    string
		x       int
		on, off float64
	}{
		{"A top-left texel", 2, field*0.75 + 200*0.25, 200},
		{"B bottom-right texel", 6, field*0.75 + 200*0.25, field},
		{"C split block", 10, (200 + 3*40) / 4.0, 200},
		{"D projected shadow", 20, float64(fixtureShadowIndex)/8 + field*7/8.0, (float64(fixtureShadowIndex) + field) / 2},
		{"E body", 26, field*0.75 + 200*0.25, 200},
		{"E silhouette shadow", 32, field * 7 / 8.0, field / 2.0},
	} {
		if err := about(tc.name+" supersampled", on, tc.x, tc.on); err != nil {
			return err
		}
		if err := about(tc.name+" single-sampled", off, tc.x, tc.off); err != nil {
			return err
		}
	}
	// Off, the bodies are exact palette colours: nothing blends at an edge.
	for _, x := range []int{2, 6, 10, 26} {
		want := byte(200)
		if x == 6 {
			want = field
		}
		if err := checkExactIndex(fmt.Sprintf("single-sampled body at (%d,2)", x), off, (2*w+x)*4, &pal, want); err != nil {
			return err
		}
	}
	return nil
}
