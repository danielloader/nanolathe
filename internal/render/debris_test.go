package render

import (
	"testing"

	"github.com/nanolathe-gg/nanolathe/internal/frame"
	"github.com/nanolathe-gg/nanolathe/internal/sim/numeric"
)

// debrisTestCRT is a presentation CRT stand-in that replays a scripted list of
// draws in order. It is not the retail recurrence: these tests lock the
// arithmetic applied to a draw, not the generator.
type debrisTestCRT struct {
	values []int32
	used   int
}

func (c *debrisTestCRT) Rand() int32 {
	if c.used >= len(c.values) {
		c.used++
		return 0
	}
	v := c.values[c.used]
	c.used++
	return v
}

// debrisTestCounts is a stand-in for the two bound entries' frame counts less
// one: twelve `smoke 1` frames and twenty `flamestream` frames in stock
// `fx.gaf` [03 R-FX-01 §3].
var debrisTestCounts = DebrisTrailFrameCounts{Smoke: 11, Flame: 19}

// TestDebrisTrailsMakeOneContainerPerSetBit locks the producer census of
// [04 R-COB-04 §2]: one smoke-puff container under the SMOKE bit, one
// flame-stream trail container under the FIRE bit, in that order, and nothing
// at all when neither is set. The families and the (bank, entry) pairs are
// [03 R-FX-01 §3]'s strip-9 rows, and each container carries its first
// sub-record from its own init.
func TestDebrisTrailsMakeOneContainerPerSetBit(t *testing.T) {
	for _, tc := range []struct {
		name         string
		smoke, fire  bool
		wantFamilies []frame.StripFamily
	}{
		{"neither", false, false, nil},
		{"smoke only", true, false, []frame.StripFamily{frame.StripFamilySmokePuff}},
		{"fire only", false, true, []frame.StripFamily{frame.StripFamilyFlameTrail}},
		{"both", true, true, []frame.StripFamily{frame.StripFamilySmokePuff, frame.StripFamilyFlameTrail}},
	} {
		v := frame.DebrisView{Smoke: tc.smoke, Fire: tc.fire}
		crt := &debrisTestCRT{}
		e := DebrisTrails(v, crt, 100, debrisTestCounts)
		if e.Count != len(tc.wantFamilies) {
			t.Fatalf("%s: %d containers, want %d", tc.name, e.Count, len(tc.wantFamilies))
		}
		var views []frame.StripView
		for i, want := range tc.wantFamilies {
			c := e.Containers[i]
			if c.Family != want {
				t.Fatalf("%s: container %d family %v, want %v", tc.name, i, c.Family, want)
			}
			if c.Count != 1 {
				t.Fatalf("%s: container %d holds %d sub-records at init, want 1: every family spawns once from its init [03 R-FX-01 §3]",
					tc.name, i, c.Count)
			}
			views = c.AppendViews(0, views)
		}
		for i := range views {
			if views[i].Strip != 9 {
				t.Fatalf("%s: view %d draws at strip %d, want 9 [03 R-FX-01 §3]", tc.name, i, views[i].Strip)
			}
			if views[i].Bank != DebrisTrailBank {
				t.Fatalf("%s: view %d bank %q, want %q", tc.name, i, views[i].Bank, DebrisTrailBank)
			}
			if views[i].Frame != 0 {
				t.Fatalf("%s: view %d starts at frame %d, want 0", tc.name, i, views[i].Frame)
			}
		}
		// One draw for the smoke puff's own last frame, four for the fire
		// particle's three jitter axes and its container life
		// [03 R-FX-01 §3][03 R-STRIP-01 §3].
		wantDraws := 0
		if tc.smoke {
			wantDraws++
		}
		if tc.fire {
			wantDraws += 4
		}
		if crt.used != wantDraws || e.CRTDraws != wantDraws {
			t.Fatalf("%s: %d/%d CRT draws, want %d", tc.name, crt.used, e.CRTDraws, wantDraws)
		}
	}
}

// TestDebrisFireParticleJitterAndLife locks the fire producer's arithmetic of
// [03 R-FX-01 §3]: three per-axis draws of `crtRand·3/0x8000 − 1` WHOLE units
// in X, Y, Z order, then a fourth of `crtRand·3/0x8000 + 1` for the container
// life. The smoke puff stays at the unjittered point, and its own draw comes
// FIRST — the smoke producer is bit 1 and runs before the fire producer.
func TestDebrisFireParticleJitterAndLife(t *testing.T) {
	base := frame.DebrisView{
		Smoke: true, Fire: true,
		X: numeric.FixedFromInt(100), Y: numeric.FixedFromInt(20), Z: numeric.FixedFromInt(300),
	}
	// The first value is the smoke puff's last-frame draw. Then 0 → -1,
	// 0x4000 → +0 (0x4000*3/0x8000 == 1, minus 1), 0x7fff → +1; the life draw
	// 0x7fff gives 3.
	crt := &debrisTestCRT{values: []int32{0, 0, 0x4000, 0x7fff, 0x7fff}}
	e := DebrisTrails(base, crt, 100, debrisTestCounts)
	if e.Count != 2 {
		t.Fatalf("%d containers, want 2", e.Count)
	}
	puff := e.Containers[0].Particles[0]
	if puff.X != base.X || puff.Y != base.Y || puff.Z != base.Z {
		t.Fatalf("smoke puff at (%v,%v,%v), want the piece's own point (%v,%v,%v)",
			puff.X, puff.Y, puff.Z, base.X, base.Y, base.Z)
	}
	fire := e.Containers[1]
	want := [3]numeric.Fixed{
		base.X - numeric.FixedFromInt(1),
		base.Y,
		base.Z + numeric.FixedFromInt(1),
	}
	if fire.Src[0] != want[0] || fire.Src[1] != want[1] || fire.Src[2] != want[2] {
		t.Fatalf("fire container source (%v,%v,%v), want (%v,%v,%v): jitter is crtRand·3/0x8000 − 1 whole units in X, Y, Z order",
			fire.Src[0], fire.Src[1], fire.Src[2], want[0], want[1], want[2])
	}
	if fire.Deadline != 100+3 {
		t.Fatalf("fire container deadline %d, want %d: life is crtRand·3/0x8000 + 1", fire.Deadline, 103)
	}
	// A == B, so every axis of the researched step is zero: a stationary flame
	// sprite [03 R-FX-01 §3].
	if fire.Travel != [3]numeric.Fixed{} {
		t.Fatalf("fire container travel %v, want zero on every axis: its source and target are the same point", fire.Travel)
	}
	if e.CRTDraws != 5 || crt.used != 5 {
		t.Fatalf("spent %d/%d draws, want 5: one for the puff's last frame and four for the fire particle", e.CRTDraws, crt.used)
	}
}

// TestDebrisTrailsWithoutAPresentationCRTMakeNothing keeps both producers
// honest when no presentation stream is bound: the puff's last frame and the
// particle's jitter have no substitute, so neither container is built rather
// than one being built with a substituted value [I4][I9].
func TestDebrisTrailsWithoutAPresentationCRTMakeNothing(t *testing.T) {
	e := DebrisTrails(frame.DebrisView{Smoke: true, Fire: true}, nil, 1, debrisTestCounts)
	if e.Count != 0 {
		t.Fatalf("built %d containers with no stream bound, want none", e.Count)
	}
}

// TestDebrisSmokePuffPersistsAndAnimates is the persistence contract of
// [03 R-FX-01 §3]: the smoke container is NOT a one-frame blit. Its puff holds
// a position on the strip list, the animation clock advances its cursor after
// the authored hold, the wind and gravity words drift the raw position words
// every tick, and the container retires only once the cursor reaches the puff's
// own drawn last frame.
func TestDebrisSmokePuffPersistsAndAnimates(t *testing.T) {
	const born = 500
	// A last-frame draw of 0x7fff gives the family's ceiling against a
	// twelve-frame `smoke 1`: trunc(0x7fff·9/0x8000) + 2 == 10, so the entry's
	// own last frame is never the puff's. Every later draw is the animation hold.
	crt := &debrisTestCRT{values: []int32{0x7fff}}
	e := DebrisTrails(frame.DebrisView{Smoke: true, X: numeric.FixedFromInt(10), Z: numeric.FixedFromInt(10)}, crt, born, debrisTestCounts)
	c := e.Containers[0]
	if c.Particles[0].LastFrame != 10 {
		t.Fatalf("puff last frame %d, want 10: crtRand·(frameCount − 3)/0x8000 + 2 [06 R-WFX-01 §5]", c.Particles[0].LastFrame)
	}

	const windX, windZ, gravity = 40, -24, 112
	startX, startY, startZ := c.Particles[0].X, c.Particles[0].Y, c.Particles[0].Z
	lastFrameSeen := int32(0)
	retiredAt := uint32(0)
	for tick := uint32(born + 1); tick <= born+400; tick++ {
		if c.Retired(tick) {
			retiredAt = tick
			break
		}
		c.Step(tick, windX, windZ, gravity, crt)
		if c.Count != 0 {
			lastFrameSeen = c.Particles[0].Frame
		}
	}
	if retiredAt == 0 {
		t.Fatal("the smoke container never retired: its puff's cursor must reach its own drawn last frame")
	}
	// The retirement is taken on the same pass that the cursor reaches the
	// drawn last frame, so the highest cursor any draw sees is one below it
	// [06 R-WFX-01 §5].
	if lastFrameSeen != 9 {
		t.Fatalf("the puff's cursor last drew frame %d, want 9: the clock must walk it up to one below its drawn last frame of 10", lastFrameSeen)
	}
	// One puff per container and no respawn: the window closed at the birth
	// tick, so the gate refuses every later spawn [03 R-FX-01 §3].
	if c.Count != 0 {
		t.Fatalf("%d sub-records survived retirement, want 0", c.Count)
	}
	// Drift is added to the RAW 16.16 words, so the puff moves a world unit or
	// two over its whole life and rises — it does not travel [03 R-FX-01 §3].
	if startX.Raw() == 0 && startZ.Raw() == 0 {
		t.Fatal("fixture error: the puff started at the origin, so drift cannot be told from it")
	}
	_ = startY
}

// TestDebrisSmokePuffDriftIsRawWords locks the three per-tick adds exactly:
// `x += windX·8`, `z += windZ·8` and `y += authoredGravity·4` against the RAW
// 16.16 words, the strips-5/9 class's scales and not the vent's
// [03 R-FX-01 §3][R-WIND-01].
func TestDebrisSmokePuffDriftIsRawWords(t *testing.T) {
	crt := &debrisTestCRT{values: []int32{0}}
	base := frame.DebrisView{Smoke: true, X: numeric.FixedFromInt(10), Y: numeric.FixedFromInt(4), Z: numeric.FixedFromInt(10)}
	e := DebrisTrails(base, crt, 1, debrisTestCounts)
	c := e.Containers[0]
	const windX, windZ, gravity = 40, -24, 112
	c.Step(2, windX, windZ, gravity, crt)
	p := c.Particles[0]
	if got, want := p.X.Raw(), base.X.Raw()+windX*8; got != want {
		t.Fatalf("puff X raw %d, want %d: windX·8 on the raw word", got, want)
	}
	if got, want := p.Z.Raw(), base.Z.Raw()+windZ*8; got != want {
		t.Fatalf("puff Z raw %d, want %d: windZ·8 on the raw word", got, want)
	}
	if got, want := p.Y.Raw(), base.Y.Raw()+gravity*4; got != want {
		t.Fatalf("puff Y raw %d, want %d: authoredGravity·4 on the raw word — the strips-5/9 scale, not the vent's 16", got, want)
	}
}

// TestDebrisFireContainerLaysLifetimePlusOneSegments locks the trail family's
// spawn gate: one segment from the init and one per tick while
// `nextSpawn ≤ deadline`, so `lifetime + 1` coincident segments in all, every
// one of them at a fresh copy of the source, all extinguished together the tick
// after the deadline. The family spends no draw while it runs
// [03 R-FX-01 §3][03 R-STRIP-01 §3].
func TestDebrisFireContainerLaysLifetimePlusOneSegments(t *testing.T) {
	const born = 40
	// The life draw 0x7fff gives 3, so four segments over ticks 40..43.
	crt := &debrisTestCRT{values: []int32{0x4000, 0x4000, 0x4000, 0x7fff}}
	e := DebrisTrails(frame.DebrisView{Fire: true, X: numeric.FixedFromInt(8), Z: numeric.FixedFromInt(9)}, crt, born, debrisTestCounts)
	c := e.Containers[0]
	spent := crt.used
	counts := []int{c.Count}
	frames := []int32{c.Particles[0].Frame}
	for tick := uint32(born + 1); tick <= born+5; tick++ {
		if c.Retired(tick) {
			counts = append(counts, -1)
			break
		}
		c.Step(tick, 40, -24, 112, crt)
		counts = append(counts, c.Count)
		if c.Count != 0 {
			frames = append(frames, c.Particles[0].Frame)
		}
	}
	want := []int{1, 2, 3, 4, 0, -1}
	if len(counts) != len(want) {
		t.Fatalf("segment census %v, want %v", counts, want)
	}
	for i := range want {
		if counts[i] != want[i] {
			t.Fatalf("segment census %v, want %v: lifetime + 1 segments, all expiring together the tick after the deadline", counts, want)
		}
	}
	if crt.used != spent {
		t.Fatalf("the trail family spent %d draws while it ran, want 0 [03 R-STRIP-01 §3]", crt.used-spent)
	}
	// hold 1, so the first segment's cursor advances every tick and wraps
	// modulo frameCount − 1, never showing the entry's last frame.
	wantFrames := []int32{0, 1, 2, 3}
	for i := range wantFrames {
		if frames[i] != wantFrames[i] {
			t.Fatalf("first segment's cursor walked %v, want %v: hold 1 advances it every tick [03 R-FX-01 §3]", frames, wantFrames)
		}
	}
}

// TestDebrisTrailWindMatchesThePublishedWords locks the drift source: the FIRST
// published word is −2·speed·sin(heading) and feeds world X, the SECOND is
// −2·speed·cos(heading) and feeds world Z [R-WIND-01]. A calm wind moves
// nothing.
func TestDebrisTrailWindMatchesThePublishedWords(t *testing.T) {
	if x, z := DebrisTrailWind(frame.WindView{Strength: 0, Heading: 0x4000}); x != 0 || z != 0 {
		t.Fatalf("a calm wind published (%d,%d), want (0,0)", x, z)
	}
	// Heading 0 is sin 0 / cos 1, so the first word is zero and the second is
	// the whole −2·speed term.
	x, z := DebrisTrailWind(frame.WindView{Strength: 50, Heading: 0})
	if x != 0 {
		t.Fatalf("wind X word %d at heading 0, want 0: the first word is the sine term", x)
	}
	if z != -2*numeric.MulRound(50, numeric.Cos(numeric.Angle(0))) {
		t.Fatalf("wind Z word %d at heading 0, want the −2·speed·cos term", z)
	}
}

// A presentation-owned trail container carries the same remaining-life hint the
// session's own strip views carry, so a burning piece's terrain pool leaves with
// its flame instead of switching off with the particle
// (DESIGN_GPU_RENDERER §31.7). Without it every burning piece reads as spent.
func TestDebrisTrailViewsCarryRemainingLife(t *testing.T) {
	c := DebrisTrailContainer{
		Family:   frame.StripFamilyFlameTrail,
		Entry:    DebrisFlameTrailEntry,
		Born:     10,
		Deadline: 13,
	}
	c.laySegment()
	// A sub-record with no deadline of its own falls back to the container's,
	// which is the case the smoke class reaches.
	c.Particles[c.Count] = DebrisTrailParticle{}
	c.Count++

	for _, tc := range []struct {
		tick    uint32
		want    uint32
		present bool
	}{
		{tick: 10, want: 3, present: true},
		{tick: 12, want: 1, present: true},
		// The last drawn tick: no ticks left, but the record still HAS a
		// deadline, which is what the fade's presence flag reads.
		{tick: 13, want: 0, present: true},
		{tick: 99, want: 0, present: true},
	} {
		views := c.AppendViews(tc.tick, nil)
		if len(views) != 2 {
			t.Fatalf("tick %d: %d views, want 2", tc.tick, len(views))
		}
		for i, v := range views {
			if v.HasRemaining != tc.present || v.Remaining != tc.want {
				t.Fatalf("tick %d view %d: remaining %d present %v, want %d present %v",
					tc.tick, i, v.Remaining, v.HasRemaining, tc.want, tc.present)
			}
		}
	}

	// A container with no deadline at all claims none, rather than publishing a
	// zero that would read as expiring.
	none := DebrisTrailContainer{Family: frame.StripFamilyFlameTrail, Entry: DebrisFlameTrailEntry}
	none.Particles[0] = DebrisTrailParticle{}
	none.Count = 1
	for _, v := range none.AppendViews(5, nil) {
		if v.HasRemaining {
			t.Fatalf("a container with no deadline published %d ticks remaining", v.Remaining)
		}
	}
}
