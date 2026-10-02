package render

import (
	"testing"

	"github.com/nanolathe-gg/nanolathe/internal/effects"
	"github.com/nanolathe-gg/nanolathe/internal/frame"
)

// TestFixedEffectPublishesPerPlayerLiveness locks the published liveness of the
// two embedded animation players [03 §1].
//
// The defect: a record survives until BOTH players are inactive, so a
// non-looping player that terminates first leaves its cursor at index 0 in a
// record that is still published every tick [03 §4.4]. Nothing on the view said
// the player was over, so the draw pass re-read index 0 as a live first frame —
// the finished layer started again under the one still playing. Liveness is now
// published per player and never inferred from the durations, the art name or
// the cursor index.
func TestFixedEffectPublishesPerPlayerLiveness(t *testing.T) {
	// A two-player record with unequal timing: the primary art runs two ticks
	// (two frames held one tick each) and the calculated flash six (three
	// frames held two ticks each).
	shortLong := frame.EffectView{
		ID: 1, Kind: "explosion", Graphic: "art", AssetID: "fx",
		HasCalculatedFlash: true, CalculatedTable: 0,
		DurationsA: []int32{1, 1},
		DurationsB: []int32{2, 2, 2},
	}
	longShort := shortLong
	longShort.DurationsA, longShort.DurationsB = shortLong.DurationsB, shortLong.DurationsA

	// (a) The primary finishes first and stops being drawn while the secondary
	// keeps the record alive.
	var p effects.FixedEffectPool
	if !p.AppendView(shortLong) {
		t.Fatal("two-player admission failed")
	}
	for tick := uint32(1); tick <= 2; tick++ {
		p.Update(tick)
	}
	views := p.SnapshotViews()
	if len(views) != 1 {
		t.Fatalf("the record retired while its secondary player was still running: %d", len(views))
	}
	if views[0].ActiveA {
		t.Fatal("the terminated primary player is still published as live")
	}
	if !views[0].ActiveB {
		t.Fatal("the running secondary player is published as dead")
	}
	if views[0].SeqA != 0 {
		t.Fatalf("terminated cursor = %d; index 0 is exactly why liveness cannot be inferred from it", views[0].SeqA)
	}
	if draws := BuildEffectDrawsInto(nil, views); len(draws) != 1 || draws[0].ActiveA || !draws[0].ActiveB {
		t.Fatalf("liveness did not reach the draw instruction: %+v", draws)
	}

	// (c) Both finishing retires the record, inside the same updater call.
	for tick := uint32(3); tick <= 6; tick++ {
		p.Update(tick)
	}
	if p.Len() != 0 {
		t.Fatalf("both players inactive but %d records survive [03 §1]", p.Len())
	}

	// (b) The inverse: the secondary finishes first and the primary keeps
	// playing.
	var q effects.FixedEffectPool
	if !q.AppendView(longShort) {
		t.Fatal("two-player admission failed")
	}
	for tick := uint32(1); tick <= 2; tick++ {
		q.Update(tick)
	}
	views = q.SnapshotViews()
	if len(views) != 1 {
		t.Fatalf("the record retired while its primary player was still running: %d", len(views))
	}
	if views[0].ActiveB {
		t.Fatal("the terminated secondary player is still published as live")
	}
	if !views[0].ActiveA {
		t.Fatal("the running primary player is published as dead")
	}

	// (d) A layer with no player of its own draws nothing. A named entry whose
	// authored timing never resolved activates no player, and nothing is
	// fabricated to stand in for it [I9]: liveness is the sequence pointer
	// alone, not "has art" and not "has durations". The impact shape that used
	// to land here — art beside a calculated flash — no longer does, because
	// the timing lookup is per player [06 R-WFX-01 §2]; see
	// TestEffectServiceResolvesPrimaryTimingBesideACalculatedFlash.
	var r effects.FixedEffectPool
	if !r.AppendView(frame.EffectView{
		ID: 2, Kind: "explosion", Graphic: "art", AssetID: "fx",
		HasCalculatedFlash: true, CalculatedTable: 0,
		DurationsB: []int32{2, 2, 2},
	}) {
		t.Fatal("unresolved-art admission failed")
	}
	r.Update(1)
	views = r.SnapshotViews()
	if len(views) != 1 || !views[0].ActiveB {
		t.Fatalf("the flash player should still be running: %+v", views)
	}
	if views[0].ActiveA {
		t.Fatal("art whose timing never resolved published a live player")
	}
	for tick := uint32(2); tick <= 6; tick++ {
		r.Update(tick)
	}
	if r.Len() != 0 {
		t.Fatalf("the flash ended but %d records survive", r.Len())
	}

	// (e) A looping player never terminates, so its layer stays live.
	var s effects.FixedEffectPool
	if !s.AppendView(frame.EffectView{
		ID: 3, Kind: "smokestart", Graphic: "smoke 1",
		DurationsA: []int32{1, 1}, LoopA: true,
	}) {
		t.Fatal("looping admission failed")
	}
	for tick := uint32(1); tick <= 20; tick++ {
		s.Update(tick)
		views = s.SnapshotViews()
		if len(views) != 1 || !views[0].ActiveA {
			t.Fatalf("a looping player went dead at tick %d: %+v", tick, views)
		}
	}
}
