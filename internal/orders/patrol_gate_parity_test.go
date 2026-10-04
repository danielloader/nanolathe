package orders

import (
	"testing"

	"github.com/nanolathe-gg/nanolathe/internal/sim/rng"
	"github.com/nanolathe-gg/nanolathe/internal/units"
)

// An accepted assist leaves the leg armed, unlike the explicit unfinished-air
// and reclaim branches [04 R-ORD-01 §4][04 R-ORD-01 §7].
func TestPatrolAcceptedAssistanceKeepsMovementGate(t *testing.T) {
	for _, air := range []bool{false, true} {
		actor, _, sim, q := repairPatrolRefusalFixture(t, air)
		actor.Flags = actor.Flags&^(stanceFieldMask<<stanceMoveShift) | 2<<stanceMoveShift
		id, handler, delay := Lookup("RepairPatrol"), repairPatrolHandler, int32(60)
		if air {
			id, handler, delay = Lookup("VTOL_RepairPatrol"), vtolRepairPatrolHandler, 45
		}
		n := &Node{ID: id, Owner: actor.Handle, Phase: 1, Deadline: -1}
		if code := handler(actor, n, 0, 100); code != 6 {
			t.Fatalf("air=%v: accepted assistance returned %d, want rotate", air, code)
		}
		if n.DynamicGate != gateMoveOutcomes|gateDeadline || n.Deadline != 100+delay {
			t.Fatalf("air=%v: gate/deadline %#x/%d, want movement plus deadline/%d", air, n.DynamicGate, n.Deadline, 100+delay)
		}
		if draws := sim.Draws(); draws != 1 {
			t.Fatalf("air=%v: accepted two-candidate pick spent %d draws, want one", air, draws)
		}
		if q.Head() == nil || q.Head().Target == 0 {
			t.Fatalf("air=%v: accepted assistance has no target", air)
		}
	}
}

// The primary wait and last-record rearm OR the deadline gate into whatever
// their handler left armed. Replacing the gate hides movement wakes [04 §3.3].
func TestPrimaryDelayResultsPreserveHandlerGates(t *testing.T) {
	for _, code := range []Code{3, 9} {
		sim := rng.NewSimulation(1)
		q := &Queue{binding: &QueueBinding{SimRNG: &sim}}
		n := q.PushHead(Lookup("Move_Ground"), Node{Phase: 4, DynamicGate: gateMoveOutcomes})
		n.DynamicGate = gateMoveOutcomes
		if !q.applyPrimaryResultCode(n, code, 100) || n.DynamicGate != gateMoveOutcomes|gateDeadline {
			t.Fatalf("code %d: gate %#x, want movement and deadline", code, n.DynamicGate)
		}
		limit := int32(145)
		if code == 9 {
			limit = 160
			if n.Phase != 0 || n.Flags&FlagRetryMark == 0 {
				t.Fatal("last-record rearm lost its reset phase or completion flag")
			}
		}
		if n.Deadline < 130 || n.Deadline >= limit || sim.Draws() != 1 {
			t.Fatalf("code %d: deadline/draws %d/%d differ from the ordinary delay arm", code, n.Deadline, sim.Draws())
		}
	}
}

// A death-pending member remains eligible while it remains in the ordinary
// sector population; finalization owns unlinking [04 R-ORD-02 §4].
func TestRepairCollectorDoesNotExcludeDeathPendingMember(t *testing.T) {
	actor, targets, _, _ := repairPatrolRefusalFixture(t, false)
	targets[0].Flags |= units.DeathPendingStatus
	targets[0].Dying = true
	if got := scanRepairCandidates(actor, actor.Def.SightDistance); len(got) != len(targets) {
		t.Fatal("death-pending member was removed before sector finalization")
	}
}
