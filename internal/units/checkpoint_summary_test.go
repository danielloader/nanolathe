package units

import (
	"math"
	"reflect"
	"strings"
	"testing"

	"github.com/nanolathe-gg/nanolathe/internal/pool"
	"github.com/nanolathe-gg/nanolathe/internal/sim/checkpoint"
)

func unitSummaryFixture() *World {
	u := &Unit{Handle: 0xfffe, AllocationSerial: 0x100000001, Owner: 9,
		Health: -3, Remaining: math.Float32frombits(0x80000000), Flags: 0xf0000001,
		X: -0x100000001, Y: 0x100000002, Z: -9, Pending: 0x80000001,
		Stunned: true, ParalyzeExpire: 0xf0000002}
	u.Move.Heading, u.Move.Speed = 0xfffe, -0x100000003
	u.Slots[0].Reload, u.Slots[0].Ammo = -1, 2
	u.Slots[0].Aim.RestoreReadyWord(0x87654321)
	u.Slots[0].Aim.IssueBit, u.Slots[0].Aim.Ready = true, false
	u.Slots[1].Reload, u.Slots[1].Ammo, u.Slots[1].Aim.Ready = 3, -4, true
	u.Slots[2].Reload, u.Slots[2].Ammo = -5, -6
	raw := &Unit{Handle: 42, Owner: 4, Remaining: math.Float32frombits(0x3f000001), Kills: 999}
	return &World{units: []*Unit{nil, u, nil, nil}, rawUnits: []*Unit{nil, u, raw, nil},
		liveCounters:    [10]int{-1, 2, 0, 0, 0, 0, 0, 0, 0, -7},
		createdCounters: [10]uint32{0x80000001, 4, 0, 0, 0, 0, 0, 0, 0, 0xffffffff}}
}

func readUnitSummary(t *testing.T, w *World) checkpoint.Summary {
	t.Helper()
	var out checkpoint.Summary
	if err := w.AppendCheckpointSummary(&out); err != nil {
		t.Fatal(err)
	}
	return out
}

func TestCheckpointSummaryUnitLiteralWords(t *testing.T) {
	w := unitSummaryFixture()
	// Independently authored §16.3.77 words: empty, live, freed residual,
	// empty; then live/created pairs. No allocator or definition is installed.
	words := []uint64{37, 4, 0,
		1, 0xfffe, 0x100000001, 9, 0xfffffffffffffffd, 0x80000000, 0xf0000001,
		0xfffffffeffffffff, 0x100000002, 0xfffffffffffffff7,
		0xfffe, 0xfffffffefffffffd, 0x80000001, 1, 0xf0000002,
		0xffffffffffffffff, 2, 1, 0, 0x87654321,
		3, 0xfffffffffffffffc, 0, 1, 0,
		0xfffffffffffffffb, 0xfffffffffffffffa, 0, 0, 0,
		2, 42, 4, 0x3f000001, 0,
		0xffffffffffffffff, 0x80000001, 2, 4, 0, 0, 0, 0, 0, 0,
		0, 0, 0, 0, 0, 0, 0, 0, 0xfffffffffffffff9, 0xffffffff}
	var out checkpoint.Summary
	out.Word(37)
	if err := w.AppendCheckpointSummary(&out); err != nil {
		t.Fatal(err)
	}
	var sum uint64
	for i, word := range words {
		sum += uint64(i+1) * word
	}
	if count, got := out.Result(); count != uint64(len(words)) || got != sum {
		t.Fatalf("summary = (%d, %x), want (%d, %x)", count, got, len(words), sum)
	}
}

func TestCheckpointSummaryUnitSelectedAndExcluded(t *testing.T) {
	baseline := readUnitSummary(t, unitSummaryFixture())
	for _, tc := range []struct {
		name   string
		change func(*World)
	}{
		{"live health", func(w *World) { w.units[1].Health++ }},
		{"live position", func(w *World) { w.units[1].Z++ }},
		{"movement", func(w *World) { w.units[1].Move.Speed++ }},
		{"status", func(w *World) { w.units[1].Stunned = false }},
		{"last weapon", func(w *World) { w.units[1].Slots[2].Ammo++ }},
		{"raw readiness", func(w *World) { w.units[1].Slots[0].Aim.RestoreReadyWord(7); w.units[1].Slots[0].Aim.Ready = false }},
		{"residual", func(w *World) { w.rawUnits[2].Owner++ }},
		{"counter", func(w *World) { w.createdCounters[9]++ }},
		{"slot presence", func(w *World) { w.rawUnits[3] = &Unit{} }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := unitSummaryFixture()
			tc.change(w)
			if readUnitSummary(t, w) == baseline {
				t.Fatal("selected mutation did not change summary")
			}
		})
	}
	w := unitSummaryFixture()
	w.deathDispatches = 42
	w.units[1].Kills++
	w.units[1].MaxHealth++
	w.units[1].Slots[0].Flags++
	w.rawUnits[2].Kills++
	w.rawUnits[2].X++
	// Live storage wins; an unrelated raw record is not traversed or checked.
	w.rawUnits[1] = &Unit{Remaining: math.Float32frombits(0x7fc00001)}
	w.SetDeathHook(func(pool.Handle, DeathCause, *Unit) { panic("summary invoked death hook") })
	before := *w.units[1]
	if got := readUnitSummary(t, w); got != baseline {
		t.Fatal("full-only fields changed summary")
	}
	if !reflect.DeepEqual(*w.units[1], before) {
		t.Fatal("summary mutated live state")
	}
}

func TestCheckpointSummaryUnitNaNAndAtomicRefusal(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(*World)
		path   string
	}{
		{"live NaN", func(w *World) { w.units[1].Remaining = math.Float32frombits(0x7fc01234) }, "slots[1].Remaining"},
		{"residual NaN", func(w *World) { w.rawUnits[2].Remaining = math.Float32frombits(0xff800001) }, "slots[2].Remaining"},
		{"short residual storage", func(w *World) { w.rawUnits = w.rawUnits[:2] }, "summary.slots"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := unitSummaryFixture()
			tc.change(w)
			var out checkpoint.Summary
			out.Word(99)
			before := out
			err := w.AppendCheckpointSummary(&out)
			if err == nil || !strings.HasPrefix(err.Error(), "nanolathe:") || !strings.Contains(err.Error(), tc.path) {
				t.Fatalf("error = %v, want %s", err, tc.path)
			}
			if out != before {
				t.Fatal("failed append changed accumulator")
			}
		})
	}
	var out checkpoint.Summary
	if err := (*World)(nil).AppendCheckpointSummary(&out); err == nil {
		t.Fatal("nil world accepted")
	}
	if err := unitSummaryFixture().AppendCheckpointSummary(nil); err == nil {
		t.Fatal("nil accumulator accepted")
	}
}

func TestCheckpointSummaryUnitFloatBitsAndNoAlloc(t *testing.T) {
	// A freed slot needs only its four selected words. Both infinities and
	// signed zeros retain exact bits; they are not NaNs.
	for _, bits := range []uint32{0, 0x80000000, 0x7f800000, 0xff800000, 1} {
		w := &World{units: []*Unit{nil}, rawUnits: []*Unit{{Remaining: math.Float32frombits(bits)}}}
		out := readUnitSummary(t, w)
		count, sum := out.Result()
		if count != 25 || sum != 5+5*uint64(bits) {
			t.Fatalf("bits %x: (%d, %x)", bits, count, sum)
		}
	}
	w := unitSummaryFixture()
	var out checkpoint.Summary
	if got := testing.AllocsPerRun(100, func() {
		out = checkpoint.Summary{}
		if err := w.AppendCheckpointSummary(&out); err != nil {
			panic(err)
		}
	}); got != 0 {
		t.Fatalf("allocations = %v", got)
	}
}
