package cob

import (
	"bytes"
	"errors"
	"testing"

	"github.com/nanolathe-gg/nanolathe/internal/sim/checkpoint"
	"github.com/nanolathe-gg/nanolathe/internal/sim/rng"
)

type checkpointRuntimeSink struct{ marker byte }

func (*checkpointRuntimeSink) EmitCOBEvent(PresentationEvent) { panic("capture invoked sink") }
func (*checkpointRuntimeSink) AdmitWholePiece(WholePieceExplosion) bool {
	panic("capture invoked whole piece")
}
func (*checkpointRuntimeSink) AdmitShatter(ShatterExplosion) bool { panic("capture invoked shatter") }
func (*checkpointRuntimeSink) AdmitBitmap(BitmapExplosion) bool   { panic("capture invoked bitmap") }

type checkpointHostileSink []int

func (checkpointHostileSink) EmitCOBEvent(PresentationEvent) { panic("capture invoked hostile sink") }
func (checkpointHostileSink) EmitSFX(int, int32, SFXKind)    { panic("capture invoked hostile SFX") }
func (checkpointHostileSink) AdmitWholePiece(WholePieceExplosion) bool {
	panic("capture invoked hostile whole piece")
}
func (checkpointHostileSink) AdmitShatter(ShatterExplosion) bool {
	panic("capture invoked hostile shatter")
}
func (checkpointHostileSink) AdmitBitmap(BitmapExplosion) bool {
	panic("capture invoked hostile bitmap")
}

func checkpointRuntimeSetup(t *testing.T) (*VM, *CheckpointContext, *checkpoint.BindingAuthority, checkpoint.Allocation) {
	t.Helper()
	v := NewVM(&Program{})
	a := checkpoint.NewBindingAuthority()
	owner := checkpoint.Allocation{Handle: 0x12345678, Serial: 0x123456789abcdef0}
	return v, checkpointPortContext(t, v, owner, a), a, owner
}

func checkpointSealRuntime(t *testing.T, r *CheckpointVMInstallation, owner checkpoint.Allocation) {
	t.Helper()
	if r == nil {
		t.Fatal("missing receipt")
	}
	if err := r.Seal(owner); err != nil {
		t.Fatal(err)
	}
}

// Independently authored full empty-program VM with external storage. This
// explicitly differs from the local-source vector by its source byte and lack
// of a byte-string length; proofs, allocation and external stores add no bytes.
func checkpointExternalVector(bindings []byte) []byte {
	out := make([]byte, 4+8*184+4+4)
	out = append(out, bindings...)
	out = append(out, make([]byte, 1+8*8+8+8*4+8+8)...)
	out = append(out, 0, 1, 1, 2, 0, 0, 0, 1, 0, 0, 0, 'p')
	out = append(out, make([]byte, 4+8*8)...)
	return append(out, 30, 0, 0, 0)
}

func TestCheckpointRuntimeLexicalVectorAndPurity(t *testing.T) {
	v, c, a, owner := checkpointRuntimeSetup(t)
	flags, pieceMap := []uint8{6, 7}, []int{1, 0}
	sim := rng.NewSimulation(7)
	beforeRNG := sim
	sink := &checkpointRuntimeSink{1}
	b := &Binding{VM: v, PieceMap: pieceMap, SimulationRNG: &sim, PresentationSink: sink}
	v.SetSimulationRNG(&sim)
	v.SetSFXSink(PresentationSinkAdapter{Sink: sink})
	v.SetExplosionSink(sink)
	if v.ExplosionSink() != sink || (*VM)(nil).ExplosionSink() != nil {
		t.Fatal("sink getter altered identity")
	}
	if err := c.SetRuntimeSources(b, &sim, flags, pieceMap); err != nil {
		t.Fatal(err)
	}
	if err := SetCheckpointPresentationSink(c, sink); err != nil {
		t.Fatal(err)
	}
	if err := SetCheckpointExplosionSink(c, sink); err != nil {
		t.Fatal(err)
	}
	queries := v.BindTransportQueriesWithPendingCheckpointBinding(func(int32) bool { panic("cargo") }, func() int32 { panic("carrier") }, a)
	mutations := v.BindTransportMutationsWithPendingCheckpointBinding(func(int32, int32, int32) { panic("attach") }, func(int32) { panic("drop") }, a)
	visible := b.SetSFXSinkWithPendingCheckpointBinding(PresentationSinkAdapter{Sink: sink}, func(int, int32) bool { panic("visibility") }, a)
	render := v.BindRenderFlagHandlersWithPendingCheckpointBinding(func() []uint8 { panic("render getter") }, func(int, uint8, bool) bool { panic("render setter") }, flags, pieceMap, a)
	for _, r := range []*CheckpointVMInstallation{queries[0], queries[1], mutations[0], mutations[1], visible, render,
		v.BindScriptTouchedWithPendingCheckpointBinding(func() { panic("marker") }, a)} {
		checkpointSealRuntime(t, r, owner)
	}
	// Lexical order, absent port maps, mapped render kind 2, all other slots 1.
	want := checkpointExternalVector([]byte{1, 1, 1, 0, 0, 1, 2, 1, 1, 1, 1, 1, 1})
	for range 2 {
		if err := v.ValidateCheckpointBindings(c); err != nil {
			t.Fatal(err)
		}
		if got := checkpointPortBytes(t, v, c); !bytes.Equal(got, want) {
			t.Fatalf("full vector differs: got %x want %x", got, want)
		}
	}
	v.pieceFlags = []uint8{99, 8, 7} // Unread external fallback residue is excluded.
	if got := checkpointPortBytes(t, v, c); !bytes.Equal(got, want) {
		t.Fatal("external fallback leaked")
	}
	if sim != beforeRNG || !bytes.Equal(flags, []byte{6, 7}) || pieceMap[0] != 1 || pieceMap[1] != 0 {
		t.Fatal("capture changed source state")
	}
}

func TestCheckpointRuntimeReceiptOwnershipAndReplacement(t *testing.T) {
	v, c, a, owner := checkpointRuntimeSetup(t)
	fn := func() { panic("marker") }
	r := v.BindScriptTouchedWithPendingCheckpointBinding(fn, a)
	checkpointPortRefusal(t, v, c, "scriptTouched")
	if err := r.Seal(checkpoint.Allocation{Handle: owner.Handle}); err == nil {
		t.Fatal("zero serial sealed")
	}
	copied := *r
	if err := copied.Seal(owner); err == nil {
		t.Fatal("copied receipt sealed")
	}
	checkpointSealRuntime(t, r, owner)
	checkpointSealRuntime(t, r, owner)
	before := *r
	if err := r.Seal(checkpoint.Allocation{Handle: 1, Serial: 2}); err == nil || r.allocation != before.allocation {
		t.Fatal("conflicting seal mutated receipt")
	}
	checkpointPortBytes(t, v, c)
	foreign := checkpointPortContext(t, v, owner, checkpoint.NewBindingAuthority())
	checkpointPortRefusal(t, v, foreign, "scriptTouched")
	foreign = checkpointPortContext(t, v, checkpoint.Allocation{Handle: owner.Handle, Serial: owner.Serial + 1}, a)
	checkpointPortRefusal(t, v, foreign, "scriptTouched")
	vmCopy := *v
	checkpointPortRefusal(t, &vmCopy, checkpointPortContext(t, &vmCopy, owner, a), "scriptTouched")
	for _, replacement := range []string{"ordinary", "canonical", "nil", "no authority"} {
		t.Run(replacement, func(t *testing.T) {
			r := v.BindScriptTouchedWithPendingCheckpointBinding(fn, a)
			var next *CheckpointVMInstallation
			switch replacement {
			case "ordinary":
				v.BindScriptTouched(fn)
			case "canonical":
				next = v.BindScriptTouchedWithPendingCheckpointBinding(fn, a)
			case "nil":
				next = v.BindScriptTouchedWithPendingCheckpointBinding(nil, a)
			case "no authority":
				next = v.BindScriptTouchedWithPendingCheckpointBinding(fn, nil)
			}
			if err := r.Seal(owner); err == nil || r.allocation != (checkpoint.Allocation{}) {
				t.Fatal("stale receipt sealed")
			}
			if next != nil {
				checkpointSealRuntime(t, next, owner)
			}
			if replacement == "ordinary" || replacement == "no authority" {
				checkpointPortRefusal(t, v, c, "scriptTouched")
			} else {
				checkpointPortBytes(t, v, c)
			}
		})
	}
	if err := (*CheckpointVMInstallation)(nil).Seal(owner); err == nil {
		t.Fatal("nil receipt sealed")
	}
}

func TestCheckpointRuntimePairedSlotsAndNestedCompletion(t *testing.T) {
	v, c, a, owner := checkpointRuntimeSetup(t)
	queries := v.BindTransportQueriesWithPendingCheckpointBinding(func(int32) bool { panic("cargo") }, func() int32 { panic("carrier") }, a)
	mutations := v.BindTransportMutationsWithPendingCheckpointBinding(func(int32, int32, int32) { panic("attach") }, func(int32) { panic("drop") }, a)
	checkpointSealRuntime(t, queries[0], owner)
	checkpointPortRefusal(t, v, c, "carrierIdentity")
	checkpointSealRuntime(t, queries[1], owner)
	checkpointSealRuntime(t, mutations[1], owner)
	checkpointPortRefusal(t, v, c, "transportAttach")
	checkpointSealRuntime(t, mutations[0], owner)
	baseline := checkpointPortBytes(t, v, c)
	v.BindScriptTouched(func() {})
	checkpointPortRefusal(t, v, c, "scriptTouched")
	v.BindScriptTouched(nil)
	if !bytes.Equal(baseline, checkpointPortBytes(t, v, c)) {
		t.Fatal("unrelated slot invalidated transport")
	}
	v.BindTransportQueries(v.cargoContains, v.carrierIdentity)
	if queries[0].Seal(owner) == nil || queries[1].Seal(owner) == nil {
		t.Fatal("ordinary query pair kept proof")
	}
	checkpointPortRefusal(t, v, c, "cargoContains")
	queries = v.BindTransportQueriesWithPendingCheckpointBinding(nil, func() int32 { return 0 }, a)
	if queries[0] != nil {
		t.Fatal("absent query got receipt")
	}
	checkpointSealRuntime(t, queries[1], owner)
	checkpointPortBytes(t, v, c)
	v.BindTransportMutations(nil, nil)
	if mutations[0].Seal(owner) == nil || mutations[1].Seal(owner) == nil {
		t.Fatal("ordinary mutation pair kept proof")
	}
	// Nested completion seals its own actual allocation before the outer one.
	outer, outerC, _, _ := checkpointRuntimeSetup(t)
	inner, innerC, _, _ := checkpointRuntimeSetup(t)
	outerC.bindingAuthority, innerC.bindingAuthority = a, a
	outerR := outer.BindScriptTouchedWithPendingCheckpointBinding(func() {}, a)
	innerR := inner.BindScriptTouchedWithPendingCheckpointBinding(func() {}, a)
	checkpointSealRuntime(t, innerR, owner)
	checkpointPortBytes(t, inner, innerC)
	checkpointPortRefusal(t, outer, outerC, "scriptTouched")
	outerC.ownerAllocation.Serial++
	checkpointSealRuntime(t, outerR, outerC.ownerAllocation)
	checkpointPortBytes(t, outer, outerC)
}

func TestCheckpointRuntimeRenderAliasResetAndSource(t *testing.T) {
	for _, flags := range [][]uint8{nil, {}} {
		v, c, a, owner := checkpointRuntimeSetup(t)
		if err := c.SetRuntimeSources(nil, nil, flags, nil); err != nil {
			t.Fatal(err)
		}
		r := v.BindRenderFlagsWithPendingCheckpointBinding(flags, a)
		checkpointSealRuntime(t, r, owner)
		want := checkpointExternalVector([]byte{0, 0, 0, 0, 0, 1, 1, 0, 0, 0, 0, 0, 0})
		if !bytes.Equal(checkpointPortBytes(t, v, c), want) {
			t.Fatal("empty external vector differs")
		}
		v.UnbindRenderFlags()
		if r.Seal(owner) == nil {
			t.Fatal("unbind retained render proof")
		}
		if !bytes.Equal(checkpointPortBytes(t, v, c), checkpointPortVector(make([]byte, 12))) {
			t.Fatal("unbound local vector differs")
		}
	}
	for _, mutate := range []string{"equal flags", "equal map", "direct mixed", "getter only", "setter only", "ordinary handlers", "ordinary direct", "nil program", "program"} {
		t.Run(mutate, func(t *testing.T) {
			v, c, a, owner := checkpointRuntimeSetup(t)
			flags, pm := []byte{6, 7}, []int{1, 0}
			b := &Binding{VM: v, PieceMap: pm}
			if err := c.SetRuntimeSources(b, nil, flags, pm); err != nil {
				t.Fatal(err)
			}
			get := func() []uint8 { panic("getter") }
			set := func(int, uint8, bool) bool { panic("setter") }
			r := v.BindRenderFlagHandlersWithPendingCheckpointBinding(get, set, flags, pm, a)
			checkpointSealRuntime(t, r, owner)
			checkpointPortBytes(t, v, c)
			switch mutate {
			case "equal flags":
				other := checkpointPortContext(t, v, owner, a)
				if err := other.SetRuntimeSources(b, nil, append([]byte(nil), flags...), pm); err != nil {
					t.Fatal(err)
				}
				c = other
			case "equal map":
				b.PieceMap = append([]int(nil), pm...)
			case "direct mixed":
				v.BindRenderFlags(flags)
				r = v.BindRenderFlagHandlersWithPendingCheckpointBinding(get, set, flags, pm, a)
				checkpointSealRuntime(t, r, owner)
			case "getter only":
				v.BindRenderFlagHandlersWithPendingCheckpointBinding(get, nil, flags, pm, a)
			case "setter only":
				v.BindRenderFlagHandlersWithPendingCheckpointBinding(nil, set, flags, pm, a)
			case "ordinary handlers":
				v.BindRenderFlagHandlers(get, set)
			case "ordinary direct":
				v.BindRenderFlags(flags)
			case "nil program":
				v.SetProgram(nil)
			case "program":
				v.SetProgram(&Program{})
			}
			if mutate == "nil program" || mutate == "program" {
				if r.Seal(owner) == nil || v.checkpointBindingsProof[checkpointVMRenderFlags] != nil {
					t.Fatal("program reset retained proof")
				}
				if err := v.ValidateCheckpointBindings(c); err != nil {
					t.Fatal(err)
				}
			} else {
				checkpointPortRefusal(t, v, c, "renderFlags")
			}
		})
	}
	v, c, a, owner := checkpointRuntimeSetup(t)
	flags := []byte{6}
	if err := c.SetRuntimeSources(nil, nil, flags, nil); err != nil {
		t.Fatal(err)
	}
	checkpointSealRuntime(t, v.BindRenderFlagsWithPendingCheckpointBinding(append([]byte(nil), flags...), a), owner)
	checkpointPortRefusal(t, v, c, "renderFlags")
}

func TestCheckpointRuntimeContextAndSinks(t *testing.T) {
	v, c, _, _ := checkpointRuntimeSetup(t)
	flags, pm := []byte{6}, []int{0}
	sim := rng.NewSimulation(9)
	b := &Binding{VM: v, PieceMap: pm}
	if err := (*CheckpointContext)(nil).SetRuntimeSources(b, nil, flags, pm); err == nil {
		t.Fatal("nil context admitted")
	}
	if err := (&CheckpointContext{}).SetRuntimeSources(b, nil, flags, pm); err == nil {
		t.Fatal("missing owner admitted")
	}
	if err := c.SetRuntimeSources(b, &sim, flags, pm); err != nil {
		t.Fatal(err)
	}
	before := *c
	if err := c.SetRuntimeSources(b, &sim, flags, pm); err != nil || *c != before {
		t.Fatal("repeat registration changed context")
	}
	for _, sources := range []checkpointRuntimeSources{
		{binding: &Binding{VM: NewVM(&Program{})}, sim: &sim, flags: flags, pieceMap: pm},
		{binding: &Binding{VM: v}, sim: &sim, flags: flags, pieceMap: pm},
		{binding: b, sim: nil, flags: flags, pieceMap: pm},
		{binding: b, sim: &sim, flags: []byte{6}, pieceMap: pm},
		{binding: b, sim: &sim, flags: flags, pieceMap: []int{0}},
	} {
		if err := c.SetRuntimeSources(sources.binding, sources.sim, sources.flags, sources.pieceMap); err == nil || *c != before {
			t.Fatal("conflicting sources changed context")
		}
	}
	checkpointPortRefusal(t, v, c, "simRng")
	v.SetSimulationRNG(&sim)
	checkpointPortRefusal(t, v, c, "simRng")
	b.SimulationRNG = &sim
	checkpointPortBytes(t, v, c)
	sink, other := &checkpointRuntimeSink{1}, &checkpointRuntimeSink{2}
	if SetCheckpointPresentationSink(c, (*checkpointRuntimeSink)(nil)) == nil || SetCheckpointExplosionSink(c, (*checkpointRuntimeSink)(nil)) == nil {
		t.Fatal("typed nil registration accepted")
	}
	if SetCheckpointPresentationSink(nil, sink) == nil || SetCheckpointExplosionSink(nil, sink) == nil {
		t.Fatal("nil context accepted")
	}
	if err := SetCheckpointPresentationSink(c, sink); err != nil {
		t.Fatal(err)
	}
	if err := SetCheckpointExplosionSink(c, sink); err != nil {
		t.Fatal(err)
	}
	before = *c
	if SetCheckpointPresentationSink(c, other) == nil || SetCheckpointExplosionSink(c, other) == nil || *c != before {
		t.Fatal("conflicting sink changed context")
	}
	if SetCheckpointPresentationSink(c, sink) != nil || SetCheckpointExplosionSink(c, sink) != nil || *c != before {
		t.Fatal("sink repeat changed context")
	}
	v.SetSFXSink(PresentationSinkAdapter{Sink: sink})
	b.PresentationSink = sink
	v.SetExplosionSink(sink)
	checkpointPortBytes(t, v, c)
	for _, actual := range []SFXSink{nil, checkpointHostileSink{1}, &PresentationSinkAdapter{Sink: sink}, PresentationSinkAdapter{Sink: other}, PresentationSinkAdapter{Sink: (*checkpointRuntimeSink)(nil)}, PresentationSinkAdapter{Sink: checkpointHostileSink{1}}} {
		v.SetSFXSink(actual)
		checkpointPortRefusal(t, v, c, "sfxSink")
	}
	v.SetSFXSink(PresentationSinkAdapter{Sink: sink})
	for _, actual := range []ExplosionSink{nil, other, (*checkpointRuntimeSink)(nil), checkpointHostileSink{1}} {
		v.SetExplosionSink(actual)
		checkpointPortRefusal(t, v, c, "explosionSink")
	}
	v.SetExplosionSink(sink)
	b.SFXSink = checkpointHostileSink{1}
	checkpointPortRefusal(t, v, c, "Binding.SFXSink")
	b.SFXSink = PresentationSinkAdapter{Sink: sink}
	b.PresentationSink = checkpointHostileSink{1}
	checkpointPortRefusal(t, v, c, "Binding.PresentationSink")
	b.PresentationSink = sink
	b.VM = NewVM(&Program{})
	checkpointPortRefusal(t, v, c, "sources")
	if (*VM)(nil).ValidateCheckpointBindings(c) == nil || v.ValidateCheckpointBindings(nil) == nil {
		t.Fatal("nil validation accepted")
	}
}

func TestCheckpointRuntimeCopiedBindingAndPreCreateTiming(t *testing.T) {
	v, _, a, owner := checkpointRuntimeSetup(t)
	fn := func(int, int32) bool { panic("visibility") }
	for _, replace := range []string{"none", "VM ordinary", "VM canonical", "Binding ordinary", "Binding canonical"} {
		t.Run(replace, func(t *testing.T) {
			preCreates := 0
			var newer *CheckpointVMInstallation
			req := BindingRequest{UnitName: "Authored", ModelPieces: []string{}, Program: v.Program(), SFXVisible: fn,
				PreCreate: func(b *Binding) error {
					preCreates++
					if b.checkpointSFXVisible == nil || b.VM.checkpointBindingsProof[checkpointVMSFXVisible] != b.checkpointSFXVisible {
						t.Fatal("proof installed after PreCreate")
					}
					switch replace {
					case "VM ordinary":
						b.VM.SetSFXVisible(fn)
					case "VM canonical":
						newer = b.VM.SetSFXVisibleWithPendingCheckpointBinding(fn, a)
					case "Binding ordinary":
						b.SetSFXSink(nil, fn)
					case "Binding canonical":
						newer = b.SetSFXSinkWithPendingCheckpointBinding(nil, fn, a)
					}
					return nil
				}}
			b, receipt, err := BindStrictWithCheckpointBinding(nil, req, a)
			if err != nil || preCreates != 1 {
				t.Fatalf("constructor err=%v callbacks=%d", err, preCreates)
			}
			c := checkpointPortContext(t, b.VM, owner, a)
			if err := c.SetRuntimeSources(b, nil, nil, b.PieceMap); err != nil {
				t.Fatal(err)
			}
			if replace == "none" {
				checkpointPortRefusal(t, b.VM, c, "sfxVisible")
				checkpointSealRuntime(t, receipt, owner)
				checkpointPortBytes(t, b.VM, c)
				copyB := *b
				copyC := checkpointPortContext(t, b.VM, owner, a)
				if err := copyC.SetRuntimeSources(&copyB, nil, nil, copyB.PieceMap); err != nil {
					t.Fatal(err)
				}
				checkpointPortRefusal(t, b.VM, copyC, "sfxVisible")
			} else {
				if receipt.Seal(owner) == nil {
					t.Fatal("constructor blessed PreCreate replacement")
				}
				if newer != nil {
					checkpointSealRuntime(t, newer, owner)
				}
				if replace == "Binding canonical" {
					checkpointPortBytes(t, b.VM, c)
				} else {
					checkpointPortRefusal(t, b.VM, c, "sfxVisible")
				}
			}
		})
	}
	b, err := BindStrict(nil, BindingRequest{UnitName: "Authored", ModelPieces: []string{}, Program: v.Program(), SFXVisible: fn})
	if err != nil {
		t.Fatal(err)
	}
	c := checkpointPortContext(t, b.VM, owner, a)
	if err := c.SetRuntimeSources(b, nil, nil, nil); err != nil {
		t.Fatal(err)
	}
	checkpointPortRefusal(t, b.VM, c, "sfxVisible")
	failure := errors.New("authored precreate failure")
	b, r, err := BindStrictWithCheckpointBinding(nil, BindingRequest{UnitName: "Authored", ModelPieces: []string{}, Program: v.Program(), SFXVisible: fn, PreCreate: func(*Binding) error { return failure }}, a)
	if err == nil || b != nil || r != nil {
		t.Fatal("failed construction returned sealable result")
	}
}

func TestCheckpointRuntimeNilInstallAndOrdinaryWork(t *testing.T) {
	a := checkpoint.NewBindingAuthority()
	var v *VM
	if v.BindScriptTouchedWithPendingCheckpointBinding(func() {}, a) != nil || v.BindRenderFlagsWithPendingCheckpointBinding(nil, a) != nil ||
		v.BindRenderFlagHandlersWithPendingCheckpointBinding(func() []byte { return nil }, func(int, uint8, bool) bool { return true }, nil, nil, a) != nil {
		t.Fatal("nil VM got proof")
	}
	if v.BindTransportQueriesWithPendingCheckpointBinding(nil, nil, a) != [2]*CheckpointVMInstallation{} || v.BindTransportMutationsWithPendingCheckpointBinding(nil, nil, a) != [2]*CheckpointVMInstallation{} {
		t.Fatal("nil pair got proof")
	}
	if (*Binding)(nil).SetSFXSinkWithPendingCheckpointBinding(nil, nil, a) != nil {
		t.Fatal("nil binding got proof")
	}
	func() {
		defer func() {
			if recover() == nil {
				t.Fatal("nil visibility setter no longer panics")
			}
		}()
		v.SetSFXVisibleWithPendingCheckpointBinding(nil, a)
	}()
	v = NewVM(&Program{})
	calls := 0
	if v.BindScriptTouchedWithPendingCheckpointBinding(func() { calls++ }, nil) != nil {
		t.Fatal("nil authority got proof")
	}
	if calls != 0 {
		t.Fatal("install invoked callback")
	}
	v.raiseScriptTouched()
	if calls != 1 {
		t.Fatal("unsupported install changed ordinary work")
	}
	if v.SetSFXVisibleWithPendingCheckpointBinding(nil, a) != nil {
		t.Fatal("nil reader got proof")
	}
	b, r, err := BindStrictWithCheckpointBinding(nil, BindingRequest{UnitName: "Authored", ModelPieces: []string{}, Program: v.Program()}, a)
	if err != nil || r != nil || b.SFXVisibleReader() != nil {
		t.Fatal("absent constructor reader got proof")
	}
}

func TestCheckpointRuntimeAdmittedConstructorPreservesCreate(t *testing.T) {
	program, err := Load(makeCOB([]uint32{0x10021001, 9, 0x10065000}, []string{"Create"}, []uint32{0}, []string{"base"}))
	if err != nil {
		t.Fatal(err)
	}
	preCreates := 0
	req := BindingRequest{UnitName: "Authored", Program: program, ModelPieces: []string{"base"}, SFXVisible: func(int, int32) bool { panic("reader") },
		PreCreate: func(b *Binding) error {
			preCreates++
			if b.CreateInvoked || b.VM.DrainCalls != 0 {
				t.Fatal("Create preceded PreCreate")
			}
			return nil
		}}
	plain, err := BindStrict(nil, req)
	if err != nil {
		t.Fatal(err)
	}
	a := checkpoint.NewBindingAuthority()
	admitted, r, err := BindStrictWithCheckpointBinding(nil, req, a)
	if err != nil {
		t.Fatal(err)
	}
	if preCreates != 2 || !admitted.CreateInvoked || admitted.VM.DrainCalls != 1 || plain.VM.DrainCalls != 1 ||
		plain.VM.Threads != admitted.VM.Threads || plain.VM.lastReturnValue != admitted.VM.lastReturnValue || plain.VM.threadIdentity != admitted.VM.threadIdentity ||
		plain.VM.activeThreadCount != admitted.VM.activeThreadCount || plain.VM.nextIdentity != admitted.VM.nextIdentity {
		t.Fatal("admission changed Create work or callback count")
	}
	owner := checkpoint.Allocation{Handle: 3, Serial: 9}
	checkpointSealRuntime(t, r, owner)
	c := checkpointPortContext(t, admitted.VM, owner, a)
	if err := c.SetRuntimeSources(admitted, nil, nil, admitted.PieceMap); err != nil {
		t.Fatal(err)
	}
	checkpointPortBytes(t, admitted.VM, c)
}
