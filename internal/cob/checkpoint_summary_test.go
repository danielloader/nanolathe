package cob

import (
	"slices"
	"testing"

	"github.com/nanolathe-gg/nanolathe/internal/sim/checkpoint"
)

func cobSummaryFixture() *VM {
	v := &VM{statics: []int32{-1, 0, 0x7fffffff}, activeThreadCount: 3,
		nextIdentity: 0x100000001, threadIdentity: [8]uint64{9, 8, 7, 6, 5, 4, 3, 0x100000002}}
	v.Threads[0] = Thread{Status: -1, PC: 0x100000001, SP: 2, Sleep: -3,
		WaitPiece: 4, WaitAxis: -5, WaitThread: -1, SignalMask: -2147483648,
		Stack: [32]int32{1, -2, 3, -4, 5, -6, 7, -8, 9, -10, 11, -12,
			13, -14, 15, -16, 17, -18, 19, -20, 21, -22, 23, -24,
			25, -26, 27, -28, 29, -30, 31, -32}}
	// The last physical thread is idle with retained words beyond SP.
	v.Threads[7] = Thread{PC: -0x100000002, Sleep: 123, WaitThread: -1,
		Stack: [32]int32{31: -17}}
	return v
}

func readCOBSummary(t *testing.T, v *VM) checkpoint.Summary {
	t.Helper()
	var out checkpoint.Summary
	if err := v.AppendCheckpointSummary(&out); err != nil {
		t.Fatal(err)
	}
	return out
}

func TestCheckpointSummaryVMLiteralWords(t *testing.T) {
	v := cobSummaryFixture()
	// Eight independently authored, fixed-width physical rows. Zero rows are
	// still present, and stack payload is not clipped by SP (§16.3.77).
	rows := [8][40]uint64{
		{0xffffffffffffffff, 0x100000001, 2, 0xfffffffffffffffd,
			4, 0xfffffffffffffffb, 0xffffffffffffffff, 0xffffffff80000000,
			1, 0xfffffffffffffffe, 3, 0xfffffffffffffffc,
			5, 0xfffffffffffffffa, 7, 0xfffffffffffffff8,
			9, 0xfffffffffffffff6, 11, 0xfffffffffffffff4,
			13, 0xfffffffffffffff2, 15, 0xfffffffffffffff0,
			17, 0xffffffffffffffee, 19, 0xffffffffffffffec,
			21, 0xffffffffffffffea, 23, 0xffffffffffffffe8,
			25, 0xffffffffffffffe6, 27, 0xffffffffffffffe4,
			29, 0xffffffffffffffe2, 31, 0xffffffffffffffe0},
		{}, {}, {}, {}, {}, {},
		{0, 0xfffffffefffffffe, 0, 123, 0, 0, 0xffffffffffffffff, 0,
			39: 0xffffffffffffffef},
	}
	tail := []uint64{3, 0xffffffffffffffff, 0, 0x7fffffff, 3, 0x100000001,
		9, 8, 7, 6, 5, 4, 3, 0x100000002}
	var out checkpoint.Summary
	out.Word(37)
	if err := v.AppendCheckpointSummary(&out); err != nil {
		t.Fatal(err)
	}
	count, sum := uint64(1), uint64(37)
	for _, row := range rows {
		for _, word := range row {
			count++
			sum += count * word
		}
	}
	for _, word := range tail {
		count++
		sum += count * word
	}
	if gotCount, gotSum := out.Result(); gotCount != count || gotSum != sum {
		t.Fatalf("summary = (%d, %x), want (%d, %x)", gotCount, gotSum, count, sum)
	}
}

func TestCheckpointSummaryVMSelectedExcludedAndPure(t *testing.T) {
	baseline := readCOBSummary(t, cobSummaryFixture())
	for _, tc := range []struct {
		name   string
		change func(*VM)
	}{
		{"idle stack residual", func(v *VM) { v.Threads[7].Stack[31]++ }},
		{"wait", func(v *VM) { v.Threads[3].WaitThread-- }},
		{"static", func(v *VM) { v.statics[2]++ }},
		{"active count", func(v *VM) { v.activeThreadCount++ }},
		{"next identity", func(v *VM) { v.nextIdentity++ }},
		{"idle identity", func(v *VM) { v.threadIdentity[7]++ }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			v := cobSummaryFixture()
			tc.change(v)
			if readCOBSummary(t, v) == baseline {
				t.Fatal("selected mutation invisible")
			}
		})
	}
	v := cobSummaryFixture()
	v.prog = &Program{}
	v.tickDenom = 7
	v.lastReturnValue[0], v.lastReturnValid[0] = 9, true
	v.onReturn[0] = func(int32) { panic("summary invoked callback") }
	v.portFuncs = map[Port]func([]int32) int32{1: func([]int32) int32 { panic("summary invoked port") }}
	v.diagnostics = []string{"excluded"}
	threads, identities, statics := v.Threads, v.threadIdentity, slices.Clone(v.statics)
	if readCOBSummary(t, v) != baseline {
		t.Fatal("full-only mutation changed summary")
	}
	if v.Threads != threads || v.threadIdentity != identities || !slices.Equal(v.statics, statics) || !v.lastReturnValid[0] {
		t.Fatal("summary mutated VM")
	}
}

func TestCheckpointSummaryAimRawWord(t *testing.T) {
	for _, tc := range []struct {
		aim   AimSlot
		words [3]uint64
	}{
		{AimSlot{}, [3]uint64{0, 0, 0}},
		{AimSlot{IssueBit: true, Ready: false, readyWord: 0xabcdef01}, [3]uint64{1, 0, 0xabcdef01}},
		{AimSlot{Ready: true}, [3]uint64{0, 1, 0}},
		{AimSlot{IssueBit: true, Ready: true, readyWord: 7}, [3]uint64{1, 1, 7}},
	} {
		before := tc.aim
		var out checkpoint.Summary
		out.Word(37)
		if err := tc.aim.AppendCheckpointSummary(&out); err != nil {
			t.Fatal(err)
		}
		sum := uint64(37)
		for i, word := range tc.words {
			sum += uint64(i+2) * word
		}
		if count, got := out.Result(); count != 4 || got != sum || tc.aim != before {
			t.Fatalf("aim summary = (%d,%x), want (4,%x), state=%+v", count, got, sum, tc.aim)
		}
	}
}

func TestCheckpointSummaryCOBNilAndNoAlloc(t *testing.T) {
	var out checkpoint.Summary
	out.Word(99)
	before := out
	if err := (*VM)(nil).AppendCheckpointSummary(&out); err == nil || out != before {
		t.Fatal("nil VM accepted or changed summary")
	}
	if err := (*AimSlot)(nil).AppendCheckpointSummary(&out); err == nil || out != before {
		t.Fatal("nil aim accepted or changed summary")
	}
	v, aim := cobSummaryFixture(), &AimSlot{IssueBit: true, Ready: true, readyWord: 23}
	if v.AppendCheckpointSummary(nil) == nil || aim.AppendCheckpointSummary(nil) == nil {
		t.Fatal("nil accumulator accepted")
	}
	if got := testing.AllocsPerRun(100, func() {
		out = checkpoint.Summary{}
		if err := v.AppendCheckpointSummary(&out); err != nil {
			panic(err)
		}
		if err := aim.AppendCheckpointSummary(&out); err != nil {
			panic(err)
		}
	}); got != 0 {
		t.Fatalf("allocations = %v", got)
	}
}
