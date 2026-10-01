package screenkit

import (
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

var shapeDeviceResult error
var shapeDeviceCases, shapeDeviceMaxDiff int

// A hidden native loop starts on the process main goroutine; ordinary focused
// tests never need a display. Run separately from the ordinary test invocation.
func TestMain(m *testing.M) {
	if os.Getenv("NANOLATHE_SCREENKIT_DEVICE_TEST") == "1" {
		g := &shapeDeviceGame{}
		ebiten.SetWindowVisible(false)
		ebiten.SetWindowSize(48, 40)
		shapeDeviceResult = ebiten.RunGame(g)
		if shapeDeviceResult == nil {
			shapeDeviceResult = g.err
		}
	}
	os.Exit(m.Run())
}

func TestSmallShapeDeviceCoverage(t *testing.T) {
	if os.Getenv("NANOLATHE_SCREENKIT_DEVICE_TEST") != "1" {
		t.Skip("set NANOLATHE_SCREENKIT_DEVICE_TEST=1 for native shape coverage and blending")
	}
	if shapeDeviceResult != nil {
		t.Fatal(shapeDeviceResult)
	}
	if shapeDeviceCases == 0 {
		t.Fatal("native shape comparison did not run")
	}
	t.Logf("%d direct/cached comparisons; maximum 8-bit channel difference %d", shapeDeviceCases, shapeDeviceMaxDiff)
}

type shapeDeviceGame struct {
	done bool
	err  error
}

func (g *shapeDeviceGame) Layout(int, int) (int, int) { return 48, 40 }
func (g *shapeDeviceGame) Update() error {
	if g.done {
		return ebiten.Termination
	}
	return nil
}
func (g *shapeDeviceGame) Draw(*ebiten.Image) {
	if !g.done {
		g.err = checkSmallShapeDeviceCoverage()
		g.done = true
	}
}

func checkSmallShapeDeviceCoverage() error {
	const w, h = 48, 40
	prior := smallShapes
	smallShapes = newShapeStampCache(shapeCacheEntries, shapeCacheBytes, shapeCacheSide, prior.build, prior.release)
	defer func() {
		for len(smallShapes.entries) > 0 {
			smallShapes.evict()
		}
		smallShapes = prior
	}()
	direct, cached, warm := ebiten.NewImage(w, h), ebiten.NewImage(w, h), ebiten.NewImage(w, h)
	before, after := make([]byte, w*h*4), make([]byte, w*h*4)
	colours := []color.RGBA{{61, 255, 92, 255}, {255, 205, 80, 255}, {255, 255, 255, 127}, {61, 255, 92, 70}, {31, 17, 63, 17}, {0, 0, 0, 255}}
	positions := [][2]float32{{24, 20}, {24.375, 20.625}, {24.9999, 20.125}, {-1.25, 12.375}}
	clips := []image.Rectangle{image.Rect(0, 0, w, h), image.Rect(16, 12, 28, 26), image.Rect(0, 0, 20, 20), image.Rect(8, 3, 42, 36)}
	boards := [3]*image.RGBA{}
	for i := range boards {
		boards[i] = image.NewRGBA(image.Rect(0, 0, w*2*len(colours), h*len(positions)))
	}
	for kind := shapeDisc; kind <= shapeLine; kind++ {
		for ci, colour := range colours {
			for pi, pos := range positions {
				radius := []float32{5, 5.25, 0.375, 17.875}[pi]
				width := []float32{1, 1.375, 0.25, 3.25}[pi]
				s := shapeSpec{kind: kind, colour: colour, geometry: [5]float32{pos[0], pos[1], radius}}
				switch kind {
				case shapeRing:
					s.geometry[3] = width
				case shapeLine:
					s.geometry = [5]float32{pos[0] - 7.25, pos[1] - 3.5, pos[0] + 6.125, pos[1] + 4.375, width}
					if pi == 0 {
						s.geometry = [5]float32{pos[0] - 7, pos[1] - 3, pos[0] + 6, pos[1] + 4, width}
					}
				}
				// Two uses admit the exact key; the comparisons all take its warm path.
				drawSmallShape(warm, s)
				drawSmallShape(warm, s)
				if elem := smallShapes.entries[s.key()]; elem == nil || elem.Value.(*shapeStamp).image == nil {
					return fmt.Errorf("shape proof failed to populate kind %d colour %d position %d", kind, ci, pi)
				}
				for bg := 0; bg < 5; bg++ {
					pixels := shapeBackground(w, h, bg)
					for clipIndex, clip := range clips {
						direct.WritePixels(pixels)
						cached.WritePixels(pixels)
						drawShapeDirect(direct.SubImage(clip).(*ebiten.Image), s)
						drawSmallShape(cached.SubImage(clip).(*ebiten.Image), s)
						direct.ReadPixels(before)
						cached.ReadPixels(after)
						shapeDeviceCases++
						for i, a := range before {
							d := abs(int(a) - int(after[i]))
							shapeDeviceMaxDiff = max(shapeDeviceMaxDiff, d)
							if d > 1 {
								return fmt.Errorf("shape kind %d colour %v position %v background %d clip %d byte %d differs by %d: direct %d cached %d", kind, colour, pos, bg, clipIndex, i, d, a, after[i])
							}
						}
						if bg == 3 && clipIndex == 0 {
							copyShapeTile(boards[kind], before, w, h, ci*2*w, pi*h)
							copyShapeTile(boards[kind], after, w, h, ci*2*w+w, pi*h)
						}
					}
				}
			}
		}
	}
	// A lit menu lamp overlaps several stamps. The per-primitive quantization
	// bound must hold for this composition too, including its translucent gleam.
	for _, pos := range positions {
		parts := []shapeSpec{
			{kind: shapeDisc, geometry: [5]float32{pos[0], pos[1], 5}, colour: color.RGBA{39, 165, 59, 255}},
			{kind: shapeDisc, geometry: [5]float32{pos[0], pos[1], 3.6}, colour: color.RGBA{61, 255, 92, 255}},
			{kind: shapeDisc, geometry: [5]float32{pos[0] - 1.25, pos[1] - 1.25, 1.25}, colour: color.RGBA{255, 255, 255, 200}},
			{kind: shapeRing, geometry: [5]float32{pos[0], pos[1], 5, 1}, colour: color.RGBA{0, 0, 0, 255}},
		}
		for _, s := range parts {
			drawSmallShape(warm, s)
			drawSmallShape(warm, s)
		}
		for bg := 0; bg < 5; bg++ {
			for _, clip := range clips {
				pixels := shapeBackground(w, h, bg)
				direct.WritePixels(pixels)
				cached.WritePixels(pixels)
				for _, s := range parts {
					drawShapeDirect(direct.SubImage(clip).(*ebiten.Image), s)
					drawSmallShape(cached.SubImage(clip).(*ebiten.Image), s)
				}
				direct.ReadPixels(before)
				cached.ReadPixels(after)
				shapeDeviceCases++
				for i, a := range before {
					d := abs(int(a) - int(after[i]))
					shapeDeviceMaxDiff = max(shapeDeviceMaxDiff, d)
					if d > 1 {
						return fmt.Errorf("layered lamp position %v background %d clip %v byte %d differs by %d", pos, bg, clip, i, d)
					}
				}
			}
		}
	}
	if dir := os.Getenv("NANOLATHE_SCREENKIT_SHOTS"); dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
		for i, img := range boards {
			f, err := os.Create(filepath.Join(dir, []string{"disc.png", "ring.png", "line.png"}[i]))
			if err != nil {
				return err
			}
			err = png.Encode(f, img)
			closeErr := f.Close()
			if err != nil {
				return err
			}
			if closeErr != nil {
				return closeErr
			}
		}
	}
	return nil
}

func shapeBackground(w, h, kind int) []byte {
	pixels := make([]byte, w*h*4)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			c := color.RGBA{0, 0, 0, 255}
			switch kind {
			case 1:
				c = color.RGBA{43, 39, 27, 255}
			case 2:
				c = color.RGBA{255, 255, 255, 255}
			case 3, 4:
				c = color.RGBA{uint8(x * 4), uint8(y * 5), 53, 255}
				if (x/5+y/5)%2 == 0 {
					c.R, c.G = c.G, c.R
				}
				if kind == 4 {
					c.R, c.G, c.B, c.A = c.R/2, c.G/2, c.B/2, 127
				}
			}
			i := (y*w + x) * 4
			pixels[i], pixels[i+1], pixels[i+2], pixels[i+3] = c.R, c.G, c.B, c.A
		}
	}
	return pixels
}

func copyShapeTile(dst *image.RGBA, pixels []byte, w, h, x, y int) {
	for row := 0; row < h; row++ {
		copy(dst.Pix[dst.PixOffset(x, y+row):dst.PixOffset(x, y+row)+w*4], pixels[row*w*4:(row+1)*w*4])
	}
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}
