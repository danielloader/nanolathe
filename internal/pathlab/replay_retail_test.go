//go:build retail

package pathlab

import (
	"testing"

	"github.com/nanolathe-gg/nanolathe/internal/testsupport"
)

// A recording can reuse an ID on the tick its former unit dies. Structures
// stage before movers, so a new structure may already own the ID when the
// former mover's death is applied. Death belongs to that recorded lifetime;
// orders after replacement belong to the current ID owner.
func TestReplayDeathKeepsRecordedInstanceIdentity(t *testing.T) {
	c, err := LoadContent([]string{testsupport.RetailRoot(t)}, "none")
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	for _, tc := range []struct {
		name      string
		id        int
		unit      string
		structure bool
	}{
		{"distinct ID", 43, "armllt", true},
		{"reused ID, structure", 42, "armllt", true},
		{"reused ID, mover", 42, "armpw", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := &Snippet{
				Schema: 1, ID: "authored-reused-id", Map: "the pass", Content: "ota",
				Start: 100, T0: 100, T1: 104, Region: [4]float64{400, 400, 1000, 700}, Players: []int{0},
				Units: []SnipUnit{
					{ID: 42, Name: "armpw", Player: 0, X: 512, Z: 512, Spawn: 100, Die: 102},
					{ID: tc.id, Name: tc.unit, Player: 0, X: 640, Z: 512, Structure: tc.structure, Spawn: 102, Die: -1},
				},
			}
			if !tc.structure {
				s.Orders = []SnipOrder{{Tick: 103, Unit: tc.id, GoalX: 900, GoalZ: 512}}
			}
			r, err := Replay(c, s, ReplayOptions{Rules: "modern", Seed: 1})
			if err != nil {
				t.Fatal(err)
			}
			if r.Log.Engine.Dropped != 0 {
				t.Fatalf("fixture dropped a unit: %+v", r.Log.Engine)
			}
			logs := r.Log.Players[0].Units
			if len(logs) != 2 {
				t.Fatalf("got %d unit logs, want both recorded lifetimes", len(logs))
			}
			old, next := logs[0], logs[1]
			if old.NetID != 42 || next.NetID != tc.id || old.Tick != 100 || next.Tick != 102 {
				t.Fatalf("recorded identities changed: old %+v, next %+v", old, next)
			}
			if old.DiedTick == nil || *old.DiedTick != 102 || next.DiedTick != nil {
				t.Fatalf("old death %v, replacement death %v: only the former lifetime should die at 102", old.DiedTick, next.DiedTick)
			}
			if !tc.structure && (len(old.Goals) != 0 || len(next.Goals) == 0 || next.Goals[0][0] != 103) {
				t.Fatalf("replacement order was misrouted: old goals %v, replacement goals %v", old.Goals, next.Goals)
			}
		})
	}
}
