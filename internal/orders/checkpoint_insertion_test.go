package orders

import (
	"reflect"
	"testing"
)

type checkpointInsertionObserver struct {
	receipt    func(CheckpointOrderReceipt)
	completion func(CheckpointInsertionResult)
}

func (o *checkpointInsertionObserver) RecordCheckpointOrder(r CheckpointOrderReceipt) {
	if o.receipt != nil {
		o.receipt(r)
	}
}

func (o *checkpointInsertionObserver) RecordCheckpointInsertion(r CheckpointInsertionResult) {
	if o.completion != nil {
		o.completion(r)
	}
}

func checkpointInsert(q *Queue, method uint8, row ID, n Node) {
	if method == 1 {
		q.Push(row, n)
	} else {
		q.CoalesceTail(row, n)
	}
}

// Completion reports the invocation's branch, including descriptor zero and
// both producer insertion shapes [04 §3.1][04 R-ORD-01 §13].
func TestCheckpointInsertionResults(t *testing.T) {
	for _, row := range []ID{0, Lookup("Move_Ground"), Lookup("Activate"), Lookup("BuildWeapon")} {
		for _, method := range []uint8{1, 2} {
			q := &Queue{}
			var got []CheckpointInsertionResult
			q.SetCheckpointObserver(&checkpointInsertionObserver{completion: func(r CheckpointInsertionResult) {
				if len(q.primary)+len(q.secondary) != 1 {
					t.Fatal("completion preceded insertion")
				}
				got = append(got, r)
			}})
			checkpointInsert(q, method, row, Node{Param1: 7, Param2: 3})
			want := []CheckpointInsertionResult{{Method: method, Row: row, Inserted: true}}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("row %d method %d: got %+v, want %+v", row, method, got, want)
			}
		}
	}
}

// Zero becomes one and the source-width count wraps [05 "Queue insertion"].
// Completion follows the unchanged coalescence receipt, even with a full list.
func TestCheckpointInsertionCoalescence(t *testing.T) {
	for _, row := range []ID{Lookup("MobileBuild"), Lookup("BuildWeapon")} {
		tail := &Node{ID: row, Param1: 7, Param2: ^uint32(0), BuildDefKey: "product", GoalX: 23, GoalZ: 29}
		q := &Queue{primary: make([]*Node, OOMGuardQueue)}
		for i := range q.primary {
			q.primary[i] = tail
		}
		if isSecondary(row) {
			q.secondary, q.primary = q.primary, nil
		}
		var receipts []CheckpointOrderReceipt
		var got []CheckpointInsertionResult
		q.SetCheckpointObserver(&checkpointInsertionObserver{
			receipt: func(r CheckpointOrderReceipt) { receipts = append(receipts, r) },
			completion: func(r CheckpointInsertionResult) {
				if len(receipts) != len(got)+1 || receipts[len(got)].Node.Param2 != tail.Param2 {
					t.Fatal("completion preceded final coalescence receipt")
				}
				got = append(got, r)
			},
		})
		q.CoalesceTail(row, Node{Param1: 7, Param2: 2, BuildDefKey: "product", GoalX: 23, GoalZ: 29})
		q.CoalesceTail(row, Node{Param1: 7, Param2: 0, BuildDefKey: "product", GoalX: 23, GoalZ: 29})
		want := []CheckpointInsertionResult{{Method: 2, Row: row, Coalesced: true}, {Method: 2, Row: row, Coalesced: true}}
		if !reflect.DeepEqual(got, want) || tail.Param2 != 2 || receipts[0].PreviousCount != ^uint32(0) || receipts[0].Added != 2 || receipts[1].PreviousCount != 1 || receipts[1].Added != 1 {
			t.Fatalf("row %d: completion %+v, receipts %+v, final count %d", row, got, receipts, tail.Param2)
		}
	}
}

// A nested successful handler insertion fills the last slot before the outer
// allocation guard. Its mutation receipt must not imply outer success.
func TestCheckpointInsertionNestedSuccessOuterRefusal(t *testing.T) {
	for _, method := range []uint8{1, 2} {
		q := &Queue{primary: make([]*Node, OOMGuardQueue-1)}
		for i := range q.primary {
			q.primary[i] = &Node{ID: Lookup("Standby")}
		}
		var nested *Node
		q.binding = &QueueBinding{Rules: receiptBeforeRules{before: func(q *Queue) {
			nested = q.PushHead(Lookup("Activate"), Node{Param1: 19})
		}}}
		var receipts []CheckpointOrderReceipt
		var got []CheckpointInsertionResult
		q.SetCheckpointObserver(&checkpointInsertionObserver{
			receipt: func(r CheckpointOrderReceipt) { receipts = append(receipts, r) },
			completion: func(r CheckpointInsertionResult) {
				if nested == nil || len(q.diagnostics) != 1 {
					t.Fatal("completion preceded nested callback or outer refusal")
				}
				got = append(got, r)
			},
		})
		row := Lookup("Move_Ground")
		checkpointInsert(q, method, row, Node{})
		want := []CheckpointInsertionResult{{Method: method, Row: row}}
		wantReceipts := []orderReceiptKey{{5, 0, 0, 1}, {5, 0, 0, 2}, {3, 1, 0, 0}}
		if !reflect.DeepEqual(got, want) || !reflect.DeepEqual(orderReceiptKeys(receipts), wantReceipts) || len(q.primary) != OOMGuardQueue || q.primary[0] != nested {
			t.Fatalf("method %d: completions %+v, receipts %+v", method, got, orderReceiptKeys(receipts))
		}
	}
}

func TestCheckpointInsertionNestedPublicOrder(t *testing.T) {
	for _, method := range []uint8{1, 2} {
		var events []string
		var got []CheckpointInsertionResult
		q := &Queue{}
		q.binding = &QueueBinding{Rules: receiptBeforeRules{before: func(q *Queue) {
			q.Push(Lookup("BuildWeapon"), Node{Param1: 7, Param2: 1})
			q.CoalesceTail(Lookup("BuildWeapon"), Node{Param1: 7, Param2: 2})
		}}}
		q.SetCheckpointObserver(&checkpointInsertionObserver{
			receipt: func(r CheckpointOrderReceipt) {
				switch r.Kind {
				case 5:
					events = append(events, "prepare")
				case 3:
					events = append(events, "insert")
				case 4:
					events = append(events, "coalesce")
				default:
					t.Fatalf("unexpected receipt %+v", r)
				}
			},
			completion: func(r CheckpointInsertionResult) {
				events = append(events, "complete")
				got = append(got, r)
			},
		})
		row, rear := Lookup("Move_Ground"), Lookup("BuildWeapon")
		checkpointInsert(q, method, row, Node{})
		want := []CheckpointInsertionResult{{Method: 1, Row: rear, Inserted: true}, {Method: 2, Row: rear, Coalesced: true}, {Method: method, Row: row, Inserted: true}}
		wantEvents := []string{"prepare", "prepare", "insert", "complete", "coalesce", "complete", "insert", "complete"}
		if !reflect.DeepEqual(got, want) || !reflect.DeepEqual(events, wantEvents) {
			t.Fatalf("method %d: completions %+v, events %v", method, got, events)
		}
	}
}

func TestCheckpointInsertionPanicHasNoCompletion(t *testing.T) {
	for _, method := range []uint8{1, 2} {
		for _, afterInsertion := range []bool{false, true} {
			const sentinel = "insertion panic"
			q := &Queue{}
			if !afterInsertion {
				q.binding = &QueueBinding{Rules: receiptBeforeRules{before: func(*Queue) { panic(sentinel) }}}
			}
			completions := 0
			q.SetCheckpointObserver(&checkpointInsertionObserver{
				receipt: func(r CheckpointOrderReceipt) {
					if afterInsertion && r.Kind == 3 {
						panic(sentinel)
					}
				},
				completion: func(CheckpointInsertionResult) { completions++ },
			})
			func() {
				defer func() {
					if got := recover(); got != sentinel {
						t.Fatalf("panic = %v, want %q", got, sentinel)
					}
				}()
				checkpointInsert(q, method, Lookup("Move_Ground"), Node{})
			}()
			if completions != 0 || (len(q.primary) != 0) != afterInsertion {
				t.Fatalf("method %d after insertion %v: completions %d, nodes %d", method, afterInsertion, completions, len(q.primary))
			}
		}
	}
}

func TestCheckpointInsertionEntryObserver(t *testing.T) {
	for _, restoreDuringCall := range []bool{false, true} {
		q := &Queue{}
		var outerResults, innerResults []CheckpointInsertionResult
		outer := &checkpointInsertionObserver{completion: func(r CheckpointInsertionResult) { outerResults = append(outerResults, r) }}
		inner := &checkpointInsertionObserver{completion: func(r CheckpointInsertionResult) { innerResults = append(innerResults, r) }}
		q.SetCheckpointObserver(outer)
		q.binding = &QueueBinding{Rules: receiptBeforeRules{before: func(q *Queue) {
			prior := q.SetCheckpointObserver(inner)
			if restoreDuringCall {
				defer q.SetCheckpointObserver(prior)
			}
			q.Push(Lookup("BuildWeapon"), Node{})
		}}}
		q.CoalesceTail(Lookup("Move_Ground"), Node{})
		wantOuter := []CheckpointInsertionResult{{Method: 2, Row: Lookup("Move_Ground"), Inserted: true}}
		wantInner := []CheckpointInsertionResult{{Method: 1, Row: Lookup("BuildWeapon"), Inserted: true}}
		if !reflect.DeepEqual(outerResults, wantOuter) || !reflect.DeepEqual(innerResults, wantInner) {
			t.Fatalf("restore %v: outer %+v, inner %+v", restoreDuringCall, outerResults, innerResults)
		}
		expected := CheckpointOrderObserver(inner)
		if restoreDuringCall {
			expected = outer
		}
		if q.SetCheckpointObserver(nil) != expected {
			t.Fatal("completion changed the observer scope")
		}
	}
}

func TestCheckpointInsertionDisabledAllocations(t *testing.T) {
	var absent *Queue
	absent.Push(0, Node{})
	absent.CoalesceTail(0, Node{})
	row := Lookup("BuildWeapon")
	for _, observeReceipts := range []bool{false, true} {
		q := &Queue{secondary: []*Node{{ID: row, Param1: 7}}}
		var receipts orderReceiptCount
		if observeReceipts {
			q.SetCheckpointObserver(&receipts)
		}
		if allocs := testing.AllocsPerRun(100, func() {
			q.CoalesceTail(row, Node{Param1: 7, Param2: 1})
		}); allocs != 0 {
			t.Fatalf("receipt observer %v: allocated %g completions", observeReceipts, allocs)
		}
		if observeReceipts && receipts.count == 0 {
			t.Fatalf("ordinary receipt observer received %d receipts", receipts.count)
		}
	}
}
