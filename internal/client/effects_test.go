package client

// The player's Enhanced effect switches (DESIGN_GPU_RENDERER §30). These are
// Nanolathe presentation rules, not retail behaviour: retail has none of these
// effects and the classic executor composes the same pixels either way.

import (
	"testing"

	"github.com/nanolathe-gg/nanolathe/internal/drawlist"
	"github.com/nanolathe-gg/nanolathe/internal/frame"
	"github.com/nanolathe-gg/nanolathe/internal/render"
	"github.com/nanolathe-gg/nanolathe/internal/sim/numeric"
)

func withoutEffect(set func(*drawlist.Effects)) drawlist.Effects {
	e := drawlist.AllEffects()
	set(&e)
	return e
}

// A new client presents every effect. The trail layer has no switch of its
// own: a zero trail strength is its off, laying no marks, and a change to or
// from zero retires the history the way a switch does, so a layer turned back
// on starts from the current tick instead of replaying marks never drawn.
func TestTrailStrengthZeroGatesTrailHistory(t *testing.T) {
	c := trailScene(t)
	c.SetEnhanced(true)
	if c.Effects() != drawlist.AllEffects() {
		t.Fatalf("new client effects = %+v", c.Effects())
	}
	strength := c.TrailStrength()
	if strength == 0 {
		t.Fatal("new client lays no trails")
	}

	c.SetTrailStrength(0)
	publishWalker(t, c, 1, 40)
	c.ObserveCommittedTick()
	publishWalker(t, c, 2, 65)
	c.ObserveCommittedTick()
	if len(c.trails.marks) != 0 {
		t.Fatalf("trail strength 0 laid %d trail marks", len(c.trails.marks))
	}

	c.SetTrailStrength(strength)
	publishWalker(t, c, 3, 90)
	c.ObserveCommittedTick()
	publishWalker(t, c, 4, 115)
	c.ObserveCommittedTick()
	laid := len(c.trails.marks)
	if laid == 0 {
		t.Fatal("trail strength back on laid no trail marks")
	}
	// A change between two non-zero strengths keeps the marks: only their
	// recorded opacity changes.
	c.SetTrailStrength(strength / 2)
	if len(c.trails.marks) != laid {
		t.Fatalf("a non-zero strength change dropped marks: %d of %d", len(c.trails.marks), laid)
	}
	// Zero retires the history instead of freezing it for the next time the
	// layer is turned on.
	c.SetTrailStrength(0)
	if len(c.trails.marks) != 0 {
		t.Fatalf("trail strength 0 kept %d of %d marks", len(c.trails.marks), laid)
	}
}

// Every water switch reads the recorded water phase, so any one of them keeps
// it; with all four off no phase is recorded and no reflection site admitted.
func TestWaterPhaseFollowsAnyWaterSwitch(t *testing.T) {
	c := reflectionScene(t)
	x, z := numeric.FixedFromInt(16), numeric.FixedFromInt(16)
	if !c.reflectionWaterAt(x, z) {
		t.Fatal("scene does not sit over reflective water")
	}
	if !c.waterSurfaceMetadata().Enabled {
		t.Fatal("scene records no water phase")
	}
	allOff := withoutEffect(func(e *drawlist.Effects) {
		e.WaterSurface, e.WaterMotion, e.WaterFoam, e.WaterReflections = false, false, false, false
	})
	c.SetEffects(allOff)
	if c.reflectionWaterAt(x, z) || c.waterSurfaceMetadata().Enabled {
		t.Fatal("every water switch off still recorded a phase or a reflection site")
	}
	for _, tc := range []struct {
		name string
		set  func(*drawlist.Effects)
	}{
		{"surface", func(e *drawlist.Effects) { e.WaterSurface = true }},
		{"motion", func(e *drawlist.Effects) { e.WaterMotion = true }},
		{"foam", func(e *drawlist.Effects) { e.WaterFoam = true }},
		{"reflections", func(e *drawlist.Effects) { e.WaterReflections = true }},
	} {
		e := allOff
		tc.set(&e)
		c.SetEffects(e)
		if !c.waterSurfaceMetadata().Enabled {
			t.Fatalf("%s alone recorded no water phase", tc.name)
		}
	}
}

// Each water switch gates only its own producer: reflections close their site
// admission and nothing else, and the surface, motion and foam switches leave
// reflection admission alone. Seabed promotion follows the switches that have
// the surface pass treat the seabed — shading or motion — and foam alone
// leaves the decals in their ordinary order.
func TestWaterSwitchesGateOnlyTheirProducers(t *testing.T) {
	c := reflectionScene(t)
	x, z := numeric.FixedFromInt(16), numeric.FixedFromInt(16)
	c.SetEffects(withoutEffect(func(e *drawlist.Effects) { e.WaterReflections = false }))
	if c.reflectionWaterAt(x, z) {
		t.Fatal("reflections off still admitted a reflection site")
	}
	if !c.waterSurfaceMetadata().Enabled {
		t.Fatal("reflections off closed the water phase")
	}
	c.SetEffects(withoutEffect(func(e *drawlist.Effects) { e.WaterSurface, e.WaterMotion, e.WaterFoam = false, false, false }))
	if !c.reflectionWaterAt(x, z) || !c.waterSurfaceMetadata().Enabled {
		t.Fatal("surface, motion and foam off closed reflections or the phase")
	}
	for _, tc := range []struct {
		name string
		e    drawlist.Effects
		want bool
	}{
		{"all on", drawlist.AllEffects(), true},
		{"surface only", drawlist.Effects{WaterSurface: true}, true},
		{"motion only", drawlist.Effects{WaterMotion: true}, true},
		{"foam only", drawlist.Effects{WaterFoam: true}, false},
		{"reflections only", drawlist.Effects{WaterReflections: true}, false},
	} {
		if got := tc.e.SeabedTreated(); got != tc.want {
			t.Fatalf("%s: seabed promotion %v, want %v", tc.name, got, tc.want)
		}
	}
}

// The scorch switch leaves the trail layer alone, and a change to any switch
// retires the histories.
func TestSwitchChangesRetireHistories(t *testing.T) {
	c := trailScene(t)
	c.SetEnhanced(true)
	c.SetEffects(withoutEffect(func(e *drawlist.Effects) { e.Scorch = false }))
	publishWalker(t, c, 1, 40)
	c.ObserveCommittedTick()
	publishWalker(t, c, 2, 65)
	c.ObserveCommittedTick()
	laid := len(c.trails.marks)
	if laid == 0 {
		t.Fatal("Scorch off stopped the trail layer")
	}
	c.SetEffects(withoutEffect(func(e *drawlist.Effects) { e.Scorch, e.WaterFoam = false, false }))
	if len(c.trails.marks) != 0 {
		t.Fatalf("turning water foam off kept %d of %d marks", len(c.trails.marks), laid)
	}
}

// The soft shadow switch records no clearance, which is what selects the
// ordinary silhouette route in the executor (§34).
func TestSoftShadowsSwitchRecordsNoClearance(t *testing.T) {
	c := reflectionScene(t)
	g := &drawlist.ModelGeometry{Supersample: &drawlist.ModelGeometry{}}
	draw := &render.UnitDraw{Airborne: true, WorldPos: [3]numeric.Fixed{0, 140 * numeric.FixedOne, 0}}
	c.setModelLightingHeight(g, draw)
	if g.AircraftShadowHeight <= 0 {
		t.Fatal("scene records no aircraft clearance")
	}
	c.SetEffects(withoutEffect(func(e *drawlist.Effects) { e.SoftShadows = false }))
	c.setModelLightingHeight(g, draw)
	if g.AircraftShadowHeight != 0 || g.Supersample.AircraftShadowHeight != 0 {
		t.Fatalf("soft shadows off kept clearance %v", g.AircraftShadowHeight)
	}
}

// The heat switches each gate their own metadata: the burning-feature heat
// tag, the wreck's cooling emission and its shimmer are independent of each
// other and of the rings.
func TestHeatPartsGateTheirOwnMetadata(t *testing.T) {
	heatTag := func(e drawlist.Effects) bool {
		c, _, _ := newFeatureRasterClient(t)
		buf := &frame.Buffer{}
		*buf.BeginWrite() = *stripTestFrame(true)
		if err := buf.Publish(42); err != nil {
			t.Fatal(err)
		}
		c.buffer, c.enhanced, c.frameTick = buf, true, 42
		c.SetEffects(e)
		f := featureRasterView()
		f.RuntimeLive, f.ShadowEnabled, f.IsBurning = true, true, true
		f.EventSeqName, f.EventSeqNameShad = "event-body", "event-shadow"
		c.drawFeature(&f)
		found := false
		c.list.VisitSprites(func(sp drawlist.Sprite) { found = found || sp.HeatSource })
		return found
	}
	wreck := func(e drawlist.Effects) drawlist.ModelGeometry {
		c, _, _ := newFeatureRasterClient(t)
		c.buffer = frame.NewBuffer()
		*c.buffer.BeginWrite() = *stripTestFrame(true)
		if err := c.buffer.Publish(1); err != nil {
			t.Fatal(err)
		}
		c.enhanced = true
		c.SetEffects(e)
		f := featureRasterView()
		f.WreckHeatKnown, f.WreckBornTick = true, 0
		var g drawlist.ModelGeometry
		c.applyWreckHeat(&g, f)
		return g
	}
	for _, tc := range []struct {
		name                string
		clear               func(*drawlist.Effects)
		heat, glow, shimmer bool
	}{
		{"all on", func(*drawlist.Effects) {}, true, true, true},
		{"rings off", func(e *drawlist.Effects) { e.BlastRings = false }, true, true, true},
		{"fire off", func(e *drawlist.Effects) { e.FireShimmer = false }, false, true, true},
		{"wreck glow off", func(e *drawlist.Effects) { e.WreckGlow = false }, true, false, true},
		{"wreck shimmer off", func(e *drawlist.Effects) { e.WreckShimmer = false }, true, true, false},
		{"all heat off", func(e *drawlist.Effects) {
			e.BlastRings, e.FireShimmer, e.WreckGlow, e.WreckShimmer = false, false, false, false
		}, false, false, false},
	} {
		e := withoutEffect(tc.clear)
		if got := heatTag(e); got != tc.heat {
			t.Fatalf("%s: heat tag = %v, want %v", tc.name, got, tc.heat)
		}
		g := wreck(e)
		if got := g.WreckEmission != ([3]float32{}); got != tc.glow {
			t.Fatalf("%s: wreck emission = %v, want %v", tc.name, got, tc.glow)
		}
		if got := g.WreckHeatStrength > 0; got != tc.shimmer {
			t.Fatalf("%s: wreck shimmer = %v, want %v", tc.name, got, tc.shimmer)
		}
		// The view scale is shared: the wreck light's reach reads it with the
		// glow alone, the plume's size with the shimmer alone.
		if got := g.WreckHeatScale > 0; got != (tc.glow || tc.shimmer) {
			t.Fatalf("%s: wreck scale = %v", tc.name, g.WreckHeatScale)
		}
	}
}
