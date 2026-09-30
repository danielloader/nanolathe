package movement

import (
	"testing"

	"github.com/nanolathe-gg/nanolathe/internal/content"
	"github.com/nanolathe-gg/nanolathe/internal/orders"
	"github.com/nanolathe-gg/nanolathe/internal/path"
	"github.com/nanolathe-gg/nanolathe/internal/pool"
	"github.com/nanolathe-gg/nanolathe/internal/units"
	"github.com/nanolathe-gg/nanolathe/internal/world"
)

// throughToken is the activation the fixture's mover is bound under.
const throughToken = 7

// throughFixture puts a mover at cell (5, 10) with a plain move to cell
// (15, 10), and the eight friends of its owner round it that have stood
// there since tick zero. With going set the friends hold goals of their own.
// The ninth unit, at cell (12, 10), belongs to owner other.
func throughFixture(t *testing.T, tr Traffic, going bool, other uint8) (sys *System, w *units.World, a pool.Handle, request path.Request) {
	t.Helper()
	profile := Profile{FootPrintX: 1, FootPrintZ: 1, MaxWaterDepth: 12, MinWaterDepth: -10000, MaxSlope: 255, MaxWaterSlope: 255}
	sys = NewSystem(syntheticTerrainForIntegrate(), profile, NewOccupancyGrid())
	sys.Rules = &trafficRules{traffic: tr}
	w = newMovementFixtureWorld(16)
	def := setScratchMovement(&content.UnitDef{
		UnitName: "through-test", FootprintX: 1, FootprintZ: 1, BMCode: 1, CanMove: true,
		MaxVelocity: 2 << 16, Acceleration: 1 << 15, BrakeRate: 1 << 15, TurnRate: 0x800,
	}, profile)
	at := func(owner uint8, x, z int32) pool.Handle {
		h, err := w.Create(def, owner, world.CellToWorld(x)+8<<16, 0, world.CellToWorld(z)+8<<16)
		if err != nil {
			t.Fatal(err)
		}
		return h
	}
	a = at(0, 5, 10)
	var ring []pool.Handle
	for dz := int32(-1); dz <= 1; dz++ {
		for dx := int32(-1); dx <= 1; dx++ {
			if dx != 0 || dz != 0 {
				ring = append(ring, at(0, 5+dx, 10+dz))
			}
		}
	}
	far := at(other, 12, 10)
	sys.BindWorld(w)
	for _, h := range append(append([]pool.Handle{a}, ring...), far) {
		u := w.Unit(h)
		u.Flags |= 1 << units.StandingMoveShift
		sys.EnsureUnit(u)
		orders.QueueForUnit(u).SetBinding(&orders.QueueBinding{Lookup: w.Unit, World: &orders.WorldQueryAdapter{}})
	}
	move := func(h pool.Handle, x, z int32) *orders.Node {
		q := orders.QueueOfUnit(w.Unit(h))
		q.Push(orders.Lookup("Move_Ground"), orders.Node{Owner: h, GoalX: world.CellToWorld(x) + 8<<16, GoalZ: world.CellToWorld(z) + 8<<16, GoalSupplied: true})
		head := q.Head()
		sys.InstallPointGoal(orders.PointGoalRequest{Owner: h, Node: head, X: head.GoalX, Z: head.GoalZ, Radius: 4})
		return head
	}
	head := move(a, 15, 10)
	setHandleRow(&sys.activeOrders, a, &activeMove{order: head, token: throughToken})
	if going {
		for _, h := range ring {
			move(h, 15, 12)
		}
	}
	request = path.Request{Unit: a, Player: 0, Start: path.Cell{X: 5, Z: 10}, Goal: path.PointGoal(path.Cell{X: 15, Z: 10}, 0), Activation: throughToken}
	return sys, w, a, request
}

// throughSearch runs request to its end on tick 100, long after the units
// of the fixture stood still.
func throughSearch(t *testing.T, sys *System, request path.Request) path.WorkResult {
	t.Helper()
	sys.BeginTick(100)
	for range 64 {
		if res := sys.searchFunc(request, 0x18000, 1000); res.Done {
			return res
		}
	}
	t.Fatal("the search did not end")
	return path.WorkResult{}
}

// A unit its friends stand round is left without a route by retail's search,
// which reads a unit that has stood a second as a wall
// [04 R-PATH-01 §14][04 R-PATH-01 §4]. Under Through the request is searched
// again with friends read as absent: at one the friends with somewhere to
// go, at two every friend.
func TestThroughPlansThroughTheFriendsThatBoxAUnitIn(t *testing.T) {
	for _, tc := range []struct {
		name    string
		traffic Traffic
		going   bool
		route   bool
	}{
		{"no policy", Traffic{}, true, false},
		{"friends on their way, at one", Traffic{Through: 1}, true, true},
		{"idle friends, at one", Traffic{Through: 1}, false, false},
		{"idle friends, at two", Traffic{Through: 2}, false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sys, _, a, request := throughFixture(t, tc.traffic, tc.going, 0)
			res := throughSearch(t, sys, request)
			if got := len(res.Points) != 0; got != tc.route {
				t.Fatalf("route=%v (status %#x), want %v", got, res.Status, tc.route)
			}
			if got := plannedThrough(sys, a); got != tc.route {
				t.Fatalf("planned through friends=%v, want %v", got, tc.route)
			}
			if !tc.route {
				return
			}
			end := res.Points[len(res.Points)-1]
			if end.X>>4 != 15 || end.Z>>4 != 10 {
				t.Fatalf("the route ends at (%d,%d), not at the goal", end.X, end.Z)
			}
		})
	}
}

// Another player's unit stays a wall under every reading: a unit boxed in by
// units that are not its friends is left as retail leaves it.
func TestThroughKeepsAnotherPlayersUnits(t *testing.T) {
	sys, w, a, request := throughFixture(t, Traffic{Through: 2}, false, 0)
	for _, u := range w.AppendLive(nil) {
		if u.Handle != a {
			u.Owner = 1
		}
	}
	if res := throughSearch(t, sys, request); len(res.Points) != 0 {
		t.Fatalf("a route of %d points leads through another player's units", len(res.Points))
	}
	if plannedThrough(sys, a) {
		t.Fatal("the unit is marked as on a route through friends")
	}
}

// A request whose own search finds a route is not searched again, and the
// mark of an earlier route through friends is taken off.
func TestThroughLeavesAFoundRouteAlone(t *testing.T) {
	sys, w, a, request := throughFixture(t, Traffic{Through: 2}, false, 0)
	throughSearch(t, sys, request)
	if !plannedThrough(sys, a) {
		t.Fatal("the boxed unit was not planned through its friends")
	}
	// The friend to the east leaves: the unit's own search has a way.
	for _, u := range w.AppendLive(nil) {
		if u.Handle != a && world.WorldToCell(u.X) == 6 {
			sys.ForgetUnit(u.Handle)
		}
	}
	var opened int
	sys.Kernel = openedKernel{&opened}
	res := throughSearch(t, sys, request)
	if len(res.Points) == 0 {
		t.Fatalf("no route with the way east open (status %#x)", res.Status)
	}
	if opened != 1 {
		t.Fatalf("%d searches were opened for a request whose own search finds a route", opened)
	}
	if plannedThrough(sys, a) {
		t.Fatal("the unit is still marked as on a route through friends")
	}
}

// openedKernel counts the searches it opens.
type openedKernel struct{ opened *int }

func (k openedKernel) NewSession(cfg path.SearchConfig) path.Search {
	*k.opened++
	return path.NewSession(cfg)
}

// With ThroughAfter the request is searched again only once the unit's own
// searches have found nothing for that long; a new goal starts the wait
// again.
func TestThroughWaitsBeforeItPlansThroughFriends(t *testing.T) {
	sys, w, a, request := throughFixture(t, Traffic{Through: 2, ThroughAfter: 90}, false, 0)
	search := func(tick uint32) bool {
		sys.BeginTick(tick)
		for range 64 {
			if res := sys.searchFunc(request, 0x18000, 1000); res.Done {
				return len(res.Points) != 0
			}
		}
		t.Fatal("the search did not end")
		return false
	}
	if search(100) {
		t.Fatal("the first search that found nothing was searched again")
	}
	if search(189) {
		t.Fatal("a route through friends after 89 ticks without one")
	}
	if !search(190) {
		t.Fatal("no route through friends after 90 ticks without one")
	}
	u := w.Unit(a)
	head := orders.QueueOfUnit(u).Head()
	sys.noteGoal(a, head.GoalX+1<<16, head.GoalZ)
	if search(200) {
		t.Fatal("a new goal did not start the wait again")
	}
}

// plannedThrough reports whether h's route was planned through its friends.
func plannedThrough(s *System, h pool.Handle) bool {
	return handleRow(s.traffic, h).through != 0
}
