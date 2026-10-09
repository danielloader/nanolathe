package session

import (
	"reflect"
	"slices"
	"strings"
	"testing"
)

func checkpointComparisonFixture() CheckpointHistory {
	var history CheckpointHistory
	for tick := uint32(30); tick <= 60; tick++ {
		position := CheckpointPosition{Tick: tick, Boundary: CheckpointFinalPumpTick, Pump: uint64(tick + 3), ConsumedInput: 9}
		row := CheckpointRingRow{Position: position, SimulationState: 17, CRTState: 19, SimulationDraws: uint64(tick), CRTDraws: 200,
			UnitCount: 2, ProjectileCount: 3, EffectCount: 4, FragmentCount: 5, DebrisCount: 6, StripCount: 7}
		row.Owners[6] = OwnerSummary{Words: 4, Sum: 123}
		history.Ticks = append(history.Ticks, row)
		if tick%30 == 0 {
			history.Records = append(history.Records, CheckpointRecord{Position: position})
		}
	}
	return history
}

func cloneComparisonHistory(history CheckpointHistory) CheckpointHistory {
	return CheckpointHistory{Records: slices.Clone(history.Records), Ticks: slices.Clone(history.Ticks)}
}

func compareCheckpointTestHistories(t *testing.T, a, b CheckpointHistory) CheckpointComparison {
	t.Helper()
	out, err := CompareCheckpointHistories(a, b)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// The selected fault starts between two digest samples. Full and cheap evidence
// remain independently located; scanning continues after the first difference.
func TestCheckpointComparisonLocatesSelectedFaultBetweenDigests(t *testing.T) {
	a := checkpointComparisonFixture()
	b := cloneComparisonHistory(a)
	for i := 11; i < len(b.Ticks); i++ {
		b.Ticks[i].Owners[6].Sum++
		b.Ticks[i].CRTDraws++
		b.Ticks[i].FragmentCount++
	}
	b.Records[1].Digests.Full[31] = 1
	b.Records[1].Digests.Owners[6][0] = 2
	beforeA, beforeB := cloneComparisonHistory(a), cloneComparisonHistory(b)
	out := compareCheckpointTestHistories(t, a, b)
	wantTick := &CheckpointDifference{Position: a.Ticks[11].Position, RNG: true, Pools: true}
	wantTick.Owners[6] = true
	wantRecord := &CheckpointDifference{Position: a.Records[1].Position, Full: true}
	wantRecord.Owners[6] = true
	want := CheckpointComparison{ComparedTicks: 31, ComparedRecords: 2, TickDifference: wantTick, RecordDifference: wantRecord}
	if !reflect.DeepEqual(out, want) {
		t.Fatalf("comparison = %+v, tick=%+v record=%+v; want %+v", out, out.TickDifference, out.RecordDifference, want)
	}
	if !reflect.DeepEqual(a, beforeA) || !reflect.DeepEqual(b, beforeB) || !reflect.DeepEqual(out, compareCheckpointTestHistories(t, a, b)) {
		t.Fatal("comparison mutated input or changed on repetition")
	}
	out.TickDifference.Owners[6] = false
	if got := compareCheckpointTestHistories(t, a, b); !got.TickDifference.Owners[6] {
		t.Fatal("returned difference borrowed later results")
	}
}

func TestCheckpointComparisonPreservesDigestBlindSpotEvidence(t *testing.T) {
	a := checkpointComparisonFixture()
	b := cloneComparisonHistory(a)
	b.Records[0].Digests.Full[0] = 7
	b.Records[0].Digests.Owners[2][0] = 3
	b.Records[1].Digests.Owners[9][0] = 4
	out := compareCheckpointTestHistories(t, a, b)
	want := CheckpointComparison{ComparedTicks: 31, ComparedRecords: 2,
		RecordDifference: &CheckpointDifference{Position: a.Records[0].Position, Full: true}}
	want.RecordDifference.Owners[2] = true
	want.UncoveredOwners[2], want.UncoveredOwners[9] = true, true
	if !reflect.DeepEqual(out, want) {
		t.Fatalf("blind-spot evidence = %+v, record=%+v", out, out.RecordDifference)
	}
	// A full-only difference cannot name an owner, while an owner digest is
	// still evidence even if the supplied full digest happens to be equal.
	for _, full := range []bool{false, true} {
		b = cloneComparisonHistory(a)
		if full {
			b.Records[0].Digests.Full[0]++
		} else {
			b.Records[0].Digests.Owners[12][0]++
		}
		out = compareCheckpointTestHistories(t, a, b)
		if out.RecordDifference == nil || out.RecordDifference.Full != full || out.RecordDifference.Owners[12] == full || out.UncoveredOwners[12] == full {
			t.Fatalf("independent digest evidence lost: %+v", out)
		}
	}
	// Equal summaries at a nearby tick, or a different pump at the digest's
	// tick, establish no same-position blind spot.
	b = cloneComparisonHistory(a)
	b.Records[0].Digests.Owners[2][0]++
	a.Ticks = a.Ticks[1:]
	b.Ticks[0].Position.Pump--
	out = compareCheckpointTestHistories(t, a, b)
	if out.UncoveredOwners != ([CheckpointOwnerCount]bool{}) || out.RecordDifference == nil {
		t.Fatal("inferred an uncovered owner without common same-position rows")
	}
}

func TestCheckpointComparisonSelectedEvidence(t *testing.T) {
	for _, tc := range []struct {
		name              string
		change            func(*CheckpointRingRow)
		rng, pools, owner bool
	}{
		{"simulation state", func(r *CheckpointRingRow) { r.SimulationState++ }, true, false, false},
		{"CRT state", func(r *CheckpointRingRow) { r.CRTState++ }, true, false, false},
		{"simulation draws", func(r *CheckpointRingRow) { r.SimulationDraws++ }, true, false, false},
		{"CRT draws", func(r *CheckpointRingRow) { r.CRTDraws++ }, true, false, false},
		{"units", func(r *CheckpointRingRow) { r.UnitCount++ }, false, true, false},
		{"projectiles", func(r *CheckpointRingRow) { r.ProjectileCount++ }, false, true, false},
		{"effects", func(r *CheckpointRingRow) { r.EffectCount++ }, false, true, false},
		{"fragments", func(r *CheckpointRingRow) { r.FragmentCount++ }, false, true, false},
		{"debris", func(r *CheckpointRingRow) { r.DebrisCount++ }, false, true, false},
		{"strips", func(r *CheckpointRingRow) { r.StripCount++ }, false, true, false},
		{"summary words", func(r *CheckpointRingRow) { r.Owners[0].Words++ }, false, false, true},
		{"summary sum", func(r *CheckpointRingRow) { r.Owners[0].Sum++ }, false, false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a := CheckpointHistory{Ticks: checkpointComparisonFixture().Ticks[:1]}
			b := cloneComparisonHistory(a)
			tc.change(&b.Ticks[0])
			out := compareCheckpointTestHistories(t, a, b)
			want := &CheckpointDifference{Position: a.Ticks[0].Position, RNG: tc.rng, Pools: tc.pools}
			want.Owners[0] = tc.owner
			if !reflect.DeepEqual(out.TickDifference, want) || !out.MayPredateTicks || out.ComparedTicks != 1 || out.RecordDifference != nil {
				t.Fatalf("selected evidence = %+v, tick=%+v", out, out.TickDifference)
			}
		})
	}
}

func TestCheckpointComparisonRequiresExactPositions(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(*CheckpointPosition)
	}{
		{"tick", func(p *CheckpointPosition) { p.Tick++ }},
		{"boundary", func(p *CheckpointPosition) { p.Boundary = CheckpointInteriorTick }},
		{"pump", func(p *CheckpointPosition) { p.Pump++ }},
		{"input", func(p *CheckpointPosition) { p.ConsumedInput++ }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a := checkpointComparisonFixture()
			a.Ticks, a.Records = a.Ticks[:1], a.Records[:1]
			b := cloneComparisonHistory(a)
			tc.change(&b.Ticks[0].Position)
			tc.change(&b.Records[0].Position)
			b.Ticks[0].SimulationState++
			b.Records[0].Digests.Full[0]++
			if out := compareCheckpointTestHistories(t, a, b); out != (CheckpointComparison{}) {
				t.Fatalf("incompatible positions diagnosed as divergence: %+v", out)
			}
		})
	}
	if out := compareCheckpointTestHistories(t, CheckpointHistory{}, CheckpointHistory{}); out != (CheckpointComparison{}) {
		t.Fatal("empty histories imply evidence")
	}
	a := checkpointComparisonFixture()
	if out := compareCheckpointTestHistories(t, a, a); out.ComparedTicks != 31 || out.ComparedRecords != 2 || out.TickDifference != nil || out.RecordDifference != nil {
		t.Fatalf("equal retained evidence: %+v", out)
	}
}

func TestCheckpointComparisonTruncationAndTickWrap(t *testing.T) {
	var a CheckpointHistory
	for i := uint64(0); i < 6; i++ {
		p := CheckpointPosition{Tick: uint32(i - 3), Boundary: CheckpointInteriorTick, Pump: 50 + i/3, ConsumedInput: i}
		if i%3 == 2 {
			p.Boundary = CheckpointFinalPumpTick
		}
		a.Ticks = append(a.Ticks, CheckpointRingRow{Position: p})
		a.Records = append(a.Records, CheckpointRecord{Position: p})
	}
	b := cloneComparisonHistory(a)
	a.Ticks, a.Records = a.Ticks[:5], a.Records[:5]
	b.Ticks, b.Records = b.Ticks[2:], b.Records[2:]
	b.Ticks[0].Owners[1].Sum++
	b.Records[1].Digests.Owners[1][0]++
	out := compareCheckpointTestHistories(t, a, b)
	if out.ComparedTicks != 3 || out.ComparedRecords != 3 || !out.MayPredateTicks ||
		out.TickDifference.Position.Tick != ^uint32(0) || out.RecordDifference.Position.Tick != 0 || !out.UncoveredOwners[1] {
		t.Fatalf("truncated wrapped evidence reordered: %+v", out)
	}
	// A preceding unmatched difference does not count as a common onset.
	b.Ticks[0].Owners[1].Sum = 0
	b.Ticks[1].Owners[1].Sum++
	a.Ticks[0].Owners[1].Sum++
	out = compareCheckpointTestHistories(t, a, b)
	if out.MayPredateTicks || out.TickDifference.Position.Tick != 0 || out.UncoveredOwners[1] {
		t.Fatal("used unmatched row or nearby equal summary as onset evidence")
	}
}

func TestCheckpointComparisonRejectsMalformedHistories(t *testing.T) {
	base := checkpointComparisonFixture()
	for _, tc := range []struct {
		name   string
		change func(*CheckpointHistory)
	}{
		{"row bound", func(h *CheckpointHistory) { h.Ticks = make([]CheckpointRingRow, 601) }},
		{"record bound", func(h *CheckpointHistory) { h.Records = make([]CheckpointRecord, 65) }},
		{"duplicate row", func(h *CheckpointHistory) { h.Ticks[1].Position = h.Ticks[0].Position }},
		{"duplicate record", func(h *CheckpointHistory) { h.Records[1].Position = h.Records[0].Position }},
		{"same tick different position", func(h *CheckpointHistory) { h.Ticks[1].Position.Tick = h.Ticks[0].Position.Tick }},
		{"repeated nonadjacent tick", func(h *CheckpointHistory) {
			h.Ticks = h.Ticks[:4]
			for i, tick := range []uint32{30, 0x6000001e, 0xc000001e, 30} {
				h.Ticks[i].Position.Tick = tick
			}
		}},
		{"row boundary zero", func(h *CheckpointHistory) { h.Ticks[0].Position.Boundary = 0 }},
		{"record boundary unknown", func(h *CheckpointHistory) { h.Records[0].Position.Boundary = 4 }},
		{"entry row", func(h *CheckpointHistory) { h.Ticks[0].Position = CheckpointPosition{Boundary: CheckpointEntry} }},
		{"entry later", func(h *CheckpointHistory) { h.Records[1].Position = CheckpointPosition{Boundary: CheckpointEntry} }},
		{"entry tick", func(h *CheckpointHistory) {
			h.Records[0].Position = CheckpointPosition{Boundary: CheckpointEntry, Tick: 1}
		}},
		{"entry pump", func(h *CheckpointHistory) {
			h.Records[0].Position = CheckpointPosition{Boundary: CheckpointEntry, Pump: 1}
		}},
		{"entry input", func(h *CheckpointHistory) {
			h.Records[0].Position = CheckpointPosition{Boundary: CheckpointEntry, ConsumedInput: 1}
		}},
		{"reverse rows", func(h *CheckpointHistory) { slices.Reverse(h.Ticks) }},
		{"reverse records", func(h *CheckpointHistory) { slices.Reverse(h.Records) }},
		{"half-range jump", func(h *CheckpointHistory) { h.Records[1].Position.Tick = h.Records[0].Position.Tick + 1<<31 }},
		{"row pump decreases", func(h *CheckpointHistory) { h.Ticks[1].Position.Pump = 0 }},
		{"record pump decreases", func(h *CheckpointHistory) { h.Records[1].Position.Pump = 0 }},
		{"row input decreases", func(h *CheckpointHistory) { h.Ticks[1].Position.ConsumedInput = 0 }},
		{"record input decreases", func(h *CheckpointHistory) { h.Records[1].Position.ConsumedInput = 0 }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			bad := cloneComparisonHistory(base)
			tc.change(&bad)
			for _, pair := range [][2]CheckpointHistory{{bad, base}, {base, bad}} {
				out, err := CompareCheckpointHistories(pair[0], pair[1])
				if err == nil || !strings.HasPrefix(err.Error(), "nanolathe: checkpoint comparison failed: logical path histories[") || out != (CheckpointComparison{}) {
					t.Fatalf("malformed history returned evidence: %+v, %v", out, err)
				}
			}
		})
	}
}

func TestCheckpointComparisonEntryAndBounds(t *testing.T) {
	var history CheckpointHistory
	for i := 0; i < 600; i++ {
		p := CheckpointPosition{Tick: uint32(i + 1), Boundary: CheckpointFinalPumpTick, Pump: uint64(i + 1)}
		history.Ticks = append(history.Ticks, CheckpointRingRow{Position: p})
		if i < 63 {
			history.Records = append(history.Records, CheckpointRecord{Position: p})
		}
	}
	history.Records = append([]CheckpointRecord{{Position: CheckpointPosition{Boundary: CheckpointEntry}}}, history.Records...)
	out := compareCheckpointTestHistories(t, history, history)
	if out.ComparedTicks != 600 || out.ComparedRecords != 64 || out.TickDifference != nil || out.RecordDifference != nil {
		t.Fatalf("valid bounds/entry rejected: %+v", out)
	}
}
