package pathlab

import (
	"reflect"
	"testing"

	"github.com/nanolathe-gg/nanolathe/internal/movement"
	"github.com/nanolathe-gg/nanolathe/internal/path"
	"github.com/nanolathe-gg/nanolathe/internal/pathlab/labrules"
	"github.com/nanolathe-gg/nanolathe/internal/session"
)

// The laboratory set Modern adopted answers every movement question as
// Modern does and binds Modern's kernel, so that what was measured under its
// name is what Modern plays (docs/PATHFINDING_LAB.md "What was adopted").
func TestTheAdoptedSetIsModern(t *testing.T) {
	sameMovement(t, labrules.Adopted, session.ModernRuleSet())
}

// The baseline answers as Modern did before the adoption: the overlap
// policies on, the staggered throttle, no traffic policy, and the
// straightening kernel.
func TestTheBaselineIsTheOverlapRules(t *testing.T) {
	set := session.ModernRuleSet()
	set.Movement = &movement.OverlapRules{}
	set.Path = path.StraightenKernel{}
	sameMovement(t, BaselineName, set)
	if !set.Movement.AlliedPassThrough(nil) {
		t.Fatal("the baseline does not let head-on friends pass")
	}
	if got := set.Movement.Traffic(nil); got != (movement.Traffic{}) {
		t.Fatalf("the baseline answers a traffic policy: %+v", got)
	}
}

func sameMovement(t *testing.T, name string, want session.RuleSet) {
	t.Helper()
	got, ok := session.LookupRuleSet(name)
	if !ok {
		t.Fatalf("%s is not registered", name)
	}
	if reflect.TypeOf(got.Path) != reflect.TypeOf(want.Path) || got.Path != want.Path {
		t.Fatalf("%s binds the kernel %#v, want %#v", name, got.Path, want.Path)
	}
	a, b := got.Movement, want.Movement
	if a.Traffic(nil) != b.Traffic(nil) {
		t.Fatalf("%s answers the traffic policy\n%+v, want\n%+v", name, a.Traffic(nil), b.Traffic(nil))
	}
	for slot := 0; slot < 64; slot++ {
		for _, last := range []uint32{0, 1, 59, 60, 1234} {
			if x, y := a.RepathDelay(nil, slot, last), b.RepathDelay(nil, slot, last); x != y {
				t.Fatalf("%s re-route delay(slot %d, last %d) = %d, want %d", name, slot, last, x, y)
			}
		}
	}
	type answers struct {
		pass, slots, wedge, queue bool
		jamAfter                  uint16
		jamLife, pocketDwell      uint32
		pocketNear, frontier      int32
		unreachDwell              uint32
		spreadMin, spreadTicks    int
		carry                     int32
		sweep                     bool
	}
	of := func(r movement.Rules) answers {
		var x answers
		x.pass, x.slots, x.wedge, x.queue = r.AlliedPassThrough(nil), r.GroupDestinationSlots(nil), r.WedgeEscape(nil), r.RepairPadQueue(nil)
		x.jamAfter, x.jamLife = r.JamRelease(nil)
		x.pocketNear, x.pocketDwell = r.PocketRelease(nil)
		x.frontier, x.unreachDwell = r.UnreachableMoves(nil)
		x.spreadMin, x.spreadTicks = r.FirstRequestSpread(nil)
		x.carry, x.sweep = r.PathWorkBound(nil)
		return x
	}
	if x, y := of(a), of(b); x != y {
		t.Fatalf("%s answers %+v, want %+v", name, x, y)
	}
}
