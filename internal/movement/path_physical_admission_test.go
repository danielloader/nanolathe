package movement

import (
	"testing"

	"github.com/nanolathe-gg/nanolathe/internal/content"
	"github.com/nanolathe-gg/nanolathe/internal/path"
	"github.com/nanolathe-gg/nanolathe/internal/pool"
)

// TestPathProviderPollsPhysicalSlots locks R1's admission surface: pending
// payloads do not determine service order, holes and non-movers consume their
// physical visits, and the timestamp belongs to the successful follower poll
// [04 R-PATH-01 §6][04 R-MOV-01 §7].
func TestPathProviderPollsPhysicalSlots(t *testing.T) {
	s := NewSystem(syntheticTerrainForIntegrate(), wiringProfile, NewOccupancyGrid())
	w := newMovementFixtureWorld(4)
	s.BindWorld(w)
	start, end, ok := w.SliceForPlayer(0)
	if !ok || end-start+1 != 4 {
		t.Fatalf("player-0 slice = %d..%d/%v, want four physical slots", start, end, ok)
	}
	mover := wiringDef()
	nonMover := &content.UnitDef{UnitName: "physical-building", MaxDamage: 1, BMCode: 0}
	h1, err := w.CreateWithForcedSlot(mover, 0, 0, 0, 0, pool.Handle(start))
	if err != nil {
		t.Fatalf("create first mover: %v", err)
	}
	h2, err := w.CreateWithForcedSlot(nonMover, 0, 0, 0, 0, pool.Handle(start+1))
	if err != nil {
		t.Fatalf("create building: %v", err)
	}
	h3, err := w.CreateWithForcedSlot(mover, 0, 0, 0, 0, pool.Handle(start+2))
	if err != nil {
		t.Fatalf("create third-slot mover: %v", err)
	}
	s.EnsureUnit(w.Unit(h1))
	s.EnsureUnit(w.Unit(h2))
	s.EnsureUnit(w.Unit(h3))
	// Submit in reverse physical order. The initial cursor is the first slot,
	// so the first poll visits the building in the second slot.
	s.SubmitMove(h3, 0, path.Cell{X: 3}, path.Cell{X: 9})
	s.SubmitMove(h1, 0, path.Cell{X: 1}, path.Cell{X: 7})
	p := s.pathProvider
	p.SetPathTick(59)
	for i, want := range []path.PollResult{path.PollVisited, path.PollVisited, path.PollNoUnit, path.PollVisited} {
		if _, got := p.Poll(0); got != want {
			t.Fatalf("tick-59 visit %d = %d, want %d", i, got, want)
		}
	}
	if handleRow(s.Routes, h1).LastRequestTick != 0 || handleRow(s.Routes, h3).LastRequestTick != 0 {
		t.Fatal("throttled physical visit stamped a request")
	}
	p.SetPathTick(60)
	if _, result := p.Poll(0); result != path.PollVisited {
		t.Fatalf("building slot result = %d, want visited", result)
	}
	got, result := p.Poll(0)
	if result != path.PollRequest || got.Unit != h3 || handleRow(s.Routes, h3).LastRequestTick != 60 {
		t.Fatalf("third physical visit = %#v/%d, timestamp=%d; want slot %d request", got, result, handleRow(s.Routes, h3).LastRequestTick, h3)
	}
	if _, result = p.Poll(0); result != path.PollNoUnit {
		t.Fatalf("hole result = %d, want no-unit", result)
	}
	got, result = p.Poll(0)
	if result != path.PollRequest || got.Unit != h1 || handleRow(s.Routes, h1).LastRequestTick != 60 {
		t.Fatalf("wrapped first-slot visit = %#v/%d, timestamp=%d; want slot %d request", got, result, handleRow(s.Routes, h1).LastRequestTick, h1)
	}
	// Advance past the building, consumed third-slot request and hole.
	for i := 0; i < 3; i++ {
		if _, result = p.Poll(0); result == path.PollRequest {
			t.Fatal("consumed request was admitted twice")
		}
	}
	// One more visit wraps to the first slot. Cancellation and replacement leave
	// no FIFO residue that could select the old request before this slot visit.
	s.SubmitMove(h1, 0, path.Cell{X: 1}, path.Cell{X: 11})
	if !s.CancelPathRequest(h1) {
		t.Fatal("cancel staged replacement")
	}
	s.SubmitMove(h1, 0, path.Cell{X: 1}, path.Cell{X: 13})
	p.SetPathTick(120)
	got, result = p.Poll(0)
	if result != path.PollRequest || got.Unit != h1 {
		t.Fatalf("wrapped replacement = %#v/%d, want replacement for slot %d", got, result, h1)
	}
	if trace := path.DescribeGoal(got.Goal); trace.Center.X != 13 {
		t.Fatalf("replacement goal = %+v, want X=13", trace)
	}
}

// TestPathAdmissionConsumesTheFixedExtraCharge uses an immediately-complete
// search so the cursor movement isolates the admission's one poll plus 100
// additional scheduler steps [04 R-PATH-01 §6].
func TestPathAdmissionConsumesTheFixedExtraCharge(t *testing.T) {
	s := NewSystem(syntheticTerrainForIntegrate(), wiringProfile, NewOccupancyGrid())
	w := newMovementFixtureWorld(1400)
	s.BindWorld(w)
	start, _, ok := w.SliceForPlayer(0)
	if !ok {
		t.Fatal("missing player-0 slice")
	}
	h, err := w.CreateWithForcedSlot(wiringDef(), 0, 0, 0, 0, pool.Handle(start+1))
	if err != nil {
		t.Fatalf("create mover: %v", err)
	}
	s.EnsureUnit(w.Unit(h))
	s.SubmitMove(h, 0, path.Cell{}, path.Cell{X: 1})
	s.Scheduler.SetSearch(func(path.Request, int32, int) path.WorkResult { return path.WorkResult{Done: true} })
	s.Scheduler.Tick(60)
	// 1333 allowance: first visit costs 1, admission costs 100, then 1232
	// no-unit physical visits bring the accumulator to zero.
	if got, want := s.pathProvider.cursor[0], start+1233; got != want {
		t.Fatalf("cursor after fixed admission charge = %d, want %d", got, want)
	}
}

// TestPathProviderBindWorldKeepsCursorForTheSamePool verifies that the
// per-tick session binding does not restart the physical traversal, while a
// replacement pool does [04 R-PATH-01 §6].
func TestPathProviderBindWorldKeepsCursorForTheSamePool(t *testing.T) {
	s := NewSystem(syntheticTerrainForIntegrate(), wiringProfile, NewOccupancyGrid())
	w := newMovementFixtureWorld(3)
	s.BindWorld(w)
	start, _, ok := w.SliceForPlayer(0)
	if !ok {
		t.Fatal("missing player-0 slice")
	}
	h1, err := w.CreateWithForcedSlot(wiringDef(), 0, 0, 0, 0, pool.Handle(start))
	if err != nil {
		t.Fatalf("create first mover: %v", err)
	}
	h2, err := w.CreateWithForcedSlot(wiringDef(), 0, 0, 0, 0, pool.Handle(start+1))
	if err != nil {
		t.Fatalf("create second mover: %v", err)
	}
	s.EnsureUnit(w.Unit(h1))
	s.EnsureUnit(w.Unit(h2))
	s.SubmitMove(h1, 0, path.Cell{X: 1}, path.Cell{X: 2})
	s.SubmitMove(h2, 0, path.Cell{X: 3}, path.Cell{X: 4})
	p := s.pathProvider
	p.SetPathTick(60)
	if got, result := p.Poll(0); result != path.PollRequest || got.Unit != h2 {
		t.Fatalf("first physical visit = %#v/%d, want slot %d request", got, result, h2)
	}
	s.BindWorld(w)
	if _, result := p.Poll(0); result != path.PollNoUnit {
		t.Fatalf("same-world bind visit = %d, want the third-slot hole", result)
	}
	if got, result := p.Poll(0); result != path.PollRequest || got.Unit != h1 {
		t.Fatalf("wrapped same-world visit = %#v/%d, want slot %d request", got, result, h1)
	}

	newWorld := newMovementFixtureWorld(3)
	newStart, _, ok := newWorld.SliceForPlayer(0)
	if !ok {
		t.Fatal("missing replacement player-0 slice")
	}
	s.BindWorld(newWorld)
	if _, result := p.Poll(0); result != path.PollNoUnit {
		t.Fatalf("replacement-world first visit = %d, want no-unit", result)
	}
	if got := p.cursor[0]; got != newStart+1 {
		t.Fatalf("replacement-world cursor = %d, want reset second slot %d", got, newStart+1)
	}
}
