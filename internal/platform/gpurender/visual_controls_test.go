package gpurender

import (
	"testing"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/nanolathe-gg/nanolathe/formats"
	"github.com/nanolathe-gg/nanolathe/internal/drawlist"
)

// gates reports each executor gate's state, in the order of the controls
// table of GPU design §30.
func gates(r *Renderer) map[string]bool {
	return map[string]bool{
		"waterSurface":       !r.water.surfaceDisabled,
		"waterMotion":        !r.water.motionDisabled,
		"waterFoam":          !r.water.foamDisabled,
		"hovercraftLandWash": !r.water.landWashDisabled,
		"reflections":        !r.reflections.disabled,
		"modelLight":         !r.lighting.modelDisabled,
		"groundLight":        !r.lighting.groundDisabled,
		"materials":          r.materialsEnabled,
		"glint":              r.metalGlint,
		"blast":              !r.distortion.blastDisabled,
		"treeHeat":           !r.heat.treeDisabled,
		"wreckGlow":          !r.wreckGlowDisabled,
		"wreckShimmer":       !r.heat.wreckDisabled,
		"scorch":             r.scorchEnabled,
		"softShadows":        !r.aircraftShadow.disabled,
		"supersample":        !r.modelSingleSample,
	}
}

// setLights turns both light switches together — the whole gather — for the
// fixtures that compare a lit frame with an unlit one.
func (r *Renderer) setLights(on bool) {
	r.setModelLight(on)
	r.setGroundLight(on)
}

// The player's Enhanced switches are the whole executor surface (§30): every
// gate is on exactly when its switch is, with no second control and no
// environment override beside it. None of this is retail behaviour.
func TestSetEffectsIsTheOnlyGate(t *testing.T) {
	r := &Renderer{}
	r.SetEffects(drawlist.AllEffects())
	for name, on := range gates(r) {
		if !on {
			t.Fatalf("all effects on left %s off", name)
		}
	}
	off := drawlist.Effects{}
	r.SetEffects(off)
	for name, on := range gates(r) {
		if on {
			t.Fatalf("all effects off left %s on", name)
		}
	}
	if r.Effects() != off {
		t.Fatalf("Effects() = %+v after an all-off selection", r.Effects())
	}
}

// Each switch owns exactly one gate: turning one off, from all on or from all
// off, moves only its own, so no switch can silently take another with it.
func TestEachSwitchOwnsOneGate(t *testing.T) {
	for _, tc := range []struct {
		name string
		flip func(*drawlist.Effects, bool)
		gate string
	}{
		{"waterSurface", func(e *drawlist.Effects, on bool) { e.WaterSurface = on }, "waterSurface"},
		{"waterMotion", func(e *drawlist.Effects, on bool) { e.WaterMotion = on }, "waterMotion"},
		{"waterFoam", func(e *drawlist.Effects, on bool) { e.WaterFoam = on }, "waterFoam"},
		{"hovercraftLandWash", func(e *drawlist.Effects, on bool) { e.HovercraftLandWash = on }, "hovercraftLandWash"},
		{"waterReflections", func(e *drawlist.Effects, on bool) { e.WaterReflections = on }, "reflections"},
		{"modelLight", func(e *drawlist.Effects, on bool) { e.ModelLight = on }, "modelLight"},
		{"groundLight", func(e *drawlist.Effects, on bool) { e.GroundLight = on }, "groundLight"},
		{"finish", func(e *drawlist.Effects, on bool) { e.Finish = on }, "materials"},
		{"glint", func(e *drawlist.Effects, on bool) { e.Glint = on }, "glint"},
		{"blastRings", func(e *drawlist.Effects, on bool) { e.BlastRings = on }, "blast"},
		{"fireShimmer", func(e *drawlist.Effects, on bool) { e.FireShimmer = on }, "treeHeat"},
		{"wreckGlow", func(e *drawlist.Effects, on bool) { e.WreckGlow = on }, "wreckGlow"},
		{"wreckShimmer", func(e *drawlist.Effects, on bool) { e.WreckShimmer = on }, "wreckShimmer"},
		{"scorch", func(e *drawlist.Effects, on bool) { e.Scorch = on }, "scorch"},
		{"softShadows", func(e *drawlist.Effects, on bool) { e.SoftShadows = on }, "softShadows"},
		{"supersample", func(e *drawlist.Effects, on bool) { e.Supersample = on }, "supersample"},
	} {
		for _, base := range []bool{true, false} {
			e := drawlist.Effects{}
			if base {
				e = drawlist.AllEffects()
			}
			tc.flip(&e, !base)
			r := &Renderer{}
			r.SetEffects(e)
			for name, on := range gates(r) {
				want := base
				if name == tc.gate {
					want = !base
				}
				if on != want {
					t.Fatalf("%s set to %v over all %v: %s on=%v", tc.name, !base, base, name, on)
				}
			}
		}
	}
}

// A source reset retires images, never the applied selection.
func TestSetEffectsSurvivesSourceReset(t *testing.T) {
	r := &Renderer{}
	sel := drawlist.Effects{ModelLight: true, WaterMotion: true}
	r.SetEffects(sel)
	r.resetSources(func(*ebiten.Image) {})
	if r.Effects() != sel || !r.water.surfaceDisabled || r.water.motionDisabled || !r.water.foamDisabled ||
		!r.heat.treeDisabled || !r.heat.wreckDisabled || !r.wreckGlowDisabled || !r.modelSingleSample || r.lighting.modelDisabled || !r.lighting.groundDisabled || !r.aircraftShadow.disabled {
		t.Fatalf("a source reset lost the applied selection: %+v", r.Effects())
	}
}

// The ground light and blast ring strengths (§30) are linear percentages of
// the tuned look, clamped to 0..EffectStrengthMax, stored so that a renderer
// never handed one draws the default. They are not effect switches: a
// selection leaves them alone and a source reset keeps them.
func TestEffectStrengthsScaleClampAndPersist(t *testing.T) {
	r := &Renderer{}
	if r.lighting.groundStrengthOffset != 0 || r.distortion.ringStrengthOffset != 0 {
		t.Fatal("a zero renderer is not at the tuned strengths")
	}
	for _, tc := range []struct {
		percent int
		scale   float32
	}{{100, 1}, {0, 0}, {-20, 0}, {50, 0.5}, {200, 2}, {900, 2}} {
		r.SetGroundLightStrength(tc.percent)
		r.SetBlastRingStrength(tc.percent)
		if got := 1 + r.lighting.groundStrengthOffset; got != tc.scale {
			t.Fatalf("ground light %d%%: scale %v, want %v", tc.percent, got, tc.scale)
		}
		if got := 1 + r.distortion.ringStrengthOffset; got != tc.scale {
			t.Fatalf("blast ring %d%%: scale %v, want %v", tc.percent, got, tc.scale)
		}
	}
	r.SetGroundLightStrength(150)
	r.SetBlastRingStrength(25)
	r.SetEffects(drawlist.Effects{})
	r.SetEffects(drawlist.AllEffects())
	r.resetSources(func(*ebiten.Image) {})
	if 1+r.lighting.groundStrengthOffset != 1.5 || 1+r.distortion.ringStrengthOffset != 0.25 {
		t.Fatal("a selection or a source reset moved the strengths")
	}
}

// The ring strength multiplies every admitted ring's amplitude, and 0 admits
// none; the ground strength multiplies every pool's terrain gain, and 0 builds
// no pool, so the pass copies and submits nothing.
func TestEffectStrengthsReachRingsAndPools(t *testing.T) {
	art := &formats.GAFFrame{Width: 64, Height: 64}
	var list drawlist.List
	list.RecordLightSource(drawlist.Sprite{Frame: art, X: 160, Y: 120, LightingKind: drawlist.SpriteLightingExplosion, LightingScale: 1, BlastSize: 64, BlastAge: 4})
	r := &Renderer{w: 320, h: 240}
	ring := func(percent int) (int, float32) {
		r.SetBlastRingStrength(percent)
		r.prepareBlastDistortion(&list)
		if len(r.distortion.candidates) == 0 {
			return 0, 0
		}
		return len(r.distortion.candidates), r.distortion.candidates[0].strength
	}
	n, base := ring(EffectStrengthDefault)
	if n != 1 || base <= 0 {
		t.Fatalf("default ring strength admitted %d rings of strength %v", n, base)
	}
	if n, s := ring(200); n != 1 || s != 2*base {
		t.Fatalf("ring strength 200: %d rings of %v, want %v", n, s, 2*base)
	}
	if n, s := ring(50); n != 1 || s != base/2 {
		t.Fatalf("ring strength 50: %d rings of %v, want %v", n, s, base/2)
	}
	if n, _ := ring(0); n != 0 {
		t.Fatalf("ring strength 0 admitted %d rings", n)
	}

	pool := func(percent int) (int, float32) {
		r.SetGroundLightStrength(percent)
		r.lighting.lights = []battleLight{{position: [3]float32{160, 120, 0}, color: [3]float32{1, 0.5, 0.25}, radius: 60, kind: lightProjectile}}
		r.modelStats.GroundLights = 0
		r.appendGroundLights()
		if len(r.ground.verts) == 0 {
			return r.modelStats.GroundLights, 0
		}
		return r.modelStats.GroundLights, r.ground.verts[0].ColorR
	}
	n, lit := pool(EffectStrengthDefault)
	if n != 1 || lit <= 0 {
		t.Fatalf("default ground strength built %d pools of %v", n, lit)
	}
	if n, got := pool(200); n != 1 || got != 2*lit {
		t.Fatalf("ground strength 200: %d pools of %v, want %v", n, got, 2*lit)
	}
	if n, _ := pool(0); n != 0 {
		t.Fatalf("ground strength 0 built %d pools", n)
	}
	r.modelStats.GroundLights = 0
	r.drawGroundLighting()
	if r.modelStats.GroundLights != 0 {
		t.Fatal("ground strength 0 reached the ground pass")
	}
}
