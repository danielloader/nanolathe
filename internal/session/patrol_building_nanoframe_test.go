package session

import (
	"testing"

	"github.com/nanolathe-gg/nanolathe/internal/construction"
	"github.com/nanolathe-gg/nanolathe/internal/content"
	"github.com/nanolathe-gg/nanolathe/internal/gameplay"
	"github.com/nanolathe-gg/nanolathe/internal/orders"
	"github.com/nanolathe-gg/nanolathe/internal/units"
	"github.com/nanolathe-gg/nanolathe/internal/world"
)

// Creation files unfinished buildings in the ordinary sectors before a patrol
// gathers nearby work [04 R-COLL-01 §4, §11][04 R-ORD-02 §4]. Issue 92.
func TestPatrolAssistsNewBuildingNanoframe(t *testing.T) {
	for _, mode := range []gameplay.Mode{gameplay.Strict31, gameplay.Community39, gameplay.Modern} {
		for _, yard := range []string{"o", "."} {
			for _, air := range []bool{false, true} {
				name := string(mode) + "/ground"
				if air {
					name = string(mode) + "/air"
				}
				t.Run(name+"/yard="+yard, func(t *testing.T) {
					s := assistSeamSession(t)
					s.SetGameplay(mode)
					s.Movement.BindWorld(s.Units)
					def := s.Catalog.Units["assistseamprod"]
					def.BMCode, def.CanMove, def.MovementClass, def.YardMap = 0, false, "", yard
					def.MinWaterDepth = -10000
					builderDef := s.Catalog.Units["assistseamcon"]
					builderDef.CanPatrol, builderDef.CanReclamate = true, true
					builderDef.SightDistance = 256
					create := func(d *content.UnitDef, x, z int32) *units.Unit {
						h, err := s.Units.Create(d, 0, world.CellToWorld(x), 0, world.CellToWorld(z))
						if err != nil {
							t.Fatal(err)
						}
						u := s.Units.Unit(h)
						s.Movement.EnsureUnit(u)
						s.bindOrderQueue(u)
						orders.QueueForUnit(u).CancelAll()
						return u
					}
					producer := create(builderDef, 8, 8)
					// Allocate through the real placement phase, leaving script readiness low
					// so no construction work or completion can register the product for us.
					if err := construction.QueueMobileBuild(producer, def.CanonicalKey, world.CellToWorld(12), world.CellToWorld(8), 1, s.Catalog); err != nil {
						t.Fatal(err)
					}
					build := orders.QueueForUnit(producer).Head()
					build.Phase, build.DynamicGate, build.Deadline = uint8(construction.State2), 0, -1
					if result := s.Build.StepUnit(construction.TickContext{Tick: 1}, producer.Handle); result.Err != nil {
						t.Fatal(result.Err)
					}
					product := s.Units.Unit(build.Target)
					if product == nil || product.Remaining != 1 || product.Health != 0 || product.Attachment.Carrier != 0 {
						t.Fatalf("placement did not leave an unattached nanoframe: %+v build=%+v", product, build)
					}
					helperDef := *builderDef
					helperDef.CanFly = air
					helper := create(&helperDef, 10, 10)
					helper.Flags = helper.Flags&^(uint32(3)<<units.StandingMoveShift) | uint32(2)<<units.StandingMoveShift
					patrolName, assistName, wantCode := "RepairPatrol", "HelpBuild", orders.Code(6)
					if air {
						patrolName, assistName, wantCode = "VTOL_RepairPatrol", "VTOL_HelpBuild", 3
					}
					if mode == gameplay.Modern {
						wantCode = 2
					}
					q := orders.QueueForUnit(helper)
					id := orders.Lookup(patrolName)
					q.Push(id, orders.Node{Owner: helper.Handle, Phase: 1, Deadline: -1, GoalX: helper.X, GoalZ: helper.Z})
					patrol := q.Head()
					stock := s.Econ.Players[0].Stock
					simBefore, crtBefore := s.SimRNG().Draws(), s.CrtRNG().Draws()
					code := orders.DescriptorFor(id).Handler(helper, patrol, 0, 2)
					if code != wantCode || q.Head().ID != orders.Lookup(assistName) || q.Head().Target != product.Handle {
						t.Fatalf("patrol ignored newly placed building: code=%d head=%+v product=%d", code, q.Head(), product.Handle)
					}
					if s.SimRNG().Draws() != simBefore || s.CrtRNG().Draws() != crtBefore || s.Econ.Players[0].Stock != stock {
						t.Fatal("single-candidate assistance selection changed RNG or resources")
					}
				})
			}
		}
	}
}
