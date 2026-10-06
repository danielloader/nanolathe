package content

import (
	"reflect"
	"testing"
)

func TestSightMutatorExtendsOnlyNeededRasterInputs(t *testing.T) {
	base := mutatorFixture(t)
	base.LOS = &LOSTables{NumTables: 3, Tables: []LOSTable{
		{TableNum: 1, NumLines: 1, Lines: [][]int32{{1, 1, 0}}},
		{TableNum: 2, NumLines: 1, Lines: [][]int32{{1, 2, 0}}},
		{TableNum: 3, NumLines: 1, Lines: [][]int32{{1, 99, 0}}},
	}}
	base.Sight = &SightShapes{Shapes: []SightShape{{W: 1, H: 1, Opaque: []bool{true}}}}
	pristine := base.Clone()
	for _, factor := range []Factor{{}, {1, 1}, {1, 2}, {3, 2}, {4, 1}} {
		c := base.Clone()
		if err := c.ApplyMutators(Mutators{Sight: factor}); err != nil {
			t.Fatal(err)
		}
		if factor.Num <= factor.Den {
			if !reflect.DeepEqual(c.LOS, base.LOS) || !reflect.DeepEqual(c.Sight, base.Sight) {
				t.Fatal("identity/reduction changed raster inputs")
			}
			continue
		}
		q := int(c.Units["armbuilder"].SightDistance) / 32
		if c.LOS.NumTables != int32(q+1) || len(c.Sight.Shapes) != q-4 {
			t.Fatalf("factor %v did not extend to radius %d", factor, q)
		}
		if !reflect.DeepEqual(c.LOS.Tables[:2], base.LOS.Tables[:2]) || !reflect.DeepEqual(c.Sight.Shapes[:1], base.Sight.Shapes) {
			t.Fatal("original reachable inputs changed")
		}
		if !reflect.DeepEqual(c.LOS.Tables[q+1:], base.LOS.Tables[2:]) {
			t.Fatal("unreachable authored residue lost or activated")
		}
		if !reflect.DeepEqual(c.LOS.Tables[2], base.LOS.Tables[1]) || !reflect.DeepEqual(c.Sight.Shapes[1], base.Sight.Shapes[0]) {
			t.Fatal("unrequested radius lost its former clamped footprint")
		}
		if c.LOS.Tables[q-1].TableNum != q || c.Sight.Shapes[q-5].AnchorX != int32(q) {
			t.Fatal("requested radius has no generated geometry")
		}
	}
	if !reflect.DeepEqual(base.LOS, pristine.LOS) || !reflect.DeepEqual(base.Sight, pristine.Sight) {
		t.Fatal("shared catalog changed")
	}
}

// Dense rays must cover the entire larger disc without a single observer
// overflowing its current-coverage byte at the stock catalog's x4 ranges.
func TestGeneratedSightRaysCoverDisc(t *testing.T) {
	for r := 9; r <= 56; r++ {
		tb := mutatorSightRays(r)
		counts := make([]uint16, (2*r+1)*(2*r+1))
		for _, line := range tb.Lines {
			for i := 0; i < int(line[0]); i++ {
				x, y := int(line[1+2*i]), int(line[2+2*i])
				for _, p := range [][2]int{{x, -y}, {y, x}, {-x, y}, {-y, -x}} {
					counts[(p[1]+r)*(2*r+1)+p[0]+r]++
				}
			}
		}
		for y := -r; y <= r; y++ {
			for x := -r; x <= r; x++ {
				if x == 0 && y == 0 {
					continue
				}
				got := counts[(y+r)*(2*r+1)+x+r]
				inside := x*x+y*y <= r*r
				if (got > 0) != inside || got >= 256 {
					t.Fatalf("radius %d at %d,%d: visits %d, inside %v", r, x, y, got, inside)
				}
			}
		}
	}
}
