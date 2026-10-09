package movement

import (
	"errors"

	"github.com/nanolathe-gg/nanolathe/internal/sim/checkpoint"
	"github.com/nanolathe-gg/nanolathe/internal/world"
)

// The world context value preserves the original opaque installation receipt.
// It is capture-local metadata, not another graph owner or wire field (§16.3.57).
type checkpointTerrainBindings struct {
	system       *System
	context      *world.CheckpointContext
	terrain      *world.Terrain
	grid         *OccupancyGrid
	authority    *checkpoint.BindingAuthority
	registration world.CheckpointContext
}

// SetTerrainBindings validates the actual captured restamp owner and occupancy
// adapter before registering either context. World handles its opaque receipt;
// movement alone narrows the interfaces, without invoking them (§16.3.57).
func (c *CheckpointContext) SetTerrainBindings(s *System, w *world.CheckpointContext, grid *OccupancyGrid, authority *checkpoint.BindingAuthority) error {
	if c == nil || s == nil || w == nil || authority == nil {
		return movementCheckpointError("movement.terrainBindings", errors.New("missing context, system, world context or authority"))
	}
	if c.system != nil && c.system != s {
		return movementCheckpointError("movement.terrainBindings", errors.New("context belongs to another system"))
	}
	if c.compositionBindings.authority != nil && !c.compositionBindings.authority.Matches(authority) {
		return movementCheckpointError("movement.terrainBindings", errors.New("composition authority differs"))
	}
	binding := checkpointTerrainBindings{system: s, context: w, terrain: s.Terrain, grid: grid, authority: authority}
	if err := binding.validateOwners(s); err != nil {
		return movementCheckpointError("movement.terrainBindings", err)
	}
	// Stage the world registration on a local value: even a later movement
	// conflict must leave the caller's world context completely untouched.
	binding.registration = *w
	if err := binding.registration.SetMovementBindings(binding.terrain, authority); err != nil {
		return movementCheckpointError("movement.terrainBindings", err)
	}
	if c.terrainBindings.authority != nil && c.terrainBindings != binding {
		return movementCheckpointError("movement.terrainBindings", errors.New("conflicting terrain binding registration"))
	}
	*w = binding.registration
	c.system, c.terrainBindings = s, binding
	return nil
}

func (b checkpointTerrainBindings) validateOwners(s *System) error {
	if s == nil || b.system != s || b.terrain == nil || b.authority == nil || s.Terrain != b.terrain || s.Grid != b.grid {
		return errors.New("terrain binding owner alias differs")
	}
	owner, admitted := b.terrain.CheckpointMovementOwner(b.authority)
	actual, ok := owner.(*System)
	if !admitted || !ok || actual == nil || actual != s {
		return errors.New("terrain restamp owner is not the exact movement system")
	}
	movers, ok := b.terrain.Movers().(gridOccupancy)
	if !ok || movers.grid != b.grid {
		return errors.New("terrain mover adapter is not the registered occupancy grid")
	}
	if b.grid != nil && b.grid.plot != b.terrain {
		return errors.New("occupancy grid terrain alias differs")
	}
	return nil
}

func (b checkpointTerrainBindings) validate(s *System) error {
	if err := b.validateOwners(s); err != nil {
		return err
	}
	if b.context == nil || *b.context != b.registration {
		return errors.New("registered world context changed")
	}
	// Recheck the original receipt using a detached value. Calling the owner
	// registration on the live context would make capture perform a write.
	registration := b.registration
	return registration.SetMovementBindings(b.terrain, b.authority)
}
