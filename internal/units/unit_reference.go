package units

import (
	"fmt"
	"math"

	"github.com/nanolathe-gg/nanolathe/internal/pool"
)

// Reference returns the current successful allocation's command reference, or
// zero for an unoccupied slot or an allocation still being bound [I5].
func (w *World) Reference(h pool.Handle) pool.UnitRef {
	u := w.Unit(h)
	if u == nil || u.AllocationSerial == 0 {
		return pool.UnitRef{}
	}
	return pool.UnitRef{Handle: h, Serial: u.AllocationSerial}
}

// LookupReference rejects stale command references instead of redirecting them
// to a new occupant of the slot (DESIGN_MULTIPLAYER §16.2 M2-C3).
func (w *World) LookupReference(ref pool.UnitRef) *Unit {
	if ref.Serial == 0 {
		return nil
	}
	u := w.Unit(ref.Handle)
	if u == nil || u.AllocationSerial != ref.Serial {
		return nil
	}
	return u
}

// LastAllocationSerial is the battle-wide successful creation count. Failed
// creations consume no serial (DESIGN_MULTIPLAYER §16.2 M2-C1).
func (w *World) LastAllocationSerial() uint64 {
	if w == nil {
		return 0
	}
	return w.lastAllocationSerial
}

// reserveAllocationSerial reserves capacity, not a serial: nested binders can
// create units before their caller finishes. Each success gets the next serial
// only after binding. Refuse before allocation or RNG draws when every remaining
// serial is already promised (DESIGN_MULTIPLAYER §16.2 M2-C1).
func (w *World) reserveAllocationSerial() error {
	if math.MaxUint64-w.lastAllocationSerial <= w.pendingAllocationSerials {
		return fmt.Errorf("nanolathe: unit allocation serial exhausted: logical path <battle>, providers searched [unit world], expected unused allocation serial")
	}
	w.pendingAllocationSerials++
	return nil
}
