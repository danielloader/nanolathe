package aikit

import (
	"github.com/nanolathe-gg/nanolathe/internal/ai"
	"github.com/nanolathe-gg/nanolathe/internal/sim/checkpoint"
	"github.com/nanolathe-gg/nanolathe/internal/units"
)

// checkpointCommand is per-command diagnostic bookkeeping. Existing return
// values, statistics and decisions never depend on these outcome flags.
type checkpointCommand struct {
	attempt                  *ai.ApplicationAttempt
	accepted, rejected, noop bool
}

// newCheckpointCommand runs only after the host has joined the applied batch.
// Capture never uses this adapter or reads the worker's command storage.
func (e *executor) newCheckpointCommand(c *Command, b *batch, tick uint32, serial uint64, ordinal uint32) *checkpointCommand {
	h := e.m.CheckpointApplicationHistory()
	if h == nil {
		return nil
	}
	a := h.BeginAttempt(tick, serial, ordinal, func(enc *checkpoint.Encoder) error {
		if e.m.Controller != ai.ControllerModern {
			return executorCheckpointError("aikit.application.controller", "a Modern computer")
		}
		intent, err := e.checkpointCommandIntent(c, b)
		if err != nil {
			return err
		}
		return intent.WriteCheckpoint(enc)
	})
	if a == nil {
		return nil
	}
	return &checkpointCommand{attempt: a}
}

func (e *executor) checkpointCommandIntent(c *Command, b *batch) (ai.ModernApplicationIntent, error) {
	if c == nil || b == nil {
		return ai.ModernApplicationIntent{}, executorCheckpointError("aikit.application.intent", "an applied command and batch")
	}
	v := ai.ModernApplicationIntent{Kind: uint8(c.Kind), Queued: c.Queued, RawTarget: uint32(c.Target), X: c.X, Z: c.Z,
		Slot: c.Slot, Count: c.Count, Spot: c.Spot, Spacing: c.Spacing, Keep: c.Keep, Exact: c.Exact, RowNear: b.rowNear}
	first, count := int64(c.first), int64(c.count)
	end := first + count
	if first < 0 || count < 0 || end > int64(len(b.actors)) || end > int64(len(b.inst)) {
		return v, executorCheckpointError("aikit.application.actors", "the command's observed actor span")
	}
	v.Operands.Actors = make([]checkpoint.Allocation, 0, int(count))
	for i := first; i < end; i++ {
		actor, err := ai.CheckpointAllocation(b.inst[i])
		if err != nil {
			return v, err
		}
		if actor.Handle != uint32(b.actors[i]) {
			return v, executorCheckpointError("aikit.application.actors", "matching raw and observed allocation handles")
		}
		v.Operands.Actors = append(v.Operands.Actors, actor)
	}
	if c.target != nil {
		target, err := ai.CheckpointAllocation(c.target)
		if err != nil {
			return v, err
		}
		if target.Handle != uint32(c.Target) {
			return v, executorCheckpointError("aikit.application.target", "matching raw and observed allocation handles")
		}
		v.Operands.Target = &target
	}
	if p := c.Product; p != nil {
		v.ProductIndex, v.ProductKey = p.Index, p.Key
		if e.table == nil || p.Index < 0 || int64(p.Index) >= int64(len(e.table.Units)) || e.table.Units[p.Index] != p || e.table.Of(p.Def) != p || e.table.Lookup(p.Key) != p {
			return v, executorCheckpointError("aikit.application.product", "the observed immutable table entry")
		}
		key, err := e.m.CheckpointApplicationKey(p.Def)
		if err != nil {
			return v, err
		}
		v.Operands.Product = &key
	}
	return v, nil
}

func (e *executor) checkpointAttempt() *ai.ApplicationAttempt {
	if e.checkpointApplication == nil {
		return nil
	}
	return e.checkpointApplication.attempt
}
func (e *executor) checkpointAccept() {
	if e.checkpointApplication != nil {
		e.checkpointApplication.accepted = true
	}
}
func (e *executor) checkpointReject() {
	if e.checkpointApplication != nil {
		e.checkpointApplication.rejected = true
	}
}
func (e *executor) checkpointNoop() {
	if e.checkpointApplication != nil {
		e.checkpointApplication.noop = true
	}
}

// checkpointActor must run before the existing call: retirement or reuse must
// not turn its operand into a lookup of a different allocation afterward.
func (e *executor) checkpointActor(u *units.Unit) checkpoint.Allocation {
	if e.checkpointAttempt() == nil {
		return checkpoint.Allocation{}
	}
	a, err := ai.CheckpointAllocation(u)
	e.m.CheckpointApplicationHistory().Fail(err)
	return a
}

func (e *executor) checkpointOrderOutcome(mark uint64, actor checkpoint.Allocation, method uint8) {
	a := e.checkpointAttempt()
	if a == nil {
		return
	}
	r, completed := a.InsertionAfter(mark, actor, method)
	if completed && (r.Inserted || r.Coalesced) {
		e.checkpointAccept()
	} else {
		e.checkpointReject()
	}
}

func (e *executor) checkpointBuildOutcome(mark uint64, actor checkpoint.Allocation, req ai.BuildRequest, err error) {
	a := e.checkpointAttempt()
	if a == nil {
		return
	}
	a.RecordBuild(actor, req, err)
	if err != nil {
		e.checkpointReject()
		return
	}
	e.checkpointOrderOutcome(mark, actor, 2)
}

func (c *checkpointCommand) finish(apm uint8) {
	if c == nil {
		return
	}
	terminal := uint8(3)
	if apm != 3 {
		committed := c.attempt.CommittedOperations() != 0
		switch {
		case !c.rejected && (c.accepted || c.noop && committed):
			terminal = 2
		case !c.rejected && c.noop:
			terminal = 1
		case committed:
			terminal = 4
		}
	}
	c.attempt.Finish(apm, terminal)
}
