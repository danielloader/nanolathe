package session

import (
	"testing"

	"github.com/nanolathe-gg/nanolathe/internal/orders"
)

// Queue teardown is the record destructor, not the explicit release
// [04 R-ORD-01 §9]: removing a record whose goal another record displaced
// leaves that record's binding, walk and pending word alone. Through the
// explicit release it raised the goal-released bit on the bound record, the
// wake a mobile build's approach retires on without a range test
// [05 R-WORK-01 §13].
func TestQueueTeardownOfADisplacedRecordLeavesTheBoundGoal(t *testing.T) {
	s := newLoopTestSession(t, 0)
	s.Movement.BindWorld(s.Units)
	h, err := s.Units.Create(s.Catalog.Units["armcom"], 0, 64<<16, 0, 64<<16)
	if err != nil {
		t.Fatal(err)
	}
	u := s.Units.Unit(h)
	s.Movement.EnsureUnit(u)
	move := orders.Lookup("Move_Ground")
	front := &orders.Node{ID: move, Owner: h, Phase: 1, DynamicGate: 0xE0}
	behind := &orders.Node{ID: move, Owner: h, Phase: 1, DynamicGate: 0xE0}
	orders.BindQueue(u, orders.NewQueueWith([]*orders.Node{front, behind}, nil))
	s.bindExistingOrderQueue(u)
	q := orders.QueueOfUnit(u)
	if b := q.Binding(); b == nil || b.Movement == nil || b.Movement.Destroy == nil {
		t.Fatal("the composed queue binding has no record-destructor port")
	}
	for _, n := range []*orders.Node{behind, front} {
		if !s.Movement.InstallPointGoal(orders.PointGoalRequest{Owner: h, Node: n, X: 160 << 16, Z: 64 << 16, Radius: 16}) {
			t.Fatal("goal not installed")
		}
	}
	if behind.Satisfied&0x80 == 0 {
		t.Fatal("the displaced record did not receive the goal-released bit [04 R-ORD-01 §9]")
	}
	q.RemovePrimaryNode(behind, true)
	if front.Satisfied&0x80 != 0 || !s.Movement.HasGroundGoal(h, front) {
		t.Fatalf("removing the displaced record released the bound one: pending %#x, bound %v", front.Satisfied, s.Movement.HasGroundGoal(h, front))
	}
	// The bound record's own removal still unbinds.
	q.RemovePrimaryNode(front, false)
	if s.Movement.HasGroundGoal(h, front) {
		t.Fatal("removing the bound record left its goal bound")
	}
}
