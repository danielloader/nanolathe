// ai_bind.go — production binding of the typed AI build request into the
// ordinary construction queues [RX-01][ON-06 F-P0-004][05 "Factory production
// lifecycle"]. Mobile sites carry the selected coordinates through to
// QueueMobileBuild; factory products queue on the factory path. The AI never
// receives privileged world mutation — this is the same command surface the
// human order path reaches.

package session

import (
	"errors"
	"fmt"
	"strings"

	"github.com/nanolathe-gg/nanolathe/internal/ai"
	"github.com/nanolathe-gg/nanolathe/internal/combat"
	"github.com/nanolathe-gg/nanolathe/internal/construction"
	"github.com/nanolathe-gg/nanolathe/internal/orders"
	"github.com/nanolathe-gg/nanolathe/internal/units"
)

func bindAIQueue(mgr *ai.Manager, s *Session) {
	if mgr == nil || s == nil {
		return
	}
	if s.Build != nil {
		// Ordinary AI move/order producers use the same concrete context as the
		// typed construction sink. The field is assigned before the manager can
		// dispatch in phase 5 [04 §3.3][06 §11.1].
		mgr.OrderBinding = s.Build.OrderBinding
	}
	mgr.SetQueueBuildTypedWithCheckpointBinding(func(req ai.BuildRequest) error {
		if s.Units == nil || s.Catalog == nil {
			return ai.WithCheckpointBuildVerdict(fmt.Errorf("ai build: session units/catalog unavailable"), ai.CheckpointBuildBinding)
		}
		builder := s.Units.Unit(req.Builder)
		if builder == nil || !builder.Alive {
			return ai.WithCheckpointBuildVerdict(fmt.Errorf("ai build: builder handle %d not alive", req.Builder), ai.CheckpointBuildOwner)
		}
		// AI may issue its first build before this unit has ever needed a
		// queue. Bind the lazy queue through the session-owned context before
		// construction performs admission [04 §3.3][05][06 §11.1].
		s.bindOrderQueue(builder)
		// Observe only after the existing lazy bind; a first factory queue
		// must not be created early just for diagnostics (§16.3.26–27).
		restore := mgr.CheckpointApplicationHistory().ActiveAttempt().ObserveQueue(orders.QueueOfUnit(builder), builder)
		defer restore()
		switch req.Kind {
		case ai.BuildKindFactoryQueue:
			if def, ok := s.Catalog.Unit(req.UnitKey); ok && def != nil && stockpileAliasName(def.UnitName) {
				// The ordinary submission helper routes a MAKENUKE/MAKEANTI
				// product to a counted BUILDWEAPON round in slot zero, not to
				// a unit order [07 R-P0-11 §1].
				// ProTA 4.8 authors those products as pseudo-unit definitions
				// in its stockpile producers' CANBUILD lists, which is how the
				// computer player's queue task reaches this arm
				// (research/extensions/prota-engine.md "authored stationary
				// stockpile producers").
				if verdict := s.queueStockpileRoundsResult(builder, req.Count, req.Tick); verdict != ai.CheckpointBuildSuccess {
					return ai.WithCheckpointBuildVerdict(fmt.Errorf("ai build: stockpile round refused for builder %d", req.Builder), verdict)
				}
				return nil
			}
			return classifyCheckpointBuildError(construction.QueueFactoryBuild(builder, req.UnitKey, req.Count, s.Catalog))
		case ai.BuildKindMobileSite:
			return classifyCheckpointBuildError(construction.QueueMobileBuild(builder, req.UnitKey, req.X, req.Z, req.Count, s.Catalog))
		default:
			return classifyCheckpointBuildError(construction.QueueFactoryBuild(builder, req.UnitKey, req.Count, s.Catalog))
		}
	}, s.checkpointBindingAuthority())
}

// classifyCheckpointBuildError adds only the diagnostic return classification
// of DESIGN_MULTIPLAYER §16.3.25. The producer has already made every admission
// decision; unknown errors keep their identity and remain unclassified.
func classifyCheckpointBuildError(err error) error {
	var verdict uint8
	switch {
	case err == nil:
		return nil
	case errors.Is(err, construction.ErrNilFactory):
		verdict = ai.CheckpointBuildOwner
	case errors.Is(err, construction.ErrEmptyDef), errors.Is(err, construction.ErrUnknownProduct), errors.Is(err, construction.ErrMissingMovementProfile):
		verdict = ai.CheckpointBuildProduct
	case errors.Is(err, construction.ErrNoQueue), errors.Is(err, construction.ErrNoBuildOrder):
		verdict = ai.CheckpointBuildBinding
	case errors.Is(err, construction.ErrLimit):
		verdict = ai.CheckpointBuildLimit
	case errors.Is(err, construction.ErrBadCount):
		verdict = ai.CheckpointBuildOther
	default:
		return err
	}
	return ai.WithCheckpointBuildVerdict(err, verdict)
}

// aiCanPursueAir is the Modern wave air targets predicate the computer
// player's ModernPlanner asks (DESIGN_SESSIONS_AI_SAVE "Modern wave air
// targets"): some weapon of member could engage the airborne target.
func (s *Session) aiCanPursueAir(member, target *units.Unit) bool {
	return combat.ModernAirPursuitAdmits(member, target, s.World, s.Catalog, s.Combat)
}

// stockpileAliasName reports whether a product name is one of the two
// stockpile aliases the ordinary submission helper routes to BUILDWEAPON: the
// name contains `MAKENUKE` or `MAKEANTI`, compared case-sensitively
// [07 R-P0-11 §1]. Stock build pages name
// the toys ARMMAKEANTI, EMPMAKENUKE and the like; ProTA 4.8 names its
// pseudo-products MAKENUKEARM, MAKEANTICOR and the like.
func stockpileAliasName(name string) bool {
	return strings.Contains(name, "MAKENUKE") || strings.Contains(name, "MAKEANTI")
}

// queueStockpileRounds is the counted BUILDWEAPON insertion shared by the
// build-page toy and the computer player's queue task: a positive count of
// rounds for weapon slot zero, coalesced into the rear segment's tail record
// [07 R-P0-11 §1][06 §11.1]. It refuses a
// slot whose weapon is not a stockpile weapon (orders.StockpileSlotAcceptsBuildWeapon).
func (s *Session) queueStockpileRounds(u *units.Unit, count int, tick uint32) bool {
	return s.queueStockpileRoundsResult(u, count, tick) == ai.CheckpointBuildSuccess
}

// queueStockpileRoundsResult labels the existing exits in their original
// evaluation order, without another slot predicate or queue bind. Success is
// the producer's return, not an insertion receipt (DESIGN_MULTIPLAYER §16.3.25).
func (s *Session) queueStockpileRoundsResult(u *units.Unit, count int, tick uint32) uint8 {
	id := orders.Lookup("BuildWeapon")
	switch {
	case u == nil:
		return ai.CheckpointBuildOwner
	case id == 0:
		return ai.CheckpointBuildBinding
	case count <= 0:
		return ai.CheckpointBuildOther
	}
	// The UI alias path always supplies zero, which is where shipped
	// stockpile weapons live [06 §11.1][06 R-WPN-05 §2].
	const stockpileAliasSlot = 0
	if !orders.StockpileSlotAcceptsBuildWeapon(u, stockpileAliasSlot) {
		return ai.CheckpointBuildProduct
	}
	s.bindOrderQueue(u)
	// The queued/non-queued argument is NOT the click's Shift bit: the
	// world-order shift chain does not participate on the counted path
	// [07 R-P0-11 §1], and this producer issues no Replace, so it never
	// purges. The argument is inert for a rear-segment record in any case
	// — the caption clear is never called for BUILDWEAPON [04 R-ORD-01 §1].
	n := orders.NewNodeForOrder(id, 0, 0, 0, 0, tick, u.Handle, false)
	n.Param1, n.Param2 = uint32(stockpileAliasSlot), uint32(count)
	q := orders.QueueForUnit(u)
	if q == nil {
		return ai.CheckpointBuildBinding
	}
	q.CoalesceTail(id, n)
	return ai.CheckpointBuildSuccess
}
