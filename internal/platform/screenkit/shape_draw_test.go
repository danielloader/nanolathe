package screenkit

import (
	"image/color"
	"math"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

func TestShapeShaderCompiles(t *testing.T) {
	if _, err := ebiten.NewShader([]byte(shapeShaderSource)); err != nil {
		t.Fatal(err)
	}
}

// Every quad corner must map its local coordinates back to its destination
// position through the quad's own axes, reach one pixel past the shape, and
// carry the kind and parameters unchanged.
func TestShapeQuadsCarryLocalFrames(t *testing.T) {
	green := color.RGBA{61, 255, 92, 70}
	var q [4]ebiten.Vertex
	check := func(name string, cx, cy, ux, uy, ex, ey float64, kind, p1, p2, p3 float32) {
		t.Helper()
		for i, v := range q {
			lx, ly := float64(v.SrcX), float64(v.SrcY)
			if math.Abs(math.Abs(lx)-ex) > 1e-4 || math.Abs(math.Abs(ly)-ey) > 1e-4 {
				t.Fatalf("%s corner %d local (%v, %v), want extents (%v, %v)", name, i, lx, ly, ex, ey)
			}
			x, y := cx+lx*ux-ly*uy, cy+lx*uy+ly*ux
			if math.Abs(float64(v.DstX)-x) > 1e-4 || math.Abs(float64(v.DstY)-y) > 1e-4 {
				t.Fatalf("%s corner %d at (%v, %v), want (%v, %v)", name, i, v.DstX, v.DstY, x, y)
			}
			if v.Custom0 != kind || v.Custom1 != p1 || v.Custom2 != p2 || v.Custom3 != p3 {
				t.Fatalf("%s corner %d custom %v %v %v %v", name, i, v.Custom0, v.Custom1, v.Custom2, v.Custom3)
			}
			// Colour is read as image/color's premultiplied RGBA, as the
			// vector painter read it.
			if v.ColorR != 61.0/255 || v.ColorG != 1 || v.ColorB != 92.0/255 || v.ColorA != 70.0/255 {
				t.Fatalf("%s corner %d colour %v %v %v %v", name, i, v.ColorR, v.ColorG, v.ColorB, v.ColorA)
			}
		}
		// The index pattern {0,1,2,1,3,2} needs corners 0 and 3 opposite.
		if q[0].SrcX != -q[3].SrcX || q[0].SrcY != -q[3].SrcY || q[1].SrcX != -q[2].SrcX || q[1].SrcY != -q[2].SrcY {
			t.Fatalf("%s corners are not paired diagonally", name)
		}
	}
	if !discQuad(&q, 24.375, 20.625, 5.25, green) {
		t.Fatal("disc rejected")
	}
	check("disc", 24.375, 20.625, 1, 0, 6.25, 6.25, shapeDisc, 5.25, 0, 0)
	if !ringQuad(&q, 24, 20, 5, 1.5, green) {
		t.Fatal("ring rejected")
	}
	check("ring", 24, 20, 1, 0, 6.75, 6.75, shapeRing, 4.25, 5.75, 0)
	// A stroke wider than the diameter is the whole outer disc.
	if !ringQuad(&q, 24, 20, 1, 3, green) {
		t.Fatal("wide ring rejected")
	}
	check("wide ring", 24, 20, 1, 0, 3.5, 3.5, shapeRing, 0, 2.5, 0)
	// A 3-4-5 segment: half-length 2.5, half-width 0.75, larger normal
	// component 0.8, quad rotated onto the segment about its midpoint.
	if !lineQuad(&q, 10, 10, 13, 14, 1.5, green) {
		t.Fatal("line rejected")
	}
	check("line", 11.5, 12, 0.6, 0.8, 3.5, 1.75, shapeLine, 2.5, 0.75, 0.8)
	if !lineQuad(&q, 13, 14, 10, 10, 1.5, green) {
		t.Fatal("reversed line rejected")
	}
	check("reversed line", 11.5, 12, -0.6, -0.8, 3.5, 1.75, shapeLine, 2.5, 0.75, 0.8)
}

func TestShapeQuadsRejectDegenerateInput(t *testing.T) {
	var q [4]ebiten.Vertex
	c := color.RGBA{255, 205, 80, 255}
	nan, inf := math.NaN(), math.Inf(1)
	for name, ok := range map[string]bool{
		"zero radius":       discQuad(&q, 1, 1, 0, c),
		"negative radius":   discQuad(&q, 1, 1, -2, c),
		"NaN radius":        discQuad(&q, 1, 1, nan, c),
		"Inf centre":        discQuad(&q, inf, 1, 2, c),
		"NaN centre":        discQuad(&q, 1, nan, 2, c),
		"distant centre":    discQuad(&q, 1<<25, 1, 2, c),
		"clear colour":      discQuad(&q, 1, 1, 2, color.RGBA{}),
		"ring zero width":   ringQuad(&q, 1, 1, 2, 0, c),
		"ring zero radius":  ringQuad(&q, 1, 1, 0, 2, c),
		"ring Inf width":    ringQuad(&q, 1, 1, 2, inf, c),
		"line zero width":   lineQuad(&q, 0, 0, 4, 4, 0, c),
		"line zero length":  lineQuad(&q, 3, 3, 3, 3, 2, c),
		"line NaN endpoint": lineQuad(&q, 0, 0, nan, 4, 2, c),
		"line NaN width":    lineQuad(&q, 0, 0, 4, 4, nan, c),
	} {
		if ok {
			t.Errorf("%s was drawn", name)
		}
	}
	// An alpha of zero with colour is still additive light under the
	// premultiplied reading, so only a fully clear colour is skipped.
	if !discQuad(&q, 1, 1, 2, color.RGBA{40, 0, 0, 0}) {
		t.Error("additive colour with zero alpha was skipped")
	}
	white := shapeColour{1, 1, 1, 1}
	for name, pts := range map[string][]float64{
		"two points":     {0, 0, 4, 0},
		"collinear":      {0, 0, 2, 2, 4, 4},
		"repeated point": {1, 1, 1, 1, 1, 1},
		"NaN vertex":     {0, 0, 4, nan, 0, 4},
		"clear":          nil,
	} {
		if vs := polyFan(nil, pts, white); len(vs) != 0 {
			t.Errorf("polygon %s drew %d vertices", name, len(vs))
		}
	}
	if vs := polyFan(nil, []float64{0, 0, 4, 0, 0, 4}, shapeColour{}); len(vs) != 0 {
		t.Error("clear polygon drew")
	}
}

// Every fan vertex lies a reach outside the two edges that meet at its
// polygon corner, whatever the winding; the original corners lie inside or
// on every edge; and each triangle names the edges at the apex and around
// its far side.
func TestPolyFanOutsetsAndEdgeSlots(t *testing.T) {
	square := []float64{2, 2, 8, 2, 8, 8, 2, 8}
	hexagon := make([]float64, 0, 12)
	for i := 0; i < 6; i++ {
		a := float64(i) * math.Pi / 3
		hexagon = append(hexagon, 20+7*math.Cos(a), 20+7*math.Sin(a))
	}
	reversed := []float64{2, 8, 8, 8, 8, 2, 2, 2}
	arrow := []float64{20, 24.4, 15.2, 16.8, 24.8, 16.8}
	for name, pts := range map[string][]float64{"square": square, "reversed": reversed, "hexagon": hexagon, "arrow": arrow, "repeated closing point": append(append([]float64{}, arrow...), 20, 24.4)} {
		vs := polyFan(nil, pts, shapeColour{1, 1, 1, 1})
		n := len(polyPoints)
		if len(vs) != 3*(n-2) {
			t.Fatalf("%s: %d vertices for %d corners", name, len(vs), n)
		}
		for _, v := range vs {
			if v.Custom0 != shapePoly {
				t.Fatalf("%s: kind %v", name, v.Custom0)
			}
			d := [5]float32{v.SrcX, v.SrcY, v.Custom1, v.Custom2, v.Custom3}
			reach := 0
			for _, x := range d {
				if x > 1.0001 {
					t.Fatalf("%s: vertex (%v, %v) is %v outside an edge", name, v.DstX, v.DstY, x)
				}
				if math.Abs(float64(x)-shapeReach) < 1e-4 {
					reach++
				}
			}
			if reach < 2 {
				t.Fatalf("%s: vertex (%v, %v) distances %v: not a reach outside two edges", name, v.DstX, v.DstY, d)
			}
		}
		for _, p := range polyPoints {
			for _, e := range polyEdges {
				if e.distance(p) > 1e-9 {
					t.Fatalf("%s: corner %v outside edge %v", name, p, e)
				}
			}
		}
	}
	// The hexagon's middle triangles keep all five slots; the arrow's only
	// triangle needs three and pads the rest inside.
	vs := polyFan(nil, hexagon, shapeColour{1, 1, 1, 1})
	if v := vs[3]; v.Custom3 == -1 {
		t.Fatal("hexagon middle triangle lost an edge slot")
	}
	vs = polyFan(nil, arrow, shapeColour{1, 1, 1, 1})
	for _, v := range vs {
		if v.Custom2 != -1 || v.Custom3 != -1 {
			t.Fatalf("triangle padding slots %v %v", v.Custom2, v.Custom3)
		}
	}
}
