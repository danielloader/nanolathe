package orders

import (
	"bytes"
	"reflect"
	"strings"
	"testing"

	"github.com/nanolathe-gg/nanolathe/internal/pool"
	"github.com/nanolathe-gg/nanolathe/internal/sim/checkpoint"
	"github.com/nanolathe-gg/nanolathe/internal/sim/rng"
	"github.com/nanolathe-gg/nanolathe/internal/units"
)

type orderReceiptObserver func(CheckpointOrderReceipt)

func (f orderReceiptObserver) RecordCheckpointOrder(r CheckpointOrderReceipt) { f(r) }

type orderReceiptKey struct {
	kind, segment uint8
	index         int64
	preparation   uint8
}

func orderReceiptKeys(receipts []CheckpointOrderReceipt) []orderReceiptKey {
	keys := make([]orderReceiptKey, len(receipts))
	for i, r := range receipts {
		keys[i] = orderReceiptKey{r.Kind, r.Segment, r.Index, r.Preparation}
	}
	return keys
}

// Receipts describe the final insertion, after constructor input consumption,
// caption arming, inherited flags and marker transfer [04 R-ORD-01 §13].
func TestCheckpointReceiptsInsertionShapes(t *testing.T) {
	primary := []*Node{{Flags: FlagActive, Param1: 1}, {Param1: 2}}
	rear := []*Node{{Flags: FlagAutoOp, Param1: 3}}
	tests := []struct {
		name      string
		primary   []*Node
		secondary []*Node
		insert    func(*Queue, Node)
		keys      []orderReceiptKey
		flags     uint32
		caption   bool
		oldActive bool
	}{
		{"after marker", primary, nil, func(q *Queue, n Node) { q.Push(Lookup("Move_Ground"), n) },
			[]orderReceiptKey{{5, 0, 0, 1}, {5, 0, 0, 2}, {3, 1, 1, 0}}, FlagActive, false, false},
		{"without marker", []*Node{{Param1: 1}, {Param1: 2}}, nil, func(q *Queue, n Node) { q.Push(Lookup("Move_Ground"), n) },
			[]orderReceiptKey{{5, 0, 0, 1}, {5, 0, 0, 2}, {3, 1, 2, 0}}, FlagActive, false, false},
		{"producer head", primary, nil, func(q *Queue, n Node) { n.QueuedIssue = false; q.Push(Lookup("Activate"), n) },
			[]orderReceiptKey{{5, 0, 0, 1}, {5, 0, 0, 2}, {3, 1, 0, 0}}, 0, true, true},
		{"producer rear", primary, rear, func(q *Queue, n Node) { n.QueuedIssue = false; q.Push(Lookup("BuildWeapon"), n) },
			[]orderReceiptKey{{3, 2, 0, 0}}, FlagAutoOp, true, true},
		{"handler head", []*Node{{Flags: FlagAutoOp | FlagActive}}, nil, func(q *Queue, n Node) { q.PushHead(Lookup("Move_Ground"), n) },
			[]orderReceiptKey{{3, 1, 0, 0}}, FlagAutoOp, false, true},
		// PushSecondary currently preserves a supplied marker. Observation must
		// not normalize this producer quirk into the other insertion's flags.
		{"secondary head", primary, rear, func(q *Queue, n Node) { q.PushSecondary(Lookup("BuildWeapon"), n) },
			[]orderReceiptKey{{3, 2, 0, 0}}, FlagActive | FlagAutoOp, false, true},
		{"primary tail", primary, nil, func(q *Queue, n Node) { q.appendTail(Lookup("Move_Ground"), n) },
			[]orderReceiptKey{{3, 1, 2, 0}}, 0, false, true},
		{"secondary tail", primary, rear, func(q *Queue, n Node) { q.appendTail(Lookup("BuildWeapon"), n) },
			[]orderReceiptKey{{3, 2, 1, 0}}, 0, false, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q := &Queue{}
			for _, n := range tt.primary {
				copy := *n
				q.primary = append(q.primary, &copy)
			}
			for _, n := range tt.secondary {
				copy := *n
				q.secondary = append(q.secondary, &copy)
			}
			oldHead := q.primary[0]
			var receipts []CheckpointOrderReceipt
			q.SetCheckpointObserver(orderReceiptObserver(func(r CheckpointOrderReceipt) {
				if r.Kind == 3 {
					segment := q.primary
					if r.Segment == 2 {
						segment = q.secondary
					}
					if r.Index < 0 || r.Index >= int64(len(segment)) || !reflect.DeepEqual(r.Node, *segment[r.Index]) {
						t.Fatalf("receipt precedes final insertion: %+v", r)
					}
				}
				receipts = append(receipts, r)
			}))
			tt.insert(q, Node{Owner: 0xfffe, GoalX: 1 << 40, DynamicGate: 0x102, Flags: FlagActive, GoalSupplied: true, QueuedIssue: true})
			if got := orderReceiptKeys(receipts); !reflect.DeepEqual(got, tt.keys) {
				t.Fatalf("receipt sequence = %v, want %v", got, tt.keys)
			}
			n := receipts[len(receipts)-1].Node
			if n.Flags != tt.flags || n.CaptionPending != tt.caption || n.Deadline != -1 || n.Owner != 0xfffe || n.GoalX != 1<<40 || n.DynamicGate != 0x102 || n.GoalSupplied || n.QueuedIssue {
				t.Fatalf("final node = %+v; want flags %x, caption %v, consumed insertion inputs", n, tt.flags, tt.caption)
			}
			if (oldHead.Flags&FlagActive != 0) != tt.oldActive {
				t.Fatalf("old head marker = %x, want active %v", oldHead.Flags, tt.oldActive)
			}
		})
	}
}

// Tail counts wrap at their source width, and a requested zero is one actual
// addition [05 "Queue insertion"]. A different product runs ordinary Push.
func TestCheckpointReceiptsCoalescence(t *testing.T) {
	for _, name := range []string{"MobileBuild", "BuildWeapon"} {
		t.Run(name, func(t *testing.T) {
			id := Lookup(name)
			head := &Node{ID: id, BuildDefKey: "other"}
			tail := &Node{ID: id, BuildDefKey: "product", Param1: 7, Param2: ^uint32(0), GoalX: 23, GoalZ: 29}
			q := &Queue{primary: []*Node{head, tail}}
			segment := uint8(1)
			if name == "BuildWeapon" {
				q.primary, q.secondary = nil, q.primary
				segment = 2
			}
			var receipts []CheckpointOrderReceipt
			q.SetCheckpointObserver(orderReceiptObserver(func(r CheckpointOrderReceipt) { receipts = append(receipts, r) }))
			input := Node{BuildDefKey: "product", Param1: 7, Param2: 2, GoalX: 23, GoalZ: 29}
			q.CoalesceTail(id, input)
			input.Param2 = 0
			q.CoalesceTail(id, input)
			if got := orderReceiptKeys(receipts); !reflect.DeepEqual(got, []orderReceiptKey{{4, segment, 1, 0}, {4, segment, 1, 0}}) {
				t.Fatalf("coalescence receipts = %v", got)
			}
			if receipts[0].PreviousCount != ^uint32(0) || receipts[0].Added != 2 || receipts[0].Node.Param2 != 1 || receipts[1].PreviousCount != 1 || receipts[1].Added != 1 || receipts[1].Node.Param2 != 2 || tail.Param2 != 2 {
				t.Fatalf("coalesced counts = %+v", receipts)
			}
			receipts = nil
			input.BuildDefKey = "different"
			q.CoalesceTail(id, input)
			want := []orderReceiptKey{{5, 0, 0, 1}, {5, 0, 0, 2}, {3, 1, 2, 0}}
			if segment == 2 {
				want = []orderReceiptKey{{3, 2, 0, 0}}
			}
			if got := orderReceiptKeys(receipts); !reflect.DeepEqual(got, want) {
				t.Fatalf("fallback = %v, want ordinary Push %v", got, want)
			}
		})
	}
}

type receiptBeforeRules struct {
	StrictRules
	before func(*Queue)
}

func (r receiptBeforeRules) BeforeCommand(q *Queue) { r.before(q) }

func TestCheckpointReceiptsPreparationBeforeOOMRefusal(t *testing.T) {
	for _, coalesce := range []bool{false, true} {
		q := &Queue{primary: make([]*Node, OOMGuardQueue), lastPumpTick: 17}
		firing := &Node{Phase: 4}
		for i := range q.primary {
			q.primary[i] = firing
		}
		q.firingPosition = firingPositionState{node: firing, active: true}
		var events []string
		q.binding = &QueueBinding{
			Movement: NewMovementGoalAdapter(MovementGoalAdapterConfig{Destroy: func(*Node) bool { events = append(events, "destroy"); return true }}),
			Rules: receiptBeforeRules{before: func(q *Queue) {
				if q.firingPosition.active || firing.Phase != 1 {
					t.Fatal("rules ran before firing-position preparation")
				}
				events = append(events, "rules")
			}},
		}
		q.SetCheckpointObserver(orderReceiptObserver(func(r CheckpointOrderReceipt) {
			switch {
			case r.Kind == 5 && r.Preparation == 1:
				events = append(events, "prepare firing")
			case r.Kind == 5 && r.Preparation == 2:
				events = append(events, "prepare rules")
			default:
				t.Fatalf("refused insertion emitted %+v", r)
			}
		}))
		if coalesce {
			q.CoalesceTail(Lookup("Move_Ground"), Node{})
		} else {
			q.Push(Lookup("Move_Ground"), Node{})
		}
		if want := []string{"prepare firing", "destroy", "prepare rules", "rules"}; !reflect.DeepEqual(events, want) {
			t.Fatalf("pre-OOM events = %v, want %v", events, want)
		}
		if len(q.primary) != OOMGuardQueue || len(q.diagnostics) != 1 {
			t.Fatal("OOM refusal changed the queue or omitted its diagnostic")
		}
	}
}

// Cleanup can re-enter a queue. The outer operation is observed before any
// nested removal or insertion, including calls on an empty segment.
func TestCheckpointReceiptsRemovalReentry(t *testing.T) {
	for _, drop := range []bool{false, true} {
		q, u := &Queue{}, newTestUnit()
		var receipts []CheckpointOrderReceipt
		var callbackHeads []int
		q.binding = NewQueueBinding(QueueBindingConfig{
			Lookup: func(pool.Handle) *units.Unit { return u },
			Work: NewWorkAdapter(WorkAdapterConfig{CancelNotice: func(_ *units.Unit, n *Node, _ uint32) bool {
				callbackHeads = append(callbackHeads, len(q.primary))
				n.DynamicGate &^= 2
				q.PurgeUnprotected()
				q.appendTail(Lookup("BuildWeapon"), Node{Param2: 7})
				return true
			}}),
		})
		q.primary = []*Node{{ID: Lookup("MobileBuild"), Owner: u.Handle, DynamicGate: 2, Flags: FlagAutoOp}}
		q.SetCheckpointObserver(orderReceiptObserver(func(r CheckpointOrderReceipt) { receipts = append(receipts, r) }))
		kind, headCount := uint8(1), 0
		if drop {
			kind, headCount = 2, 1
			q.DropLeadingAutoOps()
		} else {
			q.PurgeUnprotected()
		}
		q.PurgeUnprotected()
		q.DropLeadingAutoOps()
		want := []orderReceiptKey{{kind, 0, 0, 0}, {1, 0, 0, 0}, {3, 2, 0, 0}, {1, 0, 0, 0}, {2, 0, 0, 0}}
		if got := orderReceiptKeys(receipts); !reflect.DeepEqual(got, want) {
			t.Fatalf("drop %v: reentry = %v, want %v", drop, got, want)
		}
		if !reflect.DeepEqual(callbackHeads, []int{headCount}) || len(q.primary) != 0 || len(q.secondary) != 1 || q.secondary[0].Param2 != 7 {
			t.Fatalf("drop %v: cleanup timing/queue changed; callback heads %v", drop, callbackHeads)
		}
	}
}

type orderReceiptCount struct{ count int }

func (o *orderReceiptCount) RecordCheckpointOrder(CheckpointOrderReceipt) { o.count++ }

func TestCheckpointObserverScopeAndCaptureRefusal(t *testing.T) {
	var absent *Queue
	outer, inner := &orderReceiptCount{}, &orderReceiptCount{}
	if absent.SetCheckpointObserver(outer) != nil {
		t.Fatal("nil queue installed an observer")
	}
	q := &Queue{}
	c := orderCheckpointContext(t, &units.Unit{Orders: q})
	collectOrdersCheckpoint(t, c)
	baseline := orderCheckpointBytes(t, c)
	if q.SetCheckpointObserver(outer) != nil {
		t.Fatal("new queue had an observer")
	}
	q.PurgeUnprotected()
	prior := q.SetCheckpointObserver(inner)
	if prior != outer {
		t.Fatal("nested scope did not retain outer observer")
	}
	q.DropLeadingAutoOps()
	if got := q.SetCheckpointObserver(prior); got != inner {
		t.Fatal("nested scope did not restore its predecessor")
	}
	q.PurgeUnprotected()
	if outer.count != 2 || inner.count != 1 {
		t.Fatalf("scope fanout: outer %d inner %d", outer.count, inner.count)
	}
	if _, err := (&Pump{}).CollectCheckpointReferences(c); err == nil || !strings.Contains(err.Error(), "checkpointObserver") {
		t.Fatalf("active observer collection = %v", err)
	}
	var out bytes.Buffer
	if err := (&Pump{}).WriteCheckpoint(checkpoint.NewEncoder(&out), c); err == nil || !strings.Contains(err.Error(), "checkpointObserver") {
		t.Fatalf("active observer write = %v", err)
	}
	if q.SetCheckpointObserver(nil) != outer {
		t.Fatal("outer scope restoration failed")
	}
	if got := orderCheckpointBytes(t, c); !bytes.Equal(got, baseline) || outer.count != 2 || inner.count != 1 {
		t.Fatal("observer or capture leaked into retained state")
	}
}

func receiptNodeBytes(t *testing.T, n *Node) []byte {
	t.Helper()
	var out bytes.Buffer
	if err := n.WriteCheckpointValue(checkpoint.NewEncoder(&out)); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}

// The journal's value framing has no table/ID envelope. This independent
// vector fixes full-width raw handles, Fixed coordinates and private values.
func TestCheckpointReceiptNodeValueVector(t *testing.T) {
	n := &Node{GoalX: 1 << 40, Owner: 0xabcd, Target: 0x1234, Param2: 2, automaticAttack: true}
	want := checkpointHex(t, `
		00000000 00 0000 0000 00 00000000 00000000 00000000 00000000
		0000000000010000 0000000000000000 0000000000000000
		0000 0000 0000000000000000 00 00 cdab0000
		00000000 02000000 00000000 00000000 00 00000000 00000000 34120000
		01 00 00 0000000000000000 0000000000000000 00000000 00000000
		00000000 00000000 00000000`)
	if got := receiptNodeBytes(t, n); !bytes.Equal(got, want) {
		t.Fatalf("node value\n got %x\nwant %x", got, want)
	}
	for _, invalid := range []*Node{nil, {GoalSupplied: true}, {QueuedIssue: true}} {
		var out bytes.Buffer
		if err := invalid.WriteCheckpointValue(checkpoint.NewEncoder(&out)); err == nil || out.Len() != 0 {
			t.Fatalf("unconsumed node %v: error %v, bytes %x", invalid, err, out.Bytes())
		}
	}
}

func TestCheckpointReceiptsConsumeBorrowedPayloadImmediately(t *testing.T) {
	input := Node{RetailSubtype: []byte{7, 9}, RetailSubtypeWords16: []uint16{11}, RetailSubtypeWords32: []uint32{13}, Param2: 17}
	var payload []byte
	var words16 []uint16
	var words32 []uint32
	var encoded []byte
	q := &Queue{}
	q.SetCheckpointObserver(orderReceiptObserver(func(r CheckpointOrderReceipt) {
		payload = append([]byte(nil), r.Node.RetailSubtype...)
		words16 = append([]uint16(nil), r.Node.RetailSubtypeWords16...)
		words32 = append([]uint32(nil), r.Node.RetailSubtypeWords32...)
		encoded = receiptNodeBytes(t, &r.Node)
	}))
	n := q.PushHead(Lookup("MobileBuild"), input)
	n.Param2 = 19
	input.RetailSubtype[0], input.RetailSubtypeWords16[0], input.RetailSubtypeWords32[0] = 21, 23, 25
	if !bytes.Equal(payload, []byte{7, 9}) || !reflect.DeepEqual(words16, []uint16{11}) || !reflect.DeepEqual(words32, []uint32{13}) {
		t.Fatal("immediate payload snapshot changed after the callback")
	}
	if bytes.Equal(encoded, receiptNodeBytes(t, n)) {
		t.Fatal("immediate encoding followed a later scalar mutation")
	}
	n.Param2 = 17
	n.RetailSubtypeCode, n.RetailSubtypeUnitA, n.RetailSubtypeUnitB = 99, 101, 103
	if !bytes.Equal(encoded, receiptNodeBytes(t, n)) {
		t.Fatal("restore/save staging leaked into the retained journal value")
	}
}

// The nil-observer path must retain gameplay, callback and random-call order.
// The observer checks values only; it never participates in queue production.
func TestCheckpointReceiptsDoNotChangeApplication(t *testing.T) {
	type outcome struct {
		primary, secondary                []byte
		random                            uint32
		draws                             uint64
		lookups, cancels, rules, destroys int
	}
	run := func(observe bool) outcome {
		var result outcome
		random := rng.NewSimulation(73)
		q, u := &Queue{}, newTestUnit()
		q.binding = NewQueueBinding(QueueBindingConfig{
			Lookup: func(pool.Handle) *units.Unit { result.lookups++; return u },
			Work: NewWorkAdapter(WorkAdapterConfig{CancelNotice: func(_ *units.Unit, n *Node, _ uint32) bool {
				result.cancels++
				n.DynamicGate &^= 2
				q.appendTail(Lookup("BuildWeapon"), Node{Param1: random.Uint32n(99)})
				return true
			}}),
			Rules:    receiptBeforeRules{before: func(*Queue) { result.rules++; random.Uint32n(99) }},
			Movement: NewMovementGoalAdapter(MovementGoalAdapterConfig{Destroy: func(*Node) bool { result.destroys++; return true }}),
		})
		q.primary = []*Node{{ID: Lookup("MobileBuild"), Owner: u.Handle, DynamicGate: 2, Flags: FlagAutoOp}}
		if observe {
			q.SetCheckpointObserver(orderReceiptObserver(func(r CheckpointOrderReceipt) {
				if r.Kind == 3 || r.Kind == 4 {
					receiptNodeBytes(t, &r.Node)
				}
			}))
		}
		q.Push(Lookup("Move_Ground"), Node{Owner: u.Handle})
		q.PurgeUnprotected()
		q.CoalesceTail(Lookup("BuildWeapon"), Node{BuildDefKey: "product", Param2: 3})
		q.CoalesceTail(Lookup("BuildWeapon"), Node{BuildDefKey: "product", Param2: 0})
		for _, n := range q.primary {
			result.primary = append(result.primary, receiptNodeBytes(t, n)...)
		}
		for _, n := range q.secondary {
			result.secondary = append(result.secondary, receiptNodeBytes(t, n)...)
		}
		result.random, result.draws = random.State, random.Draws()
		return result
	}
	without, with := run(false), run(true)
	if !reflect.DeepEqual(without, with) || without.cancels != 1 || without.rules != 1 || without.draws != 2 {
		t.Fatalf("observation changed application:\nwithout %+v\nwith %+v", without, with)
	}
}

func TestCheckpointReceiptsAbsentObserverDoesNotAllocate(t *testing.T) {
	q := &Queue{secondary: []*Node{{ID: Lookup("BuildWeapon"), Param1: 7}}}
	n := Node{Param1: 7, Param2: 1}
	if allocs := testing.AllocsPerRun(100, func() {
		q.PurgeUnprotected()
		q.DropLeadingAutoOps()
		q.CoalesceTail(Lookup("BuildWeapon"), n)
	}); allocs != 0 {
		t.Fatalf("nil observer allocated %g receipts", allocs)
	}
}
