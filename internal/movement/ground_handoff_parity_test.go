package movement

import (
	"fmt"
	"testing"

	"github.com/nanolathe-gg/nanolathe/internal/orders"
	"github.com/nanolathe-gg/nanolathe/internal/path"
	"github.com/nanolathe-gg/nanolathe/internal/units"
	"github.com/nanolathe-gg/nanolathe/internal/world"
)

// Every installer completes the acceptance gates before returning to its
// handler, using the authoritative queue tick even outside BeginTick
// [04 R-PATH-01 §8]. The terminal-cell gate retains the previous three points.
func TestGroundHandoffAcceptsDuringEveryInstaller(t *testing.T) {
	for _, tc := range []struct {
		name     string
		terminal Point
		install  func(*System, *units.Unit, *orders.Node) bool
	}{
		{"point", Point{X: 152, Z: 152}, func(s *System, u *units.Unit, n *orders.Node) bool {
			return s.InstallPointGoal(orders.PointGoalRequest{Owner: u.Handle, Node: n, X: world.CellToWorld(9), Z: world.CellToWorld(9)})
		}},
		{"annulus", Point{X: 200, Z: 152}, func(s *System, u *units.Unit, n *orders.Node) bool {
			return s.InstallAnnulusGoal(orders.AnnulusGoalRequest{Owner: u.Handle, Node: n, X: world.CellToWorld(9), Z: world.CellToWorld(9), InnerRadius: 32, OuterRadius: 64})
		}},
		{"rectangle", Point{X: 136, Z: 168}, func(s *System, u *units.Unit, n *orders.Node) bool {
			return s.InstallRectangleGoal(orders.RectangleGoalRequest{Owner: u.Handle, Node: n, CellX: 9, CellZ: 9, Width: 2, Depth: 2})
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, w, h := releaseFixture(t, wiringDef(), 2)
			u := w.Unit(h)
			n := &orders.Node{ID: orders.Lookup("Move_Ground"), Owner: h}
			q := orders.NewQueueWith([]*orders.Node{n}, nil)
			orders.BindQueue(u, q)
			tick := uint32(110)
			q.SetBinding(orders.NewQueueBinding(orders.QueueBindingConfig{CurrentTick: func() uint32 { return tick }}))
			s.tick = 1 // deliberately stale movement-transaction tick
			route := handleRow(s.Routes, h)
			route.Count, route.LastRequestTick = 3, 101
			route.Points[0], route.Points[1], route.Points[2] = Point{X: 32, Z: 32}, Point{X: 80, Z: 96}, tc.terminal
			points := route.Points
			s.SubmitMove(h, 0, path.Cell{}, path.Cell{X: 18, Z: 18})
			if !tc.install(s, u, n) {
				t.Fatal("installer refused")
			}
			if s.HasPathRequest(h) || !route.Active || route.WantsRepath || !route.Dirty || route.Count != 3 || route.Points != points {
				t.Fatalf("handoff did not cancel and accept before returning: route=%+v pending=%v", route, s.HasPathRequest(h))
			}
			if route.LastRequestTick != 101 {
				t.Fatalf("nine-tick-old stamp=%d, want 101 from queue tick 110", route.LastRequestTick)
			}
			before := *route
			tick = 111
			if !s.ActivateMove(u, n) || *route != before {
				t.Fatal("activating an already installed goal repeated acceptance or the age reset")
			}
			if s.ActivateMove(u, n) || *route != before {
				t.Fatal("repeated activation changed the installed goal")
			}
		})
	}
}

// The first handoff rejects a retained route. The following handoff would
// accept that original route, but must see the two synthetic points the first
// one left instead [04 R-PATH-01 §8]. A final-head-only handoff fails this case.
func TestGroundHandoffCascadeRetainsIntermediateMutations(t *testing.T) {
	s, w, h := releaseFixture(t, wiringDef(), 2)
	u := w.Unit(h)
	a := &orders.Node{ID: orders.Lookup("Move_Ground"), Owner: h}
	b := &orders.Node{ID: orders.Lookup("Move_Ground"), Owner: h}
	orders.BindQueue(u, orders.NewQueueWith([]*orders.Node{a, b}, nil))
	route := handleRow(s.Routes, h)
	route.Count = 3
	route.Points[0], route.Points[1], route.Points[2] = Point{X: 32, Z: 32}, Point{X: 80, Z: 80}, Point{X: 152, Z: 152}
	if !s.InstallPointGoal(orders.PointGoalRequest{Owner: h, Node: a, X: world.CellToWorld(1), Z: world.CellToWorld(1)}) {
		t.Fatal("first handoff refused")
	}
	if route.Count != 2 || route.Points[1] != (Point{X: 24, Z: 24}) || !route.Active || !route.WantsRepath {
		t.Fatalf("first handoff did not leave its synthetic route: %+v", route)
	}
	s.SubmitMove(h, 0, path.Cell{}, path.Cell{X: 18, Z: 18})
	if !s.InstallPointGoal(orders.PointGoalRequest{Owner: h, Node: b, X: world.CellToWorld(9), Z: world.CellToWorld(9)}) {
		t.Fatal("second handoff refused")
	}
	if route.Count != 2 || route.Points[1] != (Point{X: 152, Z: 152}) || !route.Active || !route.WantsRepath || s.HasPathRequest(h) {
		t.Fatalf("second handoff missed the preceding route mutation: %+v", route)
	}
	if a.Satisfied&goalReleasedPending == 0 || b.Satisfied&goalPendingMask != 0 {
		t.Fatalf("handoff outcomes a=%#x b=%#x", a.Satisfied, b.Satisfied)
	}
}

// The synthetic fallback tests the current head's completion flag, which need
// not belong to the record receiving the payload [04 R-PATH-01 §8].
func TestGroundHandoffSyntheticGateUsesCurrentHead(t *testing.T) {
	for _, headRetiring := range []bool{false, true} {
		t.Run(fmt.Sprintf("head_retiring_%v", headRetiring), func(t *testing.T) {
			s, w, h := releaseFixture(t, wiringDef(), 2)
			u := w.Unit(h)
			head := &orders.Node{ID: orders.Lookup("Paralyze"), Owner: h}
			owner := &orders.Node{ID: orders.Lookup("Move_Ground"), Owner: h}
			if headRetiring {
				head.Flags = orders.FlagRetryMark
			} else {
				owner.Flags = orders.FlagRetryMark
			}
			orders.BindQueue(u, orders.NewQueueWith([]*orders.Node{head, owner}, nil))
			s.InstallPointGoal(orders.PointGoalRequest{Owner: h, Node: owner, X: world.CellToWorld(9), Z: world.CellToWorld(9)})
			route := handleRow(s.Routes, h)
			if route.Active == headRetiring || route.WantsRepath == false || (headRetiring && route.Count != 0) || (!headRetiring && route.Count != 2) {
				t.Fatalf("fallback followed the installed owner instead of the head: %+v", route)
			}
		})
	}
}

// Null handoff takes the same unsigned, inclusive ten-tick epilogue as every
// non-null handoff, including the subtraction wrap before tick ten
// [04 R-PATH-01 §8]. It retains the points/count while clearing both flags.
func TestGroundNullHandoffRequestAgeAndDirty(t *testing.T) {
	for _, tc := range []struct{ tick, stamp, want uint32 }{
		{0, 0, 0}, {9, 5, 0}, {10, 1, 1}, {100, 90, 0}, {100, 91, 91},
	} {
		t.Run(fmt.Sprintf("tick_%d_stamp_%d", tc.tick, tc.stamp), func(t *testing.T) {
			s, w, h := releaseFixture(t, wiringDef(), 2)
			u := w.Unit(h)
			n := &orders.Node{ID: orders.Lookup("Move_Ground"), Owner: h}
			q := orders.NewQueueWith([]*orders.Node{n}, nil)
			orders.BindQueue(u, q)
			q.SetBinding(orders.NewQueueBinding(orders.QueueBindingConfig{CurrentTick: func() uint32 { return tc.tick }}))
			s.InstallPointGoal(orders.PointGoalRequest{Owner: h, Node: n, X: world.CellToWorld(9), Z: world.CellToWorld(9)})
			route := handleRow(s.Routes, h)
			route.LastRequestTick, route.Active, route.WantsRepath, route.Dirty = tc.stamp, true, true, false
			points, count := route.Points, route.Count
			if !s.ReleaseGoalPayload(n) {
				t.Fatal("null handoff refused")
			}
			if route.LastRequestTick != tc.want || !route.Dirty || route.Active || route.WantsRepath || route.Points != points || route.Count != count {
				t.Fatalf("null handoff=%+v, want stamp %d and unchanged points", route, tc.want)
			}
		})
	}
}

// The follower asks the bound object even behind a fresh, goal-less head; it
// raises arrival/release on that object's owner before steering
// [04 R-ORD-01 §9][04 R-MOV-03 §2].
func TestGroundBoundOwnerArrivesBehindGoalLessHead(t *testing.T) {
	s, w, h := releaseFixture(t, wiringDef(), 2)
	u := w.Unit(h)
	head := &orders.Node{ID: orders.Lookup("Paralyze"), Owner: h}
	owner := &orders.Node{ID: orders.Lookup("Move_Ground"), Owner: h}
	orders.BindQueue(u, orders.NewQueueWith([]*orders.Node{head, owner}, nil))
	s.InstallPointGoal(orders.PointGoalRequest{Owner: h, Node: owner, X: u.X, Z: u.Z})
	x, z := u.X, u.Z
	s.BeginTick(110)
	res := s.StepUnit(h, 110)
	s.EndTick(110)
	if !res.Arrived || owner.Satisfied&0xA0 != 0xA0 || head.Satisfied != 0 || s.HasBoundGroundGoal(h) || u.X != x || u.Z != z {
		t.Fatalf("arrival did not follow the bound owner: result=%+v owner=%#x head=%#x", res, owner.Satisfied, head.Satisfied)
	}
}

// Scheduler admission, empty publication and stale-token rejection follow the
// bound payload, independently of the queue head
// [04 R-PATH-01 §4][04 R-PATH-01 §7][04 R-PATH-01 §8].
func TestGroundBoundOwnerOwnsSchedulerAndPublication(t *testing.T) {
	s, w, h := releaseFixture(t, wiringDef(), 2)
	s.ConfigurePath(1, 10, func(p int) bool { return p == 0 })
	u := w.Unit(h)
	head := &orders.Node{ID: orders.Lookup("Paralyze"), Owner: h}
	owner := &orders.Node{ID: orders.Lookup("Move_Ground"), Owner: h}
	orders.BindQueue(u, orders.NewQueueWith([]*orders.Node{head, owner}, nil))
	s.InstallPointGoal(orders.PointGoalRequest{Owner: h, Node: owner, X: world.CellToWorld(9), Z: world.CellToWorld(9)})
	route := handleRow(s.Routes, h)
	s.serviceGroundFollower(u, head, route, 100)
	s.pathProvider.SetPathTick(100)
	var request path.Request
	for i := 0; i < 10; i++ {
		r, result := s.pathProvider.Poll(0)
		if result == path.PollRequest {
			request = r
			break
		}
	}
	if request.Activation == 0 || request.Goal != s.moveGoalPayload(h, owner) || route.LastRequestTick != 100 {
		t.Fatalf("scheduler did not poll the bound owner's goal: request=%+v stamp=%d", request, route.LastRequestTick)
	}
	s.publishFunc(request, nil, path.StatusRejected)
	if owner.Satisfied&0x40 == 0 || head.Satisfied != 0 || route.Active || route.WantsRepath {
		t.Fatal("empty publication did not notify only the bound owner")
	}
	s.InstallPointGoal(orders.PointGoalRequest{Owner: h, Node: head, X: world.CellToWorld(12), Z: world.CellToWorld(12)})
	before := *route
	s.publishFunc(request, []path.Point{{X: 99, Z: 99}}, 0)
	if *route != before || head.Satisfied != 0 {
		t.Fatal("a late publication survived replacement of its bound object")
	}
}

// Removing the currently bound record does not resurrect another record's
// retained object. Rebinding those objects belongs to save reconstruction
// [04 R-ORD-01 §9][04 R-PATH-01 §8].
func TestGroundRemovalDoesNotRebindRetainedSuccessor(t *testing.T) {
	s, w, h := releaseFixture(t, wiringDef(), 2)
	u := w.Unit(h)
	a := &orders.Node{ID: orders.Lookup("Move_Ground"), Owner: h}
	b := &orders.Node{ID: orders.Lookup("Move_Ground"), Owner: h}
	q := orders.NewQueueWith([]*orders.Node{a, b}, nil)
	orders.BindQueue(u, q)
	q.SetBinding(&orders.QueueBinding{Movement: orders.NewMovementGoalAdapter(orders.MovementGoalAdapterConfig{Destroy: s.ReleaseGoal})})
	s.InstallPointGoal(orders.PointGoalRequest{Owner: h, Node: b, X: world.CellToWorld(9), Z: world.CellToWorld(9)})
	s.InstallPointGoal(orders.PointGoalRequest{Owner: h, Node: a, X: world.CellToWorld(12), Z: world.CellToWorld(12)})
	q.RemoveHead()
	if q.Head() != b || s.HasBoundGroundGoal(h) || s.ActivateMove(u, b) || s.HasPathRequest(h) {
		t.Fatal("removal implicitly rebound the successor's retained object")
	}
}
