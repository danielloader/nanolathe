package screenkit

import (
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"
)

var (
	shapeDeviceResult error
	shapeDeviceReport []string
)

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

// TestShapeDeviceCoverage compares the analytic shapes with the vector
// painter's antialiasing (and Poly with the triangle supersampler it
// replaced) on fractional, clipped, translucent and overlapping controls.
// Pixels a clear margin inside or outside a shape must blend identically;
// edge pixels differ by antialiasing method and are reported, with each
// shape's total coverage. NANOLATHE_SCREENKIT_SHOTS=<dir> writes a magnified
// contact sheet: reference, analytic, and their difference ×4.
func TestShapeDeviceCoverage(t *testing.T) {
	if os.Getenv("NANOLATHE_SCREENKIT_DEVICE_TEST") != "1" {
		t.Skip("set NANOLATHE_SCREENKIT_DEVICE_TEST=1 for native shape coverage and blending")
	}
	for _, line := range shapeDeviceReport {
		t.Log(line)
	}
	if shapeDeviceResult != nil {
		t.Fatal(shapeDeviceResult)
	}
	if len(shapeDeviceReport) == 0 {
		t.Fatal("native shape comparison did not run")
	}
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
		shapeDeviceReport, g.err = checkShapeDevice()
		g.done = true
	}
}

const (
	deviceW, deviceH = 48, 40
	// deviceMargin keeps zone pixels clear of either method's edge: the
	// analytic coverage reaches half a pixel diagonal past an edge and the
	// vector samples less.
	deviceMargin = 0.75
)

type deviceShape struct {
	kind int        // shapeDisc, shapeRing, shapeLine, shapePoly
	g    [5]float64 // disc: cx cy r; ring: cx cy r w; line: x0 y0 x1 y1 w
	pts  []float64  // polygon corners
	c    color.RGBA
	// edges are the polygon's outward edges, taken once from polyFan.
	edges []polyEdge
}

func (s deviceShape) name() string {
	switch s.kind {
	case shapeDisc:
		return fmt.Sprintf("disc (%g,%g) r=%g", s.g[0], s.g[1], s.g[2])
	case shapeRing:
		return fmt.Sprintf("ring (%g,%g) r=%g w=%g", s.g[0], s.g[1], s.g[2], s.g[3])
	case shapeLine:
		return fmt.Sprintf("line (%g,%g)-(%g,%g) w=%g", s.g[0], s.g[1], s.g[2], s.g[3], s.g[4])
	}
	return fmt.Sprintf("poly %d corners %v", len(s.pts)/2, s.pts)
}

func (s deviceShape) draw(dst *ebiten.Image) {
	switch s.kind {
	case shapeDisc:
		Disc(dst, s.g[0], s.g[1], s.g[2], s.c)
	case shapeRing:
		Ring(dst, s.g[0], s.g[1], s.g[2], s.g[3], s.c)
	case shapeLine:
		Line(dst, s.g[0], s.g[1], s.g[2], s.g[3], s.g[4], s.c)
	case shapePoly:
		Poly(dst, s.pts, s.c)
	}
}

// drawReference is the painting this package used before: the vector
// package's antialiased paths, and for Poly the supersampled triangle fan.
func (s deviceShape) drawReference(dst *ebiten.Image) {
	f := func(i int) float32 { return float32(s.g[i]) }
	switch s.kind {
	case shapeDisc:
		vector.FillCircle(dst, f(0), f(1), f(2), s.c, true)
	case shapeRing:
		vector.StrokeCircle(dst, f(0), f(1), f(2), f(3), s.c, true)
	case shapeLine:
		vector.StrokeLine(dst, f(0), f(1), f(2), f(3), f(4), s.c, true)
	case shapePoly:
		n := len(s.pts) / 2
		r, g, b, a := premul(s.c)
		v := make([]ebiten.Vertex, n)
		for i := range v {
			v[i] = ebiten.Vertex{DstX: float32(s.pts[2*i]), DstY: float32(s.pts[2*i+1]), SrcX: 1, SrcY: 1, ColorR: r, ColorG: g, ColorB: b, ColorA: a}
		}
		var idx []uint16
		for i := 1; i < n-1; i++ {
			idx = append(idx, 0, uint16(i), uint16(i+1))
		}
		//lint:ignore SA1019 The replaced supersampling path is the reference.
		op := &ebiten.DrawTrianglesOptions{AntiAlias: true, ColorScaleMode: ebiten.ColorScaleModePremultipliedAlpha}
		dst.DrawTriangles(v, idx, white, op)
	}
}

// Zones of a pixel centre: clearly inside, clearly outside, or on an edge.
const (
	zoneOutside = iota
	zoneInside
	zoneEdge
)

func (s deviceShape) zone(x, y float64) int {
	m := deviceMargin
	switch s.kind {
	case shapeDisc, shapeRing:
		rho := math.Hypot(x-s.g[0], y-s.g[1])
		outer, inner := s.g[2], -1.0
		if s.kind == shapeRing {
			outer, inner = s.g[2]+s.g[3]/2, s.g[2]-s.g[3]/2
		}
		switch {
		case rho >= outer+m || (inner > 0 && rho <= inner-m):
			return zoneOutside
		case rho <= outer-m && (inner <= 0 || rho >= inner+m):
			return zoneInside
		}
		return zoneEdge
	case shapeLine:
		dx, dy := s.g[2]-s.g[0], s.g[3]-s.g[1]
		l := math.Hypot(dx, dy)
		u := ((x-s.g[0])*dx+(y-s.g[1])*dy)/l - l/2
		v := (-(x-s.g[0])*dy + (y-s.g[1])*dx) / l
		a, b := l/2, s.g[4]/2
		switch {
		case math.Abs(u) >= a+m || math.Abs(v) >= b+m:
			return zoneOutside
		case math.Abs(u) <= a-m && math.Abs(v) <= b-m:
			return zoneInside
		}
		return zoneEdge
	}
	worst := math.Inf(-1)
	for _, e := range s.edges {
		worst = max(worst, e.distance(polyPoint{x, y}))
	}
	switch {
	case worst >= m:
		return zoneOutside
	case worst <= -m:
		return zoneInside
	}
	return zoneEdge
}

// inside reports whether the point lies in the shape.
func (s deviceShape) inside(x, y float64) bool {
	switch s.kind {
	case shapeDisc:
		return math.Hypot(x-s.g[0], y-s.g[1]) <= s.g[2]
	case shapeRing:
		rho := math.Hypot(x-s.g[0], y-s.g[1])
		return math.Abs(rho-s.g[2]) <= s.g[3]/2
	case shapeLine:
		dx, dy := s.g[2]-s.g[0], s.g[3]-s.g[1]
		l := math.Hypot(dx, dy)
		u := ((x-s.g[0])*dx+(y-s.g[1])*dy)/l - l/2
		v := (-(x-s.g[0])*dy + (y-s.g[1])*dx) / l
		return math.Abs(u) <= l/2 && math.Abs(v) <= s.g[4]/2
	}
	for _, e := range s.edges {
		if e.distance(polyPoint{x, y}) > 0 {
			return false
		}
	}
	return true
}

// area is the shape's area on the canvas, sampled 64×64 per pixel.
func (s deviceShape) area() float64 {
	const n = 64
	hits := 0
	for py := 0; py < deviceH; py++ {
		for px := 0; px < deviceW; px++ {
			if s.zone(float64(px)+0.5, float64(py)+0.5) != zoneEdge {
				if s.zone(float64(px)+0.5, float64(py)+0.5) == zoneInside {
					hits += n * n
				}
				continue
			}
			for j := 0; j < n; j++ {
				for i := 0; i < n; i++ {
					if s.inside(float64(px)+(float64(i)+0.5)/n, float64(py)+(float64(j)+0.5)/n) {
						hits++
					}
				}
			}
		}
	}
	return float64(hits) / (n * n)
}

// smallest is the shape's narrowest dimension, under which no pixel grid
// can hold an area estimate to its outline.
func (s deviceShape) smallest() float64 {
	switch s.kind {
	case shapeDisc:
		return 2 * s.g[2]
	case shapeRing:
		return s.g[3]
	case shapeLine:
		return s.g[4]
	}
	return math.Inf(1)
}

func deviceCases() []deviceShape {
	positions := [][2]float64{{24, 20}, {24.375, 20.625}, {24.9999, 20.125}, {-1.25, 12.375}}
	radii := []float64{5, 5.25, 0.375, 17.875}
	widths := []float64{1, 1.375, 0.25, 3.25}
	var cases []deviceShape
	for _, kind := range []int{shapeDisc, shapeRing, shapeLine} {
		for pi, p := range positions {
			s := deviceShape{kind: kind, g: [5]float64{p[0], p[1], radii[pi]}}
			switch kind {
			case shapeRing:
				s.g[3] = widths[pi]
			case shapeLine:
				s.g = [5]float64{p[0] - 7.25, p[1] - 3.5, p[0] + 6.125, p[1] + 4.375, widths[pi]}
				if pi == 0 {
					s.g = [5]float64{p[0] - 7, p[1] - 3, p[0] + 6, p[1] + 4, widths[pi]}
				}
			}
			cases = append(cases, s)
		}
	}
	// Controls as drawn at u = 0.82: the X button's diagonal, a chevron
	// stroke, an axis-aligned rule, a staging dot and a lamp gleam.
	cases = append(cases,
		deviceShape{kind: shapeLine, g: [5]float64{17.3, 13.1, 30.5, 26.3, 1.64}},
		deviceShape{kind: shapeLine, g: [5]float64{25.7, 20.2, 22.4, 17.9, 1}},
		deviceShape{kind: shapeLine, g: [5]float64{6.5, 30.25, 41.5, 30.25, 1.5}},
		deviceShape{kind: shapeDisc, g: [5]float64{12.6, 9.3, 1.64}},
		deviceShape{kind: shapeDisc, g: [5]float64{30.1, 28.7, 1.03}},
		// The lamp's rim alone where the layered lamp leaves the canvas.
		deviceShape{kind: shapeRing, g: [5]float64{-1.25, 12.375, 5, 1}},
	)
	// The arrow buttons' triangle at three sizes, then a rotated square and
	// a hexagon for the general fan.
	for _, k := range []float64{8, 14, 26} {
		cx, cy := 24.3, 20.4
		cases = append(cases, deviceShape{kind: shapePoly, pts: []float64{cx, cy + 0.22*k*1.6, cx - 0.24*k*1.6, cy - 0.16*k*1.6, cx + 0.24*k*1.6, cy - 0.16*k*1.6}})
	}
	cases = append(cases, deviceShape{kind: shapePoly, pts: []float64{24.2, 6.1, 37.9, 19.8, 24.2, 33.5, 10.5, 19.8}})
	hexagon := make([]float64, 0, 12)
	for i := 0; i < 6; i++ {
		a := float64(i)*math.Pi/3 + 0.2
		hexagon = append(hexagon, 23.7+15*math.Cos(a), 20.2+15*math.Sin(a))
	}
	cases = append(cases, deviceShape{kind: shapePoly, pts: hexagon})
	for i := range cases {
		if cases[i].kind == shapePoly {
			polyFan(nil, cases[i].pts, shapeColour{1, 1, 1, 1})
			cases[i].edges = append([]polyEdge(nil), polyEdges...)
		}
	}
	return cases
}

type deviceBoard struct {
	img  *image.RGBA
	row  int
	cols int
}

const deviceZoom = 4

func newDeviceBoard(rows, cols int) *deviceBoard {
	return &deviceBoard{img: image.NewRGBA(image.Rect(0, 0, cols*(deviceW*deviceZoom+4), rows*(deviceH*deviceZoom+4))), cols: cols}
}

// put draws one magnified tile; difference tiles show |a-b|×4 over black.
func (b *deviceBoard) put(col int, pixels, other []byte) {
	ox, oy := col*(deviceW*deviceZoom+4), b.row*(deviceH*deviceZoom+4)
	for y := 0; y < deviceH; y++ {
		for x := 0; x < deviceW; x++ {
			i := (y*deviceW + x) * 4
			c := color.RGBA{pixels[i], pixels[i+1], pixels[i+2], 255}
			if other != nil {
				d := func(k int) uint8 { return uint8(min(255, 4*abs(int(pixels[i+k])-int(other[i+k])))) }
				c = color.RGBA{d(0), d(1), d(2), 255}
			}
			for yy := 0; yy < deviceZoom; yy++ {
				for xx := 0; xx < deviceZoom; xx++ {
					b.img.SetRGBA(ox+x*deviceZoom+xx, oy+y*deviceZoom+yy, c)
				}
			}
		}
	}
}

func checkShapeDevice() ([]string, error) {
	const w, h = deviceW, deviceH
	ref, got := ebiten.NewImage(w, h), ebiten.NewImage(w, h)
	before, after := make([]byte, w*h*4), make([]byte, w*h*4)
	colours := []color.RGBA{{61, 255, 92, 255}, {255, 205, 80, 255}, {255, 255, 255, 127}, {61, 255, 92, 70}, {31, 17, 63, 17}, {0, 0, 0, 255}}
	clips := []image.Rectangle{image.Rect(0, 0, w, h), image.Rect(16, 12, 28, 26), image.Rect(0, 0, 20, 20), image.Rect(8, 3, 42, 36)}
	cases := deviceCases()
	board := newDeviceBoard(len(cases)+4, 6)
	var report []string
	comparisons, zoneMax, edgeMax := 0, 0, 0
	// compare checks one drawing and returns its largest edge difference.
	compare := func(what string, zone func(x, y float64) int, clip image.Rectangle) (int, error) {
		comparisons++
		edge := 0
		for y := 0; y < h; y++ {
			for x := 0; x < w; x++ {
				z := zoneOutside
				if (image.Point{x, y}).In(clip) {
					z = zone(float64(x)+0.5, float64(y)+0.5)
				}
				for k := 0; k < 4; k++ {
					i := (y*w+x)*4 + k
					d := abs(int(before[i]) - int(after[i]))
					if z == zoneEdge {
						edge = max(edge, d)
						continue
					}
					zoneMax = max(zoneMax, d)
					if d > 1 {
						return edge, fmt.Errorf("%s: pixel (%d,%d) channel %d outside the edges differs by %d: reference %d, analytic %d", what, x, y, k, d, before[i], after[i])
					}
				}
			}
		}
		edgeMax = max(edgeMax, edge)
		return edge, nil
	}
	for _, s := range cases {
		shapeEdge := 0
		for ci, colour := range colours {
			s.c = colour
			for bg := 0; bg < 5; bg++ {
				pixels := shapeBackground(w, h, bg)
				for clipIndex, clip := range clips {
					ref.WritePixels(pixels)
					got.WritePixels(pixels)
					s.drawReference(ref.SubImage(clip).(*ebiten.Image))
					s.draw(got.SubImage(clip).(*ebiten.Image))
					ref.ReadPixels(before)
					got.ReadPixels(after)
					edge, err := compare(fmt.Sprintf("%s colour %v background %d clip %d", s.name(), colour, bg, clipIndex), s.zone, clip)
					if err != nil {
						return report, err
					}
					shapeEdge = max(shapeEdge, edge)
					if bg == 3 && clipIndex == 0 && (ci == 0 || ci == 2) {
						col := 3 * ci / 2
						board.put(col, before, nil)
						board.put(col+1, after, nil)
						board.put(col+2, before, after)
					}
				}
			}
		}
		report = append(report, fmt.Sprintf("%s: maximum edge difference %d", s.name(), shapeEdge))
		board.row++
	}
	// Total coverage: opaque white on clear, unclipped, against the area the
	// shape has on the canvas.
	report = append(report, "shape | reference coverage | analytic coverage | analytic/reference | area | reference/area | analytic/area")
	transparent := make([]byte, w*h*4)
	coverage := func(pixels []byte) float64 {
		sum := 0
		for i := 3; i < len(pixels); i += 4 {
			sum += int(pixels[i])
		}
		return float64(sum) / 255
	}
	for _, s := range cases {
		s.c = color.RGBA{255, 255, 255, 255}
		ref.WritePixels(transparent)
		got.WritePixels(transparent)
		s.drawReference(ref)
		s.draw(got)
		ref.ReadPixels(before)
		got.ReadPixels(after)
		rc, gc, a := coverage(before), coverage(after), s.area()
		report = append(report, fmt.Sprintf("%s | %.3f | %.3f | %+.2f%% | %.3f | %+.2f%% | %+.2f%%", s.name(), rc, gc, 100*(gc/rc-1), a, 100*(rc/a-1), 100*(gc/a-1)))
		// The area is the arbiter. Both references are estimates themselves:
		// eight samples per pixel for the vector painter, a supersampled fan
		// for the old polygon.
		limit := 0.02
		if s.smallest() < 1 {
			limit = 0.05
		}
		if math.Abs(gc/a-1) > limit {
			return report, fmt.Errorf("%s: analytic coverage %.3f misses the area %.3f by more than %g%%", s.name(), gc, a, 100*limit)
		}
	}
	// A lit menu lamp overlaps several shapes, including a translucent gleam.
	positions := [][2]float64{{24, 20}, {24.375, 20.625}, {24.9999, 20.125}, {-1.25, 12.375}}
	for _, pos := range positions {
		lampEdge := 0
		parts := []deviceShape{
			{kind: shapeDisc, g: [5]float64{pos[0], pos[1], 5}, c: color.RGBA{39, 165, 59, 255}},
			{kind: shapeDisc, g: [5]float64{pos[0], pos[1], 3.6}, c: color.RGBA{61, 255, 92, 255}},
			{kind: shapeDisc, g: [5]float64{pos[0] - 1.25, pos[1] - 1.25, 1.25}, c: color.RGBA{255, 255, 255, 200}},
			{kind: shapeRing, g: [5]float64{pos[0], pos[1], 5, 1}, c: color.RGBA{0, 0, 0, 255}},
		}
		zone := func(x, y float64) int {
			for _, p := range parts {
				if p.zone(x, y) == zoneEdge {
					return zoneEdge
				}
			}
			return zoneInside
		}
		for bg := 0; bg < 5; bg++ {
			for clipIndex, clip := range clips {
				pixels := shapeBackground(w, h, bg)
				ref.WritePixels(pixels)
				got.WritePixels(pixels)
				for _, p := range parts {
					p.drawReference(ref.SubImage(clip).(*ebiten.Image))
					p.draw(got.SubImage(clip).(*ebiten.Image))
				}
				ref.ReadPixels(before)
				got.ReadPixels(after)
				edge, err := compare(fmt.Sprintf("layered lamp %v background %d clip %d", pos, bg, clipIndex), zone, clip)
				if err != nil {
					return report, err
				}
				lampEdge = max(lampEdge, edge)
				if bg == 3 && clipIndex == 0 {
					board.put(0, before, nil)
					board.put(1, after, nil)
					board.put(2, before, after)
				}
			}
		}
		report = append(report, fmt.Sprintf("layered lamp at %v: maximum edge difference %d", pos, lampEdge))
		board.row++
	}
	report = append(report, fmt.Sprintf("%d comparisons; maximum 8-bit channel difference: %d away from edges, %d on edge pixels", comparisons, zoneMax, edgeMax))
	if dir := os.Getenv("NANOLATHE_SCREENKIT_SHOTS"); dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return report, err
		}
		f, err := os.Create(filepath.Join(dir, "shapes.png"))
		if err != nil {
			return report, err
		}
		err = png.Encode(f, board.img)
		if closeErr := f.Close(); err == nil {
			err = closeErr
		}
		if err != nil {
			return report, err
		}
		report = append(report, "contact sheet: "+filepath.Join(dir, "shapes.png"))
	}
	return report, nil
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

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}
