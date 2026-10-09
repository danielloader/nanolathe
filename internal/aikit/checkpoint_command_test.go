package aikit

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/nanolathe-gg/nanolathe/formats"
	"github.com/nanolathe-gg/nanolathe/internal/ai"
	"github.com/nanolathe-gg/nanolathe/internal/content"
	"github.com/nanolathe-gg/nanolathe/internal/orders"
	"github.com/nanolathe-gg/nanolathe/internal/pool"
	"github.com/nanolathe-gg/nanolathe/internal/sim/checkpoint"
	"github.com/nanolathe-gg/nanolathe/internal/units"
	"github.com/nanolathe-gg/nanolathe/vfs"
)

func checkpointCommandKeys(t *testing.T, cat *content.Catalog) *content.CheckpointKeys {
	t.Helper()
	root := t.TempDir()
	data, err := formats.EncodeThreeDO(&formats.ThreeDO{Root: 0, Objects: []formats.ThreeDOObject{{Version: 1, Name: "base", Selection: -1, Parent: -1, FirstChild: -1, NextSibling: -1, Vertices: []formats.ThreeDOVertex{{}}}}})
	if err != nil {
		t.Fatal(err)
	}
	if err = os.Mkdir(filepath.Join(root, "objects3d"), 0755); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(root, "objects3d", "fixture.3do"), data, 0644); err != nil {
		t.Fatal(err)
	}
	fs := vfs.New()
	if err = fs.MountDirectory(root, 10); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { fs.Close() })
	sources, err := content.CaptureSimulationSources(fs, "")
	if err != nil {
		t.Fatal(err)
	}
	in, err := content.FreezeSimulationInputs(sources, content.SimulationInputRequest{Catalog: cat})
	if err != nil {
		t.Fatal(err)
	}
	keys, err := in.CheckpointKeys()
	if err != nil {
		t.Fatal(err)
	}
	return keys
}

func checkpointCommandFixture(t *testing.T) (*executor, *units.Unit, *UnitInfo) {
	t.Helper()
	def := &content.UnitDef{DefinitionHeader: content.DefinitionHeader{CanonicalKey: "builder"}, UnitName: "builder", ObjectName: "fixture"}
	cat := &content.Catalog{Units: map[string]*content.UnitDef{"builder": def}}
	keys := checkpointCommandKeys(t, cat)
	m := &ai.Manager{Controller: ai.ControllerModern, Catalog: cat, Player: 3}
	if err := m.EnableCheckpointApplications(checkpoint.Identity{}, keys); err != nil {
		t.Fatal(err)
	}
	e := &executor{m: m, table: BuildTable(cat, nil)}
	return e, &units.Unit{Handle: 7, AllocationSerial: 9, Def: def}, e.table.Of(def)
}

func TestCheckpointModernIntentObservesExactObjectsAndOperands(t *testing.T) {
	e, u, p := checkpointCommandFixture(t)
	target := &units.Unit{Handle: 11, AllocationSerial: 13}
	b := &batch{actors: []pool.Handle{7, 7}, inst: []*units.Unit{u, u}, rowNear: -17}
	c := &Command{Kind: CmdReplace, Queued: true, count: 2, Target: 11, target: target, X: -19, Z: 23, Product: p, Slot: -2, Count: -3, Spot: -1, Spacing: -5, Keep: true, Exact: true}
	got, err := e.checkpointCommandIntent(c, b)
	if err != nil {
		t.Fatal(err)
	}
	key, err := e.m.CheckpointApplicationKey(p.Def)
	if err != nil {
		t.Fatal(err)
	}
	actor := checkpoint.Allocation{Handle: 7, Serial: 9}
	tr := checkpoint.Allocation{Handle: 11, Serial: 13}
	want := ai.ModernApplicationIntent{Kind: 11, Queued: true, RawTarget: 11, X: -19, Z: 23, ProductIndex: p.Index, ProductKey: p.Key, Slot: -2, Count: -3, Spot: -1, Spacing: -5, Keep: true, Exact: true, RowNear: -17, Operands: ai.ApplicationOperands{Actors: []checkpoint.Allocation{actor, actor}, Target: &tr, Product: &key}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("intent got %+v want %+v", got, want)
	}
	// Objects may be retired; there is no live-world lookup that substitutes a
	// current occupant or filters out duplicates from the emitted actor list.
	u.Alive, target.Alive = false, false
	if again, err := e.checkpointCommandIntent(c, b); err != nil || !reflect.DeepEqual(again, want) {
		t.Fatal("retired observed operands changed", err)
	}
	foreign := *p
	c.Product = &foreign
	if _, err := e.checkpointCommandIntent(c, b); err == nil {
		t.Fatal("equal-name foreign table row admitted")
	}
	c.Product = p
	p.Def = &content.UnitDef{DefinitionHeader: content.DefinitionHeader{CanonicalKey: "builder"}}
	if _, err := e.checkpointCommandIntent(c, b); err == nil {
		t.Fatal("foreign definition admitted")
	}
}

func TestCheckpointModernIntentRejectsInvalidObservedSpans(t *testing.T) {
	e, u, _ := checkpointCommandFixture(t)
	b := &batch{actors: []pool.Handle{7}, inst: []*units.Unit{u}}
	for _, c := range []Command{{Kind: CmdMove, first: -1}, {Kind: CmdMove, count: -1}, {Kind: CmdMove, first: 1 << 30, count: 1 << 30}} {
		if _, err := e.checkpointCommandIntent(&c, b); err == nil {
			t.Fatal("invalid span admitted")
		}
	}
	c := &Command{Kind: CmdMove, count: 1}
	b.actors[0] = 8
	if _, err := e.checkpointCommandIntent(c, b); err == nil {
		t.Fatal("mismatched actor handle admitted")
	}
	b.actors[0] = 7
	c.Target = 19
	c.target = u
	if _, err := e.checkpointCommandIntent(c, b); err == nil {
		t.Fatal("mismatched target handle admitted")
	}
	if _, err := e.checkpointCommandIntent(nil, b); err == nil {
		t.Fatal("missing command admitted")
	}
}

func TestCheckpointModernCommandOutcomesUseOuterCompletion(t *testing.T) {
	for _, build := range []bool{false, true} {
		e, u, _ := checkpointCommandFixture(t)
		h := e.m.CheckpointApplicationHistory()
		c := &Command{Kind: CmdMove, count: 1}
		b := &batch{actors: []pool.Handle{7}, inst: []*units.Unit{u}}
		state := e.newCheckpointCommand(c, b, 77, h.NextSerial(), 0)
		if state == nil {
			t.Fatal("missing command scope")
		}
		e.checkpointApplication = state
		a := e.checkpointAttempt()
		q := &orders.Queue{}
		restore := a.ObserveQueue(q, u)
		actor := e.checkpointActor(u)
		mark := a.InsertionIndex()
		if build {
			req := ai.BuildRequest{Builder: u.Handle, UnitKey: "builder", Count: 1}
			q.CoalesceTail(orders.Lookup("Move_Ground"), orders.Node{})
			e.checkpointBuildOutcome(mark, actor, req, nil)
		} else {
			q.Push(orders.Lookup("Move_Ground"), orders.Node{})
			e.checkpointOrderOutcome(mark, actor, 1)
		}
		if !state.accepted || state.rejected {
			t.Fatal("actual completed work was rejected")
		}
		// No new completion must never borrow an earlier successful one.
		mark = a.InsertionIndex()
		e.checkpointOrderOutcome(mark, actor, 1)
		if !state.rejected {
			t.Fatal("stale completion reported success")
		}
		restore()
		state.finish(2)
		e.checkpointApplication = nil
		if snap, err := h.Snapshot(); err != nil || snap.Count != 1 {
			t.Fatal(snap, err)
		}
	}
}

func TestCheckpointModernTerminalFramingAndDisabled(t *testing.T) {
	for _, tc := range []struct {
		accept, reject, noop, commit bool
		apm, terminal                uint8
	}{
		{false, false, true, false, 1, 1}, {true, false, false, true, 2, 2}, {false, true, false, false, 2, 3},
		{true, true, false, true, 2, 4}, {false, true, false, true, 2, 4}, {false, false, false, false, 3, 3},
	} {
		e, u, _ := checkpointCommandFixture(t)
		h := e.m.CheckpointApplicationHistory()
		c := &Command{Kind: CmdStop, count: 1}
		b := &batch{actors: []pool.Handle{7}, inst: []*units.Unit{u}}
		state := e.newCheckpointCommand(c, b, 1, h.NextSerial(), 0)
		e.checkpointApplication = state
		if tc.accept {
			e.checkpointAccept()
		}
		if tc.reject {
			e.checkpointReject()
		}
		if tc.noop {
			e.checkpointNoop()
		}
		actor := e.checkpointActor(u)
		if tc.commit {
			state.attempt.RecordActivation(actor, true)
		}
		intent, err := e.checkpointCommandIntent(c, b)
		if err != nil {
			t.Fatal(err)
		}
		expected, err := ai.NewApplicationHistory(checkpoint.Identity{}, 3, 2)
		if err != nil {
			t.Fatal(err)
		}
		a := expected.BeginAttempt(1, expected.NextSerial(), 0, intent.WriteCheckpoint)
		if tc.commit {
			a.RecordActivation(actor, true)
		}
		a.Finish(tc.apm, tc.terminal)
		state.finish(tc.apm)
		e.checkpointApplication = nil
		got, err := h.Snapshot()
		if err != nil {
			t.Fatal(err)
		}
		want, err := expected.Snapshot()
		if err != nil || got != want {
			t.Fatal("terminal framing", got, want, err)
		}
	}
	e := &executor{}
	if n := testing.AllocsPerRun(20, func() {
		if e.newCheckpointCommand(nil, nil, 0, 0, 0) != nil {
			panic("disabled scope")
		}
		e.checkpointAccept()
		e.checkpointReject()
		e.checkpointNoop()
		e.checkpointActor(nil)
		e.checkpointOrderOutcome(0, checkpoint.Allocation{}, 1)
		e.checkpointBuildOutcome(0, checkpoint.Allocation{}, ai.BuildRequest{}, nil)
	}); n != 0 {
		t.Fatal("disabled command observation allocated", n)
	}
	e, u, _ := checkpointCommandFixture(t)
	h := e.m.CheckpointApplicationHistory()
	c := &Command{Kind: CmdProduce, count: 1}
	b := &batch{actors: []pool.Handle{7}, inst: []*units.Unit{u}}
	state := e.newCheckpointCommand(c, b, 1, h.NextSerial(), 0)
	e.checkpointApplication = state
	e.checkpointBuildOutcome(0, e.checkpointActor(u), ai.BuildRequest{}, errors.New("unknown producer refusal"))
	state.finish(2)
	e.checkpointApplication = nil
	if _, err := h.Snapshot(); err == nil {
		t.Fatal("unknown typed error accepted")
	}
}
