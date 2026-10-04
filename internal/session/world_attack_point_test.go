package session

import (
	"testing"

	"github.com/nanolathe-gg/nanolathe/internal/gameplay"
	"github.com/nanolathe-gg/nanolathe/internal/orders"
	"github.com/nanolathe-gg/nanolathe/internal/pool"
	"github.com/nanolathe-gg/nanolathe/internal/units"
)

// The hovered unit and cursor-ground triple are independent producer inputs
// [07 R-P0-11 §6][04 R-STANCE-01 §5][04 R-ORD-01 §13]. This armed ground
// actor's friendly attack resolves to Suppress, whose firing point comes from
// that triple [04 R-ORD-01 §3].
func TestWorldAttackPreservesCapturedGroundPoint(t *testing.T) {
	for _, mode := range []gameplay.Mode{gameplay.Strict31, gameplay.Community39, gameplay.Modern} {
		for _, path := range []string{"local", "online"} {
			for _, producer := range []string{"single", "batch"} {
				t.Run(string(mode)+"/"+path+"/"+producer, func(t *testing.T) {
					f := newSeatFixture(t, path == "online", false)
					f.s.SetGameplay(mode)
					f.def.CanAttack = true
					f.s.Units.Unit(f.own0).Flags |= units.ArmedStatus
					target := f.s.Units.Unit(f.own1a)
					target.Owner = 0
					point := CommandPosition{X: 116<<16 + 123, Y: 7<<16 + 456, Z: 68<<16 + 789}
					c := SeatCommand{Kind: SeatOrder, Order: OrderPayload{
						Actors: []pool.UnitRef{f.ref(f.own0)}, Code: 3, Target: f.ref(target.Handle), Position: point,
					}}
					if producer == "batch" {
						c.Order.Targets = []CommandTarget{{Target: c.Order.Target, Position: point}}
						c.Order.Target = pool.UnitRef{}
					}
					before := f.streams()
					stock := f.s.Econ.Players[0].Stock
					if err := f.s.EnqueueSeatCommand(f.stamp(0), c); err != nil {
						t.Fatal(err)
					}
					// Revalidation uses the live target identity, while a later
					// position change must not replace the captured click.
					target.X += 3 << 16
					target.Y += 2 << 16
					target.Z += 4 << 16
					rs := f.tick()
					if len(rs) != 1 {
						t.Fatalf("receipts = %+v, want one", rs)
					}
					expectOutcome(t, rs[0], CommandApplied)
					n := f.head(t, f.own0)
					if n.ID != orders.Lookup("Suppress") || n.GoalX != point.X || n.GoalY != point.Y || n.GoalZ != point.Z {
						t.Fatalf("order = %+v, want Suppress at captured point %+v", n, point)
					}
					if f.streams() != before || f.s.Econ.Players[0].Stock != stock {
						t.Fatal("command insertion changed RNG or resources")
					}
				})
			}
		}
	}
}
