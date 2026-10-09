package units

import (
	"bytes"
	"errors"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/nanolathe-gg/nanolathe/internal/cob"
	"github.com/nanolathe-gg/nanolathe/internal/content"
	"github.com/nanolathe-gg/nanolathe/internal/model"
	"github.com/nanolathe-gg/nanolathe/internal/pool"
	"github.com/nanolathe-gg/nanolathe/internal/sim/checkpoint"
	"github.com/nanolathe-gg/nanolathe/internal/sim/rng"
)

type checkpointUnitScriptFixture struct {
	w         *World
	inputs    *content.SimulationInputs
	keys      *content.CheckpointKeys
	authority *checkpoint.BindingAuthority
	mdl       *model.Model
	sim       *rng.Simulation
}

func newCheckpointUnitScriptFixture(t *testing.T) checkpointUnitScriptFixture {
	t.Helper()
	w, inputs := checkpointFixture(t)
	keys, err := inputs.CheckpointKeys()
	if err != nil {
		t.Fatal(err)
	}
	mdl, ok := inputs.Model("fixture")
	if !ok {
		t.Fatal("missing frozen fixture model")
	}
	sim := rng.NewSimulation(31)
	a := checkpoint.NewBindingAuthority()
	w.SetSimulationRNGWithCheckpointBinding(&sim, a)
	return checkpointUnitScriptFixture{w, inputs, keys, a, mdl, &sim}
}

func (f checkpointUnitScriptFixture) install(admitted bool, visible func(int, int32) bool, sink cob.PresentationSink, pre func(*Unit, *cob.Binding) error) {
	f.w.SetCOBBinderWithCheckpointBinding(func(u *Unit) error {
		var authority *checkpoint.BindingAuthority
		if admitted {
			authority = f.authority
		}
		_, err := BindCOBForUnitWithCheckpointBinding(f.inputs.Filesystem(), u, f.mdl, f.sim, sink, visible, func(b *cob.Binding) error {
			if pre != nil {
				return pre(u, b)
			}
			return nil
		}, authority)
		return err
	}, f.inputs.Filesystem(), f.authority)
}

func (f checkpointUnitScriptFixture) context(t *testing.T) *CheckpointContext {
	t.Helper()
	c := NewCheckpointContext(f.keys)
	if err := c.SetLifecycleBindings(f.w, f.authority); err != nil {
		t.Fatal(err)
	}
	if err := c.SetWorldBindings(f.w, f.inputs, nil, f.sim, nil, f.authority); err != nil {
		t.Fatal(err)
	}
	if _, err := f.w.CollectCheckpointReferences(c); err != nil {
		t.Fatal(err)
	}
	return c
}

func (f checkpointUnitScriptFixture) prepare(t *testing.T, c *CheckpointContext, u *Unit) *cob.CheckpointContext {
	t.Helper()
	lower, err := c.ScriptBindings(u)
	if err != nil {
		t.Fatal(err)
	}
	b := u.ScriptState.Binding
	if err := lower.SetRuntimeSources(b, f.sim, u.RenderPieceFlags, b.PieceMap); err != nil {
		t.Fatal(err)
	}
	return lower
}

func checkpointScriptBytes(t *testing.T, w *World, c *CheckpointContext) []byte {
	t.Helper()
	var out bytes.Buffer
	if err := w.WriteScriptCheckpoint(checkpoint.NewEncoder(&out), c); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}

func checkpointScriptRefused(t *testing.T, w *World, c *CheckpointContext) {
	t.Helper()
	var out bytes.Buffer
	e := checkpoint.NewEncoder(&out)
	if err := w.WriteScriptCheckpoint(e, c); err == nil || out.Len() != 0 {
		t.Fatalf("script preflight = %v, %d bytes", err, out.Len())
	}
	e.U8(1)
	if out.Len() != 0 {
		t.Fatal("script refusal was not sticky")
	}
}

func TestCheckpointScriptReceiptsSealActualCreation(t *testing.T) {
	for _, forced := range []bool{false, true} {
		t.Run(map[bool]string{false: "normal", true: "forced"}[forced], func(t *testing.T) {
			f := newCheckpointUnitScriptFixture(t)
			preCalls := 0
			f.install(true, nil, nil, func(u *Unit, b *cob.Binding) error {
				preCalls++
				if u.AllocationSerial != 0 || u.GetScript() != b.VM || u.checkpointScript.owner != u {
					t.Fatal("pre-create identity/order changed")
				}
				key, err := f.keys.ProgramForUnit(u.Def, b.Program)
				if err != nil {
					t.Fatal(err)
				}
				pending := &cob.CheckpointContext{Program: key}
				if err := pending.SetOwnerBindings(b.VM, checkpoint.Allocation{Handle: uint32(u.Handle), Serial: 1}, f.authority); err != nil {
					t.Fatal(err)
				}
				if err := pending.SetRuntimeSources(b, f.sim, u.RenderPieceFlags, b.PieceMap); err != nil {
					t.Fatal(err)
				}
				if err := b.VM.ValidateCheckpointBindings(pending); err == nil {
					t.Fatal("pending receipts captured before successful allocation")
				}
				return nil
			})
			var h pool.Handle
			var err error
			if forced {
				h, err = f.w.CreateWithForcedSlot(f.w.catalog.Units["armdef"], 0, 0, 0, 0, 2)
			} else {
				h, err = f.w.Create(f.w.catalog.Units["armdef"], 0, 0, 0, 0)
			}
			if err != nil {
				t.Fatal(err)
			}
			u := f.w.Unit(h)
			if preCalls != 1 || u.AllocationSerial != 1 || f.w.lastAllocationSerial != 1 || u.checkpointScript.err != nil || len(u.checkpointScript.ports) != 0 || len(u.checkpointScript.runtime) != 0 {
				t.Fatal("creation did not seal and release its own receipts")
			}
			if u.Script.DrainCalls != 1 || !u.ScriptState.Binding.CreateInvoked {
				t.Fatal("Create barrier changed")
			}
			c := f.context(t)
			lower := f.prepare(t, c, u)
			if err := u.Script.ValidateCheckpointBindings(lower); err != nil {
				t.Fatal(err)
			}
			first := checkpointScriptBytes(t, f.w, c)
			u.RetainCheckpointPortInstallation(u.Script.BindPortWithPendingCheckpointBinding(77, func([]int32) int32 { panic("capture called late port") }, f.authority))
			u.RetainCheckpointVMInstallation(u.Script.BindTransportMutationsWithPendingCheckpointBinding(func(int32, int32, int32) { panic("attach") }, nil, f.authority)[0])
			if u.checkpointScript.err != nil || len(u.checkpointScript.ports) != 0 || len(u.checkpointScript.runtime) != 0 {
				t.Fatal("post-allocation receipts were not sealed immediately")
			}
			if got := checkpointScriptBytes(t, f.w, c); bytes.Equal(first, got) {
				t.Fatal("late retained bindings did not enter script bytes")
			}
		})
	}
}

func TestCheckpointScriptReceiptsNestedAndFailedCreation(t *testing.T) {
	f := newCheckpointUnitScriptFixture(t)
	var outer, inner *Unit
	f.install(true, nil, nil, func(u *Unit, _ *cob.Binding) error {
		if outer == nil {
			outer = u
			h, err := f.w.Create(f.w.catalog.Units["cordef"], 0, 0, 0, 0)
			if err != nil {
				return err
			}
			inner = f.w.Unit(h)
			if inner.AllocationSerial != 1 || outer.AllocationSerial != 0 {
				t.Fatal("nested creation did not commit independently")
			}
		}
		return nil
	})
	if _, err := f.w.Create(f.w.catalog.Units["armdef"], 0, 0, 0, 0); err != nil {
		t.Fatal(err)
	}
	if outer.AllocationSerial != 2 || inner.AllocationSerial != 1 {
		t.Fatal("predicted serial replaced actual completion order")
	}
	c := f.context(t)
	f.prepare(t, c, outer)
	f.prepare(t, c, inner)
	checkpointScriptBytes(t, f.w, c)
	for _, forced := range []bool{false, true} {
		f := newCheckpointUnitScriptFixture(t)
		var failed *Unit
		want := errors.New("authored pre-create refusal")
		f.install(true, nil, nil, func(u *Unit, _ *cob.Binding) error { failed = u; return want })
		var err error
		if forced {
			_, err = f.w.CreateWithForcedSlot(f.w.catalog.Units["armdef"], 0, 0, 0, 0, 2)
		} else {
			_, err = f.w.Create(f.w.catalog.Units["armdef"], 0, 0, 0, 0)
		}
		if err == nil || !strings.Contains(err.Error(), want.Error()) || failed == nil || failed.AllocationSerial != 0 || f.w.lastAllocationSerial != 0 || f.w.pendingAllocationSerials != 0 || f.w.Unit(failed.Handle) != nil {
			t.Fatal("failed creation committed an allocation")
		}
		if len(failed.checkpointScript.ports) == 0 || len(failed.checkpointScript.runtime) == 0 {
			t.Fatal("failed creation sealed or released its pending proof")
		}
	}
}

func TestCheckpointScriptReceiptFailureIsDiagnosticOnly(t *testing.T) {
	for _, stalePort := range []bool{false, true} {
		f := newCheckpointUnitScriptFixture(t)
		var retained *Unit
		f.install(true, nil, nil, func(u *Unit, b *cob.Binding) error {
			retained = u
			if stalePort {
				fn := func([]int32) int32 { return 3 }
				u.RetainCheckpointPortInstallation(b.VM.BindPortWithPendingCheckpointBinding(77, fn, f.authority))
				b.VM.BindPort(77, fn)
			} else {
				u.RetainCheckpointVMInstallation(b.VM.SetSFXVisibleWithPendingCheckpointBinding(func(int, int32) bool { return true }, f.authority))
				b.VM.SetSFXVisible(nil)
			}
			return nil
		})
		calls := 0
		f.w.SetCreateHookWithCheckpointBinding(func(_ pool.Handle, u *Unit) {
			calls++
			if u.checkpointScript.err == nil {
				t.Fatal("hook preceded diagnostic sealing")
			}
		}, f.authority)
		h, err := f.w.Create(f.w.catalog.Units["armdef"], 0, 0, 0, 0)
		if err != nil || h == 0 || calls != 1 || retained.AllocationSerial != 1 || !retained.ScriptState.Binding.CreateInvoked {
			t.Fatalf("proof failure changed creation: %v", err)
		}
		first := retained.checkpointScript.err
		retained.RetainCheckpointPortInstallation(&cob.CheckpointPortInstallation{})
		retained.RetainCheckpointVMInstallation(&cob.CheckpointVMInstallation{})
		if retained.checkpointScript.err != first {
			t.Fatal("later refusal overwrote first diagnostic")
		}
		c := f.context(t)
		if _, err := c.ScriptBindings(retained); err == nil {
			t.Fatal("failed receipt admitted a context")
		}
		checkpointScriptRefused(t, f.w, c)
	}
}

func TestCheckpointScriptReceiptOwnerAndOrdinaryPath(t *testing.T) {
	f := newCheckpointUnitScriptFixture(t)
	f.install(false, nil, nil, nil)
	u := checkpointCreate(t, f.w, "armdef", 0)
	if !reflect.DeepEqual(u.checkpointScript, checkpointUnitScriptInstallation{}) {
		t.Fatal("nil authority created receipt metadata")
	}
	c := f.context(t)
	f.prepare(t, c, u)
	checkpointScriptRefused(t, f.w, c)
	u.RetainCheckpointPortInstallation(nil)
	u.RetainCheckpointVMInstallation(nil)
	if u.checkpointScript.err != nil {
		t.Fatal("nil receipts recorded a failure")
	}
	u.RetainCheckpointPortInstallation(&cob.CheckpointPortInstallation{})
	if u.checkpointScript.err == nil {
		t.Fatal("receipt without admission was accepted")
	}
	f = newCheckpointUnitScriptFixture(t)
	f.install(true, nil, nil, nil)
	u = checkpointCreate(t, f.w, "armdef", 0)
	copied := *u
	copied.RetainCheckpointVMInstallation(&cob.CheckpointVMInstallation{})
	if copied.checkpointScript.err == nil || u.checkpointScript.err != nil {
		t.Fatal("copy reused or corrupted original unit proof")
	}
	c = f.context(t)
	if _, err := c.Allocations.Add(&copied); err != nil {
		t.Fatal(err)
	}
	if _, err := c.ScriptBindings(&copied); err == nil {
		t.Fatal("copied unit metadata accepted")
	}
	if _, err := c.ScriptBindings(u); err != nil {
		t.Fatal("original unit proof changed", err)
	}
}

func TestCheckpointScriptContextRejectsChangesBeforeBytes(t *testing.T) {
	for _, tc := range []struct {
		name string
		edit func(*Unit, *cob.CheckpointContext)
	}{
		{"VM", func(u *Unit, _ *cob.CheckpointContext) { u.Script = cob.NewVM(u.Def.Script) }},
		{"allocation", func(u *Unit, _ *cob.CheckpointContext) { u.AllocationSerial++ }},
		{"binding", func(u *Unit, _ *cob.CheckpointContext) { b := *u.ScriptState.Binding; u.ScriptState.Binding = &b }},
		{"missing binding", func(u *Unit, _ *cob.CheckpointContext) { u.ScriptState.Binding = nil }},
		{"render backing", func(u *Unit, _ *cob.CheckpointContext) { u.RenderPieceFlags = slices.Clone(u.RenderPieceFlags) }},
		{"render length", func(u *Unit, _ *cob.CheckpointContext) { u.RenderPieceFlags = nil }},
		{"program key", func(_ *Unit, c *cob.CheckpointContext) { c.Program.Ordinal++ }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newCheckpointUnitScriptFixture(t)
			f.install(true, nil, nil, nil)
			u := checkpointCreate(t, f.w, "armdef", 0)
			c := f.context(t)
			lower := f.prepare(t, c, u)
			if again, err := c.ScriptBindings(u); err != nil || again != lower {
				t.Fatal("repeat context changed", err)
			}
			tc.edit(u, lower)
			if _, err := c.ScriptBindings(u); err == nil {
				t.Fatal("changed tuple reused context")
			}
			checkpointScriptRefused(t, f.w, c)
		})
	}
	f := newCheckpointUnitScriptFixture(t)
	f.install(true, nil, nil, nil)
	u := checkpointCreate(t, f.w, "armdef", 0)
	for _, c := range []*CheckpointContext{nil, NewCheckpointContext(nil), NewCheckpointContext(f.keys)} {
		if _, err := c.ScriptBindings(u); err == nil {
			t.Fatal("incomplete context accepted")
		}
	}
	c := NewCheckpointContext(f.keys)
	if err := c.SetLifecycleBindings(f.w, f.authority); err != nil {
		t.Fatal(err)
	}
	if _, err := c.ScriptBindings(u); err == nil {
		t.Fatal("undiscovered unit admitted")
	}
	if _, err := c.Allocations.Add(u); err != nil {
		t.Fatal(err)
	}
	if _, err := c.ScriptBindings(u); err == nil {
		t.Fatal("undiscovered VM admitted")
	}
	c = f.context(t)
	f.prepare(t, c, u)
	checkpointScriptRefused(t, NewSliced(2, f.w.catalog), c)
}

type checkpointScriptSink struct{ calls int }

func (s *checkpointScriptSink) EmitCOBEvent(cob.PresentationEvent) {
	s.calls++
	panic("capture called sink")
}

func TestCheckpointScriptPresenceAndPureCapture(t *testing.T) {
	f := newCheckpointUnitScriptFixture(t)
	sink := &checkpointScriptSink{}
	f.install(true, func(int, int32) bool { panic("capture called visibility") }, sink, nil)
	u := checkpointCreate(t, f.w, "armdef", 0)
	c := f.context(t)
	lower := f.prepare(t, c, u)
	if err := cob.SetCheckpointPresentationSink(lower, sink); err != nil {
		t.Fatal(err)
	}
	beforeRNG, beforeThreads := *f.sim, u.Script.Threads
	beforeFlags := slices.Clone(u.RenderPieceFlags)
	beforeDrain := u.Script.DrainCalls
	data := checkpointScriptBytes(t, f.w, c)
	// Root prefix is independently framed: one allocation, table1/id1,
	// table4/id1, state+binding present, then model family3/ordinal0/key.
	prefix := []byte{1, 0, 0, 0, 1, 0, 1, 0, 0, 0, 4, 0, 1, 0, 0, 0, 1, 1, 3, 0, 0, 0, 0, 21, 0, 0, 0}
	prefix = append(prefix, []byte("objects3d/fixture.3do")...)
	// PresentationSink, SFXSink, SFXVisible, SimulationRNG then bridge/create.
	prefix = append(prefix, 1, 0, 1, 1, 1, 1)
	if !bytes.HasPrefix(data, prefix) {
		t.Fatalf("script prefix = %x; want %x", data[:len(prefix)], prefix)
	}
	for range 3 {
		if !bytes.Equal(data, checkpointScriptBytes(t, f.w, c)) {
			t.Fatal("repeat capture changed bytes")
		}
	}
	if *f.sim != beforeRNG || u.Script.Threads != beforeThreads || !slices.Equal(beforeFlags, u.RenderPieceFlags) || u.Script.DrainCalls != beforeDrain || sink.calls != 0 {
		t.Fatal("capture invoked gameplay or changed state")
	}
	// A paired visibility replacement is retained and sealed immediately.
	u.RetainCheckpointVMInstallation(u.ScriptState.Binding.SetSFXSinkWithPendingCheckpointBinding(cob.PresentationSinkAdapter{Sink: sink}, func(int, int32) bool { panic("visibility") }, f.authority))
	prefix[len(prefix)-5] = 1
	if got := checkpointScriptBytes(t, f.w, c); !bytes.HasPrefix(got, prefix) {
		t.Fatal("actual SFXSink presence not retained")
	}
	// A later ordinary replacement cannot borrow the original reader proof.
	u.Script.SetSFXVisible(func(int, int32) bool { panic("ordinary reader") })
	checkpointScriptRefused(t, f.w, c)
}

// The admitted path must preserve Create's engine writes and touched marker,
// the callback trace, both existing allocation draws and the later create
// hook. Proof refusal is a diagnostic, never a creation outcome (§16.3.60;
// [04 R-CB-01 §4], [04 R-COB-06]). This comparison does not capture content.
func TestCheckpointScriptInstallationPreservesCreation(t *testing.T) {
	type result struct {
		unit    Unit
		threads [8]cob.Thread
		trace   []cob.LifecycleEvent
		sim     rng.Simulation
		drains  int
		hooks   int
	}
	for _, forced := range []bool{false, true} {
		for _, fail := range []bool{false, true} {
			var baseline result
			for mode := 0; mode < 3; mode++ {
				f := newCheckpointUnitScriptFixture(t)
				def := f.w.catalog.Units["armdef"]
				// Authored Create writes INBUILDSTANCE and BUSY before return.
				def.Script = &cob.Program{Code: []uint32{
					0x10021001, 5, 0x10021001, 1, 0x10082000,
					0x10021001, 6, 0x10021001, 1, 0x10082000,
					0x10021001, 0, 0x10065000,
				}, Scripts: map[string]int{"Create": 0}, ScriptsByID: []int{0}, Pieces: []string{"base"}}
				def.BuildAngle, def.ActivateWhenBuilt = 2048, true
				var u *Unit
				var trace []cob.LifecycleEvent
				f.install(mode != 0, nil, nil, func(current *Unit, b *cob.Binding) error {
					u = current
					b.Callbacks.SetLifecycleSink(func(event cob.LifecycleEvent) { trace = append(trace, event) })
					if mode == 2 {
						u.RetainCheckpointPortInstallation(b.VM.BindPortWithPendingCheckpointBinding(77, func([]int32) int32 { return 0 }, f.authority))
						b.VM.BindPort(77, nil)
					}
					if fail {
						return errors.New("authored creation refusal")
					}
					return nil
				})
				hooks := 0
				f.w.SetCreateHook(func(pool.Handle, *Unit) { hooks++ })
				beforeRNG := *f.sim
				var err error
				if forced {
					_, err = f.w.CreateWithForcedSlot(def, 0, 0, 0, 0, 2)
				} else {
					_, err = f.w.Create(def, 0, 0, 0, 0)
				}
				if fail {
					var bindingError *cob.BindingError
					if !errors.As(err, &bindingError) || !bindingError.Has(cob.BindingInvalidRequest) || hooks != 0 {
						t.Fatalf("creation refusal changed: %v", err)
					}
				} else {
					if err != nil || hooks != 1 || !u.InBuildStance || !u.Busy || u.Pending&PendingScriptTouched == 0 || !u.Activated {
						t.Fatalf("Create effects changed: %v", err)
					}
					if (u.checkpointScript.err != nil) != (mode == 2) {
						t.Fatal("receipt failure did not remain diagnostic only")
					}
				}
				if *f.sim == beforeRNG {
					t.Fatal("fixture did not exercise allocation RNG")
				}
				copyUnit := *u
				copyUnit.Def, copyUnit.Script, copyUnit.ScriptState = nil, nil, nil
				copyUnit.checkpointScript = checkpointUnitScriptInstallation{}
				got := result{copyUnit, u.Script.Threads, trace, *f.sim, u.Script.DrainCalls, hooks}
				if mode == 0 {
					baseline = got
				} else if !reflect.DeepEqual(got, baseline) {
					t.Fatalf("mode %d changed creation, forced=%v failure=%v", mode, forced, fail)
				}
			}
		}
	}
}

func TestCheckpointScriptContextEmptyFlagsAndMissingVM(t *testing.T) {
	w, inputs, u := checkpointScriptFixture(t)
	keys, err := inputs.CheckpointKeys()
	if err != nil {
		t.Fatal(err)
	}
	c := NewCheckpointContext(keys)
	if err := c.SetLifecycleBindings(w, checkpoint.NewBindingAuthority()); err != nil {
		t.Fatal(err)
	}
	if _, err := w.CollectCheckpointReferences(c); err != nil {
		t.Fatal(err)
	}
	u.RenderPieceFlags = nil
	lower, err := c.ScriptBindings(u)
	if err != nil {
		t.Fatal(err)
	}
	u.RenderPieceFlags = make([]uint8, 0)
	if again, err := c.ScriptBindings(u); err != nil || again != lower {
		t.Fatal("empty render storage acquired an invented backing identity", err)
	}
	checkpointScriptBytes(t, w, c)
	u.Script = nil
	if _, err := c.ScriptBindings(u); err == nil {
		t.Fatal("missing registered VM accepted")
	}
	checkpointScriptRefused(t, w, c)
}

func TestCheckpointScriptOrdinaryReplacementCannotReuseAdmission(t *testing.T) {
	f := newCheckpointUnitScriptFixture(t)
	f.install(true, nil, nil, nil)
	u := checkpointCreate(t, f.w, "armdef", 0)
	u.SetScript(nil)
	if _, err := f.context(t).ScriptBindings(u); err == nil {
		t.Fatal("cleared script reused admission")
	}
	u.SetScript(cob.NewVM(u.Def.Script))
	bindTransportQueries(u)
	lower := &cob.CheckpointContext{}
	if err := lower.SetOwnerBindings(u.Script, checkpoint.Allocation{Handle: uint32(u.Handle), Serial: u.AllocationSerial}, f.authority); err != nil {
		t.Fatal(err)
	}
	if err := u.Script.ValidateCheckpointBindings(lower); err == nil {
		t.Fatal("ordinary replacement borrowed transport-query proof")
	}
	c := f.context(t)
	if _, err := c.ScriptBindings(u); err == nil {
		t.Fatal("ordinary replacement reused the prior admitted VM identity")
	}
	checkpointScriptRefused(t, f.w, c)
}
