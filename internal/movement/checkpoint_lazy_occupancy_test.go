package movement

import (
	"bytes"
	"testing"

	"github.com/nanolathe-gg/nanolathe/internal/sim/checkpoint"
)

// Physical lengths remain in the schema even before either lazy plane has
// been allocated (DESIGN_MULTIPLAYER §16.3.61). An occupant stores identity+1.
func TestMovementCheckpointLazyOccupancyVectors(t *testing.T) {
	suffix := movementCheckpointVector(t, uint8(0), uint64(0), uint32(0), uint8(0), uint8(0), uint8(0), int32(1), int32(2), uint8(0), int32(0), uint32(0), int32(0))
	for _, tc := range []struct {
		name                  string
		ground, air           []int32
		groundCount, airCount int
		prefix                []byte
	}{
		{"untouched", nil, nil, 0, 0, movementCheckpointVector(t, uint32(0), uint32(0))},
		{"ground only", []int32{1, 0}, nil, 1, 0, movementCheckpointVector(t, uint32(0), uint32(2), uint8(1), int64(0), uint8(0))},
		{"air only", nil, []int32{0, 4}, 0, 1, movementCheckpointVector(t, uint32(2), uint8(0), uint8(1), int64(3), uint32(0))},
	} {
		t.Run(tc.name, func(t *testing.T) {
			g := &OccupancyGrid{planeW: 2, planeH: 1, cells: tc.ground, air: tc.air, cellCount: tc.groundCount, airCount: tc.airCount}
			s, c := &System{Grid: g}, movementCheckpointContext()
			want := append(tc.prefix, suffix...)
			for range 2 {
				got := movementCheckpointWrite(t, func(e *checkpoint.Encoder) { writeMovementGrid(e, c, s, g, "grid") })
				if !bytes.Equal(got, want) {
					t.Fatalf("grid vector\ngot %x\nwant %x", got, want)
				}
			}
			if (g.cells == nil) != (tc.ground == nil) || (g.air == nil) != (tc.air == nil) || g.cellCount != tc.groundCount || g.airCount != tc.airCount || g.rev != 0 {
				t.Fatal("capture allocated planes or changed counts")
			}
			collectMovementCheckpoint(t, s, c)
		})
	}
}

func TestMovementCheckpointLazyOccupancyMalformedRefusals(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(*OccupancyGrid)
	}{
		{"negative width", func(g *OccupancyGrid) { g.planeW = -1 }},
		{"negative height", func(g *OccupancyGrid) { g.planeH = -1 }},
		{"empty ground", func(g *OccupancyGrid) { g.cells = []int32{} }},
		{"empty air", func(g *OccupancyGrid) { g.air = []int32{} }},
		{"short ground", func(g *OccupancyGrid) { g.cells = []int32{0} }},
		{"long air", func(g *OccupancyGrid) { g.air = []int32{0, 0, 0} }},
		{"nil ground count", func(g *OccupancyGrid) { g.cellCount = 1 }},
		{"nil air count", func(g *OccupancyGrid) { g.airCount = 1 }},
		{"wrong stored count", func(g *OccupancyGrid) { g.cells = []int32{0, 1} }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			g := &OccupancyGrid{planeW: 2, planeH: 1}
			tc.change(g)
			s, c := &System{Grid: g}, movementCheckpointContext()
			movementAuxiliaryRefused(t, s, c)
			var out bytes.Buffer
			e := checkpoint.NewEncoder(&out)
			writeMovementGrid(e, c, s, g, "grid")
			if e.Err() == nil || out.Len() != 0 {
				t.Fatal("malformed grid wrote partial bytes")
			}
		})
	}
	// Non-nil empty storage is valid for zero dimensions and encodes its actual
	// zero length; nil and empty do not invent distinct wire presence tags.
	g := &OccupancyGrid{cells: []int32{}, air: []int32{}}
	s := &System{Grid: g}
	if err := validateMovementGrid(s, movementCheckpointContext(), g); err != nil {
		t.Fatal(err)
	}
}
