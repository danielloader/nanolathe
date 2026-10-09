package movement

import (
	"bytes"
	"encoding/binary"
	"strings"
	"testing"

	"github.com/nanolathe-gg/nanolathe/internal/orders"
	"github.com/nanolathe-gg/nanolathe/internal/path"
	"github.com/nanolathe-gg/nanolathe/internal/sim/checkpoint"
	"github.com/nanolathe-gg/nanolathe/internal/units"
)

func movementCheckpointContext() *CheckpointContext {
	return NewCheckpointContext(orders.NewCheckpointContext(units.NewCheckpointContext(nil)), path.NewCheckpointContext())
}

func collectMovementCheckpoint(t *testing.T, s *System, c *CheckpointContext) {
	t.Helper()
	for pass := 0; pass < 12; pass++ {
		n, err := s.CollectCheckpointReferences(c)
		if err != nil {
			t.Fatal(err)
		}
		if n == 0 {
			return
		}
	}
	t.Fatal("movement reference discovery did not converge")
}

func movementCheckpointBytes(t *testing.T, s *System) []byte {
	t.Helper()
	c := movementCheckpointContext()
	collectMovementCheckpoint(t, s, c)
	var b bytes.Buffer
	if err := s.WriteCheckpoint(checkpoint.NewEncoder(&b), c); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

// Authored values carry explicit widths and do not read the tested fixture.
func movementCheckpointVector(t *testing.T, words ...any) []byte {
	t.Helper()
	var b bytes.Buffer
	for _, v := range words {
		if err := binary.Write(&b, binary.LittleEndian, v); err != nil {
			t.Fatal(err)
		}
	}
	return b.Bytes()
}

func movementCheckpointWrite(t *testing.T, f func(*checkpoint.Encoder)) []byte {
	t.Helper()
	var b bytes.Buffer
	e := checkpoint.NewEncoder(&b)
	f(e)
	if err := e.Err(); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func TestMovementCheckpointMoveGoalAliasesAndDetachedClosure(t *testing.T) {
	build := func(split bool) (*System, *CheckpointContext) {
		g := &moveGoal{goal: path.PointGoal(path.Cell{X: 3, Z: 4}, 1), order: &orders.Node{}, x: 5, z: 6}
		other := g
		if split {
			v := *g
			other = &v
		}
		s := &System{moveGoals: []*moveGoal{nil, g}, recordGoals: [][]recordGoal{{{ground: other}}}}
		c := movementCheckpointContext()
		// Pre-registered detached handles are roots, including their order/goal.
		if _, err := c.moveGoals.Add(&moveGoal{goal: path.PointGoal(path.Cell{X: 7}, 0), order: &orders.Node{}}); err != nil {
			t.Fatal(err)
		}
		collectMovementCheckpoint(t, s, c)
		return s, c
	}
	s, c := build(false)
	if len(c.moveGoals.Values()) != 2 || len(c.Paths.Goals.Values()) != 2 || len(c.Orders.Nodes.Values()) != 2 {
		t.Fatal("detached graph closure missing")
	}
	a := movementCheckpointWrite(t, func(e *checkpoint.Encoder) { _ = s.WriteCheckpoint(e, c) })
	s, c = build(false)
	b := movementCheckpointWrite(t, func(e *checkpoint.Encoder) { _ = s.WriteCheckpoint(e, c) })
	if !bytes.Equal(a, b) {
		t.Fatal("allocation addresses changed bytes")
	}
	s, c = build(true)
	if len(c.moveGoals.Values()) != 3 {
		t.Fatal("equal-value distinct move-goal identity collapsed")
	}
	b = movementCheckpointWrite(t, func(e *checkpoint.Encoder) { _ = s.WriteCheckpoint(e, c) })
	if bytes.Equal(a, b) {
		t.Fatal("move-goal alias split was invisible")
	}
}

func TestMovementCheckpointRetainedResiduals(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(*System)
	}{
		{"route residual", func(s *System) { s.Routes[0].Points[19].Z++ }},
		{"route held request", func(s *System) { s.Routes[0].firstHold++ }},
		{"steer pending", func(s *System) { s.Steers[0].PendingHeading++ }},
		{"collision residual", func(s *System) { s.Collisions[0].TurnResidual++ }},
		{"collision stamp", func(s *System) { s.Collisions[0].LastStampTick++ }},
		{"flight command", func(s *System) { s.Flights[0].Command.Pos.X++ }},
		{"flight mirror", func(s *System) { s.Flights[0].ModeMirror++ }},
		{"profile key", func(s *System) { s.profileNames[0] = "other" }},
		{"profile value", func(s *System) { s.profiles[0].MinWaterDepth++ }},
		{"tier residual", func(s *System) { s.prevMoveTier[0]++ }},
		{"sfx residual", func(s *System) { s.prevSFXBand[0]++ }},
		{"pending filing", func(s *System) { s.pendingFiled[0] = true }},
		{"pocket count", func(s *System) { s.pocketLive++ }},
		{"unreachable count", func(s *System) { s.unreachableLive++ }},
		{"pocket residual", func(s *System) { s.pockets[0].grants++ }},
		{"unreachable residual", func(s *System) { s.unreachable[0].since++ }},
		{"traffic residual", func(s *System) { s.traffic[0].side = -1 }},
		{"jam residual", func(s *System) { s.jamReleases[0].run++ }},
		{"work", func(s *System) { s.workSmooth++ }},
		{"learned", func(s *System) { s.learned.words[0]++ }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := &System{Routes: []*Route{{}}, Steers: []*SteerState{{}}, Collisions: []*CollisionState{{}}, Flights: []*FlightState{{Command: &FlightCommand{}}},
				profileNames: []string{"class"}, profiles: []*Profile{{}}, prevMoveTier: []int{0}, prevSFXBand: []int{0}, pendingFiled: []bool{false},
				pockets: []pocketCert{{}}, unreachable: []unreachableCert{{}}, traffic: []trafficState{{}}, jamReleases: []jamRelease{{}}, learned: &LearnedTerrain{words: []uint16{0}}}
			a := movementCheckpointBytes(t, s)
			tc.change(s)
			b := movementCheckpointBytes(t, s)
			if bytes.Equal(a, b) {
				t.Fatal("retained mutation was invisible")
			}
		})
	}
}

func TestMovementCheckpointScratchAndDiagnosticsExcluded(t *testing.T) {
	s := &System{Routes: []*Route{{}}, Flights: []*FlightState{{Command: &FlightCommand{}}}, Grid: &OccupancyGrid{}}
	a := movementCheckpointBytes(t, s)
	s.Routes[0].StaticRevision = 55
	s.Flights[0].Command.Flags = 7
	s.Grid.rev = 99
	s.collisionTraceEnabled = true
	s.collisionHistoryLimit = 8
	s.collisionHistoryDropped = true
	s.countRefusals = true
	s.pocketCells = []uint8{1}
	s.pocketSeen = []bool{true}
	s.pocketStack = []Cell{{X: 9}}
	s.unreachableGoals = []path.Cell{{X: 10}}
	s.searchCfg.Revise = func() { panic("scratch called") }
	s.passAlliance = func(uint8, uint8) bool { panic("per-tick scratch called") }
	if b := movementCheckpointBytes(t, s); !bytes.Equal(a, b) {
		t.Fatal("scratch/diagnostic changed checkpoint")
	}
}

func TestMovementCheckpointRefusesUnattestedAndActiveBindings(t *testing.T) {
	for _, tc := range []struct {
		name string
		s    *System
		part string
	}{
		{"tick", &System{tickStarted: true}, "sweep"},
		{"handoff", &System{checkpointSearch: &checkpointAccessorRoots{}}, "handoff"},
		{"resolver", &System{productFootprint: func(uint32) (int32, int32, bool) { panic("resolver called") }}, "unattested"},
		{"grid callback", &System{Grid: &OccupancyGrid{ownerState: func(uint8) uint8 { panic("owner called") }}}, "unattested"},
		{"wrong overlap", &System{Grid: &OccupancyGrid{overlap: &System{}}}, "overlap"},
		{"pilot value", &System{PilotState: NoPilot{}}, "unsupported"},
		{"pilot typed nil", &System{PilotState: (*claimState)(nil)}, "typed-nil"},
		{"pilot unhashable", &System{PilotState: []int{1}}, "unsupported"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := movementCheckpointContext()
			_, err := tc.s.CollectCheckpointReferences(c)
			if err == nil || !strings.Contains(err.Error(), tc.part) {
				t.Fatalf("error=%v", err)
			}
			var b bytes.Buffer
			if err := tc.s.WriteCheckpoint(checkpoint.NewEncoder(&b), c); err == nil {
				t.Fatal("writer accepted refusal")
			}
		})
	}
}

func TestMovementCheckpointWriterRequiresDiscovery(t *testing.T) {
	s := &System{activeOrders: []*activeMove{{order: &orders.Node{}}}}
	c := movementCheckpointContext()
	var b bytes.Buffer
	if err := s.WriteCheckpoint(checkpoint.NewEncoder(&b), c); err == nil || !strings.Contains(err.Error(), "undiscovered") {
		t.Fatalf("error=%v", err)
	}
	if len(c.Orders.Nodes.Values()) != 0 {
		t.Fatal("writer added reference")
	}
}
