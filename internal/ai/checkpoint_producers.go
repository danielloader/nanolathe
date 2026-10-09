package ai

import (
	"math"

	"github.com/nanolathe-gg/nanolathe/internal/orders"
	"github.com/nanolathe-gg/nanolathe/internal/sim/checkpoint"
	"github.com/nanolathe-gg/nanolathe/internal/units"
)

// ActiveAttempt is only a simulation-thread diagnostic scope. Session uses it
// after its existing lazy queue binding, never to decide whether to bind a queue.
func (h *ApplicationHistory) ActiveAttempt() *ApplicationAttempt {
	if h == nil {
		return nil
	}
	return h.active
}

// CommittedOperations counts semantic operations that actually ran, including
// empty purge/drop/preparation invocations. Typed-build/stockpile attempt metadata
// (tags 5/4) alone is not a committed operation (DESIGN_MULTIPLAYER §16.3.26).
func (a *ApplicationAttempt) CommittedOperations() uint32 {
	if a == nil {
		return 0
	}
	return a.committedOperations
}

// IssuedOrders counts actual insertions/coalescences; a nil producer error does
// not prove either occurred, since existing insertion guards can refuse work.
func (a *ApplicationAttempt) IssuedOrders() uint32 {
	if a == nil {
		return 0
	}
	return a.issuedOrders
}

type checkpointQueueObservation struct {
	attempt *ApplicationAttempt
	actor   checkpoint.Allocation
}

func (o checkpointQueueObservation) RecordCheckpointOrder(receipt orders.CheckpointOrderReceipt) {
	o.attempt.RecordOrder(o.actor, receipt)
}
func noCheckpointQueueObservation() {}

// ObserveQueue observes an already-existing queue for one known actor and
// returns its scope restoration. It never resolves, creates or binds a queue.
// Nested typed producers replace/restore the same attempt observer, so one
// actual queue operation produces one receipt, not duplicate fan-out.
func (a *ApplicationAttempt) ObserveQueue(q *orders.Queue, actor *units.Unit) func() {
	if a == nil || q == nil {
		return noCheckpointQueueObservation
	}
	ref, err := CheckpointAllocation(actor)
	if err != nil {
		a.history.Fail(err)
		return noCheckpointQueueObservation
	}
	prior := q.SetCheckpointObserver(checkpointQueueObservation{a, ref})
	return func() { q.SetCheckpointObserver(prior) }
}

// RecordActivation precedes the existing setter invocation, including an
// already-equal value. Any nested queue receipts therefore follow it.
func (a *ApplicationAttempt) RecordActivation(actor checkpoint.Allocation, active bool) {
	a.Operation(6, func(e *checkpoint.Encoder) error {
		if !validApplicationAllocation(actor) {
			return aiCheckpointError("ai.history.activation", "an observed allocation")
		}
		e.Allocation(actor)
		e.Bool(active)
		return e.Err()
	})
}

// RecordBuild records the actual typed request and known producer verdict.
// The actor is the observed issuing allocation; Builder remains its raw request
// handle. Definition identity was already recorded in the intent when known.
// Typed build currently has no facing input, so its actual facing is zero.
func (a *ApplicationAttempt) RecordBuild(actor checkpoint.Allocation, req BuildRequest, result error) {
	if a == nil {
		return
	}
	verdict, known := CheckpointBuildVerdict(result)
	if !known {
		a.history.Fail(aiCheckpointError("ai.history.build.verdict", "a classified typed-build outcome"))
		return
	}
	a.Operation(5, func(e *checkpoint.Encoder) error {
		if !validApplicationAllocation(actor) {
			return aiCheckpointError("ai.history.build.actor", "an observed allocation")
		}
		e.Allocation(actor)
		e.U32(uint32(req.Builder))
		e.String(req.UnitKey)
		e.I64(int64(req.X))
		e.I64(int64(req.Z))
		e.U8(0)
		e.I64(int64(req.Count))
		e.I64(int64(req.Kind))
		e.U32(req.Tick)
		e.U8(verdict)
		return e.Err()
	})
}

// CoalescedOrders counts only actual tail-coalescence receipts. A successful
// insertion alone does not mean an existing counted node was changed.
func (a *ApplicationAttempt) CoalescedOrders() uint32 {
	if a == nil {
		return 0
	}
	return a.coalescedOrders
}

// RecordStockpile records the producer's already-capped count after the actual
// queue call. The Boolean comes from its coalescence receipt, not its return.
func (a *ApplicationAttempt) RecordStockpile(actor checkpoint.Allocation, row orders.ID, count int64, coalesced bool) {
	a.Operation(4, func(e *checkpoint.Encoder) error {
		if !validApplicationAllocation(actor) {
			return aiCheckpointError("ai.history.stockpile", "an observed allocation")
		}
		e.Allocation(actor)
		e.U8(uint8(row))
		e.I64(count)
		e.Bool(coalesced)
		return e.Err()
	})
}

// checkpointInsertion is transient outer-call completion evidence. It is not
// part of the operation stream or the full checkpoint payload.
type checkpointInsertion struct {
	index  uint64
	actor  checkpoint.Allocation
	result orders.CheckpointInsertionResult
}

func (o checkpointQueueObservation) RecordCheckpointInsertion(result orders.CheckpointInsertionResult) {
	a := o.attempt
	if a == nil {
		return
	}
	h := a.history
	if a.finished || h.active != a {
		h.Fail(aiCheckpointError("ai.history.insertion", "an active attempt"))
		return
	}
	if h.err != nil {
		return
	}
	if !validApplicationAllocation(o.actor) || result.Method < 1 || result.Method > 2 || result.Inserted && result.Coalesced || result.Method == 1 && result.Coalesced || a.lastInsertion.index == math.MaxUint64 {
		h.Fail(aiCheckpointError("ai.history.insertion", "a known completion and available position"))
		return
	}
	a.lastInsertion = checkpointInsertion{index: a.lastInsertion.index + 1, actor: o.actor, result: result}
}

// InsertionIndex marks the completion position immediately before one existing
// insertion/typed call, after any preceding purge/drop (§16.3.29).
func (a *ApplicationAttempt) InsertionIndex() uint64 {
	if a == nil {
		return 0
	}
	return a.lastInsertion.index
}

// InsertionAfter returns only the latest fresh completion of the expected actor
// and method (Push 1, CoalesceTail 2). A refused outer completion supersedes any
// nested success. Absence is not insertion success.
func (a *ApplicationAttempt) InsertionAfter(index uint64, actor checkpoint.Allocation, method uint8) (orders.CheckpointInsertionResult, bool) {
	if a == nil || a.lastInsertion.index <= index || a.lastInsertion.actor != actor || a.lastInsertion.result.Method != method {
		return orders.CheckpointInsertionResult{}, false
	}
	return a.lastInsertion.result, true
}
