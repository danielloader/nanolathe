package session

import (
	"testing"

	"github.com/nanolathe-gg/nanolathe/internal/orders"
	"github.com/nanolathe-gg/nanolathe/internal/world"
)

// A queue head without a goal does not detach the follower's retained object.
// Arrival belongs to that object's record, even behind the gated head
// [04 R-ORD-01 §9][04 R-PATH-01 §8]. This checks the session boundary as well
// as the movement service, since either can otherwise erase the binding.
func TestUnitPhaseServicesBoundGroundGoalBehindGatedHead(t *testing.T) {
	s := wreckOrientationSession(t)
	uDef := s.Catalog.Units["wreckvictim"]
	h, err := s.Units.Create(uDef, 0, world.CellToWorld(4), 0, world.CellToWorld(4))
	if err != nil {
		t.Fatal(err)
	}
	u := s.Units.Unit(h)
	s.Movement.EnsureUnit(u)
	s.bindOrderQueue(u)
	q := orders.QueueForUnit(u)
	q.CancelAll()
	id := orders.Lookup("Move_Ground")
	q.Push(id, orders.NewMoveNode(id, u.X, u.Z, 0, h, false))
	owner := q.Head()
	if !s.Movement.InstallPointGoal(orders.PointGoalRequest{Owner: h, Node: owner, X: u.X, Z: u.Z, Radius: 16}) {
		t.Fatal("goal install refused")
	}
	head := q.PushHead(orders.Lookup("Paralyze"), orders.Node{Owner: h, DynamicGate: 1, Deadline: 1000})
	s.Clock.GlobalTick = 1
	s.stepUnitPhase(1)
	if q.Head() != head || head.Satisfied&0xE0 != 0 {
		t.Fatalf("gated head changed or received another record's movement outcome: %+v", head)
	}
	if owner.Satisfied&0x20 == 0 {
		t.Fatalf("bound owner did not receive arrival: pending=%#x", owner.Satisfied)
	}
}
