package combat

import (
	"testing"

	"github.com/nanolathe-gg/nanolathe/internal/cob"
	"github.com/nanolathe-gg/nanolathe/internal/sim/checkpoint"
	"github.com/nanolathe-gg/nanolathe/internal/units"
)

// The descriptor observes the existing receiver; it cannot advance its return,
// grant readiness, or address a later allocation that reuses the raw handle
// (DESIGN_MULTIPLAYER §16.3.5; [04 R-CB-01 §6]).
func TestCheckpointAimDescriptorKeepsCapturedReceiver(t *testing.T) {
	var descriptors [2][8]cob.CheckpointContinuation
	for pass, trace := range []bool{false, true} {
		vm := cob.NewVM(progWithAim([]uint32{
			0x10021001, 1, 0x10065000, // authored return 1
		}, "AimPrimary", 0))
		bridge := cob.NewCallbackBridge(vm)
		if trace {
			bridge.SetLifecycleSink(func(cob.LifecycleEvent) {})
		}
		u := &units.Unit{Handle: 7, AllocationSerial: 41}
		u.Slots[0].Weapon = weaponTurret(1)
		var svc Service
		var sum UnitStepSummary
		svc.dispatchSlotAim(u, &u.Slots[0], 0, 9, bridge, 100, 200, &sum)
		before := vm.Threads
		got, err := vm.CheckpointContinuations()
		if err != nil {
			t.Fatal(err)
		}
		descriptors[pass] = got
		thread := vm.LastStartedThread()
		want := checkpoint.Allocation{Handle: 7, Serial: 41}
		if thread < 0 || got[thread].Kind != cob.CheckpointContinuationSlotAim || got[thread].ThreadSlot != uint8(thread) ||
			got[thread].ThreadIdentity == 0 || got[thread].Target != want || got[thread].RawUnitKey != 7 || got[thread].WeaponSlot != 0 {
			t.Fatalf("wrong captured receiver: %+v", got)
		}
		if vm.Threads != before || u.Slots[0].Aim.Ready || sum.ReturnSeen {
			t.Fatal("descriptor inspection advanced or completed the callback")
		}
		// The callback captured this slot, regardless of a different record
		// now carrying its raw handle in an outer allocation index.
		reused := &units.Unit{Handle: u.Handle, AllocationSerial: 42}
		vm.Drain(1)
		if !u.Slots[0].Aim.Ready || reused.Slots[0].Aim.Ready || len(svc.pendingAims) != 0 {
			t.Fatal("the original receiver or completion cleanup changed")
		}
		got, err = vm.CheckpointContinuations()
		if err != nil || got != ([8]cob.CheckpointContinuation{}) {
			t.Fatalf("completed continuation retained: %+v, %v", got, err)
		}
	}
	if descriptors[0] != descriptors[1] {
		t.Fatal("lifecycle tracing changed the gameplay descriptor")
	}
}
