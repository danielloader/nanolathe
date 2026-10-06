package main

import (
	"testing"

	"github.com/nanolathe-gg/nanolathe/internal/economy"
	"github.com/nanolathe-gg/nanolathe/internal/input"
	"github.com/nanolathe-gg/nanolathe/internal/pool"
	"github.com/nanolathe-gg/nanolathe/internal/sim/numeric"
	"github.com/nanolathe-gg/nanolathe/internal/units"
)

func configureFollowCycleOwner(b *battleSession) {
	b.sess.Econ = &economy.Service{}
	b.sess.Econ.Players[b.sess.LocalOwner].Exists = true
	b.sess.Skirmish.UnitLimit = b.sess.Units.UnitLimit()
}

func followCycleFixture(t *testing.T, owner uint8, permutation pool.PlayerPermutation) (*battleSession, []*units.Unit) {
	t.Helper()
	b := newTestBattle(testCatalogON05(), testWorldON05(40, 40))
	var err error
	b.sess.Units, err = units.NewSlicedWithOrder(8, b.cat, permutation)
	if err != nil {
		t.Fatal(err)
	}
	b.sess.LocalOwner = owner
	configureFollowCycleOwner(b)
	var selected []*units.Unit
	for i := 0; i < 3; i++ {
		h, err := b.sess.Units.Create(b.cat.Units["armcons"], owner, (200+80*numeric.Fixed(i))<<16, 0, 120<<16)
		if err != nil {
			t.Fatal(err)
		}
		selected = append(selected, b.sess.Units.Unit(h))
	}
	replaceSelectionForTest(t, b, selected...)
	return b, selected
}

// Deselecting a still-tracked unit does not restart the slot scan at an end
// of the remaining selection [07 R-CAM-01 §12].
func TestFollowCycleRetainsDeselectedAnchor(t *testing.T) {
	b, us := followCycleFixture(t, 0, pool.IdentityPlayerPermutation())
	b.cam.SetTracked(us[1].Handle)
	x, y := screenPos(b.cam, us[1])
	clickAt(b, x, y, true)
	if hostSelected(b, us[1]) || b.cam.Tracked() != us[1].Handle {
		t.Fatal("Shift-click did not leave a deselected tracked unit")
	}
	pressKeys(b, input.KeyT)
	if b.cam.Tracked() != us[2].Handle {
		t.Fatal("forward cycle restarted before the deselected anchor")
	}
	b.cam.SetTracked(us[1].Handle)
	pressKeys(b, input.KeyShift, input.KeyT)
	if b.cam.Tracked() != us[0].Handle {
		t.Fatal("backward cycle restarted after the deselected anchor")
	}
}

func TestFollowCycleUsesPublishedOwnerRange(t *testing.T) {
	for _, owner := range []uint8{0, 3} {
		order := pool.IdentityPlayerPermutation()
		if owner == 3 {
			order[1], order[3] = order[3], order[1]
		}
		b, us := followCycleFixture(t, owner, order)
		f, _ := b.currentSnapshot()
		first, last, _ := b.sess.Units.SliceForPlayer(int(owner))
		if int(f.Players[owner].UnitSlotStart) != first || int(f.Strip.UnitLimit) != last-first+1 {
			t.Fatal("publication does not describe the permuted allocator range")
		}
		replaceSelectionForTest(t, b, us[0], us[2])
		for _, anchor := range []pool.Handle{0, pool.Handle(last + 1)} {
			b.cam.SetTracked(anchor)
			pressKeys(b, input.KeyT)
			if b.cam.Tracked() != us[2].Handle {
				t.Fatalf("owner %d forward normalized anchor %d did not skip first slot", owner, anchor)
			}
			b.cam.SetTracked(anchor)
			pressKeys(b, input.KeyShift, input.KeyT)
			if b.cam.Tracked() != us[2].Handle {
				t.Fatalf("owner %d backward normalized anchor did not wrap", owner)
			}
		}
		b.cam.SetTracked(us[2].Handle)
		pressKeys(b, input.KeyT)
		if b.cam.Tracked() != us[0].Handle {
			t.Fatal("forward wrap did not reach the first actual slot")
		}
		replaceSelectionForTest(t, b, us[0])
		for _, keys := range [][]input.Key{{input.KeyT}, {input.KeyShift, input.KeyT}} {
			pressKeys(b, keys...)
			if b.cam.Tracked() != us[0].Handle {
				t.Fatal("sole selected unit was lost on wrap")
			}
		}
		replaceSelectionForTest(t, b)
		pressKeys(b, input.KeyT)
		if b.cam.Tracked() != 0 {
			t.Fatal("empty selection retained tracking")
		}
	}
}

func TestFollowCycleRequiresPublishedRange(t *testing.T) {
	b := newTestBattle(testCatalogON05(), testWorldON05(40, 40))
	applyPendingBattleCommands(b)
	b.cam.SetTracked(7)
	b.cycleFollowTarget(false)
	if b.cam.Tracked() != 7 {
		t.Fatal("absent owner range fabricated a scan")
	}
	b.sess.Snapshot = nil
	b.cycleFollowTarget(true)
	if b.cam.Tracked() != 7 {
		t.Fatal("absent publication fabricated a scan")
	}
}
