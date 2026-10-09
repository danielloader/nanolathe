package session

import (
	"github.com/nanolathe-gg/nanolathe/internal/movement"
	"github.com/nanolathe-gg/nanolathe/internal/world"
)

// prepareCheckpointMovement verifies the actual path, terrain and unit-world
// singleton edges before lower owners collect mutable references (§16.3.61, §16.3.69).
func (s *Session) prepareCheckpointMovement(c *movement.CheckpointContext, w *world.CheckpointContext) error {
	if s == nil || c == nil || c.Orders == nil || c.Orders.Units == nil || c.Orders.Units.Keys == nil || w == nil || s.checkpointBindingAuthority() == nil ||
		s.Movement == nil || s.Units == nil || s.World == nil || s.Movement.Terrain != s.World ||
		s.Path == nil || s.Path != s.Movement.Scheduler {
		return runtimeCheckpointError("movement.bindings", "the admitted session's exact movement, terrain, scheduler and unit owners")
	}
	a := s.checkpointBindingAuthority()
	if err := c.SetCompositionBindings(s.Movement, c.Orders.Units.Keys, a); err != nil {
		return err
	}
	if err := c.SetTerrainBindings(s.Movement, w, s.Movement.Grid, a); err != nil {
		return err
	}
	if err := c.SetPathBindings(s.Movement, s.Units, a); err != nil {
		return err
	}
	return c.SetAuxiliaryBindings(s.Movement, s.Units, a)
}
