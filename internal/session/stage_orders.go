package session

import (
	"github.com/nanolathe-gg/nanolathe/internal/orders"
	"github.com/nanolathe-gg/nanolathe/internal/pool"
	"github.com/nanolathe-gg/nanolathe/internal/sim/numeric"
	"github.com/nanolathe-gg/nanolathe/internal/units"
	"github.com/nanolathe-gg/nanolathe/internal/visibility"
)

// BindStagedOrderQueue gives a directly staged unit the session-owned order
// queue a command would have bound for it. A capture fixture creates units
// outside the command boundary, where a fresh unit has no queue at all, so an
// order pushed at staging time would otherwise be dropped without a trace.
func (s *Session) BindStagedOrderQueue(u *units.Unit) { s.bindOrderQueue(u) }

// RevealStagedMap lifts the viewing player's fog at once, with the `+nowisee`
// command's own refresh, for a capture that publishes an opening frame before
// any tick could apply that command. It changes no gameplay rule.
func (s *Session) RevealStagedMap() {
	if s == nil || s.Vis == nil {
		return
	}
	mode := s.Vis.Mode() &^ (visibility.ModeHistoryEnabled | visibility.ModeCurrentEnabled)
	eligible, observers := visibilityModeRefreshInputs(s, mode)
	s.Vis.RefreshMode(mode, true, eligible, observers)
}

// StageGroupMove issues one ordinary group move to units of owner exactly as
// the command boundary issues a player's: the selection's centre, each
// actor's formation goal [04 R-STANCE-01 §5] and, under a rule set that asks
// for them, its destination slot. A replay of a recorded game stages other
// players' group orders through it; the command boundary itself admits only
// the local player's units. It draws from neither random stream.
//
// count is the size of the recorded selection when it is known to have been
// larger than handles; zero counts handles.
func (s *Session) StageGroupMove(owner uint8, handles []pool.Handle, x, z numeric.Fixed, count int32) {
	if s == nil || s.World == nil || len(handles) == 0 {
		return
	}
	local := s.LocalOwner
	s.LocalOwner = owner
	s.applyHumanCommand(HumanCommand{Kind: HumanOrder, Order: HumanOrderCommand{
		Handles:     handles,
		StagedCount: count,
		Code:        2,
		Position:    orders.ResolvePos{X: x, Y: s.World.HeightAt(x, z), Z: z, InterfaceType: orders.InterfaceTypeRightClick},
	}}, s.Clock.GlobalTick+1)
	s.LocalOwner = local
}
