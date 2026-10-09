package triggers

import (
	"fmt"

	"github.com/nanolathe-gg/nanolathe/internal/sim/checkpoint"
)

// WriteCheckpoint writes stored trigger progress, not an authored replacement
// or a fresh evaluation ([08 R-TRIG-01 §2, §8]; DESIGN_MULTIPLAYER §16.3.38).
// Presence belongs to the caller. The payload's lexical fields are Args,
// Celebrated, CenterReady, CenterX, CenterY, CenterZ, Completed, Kind, Type.
func (t *Trigger) WriteCheckpoint(e *checkpoint.Encoder) error {
	e.Field("triggers.Trigger")
	if t == nil {
		e.Fail(fmt.Errorf("nanolathe: checkpoint capture failed: logical path triggers.Trigger, providers searched [triggers], expected a present trigger"))
		return e.Err()
	}
	for _, v := range t.Args {
		e.I32(v)
	}
	e.Bool(t.Celebrated)
	e.Bool(t.CenterReady)
	e.I32(t.CenterX)
	e.I32(t.CenterY)
	e.I32(t.CenterZ)
	e.Bool(t.Completed)
	e.U8(uint8(t.Kind))
	e.String(t.Type)
	return e.Err()
}
