package frame

import (
	"fmt"

	"github.com/nanolathe-gg/nanolathe/internal/sim/checkpoint"
)

// WriteCheckpoint writes exhausted, limits (MaxEffectEvents, MaxEvents), nextID,
// nextSequence in source-field lexical order. The event window and its derived
// effect count must be empty; drop/overflow diagnostics and backing capacity
// are excluded (DESIGN_MULTIPLAYER §16.3.5–§16.3.6, §16.3.17).
func (b *EventBuffer) WriteCheckpoint(e *checkpoint.Encoder) error {
	e.Field("frame.EventBuffer")
	if b == nil {
		e.Fail(eventCheckpointError("frame.EventBuffer", "a present event buffer"))
		return e.Err()
	}
	if b.effects != 0 {
		e.Fail(eventCheckpointError("frame.EventBuffer.effects", "an empty completed event window"))
		return e.Err()
	}
	if len(b.events) != 0 {
		e.Fail(eventCheckpointError("frame.EventBuffer.events", "an empty completed event window"))
		return e.Err()
	}
	e.Field("frame.EventBuffer.exhausted")
	e.Bool(b.exhausted)
	e.Field("frame.EventBuffer.limits.MaxEffectEvents")
	e.I64(int64(b.limits.MaxEffectEvents))
	e.Field("frame.EventBuffer.limits.MaxEvents")
	e.I64(int64(b.limits.MaxEvents))
	e.Field("frame.EventBuffer.nextID")
	e.U32(b.nextID)
	e.Field("frame.EventBuffer.nextSequence")
	e.U64(b.nextSequence)
	return e.Err()
}

// AppendCheckpointSummary appends nextID, nextSequence, exhausted directly;
// session checks the completed-tick boundary (DESIGN_MULTIPLAYER §16.3.17).
func (b *EventBuffer) AppendCheckpointSummary(summary *checkpoint.Summary) error {
	if b == nil {
		return eventCheckpointError("frame.EventBuffer", "a present event buffer")
	}
	if summary == nil {
		return eventCheckpointError("frame.summary", "a summary accumulator")
	}
	summary.Word(uint64(b.nextID))
	summary.Word(b.nextSequence)
	var exhausted uint64
	if b.exhausted {
		exhausted = 1
	}
	summary.Word(exhausted)
	return nil
}

func eventCheckpointError(path, expected string) error {
	return fmt.Errorf("nanolathe: checkpoint capture failed: logical path %s, providers searched [frame], expected %s", path, expected)
}
