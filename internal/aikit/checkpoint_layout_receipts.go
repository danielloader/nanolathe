package aikit

import (
	"github.com/nanolathe-gg/nanolathe/internal/ai"
	"github.com/nanolathe-gg/nanolathe/internal/sim/checkpoint"
)

// Layout observation belongs only to a command's simulation-thread scope.
// Each guard slot keeps its physical index; selfGrid is never installed here
// (DESIGN_MULTIPLAYER §16.3.31).
type checkpointGridObservation struct {
	attempt *ai.ApplicationAttempt
	slot    int64
}

func noCheckpointLayoutObservation() {}

func (e *executor) observeCheckpointLayout(a *ai.ApplicationAttempt) func() {
	if e == nil || a == nil {
		return noCheckpointLayoutObservation
	}
	var previous [4]checkpointGridObservation
	for i := range e.grids {
		previous[i] = e.grids[i].checkpointObservation
		e.grids[i].checkpointObservation = checkpointGridObservation{attempt: a, slot: int64(i)}
	}
	return func() {
		for i := range e.grids {
			e.grids[i].checkpointObservation = previous[i]
		}
	}
}

// A write is captured immediately after its assignment, including equal writes
// and stamp wrap's two assignments. The integer carrier holds every source i32,
// u32 and length without narrowing until the field's reviewed wire encoding.
type checkpointGridWrite struct {
	field, action uint8
	index, value  int64
	boolean       bool
}

// Callers guard each observed store directly, so disabled placement never
// calls an encoding helper in its per-cell loops.
func (o checkpointGridObservation) record(w checkpointGridWrite) {
	o.attempt.Operation(8, func(enc *checkpoint.Encoder) error {
		return w.writeCheckpoint(enc, o.slot)
	})
}

func (w checkpointGridWrite) writeCheckpoint(enc *checkpoint.Encoder, slot int64) error {
	const path = "aikit.layout.grid"
	enc.Field(path)
	valid := false
	switch w.action {
	case 1:
		valid = w.field == 1 || w.field == 3 || w.field == 4 || w.field == 5 || w.field == 7 || w.field == 8 || w.field == 11
	case 2, 3:
		valid = w.field == 2 || w.field == 6 || w.field == 10
	case 4, 5:
		valid = w.field == 9
	}
	if slot < 0 || slot >= 4 || !valid || ((w.action == 3 || w.action == 5) && w.index < 0) {
		enc.Fail(executorCheckpointError(path, "a guard slot and published field/action operands"))
		return enc.Err()
	}
	enc.I64(slot)
	enc.U8(w.field)
	enc.U8(w.action)
	switch w.action {
	case 1:
		switch w.field {
		case 4, 5:
			enc.I32(int32(w.value))
		case 8:
			enc.Bool(w.boolean)
		default:
			enc.U32(uint32(w.value))
		}
	case 2:
		if w.field == 2 {
			enc.Bool(w.boolean)
		}
		enc.Count(int(w.value))
	case 3:
		enc.I64(w.index)
		if w.field == 2 {
			enc.I32(int32(w.value))
		} else {
			enc.U32(uint32(w.value))
		}
	case 4:
		enc.Count(int(w.value))
	case 5:
		enc.I64(w.index)
		enc.I32(int32(w.value))
	}
	return enc.Err()
}

func (e *executor) recordCheckpointPending(slot int) {
	a := e.m.CheckpointApplicationHistory().ActiveAttempt()
	if a == nil {
		return
	}
	p, next := e.pending[slot], e.nextPending
	a.Operation(7, func(enc *checkpoint.Encoder) error {
		enc.Field("aikit.layout.pending")
		enc.I64(int64(slot))
		if err := p.writeCheckpoint(enc, "aikit.layout.pending.row"); err != nil {
			return err
		}
		enc.Field("aikit.layout.nextPending")
		enc.I64(int64(next))
		return enc.Err()
	})
}
