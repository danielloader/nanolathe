package session

import (
	"testing"

	"github.com/nanolathe-gg/nanolathe/internal/content"
	"github.com/nanolathe-gg/nanolathe/internal/orders"
	"github.com/nanolathe-gg/nanolathe/internal/sim/numeric"
	"github.com/nanolathe-gg/nanolathe/internal/units"
	"github.com/nanolathe-gg/nanolathe/internal/world"
)

// Sea equality is admitted, the full signed model height decides water reach,
// and querying presentation copies cannot rebind a live queue [04 R-ORD-01 §7].
func TestCursorRepairAdmissionUsesCommittedWaterReach(t *testing.T) {
	s := &Session{World: &world.Terrain{SeaLevel: 55}}
	actor := &units.Unit{Alive: true, Def: &content.UnitDef{CanFly: true, CanReclamate: true}}
	binding := &orders.QueueBinding{}
	queue := orders.BindQueueBinding(actor, binding)
	target := &units.Unit{Alive: true, Health: 1, Remaining: 1,
		Def: &content.UnitDef{MaxDamage: 100, ModelTopFixed: 300 << 16, ModelTop: 44}}
	for _, tc := range []struct {
		y    int32
		want bool
	}{{-246, false}, {-245, true}, {-244, true}} {
		target.Y = numeric.Fixed(tc.y) << 16
		if got := s.CursorRepairAdmits(actor, target); got != tc.want {
			t.Fatalf("target top %d admitted=%v, want %v", tc.y+300, got, tc.want)
		}
	}
	if actor.Orders != queue || queue.Binding() != binding {
		t.Fatal("read-only cursor query changed the caller's order queue")
	}
	target.Move.ModeMirror = 2
	if s.CursorRepairAdmits(actor, target) {
		t.Fatal("airborne committed target admitted")
	}
}
