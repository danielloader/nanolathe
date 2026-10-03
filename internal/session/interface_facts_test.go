package session

import (
	"slices"
	"testing"

	"github.com/nanolathe-gg/nanolathe/internal/frame"
	"github.com/nanolathe-gg/nanolathe/internal/hud"
	"github.com/nanolathe-gg/nanolathe/internal/pool"
	"github.com/nanolathe-gg/nanolathe/internal/units"
)

// interfaceFactsFixture is the paused-input fixture with a third finished
// constructor, all three the local player's, in ascending slots.
func interfaceFactsFixture(t *testing.T) (*Session, [3]*units.Unit) {
	t.Helper()
	s, a, b := pausedInputFixture(t)
	def, _ := s.Catalog.Unit("fixbuilder")
	h, err := s.Units.Create(def, 0, numeric16(30), 0, numeric16(30))
	if err != nil {
		t.Fatal(err)
	}
	c := s.Units.Unit(h)
	c.Health, c.MaxHealth, c.Remaining = 100, 100, 0
	return s, [3]*units.Unit{a, b, c}
}

func unitRef(u *units.Unit) pool.UnitRef {
	return pool.UnitRef{Handle: u.Handle, Serial: u.AllocationSerial}
}

func stepOneTick(s *Session) { s.Step(s.Clock.ScaledAnchor + 1) }

// The tick's facts report the units the sweep found not ready at their own
// visit and, for the local seat, the ready units at the sweep tail. Producing
// them is a pure observation: a session without a consumer reaches the same
// status words and the same partial fingerprint (DESIGN_MULTIPLAYER §16.2
// M2-C6, M2-C7).
func TestReadinessObservationReportsVisitAndTailFacts(t *testing.T) {
	consumed, us := interfaceFactsFixture(t)
	plain, plainUnits := interfaceFactsFixture(t)
	consumed.SetLocalInterfaceFacts(true)
	for _, u := range []*units.Unit{us[1], plainUnits[1]} {
		u.Remaining = 0.5 // under construction: not ready
	}
	stepOneTick(consumed)
	stepOneTick(plain)

	f := consumed.Snapshot.Current()
	if !f.Interface.Valid || f.Interface.Tick != consumed.Clock.GlobalTick || f.Interface.Owner != 0 {
		t.Fatalf("facts = %+v, want valid facts for the local seat at tick %d", f.Interface, consumed.Clock.GlobalTick)
	}
	if want := []pool.UnitRef{unitRef(us[1])}; !slices.Equal(f.Interface.Unready, want) {
		t.Fatalf("unready at visit = %v, want %v", f.Interface.Unready, want)
	}
	if want := []pool.UnitRef{unitRef(us[0]), unitRef(us[2])}; !slices.Equal(f.Interface.Ready, want) {
		t.Fatalf("ready at the sweep tail = %v, want %v in slice order", f.Interface.Ready, want)
	}
	for i := range us {
		if us[i].Flags != plainUnits[i].Flags {
			t.Fatalf("unit %d status %#x with a consumer, %#x without", i, us[i].Flags, plainUnits[i].Flags)
		}
	}
	for _, v := range f.Units {
		if v.Flags&units.SelectedStatus != 0 || f.Selection.Count != 0 || f.CommandPage.Builder != 0 {
			t.Fatal("publication carried a selection: it is the host's to compose")
		}
	}
	retained := consumed.Snapshot.DrainInterfaceFacts(nil)
	if len(retained) != 1 || retained[0].Tick != f.Interface.Tick || !slices.Equal(retained[0].Unready, f.Interface.Unready) {
		t.Fatalf("retained facts = %+v", retained)
	}
	if again := consumed.Snapshot.DrainInterfaceFacts(nil); len(again) != 0 {
		t.Fatal("facts drained twice")
	}

	if pf := plain.Snapshot.Current(); pf.Interface.Valid {
		t.Fatal("a session without a consumer published facts")
	}
	a, err := consumed.PartialStateFingerprint()
	if err != nil {
		t.Fatal(err)
	}
	b, err := plain.PartialStateFingerprint()
	if err != nil {
		t.Fatal(err)
	}
	if a != b {
		t.Fatalf("a facts consumer changed the simulation: %s vs %s", a, b)
	}
}

// A unit that turns unready before its visit and ready again before the tick
// ends leaves the selection: the sweep's step 7 sees it at the visit
// [04 R-MOV-03 §1]. The committed frame shows it ready, so the frame alone
// could not say so; the tick's facts do (M2-C6 transient readiness).
func TestLocalSelectionDropsAUnitUnreadyOnlyAtItsVisit(t *testing.T) {
	s, us := interfaceFactsFixture(t)
	s.SetLocalInterfaceFacts(true)
	local := hud.NewLocalInterface()
	local.ReplaceSelection([]pool.UnitRef{unitRef(us[0]), unitRef(us[1])})
	transient := us[0]
	s.PhaseObserver = func(phase string, tick uint32) {
		switch phase {
		case "phase1-network": // after the probe is armed, before the sweep
			transient.Remaining = 0.5
		case "phase2-units": // after the sweep tail
			transient.Remaining = 0
		}
	}
	stepOneTick(s)
	s.PhaseObserver = nil

	f := s.Snapshot.Current()
	for _, v := range f.Units {
		if v.Slot == transient.Handle && v.BuildRemaining != 0 {
			t.Fatal("fixture: the transient unit is not ready at publication")
		}
	}
	for _, facts := range s.Snapshot.DrainInterfaceFacts(nil) {
		local.Advance(facts)
	}
	if local.Selected(unitRef(transient)) {
		t.Fatal("a unit unready at its visit stayed selected")
	}
	if !local.Selected(unitRef(us[1])) {
		t.Fatal("a unit ready at its visit left the selection")
	}
}

// A catch-up batch whose newest publication alone is presented advances the
// local state exactly as consuming every boundary does: the retained facts
// carry every intervening tick, including a selected unit that dies and a new
// unit that takes its slot within the batch, and a transient unready tick
// (M2-C6 skipped display frames and handle reuse).
func TestLocalStateFromACatchUpBatchEqualsEveryBoundary(t *testing.T) {
	type run struct {
		s     *Session
		us    [3]*units.Unit
		local *hud.LocalInterface
		reuse pool.UnitRef
	}
	newRun := func() *run {
		s, us := interfaceFactsFixture(t)
		s.SetLocalInterfaceFacts(true)
		r := &run{s: s, us: us, local: hud.NewLocalInterface()}
		r.local.ReplaceSelection([]pool.UnitRef{unitRef(us[0]), unitRef(us[1]), unitRef(us[2])})
		def, _ := s.Catalog.Unit("fixbuilder")
		s.PhaseObserver = func(phase string, tick uint32) {
			switch {
			case tick == 2 && phase == "phase1-network":
				s.Units.Destroy(us[1].Handle, units.DeathKilled)
			case tick == 3 && phase == "phase5-orders":
				h, err := s.Units.Create(def, 0, numeric16(40), 0, numeric16(40))
				if err != nil {
					t.Fatal(err)
				}
				u := s.Units.Unit(h)
				u.Health, u.MaxHealth, u.Remaining = 100, 100, 0
				r.reuse = unitRef(u)
			case tick == 4 && phase == "phase1-network":
				us[2].Remaining = 0.5
			case tick == 4 && phase == "phase2-units":
				us[2].Remaining = 0
			}
		}
		return r
	}
	every, batch := newRun(), newRun()

	for i := 0; i < 6; i++ {
		stepOneTick(every.s)
		for _, facts := range every.s.Snapshot.DrainInterfaceFacts(nil) {
			every.local.Advance(facts)
		}
	}
	batch.s.Step(batch.s.Clock.ScaledAnchor + 5)
	stepOneTick(batch.s)
	latest := batch.s.Snapshot.Current()
	if latest.Tick != 6 {
		t.Fatalf("fixture: the batch presented tick %d, want 6", latest.Tick)
	}
	facts := batch.s.Snapshot.DrainInterfaceFacts(nil)
	if len(facts) != 6 {
		t.Fatalf("retained %d ticks of facts, want 6", len(facts))
	}
	for _, f := range facts {
		batch.local.Advance(f)
	}

	if every.reuse.Handle != every.us[1].Handle || every.reuse == unitRef(every.us[1]) {
		t.Fatalf("fixture: slot %d was not reused by a new allocation (%v)", every.us[1].Handle, every.reuse)
	}
	got, want := batch.local.SelectedRefs(), every.local.SelectedRefs()
	if !slices.Equal(got, want) {
		t.Fatalf("catch-up selection %v, every-boundary selection %v", got, want)
	}
	// The dying unit's last visit finds it unready, so step 7 drops it before
	// presentation's prune would [04 R-MOV-03 §1 step 7].
	if !slices.Equal(want, []pool.UnitRef{unitRef(every.us[0])}) {
		t.Fatalf("selection %v: want only the survivor", want)
	}
	if batch.local.Selected(batch.reuse) {
		t.Fatal("the unit created in a selected unit's slot inherited the selection")
	}
	// The presented frame alone would keep the transient unit selected: it is
	// ready there.
	for _, v := range latest.Units {
		if v.Slot == batch.us[2].Handle && v.BuildRemaining != 0 {
			t.Fatal("fixture: the transient unit is not ready in the presented frame")
		}
	}
}

// BigBrother's cycle runs at the sweep tail over the tick's ready units: a
// session-driven cycle selects the ready unit after the selected one and
// raises its notices (§7.1; [07 R-CAM-01 §12]).
func TestBigBrotherCyclesOverTheTickFacts(t *testing.T) {
	s, us := interfaceFactsFixture(t)
	s.SetLocalInterfaceFacts(true)
	local := hud.NewLocalInterface()
	local.ReplaceSelection([]pool.UnitRef{unitRef(us[0])})
	local.ToggleBigBrother()
	stepOneTick(s)
	var ev hud.InterfaceEvents
	for _, facts := range s.Snapshot.DrainInterfaceFacts(nil) {
		ev = local.Advance(facts)
	}
	if !ev.Cycle || !ev.ResetVisited || ev.CancelFollow {
		t.Fatalf("first tick notices %+v, want cycle and reset", ev)
	}
	if !slices.Equal(local.SelectedRefs(), []pool.UnitRef{unitRef(us[1])}) {
		t.Fatalf("selection %v, want the next ready unit", local.SelectedRefs())
	}
}

// The local interface kinds never reach the session, in either context: the
// session holds no selection, page, BigBrother or Shift state (§7.3).
func TestSessionRefusesLocalInterfaceKinds(t *testing.T) {
	s, us := interfaceFactsFixture(t)
	for _, c := range []HumanCommand{
		{Kind: HumanSelectionReplace, Selection: HumanSelectionCommand{Handles: []pool.Handle{us[0].Handle}}},
		{Kind: HumanSelectionToggle}, {Kind: HumanSelectionClear},
		{Kind: HumanBuildPage, BuildPage: HumanBuildPageCommand{Builder: us[0].Handle, Page: 1}},
		{Kind: HumanGroupRecall, Group: HumanGroupCommand{Group: 1}},
		{Kind: HumanBigBrother}, {Kind: HumanShiftState, ShiftHeld: true},
	} {
		if err := s.EnqueueHumanCommand(c); err == nil {
			t.Fatalf("kind %d reached the session", c.Kind)
		}
	}
	if len(s.PendingHumanCommands()) != 0 {
		t.Fatal("a refused local kind was queued")
	}
}

// composeLocal composes a client's local selection and command page onto f, as
// the battle host does: the publication carries neither (DESIGN_MULTIPLAYER
// §7.3).
func composeLocal(s *Session, local *hud.LocalInterface, f *frame.Frame) {
	local.ComposeSelection(f)
	hud.ComposeCommandPage(&f.CommandPage, f, s.Catalog, s.CommandPageProducts)
}

// selectLocal is a client whose local selection is exactly us.
func selectLocal(us ...*units.Unit) *hud.LocalInterface {
	local := hud.NewLocalInterface()
	refs := make([]pool.UnitRef, len(us))
	for i, u := range us {
		refs[i] = unitRef(u)
	}
	local.ReplaceSelection(refs)
	return local
}

// A status word that already carries bit 4 — only a retail save's restore
// writes it now — keeps whatever the sweep's own step 7 leaves, so a battle
// with a facts consumer reaches the same status words and fingerprint as one
// without, over several ticks (DESIGN_MULTIPLAYER §16.2 M2-C7, I6).
func TestFactsConsumerLeavesALoadedSelectedBitToStep7(t *testing.T) {
	consumed, us := interfaceFactsFixture(t)
	plain, plainUnits := interfaceFactsFixture(t)
	consumed.SetLocalInterfaceFacts(true)
	for _, set := range [][3]*units.Unit{us, plainUnits} {
		set[0].Flags |= units.SelectedStatus // ready: step 7 keeps it
		set[1].Flags |= units.SelectedStatus
		set[1].Remaining = 0.5 // unready: step 7 clears it
	}
	for tick := 0; tick < 3; tick++ {
		stepOneTick(consumed)
		stepOneTick(plain)
		for i := range us {
			if us[i].Flags != plainUnits[i].Flags {
				t.Fatalf("tick %d: unit %d status %#x with a consumer, %#x without", tick, i, us[i].Flags, plainUnits[i].Flags)
			}
		}
		a, err := consumed.PartialStateFingerprint()
		if err != nil {
			t.Fatal(err)
		}
		b, err := plain.PartialStateFingerprint()
		if err != nil {
			t.Fatal(err)
		}
		if a != b {
			t.Fatalf("tick %d: a facts consumer changed the simulation: %s vs %s", tick, a, b)
		}
	}
	if us[0].Flags&units.SelectedStatus == 0 || us[1].Flags&units.SelectedStatus != 0 || us[2].Flags&units.SelectedStatus != 0 {
		t.Fatal("step 7 did not keep the ready unit's restored bit and clear the unready one's")
	}
	if f := consumed.Snapshot.Current(); !slices.Contains(f.Interface.Unready, unitRef(us[1])) {
		t.Fatalf("unready %v: the unready restored unit was not reported", f.Interface.Unready)
	}
}

// The facts report a unit unready at its visit whoever owns it: step 7 clears
// the selected bit on any unit [04 R-MOV-03 §1 step 7], so a selection a
// client still holds after a capture is maintained like any other.
func TestUnreadyFactsCoverEveryOwner(t *testing.T) {
	s, _ := interfaceFactsFixture(t)
	s.Econ.Players[1].Exists = true
	s.Econ.Players[1].ControllerState = 2
	def, _ := s.Catalog.Unit("fixbuilder")
	h, err := s.Units.Create(def, 1, numeric16(40), 0, numeric16(40))
	if err != nil {
		t.Fatal(err)
	}
	other := s.Units.Unit(h)
	other.Health, other.MaxHealth, other.Remaining = 100, 100, 0.5
	s.SetLocalInterfaceFacts(true)
	stepOneTick(s)
	f := s.Snapshot.Current()
	if !slices.Equal(f.Interface.Unready, []pool.UnitRef{unitRef(other)}) {
		t.Fatalf("unready %v: want only the other owner's unready unit", f.Interface.Unready)
	}
	other.Remaining = 0
	stepOneTick(s)
	if f := s.Snapshot.Current(); slices.Contains(f.Interface.Ready, unitRef(other)) {
		t.Fatal("the tail walk left the local player's slice")
	}
}
