//go:build retail

package visibility

import (
	"fmt"
	"testing"

	"github.com/nanolathe-gg/nanolathe/internal/content"
	"github.com/nanolathe-gg/nanolathe/internal/testsupport/retailcat"
)

// Issue 94: the scaled definition alone cannot enlarge stock sight when the
// original tables clamp it. Exercise the actual reference catalog through
// both publishers, including current coverage and balanced removal.
func TestStockSightMutatorCoverage(t *testing.T) {
	base, _ := retailcat.Shared(t)
	for _, f := range []content.Factor{{Num: 1, Den: 4}, {Num: 3, Den: 4}, {Num: 1, Den: 1}, {Num: 3, Den: 2}, {Num: 4, Den: 1}} {
		c := base.Clone()
		if err := c.ApplyMutators(content.Mutators{Sight: f}); err != nil {
			t.Fatal(err)
		}
		for _, name := range []string{"armcom", "corcom", "armfav", "armpeep"} {
			for _, ray := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/%v/ray=%v", name, f, ray), func(t *testing.T) {
					mode := ModeHistoryEnabled | ModeCurrentEnabled
					if ray {
						mode |= ModeTerrainRay
					}
					s := New(flatTerrain(256, 0), mode)
					s.SetShapes(c.Sight)
					s.SetRayTables(c.LOS)
					radius := c.Units[name].SightDistance
					reach := radius / 32
					if f.Num <= f.Den {
						if ray {
							reach = min(reach, int32(int16(base.LOS.NumTables))-1)
						} else {
							reach = max(5, min(reach, int32(base.Sight.Count()+4)))
						}
					}
					s.Publish(0, 64, 64, 30, radius)
					// The four cardinal extents expose either raster's old table clamp.
					for _, d := range [][2]int32{{1, 0}, {-1, 0}, {0, 1}, {0, -1}} {
						inside := (64+d[1]*reach)*s.W + 64 + d[0]*reach
						outside := (64+d[1]*(reach+1))*s.W + 64 + d[0]*(reach+1)
						if s.ByteGrid(0)[inside] == 0 || s.ByteGrid(0)[outside] != 0 {
							t.Fatalf("sight %d: current coverage did not reach %d tiles", radius, reach)
						}
					}
					s.Unpublish(0, 64, 64, 30, radius)
					for _, v := range s.ByteGrid(0) {
						if v != 0 {
							t.Fatal("unpublish left coverage")
						}
					}
				})
			}
		}
	}
}
