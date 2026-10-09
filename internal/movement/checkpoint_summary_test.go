package movement

import (
	"testing"

	"github.com/nanolathe-gg/nanolathe/internal/path"
	"github.com/nanolathe-gg/nanolathe/internal/sim/checkpoint"
)

func checkMovementSummaryWords(t *testing.T, out checkpoint.Summary, words []int64) {
	t.Helper()
	var sum uint64
	for i, w := range words {
		sum += uint64(i+1) * uint64(w)
	}
	if n, s := out.Result(); n != uint64(len(words)) || s != sum {
		t.Fatalf("summary=(%d,%d), want=(%d,%d)", n, s, len(words), sum)
	}
}

func TestMovementCheckpointSummarySelectedScalarOrder(t *testing.T) {
	r := &Route{Count: 1, Active: true, Dirty: true, Status: 2, LastRequestTick: 3}
	r.Points[0] = Point{X: -4, Z: 5}
	r.Points[19] = Point{X: 6, Z: -7}
	s := &System{Routes: []*Route{nil, r}, Steers: []*SteerState{{X: -8, Z: 9, Heading: 10, Speed: -11, Dirty: true}},
		Collisions: []*CollisionState{{HasStamp: true, StampedAnchor: Cell{X: -12, Z: 13}, LastStampTick: 14, StampedPlane: 2, BlockerID: -1, Blocked: true}},
		Flights:    []*FlightState{{Mode: 2, X: 15, Y: -16, Z: 17, VX: 18, VY: 19, VZ: -20}}, workTick: -21, workSmooth: 22}
	var out checkpoint.Summary
	if err := s.AppendCheckpointSummary(&out); err != nil {
		t.Fatal(err)
	}
	words := []int64{2, 0, 1, 1, 1, 1, 2, 3, -4, 5}
	words = append(words, make([]int64, 36)...)
	words = append(words, 6, -7, 1, 1, -8, 9, 10, -11, 1, 1, 1, 1, -12, 13, 14, 2, -1, 1, 1, 1, 2, 15, -16, 17, 18, 19, -20, -21, 22)
	checkMovementSummaryWords(t, out, words)
	before := out
	// Deliberate blind spots include callbacks, graph identity and unselected
	// residuals. Full capture would refuse this unreviewed composition.
	s.SetProductFootprint(func(uint32) (int32, int32, bool) { panic("summary resolver called") })
	s.PilotState = []int{1}
	s.Routes[1].firstHold++
	s.Collisions[0].LeanX++
	s.Flights[0].Command = &FlightCommand{}
	out = checkpoint.Summary{}
	if err := s.AppendCheckpointSummary(&out); err != nil {
		t.Fatal(err)
	}
	if out != before {
		t.Fatal("unselected binding/state changed summary")
	}
	s.Collisions[0].LastStampTick++
	out = checkpoint.Summary{}
	_ = s.AppendCheckpointSummary(&out)
	if out == before {
		t.Fatal("selected stamp change invisible")
	}
	if a := testing.AllocsPerRun(20, func() { var out checkpoint.Summary; _ = s.AppendCheckpointSummary(&out) }); a != 0 {
		t.Fatalf("summary allocated %g times", a)
	}
}

func TestMovementCheckpointPathSummaryOrderAndAtomicRefusal(t *testing.T) {
	s := &System{sessions: []*pathWorkingSet{nil, {}, {session: &path.Session{}}}}
	var out checkpoint.Summary
	out.Word(9)
	if err := s.AppendPathCheckpointSummary(&out); err != nil {
		t.Fatal(err)
	}
	checkMovementSummaryWords(t, out, []int64{9, 3, 0, 0, 1, 0, 0, 0, 0, 0})
	before := out
	s.sessions = append(s.sessions, &pathWorkingSet{session: movementCheckpointUnknownSearch{}})
	if err := s.AppendPathCheckpointSummary(&out); err == nil {
		t.Fatal("unknown search accepted")
	}
	if out != before {
		t.Fatal("failed fragment partially appended")
	}
	s.sessions = s.sessions[:3]
	if a := testing.AllocsPerRun(20, func() { var out checkpoint.Summary; _ = s.AppendPathCheckpointSummary(&out) }); a != 0 {
		t.Fatalf("path summary allocated %g times", a)
	}
}
