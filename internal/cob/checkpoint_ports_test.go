package cob

import (
	"bytes"
	"maps"
	"slices"
	"strings"
	"testing"

	"github.com/nanolathe-gg/nanolathe/internal/sim/checkpoint"
)

func checkpointPortContext(t *testing.T, v *VM, owner checkpoint.Allocation, authority *checkpoint.BindingAuthority) *CheckpointContext {
	t.Helper()
	c := &CheckpointContext{Program: checkpoint.Definition{Family: 1, Ordinal: 2, Key: "p"}}
	if err := c.SetOwnerBindings(v, owner, authority); err != nil {
		t.Fatal(err)
	}
	return c
}

func checkpointPortBytes(t *testing.T, v *VM, c *CheckpointContext) []byte {
	t.Helper()
	var out bytes.Buffer
	if err := v.WriteCheckpoint(checkpoint.NewEncoder(&out), c); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}

func checkpointPortRefusal(t *testing.T, v *VM, c *CheckpointContext, path string) {
	t.Helper()
	var out bytes.Buffer
	err := v.WriteCheckpoint(checkpoint.NewEncoder(&out), c)
	if err == nil || !strings.Contains(err.Error(), path) || out.Len() != 0 {
		t.Fatalf("capture: error %v, bytes %d; want refusal at %s before bytes", err, out.Len(), path)
	}
}

// An independently authored empty-program VM vector, with only the binding
// record supplied by each case. No encoder or writer constructs the oracle.
func checkpointPortVector(bindings []byte) []byte {
	// Pieces count, eight 184-byte physical threads, active count, anims count.
	out := make([]byte, 4+8*184+4+4)
	out = append(out, bindings...)
	// dirty, return identities/valid/value arrays, next identity, onReturn tags.
	out = append(out, make([]byte, 1+8*8+8+8*4+8+8)...)
	// Local piece flag store (empty), present program, admitted definition.
	out = append(out, 1, 0, 0, 0, 0, 1, 1, 2, 0, 0, 0, 1, 0, 0, 0, 'p')
	// Empty statics, never-allocated thread identities, 30-tick denominator.
	out = append(out, make([]byte, 4+8*8)...)
	return append(out, 30, 0, 0, 0)
}

func TestCheckpointPortsLiteralFramingAndOrder(t *testing.T) {
	owner := checkpoint.Allocation{Handle: 0x12345678, Serial: 0x123456789abcdef0}
	authority := checkpoint.NewBindingAuthority()
	read := func([4]int32) int32 { panic("capture invoked read") }
	write := func(int32) { panic("capture invoked write") }
	legacy := func([]int32) int32 { panic("capture invoked legacy") }
	ports := []Port{0x100000002, -0x100000001, 2}
	rows := []PortBinding{{Read: read, Write: write}, {Read: read}, {Write: write}}
	// Full-width signed port keys, sorted independently in both maps. The
	// explicit and legacy slots encode distinct overlapping key sets (.47).
	wantBindings := []byte{
		0, 0, 0, // cargoContains, carrierIdentity, explosionSink
		1, 3, 0, 0, 0,
		0xff, 0xff, 0xff, 0xff, 0xfe, 0xff, 0xff, 0xff, 1, 0,
		2, 0, 0, 0, 0, 0, 0, 0, 0, 1,
		2, 0, 0, 0, 1, 0, 0, 0, 1, 1,
		1, 2, 0, 0, 0,
		0xff, 0xff, 0xff, 0xff, 0xfe, 0xff, 0xff, 0xff,
		2, 0, 0, 0, 1, 0, 0, 0,
		0, 0, 0, 0, 0, 0, 0, // remaining seven binding slots
	}
	for _, order := range [][]int{{0, 1, 2}, {2, 1, 0}} {
		v := NewVM(&Program{})
		c := checkpointPortContext(t, v, owner, authority)
		if got := checkpointPortBytes(t, v, c); !bytes.Equal(got, checkpointPortVector(make([]byte, 12))) {
			t.Fatal("registered nil fixture differs from independent vector")
		}
		for _, i := range order {
			v.BindPortBindingWithCheckpointBinding(ports[i], rows[i], owner, authority)
			if i != 2 {
				v.BindPortWithCheckpointBinding(ports[i], legacy, owner, authority)
			}
		}
		v.BindPort(-7, nil)
		v.BindPortBinding(9, PortBinding{})
		got := checkpointPortBytes(t, v, c)
		if want := checkpointPortVector(wantBindings); !bytes.Equal(got, want) {
			t.Fatalf("binding vector mismatch: got %x, want %x", got[1484:1484+len(wantBindings)], wantBindings)
		}
		before := *v
		bindingProofs, legacyProofs := maps.Clone(v.checkpointPortBindings), maps.Clone(v.checkpointPortFuncs)
		if !bytes.Equal(got, checkpointPortBytes(t, v, c)) || before.Threads != v.Threads ||
			!maps.Equal(bindingProofs, v.checkpointPortBindings) || !maps.Equal(legacyProofs, v.checkpointPortFuncs) ||
			v.cacheRevision != before.cacheRevision || v.dirty != before.dirty || len(v.diagnostics) != 0 {
			t.Fatal("capture changed VM or copied proof state")
		}
	}
}

func TestCheckpointPortsIndependentReplacement(t *testing.T) {
	v := NewVM(&Program{})
	owner := checkpoint.Allocation{Handle: 7, Serial: 11}
	authority := checkpoint.NewBindingAuthority()
	c := checkpointPortContext(t, v, owner, authority)
	fn := func([]int32) int32 { return 3 }
	binding := PortBinding{Read: func([4]int32) int32 { return 5 }}
	v.BindPortWithCheckpointBinding(1, fn, owner, authority)
	v.BindPortWithCheckpointBinding(2, fn, owner, authority)
	v.BindPortBindingWithCheckpointBinding(1, binding, owner, authority)
	baseline := checkpointPortBytes(t, v, c)
	other := v.checkpointPortFuncs[2]

	v.BindPortBinding(1, binding) // Same function still invalidates this row.
	checkpointPortRefusal(t, v, c, "VM.bindings.portBindings[1]")
	if _, ok := v.checkpointPortBindings[1]; ok || v.checkpointPortFuncs[2] != other || !v.checkpointPortFuncs[1].matches(v, c) {
		t.Fatal("explicit replacement invalidated a different proof slot")
	}
	v.BindPortBindingWithCheckpointBinding(1, binding, owner, authority)
	if !bytes.Equal(baseline, checkpointPortBytes(t, v, c)) {
		t.Fatal("canonical explicit reinstall changed bytes")
	}
	v.BindPort(1, fn)
	checkpointPortRefusal(t, v, c, "VM.bindings.portFuncs[1]")
	if _, ok := v.checkpointPortFuncs[1]; ok || !v.checkpointPortBindings[1].matches(v, c) || v.checkpointPortFuncs[2] != other {
		t.Fatal("legacy replacement invalidated a different proof slot")
	}
	// Reinstalling the other map cannot repair the legacy slot.
	v.BindPortBindingWithCheckpointBinding(1, binding, owner, authority)
	checkpointPortRefusal(t, v, c, "VM.bindings.portFuncs[1]")
	v.BindPortWithCheckpointBinding(1, fn, owner, authority)
	if !bytes.Equal(baseline, checkpointPortBytes(t, v, c)) {
		t.Fatal("canonical legacy reinstall changed bytes")
	}
	// Nil rows lose their proof and need none; each nil setter remains local.
	v.BindPortWithCheckpointBinding(1, nil, owner, authority)
	v.BindPortBindingWithCheckpointBinding(1, PortBinding{}, owner, authority)
	v.BindPort(2, nil)
	if len(v.checkpointPortBindings) != 0 || len(v.checkpointPortFuncs) != 0 {
		t.Fatal("nil rows retained proof")
	}
	if !bytes.Equal(checkpointPortBytes(t, v, c), checkpointPortVector(make([]byte, 12))) {
		t.Fatal("nil rows changed absent fixture bytes")
	}
}

func TestCheckpointPortsContextAndExactOwnership(t *testing.T) {
	v := NewVM(&Program{})
	owner := checkpoint.Allocation{Handle: 3, Serial: 4}
	authority := checkpoint.NewBindingAuthority()
	otherAuthority := checkpoint.NewBindingAuthority()
	c := checkpointPortContext(t, v, owner, authority)
	before := *c
	for _, tc := range []struct {
		name  string
		vm    *VM
		owner checkpoint.Allocation
		a     *checkpoint.BindingAuthority
	}{
		{"nil VM", nil, owner, authority},
		{"zero handle", v, checkpoint.Allocation{Serial: 4}, authority},
		{"zero serial", v, checkpoint.Allocation{Handle: 3}, authority},
		{"nil authority", v, owner, nil},
		{"other VM", NewVM(&Program{}), owner, authority},
		{"other handle", v, checkpoint.Allocation{Handle: 9, Serial: 4}, authority},
		{"other serial", v, checkpoint.Allocation{Handle: 3, Serial: 9}, authority},
		{"other authority", v, owner, otherAuthority},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := c.SetOwnerBindings(tc.vm, tc.owner, tc.a); err == nil || *c != before {
				t.Fatalf("registration changed on refusal: %v", err)
			}
		})
	}
	if err := (*CheckpointContext)(nil).SetOwnerBindings(v, owner, authority); err == nil {
		t.Fatal("nil context accepted")
	}
	if err := c.SetOwnerBindings(v, owner, authority); err != nil || *c != before {
		t.Fatalf("exact repeat: %v", err)
	}
	checkpointPortRefusal(t, NewVM(&Program{}), c, "VM.bindings.owner")
	fn := func([]int32) int32 { panic("capture called legacy callback") }
	v.BindPort(1, fn)
	// A declared owner never attests an ordinary installation.
	checkpointPortRefusal(t, v, c, "VM.bindings.portFuncs[1]")
	v.BindPortWithCheckpointBinding(1, fn, owner, authority)
	v.BindPortBindingWithCheckpointBinding(2, PortBinding{Write: func(int32) { panic("capture called write") }}, owner, authority)
	checkpointPortBytes(t, v, c)
	missing := &CheckpointContext{Program: c.Program}
	checkpointPortRefusal(t, v, missing, "VM.bindings.portBindings[2]")
	foreign := checkpointPortContext(t, v, owner, otherAuthority)
	checkpointPortRefusal(t, v, foreign, "VM.bindings.portBindings[2]")
	foreign = checkpointPortContext(t, v, checkpoint.Allocation{Handle: 3, Serial: 5}, authority)
	checkpointPortRefusal(t, v, foreign, "VM.bindings.portBindings[2]")
	copied := *v
	copyContext := checkpointPortContext(t, &copied, owner, authority)
	checkpointPortRefusal(t, &copied, copyContext, "VM.bindings.portBindings[2]")
	checkpointPortBytes(t, v, c)
	// Attested ports cannot bless unrelated live callbacks.
	v.scriptTouched = func() { panic("capture called scriptTouched") }
	checkpointPortRefusal(t, v, c, "VM.bindings.scriptTouched")
}

func TestCheckpointPortsInvalidProofDoesNotGateInstallation(t *testing.T) {
	owner := checkpoint.Allocation{Handle: 3, Serial: 4}
	authority := checkpoint.NewBindingAuthority()
	for _, tc := range []struct {
		name  string
		owner checkpoint.Allocation
		a     *checkpoint.BindingAuthority
	}{
		{"nil authority", owner, nil},
		{"zero handle", checkpoint.Allocation{Serial: 4}, authority},
		{"zero serial", checkpoint.Allocation{Handle: 3}, authority},
		{"foreign authority", owner, checkpoint.NewBindingAuthority()},
		{"foreign handle", checkpoint.Allocation{Handle: 5, Serial: 4}, authority},
		{"foreign serial", checkpoint.Allocation{Handle: 3, Serial: 5}, authority},
	} {
		for _, explicit := range []bool{false, true} {
			t.Run(tc.name+map[bool]string{false: "/legacy", true: "/explicit"}[explicit], func(t *testing.T) {
				v := NewVM(&Program{})
				c := checkpointPortContext(t, v, owner, authority)
				calls := 0
				path := "VM.bindings.portFuncs[1]"
				if explicit {
					binding := PortBinding{Read: func([4]int32) int32 { calls++; return 17 }}
					v.BindPortBindingWithCheckpointBinding(1, binding, owner, authority)
					v.BindPortBindingWithCheckpointBinding(1, binding, tc.owner, tc.a)
					path = "VM.bindings.portBindings[1]"
				} else {
					fn := func([]int32) int32 { calls++; return 17 }
					v.BindPortWithCheckpointBinding(1, fn, owner, authority)
					v.BindPortWithCheckpointBinding(1, fn, tc.owner, tc.a)
				}
				checkpointPortRefusal(t, v, c, path)
				if calls != 0 || v.readPort(1, [4]int32{}) != 17 || calls != 1 {
					t.Fatalf("installation/capture changed callback execution: calls %d", calls)
				}
			})
		}
	}
}

func TestCheckpointPortsCopiedInputAndFallbackArms(t *testing.T) {
	owner := checkpoint.Allocation{Handle: 3, Serial: 4}
	authority := checkpoint.NewBindingAuthority()
	v := NewVM(&Program{})
	c := checkpointPortContext(t, v, owner, authority)
	var calls []string
	read := func(args [4]int32) int32 {
		if args != ([4]int32{11, 22, 33, 44}) {
			t.Fatalf("explicit read operands %v", args)
		}
		calls = append(calls, "explicit read")
		return 23
	}
	write := func(value int32) {
		if value != -19 {
			t.Fatalf("explicit write operand %d", value)
		}
		calls = append(calls, "explicit write")
	}
	legacy := func(args []int32) int32 {
		switch {
		case slices.Equal(args, []int32{1, -19}):
			calls = append(calls, "legacy write")
		case slices.Equal(args, []int32{2, 11, 22, 33, 44}):
			calls = append(calls, "legacy read")
		default:
			t.Fatalf("legacy fallback operands %v", args)
		}
		return 29
	}
	input := map[Port]PortBinding{1: {Read: read}, 2: {Write: write}}
	for _, port := range []Port{1, 2} {
		v.BindPortBindingWithCheckpointBinding(port, input[port], owner, authority)
		v.BindPortWithCheckpointBinding(port, legacy, owner, authority)
	}
	baseline := checkpointPortBytes(t, v, c)
	local := input[1]
	local.Read, local.Write = nil, write
	input[1] = local
	delete(input, 2)
	if !bytes.Equal(baseline, checkpointPortBytes(t, v, c)) || len(calls) != 0 {
		t.Fatal("input row mutation changed the copied binding, or installation/capture invoked a callback")
	}
	// Host fallback arms remain independent (§16.3.47); read and write
	// operand slots retain their authored order [04 R-COB-03 §1].
	if v.readPort(1, [4]int32{11, 22, 33, 44}) != 23 || v.readPort(2, [4]int32{11, 22, 33, 44}) != 29 {
		t.Fatal("read fallback precedence changed")
	}
	v.writePort(1, -19)
	v.writePort(2, -19)
	if !slices.Equal(calls, []string{"explicit read", "legacy read", "legacy write", "explicit write"}) {
		t.Fatalf("callback order/count changed: %v", calls)
	}
}

func TestCheckpointPortsPreserveNilReceiverInstallation(t *testing.T) {
	var v *VM
	// The explicit ordinary API is a nil-receiver no-op; its sibling is too.
	v.BindPortBindingWithCheckpointBinding(1, PortBinding{Read: func([4]int32) int32 { panic("called") }},
		checkpoint.Allocation{Handle: 1, Serial: 1}, checkpoint.NewBindingAuthority())
	// The legacy ordinary API panics on a nil receiver; do not add a guard.
	defer func() {
		if recover() == nil {
			t.Fatal("legacy nil receiver no longer panics")
		}
	}()
	v.BindPortWithCheckpointBinding(1, nil, checkpoint.Allocation{}, nil)
}
