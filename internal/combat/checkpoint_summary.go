package combat

import "github.com/nanolathe-gg/nanolathe/internal/sim/checkpoint"

// AppendCheckpointSummary is the deliberately cheap row in
// DESIGN_MULTIPLAYER §16.3.7 and §16.3.16. It reads stored scalar values only:
// no bindings, keys, allocation graph, target query or arena initialization.
// Selected target lists and modernTick are deliberate blind spots; the full
// writer retains both. All signed operands extend to 64 bits before the fold.
func (s *Service) AppendCheckpointSummary(summary *checkpoint.Summary) error {
	if s == nil {
		return combatCheckpointError("combat.Service", "a present service")
	}
	if summary == nil {
		return combatCheckpointError("combat.summary", "a summary accumulator")
	}
	next := *summary
	next.Word(uint64(s.Slots.Count()))
	next.Word(uint64(s.Slots.Capacity()))
	next.Word(uint64(len(s.Records)))
	for _, p := range s.Records {
		dead := uint64(0)
		if p.Dead {
			dead = 1
		}
		next.Word(dead)
		next.Word(uint64(int64(p.WeaponID)))
		next.Word(uint64(p.Pos.X))
		next.Word(uint64(p.Pos.Y))
		next.Word(uint64(p.Pos.Z))
		next.Word(uint64(p.TargetUnit))
		next.Word(uint64(p.TargetProjectile))
		next.Word(uint64(p.TargetPos.X))
		next.Word(uint64(p.TargetPos.Y))
		next.Word(uint64(p.TargetPos.Z))
		next.Word(uint64(p.Shooter))
		next.Word(uint64(p.Velocity.X))
		next.Word(uint64(p.Velocity.Y))
		next.Word(uint64(p.Velocity.Z))
		next.Word(uint64(p.ExpiryTick))
		next.Word(uint64(int64(p.BurstRemaining)))
	}
	for _, v := range s.targets.lastRebuild {
		next.Word(uint64(v))
	}
	for _, v := range s.targets.gate {
		word := uint64(0)
		if v {
			word = 1
		}
		next.Word(word)
	}
	for _, v := range s.scanCursor.next {
		next.Word(uint64(int64(v)))
	}
	next.Word(uint64(s.modernNextProjectileTick))
	next.Word(uint64(s.communityAreaGenCounter))
	*summary = next
	return nil
}
