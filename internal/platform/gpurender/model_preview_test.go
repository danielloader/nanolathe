package gpurender

import (
	"fmt"
	"image/color"
	"math"
	"testing"

	"github.com/nanolathe-gg/nanolathe/formats"
	"github.com/nanolathe-gg/nanolathe/internal/drawlist"
)

func TestModelPreviewShadersCompile(t *testing.T) {
	for _, source := range []string{modelPreviewShaderSource(), modelPreviewResolveSource} {
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
	return nil
}
