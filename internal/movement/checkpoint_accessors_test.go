package movement

import (
	"bytes"
	"slices"
	"strings"
	"testing"

	"github.com/nanolathe-gg/nanolathe/internal/path"
	"github.com/nanolathe-gg/nanolathe/internal/sim/checkpoint"
)

func movementCheckpointAccessorFixture(t *testing.T) (*System, *CheckpointContext, *checkpointAccessorRoots) {
	t.Helper()
	s := &System{learned: &LearnedTerrain{words: []uint16{3}}}
	l := &ClassLayer{Profile: Profile{FootPrintX: 1, FootPrintZ: 2}, cells: []uint32{13}}
	s.layerRegistry = &ClassLayers{byName: map[string]*ClassLayer{"class": l}, names: []string{"class"}}
	base := checkpointClassCapture(l, l.Profile, 4, 2, 5)
	learned := checkpointLearnedCapture(base, s.learned, 2, 1, 2)
	through := checkpointThroughCapture(base, 2, 1, 1, 2)
	wall := checkpointWallCapture(through, s, 2, 1)
	wedge := checkpointWedgeCapture(learned, wall, Cell{X: 6, Z: 7}, 8, 9, 1, 2)
	own := &checkpointClaimRow{kind: 1, own: []uint32{10}}
	counts := &checkpointClaimRow{kind: 2, counts: [][8]uint8{{11}}}
	claims := &checkpointAccessorCapture{value: path.CheckpointAccessor{Kind: 7, Owner: 2, Serial: 12, Row: 2}, own: own, counts: counts}
	revision := &checkpointAccessorCapture{value: path.CheckpointAccessor{Kind: 8, Profile: checkpointProfile(l.Profile), Requester: 4, Tick: 5}, registry: s.layerRegistry, class: "class", layer: l}
	roots := &checkpointAccessorRoots{cost: claims, leg: learned, passable: wedge, revise: revision}
	armed := false
	cfg := path.SearchConfig{CostDir: func(path.Cell, uint8) int32 { panic("cost called") }, LegValue: func(path.Cell) uint8 { panic("leg called") }, PassableValue: func(path.Cell) uint8 { panic("passable called") }, Revise: func() {
		if armed {
			panic("revise called")
		}
	}}
	// The constructor makes its ordinary initial revision before capture.
	s.sessions = []*pathWorkingSet{{session: path.NewSession(cfg), checkpoint: roots}}
	armed = true
	c := movementCheckpointContext()
	return s, c, roots
}

func TestMovementCheckpointAccessorDAGOrderAndPurity(t *testing.T) {
	s, c, roots := movementCheckpointAccessorFixture(t)
	collectMovementCheckpoint(t, s, c)
	a, err := lowerCheckpointAccessors(c, roots)
	if err != nil {
		t.Fatal(err)
	}
	var kinds []uint8
	for _, n := range a.Nodes {
		kinds = append(kinds, n.Kind)
	}
	if !slices.Equal(kinds, []uint8{7, 1, 2, 3, 5, 6, 8}) {
		t.Fatalf("node order=%v", kinds)
	}
	if a.Cost != 1 || a.Leg != 3 || a.Passable != 6 || a.Revise != 7 || a.Nodes[2].Learned != 3 {
		t.Fatalf("lowered roots=%+v", a)
	}
	if !slices.Equal(a.Nodes[5].Inputs, []uint32{3, 5}) || a.Nodes[3].Inputs[0] != 2 {
		t.Fatal("shared/input order lost")
	}
	if a.Nodes[0].ClaimCounts != 1 || a.Nodes[0].ClaimOwn != 2 || a.Nodes[1].Layer != 1 {
		t.Fatal("external IDs not lexical encounter order")
	}
	before := movementCheckpointWrite(t, func(e *checkpoint.Encoder) { _ = s.WriteCheckpoint(e, c) })
	// Capture leaves source metadata unresolved and callback state untouched.
	if roots.cost.value.ClaimOwn != 0 || roots.leg.value.Inputs != nil || len(s.sessions) != 1 {
		t.Fatal("capture mutated retained metadata")
	}
	q := &path.Scheduler{}
	for {
		n, err := q.CollectCheckpointReferences(c.Paths)
		if err != nil {
			t.Fatal(err)
		}
		if n == 0 {
			break
		}
	}
	_ = movementCheckpointWrite(t, func(e *checkpoint.Encoder) { _ = q.WriteCheckpoint(e, c.Paths) })
	if after := movementCheckpointWrite(t, func(e *checkpoint.Encoder) { _ = s.WriteCheckpoint(e, c) }); !bytes.Equal(before, after) {
		t.Fatal("repeat capture changed movement")
	}
}

func TestMovementCheckpointOldClaimRowsAndPilotAlias(t *testing.T) {
	s, c, roots := movementCheckpointAccessorFixture(t)
	oldOwn, oldCounts := roots.cost.own, roots.cost.counts
	newOwn := &checkpointClaimRow{kind: 1, own: []uint32{20, 21}}
	newCounts := &checkpointClaimRow{kind: 2, counts: [][8]uint8{{22}, {23}}}
	state := &claimState{own: newOwn.own, checkpointOwn: newOwn, grids: []*claimGrid{{all: newCounts.counts, checkpointAll: newCounts}}, h: 2, w: 1}
	s.PilotState = &pilotsState{state, state, &NoPilot{}, nil}
	collectMovementCheckpoint(t, s, c)
	if len(c.pilots.Values()) != 3 || len(c.claimRows.Values()) != 4 {
		t.Fatal("pilot or old row identity lost")
	}
	if id, _ := c.claimRows.Find(oldOwn); id == 0 {
		t.Fatal("old own row absent")
	}
	if id, _ := c.claimRows.Find(oldCounts); id == 0 {
		t.Fatal("old count row absent")
	}
	a := movementCheckpointWrite(t, func(e *checkpoint.Encoder) { _ = s.WriteCheckpoint(e, c) })
	oldOwn.own[0]++
	b := movementCheckpointWrite(t, func(e *checkpoint.Encoder) { _ = s.WriteCheckpoint(e, c) })
	if bytes.Equal(a, b) {
		t.Fatal("old captured row change invisible")
	}
	state.own = []uint32{20, 21}
	var mismatch error
	for c, pass := movementCheckpointContext(), 0; pass < 3 && mismatch == nil; pass++ {
		_, mismatch = s.CollectCheckpointReferences(c)
	}
	if mismatch == nil {
		t.Fatal("mismatched current holder accepted")
	}
}

func TestMovementCheckpointAccessorRefusals(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(*System, *checkpointAccessorRoots)
		part   string
	}{
		{"cycle", func(_ *System, r *checkpointAccessorRoots) { r.leg.inputs[0] = r.leg }, "cyclic"},
		{"foreign learned", func(_ *System, r *checkpointAccessorRoots) { r.leg.learned = &LearnedTerrain{} }, "learned"},
		{"foreign wall", func(_ *System, r *checkpointAccessorRoots) { r.passable.inputs[1].system = &System{} }, "receiver"},
		{"lookup mismatch", func(_ *System, r *checkpointAccessorRoots) { r.revise.class = "missing" }, "lookup"},
		{"unknown", func(_ *System, r *checkpointAccessorRoots) { r.cost.value.Kind = 99 }, "irrelevant"},
		{"source width", func(_ *System, r *checkpointAccessorRoots) { r.revise.value.Profile[0] = 40000 }, "width"},
		{"wrong root", func(_ *System, r *checkpointAccessorRoots) { r.cost = r.leg }, "Cost"},
		{"irrelevant scalar", func(_ *System, r *checkpointAccessorRoots) { r.leg.value.Serial = 1 }, "irrelevant"},
		{"absent capture", func(_ *System, r *checkpointAccessorRoots) { r.passable = nil }, "presence"},
		{"unsupported pilot", func(_ *System, r *checkpointAccessorRoots) { r.unsupported = "unsupported pilot capture" }, "unsupported"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, c, r := movementCheckpointAccessorFixture(t)
			tc.change(s, r)
			_, err := s.CollectCheckpointReferences(c)
			if err == nil || !strings.Contains(err.Error(), tc.part) {
				t.Fatalf("error=%v", err)
			}
		})
	}
}

func TestMovementCheckpointEmptyClassRevisionUsesCapturedProfile(t *testing.T) {
	s, c, r := movementCheckpointAccessorFixture(t)
	r.revise.class = ""
	s.layerRegistry.byName = map[string]*ClassLayer{scratchLayerKey(Profile{FootPrintX: 1, FootPrintZ: 2}): r.revise.layer}
	// The mutable layer's current profile is not the old closure's profile.
	r.revise.layer.Profile.FootPrintX = 9
	collectMovementCheckpoint(t, s, c)
	if _, err := lowerCheckpointAccessors(c, r); err != nil {
		t.Fatal(err)
	}
}

type movementCheckpointUnknownSearch struct{ path.Search }
type movementCheckpointUnknownGoal struct {
	path.Goal
	_ []int // keeps the unsupported value non-comparable
}
type movementCheckpointUnknownPayload struct {
	GoalPayload
	_ []int // keeps the unsupported value non-comparable
}

func TestMovementCheckpointClosedInterfacesNeverInvokeMethods(t *testing.T) {
	for _, s := range []*System{
		{sessions: []*pathWorkingSet{{session: movementCheckpointUnknownSearch{}}}},
		{sessions: []*pathWorkingSet{{session: (*path.Session)(nil)}}},
		{unreachable: []unreachableCert{{goal: movementCheckpointUnknownGoal{}}}},
		{recordGoals: [][]recordGoal{{{air: movementCheckpointUnknownPayload{}}}}},
		{recordGoals: [][]recordGoal{{{air: (*airMarker)(nil)}}}},
	} {
		if _, err := s.CollectCheckpointReferences(movementCheckpointContext()); err == nil {
			t.Fatal("unknown/typed nil accepted")
		}
	}
}
