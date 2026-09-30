package movement

// OverlapRules is Modern's movement as it stood until 2026-09-29, kept as the
// pathfinding laboratory's baseline (docs/PATHFINDING_LAB.md): friendly units
// may share cells under allied pass-through, jam release and pocket release,
// the re-route throttle is retail's lengthened by a stagger, and there is no
// traffic policy. Modern retired those answers for its traffic policy, under
// which every unit keeps its own cells (docs/DESIGN_MOVEMENT_PATH.md "Modern
// traffic").
//
// No reserved rule set binds it and the game links no rule set that does. The
// laboratory's replays, the path benchmark's comparison sets and the retired
// policies' own tests bind it, so that what Modern was can be measured beside
// what it is. Every other answer is Modern's.
type OverlapRules struct{ ModernRules }

// AlliedPassThrough lets head-on friendly movers pass
// (DESIGN_MOVEMENT_PATH "Modern allied pass-through").
func (*OverlapRules) AlliedPassThrough(*System) bool { return true }

// Traffic is none: a refused step is answered as retail answers it.
func (*OverlapRules) Traffic(*System) Traffic { return Traffic{} }

// overlapRepathSpread is the number of distinct staggered re-route delays,
// retail's 60 through 67 ticks
// (docs/DESIGN_MOVEMENT_PATH.md "Modern re-route staggering").
const overlapRepathSpread = 8

// RepathDelay adds a deterministic per-admission offset of 0..7 ticks to
// retail's throttle, so followers admitted on the same tick — a group order,
// or a cohort that has re-requested in step ever since — come due on
// different ticks. The offset mixes the unit's slot with the tick of its last
// admission rather than using the slot alone: a fixed per-slot phase would
// split a cohort into eight sub-cohorts that then stay in step for ever,
// while re-mixing at every admission keeps separating units that happen to
// share a tick (docs/DESIGN_MOVEMENT_PATH.md "Modern re-route staggering").
func (*OverlapRules) RepathDelay(_ *System, slot int, last uint32) uint32 {
	// Odd multipliers of the kind public integer hashes use (the first is the
	// 32-bit golden ratio); any well-mixing odd constants would serve.
	mix := uint32(slot)*0x9e3779b1 ^ (last+1)*0x85ebca6b
	mix ^= mix >> 15
	mix *= 0x2c1b3c6d
	return retailRepathDelay + uint32(uint64(mix)*overlapRepathSpread>>32)
}
