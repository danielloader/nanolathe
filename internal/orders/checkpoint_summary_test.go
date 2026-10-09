package orders

import (
	"reflect"
	"strings"
	"testing"

	"github.com/nanolathe-gg/nanolathe/internal/sim/checkpoint"
)

func orderSummaryFixture() *Queue {
	a := &Node{ID: 9, Phase: 2, Target: 0xfffe, GoalX: -0x100000001,
		GoalY: 0x100000002, GoalZ: -3, Deadline: -2147483648,
		Param1: 0x80000001, Param2: 12, Param3: 13, Flags: 0xffffffff}
	b := &Node{ID: 15, Phase: 16, Target: 17, GoalX: 18, GoalY: 19,
		GoalZ: 20, Deadline: 21, Param1: 22, Param2: 23, Param3: 24, Flags: 25}
	return &Queue{primary: []*Node{a, b}, secondary: []*Node{a}, lastPumpTick: 0xf0000001}
}

func readOrderSummary(t *testing.T, q *Queue) checkpoint.Summary {
	t.Helper()
	var out checkpoint.Summary
	if err := q.AppendCheckpointSummary(&out); err != nil {
		t.Fatal(err)
	}
	return out
}

func TestCheckpointSummaryQueueLiteralWords(t *testing.T) {
	q := orderSummaryFixture()
	// A repeated node is retained independently at both physical positions.
	words := []uint64{37, 2,
		9, 2, 0xfffe, 0xfffffffeffffffff, 0x100000002, 0xfffffffffffffffd,
		0xffffffff80000000, 0x80000001, 12, 13, 0xffffffff,
		15, 16, 17, 18, 19, 20, 21, 22, 23, 24, 25,
		1, 9, 2, 0xfffe, 0xfffffffeffffffff, 0x100000002, 0xfffffffffffffffd,
		0xffffffff80000000, 0x80000001, 12, 13, 0xffffffff, 0xf0000001}
	var out checkpoint.Summary
	out.Word(37)
	if err := q.AppendCheckpointSummary(&out); err != nil {
		t.Fatal(err)
	}
	var sum uint64
	for i, word := range words {
		sum += uint64(i+1) * word
	}
	if count, got := out.Result(); count != uint64(len(words)) || got != sum {
		t.Fatalf("summary = (%d,%x), want (%d,%x)", count, got, len(words), sum)
	}
	empty := readOrderSummary(t, &Queue{})
	if count, sum := empty.Result(); count != 3 || sum != 0 {
		t.Fatalf("empty summary = (%d,%d)", count, sum)
	}
}

func TestCheckpointSummaryQueueSelectedExcludedAndPure(t *testing.T) {
	baseline := readOrderSummary(t, orderSummaryFixture())
	for _, tc := range []struct {
		name   string
		change func(*Queue)
	}{
		{"node order", func(q *Queue) { q.primary[0], q.primary[1] = q.primary[1], q.primary[0] }},
		{"segment", func(q *Queue) { q.primary, q.secondary = q.secondary, q.primary }},
		{"secondary selected", func(q *Queue) { n := *q.secondary[0]; n.Param3++; q.secondary[0] = &n }},
		{"tick", func(q *Queue) { q.lastPumpTick++ }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			q := orderSummaryFixture()
			tc.change(q)
			if readOrderSummary(t, q) == baseline {
				t.Fatal("selected mutation invisible")
			}
		})
	}
	q := orderSummaryFixture()
	q.primary[0].Owner, q.primary[0].DynamicGate, q.primary[0].GuardX = 4, 5, 6
	q.detachedNode, q.detachedHasSuccessor = &Node{ID: 255, GoalX: 99}, true
	q.diagnostics = []string{"excluded"}
	q.danger.quietUntil, q.firingPosition.nextAttempt = 77, 88
	beforeA, beforeB := *q.primary[0], *q.primary[1]
	if readOrderSummary(t, q) != baseline {
		t.Fatal("full-only mutation changed summary")
	}
	if !reflect.DeepEqual(*q.primary[0], beforeA) || !reflect.DeepEqual(*q.primary[1], beforeB) || q.detachedNode == nil || !q.detachedHasSuccessor {
		t.Fatal("summary mutated queue")
	}
}

func TestCheckpointSummaryQueueMalformedAtomicAndNoAlloc(t *testing.T) {
	for _, secondary := range []bool{false, true} {
		q := orderSummaryFixture()
		path := "primary[1]"
		if secondary {
			q.secondary[0] = nil
			path = "secondary[0]"
		} else {
			q.primary[1] = nil
		}
		var out checkpoint.Summary
		out.Word(99)
		before := out
		err := q.AppendCheckpointSummary(&out)
		if err == nil || !strings.HasPrefix(err.Error(), "nanolathe:") || !strings.Contains(err.Error(), path) || out != before {
			t.Fatalf("malformed queue: error=%v, summary changed=%v", err, out != before)
		}
	}
	var out checkpoint.Summary
	if (*Queue)(nil).AppendCheckpointSummary(&out) == nil || (&Queue{}).AppendCheckpointSummary(nil) == nil {
		t.Fatal("nil accepted")
	}
	q := orderSummaryFixture()
	if got := testing.AllocsPerRun(100, func() {
		out = checkpoint.Summary{}
		if err := q.AppendCheckpointSummary(&out); err != nil {
			panic(err)
		}
	}); got != 0 {
		t.Fatalf("allocations = %v", got)
	}
}
