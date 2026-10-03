package main

import (
	"slices"
	"testing"

	"github.com/nanolathe-gg/nanolathe/internal/content"
	"github.com/nanolathe-gg/nanolathe/internal/frame"
	"github.com/nanolathe-gg/nanolathe/internal/hud"
	"github.com/nanolathe-gg/nanolathe/internal/orders"
	"github.com/nanolathe-gg/nanolathe/internal/pool"
	"github.com/nanolathe-gg/nanolathe/internal/session"
	"github.com/nanolathe-gg/nanolathe/internal/sim/numeric"
	"github.com/nanolathe-gg/nanolathe/internal/units"
)

// localBatchFixture is a battle with three published constructors of the
// local player, ready to select from.
func localBatchFixture(t *testing.T) (*battleSession, [3]*units.Unit) {
	t.Helper()
	b := newTestBattle(testCatalogON05(), testWorldON05(40, 40))
	b.sess.LocalOwner = 0
	var us [3]*units.Unit
	for i := range us {
		us[i] = placeUnit(b, "armcons", numeric.Fixed(int64(100+60*i)<<16), numeric.Fixed(120<<16))
	}
	applyPendingBattleCommands(b)
	return b, us
}

func unitRefOf(u *units.Unit) pool.UnitRef {
	return pool.UnitRef{Handle: u.Handle, Serial: u.AllocationSerial}
}

// Within one input batch a later command acts on the selection an earlier
// selection command produced: selection applies to the local state at once
// and every order resolves its units from it when it is sent
// (DESIGN_MULTIPLAYER §7.3, §16.2 M2-C6). Nothing selection-related reaches
// the session queue.
func TestSelectThenOrderInOneInputBatch(t *testing.T) {
	b, us := localBatchFixture(t)
	submit := func(c session.HumanCommand) {
		t.Helper()
		if err := b.enqueueHumanCommand(c); err != nil {
			t.Fatal(err)
		}
	}
	submit(session.HumanCommand{Kind: session.HumanSelectionReplace, Selection: session.HumanSelectionCommand{Handles: []pool.Handle{us[2].Handle, us[0].Handle}}})
	if err := b.DispatchOrderCommand(session.HumanOrderCommand{Code: 2, Position: orders.ResolvePos{X: 300 << 16, Z: 300 << 16}}); err != nil {
		t.Fatal(err)
	}
	submit(session.HumanCommand{Kind: session.HumanSelectionToggle, Selection: session.HumanSelectionCommand{Handles: []pool.Handle{us[0].Handle, us[1].Handle}}})
	if err := b.dispatchStopCommand(); err != nil {
		t.Fatal(err)
	}
	submit(session.HumanCommand{Kind: session.HumanGroupAssign, Group: session.HumanGroupCommand{Group: 5}})
	pending := b.sess.PendingHumanCommands()
	if len(pending) != 3 || pending[0].Kind != session.HumanOrder || pending[1].Kind != session.HumanStop || pending[2].Kind != session.HumanGroupAssign {
		t.Fatalf("queued %+v, want the order, the stop and the assignment alone", pending)
	}
	if got := pending[0].Order.Handles; !slices.Equal(got, []pool.Handle{us[0].Handle, us[2].Handle}) {
		t.Fatalf("order actors %v, want the batch's first selection in slot order", got)
	}
	want := []pool.Handle{us[1].Handle, us[2].Handle}
	if got := pending[1].Stop.Handles; !slices.Equal(got, want) {
		t.Fatalf("stop actors %v, want the toggled selection %v", got, want)
	}
	if got := pending[2].Group.Handles; !slices.Equal(got, want) {
		t.Fatalf("assignment membership %v, want the toggled selection %v", got, want)
	}
	applyPendingBattleCommands(b)
	if us[1].Group != 5 || us[2].Group != 5 || us[0].Group != 0 {
		t.Fatal("the assignment's explicit membership did not apply")
	}
	if q := orders.QueueOfUnit(us[0]); q == nil || q.Head() == nil || orders.DescriptorFor(q.Head().ID).Name != "Move_Ground" {
		t.Fatal("the order did not reach the unit selected when it was sent")
	}
}

// Group assignment is a seat command; recall is local selection. A recall
// that follows an assignment in the same batch recalls the assignment's
// membership before any tick publishes it, as phase 1 applied both in order
// [07 §9].
func TestGroupAssignThenRecallInOneInputBatch(t *testing.T) {
	b, us := localBatchFixture(t)
	if err := b.enqueueHumanCommand(session.HumanCommand{Kind: session.HumanSelectionReplace, Selection: session.HumanSelectionCommand{Handles: []pool.Handle{us[1].Handle}}}); err != nil {
		t.Fatal(err)
	}
	if err := b.DispatchGroupAssign(3); err != nil {
		t.Fatal(err)
	}
	if err := b.enqueueHumanCommand(session.HumanCommand{Kind: session.HumanSelectionClear}); err != nil {
		t.Fatal(err)
	}
	if err := b.DispatchGroupRecall(3, false); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(b.localState().SelectedRefs(), []pool.UnitRef{unitRefOf(us[1])}) {
		t.Fatalf("recall before the assignment's tick selected %v", b.localState().SelectedRefs())
	}
	if pending := b.sess.PendingHumanCommands(); len(pending) != 1 || pending[0].Kind != session.HumanGroupAssign {
		t.Fatalf("queued %+v, want the assignment alone", pending)
	}
	applyPendingBattleCommands(b)
	b.localState().ClearSelection()
	if err := b.DispatchGroupRecall(3, false); err != nil {
		t.Fatal(err)
	}
	f, _ := b.currentSnapshot()
	if !slices.Equal(f.Selection.Handles, []pool.Handle{us[1].Handle}) {
		t.Fatalf("recall after publication composed %v", f.Selection.Handles)
	}
}

// A catch-up batch whose newest publication alone reaches the host still
// advances the local state across every tick: the facts are retained in tick
// order and drained before the next input (M2-C6 skipped display frames).
func TestHostLocalStateCatchesUpAcrossABatch(t *testing.T) {
	b, us := localBatchFixture(t)
	b.localState().ReplaceSelection([]pool.UnitRef{unitRefOf(us[0]), unitRefOf(us[1])})
	b.sess.PhaseObserver = func(phase string, tick uint32) {
		if tick != b.sess.Clock.GlobalTick || tick != 3 {
			return
		}
		switch phase {
		case "phase1-network":
			us[1].Remaining = 0.5
		case "phase2-units":
			us[1].Remaining = 0
		}
	}
	start := b.sess.Clock.GlobalTick
	if start >= 3 {
		t.Fatalf("fixture starts at tick %d", start)
	}
	b.sess.Step(b.sess.Clock.ScaledAnchor + 5)
	b.sess.PhaseObserver = nil
	if b.sess.Clock.GlobalTick < 3 {
		t.Fatalf("batch ended at tick %d", b.sess.Clock.GlobalTick)
	}
	b.syncLocalInterface()
	if hostSelected(b, us[1]) || !hostSelected(b, us[0]) {
		t.Fatalf("selection %v after the batch: the transient unready unit must have left", b.localState().SelectedRefs())
	}
	f, _ := b.currentSnapshot()
	if !slices.Equal(f.Selection.Handles, []pool.Handle{us[0].Handle}) {
		t.Fatalf("the presented frame composed %v", f.Selection.Handles)
	}
}

// A host that stops draining loses the facts of every tick past the frame
// buffer's bound. The retained ticks still apply in order; over the lost ones
// the host resynchronises from the newest frame: a selected unit not ready
// there leaves, and one unready only inside the gap stays, the documented
// residue (hud.LocalInterface.ResyncAfterGap). Facts flow again afterwards.
func TestHostResyncsWhenTickFactsOverflow(t *testing.T) {
	b, us := localBatchFixture(t)
	b.syncLocalInterface()
	b.localState().ReplaceSelection([]pool.UnitRef{unitRefOf(us[0]), unitRefOf(us[1]), unitRefOf(us[2])})
	start := b.sess.Clock.GlobalTick
	const span = 300 // more ticks than the buffer retains
	b.sess.PhaseObserver = func(phase string, tick uint32) {
		if tick != b.sess.Clock.GlobalTick {
			return
		}
		switch {
		case tick == start+10 && phase == "phase1-network": // retained
			us[0].Remaining = 0.5
		case tick == start+10 && phase == "phase2-units":
			us[0].Remaining = 0
		case tick == start+280 && phase == "phase1-network": // lost
			us[1].Remaining = 0.5
		case tick == start+280 && phase == "phase2-units":
			us[1].Remaining = 0
		case tick == start+295 && phase == "phase1-network": // lost, and still unready at the end
			us[2].Remaining = 0.5
		}
	}
	for b.sess.Clock.GlobalTick < start+span {
		b.sess.Step(b.sess.Clock.ScaledAnchor + 1)
	}
	b.sess.PhaseObserver = nil
	if dropped, newest := b.sess.Snapshot.InterfaceFactsDropped(); dropped == 0 || newest != start+span {
		t.Fatalf("fixture: dropped %d ticks up to %d, want a gap ending at %d", dropped, newest, start+span)
	}

	b.syncLocalInterface()
	if b.localGap {
		t.Fatal("the gap was not resynchronised at the newest frame")
	}
	if got, want := b.localState().SelectedRefs(), []pool.UnitRef{unitRefOf(us[1])}; !slices.Equal(got, want) {
		t.Fatalf("selection %v, want %v: the retained tick drops us[0], the resync us[2]", got, want)
	}

	b.sess.PhaseObserver = func(phase string, tick uint32) {
		switch phase {
		case "phase1-network":
			us[1].Remaining = 0.5
		case "phase2-units":
			us[1].Remaining = 0
		}
	}
	b.sess.Step(b.sess.Clock.ScaledAnchor + 1)
	b.sess.PhaseObserver = nil
	b.syncLocalInterface()
	if b.localState().SelectionCount() != 0 {
		t.Fatalf("selection %v: facts after the gap did not apply", b.localState().SelectedRefs())
	}
}

// A battle entered from a retail save starts with the selection its restored
// status words carry: attaching the local state adopts it once, by allocation
// reference (hud.LocalInterface.AdoptStatusWords).
func TestAttachAdoptsTheRestoredSelection(t *testing.T) {
	b, us := localBatchFixture(t)
	us[1].Flags |= units.SelectedStatus // as a restore writes it
	b.sess.Step(b.sess.Clock.ScaledAnchor + 1)
	b.attachLocalInterface()
	if got := b.localState().SelectedRefs(); !slices.Equal(got, []pool.UnitRef{unitRefOf(us[1])}) {
		t.Fatalf("selection %v, want the restored unit", got)
	}
}

// An admitted seat command names its actors; no client's local selection can
// reach it. Two identical battles whose hosts hold different selections reach
// the same state from the same stamped commands, an empty actor list included
// (M2-C7).
func TestStampedSeatCommandIgnoresLocalSelection(t *testing.T) {
	run := func(selected int) (string, [3]*units.Unit) {
		b, us := localBatchFixture(t)
		b.localState().ReplaceSelection([]pool.UnitRef{unitRefOf(us[selected])})
		tick := b.sess.Clock.GlobalTick + 1
		stop := session.SeatCommand{Kind: session.SeatStop, Stop: session.StopPayload{Actors: []pool.UnitRef{unitRefOf(us[0])}}}
		if err := b.sess.EnqueueSeatCommand(session.CommandStamp{Seat: 0, Tick: tick, Position: 1}, stop); err != nil {
			t.Fatal(err)
		}
		if err := b.sess.EnqueueSeatCommand(session.CommandStamp{Seat: 0, Tick: tick, Position: 2}, session.SeatCommand{Kind: session.SeatStop}); err != nil {
			t.Fatal(err)
		}
		applyPendingBattleCommands(b)
		receipts := b.sess.DrainCommandReceipts()
		if len(receipts) != 2 || receipts[0].Outcome != session.CommandApplied || receipts[1].Outcome != session.CommandNoOp {
			t.Fatalf("receipts %+v", receipts)
		}
		hash, err := b.sess.PartialStateFingerprint()
		if err != nil {
			t.Fatal(err)
		}
		return hash, us
	}
	first, us := run(1)
	second, _ := run(2)
	if first != second {
		t.Fatalf("different local selections reached different states: %s / %s", first, second)
	}
	for _, u := range us[1:] {
		if q := orders.QueueOfUnit(u); q != nil && q.LenPrimary() != 0 {
			t.Fatal("a stamped command fell back to the local selection")
		}
	}
}

// testSelection is a client's local selection of exactly us, for tests that
// drive a raw session before a battle host exists. Assign it to a battle's
// local field once one does.
func testSelection(us ...*units.Unit) *hud.LocalInterface {
	local := hud.NewLocalInterface()
	refs := make([]pool.UnitRef, 0, len(us))
	for _, u := range us {
		if u != nil {
			refs = append(refs, unitRefOf(u))
		}
	}
	local.ReplaceSelection(refs)
	return local
}

// composedFrame is the session's current frame with local composed onto it,
// as the battle host composes every frame it presents
// (battle_local_interface.go).
func composedFrame(sess *session.Session, cat *content.Catalog, local *hud.LocalInterface) *frame.Frame {
	f := sess.Snapshot.Current()
	if f != nil && local.ComposeSelection(f) {
		hud.ComposeCommandPage(&f.CommandPage, f, cat, sess.CommandPageProducts)
	}
	return f
}
