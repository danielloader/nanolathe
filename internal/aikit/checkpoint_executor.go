package aikit

import (
	"fmt"

	"github.com/nanolathe-gg/nanolathe/internal/ai"
	"github.com/nanolathe-gg/nanolathe/internal/sim/checkpoint"
)

// writeCheckpoint writes the private simulation-thread executor record in
// source-lexical order (DESIGN_MULTIPLAYER §16.3.20). The host owns completed-
// application admission. No worker pointer, observation, batch or producer is
// read here; this leaf alone does not admit a live Modern controller.
func (e *executor) writeCheckpoint(enc *checkpoint.Encoder, c *ai.CheckpointContext) error {
	const path = "aikit.executor"
	enc.Field(path)
	kind, err := e.checkpointBindingKind(c)
	if err != nil {
		enc.Fail(err)
		return enc.Err()
	}
	return e.writeCheckpointValues(enc, kind)
}

// Bindings are already validated before any owner payload. A present table
// carries its original construction-rule tag, even after a mode switch (§75).
func (e *executor) writeCheckpointValues(enc *checkpoint.Encoder, tableKind uint8) error {
	const path = "aikit.executor"
	if err := e.dedupe.writeCheckpoint(enc, path+".dedupe"); err != nil {
		return err
	}
	enc.Field(path + ".freeSeq")
	enc.U32(e.freeSeq)
	enc.Field(path + ".freeSlot")
	enc.I64(int64(e.freeSlot))
	for i := range e.frees {
		if err := e.frees[i].writeCheckpoint(enc, fmt.Sprintf("%s.frees[%d]", path, i)); err != nil {
			return err
		}
	}
	for i := range e.grids {
		if err := e.grids[i].writeGuardCheckpoint(enc, fmt.Sprintf("%s.grids[%d]", path, i)); err != nil {
			return err
		}
	}
	enc.Field(path + ".lastFill")
	enc.U32(e.lastFill)
	enc.Field(path + ".lastTick")
	enc.U32(e.lastTick)
	enc.Field(path + ".m")
	enc.Bool(e.m != nil)
	enc.Field(path + ".nextPending")
	enc.I64(int64(e.nextPending))
	for i := range e.pending {
		if err := e.pending[i].writeCheckpoint(enc, fmt.Sprintf("%s.pending[%d]", path, i)); err != nil {
			return err
		}
	}
	if err := e.selfGrid.writeSelfCheckpoint(enc, path+".selfGrid"); err != nil {
		return err
	}
	writeExecutorCheckpointBools(enc, path+".spotCover", e.spotCover)
	enc.Field(path + ".table")
	enc.Bool(e.table != nil)
	if e.table != nil {
		enc.U8(tableKind)
	}
	enc.Field(path + ".tokens")
	enc.I64(e.tokens)
	return enc.Err()
}

// writeCheckpoint preserves the effective stored persona; Name is a label and
// Async is scheduling. Normalization belongs to construction, never capture
// (DESIGN_MULTIPLAYER §16.3.20).
func (p *Persona) writeCheckpoint(enc *checkpoint.Encoder) error {
	const path = "aikit.Persona"
	enc.Field(path)
	if p == nil {
		enc.Fail(executorCheckpointError(path, "a present persona"))
		return enc.Err()
	}
	enc.Field(path + ".APM")
	enc.I32(p.APM)
	enc.Field(path + ".Ambition")
	enc.I32(p.Ambition)
	enc.Field(path + ".Attention")
	enc.I32(p.Attention)
	enc.Field(path + ".Burst")
	enc.I32(p.Burst)
	enc.Field(path + ".Omniscient")
	enc.Bool(p.Omniscient)
	enc.Field(path + ".Reaction")
	enc.U32(p.Reaction)
	enc.Field(path + ".Skill")
	enc.I32(p.Skill)
	enc.Field(path + ".ThinkEvery")
	enc.U32(p.ThinkEvery)
	return enc.Err()
}

func executorCheckpointError(path, expected string) error {
	return fmt.Errorf("nanolathe: executor checkpoint failed: logical path %s, providers searched [], expected %s", path, expected)
}
