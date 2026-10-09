package movement

import (
	"testing"

	"github.com/nanolathe-gg/nanolathe/internal/content"
	"github.com/nanolathe-gg/nanolathe/internal/orders"
	"github.com/nanolathe-gg/nanolathe/internal/pool"
	"github.com/nanolathe-gg/nanolathe/internal/sim/numeric"
	"github.com/nanolathe-gg/nanolathe/internal/sim/rng"
	"github.com/nanolathe-gg/nanolathe/internal/units"
	"github.com/nanolathe-gg/nanolathe/internal/world"
)

// Nanolathe Modern policy: docs/DESIGN_MOVEMENT_PATH.md "Modern traffic".
// These tests hold the contract Modern keeps and the bypass Strict 3.1
// keeps; the mechanisms are tested one at a time in traffic_test.go,
// pilot_arrive_test.go and pilot_claims_test.go.

// Modern's answer, value for value. The numbers are written out so that a
// change of tuning is a deliberate edit here and in the design document.
func TestModernTrafficAnswer(t *testing.T) {
	want := Traffic{
		Sidestep: true, PassBehind: true, NoBrake: true, ClearOnly: true, NoOvertake: true, ToWaypoint: true, KeepRound: true,
		LegsThroughMovers: true,
		HeuristicScale:    0x18000, BusyScale: 0x30000, BusyWork: 3000,
		Through: 2, ThroughAfter: 150,
		Pilot: Pilots{
			ClaimsPilot{Rank: true, Per: 4, Oncoming: 24},
			ArrivePilot{Places: true, Exchange: true, Stuck: 20, StuckParked: true, Sealed: 150},
		},
	}
	if got := (&ModernRules{}).Traffic(nil); got != want {
		t.Fatalf("Modern answers\n%+v, want\n%+v", got, want)
	}
	s := &System{Rules: &ModernRules{}}
	var sink Traffic
	if allocs := testing.AllocsPerRun(200, func() { sink = s.rules().Traffic(s) }); allocs != 0 {
		t.Fatalf("the answer allocated %v times", allocs)
	}
	_ = sink
}

// trafficStateless reports whether the system holds nothing a traffic policy
// would have written.
func trafficStateless(s *System) bool {
	for _, st := range s.traffic {
		if st != (trafficState{}) {
			return false
		}
	}
	stats := s.LabStats()
	return s.PilotState == nil && stats.Visits == 0 && stats.Probes == 0 && stats.Anchors == 0
}

// A friend at rest on a mover's line. Modern turns past it before it reaches
// it, is never refused a step and never shares a cell with it. Strict 3.1,
// Community 3.9 and an unbound System run into it and stay behind it, as
// retail does [04 R-COLL-01 §1]; they keep no traffic state and draw from
// neither random stream, and neither does Modern draw.
func TestModernGoesRoundAParkedFriendAndStrictRunsIntoIt(t *testing.T) {
	for _, tc := range []struct {
		name   string
		rules  Rules
		passes bool
	}{
		{"unbound", nil, false},
		{"strict", StrictRules{}, false},
		{"community", CommunityRules{}, false},
		{"modern", &ModernRules{}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rng.SeedGlobal(1, 1)
			sim, crt := *rng.Global.Sim, *rng.Global.Crt
			sys, w, a, b, step := trafficFixtureUnder(t, tc.rules, 0)
			mover, friend := w.Unit(a), w.Unit(b)
			refused := false
			for tick := uint32(1); tick <= 240; tick++ {
				if step(tick).Blocked {
					refused = true
				}
				if handleRow(sys.Collisions, a).CachedAnchor == handleRow(sys.Collisions, b).CachedAnchor {
					t.Fatalf("tick %d: the mover shares the friend's cell", tick)
				}
			}
			if past := mover.X > friend.X+world.CellToWorld(1); past != tc.passes {
				t.Fatalf("mover at x=%d, friend at x=%d: past=%v, want %v", mover.X>>16, friend.X>>16, past, tc.passes)
			}
			if refused == tc.passes {
				t.Fatalf("refused=%v with passes=%v", refused, tc.passes)
			}
			if friend.X != world.CellToWorld(8) || handleRow(sys.Collisions, b).CachedAnchor != (Cell{X: 8, Z: 10}) {
				t.Fatal("the friend at rest was moved")
			}
			if !tc.passes && !trafficStateless(sys) {
				t.Fatalf("%T kept traffic state", tc.rules)
			}
			if *rng.Global.Sim != sim || *rng.Global.Crt != crt {
				t.Fatalf("%T drew randomness", tc.rules)
			}
		})
	}
}

// headOnFixture puts two units of one owner in row 10, the first at cell 3
// bound east for cell 17 and the second at cell 14 bound west for cell 0.
// The stepper re-publishes each route from where its unit stands, the
// fixture having no scheduler, and visits the two in pool order.
func headOnFixture(t *testing.T, rules Rules) (sys *System, w *units.World, east, west pool.Handle, step func(tick uint32) (eastRefused, westRefused bool)) {
	t.Helper()
	profile := Profile{FootPrintX: 1, FootPrintZ: 1, MaxWaterDepth: 12, MinWaterDepth: -10000, MaxSlope: 255, MaxWaterSlope: 255}
	sys = NewSystem(syntheticTerrainForIntegrate(), profile, NewOccupancyGrid())
	sys.Rules = rules
	w = newMovementFixtureWorld(4)
	def := setScratchMovement(&content.UnitDef{
		UnitName: "head-on-test", FootprintX: 1, FootprintZ: 1, BMCode: 1, CanMove: true,
		MaxVelocity: 2 << 16, Acceleration: 1 << 15, BrakeRate: 1 << 15, TurnRate: 0x800,
	}, profile)
	row := world.CellToWorld(10) + 8<<16
	var err error
	if east, err = w.Create(def, 0, world.CellToWorld(3)+8<<16, 0, row); err != nil {
		t.Fatal(err)
	}
	if west, err = w.Create(def, 0, world.CellToWorld(14)+8<<16, 0, row); err != nil {
		t.Fatal(err)
	}
	sys.BindWorld(w)
	ends := map[pool.Handle]int32{east: 17*16 + 8, west: 8}
	for _, h := range []pool.Handle{east, west} {
		u := w.Unit(h)
		heading := uint16(0xC000) // east
		if h == west {
			heading = 0x4000
		}
		u.Move.Heading = heading
		u.Flags |= 1 << units.StandingMoveShift
		sys.EnsureUnit(u)
		handleRow(sys.Collisions, h).Heading = heading
		handleRow(sys.Steers, h).Heading, handleRow(sys.Steers, h).PendingHeading = heading, heading
		q := orders.QueueForUnit(u)
		q.SetBinding(orders.NewQueueBinding(orders.QueueBindingConfig{Lookup: w.Unit, World: &orders.WorldQueryAdapter{}}))
		q.Push(orders.Lookup("Move_Ground"), orders.Node{Owner: h, GoalX: numeric.Fixed(int64(ends[h]) << 16), GoalZ: row, GoalSupplied: true})
		head := q.Head()
		sys.InstallPointGoal(orders.PointGoalRequest{Owner: h, Node: head, X: head.GoalX, Z: head.GoalZ, Radius: 4})
		setHandleRow(&sys.activeOrders, h, &activeMove{order: head, token: 7})
	}
	rowZ := int32(row.Raw() >> 16)
	step = func(tick uint32) (bool, bool) {
		for _, h := range []pool.Handle{east, west} {
			c := handleRow(sys.Collisions, h)
			handleRow(sys.Routes, h).PublishAtRevision([]Point{{X: c.X >> 16, Z: c.Z >> 16}, {X: ends[h], Z: rowZ}}, sys.staticObstacleRevision())
		}
		sys.BeginTick(tick)
		a := sys.StepUnit(east, tick)
		b := sys.StepUnit(west, tick)
		sys.EndTick(tick)
		return a.Blocked, b.Blocked
	}
	return sys, w, east, west, step
}

// Two friendly movers that meet head-on. Under Modern each keeps to its
// right and they part, and no tick finds them in one cell: allied
// pass-through is retired. Under Strict 3.1 they refuse each other and
// neither gets by [04 R-COLL-01 §1][04 R-COLL-01 §2].
func TestModernHeadOnMoversPartWithoutSharingACell(t *testing.T) {
	for _, tc := range []struct {
		name  string
		rules Rules
		part  bool
	}{
		{"strict", StrictRules{}, false},
		{"community", CommunityRules{}, false},
		{"modern", &ModernRules{}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rng.SeedGlobal(1, 1)
			sim, crt := *rng.Global.Sim, *rng.Global.Crt
			sys, w, east, west, step := headOnFixture(t, tc.rules)
			for tick := uint32(1); tick <= 240; tick++ {
				step(tick)
				if handleRow(sys.Collisions, east).CachedAnchor == handleRow(sys.Collisions, west).CachedAnchor {
					t.Fatalf("tick %d: the two share a cell", tick)
				}
			}
			if parted := w.Unit(east).X > w.Unit(west).X+world.CellToWorld(2); parted != tc.part {
				t.Fatalf("eastbound at x=%d, westbound at x=%d: parted=%v, want %v", w.Unit(east).X>>16, w.Unit(west).X>>16, parted, tc.part)
			}
			if !tc.part && !trafficStateless(sys) {
				t.Fatalf("%T kept traffic state", tc.rules)
			}
			if *rng.Global.Sim != sim || *rng.Global.Crt != crt {
				t.Fatalf("%T drew randomness", tc.rules)
			}
		})
	}
}

// Strict 3.1 after a Modern session: what the traffic policy remembered
// before the switch is neither read nor written, so the mover that was going
// round the friend wants its route's own heading again and runs into it.
func TestStrictIgnoresTrafficStateAfterASwitch(t *testing.T) {
	sys, w, a, b, step := trafficFixtureUnder(t, &ModernRules{}, 0)
	tick := uint32(1)
	for ; tick <= 240 && handleRow(sys.traffic, a).side == 0; tick++ {
		step(tick)
	}
	before := handleRow(sys.traffic, a)
	if before.side == 0 || w.Unit(a).X > w.Unit(b).X {
		t.Fatalf("fixture: Modern had not begun to go round the friend by tick %d", tick)
	}
	sys.Rules = StrictRules{}
	pilot := sys.PilotState
	refused := false
	for end := tick + 240; tick < end; tick++ {
		if step(tick).Blocked {
			refused = true
		}
	}
	if !refused || w.Unit(a).X > w.Unit(b).X+world.CellToWorld(1) {
		t.Fatalf("Strict went round the friend: mover at x=%d, friend at x=%d, refused=%v", w.Unit(a).X>>16, w.Unit(b).X>>16, refused)
	}
	if handleRow(sys.traffic, a) != before || sys.PilotState != pilot {
		t.Fatal("Strict wrote traffic state")
	}
}

// A route published under Modern stays installed after a switch, but its
// corner is consumed with retail's five-world-unit radius [04 §7.3]. Retained
// sidestepping state must not discard a farther corner until Modern resumes.
func TestWaypointPassingStopsAfterARuleSwitch(t *testing.T) {
	for _, tc := range []struct {
		name  string
		rules Rules
	}{
		{"strict", StrictRules{}},
		{"community", CommunityRules{}},
		{"unbound", nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rng.SeedGlobal(1, 1)
			sys, w, h, _, step := trafficFixtureUnder(t, &ModernRules{}, 0)
			tick := uint32(1)
			for ; tick <= 240 && !handleRow(sys.traffic, h).steering; tick++ {
				step(tick)
			}
			before := handleRow(sys.traffic, h)
			if !before.steering || before.side == 0 {
				t.Fatal("fixture never began steering")
			}
			u := w.Unit(h)
			x, z := int32(u.X>>16), int32(u.Z>>16)
			route := handleRow(sys.Routes, h)
			route.Publish([]Point{{X: x - 80, Z: z - 12}, {X: x - 16, Z: z - 12}, {X: x + 100, Z: z - 12}})
			handleRow(sys.Collisions, h).Blocked = false
			head := orders.QueueOfUnit(u).Head()
			sim, crt := *rng.Global.Sim, *rng.Global.Crt
			sys.Rules = tc.rules
			sys.BeginTick(tick)
			sys.serviceGroundFollower(u, head, route, tick)
			if route.Count != 3 {
				t.Fatalf("a corner twenty world units away was pruned after the switch: %d points", route.Count)
			}
			if handleRow(sys.traffic, h) != before {
				t.Fatal("retained traffic state changed after the switch")
			}
			sys.Rules = &ModernRules{}
			sys.BeginTick(tick + 1)
			sys.serviceGroundFollower(u, head, route, tick+1)
			if route.Count != 2 {
				t.Fatal("Modern did not resume passing the corner beside the mover")
			}
			if *rng.Global.Sim != sim || *rng.Global.Crt != crt {
				t.Fatal("waypoint consumption drew randomness")
			}
		})
	}
}

// The rule object and its pilots are shared by every session of the process
// and hold nothing: two battles bound to one rules value, stepped turn and
// turn about, each go as a battle stepped alone does.
func TestModernTrafficKeepsItsStateOnTheSystem(t *testing.T) {
	rules := &ModernRules{}
	type pose struct {
		x, z    int64
		heading uint16
	}
	trace := func(n int) [][]pose {
		type battle struct {
			w    *units.World
			a    pool.Handle
			step func(uint32) StepResult
		}
		battles := make([]battle, n)
		for i := range battles {
			_, w, a, _, step := trafficFixtureUnder(t, rules, 0)
			battles[i] = battle{w, a, step}
		}
		out := make([][]pose, n)
		for tick := uint32(1); tick <= 160; tick++ {
			for i, b := range battles {
				b.step(tick)
				u := b.w.Unit(b.a)
				out[i] = append(out[i], pose{int64(u.X), int64(u.Z), u.Move.Heading})
			}
		}
		return out
	}
	alone := trace(1)[0]
	for i, together := range trace(2) {
		for tick := range alone {
			if together[tick] != alone[tick] {
				t.Fatalf("battle %d at tick %d stands at %+v, alone at %+v", i, tick+1, together[tick], alone[tick])
			}
		}
	}
}
