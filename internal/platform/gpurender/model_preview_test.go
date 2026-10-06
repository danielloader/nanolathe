package gpurender

import (
	"fmt"
	"image/color"
	"math"
	"testing"

	"github.com/nanolathe-gg/nanolathe/formats"
	"github.com/nanolathe-gg/nanolathe/internal/drawlist"
	presentationrender "github.com/nanolathe-gg/nanolathe/internal/render"
)

func TestModelPreviewShadersCompile(t *testing.T) {
	for _, source := range []string{modelPreviewShaderSource(), modelPreviewResolveSource, modelPreviewComposeShaderSource(), modelPreviewMergeSource} {
		if _, err := compileShader(source); err != nil {
			t.Fatal(err)
		}
	}
}

func TestModelPreviewRejectsOverflowingDepthSpan(t *testing.T) {
	f := previewFixtureFace(80,
		drawlist.ModelPreviewPosition{Depth: -math.MaxFloat64},
		drawlist.ModelPreviewPosition{X: 1, Depth: math.MaxFloat64},
		drawlist.ModelPreviewPosition{Y: 1})
	if _, _, err := modelPreviewDepthBounds(&drawlist.ModelPreviewGeometry{Faces: []drawlist.ModelPreviewFace{f}}); err == nil {
		t.Fatal("finite extremes produced a non-finite normalization span")
	}
}

func previewFixtureFace(index byte, positions ...drawlist.ModelPreviewPosition) drawlist.ModelPreviewFace {
	f := drawlist.ModelPreviewFace{Face: drawlist.ModelFace{Color: index}, Positions: positions}
	for _, p := range positions {
		f.Face.Vertices = append(f.Face.Vertices, drawlist.ModelVertex{X: int32(math.Floor(p.X)), Y: int32(math.Floor(p.Y))})
	}
	return f
}

// These authored fixtures lock viewer policy, not retail depth arithmetic:
// fractional separated planes, near-coincident ordered ties, shared edges,
// negative/large depth and transparent texture holes. The real device executes
// all depth bytes and the colour resolve, so a wrong MAX byte carry or a
// key/colour disagreement cannot pass a CPU-only arithmetic test.
func checkModelPreviewDevicePixels() error {
	pal := fixturePalette()
	r, err := NewChecked(&pal, 64, 64)
	if err != nil {
		return err
	}
	defer r.ResetSources()
	dst := newRendererImage(64, 64)
	defer dst.Deallocate()
	pixels := make([]byte, 64*64*4)
	draw := func(faces ...drawlist.ModelPreviewFace) error {
		dst.Fill(color.RGBA{7, 7, 7, 255})
		if err := r.DrawModelPreview(dst, &drawlist.ModelPreviewGeometry{Faces: faces}); err != nil {
			return err
		}
		dst.ReadPixels(pixels)
		return nil
	}
	at := func(x, y int) byte { return pixels[4*(y*64+x)] }
	for step := range 32 {
		s, c := math.Sincos(float64(step) * math.Pi / 16)
		plane := func(index byte, offset float64) drawlist.ModelPreviewFace {
			var positions []drawlist.ModelPreviewPosition
			for _, xy := range [][2]float64{{-23.2, -22.7}, {23.2, -22.7}, {23.2, 22.7}, {-23.2, 22.7}} {
				x, y := 32+c*xy[0]-s*xy[1], 32+s*xy[0]+c*xy[1]
				positions = append(positions, drawlist.ModelPreviewPosition{X: x, Y: y, Depth: offset + 0.313*x + 0.427*y})
			}
			return previewFixtureFace(index, positions...)
		}
		// An offscreen corner range keeps the normalization wide enough that
		// the visible fragments exercise carries between all three bytes.
		rangeFace := previewFixtureFace(20,
			drawlist.ModelPreviewPosition{X: -50, Y: -50, Depth: -600},
			drawlist.ModelPreviewPosition{X: -40, Y: -50, Depth: 600},
			drawlist.ModelPreviewPosition{X: -40, Y: -40, Depth: 0})
		for _, nearTie := range []bool{false, true} {
			gap := 0.42254638671875
			if nearTie {
				// Opposite tiny errors on successive frames cannot change the
				// first recorded surface selected by the tie policy.
				gap = float64(2*(step&1)-1) / 65536
			}
			front, back := plane(80, -50.123), plane(200, -50.123-gap)
			if err := draw(front, back, rangeFace); err != nil {
				return err
			}
			for y := 24; y <= 40; y++ {
				for x := 24; x <= 40; x++ {
					if got := at(x, y); got != 80 {
						return fmt.Errorf("preview plane step %d tie=%v at (%d,%d): got %d, want foreground 80", step, nearTie, x, y, got)
					}
				}
			}
		}
	}
	left := previewFixtureFace(80,
		drawlist.ModelPreviewPosition{X: 4, Y: 4, Depth: -2},
		drawlist.ModelPreviewPosition{X: 30.2, Y: 4, Depth: 10},
		drawlist.ModelPreviewPosition{X: 35.8, Y: 60, Depth: -20},
		drawlist.ModelPreviewPosition{X: 4, Y: 60, Depth: -32})
	right := previewFixtureFace(160,
		left.Positions[1], drawlist.ModelPreviewPosition{X: 60, Y: 4, Depth: -10},
		drawlist.ModelPreviewPosition{X: 60, Y: 60, Depth: -40}, left.Positions[2])
	if err := draw(left, right); err != nil {
		return err
	}
	for y := 5; y < 59; y++ {
		for x := 5; x < 59; x++ {
			if got := at(x, y); got < 80 || got > 160 {
				return fmt.Errorf("preview shared edge at (%d,%d): got %d, want continuous covered faces", x, y, got)
			}
		}
	}
	front := previewFixtureFace(0,
		drawlist.ModelPreviewPosition{X: 4, Y: 4, Depth: 300},
		drawlist.ModelPreviewPosition{X: 60, Y: 4, Depth: 300},
		drawlist.ModelPreviewPosition{X: 60, Y: 60, Depth: 300},
		drawlist.ModelPreviewPosition{X: 4, Y: 60, Depth: 300})
	front.Face.Texture = &formats.GAFFrame{Width: 4, Height: 4, Pixels: []byte{1, 1, 80, 80, 1, 1, 80, 80, 1, 1, 80, 80, 1, 1, 80, 80}}
	for i, uv := range [][2]int32{{0, 0}, {3, 0}, {3, 3}, {0, 3}} {
		front.Face.Vertices[i].U, front.Face.Vertices[i].V = uv[0], uv[1]
	}
	back := previewFixtureFace(40,
		drawlist.ModelPreviewPosition{X: 4, Y: 4, Depth: -300},
		drawlist.ModelPreviewPosition{X: 60, Y: 4, Depth: -300},
		drawlist.ModelPreviewPosition{X: 60, Y: 60, Depth: -300},
		drawlist.ModelPreviewPosition{X: 4, Y: 60, Depth: -300})
	if err := draw(front, back); err != nil {
		return err
	}
	if at(10, 30) != 40 || at(50, 30) != 80 {
		return fmt.Errorf("preview texture hole/skin: got %d/%d, want 40/80", at(10, 30), at(50, 30))
	}
	return checkModelPreviewAttachmentDevicePixels()
}

// previewKeyedFace is an attachment face whose corners all carry one
// nanoframe key, at the given depths.
func previewKeyedFace(index byte, key int32, depth float64, corners ...[2]float64) drawlist.ModelPreviewFace {
	var positions []drawlist.ModelPreviewPosition
	for _, c := range corners {
		positions = append(positions, drawlist.ModelPreviewPosition{X: c[0], Y: c[1], Depth: depth})
	}
	f := previewFixtureFace(index, positions...)
	for i := range f.Face.Vertices {
		f.Face.Vertices[i].Key = key
	}
	return f
}

func previewOutlinePixel(x, y int32, color uint8, depth float64) drawlist.ModelPreviewOutlinePixel {
	return drawlist.ModelPreviewOutlinePixel{X: x, Y: y, Color: color, Depth: [4]float64{depth, depth, depth, depth}}
}

// These authored fixtures lock the composed viewer record on the device:
// per-sample geometric occlusion between parent and attachment with the
// attachment winning ties, the battle's reveal verdicts on the attachment's
// own top surface (an erased texel shows the parent, never the attachment's
// far side), wrapped keys through both key lanes, outline pixels tested by
// depth, and the remaining-fraction boundaries [03 R-P0-19-N]
// [03 R-COMP-01 §3]. The no-attachment path is checked above.
func checkModelPreviewAttachmentDevicePixels() error {
	pal := fixturePalette()
	r, err := NewChecked(&pal, 64, 64)
	if err != nil {
		return err
	}
	defer r.ResetSources()
	dst := newRendererImage(64, 64)
	defer dst.Deallocate()
	pixels := make([]byte, 64*64*4)
	draw := func(parent []drawlist.ModelPreviewFace, a *drawlist.ModelPreviewAttachment) error {
		dst.Fill(color.RGBA{7, 7, 7, 255})
		if err := r.DrawModelPreview(dst, &drawlist.ModelPreviewGeometry{Faces: parent, Attachment: a}); err != nil {
			return err
		}
		dst.ReadPixels(pixels)
		return nil
	}
	at := func(x, y int) byte { return pixels[4*(y*64+x)] }
	expect := func(scene string, want map[[2]int]byte) error {
		for p, v := range want {
			if got := at(p[0], p[1]); got != v {
				return fmt.Errorf("preview attachment %s at %v: got %d, want %d", scene, p, got, v)
			}
		}
		return nil
	}
	square := [][2]float64{{4, 4}, {60, 4}, {60, 60}, {4, 60}}
	sloped := func(index byte, slope, offset float64) drawlist.ModelPreviewFace {
		var positions []drawlist.ModelPreviewPosition
		for _, c := range square {
			positions = append(positions, drawlist.ModelPreviewPosition{X: c[0], Y: c[1], Depth: slope*c[0] + offset})
		}
		return previewFixtureFace(index, positions...)
	}
	// Crossing planes: each model occludes the other on its own side.
	parent := sloped(80, .5, -16)
	if err := draw([]drawlist.ModelPreviewFace{parent}, &drawlist.ModelPreviewAttachment{Faces: []drawlist.ModelPreviewFace{sloped(200, -.5, 16)}}); err != nil {
		return err
	}
	if err := expect("crossing", map[[2]int]byte{{16, 32}: 200, {24, 10}: 200, {40, 32}: 80, {56, 50}: 80}); err != nil {
		return err
	}
	// A coincident attachment wins, as a carried child wins key ties.
	if err := draw([]drawlist.ModelPreviewFace{sloped(80, 0, 5)}, &drawlist.ModelPreviewAttachment{Faces: []drawlist.ModelPreviewFace{sloped(200, 0, 5)}}); err != nil {
		return err
	}
	if err := expect("tie", map[[2]int]byte{{16, 32}: 200, {48, 48}: 200}); err != nil {
		return err
	}
	// Reveal verdicts on the attachment's own top surface.
	reveal := &drawlist.ModelReveal{Line: 100, Floor: 96, Below: -2, Band: 60, Above: -1}
	back := previewKeyedFace(170, 256+97, 10, square...)                                           // quad lanes; wraps into the band
	front := previewKeyedFace(150, 50, 20, [2]float64{4, 4}, [2]float64{30, 4}, [2]float64{4, 60}) // vertex lane; below: erase
	kept := previewKeyedFace(210, 150, 30, [2]float64{40, 4}, [2]float64{60, 4}, [2]float64{60, 20}, [2]float64{40, 20})
	high := previewFixtureFace(90,
		drawlist.ModelPreviewPosition{X: 4, Y: 44, Depth: 50}, drawlist.ModelPreviewPosition{X: 16, Y: 44, Depth: 50},
		drawlist.ModelPreviewPosition{X: 16, Y: 60, Depth: 50}, drawlist.ModelPreviewPosition{X: 4, Y: 60, Depth: 50})
	base := sloped(80, 0, 0)
	if err := draw([]drawlist.ModelPreviewFace{high, base}, &drawlist.ModelPreviewAttachment{
		Faces:  []drawlist.ModelPreviewFace{front, kept, back},
		Reveal: reveal,
		Outline: []drawlist.ModelPreviewOutlinePixel{
			previewOutlinePixel(20, 50, 165, 40), // in front of everything
			previewOutlinePixel(50, 10, 165, 25), // behind the kept face
			previewOutlinePixel(8, 52, 165, 40),  // behind the parent's high face
		},
	}); err != nil {
		return err
	}
	if err := expect("reveal", map[[2]int]byte{
		{8, 10}: 80, {12, 20}: 80, // erased top surface: the parent, not the band behind
		{50, 50}: 60, {36, 30}: 60, // the band verdict, unshaded
		{50, 14}: 210,                             // keep
		{20, 50}: 165, {50, 10}: 210, {8, 52}: 90, // outline depth tests
	}); err != nil {
		return err
	}
	// Remaining 1 erases this low body and leaves the outline; a vanishing
	// fraction keeps it whole.
	band, outline := presentationrender.NanoframePulse(3, 40)
	for _, tt := range []struct {
		remaining float32
		want      byte
	}{{1, 80}, {1e-6, 170}} {
		v := presentationrender.BuildNanoframeReveal(tt.remaining, band, outline)
		if err := draw([]drawlist.ModelPreviewFace{base}, &drawlist.ModelPreviewAttachment{
			Faces:   []drawlist.ModelPreviewFace{previewKeyedFace(170, 55, 10, square...)},
			Reveal:  &drawlist.ModelReveal{Line: v.Line, Floor: v.Floor, Below: v.Below, Band: v.Band, Above: v.Above},
			Outline: []drawlist.ModelPreviewOutlinePixel{previewOutlinePixel(30, 30, outline, 12)},
		}); err != nil {
			return err
		}
		if err := expect(fmt.Sprint("remaining ", tt.remaining), map[[2]int]byte{{12, 12}: tt.want, {50, 40}: tt.want, {30, 30}: outline}); err != nil {
			return err
		}
	}
	return nil
}
