package pool

import (
	"fmt"

	"github.com/nanolathe-gg/nanolathe/internal/sim/checkpoint"
)

// WriteCheckpoint writes the allocator's retained fields in lexical order:
// alive, defID, limit, slices (end, start). The identity slotIndex and used
// count are validated derivations; sliced is derived from limit. Slot zero
// remains in both physical arrays (DESIGN_MULTIPLAYER §16.3.5–§16.3.6).
func (p *Units) WriteCheckpoint(e *checkpoint.Encoder) error {
	e.Field("pool.Units")
	if err := p.validateCheckpoint(); err != nil {
		e.Fail(err)
		return e.Err()
	}
	e.Field("pool.Units.alive")
	e.Count(len(p.alive))
	for _, alive := range p.alive {
		e.Bool(alive)
	}
	e.Field("pool.Units.defID")
	e.Count(len(p.defID))
	for _, id := range p.defID {
		e.U16(id)
	}
	e.Field("pool.Units.limit")
	e.I64(int64(p.limit))
	e.Field("pool.Units.slices")
	for _, bounds := range p.slices {
		e.I64(int64(bounds.end))
		e.I64(int64(bounds.start))
	}
	return e.Err()
}

func (p *Units) validateCheckpoint() error {
	if p == nil {
		return fmt.Errorf("missing unit pool")
	}
	if p.limit < 0 || len(p.alive) < 1 || (len(p.alive)-1)/PlayerCount != p.limit || (len(p.alive)-1)%PlayerCount != 0 {
		return fmt.Errorf("arena length does not match per-player limit")
	}
	if len(p.defID) != len(p.alive) || len(p.slotIndex) != len(p.alive) {
		return fmt.Errorf("arena arrays have different lengths")
	}
	used := 0
	for i, alive := range p.alive {
		if p.slotIndex[i] != uint16(i) {
			return fmt.Errorf("slotIndex[%d] is not its physical identity", i)
		}
		if alive != (p.defID[i] != 0) || (i == 0 && alive) {
			return fmt.Errorf("alive/definition identity disagree at slot %d", i)
		}
		if alive {
			used++
		}
	}
	if used != p.used {
		return fmt.Errorf("used count disagrees with occupied records")
	}
	if p.sliced != (p.limit > 0) {
		return fmt.Errorf("sliced state disagrees with per-player limit")
	}
	var seen [PlayerCount]bool
	for player, bounds := range p.slices {
		if p.limit == 0 {
			if bounds.start != 0 || bounds.end != -1 {
				return fmt.Errorf("empty slice %d has invalid bounds", player)
			}
			continue
		}
		// Bounds encode the admitted player permutation. Do not sort it away
		// or reconstruct it in player order [04 R-P0-16-A].
		if bounds.start < 1 || bounds.end >= len(p.alive) || bounds.end-bounds.start+1 != p.limit || (bounds.start-1)%p.limit != 0 {
			return fmt.Errorf("player slice %d has invalid bounds", player)
		}
		slice := (bounds.start - 1) / p.limit
		if seen[slice] {
			return fmt.Errorf("player slice %d repeats another slice", player)
		}
		seen[slice] = true
	}
	return nil
}
