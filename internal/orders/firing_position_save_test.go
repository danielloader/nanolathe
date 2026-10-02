package orders

import (
	"encoding/binary"
	"reflect"
	"testing"

	"github.com/nanolathe-gg/nanolathe/internal/pool"
	"github.com/nanolathe-gg/nanolathe/internal/save"
	"github.com/nanolathe-gg/nanolathe/internal/units"
)

func TestFiringPositionSaveRestartsAttackWithoutMutatingLiveOverlay(t *testing.T) {
	f := newFiringFixture()
	if !f.step(10) {
		t.Fatal("start failed")
	}
	f.n.Satisfied = 0x3E0 | pendDisengage
	f.n.RetailSubtypeCode = 2
	f.n.RetailSubtype = make([]byte, save.OrderSubtypeCode2)
	before, overlay := *f.n, f.q.firingPosition
	sourceCalls := 0
	images, err := RetailOrderImagesWithPayload(f.u, func(h pool.Handle) (uint16, bool) { return uint16(h), h != 0 }, nil, func(n *Node) (RetailOrderPayload, error) {
		sourceCalls++
		if n == f.n {
			t.Fatal("temporary movement payload reached save source")
		}
		return RetailOrderPayload{}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if sourceCalls != 1 || !reflect.DeepEqual(*f.n, before) || f.q.firingPosition != overlay || f.bound != f.n {
		t.Fatal("save changed live order/goal")
	}
	image := images[0]
	if image.Main[9] != 1 || binary.LittleEndian.Uint32(image.Main[0x0A:]) != pendDisengage || int32(binary.LittleEndian.Uint32(image.Main[0x0E:])) != -1 || binary.LittleEndian.Uint32(image.Main[0x36:]) != pendDisengage || image.SubtypeCode != 0 || len(image.Subtype) != 0 {
		t.Fatal("detached restart image retained travel")
	}
	record := save.OrderRecord{ParentStableID: image.ParentStableID, Sequence: image.Sequence, Secondary: image.Secondary, Main: image.Main, SubtypeCode: image.SubtypeCode, Subtype: image.Subtype, DescriptorName: image.DescriptorName}
	restored := &units.Unit{Handle: 10, Alive: true}
	if err := RetailRestoreOrdersAtTick(restored, []save.OrderRecord{record}, map[uint16]pool.Handle{1: 10, 2: 20}, nil, 10); err != nil {
		t.Fatal(err)
	}
	n := QueueOfUnit(restored).Head()
	if n.Phase != 1 || n.DynamicGate != pendDisengage || n.Target != 20 || n.Param2 != before.Param2 || n.GoalX != before.GoalX || n.GoalZ != before.GoalZ || QueueOfUnit(restored).firingPosition.active {
		t.Fatal("restore did not preserve restarted attack")
	}
}

func TestFiringPositionRestoredPumpDeliversSavedControl(t *testing.T) {
	for _, name := range []string{"Attack_Chase", "Attack_NoMove"} {
		for _, event := range []struct {
			name string
			bit  uint32
		}{{"disengage", pendDisengage}, {"removed", pendTargetRemoved}, {"cloaked", pendTargetCloaked}} {
			for _, fromUnit := range []bool{false, true} {
				origin := "record"
				if fromUnit {
					origin = "unit"
				}
				t.Run(name+"/"+event.name+"/"+origin, func(t *testing.T) {
					f := newFiringFixture()
					f.n.ID = Lookup(name)
					f.n.automaticAttack = true
					if !f.step(10) {
						t.Fatal("start failed")
					}
					f.n.Satisfied = 0x3E0
					if fromUnit {
						f.u.Pending = event.bit | 0x3E0
					} else {
						f.n.Satisfied |= event.bit
					}
					f.q.primary[1].DynamicGate, f.q.primary[1].Deadline = 0x400, -1
					before, overlay, pending := *f.n, f.q.firingPosition, f.u.Pending
					images, err := RetailOrderImagesWithPayload(f.u, func(h pool.Handle) (uint16, bool) { return uint16(h), h != 0 }, nil, nil)
					if err != nil {
						t.Fatal(err)
					}
					records := make([]save.OrderRecord, len(images))
					for i, image := range images {
						records[i] = save.OrderRecord{ParentStableID: image.ParentStableID, Sequence: image.Sequence, Secondary: image.Secondary, Main: image.Main, SubtypeCode: image.SubtypeCode, Subtype: image.Subtype, DescriptorName: image.DescriptorName}
					}
					restored := newFiringFixture()
					restored.blocked = false
					restored.u.Pending = pending // the unit save carries this word separately
					restored.q.Binding().Movement.Release = func(*Node) bool { return true }
					restored.q.Binding().Weapons.CanEngage = func(*units.Unit, pool.Handle, int) bool { return true }
					if err := RetailRestoreOrdersAtTick(restored.u, records, map[uint16]pool.Handle{1: 1, 2: 2}, restored.q.Binding(), 10); err != nil {
						t.Fatal(err)
					}
					q := QueueOfUnit(restored.u)
					attack, successor := q.primary[0], q.primary[1]
					handler := DescriptorFor(attack.ID).Handler
					calls := 0
					restore := setHandler(attack.ID, func(u *units.Unit, n *Node, satisfied, tick uint32) Code {
						calls++
						if satisfied != event.bit {
							t.Errorf("restored handler satisfied=%#x, want %#x only", satisfied, event.bit)
						}
						return handler(u, n, satisfied, tick)
					})
					defer restore()
					random := *q.Binding().SimRNG
					q.Pump(restored.u, 11)
					if calls != 1 || q.indexOfPrimary(attack) >= 0 || q.Head() != successor || attack.Satisfied&event.bit != 0 || restored.u.Pending&event.bit != 0 {
						t.Fatal("saved control event failed to complete attack before rebinding")
					}
					if *q.Binding().SimRNG != random {
						t.Fatal("restored cancellation drew RNG")
					}
					if !reflect.DeepEqual(*f.n, before) || f.q.firingPosition != overlay || f.bound != f.n || f.u.Pending != pending {
						t.Fatal("projection/restore mutated live overlay")
					}
				})
			}
		}
	}
}

func TestFiringPositionSaveDisplacedAttackDoesNotTakeUnitEvents(t *testing.T) {
	f := newFiringFixture()
	if !f.step(10) {
		t.Fatal("start failed")
	}
	control := &Node{ID: Lookup("Paralyze"), Owner: f.u.Handle}
	f.q.primary = append([]*Node{control}, f.q.primary...)
	f.u.Pending = pendDisengage | pendTargetGone
	before, pending := *f.n, f.u.Pending
	images, err := RetailOrderImagesWithPayload(f.u, func(h pool.Handle) (uint16, bool) { return uint16(h), h != 0 }, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	attack := images[1]
	if binary.LittleEndian.Uint32(attack.Main[0x0A:]) != 0 || binary.LittleEndian.Uint32(attack.Main[0x36:])&(pendDisengage|pendTargetGone) != 0 {
		t.Fatal("displaced attack stole the controlling head's unit events")
	}
	if !reflect.DeepEqual(*f.n, before) || f.u.Pending != pending {
		t.Fatal("save mutated live record or pending events")
	}
}
