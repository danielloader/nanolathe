package movement

import (
	"errors"

	"github.com/nanolathe-gg/nanolathe/internal/orders"
	"github.com/nanolathe-gg/nanolathe/internal/sim/checkpoint"
)

// Constructor aliases are private diagnostic metadata, never payload. The
// cached function and its immutable copied value are installed together only
// at RegisterOrderHandlers' existing lazy creation site. No other writer can
// replace the private function, so capture needs no function comparison or
// invocation (DESIGN_MULTIPLAYER §16.3.62 and §16.3.64).
type checkpointOrderHandlerSource struct {
	system    *System
	authority *checkpoint.BindingAuthority
	source    *orders.CheckpointHandlerSource
}

func (s *System) hasCheckpointOrderHandlerSource() bool {
	return s != nil && s.checkpointOrderHandlers != nil && s.checkpointOrderHandlers.system == s &&
		s.checkpointOrderHandlers.authority != nil && s.checkpointOrderHandlers.source != nil
}

// RegisterCheckpointOrderHandlers registers the constructor's original source
// against its exact owner. It neither creates the lazy handler nor installs a
// queue row. Orders validates copied rows using this source; movement validates
// its own cache without changing the lower context during capture (§16.3.64).
func (s *System) RegisterCheckpointOrderHandlers(c *orders.CheckpointContext, authority *checkpoint.BindingAuthority) error {
	if !s.hasCheckpointOrderHandlerSource() || !s.checkpointOrderHandlers.authority.Matches(authority) {
		return movementCheckpointError("movement.airLegHandler", errors.New("unattested movement handler constructor owner or authority"))
	}
	if err := s.validateCheckpointOrderHandlers(); err != nil {
		return movementCheckpointError("movement.airLegHandler", err)
	}
	return orders.RegisterCheckpointHandlerSource(c, orders.CheckpointAirStandby, s.checkpointOrderHandlers.source, s, authority)
}

func (s *System) validateCheckpointOrderHandlers() error {
	if s.checkpointOrderHandlers != nil && !s.hasCheckpointOrderHandlerSource() {
		return errors.New("unattested movement handler constructor owner")
	}
	if s.airLegHandler == nil {
		if s.checkpointAirLegHandler.Handler() != nil {
			return errors.New("retained movement handler proof without cached function")
		}
		return nil
	}
	if !s.hasCheckpointOrderHandlerSource() || s.checkpointAirLegHandler.Handler() == nil {
		return errors.New("unattested movement airLegHandler; TODO(M3-U6) for other handlers")
	}
	return nil
}
