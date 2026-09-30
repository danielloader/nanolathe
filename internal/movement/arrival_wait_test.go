package movement

import (
	"testing"

	"github.com/nanolathe-gg/nanolathe/internal/orders"
	"github.com/nanolathe-gg/nanolathe/internal/world"
)

// Phase 0 can be an armed retry, not just a newly constructed record. Only the
// order pump consumes its wait [04 §3.3]; arrival binding must leave it intact.
func TestArrivalBindingPreservesPhaseZeroWait(t *testing.T) {
	terrain := syntheticTerrainForIntegrate()
	sys := NewSystem(terrain, wiringProfile, NewOccupancyGrid())
	w := newMovementFixtureWorld(10)
	sys.BindWorld(w)
	h, err := w.Create(wiringDef(), 0, world.CellToWorld(2), 0, world.CellToWorld(2))
	if err != nil {
		t.Fatal(err)
	}
	u := w.Unit(h)
	sys.EnsureUnit(u)
	q := orders.QueueForUnit(u)
	q.Push(orders.Lookup("Move_Ground"), orders.Node{
		Owner: h, GoalX: world.CellToWorld(8), GoalZ: world.CellToWorld(8), GoalSupplied: true,
		DynamicGate: 1, Deadline: 120, Flags: orders.FlagRetryMark,
	})
	head := q.Head()
	if !sys.ActivateMove(u, head) {
		t.Fatal("movement activation refused")
	}
	if head.Phase != 0 || head.DynamicGate != 1 || head.Deadline != 120 || head.Satisfied != 0 {
		t.Fatalf("arrival binding changed the phase-0 wait: phase=%d gate=%#x deadline=%d satisfied=%#x", head.Phase, head.DynamicGate, head.Deadline, head.Satisfied)
	}
}
