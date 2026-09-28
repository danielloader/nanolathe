package session

import (
	"testing"

	"github.com/nanolathe-gg/nanolathe/internal/content"
	"github.com/nanolathe-gg/nanolathe/internal/orders"
	"github.com/nanolathe-gg/nanolathe/internal/pool"
	"github.com/nanolathe-gg/nanolathe/internal/units"
)

// Ctrl+D is a toggle [07 R-CAM-01 §2]. The first press issues the rear-segment
// `SelfDestruct` and leaves the mission queue alone. A second press while it
// counts removes the record, the removal says `Self destruct terminated`
// (status 23), and nothing detonates [04 R-SPEC-01 §13]. A press is tested
// over the whole selection, so a mixed selection only cancels.
func TestHumanSelfDestructToggles(t *testing.T) {
	def := &content.UnitDef{BMCode: 1, MaxDamage: 100}
	def.CanonicalKey = "tank"
	cat := &content.Catalog{Units: map[string]*content.UnitDef{"tank": def}}
	w := newSessionFixtureWorld(4, cat)
	a, err := w.Create(def, 0, 0, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	b, err := w.Create(def, 0, 0, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	s := &Session{Units: w, Catalog: cat, LocalOwner: 0}
	ua, ub := w.Unit(a), w.Unit(b)
	s.bindOrderQueue(ua)
	s.bindOrderQueue(ub)
	type cue struct {
		unit pool.Handle
		kind uint8
	}
	var cues []cue
	s.Build.OrderBinding.Presentation = &orders.PresentationAdapter{
		Ready: func() bool { return true },
		Status: func(u *units.Unit, kind uint8, _ string) bool {
			cues = append(cues, cue{u.Handle, kind})
			return true
		},
	}
	id := orders.Lookup("SelfDestruct")
	press := func(tick uint32, handles ...pool.Handle) {
		s.applyHumanCommand(HumanCommand{Kind: HumanSelfDestruct, SelfDestruct: HumanSelfDestructCommand{Handles: handles}}, tick)
	}
	qa, qb := orders.QueueForUnit(ua), orders.QueueForUnit(ub)

	// The first press lands on the rear segment and keeps the mission queue.
	qa.Push(orders.Lookup("Move_Ground"), orders.Node{Owner: a, GoalX: 64 << 16, GoalZ: 64 << 16, GoalSupplied: true})
	move := qa.Primary()[0]
	press(100, a)
	if qa.LenSecondary() != 1 || qa.Secondary()[0].ID != id {
		t.Fatalf("Ctrl+D did not issue a rear-segment SelfDestruct: secondary=%d", qa.LenSecondary())
	}
	if qa.LenPrimary() != 1 || qa.Primary()[0] != move {
		t.Fatal("Ctrl+D disturbed the mission queue [04 R-ORD-01 §13]")
	}
	qa.RemovePrimaryNode(move, true)

	// One counting visit arms the cancel bit and announces `five`.
	qa.Pump(ua, 100)
	if len(cues) != 1 || cues[0] != (cue{a, 17}) {
		t.Fatalf("first counting visit cues = %v, want [five]", cues)
	}

	// The second press cancels instead of re-issuing.
	cues = cues[:0]
	press(101, a)
	if qa.LenSecondary() != 0 {
		t.Fatalf("second Ctrl+D left %d rear records, want the countdown removed", qa.LenSecondary())
	}
	if len(cues) != 1 || cues[0] != (cue{a, 23}) {
		t.Fatalf("cancel cues = %v, want one `Self destruct terminated`", cues)
	}
	for tick := uint32(102); tick < 400; tick++ {
		qa.Pump(ua, tick)
	}
	if ua.Dying || qa.LenSecondary() != 0 {
		t.Fatalf("cancelled countdown still ran: dying=%t rear=%d", ua.Dying, qa.LenSecondary())
	}

	// Mixed selection: a counts, b does not. The press cancels a and does not
	// start b.
	press(400, a)
	qa.Pump(ua, 400)
	cues = cues[:0]
	press(401, a, b)
	if qa.LenSecondary() != 0 || qb.LenSecondary() != 0 {
		t.Fatalf("mixed press: rear records a=%d b=%d, want both empty", qa.LenSecondary(), qb.LenSecondary())
	}
	if len(cues) != 1 || cues[0] != (cue{a, 23}) {
		t.Fatalf("mixed press cues = %v, want only a's termination", cues)
	}
}
