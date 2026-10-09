package movement

import (
	"errors"

	"github.com/nanolathe-gg/nanolathe/internal/orders"
)

// BindMappingWordWithCheckpointBinding performs the ordinary allocation-order
// installation and carries the copied value beside every replaced reader.
// An absent reader keeps the ordinary nil no-op, including existing proof.
// No callback or stamp is invoked (DESIGN_MULTIPLAYER §16.3.66).
func (c *ClassLayers) BindMappingWordWithCheckpointBinding(value orders.CheckpointMappingWord) {
	c.bindMappingWord(value.Reader(), value)
}

// Presence must agree with the copied value; private installation and
// inheritance establish function identity without comparing function values.
// Reader returns a stored function only: capture never invokes that function.
func validateCheckpointMapping(c *CheckpointContext, actual MappingWordSource, value orders.CheckpointMappingWord) error {
	if (actual != nil) != (value.Reader() != nil) {
		return errors.New("mapping reader and copied installation presence differ")
	}
	if c == nil || c.Orders == nil {
		return errors.New("mapping reader needs the shared orders context")
	}
	return c.Orders.ValidateMappingWord(value)
}
