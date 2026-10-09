package ai

import (
	"github.com/nanolathe-gg/nanolathe/internal/orders"
	"github.com/nanolathe-gg/nanolathe/internal/sim/checkpoint"
)

// RecordOrder consumes a scoped order receipt synchronously, preserving the
// actual stored node and counts (DESIGN_MULTIPLAYER §16.3.21–§16.3.23). Actor
// identity belongs to the adapter; a nested cleanup's node owner may differ.
// Diagnostic failure is sticky in the history and never changes gameplay.
// Receipt count does not classify the attempt's terminal outcome.
func (a *ApplicationAttempt) RecordOrder(actor checkpoint.Allocation, receipt orders.CheckpointOrderReceipt) {
	if a == nil {
		return
	}
	var kind uint16
	switch receipt.Kind {
	case 1, 2, 3:
		kind = uint16(receipt.Kind)
	case 4:
		kind = 10
	case 5:
		kind = 9
	default:
		a.history.Fail(aiCheckpointError("ai.history.orderReceipt.Kind", "order receipt kind 1 through 5"))
		return
	}
	a.Operation(kind, func(e *checkpoint.Encoder) error {
		e.Field("ai.history.orderReceipt")
		if !validApplicationAllocation(actor) {
			return aiCheckpointError("ai.history.orderReceipt.actor", "a nonzero allocation handle and serial")
		}
		if (receipt.Kind == 3 || receipt.Kind == 4) && (receipt.Segment < 1 || receipt.Segment > 2 || receipt.Index < 0) {
			return aiCheckpointError("ai.history.orderReceipt.position", "segment 1 or 2 and a nonnegative index")
		}
		if receipt.Kind == 5 && (receipt.Preparation < 1 || receipt.Preparation > 2) {
			return aiCheckpointError("ai.history.orderReceipt.Preparation", "preparation subkind 1 or 2")
		}
		e.Allocation(actor)
		switch receipt.Kind {
		case 3, 4:
			e.U8(receipt.Segment)
			e.I64(receipt.Index)
			if receipt.Kind == 4 {
				e.U32(receipt.PreviousCount)
				e.U32(receipt.Added)
			}
			return receipt.Node.WriteCheckpointValue(e)
		case 5:
			e.U8(receipt.Preparation)
		}
		return e.Err()
	})
}
