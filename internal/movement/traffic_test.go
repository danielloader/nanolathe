package movement

import (
	"testing"

	"github.com/nanolathe-gg/nanolathe/internal/content"
	"github.com/nanolathe-gg/nanolathe/internal/orders"
	"github.com/nanolathe-gg/nanolathe/internal/path"
	"github.com/nanolathe-gg/nanolathe/internal/pool"
	"github.com/nanolathe-gg/nanolathe/internal/sim/numeric"
	"github.com/nanolathe-gg/nanolathe/internal/units"
	"github.com/nanolathe-gg/nanolathe/internal/world"
)

// trafficRules is the laboratory's baseline without its overlap policies and
// with one traffic policy: what a laboratory rule set binds
// (docs/PATHFINDING_LAB.md). The tests below are of the mechanisms, one at a
// time; modern_traffic_test.go holds Modern's contract.
type trafficRules struct {
	OverlapRules
	traffic Traffic
}

func (r *trafficRules) Traffic(*System) Traffic             { return r.traffic }
func (*trafficRules) AlliedPassThrough(*System) bool        { return false }
func (*trafficRules) JamRelease(*System) (uint16, uint32)   { return 0, 0 }
func (*trafficRules) PocketRelease(*System) (int32, uint32) { return 0, 0 }

// Strict 3.1, Community 3.9 and the laboratory's baseline answer retail's
// traffic: none. Modern's answer is locked in modern_traffic_test.go.
func TestTrafficAnswers(t *testing.T) {
	for _, r := range []Rules{StrictRules{}, CommunityRules{}, &OverlapRules{}} {
		if got := r.Traffic(nil); got != (Traffic{}) {
			t.Fatalf("%T answers %+v, want the zero value", r, got)
		}
	}
	if got := (&System{}).rules().Traffic(nil); got != (Traffic{}) {
		t.Fatalf("an unbound System answers %+v, want the zero value", got)
	}
}

// trafficFixture puts a mover in row 10 heading east for cell 17 along a
// line, and a friendly unit at rest in the same row at cell 8, its centre
// offset world units off that line: heading east, the right is the side z
// grows to. The stepper re-publishes the mover's route from where it stands
// each tick, the fixture having no scheduler, and runs one visit of the
// mover.
func trafficFixture(t *testing.T, tr Traffic, offset int64) (sys *System, w *units.World, a, b pool.Handle, step func(tick uint32) StepResult) {
	t.Helper()
	return trafficFixtureUnder(t, &trafficRules{traffic: tr}, offset)
}

// trafficFixtureUnder is trafficFixture under a rule set's own answers.
func trafficFixtureUnder(t *testing.T, rules Rules, offset int64) (sys *System, w *units.World, a, b pool.Handle, step func(tick uint32) StepResult) {
	t.Helper()
	profile := Profile{FootPrintX: 1, FootPrintZ: 1, MaxWaterDepth: 12, MinWaterDepth: -10000, MaxSlope: 255, MaxWaterSlope: 255}
	sys = NewSystem(syntheticTerrainForIntegrate(), profile, NewOccupancyGrid())
	sys.Rules = rules
	w = newMovementFixtureWorld(4)
	def := setScratchMovement(&content.UnitDef{
		UnitName: "traffic-test", FootprintX: 1, FootprintZ: 1, BMCode: 1, CanMove: true,
		MaxVelocity: 2 << 16, Acceleration: 1 << 15, BrakeRate: 1 << 15, TurnRate: 0x800,
	}, profile)
	// The line lies six world units into the row, so that the friend may
	// stand to either side of it and still in the row.
	row := world.CellToWorld(10) + 6<<16
	var err error
	if b, err = w.Create(def, 0, world.CellToWorld(8), 0, row+numeric.Fixed(offset<<16)); err != nil {
		t.Fatal(err)
	}
	if a, err = w.Create(def, 0, world.CellToWorld(3), 0, row); err != nil {
		t.Fatal(err)
	}
	sys.BindWorld(w)
	for _, h := range []pool.Handle{b, a} {
		u := w.Unit(h)
		u.Move.Heading = 0xC000 // east
		u.Flags |= 1 << units.StandingMoveShift
		sys.EnsureUnit(u)
		handleRow(sys.Collisions, h).Heading = 0xC000
		handleRow(sys.Steers, h).Heading = 0xC000
		orders.QueueForUnit(u).SetBinding(&orders.QueueBinding{Lookup: w.Unit, World: &orders.WorldQueryAdapter{}})
	}
	end := world.CellToWorld(17)
	q := orders.QueueOfUnit(w.Unit(a))
	q.Push(orders.Lookup("Move_Ground"), orders.Node{Owner: a, GoalX: end, GoalZ: row, GoalSupplied: true})
	head := q.Head()
	sys.InstallPointGoal(orders.PointGoalRequest{Owner: head.Owner, Node: head, X: head.GoalX, Z: head.GoalZ, Radius: 4})
	setHandleRow(&sys.activeOrders, a, &activeMove{order: head, token: 7})
	rowZ := int32(row.Raw() >> 16)
	step = func(tick uint32) StepResult {
		ax := handleRow(sys.Collisions, a).X >> 16
		handleRow(sys.Routes, a).PublishAtRevision([]Point{{X: ax, Z: rowZ}, {X: int32(end.Raw() >> 16), Z: rowZ}}, sys.staticObstacleRevision())
		sys.BeginTick(tick)
		res := sys.StepUnit(a, tick)
		sys.EndTick(tick)
		return res
	}
	return sys, w, a, b, step
}

// A mover under Sidestep turns past a friend at rest on its line before it
// reaches it, on the side the friend's centre is not, and is never refused;
// without the policy it runs into the friend and stays behind it.
func TestSidestepGoesRoundAParkedFriend(t *testing.T) {
	for _, tc := range []struct {
		name    string
		traffic Traffic
		offset  int64 // the friend's centre off the mover's line, in world units; positive is to the right
		passes  bool
		side    int8
	}{
		{"no policy", Traffic{}, 0, false, 0},
		{"dead ahead is passed on the right", Traffic{Sidestep: true, NoBrake: true}, 0, true, -1},
		{"a friend to the left is passed on the right", Traffic{Sidestep: true, NoBrake: true}, -5, true, -1},
		{"a friend to the right is passed on the left", Traffic{Sidestep: true, NoBrake: true}, 5, true, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sys, w, a, b, step := trafficFixture(t, tc.traffic, tc.offset)
			mover := w.Unit(a)
			refused, side := false, int8(0)
			for tick := uint32(1); tick <= 240; tick++ {
				if step(tick).Blocked {
					refused = true
				}
				if s := handleRow(sys.traffic, a).side; s != 0 && side == 0 {
					side = s
				}
			}
			past := mover.X > w.Unit(b).X+world.CellToWorld(1)
			if past != tc.passes {
				t.Fatalf("mover at x=%d, friend at x=%d: past=%v, want %v", mover.X>>16, w.Unit(b).X>>16, past, tc.passes)
			}
			if refused == tc.passes {
				t.Fatalf("refused=%v with passes=%v", refused, tc.passes)
			}
			if side != tc.side {
				t.Fatalf("passed on side %d, want %d", side, tc.side)
			}
			if w.Unit(b).X != world.CellToWorld(8) || handleRow(sys.Collisions, b).CachedAnchor != (Cell{X: 8, Z: 10}) {
				t.Fatal("the friend at rest was moved")
			}
		})
	}
}

// visitPilot counts what it is told.
type visitPilot struct{ NoPilot }

type visitCounts struct{ visited, grouped int }

func (visitPilot) state(s *System) *visitCounts {
	st, _ := s.PilotState.(*visitCounts)
	if st == nil {
		st = new(visitCounts)
		s.PilotState = st
	}
	return st
}

func (p visitPilot) Visit(s *System, v *Visit) {
	if v.Unit != nil && v.Coll != nil && v.Head != nil {
		p.state(s).visited++
	}
}

func (p visitPilot) GroupOrdered(s *System, _ uint8, members []pool.Handle, _, _ numeric.Fixed, _ uint32) {
	p.state(s).grouped += len(members)
}

// A pilot is told of every visit of a mover with an order, with the unit,
// its collision record and its order, and a group order reaches it only when
// it names two units or more.
func TestPilotIsToldOfVisitsAndGroupOrders(t *testing.T) {
	sys, _, a, b, step := trafficFixture(t, Traffic{Pilot: visitPilot{}}, 0)
	for tick := uint32(1); tick <= 40; tick++ {
		step(tick)
	}
	st := visitPilot{}.state(sys)
	if st.visited != 40 {
		t.Fatalf("told of %d visits of 40", st.visited)
	}
	sys.NoteGroupOrder(0, []pool.Handle{a}, 0, 0, 41)
	sys.NoteGroupOrder(0, []pool.Handle{a, b}, 0, 0, 41)
	if st.grouped != 2 {
		t.Fatalf("group orders named %d members, want 2", st.grouped)
	}
}

// scaleSpy is a kernel that notes the heuristic weight of the searches it is
// asked to open and finds nothing.
type scaleSpy struct{ scales *[]int32 }

func (k scaleSpy) NewSession(cfg path.SearchConfig) path.Search {
	*k.scales = append(*k.scales, cfg.Scale)
	return path.NewSession(cfg)
}

// A search weighs its heuristic by BusyScale while route searching is busy —
// more work a tick than BusyWork, smoothed over eight ticks — and by
// HeuristicScale otherwise.
func TestBusyScaleWeighsSearchesUnderLoad(t *testing.T) {
	tr := Traffic{HeuristicScale: 0x18000, BusyScale: 0x30000, BusyWork: 3000}
	sys, _, a, _, _ := trafficFixture(t, tr, 0)
	var scales []int32
	sys.Kernel = scaleSpy{&scales}
	request := path.Request{Unit: a, Start: path.Cell{X: 3, Z: 10}, Goal: path.PointGoal(path.Cell{X: 17, Z: 10}, 0)}
	for _, tc := range []struct {
		work int64
		want int32
	}{{0, 0x18000}, {3000, 0x18000}, {3001, 0x30000}} {
		sys.BeginTick(1)
		sys.workSmooth = tc.work
		sys.dropPathSession(int(a))
		sys.searchFunc(request, 0x90000, 100)
		if got := scales[len(scales)-1]; got != tc.want {
			t.Errorf("with %d units of work a tick the search weighs %#x, want %#x", tc.work, got, tc.want)
		}
	}
}

// A mover's sidestep looks three cells ahead, and a route that turns at a
// point two cells ahead may have a wall beyond that point: without ToWaypoint
// the mover turns away from the wall before it has reached the point, and
// with it the mover keeps the heading it wants.
func TestSidestepLooksNoFartherThanTheWaypoint(t *testing.T) {
	for _, tc := range []struct {
		name    string
		traffic Traffic
		steers  bool
	}{
		{"sidestep", Traffic{Sidestep: true, NoBrake: true}, true},
		{"to the waypoint", Traffic{Sidestep: true, NoBrake: true, ToWaypoint: true}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sys, w, a, b, _ := trafficFixture(t, tc.traffic, 0)
			sys.ForgetUnit(b)
			// A wall across the mover's row three cells ahead of it, and
			// the route turning south one cell before the wall.
			for z := int32(8); z <= 12; z++ {
				sys.Terrain.PlotAt(6, z).SetFeature(world.PlotFeatureVoid)
			}
			u := w.Unit(a)
			coll := handleRow(sys.Collisions, a)
			row := int32(u.Z.Raw() >> 16)
			x := int32(u.X.Raw() >> 16)
			route := handleRow(sys.Routes, a)
			route.PublishAtRevision([]Point{{X: x, Z: row}, {X: x + 32, Z: row}, {X: x + 32, Z: row + 200}}, sys.staticObstacleRevision())
			sys.BeginTick(1)
			coll.X, coll.Z, coll.Heading = int32(u.X.Raw()), int32(u.Z.Raw()), 0xC000
			got, _ := sys.steerAround(u, coll, sys.ProfileFor(a), route, 0xC000, 1, tc.traffic)
			if steers := got != 0xC000; steers != tc.steers {
				t.Fatalf("the mover wants heading %#x for a wanted %#x: steers=%v, want %v", got, 0xC000, steers, tc.steers)
			}
		})
	}
}

// A waypoint a mover stands beside and past is taken from it only while it
// is steering round something: a side it chose long ago says nothing of the
// point ahead, and the first turn of a new route is the way round whatever
// holds the unit. A unit whose last step was refused is passing nothing.
func TestWaypointBesideIsTakenFromAUnitGoingRound(t *testing.T) {
	for _, tc := range []struct {
		name     string
		traffic  Traffic
		steering bool
		until    uint32
		refused  bool
		taken    bool
	}{
		{"steering now", Traffic{Sidestep: true}, true, 40, false, true},
		{"a side chosen long ago", Traffic{Sidestep: true}, true, 5, false, false},
		{"not steering on this visit", Traffic{Sidestep: true}, false, 40, false, false},
		{"steering now, its last step refused", Traffic{Sidestep: true}, true, 40, true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sys, w, a, b, _ := trafficFixture(t, tc.traffic, 0)
			sys.ForgetUnit(b)
			u := w.Unit(a)
			x, row := int32(u.X.Raw()>>16), int32(u.Z.Raw()>>16)
			// The route's next point lies twenty world units north-west of
			// the mover, and the one after it far to the east: the mover is
			// beside the first and past it.
			route := handleRow(sys.Routes, a)
			route.PublishAtRevision([]Point{{X: x - 40, Z: row}, {X: x - 16, Z: row - 12}, {X: x + 160, Z: row - 12}}, sys.staticObstacleRevision())
			sys.BeginTick(20)
			coll := handleRow(sys.Collisions, a)
			coll.X, coll.Z, coll.Blocked = int32(u.X.Raw()), int32(u.Z.Raw()), tc.refused
			st := handleRow(sys.traffic, a)
			st.side, st.sideUntil, st.steering = 1, tc.until, tc.steering
			setHandleRow(&sys.traffic, a, st)
			sys.passWaypoint(u, route, 20)
			if taken := route.Count == 2; taken != tc.taken {
				t.Fatalf("the route holds %d points: taken=%v, want %v", route.Count, taken, tc.taken)
			}
		})
	}
}

// A mover whose last step was refused keeps the way round it chose on a
// visit its own way looks clear on, while that way round is clear: without
// KeepRound it wants its own way again, and a unit that turns slowly where
// it stands turns back and forth between the two.
func TestRefusedMoverKeepsItsWayRound(t *testing.T) {
	for _, tc := range []struct {
		name    string
		traffic Traffic
		refused bool
		keeps   bool
	}{
		{"sidestep", Traffic{Sidestep: true, NoBrake: true}, true, false},
		{"keep round, refused", Traffic{Sidestep: true, NoBrake: true, KeepRound: true}, true, true},
		{"keep round, moving", Traffic{Sidestep: true, NoBrake: true, KeepRound: true}, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sys, w, a, b, _ := trafficFixture(t, tc.traffic, 0)
			sys.ForgetUnit(b)
			u := w.Unit(a)
			x, row := int32(u.X.Raw()>>16), int32(u.Z.Raw()>>16)
			route := handleRow(sys.Routes, a)
			route.PublishAtRevision([]Point{{X: x, Z: row}, {X: x + 200, Z: row}}, sys.staticObstacleRevision())
			sys.BeginTick(10)
			coll := handleRow(sys.Collisions, a)
			coll.X, coll.Z, coll.Heading, coll.Blocked = int32(u.X.Raw()), int32(u.Z.Raw()), 0xC000, tc.refused
			// It chose a way round to its left six ticks ago.
			const round = 0xC000 + 2*steerStep
			st := handleRow(sys.traffic, a)
			st.side, st.sideUntil, st.round, st.steering = 1, 28, round, true
			setHandleRow(&sys.traffic, a, st)
			got, _ := sys.steerAround(u, coll, sys.ProfileFor(a), route, 0xC000, 10, tc.traffic)
			if keeps := got == round; keeps != tc.keeps {
				t.Fatalf("the mover wants heading %#x: keeps its way round=%v, want %v", got, keeps, tc.keeps)
			}
			if tc.keeps && handleRow(sys.traffic, a).sideUntil != 10+24 {
				t.Fatalf("the choice is good until tick %d, want %d", handleRow(sys.traffic, a).sideUntil, 10+24)
			}
		})
	}
}
