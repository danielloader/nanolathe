package session

import (
	"fmt"

	"github.com/nanolathe-gg/nanolathe/internal/sim/checkpoint"
)

// The section composer supplies table presence. The payload preserves live,
// poolCapacity, steadyCap, then ten ordered lists (DESIGN_MULTIPLAYER §16.3.37).
// It reads stored animation operands without resolving art or drawing RNG.
func (t *stripTable) writeCheckpoint(e *checkpoint.Encoder) error {
	e.Field("effects.strips")
	if err := t.checkpointBoundary(); err != nil {
		e.Fail(err)
		return e.Err()
	}
	e.I64(int64(t.live))
	e.I64(int64(t.poolCapacity))
	e.I64(int64(t.steadyCap))
	for i := range t.strips {
		e.Field(fmt.Sprintf("effects.strips.lists[%d]", i))
		e.Count(len(t.strips[i]))
		for j := range t.strips[i] {
			o := &t.strips[i][j]
			path := fmt.Sprintf("effects.strips.lists[%d][%d]", i, j)
			e.Field(path)
			for _, v := range o.dst {
				e.I64(int64(v))
			}
			for _, v := range o.dstExtent {
				e.I64(int64(v))
			}
			e.U8(uint8(o.family))
			e.I32(o.frameCountBase)
			e.I32(o.frameDelayParam)
			e.U32(o.nextSpawn)
			e.I32(o.particleLife)
			e.Count(len(o.particles))
			for k := range o.particles {
				p := &o.particles[k]
				e.Field(fmt.Sprintf("%s.particles[%d]", path, k))
				// Particle fields are lexical, including the deferred draw and
				// its presence flag; these control future animation lifetime.
				e.U32(p.expiry)
				e.I32(p.frame)
				e.I32(p.frameDelay)
				e.I32(p.lastFrame)
				e.I32(p.lastFrameDraw)
				e.Bool(p.lastFrameDrawn)
				e.I32(p.phase)
				e.I64(int64(p.vx))
				e.I64(int64(p.vy))
				e.I64(int64(p.vz))
				e.I64(int64(p.x))
				e.I64(int64(p.y))
				e.I64(int64(p.z))
			}
			e.Field(path)
			e.I32(o.phaseModulus)
			e.U8(o.smokeSelector)
			e.I32(o.spawnInterval)
			for _, v := range o.src {
				e.I64(int64(v))
			}
			for _, v := range o.srcExtent {
				e.I64(int64(v))
			}
			e.U32(o.windowEnd)
		}
	}
	return e.Err()
}

func (t *stripTable) checkpointBoundary() error {
	if t == nil {
		return stripCheckpointError("effects.strips", "a present strip table")
	}
	if t.live < 0 || t.poolCapacity <= 0 || t.live > t.poolCapacity || t.steadyCap < 0 {
		return stripCheckpointError("effects.strips.capacity", "nonnegative live/steady counts and a positive pool containing live")
	}
	var count uint64
	for i := range t.strips {
		count += uint64(len(t.strips[i]))
		if uint64(len(t.strips[i])) > uint64(^uint32(0)) {
			return stripCheckpointError("effects.strips.lists", "a list length representable by u32")
		}
		for j := range t.strips[i] {
			o := &t.strips[i][j]
			if o.family < stripFamilyNano || o.family > stripFamilyFlameTrail {
				return stripCheckpointError(fmt.Sprintf("effects.strips.lists[%d][%d].family", i, j), "a reviewed strip family 1 through 6")
			}
			if uint64(len(o.particles)) > uint64(^uint32(0)) {
				return stripCheckpointError(fmt.Sprintf("effects.strips.lists[%d][%d].particles", i, j), "a list length representable by u32")
			}
		}
	}
	if count != uint64(t.live) {
		return stripCheckpointError("effects.strips.live", "the stored live count to equal the ten list lengths")
	}
	return nil
}

func stripCheckpointError(path, expected string) error {
	return fmt.Errorf("nanolathe: checkpoint owner effects: logical path %s, providers searched [], expected %s", path, expected)
}

// This direct walk appends only the cheap ring's selected words. Velocity,
// deferred frame draws and capacity are full-digest-only blind spots (.37).
func (t *stripTable) appendCheckpointSummary(s *checkpoint.Summary) error {
	if s == nil {
		return stripCheckpointError("effects.strips.summary", "a summary accumulator")
	}
	if t == nil {
		return stripCheckpointError("effects.strips", "a present strip table")
	}
	s.Word(uint64(t.live))
	for i := range t.strips {
		for j := range t.strips[i] {
			o := &t.strips[i][j]
			s.Word(uint64(o.family))
			s.Word(uint64(o.nextSpawn))
			s.Word(uint64(o.windowEnd))
			s.Word(uint64(len(o.particles)))
			for k := range o.particles {
				p := &o.particles[k]
				s.Word(uint64(int64(p.x)))
				s.Word(uint64(int64(p.y)))
				s.Word(uint64(int64(p.z)))
				s.Word(uint64(p.expiry))
				s.Word(uint64(int64(p.frame)))
				s.Word(uint64(int64(p.frameDelay)))
				s.Word(uint64(int64(p.phase)))
			}
		}
	}
	return nil
}
