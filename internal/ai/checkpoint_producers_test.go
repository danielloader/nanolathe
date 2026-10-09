package ai

import (
	"bytes"
	"errors"
	"testing"

	"github.com/nanolathe-gg/nanolathe/internal/orders"
	"github.com/nanolathe-gg/nanolathe/internal/sim/checkpoint"
	"github.com/nanolathe-gg/nanolathe/internal/units"
)

type producerObserverCount struct{ count int }

func (o *producerObserverCount) RecordCheckpointOrder(orders.CheckpointOrderReceipt) { o.count++ }

func TestCheckpointProducerQueueScopes(t *testing.T) {
	h := historyFixture(t, 1)
	a := h.BeginAttempt(1, h.NextSerial(), 0, historyIntent(1))
	if h.ActiveAttempt() != a {
		t.Fatal("active attempt lost")
	}
	q := &orders.Queue{}
	prior := &producerObserverCount{}
	q.SetCheckpointObserver(prior)
	actor := &units.Unit{Handle: 7, AllocationSerial: 9}
	restore := a.ObserveQueue(q, actor)
	inner := a.ObserveQueue(q, actor)
	q.Push(orders.Lookup("Move_Ground"), orders.Node{Owner: 7})
	inner()
	q.Push(orders.Lookup("Move_Ground"), orders.Node{Owner: 7})
	restore()
	if a.IssuedOrders() != 2 || a.CommittedOperations() != 6 || a.operationCount != 6 || prior.count != 0 {
		t.Fatal("nested observer fanout", a.IssuedOrders(), a.CommittedOperations(), a.operationCount, prior.count)
	}
	a.Finish(1, 2)
	if h.ActiveAttempt() != nil {
		t.Fatal("completed attempt retained")
	}
	q.PurgeUnprotected()
	if prior.count != 1 {
		t.Fatal("prior observer not restored")
	}
}

func TestCheckpointActivationAndBuildReceiptVector(t *testing.T) {
	h := historyFixture(t, 1)
	a := h.BeginAttempt(2, h.NextSerial(), 0, historyIntent(1))
	actor := checkpoint.Allocation{Handle: 7, Serial: 9}
	a.RecordActivation(actor, true)
	req := BuildRequest{Builder: 0xabcd, UnitKey: "ARM", X: -2, Z: 1 << 40, Count: -3, Kind: BuildKindFactoryQueue}
	a.RecordBuild(actor, req, WithCheckpointBuildVerdict(errors.New("count"), CheckpointBuildOther))
	want := applicationCodecHex(t, `0600 07000000 0900000000000000 01
 0500 07000000 0900000000000000 cdab0000 03000000 41524d
 feffffffffffffff 0000000000010000 00 fdffffffffffffff 0100000000000000 00000000 07`)
	if !bytes.Equal(a.operations.Bytes(), want) || a.CommittedOperations() != 1 || a.IssuedOrders() != 0 {
		t.Fatalf("producer receipts %x", a.operations.Bytes())
	}
	a.Finish(1, 4)
	if _, err := h.Snapshot(); err != nil {
		t.Fatal(err)
	}
}

func TestCheckpointProducerRefusalsAndDisabledScopes(t *testing.T) {
	var absent *ApplicationHistory
	if absent.ActiveAttempt() != nil {
		t.Fatal("nil history")
	}
	var a *ApplicationAttempt
	q := &orders.Queue{}
	prior := &producerObserverCount{}
	q.SetCheckpointObserver(prior)
	if n := testing.AllocsPerRun(20, func() {
		a.ObserveQueue(q, nil)()
		a.RecordActivation(checkpoint.Allocation{}, false)
		a.RecordBuild(checkpoint.Allocation{}, BuildRequest{}, nil)
	}); n != 0 {
		t.Fatal("disabled receipts allocate", n)
	}
	if a.IssuedOrders() != 0 || a.CommittedOperations() != 0 {
		t.Fatal("disabled counters")
	}
	q.PurgeUnprotected()
	if prior.count != 1 {
		t.Fatal("disabled observation replaced binding")
	}
	for _, edit := range []func(*ApplicationAttempt){
		func(a *ApplicationAttempt) { a.ObserveQueue(q, nil)() },
		func(a *ApplicationAttempt) { a.RecordActivation(checkpoint.Allocation{}, true) },
		func(a *ApplicationAttempt) { a.RecordBuild(checkpoint.Allocation{}, BuildRequest{}, nil) },
		func(a *ApplicationAttempt) {
			a.RecordBuild(checkpoint.Allocation{Handle: 7, Serial: 9}, BuildRequest{}, errors.New("unclassified"))
		},
	} {
		h := historyFixture(t, 1)
		a := h.BeginAttempt(0, h.NextSerial(), 0, historyIntent(1))
		edit(a)
		a.Finish(1, 3)
		if _, err := h.Snapshot(); err == nil || a.CommittedOperations() != 0 || a.IssuedOrders() != 0 {
			t.Fatal("invalid producer receipt accepted")
		}
	}
	h := historyFixture(t, 1)
	a = h.BeginAttempt(0, h.NextSerial(), 0, historyIntent(1))
	a.ObserveQueue(nil, nil)()
	a.Finish(1, 1)
	if _, err := h.Snapshot(); err != nil {
		t.Fatal("absent queue mutated history", err)
	}
}

func TestCheckpointStockpileMetadataFollowsActualCoalescence(t *testing.T) {
	h := historyFixture(t, 2)
	a := h.BeginAttempt(1, h.NextSerial(), 0, historyIntent(1))
	actor := &units.Unit{Handle: 7, AllocationSerial: 9}
	q := &orders.Queue{}
	restore := a.ObserveQueue(q, actor)
	row := orders.Lookup("BuildWeapon")
	n := orders.NewNodeForOrder(row, 0, 0, 0, 0, 77, actor.Handle, false)
	n.Param2 = 3
	q.CoalesceTail(row, n)
	before := a.CoalescedOrders()
	n.Param2 = 4
	q.CoalesceTail(row, n)
	if before != 0 || a.CoalescedOrders() != 1 || a.IssuedOrders() != 2 || a.CommittedOperations() != 2 {
		t.Fatal("actual counted work lost")
	}
	start := a.operations.Len()
	a.RecordStockpile(checkpoint.Allocation{Handle: 7, Serial: 9}, row, 4, a.CoalescedOrders() != before)
	want := applicationCodecHex(t, "0400 07000000 0900000000000000")
	want = append(want, byte(row))
	want = append(want, applicationCodecHex(t, "0400000000000000 01")...)
	if !bytes.Equal(a.operations.Bytes()[start:], want) || a.CommittedOperations() != 2 {
		t.Fatal("stockpile metadata framing or commit count")
	}
	restore()
	a.Finish(2, 2)
	if _, err := h.Snapshot(); err != nil {
		t.Fatal(err)
	}
	var disabled *ApplicationAttempt
	if disabled.CoalescedOrders() != 0 {
		t.Fatal("disabled count")
	}
	if n := testing.AllocsPerRun(20, func() { disabled.RecordStockpile(checkpoint.Allocation{}, 0, 0, false) }); n != 0 {
		t.Fatal("disabled stockpile metadata allocates")
	}
}
