package aikit

import (
	"math"

	"github.com/nanolathe-gg/nanolathe/internal/orders"
	"github.com/nanolathe-gg/nanolathe/internal/pool"
	"github.com/nanolathe-gg/nanolathe/internal/units"
)

// Stockpile requests ordinary ammunition production for one weapon slot.
// It spends one persona action and preserves the actor's primary orders.
func (k *Kit) Stockpile(actor pool.Handle, slot, count int32) {
	k.push(Command{Kind: CmdStockpile, Slot: slot, Count: count}, []pool.Handle{actor})
}

// stockpileState copies the completed rounds and positive signed queue counts
// [06 §11.1]. Saturation keeps malformed queues from wrapping observations.
func stockpileState(u *units.Unit) (ammo, queued [3]int32) {
	hasStockpile := false
	for i := range ammo {
		ammo[i] = u.Slots[i].Ammo
		w := u.Slots[i].Weapon
		hasStockpile = hasStockpile || (w != nil && w.Stockpile)
	}
	if !hasStockpile {
		return
	}
	q := orders.QueueOfUnit(u)
	if q == nil {
		return
	}
	id := orders.Lookup("BuildWeapon")
	for _, n := range q.Secondary() {
		if n == nil || n.ID != id || n.Param1 >= uint32(len(queued)) || int32(n.Param2) <= 0 {
			continue
		}
		i := n.Param1
		queued[i] = int32(min(int64(math.MaxInt32), int64(queued[i])+int64(int32(n.Param2))))
	}
	return
}

func (e *executor) execStockpile(c *Command, b *batch, tick uint32, w *units.World) bool {
	if c.count != 1 || c.Slot < 0 || c.Slot >= units.NumSlots || c.Count <= 0 {
		e.stats.Failed++
		return false
	}
	u := e.actorOK(w, b.actors[c.first], b.inst[c.first])
	if u == nil {
		e.stats.Stale++
		e.stats.Reasons[FailNoActor]++
		return false
	}
	if u.Remaining != 0 || !orders.StockpileSlotAcceptsBuildWeapon(u, int(c.Slot)) || !weaponActive(u.Slots[c.Slot].Weapon) {
		e.stats.Failed++
		e.stats.Reasons[FailBuildGate]++
		return false
	}
	ammo, queued := stockpileState(u)
	// Controller request bound: at most the ordinary production cap of 200
	// completed plus pending rounds [06 §11.1]. Production still owns the
	// actual cap, timing and resource admission; enqueue grants no rounds.
	count := min(int64(c.Count), 200-int64(max(ammo[c.Slot], 0))-int64(queued[c.Slot]))
	if count <= 0 {
		e.stats.Failed++
		e.stats.Reasons[FailQueue]++
		return false
	}
	q := orders.BindQueueBinding(u, e.m.OrderBinding)
	if q == nil {
		e.stats.Failed++
		e.stats.Reasons[FailQueue]++
		return false
	}
	id := orders.Lookup("BuildWeapon")
	n := orders.NewNodeForOrder(id, 0, 0, 0, 0, tick, u.Handle, false)
	n.Param1, n.Param2 = uint32(c.Slot), uint32(count)
	q.CoalesceTail(id, n)
	return true
}
