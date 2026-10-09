package effects

import (
	"fmt"

	"github.com/nanolathe-gg/nanolathe/internal/sim/checkpoint"
)

// WriteCheckpoint retains blocks, count, cursor, points, serial, slots,
// storageCharge in lexical order. Blocks are the active prefix only, with
// charge/generation/occupied/slot/start. Points are occupied spans in block
// order, each block index i64, pointStart i64, u32 point count, then XYZ i64.
// Slots have their physical count, then live presence and live payload only:
// angles, angularRates, explodeOnHit, fall, generation, lifetime, pointCount,
// pointStart, position, velocity. Drawing metadata, dead-slot payload, inactive
// partition backing and free points are excluded (DESIGN_MULTIPLAYER §16.3.17).
func (p *DebrisPool) WriteCheckpoint(e *checkpoint.Encoder) error {
	e.Field("effects.DebrisPool")
	if err := p.checkpointBoundary(); err != nil {
		e.Fail(err)
		return e.Err()
	}
	e.Field("effects.DebrisPool.blocks")
	e.Count(p.count)
	occupied := 0
	for i := 0; i < p.count; i++ {
		b := &p.blocks[i]
		path := fmt.Sprintf("effects.DebrisPool.blocks[%d]", i)
		e.Field(path + ".charge")
		e.I64(int64(b.charge))
		e.Field(path + ".generation")
		e.U64(b.generation)
		e.Field(path + ".occupied")
		e.Bool(b.occupied)
		e.Field(path + ".slot")
		e.I64(int64(b.slot))
		e.Field(path + ".start")
		e.I64(int64(b.start))
		if b.occupied {
			occupied++
		}
	}
	e.Field("effects.DebrisPool.count")
	e.I64(int64(p.count))
	e.Field("effects.DebrisPool.cursor")
	e.I64(int64(p.cursor))
	e.Field("effects.DebrisPool.points")
	e.Count(occupied)
	for i := 0; i < p.count; i++ {
		b := &p.blocks[i]
		if !b.occupied {
			continue
		}
		start, count := checkpointDebrisSpan(b)
		e.Field(fmt.Sprintf("effects.DebrisPool.points.block[%d]", i))
		e.I64(int64(i))
		e.I64(int64(start))
		e.Count(count)
		for _, point := range p.points[start : start+count] {
			for _, value := range point {
				e.I64(int64(value))
			}
		}
	}
	e.Field("effects.DebrisPool.serial")
	e.U64(p.serial)
	e.Field("effects.DebrisPool.slots")
	e.Count(len(p.slots))
	for i := range p.slots {
		s := &p.slots[i]
		path := fmt.Sprintf("effects.DebrisPool.slots[%d]", i)
		e.Field(path + ".live")
		e.Bool(s.live)
		if !s.live {
			continue
		}
		e.Field(path + ".angles")
		for _, value := range s.angles {
			e.U16(value)
		}
		e.Field(path + ".angularRates")
		for _, value := range s.angularRates {
			e.U16(value)
		}
		e.Field(path + ".explodeOnHit")
		e.Bool(s.explodeOnHit)
		e.Field(path + ".fall")
		e.Bool(s.fall)
		e.Field(path + ".generation")
		e.U64(s.generation)
		e.Field(path + ".lifetime")
		e.U16(s.lifetime)
		e.Field(path + ".pointCount")
		e.I64(int64(s.pointCount))
		e.Field(path + ".pointStart")
		e.I64(int64(s.pointStart))
		e.Field(path + ".position")
		for _, value := range s.position {
			e.I64(int64(value))
		}
		e.Field(path + ".velocity")
		for _, value := range s.velocity {
			e.I64(int64(value))
		}
	}
	e.Field("effects.DebrisPool.storageCharge")
	e.I64(int64(p.storageCharge))
	return e.Err()
}

// Admission charges twelve per copied vertex plus the fixed charge and may
// absorb fewer than nine trailing charges. Integer division thus recovers the
// copied span even after the original slot has died or been reused; today's
// slot pointCount is not its authority ([04 R-COB-04 §2]; §16.3.17).
func checkpointDebrisSpan(b *debrisBlock) (start, count int) {
	return b.start / 12, (b.charge - debrisFixedCharge) / 12
}

func (p *DebrisPool) checkpointBoundary() error {
	if p == nil {
		return effectsCheckpointError("effects.DebrisPool", "a present debris pool")
	}
	if p.storageCharge == 0 && len(p.slots) == 0 && len(p.blocks) == 0 && len(p.points) == 0 && p.count == 0 && p.cursor == 0 {
		return nil // The zero value remains uninitialized; capture never ensures storage.
	}
	if p.storageCharge <= 0 || len(p.slots) == 0 || len(p.blocks) != p.storageCharge/debrisSplitMinimum+1 || len(p.points) != p.storageCharge/12+1 {
		return effectsCheckpointError("effects.DebrisPool.storageCharge", "initialized backing dimensions matching the stored charge")
	}
	if p.count < 0 || p.count > len(p.blocks) {
		return effectsCheckpointError("effects.DebrisPool.count", "an active partition prefix within backing storage")
	}
	if p.cursor < 0 || (p.count == 0 && p.cursor != 0) || (p.count > 0 && p.cursor >= p.count) {
		return effectsCheckpointError("effects.DebrisPool.cursor", "a cursor within the active partition prefix")
	}
	covered := 0
	for i := 0; i < p.count; i++ {
		b := &p.blocks[i]
		path := fmt.Sprintf("effects.DebrisPool.blocks[%d]", i)
		if b.start != covered || b.charge <= 0 || b.charge > p.storageCharge-covered {
			return effectsCheckpointError(path, "ordered contiguous partitions within the stored charge")
		}
		covered += b.charge
		if !b.occupied {
			continue
		}
		if b.slot < 0 || b.slot >= len(p.slots) || b.charge < debrisFixedCharge {
			return effectsCheckpointError(path, "an occupied partition with an in-range slot and geometry charge")
		}
		start, count := checkpointDebrisSpan(b)
		if start > len(p.points) || count > len(p.points)-start {
			return effectsCheckpointError(path, "an occupied geometry span within point storage")
		}
	}
	if p.count != 0 && covered != p.storageCharge {
		return effectsCheckpointError("effects.DebrisPool.blocks", "partitions covering the stored charge")
	}
	for i := range p.slots {
		s := &p.slots[i]
		if s.live && (s.pointStart < 0 || s.pointCount < 0 || s.pointStart > len(p.points) || s.pointCount > len(p.points)-s.pointStart) {
			return effectsCheckpointError(fmt.Sprintf("effects.DebrisPool.slots[%d]", i), "a live point span within storage")
		}
	}
	return nil
}

// AppendCheckpointSummary contributes the live-slot count directly. Partition
// count/charge and geometry are full-only state (DESIGN_MULTIPLAYER §16.3.17).
func (p *DebrisPool) AppendCheckpointSummary(summary *checkpoint.Summary) error {
	if p == nil {
		return effectsCheckpointError("effects.DebrisPool", "a present debris pool")
	}
	if summary == nil {
		return effectsCheckpointError("effects.summary", "a summary accumulator")
	}
	var live uint64
	for i := range p.slots {
		if p.slots[i].live {
			live++
		}
	}
	summary.Word(live)
	return nil
}
