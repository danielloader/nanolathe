package session

import (
	"github.com/nanolathe-gg/nanolathe/internal/movement"
	"github.com/nanolathe-gg/nanolathe/internal/orders"
	"github.com/nanolathe-gg/nanolathe/internal/sim/checkpoint"
)

// Work and presentation capture the factory's world adapter, and some movement
// methods capture the original System. Retain these actual construction edges;
// observing a later replacement's current fields cannot prove them (§16.3.59).
// This is diagnostic ownership metadata, excluded from the canonical payload.
type sessionCheckpointOrderBinding struct {
	owner          *Session
	authority      *checkpoint.BindingAuthority
	binding        *orders.QueueBinding
	movement       *orders.MovementGoalAdapter
	world          *orders.WorldQueryAdapter
	work           *orders.WorkAdapter
	weapons        *orders.WeaponAdapter
	presentation   *orders.PresentationAdapter
	movementSystem *movement.System
}

func (s *Session) prepareCheckpointOrderBinding(c *orders.CheckpointContext) error {
	if s == nil || c == nil || s.Build == nil || s.Econ == nil || s.Movement == nil {
		return runtimeCheckpointError("orders.binding", "the complete session order owners")
	}
	proof, b := s.checkpointOrders, s.Build.OrderBinding
	if proof.owner != s || !proof.authority.Matches(s.checkpointBindingAuthority()) || b == nil || proof.binding != b ||
		proof.movement != b.Movement || proof.world != b.World || proof.work != b.Work || proof.weapons != b.Weapons ||
		proof.presentation != b.Presentation || proof.movementSystem != s.Movement {
		return runtimeCheckpointError("orders.binding", "the original session factory and all captured adapter owners")
	}
	return c.SetBindings(b, s.Econ, s.SimRNG(), proof.authority)
}

// prepareCheckpointOrderHandlers registers both exact producer sources before
// queue discovery. Stage the three source kinds together so a conflicting
// movement source cannot partially register construction (§16.3.64).
func (s *Session) prepareCheckpointOrderHandlers(c *orders.CheckpointContext) error {
	if s == nil || c == nil || s.checkpointBindingAuthority() == nil || s.Build == nil || s.Movement == nil {
		return runtimeCheckpointError("orders.handlers", "the admitted session's construction and movement owners")
	}
	next := *c
	a := s.checkpointBindingAuthority()
	if err := s.Build.RegisterCheckpointOrderHandlers(&next, a); err != nil {
		return err
	}
	if err := s.Movement.RegisterCheckpointOrderHandlers(&next, a); err != nil {
		return err
	}
	*c = next
	return nil
}
