package movement

import (
	"testing"

	"github.com/nanolathe-gg/nanolathe/internal/content"
	"github.com/nanolathe-gg/nanolathe/internal/orders"
	"github.com/nanolathe-gg/nanolathe/internal/pool"
	"github.com/nanolathe-gg/nanolathe/internal/sim/numeric"
	"github.com/nanolathe-gg/nanolathe/internal/world"
)

// jamCase steps a mover heading east against a unit holding the cell ahead
// for up to ticks ticks and reports the first tick the mover's commit was not
// rejected, or 0 (DESIGN_MOVEMENT_PATH "Modern jam release").
type jamCase struct {
	rules        Rules
	ownerB       uint8
	headingB     uint16 // the blocker's committed heading; 0xC000 is east
	blockerRoute bool
	moverEndCell int32
	blockerFoot  int32 // the blocker's square footprint in cells; 0 is 1
	oneWayAlly   bool  // the mover's owner declares the blocker's owner allied, not back
	moverCell    int32 // the mover's starting cell along the row; 0 is 5
	routeless    bool  // the mover never holds a route
	sameHeading  bool  // the blocker always heads exactly as the mover does
	// siteCell, when nonzero, gives the mover a work approach instead of a
	// point goal: a MobileBuild head with the rectangle goal of a one-cell
	// site anchored at that cell of the row, or with assist set a HelpBuild
	// head with the annulus goal an assist installs around it. outer, when
	// nonzero, replaces the annulus's outer radius; row, when set, replaces
	// the head's order row.
	siteCell int32
	assist   bool
	outer    int32
	row      string
	// wedge, when nonzero, registers a third friend on the mover's own cell
	// before the mover, so the mover stands inside it from the first tick:
	// wedgeMoving gives that friend an active route, wedgeParked none.
	wedge int
}

const (
	wedgeMoving = 1
	wedgeParked = 2
)

func runJamCase(t *testing.T, c jamCase, ticks uint32) (freed uint32, sys *System) {
	t.Helper()
	sys, a, b, step := setupJamCase(t, c)
	for tick := uint32(1); tick <= ticks; tick++ {
		if !step(tick).Blocked {
			return tick, sys
		}
		if got := handleRow(sys.Collisions, a).BlockerID; got != int(b) {
			t.Fatalf("tick %d: rejected by %d, not by the blocker %d", tick, got, b)
		}
	}
	return 0, sys
}

// setupJamCase builds the fixture and returns a stepper that re-publishes both
// routes each tick (the fixture has no scheduler) and runs one mover visit.
func setupJamCase(t *testing.T, c jamCase) (*System, pool.Handle, pool.Handle, func(tick uint32) StepResult) {
	t.Helper()
	sys := NewSystem(syntheticTerrainForIntegrate(), Profile{FootPrintX: 1, FootPrintZ: 1, MinWaterDepth: -10000, MaxSlope: 255}, NewOccupancyGrid())
	sys.Rules = c.rules
	w := newMovementFixtureWorld(4)
	def := setScratchMovement(&content.UnitDef{
		UnitName: "jam-release-test", FootprintX: 1, FootprintZ: 1, BMCode: 1,
		MaxVelocity: 2 * int32(worldUnitsPerCell), Acceleration: 2 * int32(worldUnitsPerCell),
		BrakeRate: 2 * int32(worldUnitsPerCell), TurnRate: 65535,
	}, Profile{FootPrintX: 1, FootPrintZ: 1, MaxWaterDepth: 12, MinWaterDepth: -10000, MaxSlope: 255, MaxWaterSlope: 255})
	blockerDef := def
	bx := world.CellToWorld(7)
	if f := int16(c.blockerFoot); f > 1 {
		// A parked f-by-f friend whose footprint starts at cell 7 of the row.
		blockerDef = setScratchMovement(&content.UnitDef{
			UnitName: "jam-release-wide", FootprintX: int32(f), FootprintZ: int32(f), BMCode: 1,
			MaxVelocity: int32(worldUnitsPerCell), Acceleration: int32(worldUnitsPerCell),
			BrakeRate: int32(worldUnitsPerCell), TurnRate: 65535,
		}, Profile{FootPrintX: f, FootPrintZ: f, MaxWaterDepth: 12, MinWaterDepth: -10000, MaxSlope: 255, MaxWaterSlope: 255})
		bx = numeric.Fixed(int64(7)<<20 + int64(f)<<19)
	}
	row := world.CellToWorld(10)
	b, err := w.Create(blockerDef, c.ownerB, bx, 0, row)
	if err != nil {
		t.Fatal(err)
	}
	moverCell := c.moverCell
	if moverCell == 0 {
		moverCell = 5
	}
	a, err := w.Create(def, 0, world.CellToWorld(moverCell), 0, row)
	if err != nil {
		t.Fatal(err)
	}
	var wedge pool.Handle
	if c.wedge != 0 {
		if wedge, err = w.Create(def, 0, world.CellToWorld(moverCell), 0, row); err != nil {
			t.Fatal(err)
		}
	}
	sys.BindWorld(w)
	// The blocker registers first, so a mover placed inside it overlaps an
	// incumbent that keeps its cells, as a wedge does.
	sys.EnsureUnit(w.Unit(b))
	if wedge != 0 {
		sys.EnsureUnit(w.Unit(wedge))
	}
	sys.EnsureUnit(w.Unit(a))
	rowZ := int32(row.Raw() >> 16)
	name := "Move_Ground"
	switch {
	case c.row != "":
		name = c.row
	case c.siteCell != 0 && c.assist:
		name = "HelpBuild"
	case c.siteCell != 0:
		name = "MobileBuild"
	}
	q := orders.QueueForUnit(w.Unit(a))
	q.Push(orders.Lookup(name), orders.Node{Owner: a, GoalX: world.CellToWorld(c.moverEndCell), GoalZ: row, GoalSupplied: true})
	if c.oneWayAlly {
		q.SetBinding(&orders.QueueBinding{Lookup: w.Unit, World: &orders.WorldQueryAdapter{DeclaresAlliance: func(from, toward uint8) bool { return from == 0 && toward == c.ownerB }}})
	}
	head := q.Head()
	if c.routeless {
		// A completed/re-armed record suppresses the installer's immediate
		// synthetic fallback [04 R-PATH-01 §8][05 R-EGRESS-02].
		head.Flags |= orders.FlagRetryMark
	}
	switch {
	case c.siteCell != 0 && c.assist:
		// Half a cell of stand-off out to two cells of build distance, as the
		// assist approach sizes its band.
		outer := c.outer
		if outer == 0 {
			outer = 40
		}
		sys.InstallAnnulusGoal(orders.AnnulusGoalRequest{Owner: head.Owner, Node: head, X: world.CellToWorld(c.siteCell), Z: row, InnerRadius: 8, OuterRadius: outer})
	case c.siteCell != 0:
		sys.InstallRectangleGoal(orders.RectangleGoalRequest{Owner: head.Owner, Node: head, CellX: c.siteCell, CellZ: rowZ >> 4, Width: 1, Depth: 1})
	default:
		sys.InstallPointGoal(orders.PointGoalRequest{Owner: head.Owner, Node: head, X: head.GoalX, Z: head.GoalZ, Radius: 4})
	}
	setHandleRow(&sys.activeOrders, a, &activeMove{order: head, token: 7})
	step := func(tick uint32) StepResult {
		// Both routes are re-published each tick so the fixture has no
		// scheduler and the blocker never moves: only the commit is tested.
		handleRow(sys.Collisions, b).Heading = c.headingB
		if c.sameHeading {
			handleRow(sys.Collisions, b).Heading = handleRow(sys.Collisions, a).Heading
		}
		if c.blockerRoute {
			handleRow(sys.Routes, b).PublishAtRevision([]Point{{X: 7*16 + 8, Z: rowZ}, {X: 30*16 + 8, Z: rowZ}}, sys.staticObstacleRevision())
		}
		if c.wedge == wedgeMoving {
			handleRow(sys.Routes, wedge).PublishAtRevision([]Point{{X: moverCell*16 + 8, Z: rowZ}, {X: 8, Z: rowZ}}, sys.staticObstacleRevision())
		}
		if !c.routeless {
			ax := handleRow(sys.Collisions, a).X >> 16
			handleRow(sys.Routes, a).PublishAtRevision([]Point{{X: ax, Z: rowZ}, {X: c.moverEndCell*16 + 8, Z: rowZ}}, sys.staticObstacleRevision())
		}
		sys.BeginTick(tick)
		res := sys.StepUnit(a, tick)
		sys.EndTick(tick)
		return res
	}
	return sys, a, b, step
}

func TestJamRelease(t *testing.T) {
	const ticks = 60
	for _, tc := range []struct {
		name  string
		c     jamCase
		freed uint32
	}{
		{"strict stays blocked", jamCase{rules: StrictRules{}, moverEndCell: 30}, 0},
		{"community stays blocked", jamCase{rules: CommunityRules{}, moverEndCell: 30}, 0},
		// Released after thirty jammed ticks, the mover commits on the next.
		{"overlap releases a unit parked friends hold", jamCase{rules: &OverlapRules{}, moverEndCell: 30}, modernJamReleaseAfter + 1},
		{"overlap keeps a same-way queue", jamCase{rules: &OverlapRules{}, headingB: 0xC000, blockerRoute: true, moverEndCell: 30}, 0},
		{"overlap never releases into an enemy", jamCase{rules: &OverlapRules{}, ownerB: 1, moverEndCell: 30}, 0},
		// Near the destination the release only takes the unit through the
		// friend that blocks it (see TestJamReleaseNearDestinationEndsWhenClear).
		{"overlap releases near the route end to pass the blocker", jamCase{rules: &OverlapRules{}, moverEndCell: 12}, modernJamReleaseAfter + 1},
		{"overlap never releases into a one-way ally", jamCase{rules: &OverlapRules{}, ownerB: 1, oneWayAlly: true, moverEndCell: 30}, 0},
		// A builder already working flush against the site is where a second
		// builder is going: releasing into it stacks the two (issue #31).
		{"overlap never releases into a builder at the site", jamCase{rules: &OverlapRules{}, moverEndCell: 9, siteCell: 8}, 0},
		{"overlap never releases into an assister at the site", jamCase{rules: &OverlapRules{}, moverEndCell: 7, siteCell: 9, assist: true}, 0},
		// An outer radius of 50 puts the arrival band (50/16 = 3 cells) a cell
		// past the zero band (50/18 = 2): the follower halts an assister three
		// cells out, where the friend stands, so that cell is the site too.
		{"overlap never releases into an assister on the arrival band", jamCase{rules: &OverlapRules{}, moverEndCell: 8, siteCell: 10, assist: true, outer: 50}, 0},
		// A parked friend short of the site is still passed as before.
		{"overlap releases past a friend short of the site", jamCase{rules: &OverlapRules{}, moverEndCell: 10, siteCell: 11}, modernJamReleaseAfter + 1},
		// An attack band is no work site: the point test at its centre keeps
		// passing a friend parked in the band.
		{"overlap releases past a friend in an attack band", jamCase{rules: &OverlapRules{}, moverEndCell: 7, siteCell: 9, assist: true, row: "Attack_Chase"}, modernJamReleaseAfter + 1},
		{"strict never releases at a site", jamCase{rules: StrictRules{}, moverEndCell: 9, siteCell: 8}, 0},
		// A builder jammed at a held site while a moving friend passes
		// through it is not wedged: the pass-through separates by itself, and
		// a release would carry it into the builder at the site. Inside a
		// parked friend it is wedged, and a release takes it out.
		{"overlap never releases out of a moving friend at a held site", jamCase{rules: &OverlapRules{}, moverEndCell: 9, siteCell: 8, wedge: wedgeMoving}, 0},
		{"overlap releases out of a parked friend at a held site", jamCase{rules: &OverlapRules{}, moverEndCell: 9, siteCell: 8, wedge: wedgeParked}, modernJamReleaseAfter + 1},
		{"overlap releases out of a moving friend near a point goal", jamCase{rules: &OverlapRules{}, moverEndCell: 9, wedge: wedgeMoving}, modernJamReleaseAfter + 1},
		{"strict never releases out of a friend at a site", jamCase{rules: StrictRules{}, moverEndCell: 9, siteCell: 8, wedge: wedgeParked}, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			freed, sys := runJamCase(t, tc.c, ticks)
			if freed != tc.freed {
				t.Fatalf("freed on tick %d, want %d", freed, tc.freed)
			}
			if _, strict := tc.c.rules.(StrictRules); strict && sys.jamReleases != nil {
				t.Fatal("a Strict run allocated jam-release state")
			}
		})
	}
}

func TestJamReleaseAnswers(t *testing.T) {
	// Modern retired the policy with the other two that let friendly units
	// share cells (DESIGN_MOVEMENT_PATH "Modern traffic").
	for _, r := range []Rules{StrictRules{}, CommunityRules{}, &ModernRules{}} {
		if after, life := r.JamRelease(nil); after != 0 || life != 0 {
			t.Fatalf("%T releases jams: (%d, %d)", r, after, life)
		}
	}
	if after, life := (&OverlapRules{}).JamRelease(nil); after != 30 || life != 90 {
		t.Fatalf("the laboratory's baseline answers (%d, %d), want (30, 90)", after, life)
	}
}

// A release that reaches the destination guard while the unit still stands
// inside a wide parked friend is held open until the unit is clear, so it is
// never left inside a friend that would block every later step (review probe:
// 1x1 mover, parked 3x3 friend on cells 7-9, goal at cell 15).
func TestJamReleaseNeverEndsInsideAFriend(t *testing.T) {
	sys, a, _, step := setupJamCase(t, jamCase{rules: &OverlapRules{}, moverEndCell: 15, blockerFoot: 3})
	u := sys.world.Unit(a)
	for tick := uint32(1); tick <= 400; tick++ {
		step(tick)
		coll := handleRow(sys.Collisions, a)
		if !sys.releasing(a, tick+1) && sys.insideFriend(u, coll) && tick > modernJamReleaseAfter+1 {
			t.Fatalf("tick %d: release over with the mover still inside the friend at %+v (state %+v)", tick, coll.CachedAnchor, handleRow(sys.jamReleases, a))
		}
	}
	if x := handleRow(sys.Collisions, a).CachedAnchor.X; x < 10 {
		t.Fatalf("mover never cleared the friend: anchor x %d", x)
	}
}

// Forgetting a unit drops its release, and a finished order forgets the run.
func TestJamReleaseStateIsCleared(t *testing.T) {
	sys, a, _, step := setupJamCase(t, jamCase{rules: &OverlapRules{}, moverEndCell: 30})
	for tick := uint32(1); tick <= modernJamReleaseAfter-1; tick++ {
		step(tick)
	}
	if handleRow(sys.jamReleases, a).run == 0 {
		t.Fatal("fixture did not count jammed ticks")
	}
	sys.DeactivateMove(a)
	if handleRow(sys.jamReleases, a).run != 0 {
		t.Fatal("a deactivated move kept its jammed run")
	}
	setHandleRow(&sys.jamReleases, a, jamRelease{until: 500, limit: 600})
	sys.ForgetUnit(a)
	if handleRow(sys.jamReleases, a) != (jamRelease{}) {
		t.Fatal("a forgotten unit kept its release")
	}
}

// Near its destination a released unit passes the friend that blocked it and
// the release ends at the first commit that leaves it clear of every friend.
func TestJamReleaseNearDestinationEndsWhenClear(t *testing.T) {
	sys, a, _, step := setupJamCase(t, jamCase{rules: &OverlapRules{}, moverEndCell: 12})
	u := sys.world.Unit(a)
	passed := false
	for tick := uint32(1); tick <= 90; tick++ {
		step(tick)
		coll := handleRow(sys.Collisions, a)
		if coll.CachedAnchor.X > 7 && !sys.insideFriend(u, coll) {
			passed = true
			if sys.releasing(a, tick+1) {
				t.Fatalf("tick %d: release still open after the mover cleared the friend near its destination", tick)
			}
			break
		}
	}
	if !passed {
		t.Fatal("mover never passed the friend blocking it near its destination")
	}
}

// A unit wedged inside a same-way friend is no queue member: the pair would
// wait on itself for ever, so the jam counts and the release passes the
// friend it overlaps. A route-less unit standing inside a friend proposes no
// step and never reads blocked, yet is wedged all the same and is released
// (DESIGN_MOVEMENT_PATH "Modern jam release").
func TestJamReleaseFreesWedgedUnits(t *testing.T) {
	queued := jamCase{rules: &OverlapRules{}, sameHeading: true, blockerRoute: true, moverEndCell: 30, blockerFoot: 5}
	if freed, _ := runJamCase(t, queued, 60); freed != 0 {
		t.Fatalf("a unit behind a same-way friend it does not overlap left the queue on tick %d", freed)
	}
	wedged := queued
	wedged.moverCell = 8 // inside the friend's cells 7-11, too deep to leave in one step
	sys, a, _, step := setupJamCase(t, wedged)
	if !sys.insideFriend(sys.world.Unit(a), handleRow(sys.Collisions, a)) {
		t.Fatal("fixture: the mover does not start inside the friend")
	}
	cleared := uint32(0)
	for tick := uint32(1); tick <= 150 && cleared == 0; tick++ {
		step(tick)
		if handleRow(sys.Collisions, a).CachedAnchor.X >= 12 {
			cleared = tick
		}
	}
	if cleared == 0 {
		t.Fatal("a unit wedged inside a same-way friend never got past it")
	}
	stuck := jamCase{rules: &OverlapRules{}, moverEndCell: 30, blockerFoot: 3, moverCell: 8, routeless: true}
	sys, a, _, step = setupJamCase(t, stuck)
	released := false
	for tick := uint32(1); tick <= 45 && !released; tick++ {
		step(tick)
		released = sys.releasing(a, tick+1)
	}
	if !released {
		t.Fatal("a route-less unit standing inside a friend was never released")
	}
	strict := stuck
	strict.rules = StrictRules{}
	sys, a, _, step = setupJamCase(t, strict)
	for tick := uint32(1); tick <= 45; tick++ {
		step(tick)
	}
	if sys.releasing(a, 46) || sys.jamReleases != nil {
		t.Fatal("Strict released a wedged unit")
	}
}
