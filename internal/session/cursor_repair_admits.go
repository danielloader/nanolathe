package session

import (
	"github.com/nanolathe-gg/nanolathe/internal/orders"
	"github.com/nanolathe-gg/nanolathe/internal/units"
)

// CursorRepairAdmits asks the shared repair admission used by the idle, MOVE
// and REPAIR cursor rows [07 §8][04 R-ORD-01 §7]. The supplied units are
// committed presentation copies. A private queue binds the immutable sea
// level without observing or changing an authoritative order queue [I6].
func (s *Session) CursorRepairAdmits(actor, target *units.Unit) bool {
	if s == nil || s.World == nil || actor == nil || actor.Def == nil {
		return false
	}
	probe := *actor
	probe.Orders = nil
	sea := s.World.SeaLevel
	orders.BindQueueBinding(&probe, &orders.QueueBinding{World: orders.NewWorldQueryAdapter(orders.WorldQueryAdapterConfig{
		SeaLevel: func() uint8 { return sea },
	})})
	return orders.Resolve(8, &probe, target, nil) != 0
}
