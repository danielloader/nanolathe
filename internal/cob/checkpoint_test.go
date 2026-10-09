package cob

import (
	"bytes"
	"strings"
	"testing"

	"github.com/nanolathe-gg/nanolathe/internal/sim/checkpoint"
	"github.com/nanolathe-gg/nanolathe/internal/sim/rng"
)

func TestCheckpointAimPreservesUnprojectedReadyWord(t *testing.T) {
	for _, ready := range []bool{false, true} {
		s := AimSlot{IssueBit: true, Ready: ready, readyWord: 0x12345678}
		var out bytes.Buffer
		if err := s.WriteCheckpoint(checkpoint.NewEncoder(&out)); err != nil {
			t.Fatal(err)
		}
		want := []byte{1, 0, 0x78, 0x56, 0x34, 0x12}
		if ready {
			want[1] = 1
		}
		if !bytes.Equal(out.Bytes(), want) {
			t.Fatalf("ready %v: bytes %x, want %x", ready, out.Bytes(), want)
		}
		if s.readyWord != 0x12345678 || s.Ready != ready {
			t.Fatal("checkpoint changed aim state")
		}
	}
}

func checkpointVM(t *testing.T) *VM {
	t.Helper()
	prog, err := Load(makeCOB([]uint32{0x10021001, 67, 0x10013000, 0x10021001, 1, 0x10065000},
		[]string{"AimPrimary", "Create"}, []uint32{0, 0}, []string{"base"}))
	if err != nil {
		t.Fatal(err)
	}
	return NewVM(prog)
}

func checkpointVMBytes(t *testing.T, vm *VM) []byte {
	t.Helper()
	var out bytes.Buffer
	c := CheckpointContext{Program: checkpoint.Definition{Family: 1, Ordinal: 2, Key: "authored-test"}}
	if err := vm.WriteCheckpoint(checkpoint.NewEncoder(&out), &c); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}

func TestCheckpointVMMutatesEveryRetainedStateFamily(t *testing.T) {
	// Inactive thread words and animation lanes are stored state, even where
	// the current status/dirty bits prevent an immediate reader [04 §4.2].
	cases := []struct {
		name   string
		mutate func(*VM)
	}{
		{"piece rotation", func(v *VM) { v.Pieces[0].RotX++ }},
		{"piece translation", func(v *VM) { v.Pieces[0].Trans[2]++ }},
		{"inactive PC", func(v *VM) { v.Threads[7].PC++ }},
		{"inactive SP", func(v *VM) { v.Threads[7].SP++ }},
		{"signal", func(v *VM) { v.Threads[7].SignalMask++ }},
		{"sleep", func(v *VM) { v.Threads[7].Sleep++ }},
		{"stack above SP", func(v *VM) { v.Threads[7].Stack[31]++ }},
		{"status", func(v *VM) { v.Threads[7].Status++ }},
		{"wait axis", func(v *VM) { v.Threads[7].WaitAxis++ }},
		{"wait piece", func(v *VM) { v.Threads[7].WaitPiece++ }},
		{"wait thread", func(v *VM) { v.Threads[7].WaitThread++ }},
		{"active count", func(v *VM) { v.activeThreadCount++ }},
		{"move busy", func(v *VM) { v.anims[0].axes[1].moveBusy = true }},
		{"move speed", func(v *VM) { v.anims[0].axes[1].moveSpeed++ }},
		{"move target", func(v *VM) { v.anims[0].axes[1].moveTarget++ }},
		{"spin acceleration", func(v *VM) { v.anims[0].axes[1].spinAccel++ }},
		{"spin active", func(v *VM) { v.anims[0].axes[1].spinActive = true }},
		{"spin target", func(v *VM) { v.anims[0].axes[1].spinTarget++ }},
		{"turn busy", func(v *VM) { v.anims[0].axes[1].turnBusy = true }},
		{"turn speed", func(v *VM) { v.anims[0].axes[1].turnSpeed++ }},
		{"turn target", func(v *VM) { v.anims[0].axes[1].turnTarget++ }},
		{"dirty", func(v *VM) { v.dirty = true }},
		{"last return identity", func(v *VM) { v.lastReturnIdentity[7]++ }},
		{"last return valid", func(v *VM) { v.lastReturnValid[7] = true }},
		{"last return value", func(v *VM) { v.lastReturnValue[7]++ }},
		{"next identity", func(v *VM) { v.nextIdentity++ }},
		{"fallback flags", func(v *VM) { v.pieceFlags[0] ^= 1 }},
		{"statics", func(v *VM) { v.statics = append(v.statics, 1) }},
		{"thread identity", func(v *VM) { v.threadIdentity[7]++ }},
		{"tick denominator", func(v *VM) { v.tickDenom++ }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			vm := checkpointVM(t)
			before := checkpointVMBytes(t, vm)
			tc.mutate(vm)
			after := checkpointVMBytes(t, vm)
			if bytes.Equal(before, after) {
				t.Fatal("retained state missing from checkpoint")
			}
			if !bytes.Equal(after, checkpointVMBytes(t, vm)) {
				t.Fatal("capture mutated retained state")
			}
		})
	}
}

func TestCheckpointVMExcludesScratchAndPresentation(t *testing.T) {
	vm := checkpointVM(t)
	before := checkpointVMBytes(t, vm)
	vm.cacheRevision++
	vm.cacheValidityRevision++
	vm.DrainCalls++
	vm.lastStarted, vm.lastQueryThread = 7, 6
	vm.diagnostics = []string{"authored diagnostic"}
	vm.diagsDropped++
	vm.pieceBusy[0] = true
	vm.Pieces[0].Hidden, vm.Pieces[0].DontCache = true, true
	vm.Pieces[0].DontShade, vm.Pieces[0].DontShadow = true, true
	if !bytes.Equal(before, checkpointVMBytes(t, vm)) {
		t.Fatal("excluded state changed checkpoint")
	}
}

func TestCheckpointTraceOnlyClosureMatchesUntracedState(t *testing.T) {
	plain, traced := NewCallbackBridge(checkpointVM(t)), NewCallbackBridge(checkpointVM(t))
	traced.SetLifecycleSink(func(LifecycleEvent) {})
	traced.SetLifecycleContext(123, 45)
	plain.Create()
	traced.Create()
	for i := 0; i < 3; i++ {
		if !bytes.Equal(checkpointVMBytes(t, plain.VM), checkpointVMBytes(t, traced.VM)) {
			t.Fatal("trace-only receiver changed canonical VM")
		}
		plain.Drain(1)
		traced.Drain(1)
	}
	var a, b bytes.Buffer
	if err := plain.WriteCheckpoint(checkpoint.NewEncoder(&a)); err != nil {
		t.Fatal(err)
	}
	if err := traced.WriteCheckpoint(checkpoint.NewEncoder(&b)); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(a.Bytes(), b.Bytes()) || !bytes.Equal(a.Bytes(), []byte{1}) {
		t.Fatal("bridge checkpoint did not retain only Create invocation")
	}
}

func TestCheckpointSlotAimMetadataFollowsReceiverLifecycle(t *testing.T) {
	cases := []struct {
		name string
		end  func(*testing.T, *VM)
	}{
		{"explicit return", func(_ *testing.T, v *VM) { v.Drain(1); v.Drain(1); v.Drain(1) }},
		{"signal", func(_ *testing.T, v *VM) { v.Signal(1) }},
		{"invalid opcode", func(_ *testing.T, v *VM) { v.prog.Code[0] = 0; v.Drain(1) }},
		{"program replacement", func(_ *testing.T, v *VM) { v.SetProgram(v.Program()) }},
		{"retail restore", func(t *testing.T, v *VM) {
			image, err := RetailScriptImage(v)
			if err != nil {
				t.Fatal(err)
			}
			if err := RetailScriptRestore(v, image); err != nil {
				t.Fatal(err)
			}
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			vm := checkpointVM(t)
			bridge := NewCallbackBridge(vm)
			bridge.SetLifecycleSink(func(LifecycleEvent) {})
			calls := 0
			input := CheckpointContinuation{RawUnitKey: 7, Target: checkpoint.Allocation{Handle: 7, Serial: 9}}
			result := bridge.AimWithCheckpoint(WeaponPrimary, 11, 12, func(ret CallbackReturn) {
				calls++
				// Metadata is removed before calling the existing receiver.
				got, err := vm.CheckpointContinuations()
				if err != nil || got != ([8]CheckpointContinuation{}) {
					t.Fatalf("metadata still present during completion: %v, %v", got, err)
				}
				if ret.Value != 1 || !ret.Explicit {
					t.Fatalf("changed return: %+v", ret)
				}
			}, input)
			if !result.Started {
				t.Fatal("Aim start failed")
			}
			got, err := vm.CheckpointContinuations()
			if err != nil {
				t.Fatal(err)
			}
			want := input
			want.Kind, want.Mode = CheckpointContinuationSlotAim, ModeDeferred
			want.ThreadIdentity = vm.ThreadIdentity(result.Thread)
			if got[0] != want {
				t.Fatalf("descriptor %+v, want %+v", got[0], want)
			}
			checkpointVMBytes(t, vm)
			tc.end(t, vm)
			got, err = vm.CheckpointContinuations()
			if err != nil || got != ([8]CheckpointContinuation{}) {
				t.Fatalf("retired metadata: %+v, %v", got, err)
			}
			wantCalls := 0
			if tc.name == "explicit return" {
				wantCalls = 1
			}
			if calls != wantCalls {
				t.Fatalf("completion calls %d, want %d", calls, wantCalls)
			}
		})
	}
}

func TestCheckpointAimDescriptorDoesNotFollowReusedThread(t *testing.T) {
	vm := NewVM(buildReuseProg(t, []uint32{0x10021001, 1000, 0x10013000, 0x10065000}, true))
	b := NewCallbackBridge(vm)
	input := CheckpointContinuation{RawUnitKey: 7, Target: checkpoint.Allocation{Handle: 7, Serial: 9}}
	b.AimWithCheckpoint(WeaponPrimary, 0, 0, func(CallbackReturn) { t.Error("signalled aim answered") }, input)
	old := vm.ThreadIdentity(0)
	b.Deferred("Replace", nil, nil)
	b.Drain(1)
	if !vm.IsThreadAlive(0) || vm.ThreadIdentity(0) == old {
		t.Fatal("fixture failed to reuse slot inside drain")
	}
	got, err := vm.CheckpointContinuations()
	if err != nil || got != ([8]CheckpointContinuation{}) {
		t.Fatalf("replacement inherited descriptor: %+v, %v", got, err)
	}
	checkpointVMBytes(t, vm)
	b.Drain(1)
}

func TestCheckpointAimDescriptorWritesCapturedOperands(t *testing.T) {
	vm := checkpointVM(t)
	b := NewCallbackBridge(vm)
	b.AimWithCheckpoint(WeaponPrimary, 0, 0, func(CallbackReturn) {},
		CheckpointContinuation{RawUnitKey: 7, Target: checkpoint.Allocation{Handle: 7, Serial: 9}})
	before := checkpointVMBytes(t, vm)
	for _, mutate := range []func(*CheckpointContinuation){
		func(c *CheckpointContinuation) { c.RawUnitKey++ },
		func(c *CheckpointContinuation) { c.Target.Handle++ },
		func(c *CheckpointContinuation) { c.Target.Serial++ },
		func(c *CheckpointContinuation) { c.WeaponSlot++ },
	} {
		original := vm.checkpointReturns[0].continuation
		mutate(&vm.checkpointReturns[0].continuation)
		if bytes.Equal(before, checkpointVMBytes(t, vm)) {
			t.Fatal("captured continuation operand missing")
		}
		vm.checkpointReturns[0].continuation = original
	}
	vm.SetProgram(nil)
	// Nil replacement already leaves existing receivers untouched. Capture
	// refuses this unsupported state rather than changing its lifecycle.
	got, err := vm.CheckpointContinuations()
	if err != nil || got[0] != vm.checkpointReturns[0].continuation || vm.onReturn[0] == nil {
		t.Fatalf("nil-program replacement changed receiver metadata: %+v, %v", got, err)
	}
}

func TestCheckpointRejectsUnknownCompletionAndUnattestedBindings(t *testing.T) {
	cases := []struct {
		name string
		bind func(*VM)
	}{
		{"unknown direct completion", func(v *VM) {
			v.StartByName("AimPrimary", nil)
			v.SetThreadCompletion(0, v.ThreadIdentity(0), func(int32) {})
		}},
		{"unknown bridge completion", func(v *VM) { NewCallbackBridge(v).Aim(WeaponPrimary, 0, 0, func(CallbackReturn) {}) }},
		{"portFuncs", func(v *VM) { v.BindPort(1, func([]int32) int32 { return 0 }) }},
		{"portBindings", func(v *VM) { v.BindPortBinding(1, PortBinding{Write: func(int32) {}}) }},
		{"scriptTouched", func(v *VM) { v.BindScriptTouched(func() {}) }},
		{"cargoContains", func(v *VM) { v.BindTransportQueries(func(int32) bool { return false }, nil) }},
		{"carrierIdentity", func(v *VM) { v.BindTransportQueries(nil, func() int32 { return 0 }) }},
		{"transportAttach", func(v *VM) { v.BindTransportMutations(func(int32, int32, int32) {}, nil) }},
		{"transportDrop", func(v *VM) { v.BindTransportMutations(nil, func(int32) {}) }},
		{"renderFlags", func(v *VM) { v.BindRenderFlags(nil) }},
		{"render getter", func(v *VM) { v.BindRenderFlagHandlers(func() []uint8 { panic("capture called getter") }, nil) }},
		{"sfxSink", func(v *VM) { v.SetSFXSink(PresentationSinkAdapter{}) }},
		{"explosionSink", func(v *VM) { v.SetExplosionSink(&recordingExplosionSink{}) }},
		{"sfxVisible", func(v *VM) { v.SetSFXVisible(func(int, int32) bool { return true }) }},
		{"simRng", func(v *VM) { v.SetSimulationRNG(&rng.Simulation{}) }},
		{"nil program", func(v *VM) { v.SetProgram(nil) }},
		{"presentation VM", func(v *VM) { v.presentationInstructionLimit = 1 }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			vm := checkpointVM(t)
			tc.bind(vm)
			var out bytes.Buffer
			c := CheckpointContext{Program: checkpoint.Definition{Family: 1, Key: "authored-test"}}
			err := vm.WriteCheckpoint(checkpoint.NewEncoder(&out), &c)
			if err == nil || !strings.Contains(err.Error(), "logical path VM") || out.Len() != 0 {
				t.Fatalf("unsupported capture returned %v and %d bytes", err, out.Len())
			}
		})
	}
}
