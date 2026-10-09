package ai

import (
	"github.com/nanolathe-gg/nanolathe/internal/sim/checkpoint"
	"github.com/nanolathe-gg/nanolathe/internal/units"
	"github.com/nanolathe-gg/nanolathe/internal/world"
)

// CheckpointContext shares the admitted definition and allocation identities.
// Computer owners add no graph table (DESIGN_MULTIPLAYER §16.3.19).
type CheckpointContext struct {
	Units    *units.CheckpointContext
	World    *world.CheckpointContext
	bindings *checkpointManagerBindings
	modern   *checkpointModernBindings
}

func NewCheckpointContext(u *units.CheckpointContext, w *world.CheckpointContext) *CheckpointContext {
	return &CheckpointContext{Units: u, World: w}
}

// ControllerCheckpoint contains simulation-thread values only. Capturing it
// must never join a worker or inspect an observation, batch, brain or private
// generator (DESIGN_MULTIPLAYER §16.3.7).
type ControllerCheckpoint struct {
	Present, Initialized              bool
	NextThinkPresent                  bool
	NextThinkTick                     uint32
	DeadlinePresent                   bool
	DeadlineTick                      uint32
	NextBatchSerial, ApplicationCount uint64
	ApplicationHash                   checkpoint.Digest
	Tokens                            int64
	LastFill                          uint32
}

// ControllerCheckpointProvider is the existing Manager.Ext boundary's
// diagnostic contract. Only reviewed production providers are admitted by
// session composition; implementing this interface alone is not admission.
type ControllerCheckpointProvider interface {
	ControllerCheckpoint() ControllerCheckpoint
	WriteControllerCheckpoint(*checkpoint.Encoder, *CheckpointContext) error
	AppendControllerCheckpointSummary(*checkpoint.Summary) error
}
