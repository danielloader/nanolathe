package visibility

import (
	"fmt"
	"testing"

	"github.com/nanolathe-gg/nanolathe/internal/content"
)

func sightMutatorCatalog() *content.Catalog {
	c := &content.Catalog{Units: map[string]*content.UnitDef{"scout": {SightDistance: 320}}, Sight: fixtureShapes(), LOS: &content.LOSTables{NumTables: 9}}
	for r := 1; r <= 9; r++ {
		line := []int32{int32(r)}
		for x := 1; x <= r; x++ {
			line = append(line, int32(x), 0)
		}
		c.LOS.Tables = append(c.LOS.Tables, content.LOSTable{TableNum: r, NumLines: 1, Lines: [][]int32{line}})
	}
	return c
}

func TestSightMutatorWidensCoverageAndRetires(t *testing.T) {
	for _, ray := range []bool{false, true} {
		for _, f := range []content.Factor{{Num: 1, Den: 2}, {Num: 1, Den: 1}, {Num: 3, Den: 2}, {Num: 4, Den: 1}} {
			t.Run(fmt.Sprintf("ray=%v/%v", ray, f), func(t *testing.T) {
				c := sightMutatorCatalog()
				if err := c.ApplyMutators(content.Mutators{Sight: f}); err != nil {
					t.Fatal(err)
				}
				mode := ModeHistoryEnabled | ModeCurrentEnabled
				if ray {
					mode |= ModeTerrainRay
				}
				s := New(flatTerrain(256, 0), mode)
				s.SetShapes(c.Sight)
				s.SetRayTables(c.LOS)
				radius := c.Units["scout"].SightDistance
				reach := radius / 32
				if ray && f.Num <= f.Den {
					reach = min(reach, 8)
				}
				ob := Observer{Owner: 0, CX: 64, CZ: 64, HeightByte: 20, Radius: radius}
				s.Refresh(1, ob)
				at := func(x int32) uint8 { return s.ByteGrid(0)[64*s.W+64+x] }
				if at(reach) == 0 || at(reach+1) != 0 {
					t.Fatalf("reach %d: boundary=%d outside=%d", reach, at(reach), at(reach+1))
				}
				if f.Num > f.Den {
					for z := -reach; z <= reach; z++ {
						for x := -reach; x <= reach; x++ {
							if x*x+z*z <= reach*reach && s.ByteGrid(0)[(64+z)*s.W+64+x] == 0 {
								t.Fatalf("hole at %d,%d", x, z)
							}
						}
					}
				}
				s.RetireObserver(1)
				for _, v := range s.ByteGrid(0) {
					if v != 0 {
						t.Fatal("retired observer left current coverage")
					}
				}
			})
		}
	}
}

func TestSightMutatorKeepsTerrainOcclusion(t *testing.T) {
	c := sightMutatorCatalog()
	if err := c.ApplyMutators(content.Mutators{Sight: content.Factor{Num: 4, Den: 1}}); err != nil {
		t.Fatal(err)
	}
	terrain := flatTerrain(256, 0)
	terrain.SetLOSHeightWord(65, 64, 100, 100)
	s := New(terrain, ModeHistoryEnabled|ModeCurrentEnabled|ModeTerrainRay)
	s.SetRayTables(c.LOS)
	s.Publish(0, 64, 64, 20, c.Units["scout"].SightDistance)
	if s.ByteGrid(0)[64*s.W+65] == 0 {
		t.Fatal("ridge itself should be visible")
	}
	if s.ByteGrid(0)[64*s.W+66] != 0 {
		t.Fatal("larger sight saw through the ridge")
	}
	if s.ByteGrid(0)[64*s.W+24] == 0 {
		t.Fatal("unobstructed opposite edge did not expand")
	}
}
