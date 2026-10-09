package session

import (
	"errors"
	"testing"

	"github.com/nanolathe-gg/nanolathe/internal/ai"
	"github.com/nanolathe-gg/nanolathe/internal/content"
	"github.com/nanolathe-gg/nanolathe/internal/orders"
	"github.com/nanolathe-gg/nanolathe/internal/sim/checkpoint"
)

type checkpointBuildObserver struct{ calls int }

func (o *checkpointBuildObserver) RecordCheckpointOrder(orders.CheckpointOrderReceipt) { o.calls++ }

// A factory's first queue and a nested mobile producer each emit receipts once,
// through the same existing bind. No observer remains installed afterward.
func TestCheckpointBuildObservesExistingLazyBinding(t *testing.T) {
	for _, stockpile := range []bool{false, true} {
		for _, nested := range []bool{false, true} {
			s, m, u, trace := checkpointBuildFixture(t)
			if err := m.EnableCheckpointApplications(checkpoint.Identity{}, &content.CheckpointKeys{}); err != nil {
				t.Fatal(err)
			}
			h := m.CheckpointApplicationHistory()
			a := h.BeginAttempt(77, h.NextSerial(), 0, func(e *checkpoint.Encoder) error { e.U8(2); return e.Err() })
			prior := &checkpointBuildObserver{}
			restore := func() {}
			if nested {
				q := orders.QueueForUnit(u)
				q.SetCheckpointObserver(prior)
				restore = a.ObserveQueue(q, u)
			} else if orders.QueueOfUnit(u) != nil {
				t.Fatal("fixture already had a queue")
			}
			product := "product"
			wantBinds, wantCommitted := 1, uint32(3)
			if stockpile {
				product = "alias"
				u.SlotAt(0).Weapon = &content.WeaponDef{Stockpile: true}
				wantBinds, wantCommitted = 2, 1
			}
			if err := m.QueueBuildTypedHook()(ai.BuildRequest{Builder: u.Handle, UnitKey: product, Count: 3, Kind: ai.BuildKindFactoryQueue, Tick: 77}); err != nil {
				t.Fatal(err)
			}
			if a.IssuedOrders() != 1 || a.CommittedOperations() != wantCommitted || trace.binds != wantBinds || prior.calls != 0 {
				t.Fatalf("stockpile=%v nested=%v: receipts=%d/%d binds=%d prior=%d", stockpile, nested, a.IssuedOrders(), a.CommittedOperations(), trace.binds, prior.calls)
			}
			restore()
			a.Finish(1, 2)
			state, err := h.Snapshot()
			if err != nil || state.Count != 1 {
				t.Fatalf("history %v, %v", state, err)
			}
			q := orders.QueueOfUnit(u)
			if q.Binding() != s.Build.OrderBinding {
				t.Fatal("binding changed")
			}
			q.PurgeUnprotected()
			if nested && prior.calls != 1 {
				t.Fatal("enclosing observer not restored")
			}
			if _, err := h.Snapshot(); err != nil {
				t.Fatal("observer remained installed", err)
			}
		}
	}
}

func TestCheckpointBuildObservationFailureDoesNotChangeProducer(t *testing.T) {
	for _, failed := range []bool{false, true} {
		_, m, u, trace := checkpointBuildFixture(t)
		if err := m.EnableCheckpointApplications(checkpoint.Identity{}, &content.CheckpointKeys{}); err != nil {
			t.Fatal(err)
		}
		h := m.CheckpointApplicationHistory()
		if failed {
			h.Fail(errors.New("authored diagnostic failure"))
		}
		a := h.BeginAttempt(7, h.NextSerial(), 0, func(e *checkpoint.Encoder) error { e.U8(2); return e.Err() })
		// This is refused after the existing lazy bind, with no queue operation.
		if err := m.QueueBuildTypedHook()(ai.BuildRequest{Builder: u.Handle, UnitKey: "missing", Count: 1, Kind: ai.BuildKindFactoryQueue}); err == nil {
			t.Fatal("unknown product admitted")
		}
		if a.CommittedOperations() != 0 || a.IssuedOrders() != 0 || trace.binds != 1 || trace.commands != 0 || orders.QueueOfUnit(u) == nil {
			t.Fatal("refusal changed producer effects")
		}
		a.Finish(1, 3)
		if err := m.QueueBuildTypedHook()(ai.BuildRequest{Builder: u.Handle, UnitKey: "product", Count: 2, Kind: ai.BuildKindFactoryQueue}); err != nil {
			t.Fatal(err)
		}
		if trace.binds != 2 || trace.commands != 1 || orders.QueueOfUnit(u).Head().Param2 != 2 {
			t.Fatal("failed diagnostics changed later work")
		}
	}
}
