package cob

import (
	"bytes"
	"maps"
	"testing"

	"github.com/nanolathe-gg/nanolathe/internal/sim/checkpoint"
	"github.com/nanolathe-gg/nanolathe/internal/sim/rng"
)

func TestCheckpointPortReceiptSealVectorAndPurity(t *testing.T) {
	v := NewVM(&Program{})
	authority := checkpoint.NewBindingAuthority()
	owner := checkpoint.Allocation{Handle: 0xfedcba98, Serial: 0x123456789abcdef0}
	c := checkpointPortContext(t, v, owner, authority)
	sim, crt := rng.NewSimulation(7), rng.NewCRT(11)
	simBefore, crtBefore := sim, crt
	calls := 0
	legacy := func([]int32) int32 { calls++; return int32(sim.Uint32n(31)) }
	binding := PortBinding{
		Read:  func([4]int32) int32 { calls++; return int32(crt.Rand()) },
		Write: func(int32) { calls++; sim.Uint32n(31) },
	}
	legacyReceipt := v.BindPortWithPendingCheckpointBinding(5, legacy, authority)
	explicitReceipt := v.BindPortBindingWithPendingCheckpointBinding(5, binding, authority)
	if legacyReceipt == nil || explicitReceipt == nil {
		t.Fatal("present installations returned no receipt")
	}
	checkpointPortRefusal(t, v, c, "VM.bindings.portBindings[5]")
	// Sealing one map cannot admit the same-key entry in the other map.
	explicitBefore := v.checkpointPortBindings[5]
	if err := legacyReceipt.Seal(owner); err != nil || v.checkpointPortBindings[5] != explicitBefore {
		t.Fatalf("legacy seal affected explicit proof: %v", err)
	}
	checkpointPortRefusal(t, v, c, "VM.bindings.portBindings[5]")
	if err := explicitReceipt.Seal(owner); err != nil {
		t.Fatal(err)
	}
	// Independently authored §16.3.47 binding record: sorted I64 keys,
	// U32 counts, read/write booleans, and all other binding tags absent.
	bindings := []byte{
		0, 0, 0,
		1, 1, 0, 0, 0, 5, 0, 0, 0, 0, 0, 0, 0, 1, 1,
		1, 1, 0, 0, 0, 5, 0, 0, 0, 0, 0, 0, 0,
		0, 0, 0, 0, 0, 0, 0,
	}
	got := checkpointPortBytes(t, v, c)
	if !bytes.Equal(got, checkpointPortVector(bindings)) {
		t.Fatalf("sealed receipt vector = %x; want binding record %x", got[1484:1484+len(bindings)], bindings)
	}
	immediate := NewVM(&Program{})
	immediate.BindPortWithCheckpointBinding(5, legacy, owner, authority)
	immediate.BindPortBindingWithCheckpointBinding(5, binding, owner, authority)
	immediateContext := checkpointPortContext(t, immediate, owner, authority)
	if !bytes.Equal(got, checkpointPortBytes(t, immediate, immediateContext)) {
		t.Fatal("pending and immediate installations encoded differently")
	}
	before := *v
	legacyProofs, explicitProofs := maps.Clone(v.checkpointPortFuncs), maps.Clone(v.checkpointPortBindings)
	for i := 0; i < 2; i++ {
		if err := explicitReceipt.Seal(owner); err != nil {
			t.Fatal(err)
		}
		if err := legacyReceipt.Seal(owner); err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(got, checkpointPortBytes(t, v, c)) || !maps.Equal(legacyProofs, v.checkpointPortFuncs) ||
			!maps.Equal(explicitProofs, v.checkpointPortBindings) || before.Threads != v.Threads ||
			before.nextIdentity != v.nextIdentity || before.cacheRevision != v.cacheRevision || len(v.diagnostics) != 0 {
			t.Fatal("idempotent seal or capture changed state")
		}
	}
	if calls != 0 || sim != simBefore || crt != crtBefore {
		t.Fatal("installation, seal or capture invoked callbacks or drew RNG")
	}
	foreignOwner := checkpointPortContext(t, v, checkpoint.Allocation{Handle: owner.Handle, Serial: owner.Serial - 1}, authority)
	checkpointPortRefusal(t, v, foreignOwner, "VM.bindings.portBindings[5]")
	foreignAuthority := checkpointPortContext(t, v, owner, checkpoint.NewBindingAuthority())
	checkpointPortRefusal(t, v, foreignAuthority, "VM.bindings.portBindings[5]")
	copied := *v
	copyContext := checkpointPortContext(t, &copied, owner, authority)
	checkpointPortRefusal(t, &copied, copyContext, "VM.bindings.portBindings[5]")
}

func receiptFixture(t *testing.T, explicit bool) (*VM, *CheckpointPortInstallation, *CheckpointContext) {
	t.Helper()
	v := NewVM(&Program{})
	a := checkpoint.NewBindingAuthority()
	c := checkpointPortContext(t, v, checkpoint.Allocation{Handle: 7, Serial: 11}, a)
	var r *CheckpointPortInstallation
	if explicit {
		r = v.BindPortBindingWithPendingCheckpointBinding(3, PortBinding{Read: func([4]int32) int32 { panic("callback ran") }}, a)
	} else {
		r = v.BindPortWithPendingCheckpointBinding(3, func([]int32) int32 { panic("callback ran") }, a)
	}
	return v, r, c
}

func receiptSealRefusal(t *testing.T, v *VM, r *CheckpointPortInstallation, owner checkpoint.Allocation) {
	t.Helper()
	legacyProofs, explicitProofs := maps.Clone(v.checkpointPortFuncs), maps.Clone(v.checkpointPortBindings)
	var before CheckpointPortInstallation
	if r != nil {
		before = *r
	}
	if err := r.Seal(owner); err == nil {
		t.Fatal("invalid seal accepted")
	}
	if !maps.Equal(legacyProofs, v.checkpointPortFuncs) || !maps.Equal(explicitProofs, v.checkpointPortBindings) || (r != nil && before != *r) {
		t.Fatal("seal refusal changed receipt or installed proof")
	}
}

func TestCheckpointPortReceiptInvalidAndConflictingSeals(t *testing.T) {
	for _, explicit := range []bool{false, true} {
		v, r, c := receiptFixture(t, explicit)
		receiptSealRefusal(t, v, nil, c.ownerAllocation)
		receiptSealRefusal(t, v, &CheckpointPortInstallation{}, c.ownerAllocation)
		receiptSealRefusal(t, v, r, checkpoint.Allocation{})
		receiptSealRefusal(t, v, r, checkpoint.Allocation{Serial: 11})
		receiptSealRefusal(t, v, r, checkpoint.Allocation{Handle: 7})
		// Receipt identity cannot be manufactured by copying its private fields.
		copied := *r
		receiptSealRefusal(t, v, &copied, c.ownerAllocation)
		if err := r.Seal(c.ownerAllocation); err != nil {
			t.Fatal(err)
		}
		baseline := checkpointPortBytes(t, v, c)
		receiptSealRefusal(t, v, r, checkpoint.Allocation{Handle: 8, Serial: 11})
		receiptSealRefusal(t, v, r, checkpoint.Allocation{Handle: 7, Serial: 12})
		if !bytes.Equal(baseline, checkpointPortBytes(t, v, c)) {
			t.Fatal("conflicting seal changed admitted bytes")
		}
	}
}

func TestCheckpointPortReceiptReplacementsRetireReceipt(t *testing.T) {
	for _, explicit := range []bool{false, true} {
		for _, replacement := range []string{"ordinary", "immediate", "pending", "nil"} {
			for _, sealed := range []bool{false, true} {
				v, r, c := receiptFixture(t, explicit)
				if sealed {
					if err := r.Seal(c.ownerAllocation); err != nil {
						t.Fatal(err)
					}
				}
				var next *CheckpointPortInstallation
				if explicit {
					binding := v.portBindings[3] // Reinstall exactly the same callbacks.
					switch replacement {
					case "ordinary":
						v.BindPortBinding(3, binding)
					case "immediate":
						v.BindPortBindingWithCheckpointBinding(3, binding, c.ownerAllocation, c.bindingAuthority)
					case "pending":
						next = v.BindPortBindingWithPendingCheckpointBinding(3, binding, c.bindingAuthority)
					case "nil":
						v.BindPortBinding(3, PortBinding{})
					}
				} else {
					fn := v.portFuncs[3]
					switch replacement {
					case "ordinary":
						v.BindPort(3, fn)
					case "immediate":
						v.BindPortWithCheckpointBinding(3, fn, c.ownerAllocation, c.bindingAuthority)
					case "pending":
						next = v.BindPortWithPendingCheckpointBinding(3, fn, c.bindingAuthority)
					case "nil":
						v.BindPort(3, nil)
					}
				}
				receiptSealRefusal(t, v, r, c.ownerAllocation)
				if replacement == "ordinary" || replacement == "pending" {
					checkpointPortRefusal(t, v, c, "VM.bindings.port")
				}
				if next != nil {
					if err := next.Seal(c.ownerAllocation); err != nil {
						t.Fatal(err)
					}
				}
				if replacement != "ordinary" {
					checkpointPortBytes(t, v, c)
				}
			}
		}
	}
}

func TestCheckpointPortReceiptIndependentOwnersAndCompletionOrder(t *testing.T) {
	a := checkpoint.NewBindingAuthority()
	outer, inner, failed := NewVM(&Program{}), NewVM(&Program{}), NewVM(&Program{})
	fn := func([]int32) int32 { panic("callback ran") }
	binding := PortBinding{Write: func(int32) { panic("callback ran") }}
	outerLegacy := outer.BindPortWithPendingCheckpointBinding(1, fn, a)
	outerExplicit := outer.BindPortBindingWithPendingCheckpointBinding(1, binding, a)
	otherKey := outer.BindPortWithPendingCheckpointBinding(2, fn, a)
	innerReceipt := inner.BindPortWithPendingCheckpointBinding(1, fn, a)
	failed.BindPortWithPendingCheckpointBinding(1, fn, a)
	// Nested creation completes before its caller. The final serial, not
	// installation order, supplies ownership (DESIGN_MULTIPLAYER §16.3.51).
	innerOwner := checkpoint.Allocation{Handle: 9, Serial: 100}
	outerOwner := checkpoint.Allocation{Handle: 8, Serial: 101}
	if err := innerReceipt.Seal(innerOwner); err != nil {
		t.Fatal(err)
	}
	checkpointPortBytes(t, inner, checkpointPortContext(t, inner, innerOwner, a))
	outerContext := checkpointPortContext(t, outer, outerOwner, a)
	checkpointPortRefusal(t, outer, outerContext, "VM.bindings.portBindings[1]")
	// Ordinary replacement in one map retires only its own receipt. Same-key
	// explicit proof and another legacy key remain sealable independently.
	outer.BindPort(1, fn)
	receiptSealRefusal(t, outer, outerLegacy, outerOwner)
	if err := outerExplicit.Seal(outerOwner); err != nil {
		t.Fatal(err)
	}
	if err := otherKey.Seal(outerOwner); err != nil {
		t.Fatal(err)
	}
	checkpointPortRefusal(t, outer, outerContext, "VM.bindings.portFuncs[1]")
	outer.BindPort(1, nil)
	checkpointPortBytes(t, outer, outerContext)
	// Failed creation never seals; another owner's completion cannot admit it.
	checkpointPortRefusal(t, failed, checkpointPortContext(t, failed, outerOwner, a), "VM.bindings.portFuncs[1]")
}

func TestCheckpointPortReceiptAbsentAndUnattestedInstallations(t *testing.T) {
	v := NewVM(&Program{})
	a := checkpoint.NewBindingAuthority()
	owner := checkpoint.Allocation{Handle: 7, Serial: 11}
	c := checkpointPortContext(t, v, owner, a)
	calls := 0
	fn := func([]int32) int32 { calls++; return 19 }
	binding := PortBinding{Read: func([4]int32) int32 { calls++; return 23 }}
	v.BindPortWithCheckpointBinding(1, fn, owner, a)
	v.BindPortBindingWithCheckpointBinding(1, binding, owner, a)
	if r := v.BindPortWithPendingCheckpointBinding(1, fn, nil); r != nil {
		t.Fatal("missing authority created legacy receipt")
	}
	if r := v.BindPortBindingWithPendingCheckpointBinding(1, binding, nil); r != nil {
		t.Fatal("missing authority created explicit receipt")
	}
	if len(v.checkpointPortFuncs) != 0 || len(v.checkpointPortBindings) != 0 || calls != 0 {
		t.Fatal("missing authority preserved proof or installation invoked callback")
	}
	checkpointPortRefusal(t, v, c, "VM.bindings.portBindings[1]")
	if v.readPort(1, [4]int32{}) != 23 || calls != 1 {
		t.Fatal("missing authority gated explicit callback installation")
	}
	if r := v.BindPortBindingWithPendingCheckpointBinding(1, PortBinding{}, a); r != nil {
		t.Fatal("nil explicit row created receipt")
	}
	if v.readPort(1, [4]int32{}) != 19 || calls != 2 {
		t.Fatal("missing authority gated legacy fallback installation")
	}
	if r := v.BindPortWithPendingCheckpointBinding(1, nil, a); r != nil {
		t.Fatal("nil legacy row created receipt")
	}
	if len(v.checkpointPortFuncs) != 0 || len(v.checkpointPortBindings) != 0 || !bytes.Equal(checkpointPortBytes(t, v, c), checkpointPortVector(make([]byte, 12))) {
		t.Fatal("nil pending installs kept proof or changed absent bytes")
	}
	var absent *VM
	if r := absent.BindPortBindingWithPendingCheckpointBinding(1, binding, a); r != nil {
		t.Fatal("nil VM explicit no-op returned receipt")
	}
	defer func() {
		if recover() == nil {
			t.Fatal("nil VM legacy installation no longer panics")
		}
	}()
	absent.BindPortWithPendingCheckpointBinding(1, nil, a)
}
