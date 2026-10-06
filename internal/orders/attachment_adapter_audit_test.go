package orders

import (
	"testing"

	"github.com/nanolathe-gg/nanolathe/internal/content"
	"github.com/nanolathe-gg/nanolathe/internal/units"
)

func TestCarriedWorkPreambleRequiresComposedDetach(t *testing.T) {
	q, u := standingFixture(&content.UnitDef{UnitName: "worker", CanFly: true, BMCode: 1})
	u.Attachment.Carrier = 2
	q.binding.Movement = &MovementGoalAdapter{}
	if code := airWorkPreamble(u, &Node{Owner: u.Handle}, "Building"); code != 7 || u.Attachment.Carrier != 2 {
		t.Fatal("incomplete host context performed local detach")
	}
	calls := 0
	q.binding.Movement.DetachTakeoff = func(got *units.Unit) bool {
		calls++
		if got != u || u.Activated {
			t.Fatal("detach must precede activation")
		}
		got.Attachment.Carrier = 0
		got.Move.Mode = 2
		return true
	}
	if code := airWorkPreamble(u, &Node{Owner: u.Handle}, "Building"); code != 1 || calls != 1 {
		t.Fatalf("adapter calls=%d code=%d", calls, code)
	}
}
