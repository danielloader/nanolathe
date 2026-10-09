package world

import (
	"fmt"
	"math"

	"github.com/nanolathe-gg/nanolathe/internal/sim/checkpoint"
)

// WriteCheckpoint preserves the wind's stored fields, lexically: Changed,
// DirX, DirZ, Heading, Max, Min, NextChange, Scalar, Strength. LastChange and
// BriefingCountdown have no battle-side consumer and remain excluded
// (DESIGN_MULTIPLAYER §16.3.5; [01 §7.3]). No redraw or normalization occurs.
func (w *Wind) WriteCheckpoint(e *checkpoint.Encoder) error {
	e.Field("world.Wind")
	if w == nil {
		e.Fail(fmt.Errorf("missing wind"))
		return e.Err()
	}
	e.Field("world.Wind.Changed")
	e.Bool(w.Changed)
	e.Field("world.Wind.DirX")
	e.I32(w.DirX)
	e.Field("world.Wind.DirZ")
	e.I32(w.DirZ)
	e.Field("world.Wind.Heading")
	e.U16(w.Heading)
	e.Field("world.Wind.Max")
	e.I32(w.Max)
	e.Field("world.Wind.Min")
	e.I32(w.Min)
	e.Field("world.Wind.NextChange")
	e.U32(w.NextChange)
	e.Field("world.Wind.Scalar")
	e.F32(w.Scalar)
	e.Field("world.Wind.Strength")
	e.I32(w.Strength)
	return e.Err()
}

// AppendCheckpointSummary appends strength, heading, scalar bits and next-change
// to the runtime owner (DESIGN_MULTIPLAYER §16.3.7). Unselected fields remain
// full-digest-only; a selected NaN fails without partially appending words.
func (w *Wind) AppendCheckpointSummary(s *checkpoint.Summary) error {
	if w == nil || s == nil {
		return worldCheckpointError("world.Wind.summary", "a present wind and summary")
	}
	bits := math.Float32bits(w.Scalar)
	if bits&0x7f800000 == 0x7f800000 && bits&0x007fffff != 0 {
		return worldCheckpointError("world.Wind.Scalar", "non-NaN binary32")
	}
	s.Word(uint64(int64(w.Strength)))
	s.Word(uint64(w.Heading))
	s.Word(uint64(bits))
	s.Word(uint64(w.NextChange))
	return nil
}
