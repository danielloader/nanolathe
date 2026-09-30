package construction

import (
	"testing"

	"github.com/nanolathe-gg/nanolathe/internal/orders"
	"github.com/nanolathe-gg/nanolathe/internal/testsupport/retailcat"
	"github.com/nanolathe-gg/nanolathe/internal/units"
	"github.com/nanolathe-gg/nanolathe/internal/world"
)

func airClearanceFixture(t *testing.T, installed bool) (*Service, *units.Unit, *units.Unit, *orders.Node) {
	t.Helper()
	s, builder, old, n := siteYieldFixture(t)
	s.Movement.ForgetUnit(old.Handle)
	s.World.FreeImmediate(old.Handle)
	d := exitAircraftDef("clearancescout", 1, 1)
	d.MovementClass = "kb"
	d.StandingMoveOrder = 1
	d.DefaultMissionType = "VTOL_Standby"
	d.BankScale = 65536
	d.MaxSlope = 10
	if installed {
		cat, _ := retailcat.Shared(t)
		var ok bool
		d, ok = cat.Unit("armpeep")
		if !ok {
			t.Fatal("stock scout definition missing")
		}
		product, ok := cat.Unit("armsolar")
		if !ok {
			t.Fatal("stock solar definition missing")
		}
		s.Catalog.Units[product.CanonicalKey] = product
		n.BuildDefKey = product.CanonicalKey
		s.Catalog.Movement[d.MovementClass] = cat.Movement[d.MovementClass]
		s.Movement.SetClasses(s.Catalog.Movement)
	}
	s.Terrain.Gravity = 0x1fdb
	s.Catalog.Units[d.CanonicalKey] = d
	h, err := s.World.Create(d, builder.Owner, world.CellToWorld(10), 0, world.CellToWorld(10))
	if err != nil {
		t.Fatal(err)
	}
	u := s.World.Unit(h)
	s.Movement.EnsureUnit(u)
	s.OrderBinding.Movement.RunAir = s.Movement.AirLegRunner()
	s.OrderBinding.Movement.InstallAir = s.Movement.InstallAirGoal
	q := s.queueForUnit(u)
	id := orders.Lookup("VTOL_Standby")
	q.Push(id, orders.NewNodeForOrder(id, 0, u.X, u.Y, u.Z, 0, u.Handle, true))
	q.Head().Flags |= orders.FlagAutoOp
	s.Community.ConstructionKickout = true
	return s, builder, u, n
}

// CP-CON-1 includes grounded aircraft; Modern uses ordinary VTOL_Move under
// its own clearance policy. Takeoff releases occupancy only at movement commit
// [04 R-ORD-02 §2][04 R-AIR-01 §6][04 R-COLL-01 §4].
func checkAirSiteClearance(t *testing.T, installed bool) {
	t.Helper()
	for _, tc := range []struct {
		name  string
		rules Rules
		on    bool
		clear bool
		crt   int
	}{
		{"Strict", StrictRules{}, true, false, 0},
		{"Community disabled", CommunityRules{}, false, false, 0},
		{"Community", CommunityRules{}, true, true, 1},
		{"Modern", &ModernRules{}, true, true, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, builder, u, n := airClearanceFixture(t, installed)
			s.Rules = tc.rules
			s.Community.ConstructionKickout = tc.on
			draws := 0
			s.CRTRandom = func(bound uint32) uint32 {
				if bound != 360 {
					t.Fatalf("CRT bound=%d, want 360", bound)
				}
				draws++
				return 0
			}
			q := orders.QueueOfUnit(u)
			idle := q.Head()
			stock := s.Economy.Players
			sim := *s.OrderBinding.SimRNG
			x, y, z := u.X, u.Y, u.Z
			cellX, cellZ := world.WorldToCell(u.X), world.WorldToCell(u.Z)
			if s.Terrain.Movers.CellOccupant(cellX, cellZ) != uint16(u.Handle) {
				t.Fatal("fixture aircraft does not hold the ground cell")
			}
			s.mobilePlacementVisit(builder, n, 1)
			if n.Target != 0 || n.Deadline != 31 || n.Param3 != 1 {
				t.Fatalf("first blocked visit changed: %+v", n)
			}
			if s.Economy.Players != stock || *s.OrderBinding.SimRNG != sim || draws != tc.crt {
				t.Fatalf("issuance changed resources/RNG: CRT draws=%d want %d", draws, tc.crt)
			}
			if u.X != x || u.Y != y || u.Z != z || u.Move.Mode != 1 || s.Terrain.Movers.CellOccupant(cellX, cellZ) != uint16(u.Handle) {
				t.Fatal("clearance directly moved the aircraft or released occupancy")
			}
			if !tc.clear {
				if q.Head() != idle || s.AdmitSiteOccupant(builder, u) {
					t.Fatal("disabled clearance admitted or ordered the blocking aircraft")
				}
				return
			}
			if !s.AdmitSiteOccupant(builder, u) || q.Head() == idle || q.Head().ID != orders.Lookup("VTOL_Move") {
				t.Fatalf("blocking aircraft did not receive VTOL_Move: %+v", q.Head())
			}
			if tick := q.Head().CreationTick; tick != 1 {
				t.Fatalf("clearance creation tick=%d, want first blocked visit", tick)
			}
			sx, sz, sw, sh, ok := s.siteAnchorCell(n)
			if !ok {
				t.Fatal("construction site footprint missing")
			}
			goalX, goalZ := world.WorldToCell(q.Head().GoalX), world.WorldToCell(q.Head().GoalZ)
			if goalX >= sx && goalX < sx+sw && goalZ >= sz && goalZ < sz+sh {
				t.Fatal("clearance goal stayed inside the construction site")
			}
			for tick := uint32(2); tick <= 31; tick++ {
				q.Pump(u, tick)
				s.Movement.BeginTick(tick)
				s.Movement.StepUnit(u.Handle, tick)
				s.Movement.EndTick(tick)
				s.mobilePlacementVisit(builder, n, tick)
			}
			if u.Move.Mode != 2 || s.Terrain.Movers.CellOccupant(cellX, cellZ) == uint16(u.Handle) || n.Target == 0 {
				t.Fatalf("takeoff did not clear site for allocation: mode=%d target=%d", u.Move.Mode, n.Target)
			}
			t.Logf("%s cleared the site and allocated %s by tick 31", u.Def.UnitName, n.BuildDefKey)
		})
	}
}

func TestLandedAircraftConstructionClearance(t *testing.T) {
	checkAirSiteClearance(t, false)
}

func TestInstalledLandedAircraftConstructionClearance(t *testing.T) {
	checkAirSiteClearance(t, true)
}

func TestModernAircraftClearancePreservesIneligibleOrders(t *testing.T) {
	for _, kind := range []string{"hold", "other owner", "attached", "incomplete", "airborne", "explicit standby", "move", "repair", "queued move", "factory", "terminal"} {
		t.Run(kind, func(t *testing.T) {
			s, builder, u, n := airClearanceFixture(t, false)
			q := orders.QueueOfUnit(u)
			switch kind {
			case "hold":
				u.Flags &^= StandingMoveMask
			case "other owner":
				u.Owner++
			case "attached":
				u.Attachment.Carrier = builder.Handle
			case "incomplete":
				u.Remaining = .5
			case "airborne":
				u.Move.Mode = 2
			case "explicit standby":
				q.Head().Flags &^= orders.FlagAutoOp
			case "move":
				id := orders.Lookup("VTOL_Move")
				q.Push(id, orders.NewMoveNode(id, world.CellToWorld(20), world.CellToWorld(20), 0, u.Handle, false))
			case "repair":
				q.Push(orders.Lookup("SelfRepair"), orders.Node{Target: builder.Handle, Flags: orders.FlagAutoOp})
			case "queued move":
				q.SetPrimary([]*orders.Node{q.Head(), {ID: orders.Lookup("VTOL_Move")}})
			case "terminal":
				n.Param3 = 11
			}
			head := q.Head()
			if kind == "factory" {
				x, z, fx, fz, _ := s.siteAnchorCell(n)
				extent, _ := world.NewFootprintExtent(fx, fz)
				clear, _ := world.NewFootprintRect(world.NewFootprintAnchor(x, z), extent)
				s.rules().YieldObstruction(s, builder, clear, nil, 1, false)
			} else {
				s.mobilePlacementVisit(builder, n, 1)
			}
			if q.Head() != head {
				t.Fatal("aircraft clearance replaced an ineligible order")
			}
		})
	}
}

func TestModernAircraftClearanceCrossesBlockedGround(t *testing.T) {
	s, builder, u, n := airClearanceFixture(t, false)
	start, fx, fz, _ := s.Movement.CommittedFootprint(u.Handle)
	for z := start.Z - 1; z <= start.Z+int32(fz); z++ {
		for x := start.X - 1; x <= start.X+int32(fx); x++ {
			if x < start.X || x >= start.X+int32(fx) || z < start.Z || z >= start.Z+int32(fz) {
				s.Terrain.PlotAt(x, z).SetOccupantA(30000)
			}
		}
	}
	s.mobilePlacementVisit(builder, n, 1)
	if head := orders.QueueOfUnit(u).Head(); head.ID != orders.Lookup("VTOL_Move") {
		t.Fatal("aircraft clearance incorrectly required a walkable ground route")
	}
}
