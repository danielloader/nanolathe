package ai

import (
	"bytes"
	"math"
	"testing"

	"github.com/nanolathe-gg/nanolathe/internal/orders"
	"github.com/nanolathe-gg/nanolathe/internal/sim/checkpoint"
)

func TestCheckpointInsertionCompletionIsFreshAndActorScoped(t *testing.T) {
	h := historyFixture(t, 1)
	a := h.BeginAttempt(7, h.NextSerial(), 0, historyIntent(1))
	actor := checkpoint.Allocation{Handle: 7, Serial: 9}
	o := checkpointQueueObservation{a, actor}
	mark := a.InsertionIndex()
	if _, ok := a.InsertionAfter(mark, actor, 1); ok {
		t.Fatal("absent completion accepted")
	}
	// A nested success is superseded by the refused outer call. These are
	// separate metadata notifications, not operations in the retained stream.
	o.RecordCheckpointInsertion(orders.CheckpointInsertionResult{Method: 1, Row: orders.Lookup("Move_Ground"), Inserted: true})
	o.RecordCheckpointInsertion(orders.CheckpointInsertionResult{Method: 1, Row: orders.Lookup("Move_Ground")})
	r, ok := a.InsertionAfter(mark, actor, 1)
	if !ok || r.Inserted || r.Coalesced || a.InsertionIndex() != 2 {
		t.Fatal("outer refusal inherited nested success", r, ok)
	}
	if _, ok := a.InsertionAfter(mark, checkpoint.Allocation{Handle: 7, Serial: 10}, 1); ok {
		t.Fatal("reused handle completion accepted")
	}
	if _, ok := a.InsertionAfter(mark, actor, 2); ok {
		t.Fatal("wrong method completion accepted")
	}
	if _, ok := a.InsertionAfter(a.InsertionIndex(), actor, 1); ok {
		t.Fatal("stale completion accepted")
	}
	mark = a.InsertionIndex()
	o.RecordCheckpointInsertion(orders.CheckpointInsertionResult{Method: 2, Row: orders.Lookup("BuildWeapon"), Coalesced: true})
	r, ok = a.InsertionAfter(mark, actor, 2)
	if !ok || !r.Coalesced || r.Inserted || a.operationCount != 0 || a.CommittedOperations() != 0 || a.IssuedOrders() != 0 || a.CoalescedOrders() != 0 {
		t.Fatal("completion changed receipt counts")
	}
	before := append([]byte(nil), a.header.Bytes()...)
	a.Finish(1, 3)
	if a.InsertionIndex() != 0 {
		t.Fatal("completed attempt retained completion")
	}
	other := historyFixture(t, 1)
	plain := other.BeginAttempt(7, other.NextSerial(), 0, historyIntent(1))
	if !bytes.Equal(before, plain.header.Bytes()) {
		t.Fatal("metadata changed intent")
	}
	plain.Finish(1, 3)
	x, err := h.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	y, err := other.Snapshot()
	if err != nil || x != y {
		t.Fatal("completion metadata changed wire history", err)
	}
}

func TestCheckpointInsertionCompletionRefusalsAndDisabled(t *testing.T) {
	var absent *ApplicationAttempt
	if absent.InsertionIndex() != 0 {
		t.Fatal("disabled position")
	}
	if _, ok := absent.InsertionAfter(0, checkpoint.Allocation{}, 1); ok {
		t.Fatal("disabled completion")
	}
	for _, result := range []orders.CheckpointInsertionResult{{Method: 0}, {Method: 3}, {Method: 1, Coalesced: true}, {Method: 2, Inserted: true, Coalesced: true}} {
		h := historyFixture(t, 1)
		a := h.BeginAttempt(0, h.NextSerial(), 0, historyIntent(1))
		checkpointQueueObservation{a, checkpoint.Allocation{Handle: 7, Serial: 9}}.RecordCheckpointInsertion(result)
		a.Finish(1, 3)
		if _, err := h.Snapshot(); err == nil {
			t.Fatal("unknown completion accepted")
		}
	}
	h := historyFixture(t, 1)
	a := h.BeginAttempt(0, h.NextSerial(), 0, historyIntent(1))
	a.lastInsertion.index = math.MaxUint64
	checkpointQueueObservation{a, checkpoint.Allocation{Handle: 7, Serial: 9}}.RecordCheckpointInsertion(orders.CheckpointInsertionResult{Method: 1})
	if a.InsertionIndex() != math.MaxUint64 {
		t.Fatal("completion position wrapped")
	}
	a.Finish(1, 3)
	if _, err := h.Snapshot(); err == nil {
		t.Fatal("overflow accepted")
	}
}
