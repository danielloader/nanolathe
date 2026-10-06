package visibility

import (
	"reflect"
	"testing"

	"github.com/nanolathe-gg/nanolathe/internal/content"
)

func TestLOSFillsSignedQuadrantsFromDeclaredLines(t *testing.T) {
	// The gap and undeclared residue cannot shift or add rays. The first
	// asymmetric pair distinguishes reflection from rotation [03 R-VIS-01 §3].
	tb := content.LOSTable{NumLines: 3, Lines: [][]int32{
		{1, 1, 2}, nil, {1, 65539, 32768}, {1, 7, 8},
	}}
	want := [][]step{
		{{1, -2, 1}}, {{3, -32768, 1}},
		{{2, 1, 1}}, {{-32768, 3, 1}},
		{{-1, 2, 1}}, {{-3, -32768, 1}},
		{{-2, -1, 1}}, {{-32768, -3, 1}},
	}
	if got := buildSpokes(tb); !reflect.DeepEqual(got, want) {
		t.Fatalf("quadrant spokes = %v, want %v", got, want)
	}
	if got := buildSpokes(content.LOSTable{Lines: tb.Lines}); len(got) != 0 {
		t.Fatalf("zero declared lines yielded %v", got)
	}
}

func TestLOSRayReentryKeepsPointOrdinal(t *testing.T) {
	terrain := flatTerrain(16, 0)
	s := New(terrain, ModeHistoryEnabled|ModeCurrentEnabled|ModeTerrainRay)
	s.SetRayTables(&content.LOSTables{NumTables: 2, Tables: []content.LOSTable{{
		NumLines: 1, Lines: [][]int32{{3, 1, 0, 99, 0, 2, 0}},
	}}})
	// East ray: first point establishes -2/1, second is off-map, third
	// offers -5/3 and passes. Renumbering it to distance two would reject.
	terrain.SetLOSHeightWord(4, 3, 8, 8)
	terrain.SetLOSHeightWord(5, 3, 5, 5)
	seen := make(map[int]bool)
	s.walkTerrainRay(3, 3, 10, 32, func(idx int) { seen[idx] = true })
	if !seen[int(3*s.W+5)] {
		t.Fatal("ray did not admit its third point after leaving and re-entering the map")
	}
	// An ordered two-point line fails the strict comparison at -5/2 instead.
	s.SetRayTables(&content.LOSTables{NumTables: 2, Tables: []content.LOSTable{{
		NumLines: 1, Lines: [][]int32{{2, 1, 0, 2, 0}},
	}}})
	seen = make(map[int]bool)
	s.walkTerrainRay(3, 3, 10, 32, func(idx int) { seen[idx] = true })
	if seen[int(3*s.W+5)] {
		t.Fatal("ordered second point passed despite its lower horizon slope")
	}
}

func TestLOSTableClampUsesNarrowedCount(t *testing.T) {
	s := New(flatTerrain(16, 0), ModeTerrainRay)
	s.SetRayTables(&content.LOSTables{NumTables: 65538})
	if got := s.rayTableCount(); got != 2 {
		t.Fatalf("table count = %d, want 2", got)
	}
	if got := s.rayTableIndex(96); got != 1 {
		t.Fatalf("table group = %d, want clamp 1", got)
	}
}
