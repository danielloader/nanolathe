package session

import (
	"math"

	"github.com/nanolathe-gg/nanolathe/internal/orders"
	"github.com/nanolathe-gg/nanolathe/internal/pool"
	"github.com/nanolathe-gg/nanolathe/internal/sim/checkpoint"
)

// The summary walks selected fields directly; it does not build reference
// tables or encode bytes. Scratch accumulators stay with diagnostics so the
// controller adapter cannot cause a new escaping accumulator each tick (.78).
func (s *Session) checkpointRingRow(position CheckpointPosition) (CheckpointRingRow, error) {
	fail := func(err error) (CheckpointRingRow, error) { return CheckpointRingRow{}, err }
	if s == nil || s.checkpoints == nil || s.Clock == nil || s.Units == nil || s.World == nil || s.Features == nil || s.Vis == nil || s.Movement == nil || s.Path == nil || s.Econ == nil || s.Build == nil || s.Combat == nil || s.publication == nil || s.publication.events == nil || s.publication.effects == nil || s.Snapshot == nil {
		return fail(runtimeCheckpointError("capture.summary", "complete owners at a published boundary"))
	}
	if tick, ok := s.Snapshot.PublishedTick(); !ok || tick != position.Tick || tick != s.Clock.GlobalTick || len(s.publication.events.StagingEvents()) != 0 {
		return fail(runtimeCheckpointError("capture.summary.publication", "a completed publication with no staged events"))
	}
	sums := &s.checkpoints.summaries
	*sums = [CheckpointOwnerCount]checkpoint.Summary{}
	row := CheckpointRingRow{Position: position, SimulationState: s.rngSim.State, CRTState: s.rngCrt.State, SimulationDraws: s.rngSim.Draws(), CRTDraws: s.rngCrt.Draws()}
	if err := s.appendCheckpointRuntimeSummary(&sums[0]); err != nil {
		return fail(err)
	}
	if err := s.Units.AppendCheckpointSummary(&sums[1]); err != nil {
		return fail(err)
	}
	for i := 1; i < s.Units.TotalRecords(); i++ {
		u := s.Units.Unit(pool.Handle(i))
		if u == nil {
			continue
		}
		switch q := u.Orders.(type) {
		case nil:
			sums[2].Word(0)
		case *orders.Queue:
			if q == nil {
				return fail(runtimeCheckpointError("capture.summary.orders", "a nonnil concrete queue or absence"))
			}
			sums[2].Word(1)
			if err := q.AppendCheckpointSummary(&sums[2]); err != nil {
				return fail(err)
			}
		default:
			return fail(runtimeCheckpointError("capture.summary.orders", "the concrete order queue"))
		}
		if u.Script == nil {
			sums[3].Word(0)
		} else {
			sums[3].Word(1)
			if err := u.Script.AppendCheckpointSummary(&sums[3]); err != nil {
				return fail(err)
			}
		}
	}
	if err := s.Features.AppendCheckpointSummary(&sums[4]); err != nil {
		return fail(err)
	}
	if err := s.World.AppendCheckpointSummary(&sums[4]); err != nil {
		return fail(err)
	}
	if err := s.Vis.AppendCheckpointSummary(&sums[5]); err != nil {
		return fail(err)
	}
	if err := s.appendCheckpointVisibilityTailSummary(&sums[5]); err != nil {
		return fail(err)
	}
	if err := s.Movement.AppendCheckpointSummary(&sums[6]); err != nil {
		return fail(err)
	}
	if err := s.Path.AppendCheckpointSummary(&sums[7]); err != nil {
		return fail(err)
	}
	if err := s.Movement.AppendPathCheckpointSummary(&sums[7]); err != nil {
		return fail(err)
	}
	if err := s.Econ.AppendCheckpointSummary(&sums[8]); err != nil {
		return fail(err)
	}
	if err := s.Build.AppendCheckpointSummary(&sums[9]); err != nil {
		return fail(err)
	}
	if err := s.Combat.AppendCheckpointSummary(&sums[10]); err != nil {
		return fail(err)
	}
	if err := s.publication.events.AppendCheckpointSummary(&sums[11]); err != nil {
		return fail(err)
	}
	if err := s.publication.effects.AppendCheckpointSummary(&sums[11]); err != nil {
		return fail(err)
	}
	fixed := s.publication.effects.CheckpointFixedPool()
	if err := fixed.AppendCheckpointSummary(&sums[11]); err != nil {
		return fail(err)
	}
	if s.debris != nil {
		if err := s.debris.AppendCheckpointSummary(&sums[11]); err != nil {
			return fail(err)
		}
	} else {
		sums[11].Word(0)
	}
	if s.strips != nil {
		if err := s.strips.appendCheckpointSummary(&sums[11]); err != nil {
			return fail(err)
		}
	} else {
		sums[11].Word(0)
	}
	for _, m := range s.AI {
		if m == nil {
			sums[12].Word(0)
			continue
		}
		sums[12].Word(1)
		if err := m.AppendCheckpointSummary(&sums[12]); err != nil {
			return fail(err)
		}
		if m.Ext != nil {
			if err := s.modernAICheckpointSource.AppendSummary(m, &sums[12]); err != nil {
				return fail(err)
			}
		} else {
			if err := m.CheckpointApplicationHistory().AppendCheckpointSummary(&sums[12]); err != nil {
				return fail(err)
			}
			for range 6 {
				sums[12].Word(0)
			}
		}
	}
	if err := s.appendScenarioCheckpointSummary(&sums[12]); err != nil {
		return fail(err)
	}
	for i := range sums {
		row.Owners[i].Words, row.Owners[i].Sum = sums[i].Result()
	}
	records, fragments := fixed.CheckpointCounts()
	strips := 0
	if s.strips != nil {
		strips = s.strips.live
	}
	counts := [...]int{s.Units.Used(), s.Combat.Count(), records, fragments, s.debris.SlotCount(), strips}
	for _, n := range counts {
		if n < 0 || uint64(n) > math.MaxUint32 {
			return fail(runtimeCheckpointError("capture.summary.counts", "pool counts representable by u32"))
		}
	}
	row.UnitCount, row.ProjectileCount, row.EffectCount, row.FragmentCount, row.DebrisCount, row.StripCount = uint32(counts[0]), uint32(counts[1]), uint32(counts[2]), uint32(counts[3]), uint32(counts[4]), uint32(counts[5])
	return row, nil
}
