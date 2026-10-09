package ai

import (
	"encoding/binary"
	"github.com/nanolathe-gg/nanolathe/internal/sim/checkpoint"
)

// WriteCheckpoint writes the detached controller record in lexical field
// order, using fixed bytes for its digest (DESIGN_MULTIPLAYER §16.3.24).
func (v ControllerCheckpoint) WriteCheckpoint(e *checkpoint.Encoder) error {
	if e == nil {
		return aiCheckpointError("ai.controller.encoder", "a checkpoint encoder")
	}
	e.Field("ai.controller")
	e.U64(v.ApplicationCount)
	for _, b := range v.ApplicationHash {
		e.U8(b)
	}
	e.Bool(v.DeadlinePresent)
	e.U32(v.DeadlineTick)
	e.Bool(v.Initialized)
	e.U32(v.LastFill)
	e.U64(v.NextBatchSerial)
	e.Bool(v.NextThinkPresent)
	e.U32(v.NextThinkTick)
	e.Bool(v.Present)
	e.I64(v.Tokens)
	return e.Err()
}

// AppendCheckpointSummary writes the selected 11 words after the manager's
// fragment. Its provider must first validate the history/application boundary.
func (v ControllerCheckpoint) AppendCheckpointSummary(s *checkpoint.Summary) error {
	if s == nil {
		return aiCheckpointError("ai.controller.summary", "a summary accumulator")
	}
	next := *s
	next.Word(v.ApplicationCount)
	for i := 0; i < len(v.ApplicationHash); i += 8 {
		next.Word(binary.LittleEndian.Uint64(v.ApplicationHash[i : i+8]))
	}
	var think, deadline uint64
	if v.NextThinkPresent {
		think = 1
	}
	if v.DeadlinePresent {
		deadline = 1
	}
	next.Word(think)
	next.Word(uint64(v.NextThinkTick))
	next.Word(deadline)
	next.Word(uint64(v.DeadlineTick))
	next.Word(uint64(v.Tokens))
	next.Word(uint64(v.LastFill))
	*s = next
	return nil
}
