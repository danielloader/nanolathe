package units

import (
	"fmt"
	"math"

	"github.com/nanolathe-gg/nanolathe/internal/sim/checkpoint"
)

// AppendCheckpointSummary reads physical live and residual slots followed by
// paired player counters. It does not collect allocation references or inspect
// content, bindings or allocator state (DESIGN_MULTIPLAYER §16.3.77).
func (w *World) AppendCheckpointSummary(out *checkpoint.Summary) error {
	if w == nil || out == nil {
		return unitCheckpointError("units.World.summary", "a present world and summary")
	}
	if len(w.rawUnits) != len(w.units) {
		return unitCheckpointError("units.World.summary.slots", "matching live and residual slot lengths")
	}
	next := *out
	next.Word(uint64(len(w.units)))
	for i, u := range w.units {
		live := u != nil
		if !live {
			u = w.rawUnits[i]
		}
		if u == nil {
			next.Word(0)
			continue
		}
		remaining := math.Float32bits(u.Remaining)
		if remaining&0x7f800000 == 0x7f800000 && remaining&0x007fffff != 0 {
			return unitCheckpointError(fmt.Sprintf("units.World.summary.slots[%d].Remaining", i), "a non-NaN selected value")
		}
		if !live {
			next.Word(2)
			next.Word(uint64(u.Handle))
			next.Word(uint64(u.Owner))
			next.Word(uint64(remaining))
			continue
		}
		next.Word(1)
		next.Word(uint64(u.Handle))
		next.Word(u.AllocationSerial)
		next.Word(uint64(u.Owner))
		next.Word(uint64(int64(u.Health)))
		next.Word(uint64(remaining))
		next.Word(uint64(u.Flags))
		next.Word(uint64(int64(u.X)))
		next.Word(uint64(int64(u.Y)))
		next.Word(uint64(int64(u.Z)))
		next.Word(uint64(u.Move.Heading))
		next.Word(uint64(int64(u.Move.Speed)))
		next.Word(uint64(u.Pending))
		if u.Stunned {
			next.Word(1)
		} else {
			next.Word(0)
		}
		next.Word(uint64(u.ParalyzeExpire))
		for slot := range u.Slots {
			s := &u.Slots[slot]
			next.Word(uint64(int64(s.Reload)))
			next.Word(uint64(int64(s.Ammo)))
			if err := s.Aim.AppendCheckpointSummary(&next); err != nil {
				return err
			}
		}
	}
	for player, count := range w.liveCounters {
		next.Word(uint64(int64(count)))
		next.Word(uint64(w.createdCounters[player]))
	}
	*out = next
	return nil
}
