package gpurender

import "github.com/nanolathe-gg/nanolathe/internal/drawlist"

// setMaterials and setScorch are the executor gates of the Finish and Scorch
// switches (GPU design §29, §30). They never change simulation or saved state.
func (r *Renderer) setMaterials(enabled bool) { r.materialsEnabled = enabled }
func (r *Renderer) setScorch(enabled bool)    { r.scorchEnabled = enabled }

// SetEffects applies the player's Enhanced effect selection (§30). It is the
// single entry point to every executor gate: each gate is set from its own
// switch and from nothing else, so a treatment is on exactly when its switch
// is, and no switch turns another's treatment off. A pass that serves several
// switches — the water surface pass, the light gather — runs whenever any of
// them is on and gates each of its lanes on that lane's own switch. The
// per-switch setters below it are package-internal.
//
// Nothing here allocates, compiles or submits; each assignment is read by the
// owning pass on its next frame. A source reset preserves these switches, so an
// executor swap or a new battle keeps the selection that was last applied.
func (r *Renderer) SetEffects(e drawlist.Effects) {
	if r == nil {
		return
	}
	for _, percent := range []*int{&e.WeaponGlowStrength, &e.ExplosionGlowStrength, &e.NanoGlowStrength, &e.ShadowSoftness} {
		*percent = min(max(*percent, 0), EffectStrengthMax)
	}
	r.effects = e
	r.weaponGlowOffset = effectStrengthOffset(e.WeaponGlowStrength)
	r.explosionGlowOffset = effectStrengthOffset(e.ExplosionGlowStrength)
	r.nanoGlowOffset = effectStrengthOffset(e.NanoGlowStrength)
	r.aircraftShadow.softnessOffset = effectStrengthOffset(e.ShadowSoftness)
	// Water: the surface shading (§26.3, §32.3); the moving field and the
	// underwater refraction (§26.5); shore and building foam; dry hover
	// land wash; and the screen-space reflections (§26.4, §32.2).
	r.setWaterSurface(e.WaterSurface)
	r.setWaterMotion(e.WaterMotion)
	r.setWaterFoam(e.WaterFoam)
	r.setHovercraftLandWash(e.HovercraftLandWash)
	r.setWaterReflections(e.WaterReflections)
	// Light on models and smoke (§23), and the ground pools it reaches
	// (§31.3, §31.6); one gather serves both.
	r.setModelLight(e.ModelLight)
	r.setGroundLight(e.GroundLight)
	// The metal/paint finishes (§29.1) and the metallic glint (§23.7).
	r.setMaterials(e.Finish)
	r.setMetalGlint(e.Glint)
	// Heat: the blast rings (§25), the vegetation heat shimmer (§27) and the
	// wreck shimmer (§28). The three share one refraction pass (§27.2), and
	// each admits its own sources. The wreck glow (§28) is the recorded
	// cooling colour and the wreck light that borrows it.
	r.setBlastDistortion(e.BlastRings)
	r.setTreeHeat(e.FireShimmer)
	r.setWreckGlow(e.WreckGlow)
	r.setWreckShimmer(e.WreckShimmer)
	// The fading scorch layer (§29.2). The trail layer is governed by its
	// recorded strength alone, so it has no executor switch.
	r.setScorch(e.Scorch)
	// The aircraft soft shadows (§34).
	r.setSoftShadows(e.SoftShadows)
	// The model subjects' coverage resolve (§17).
	r.setSupersample(e.Supersample)
}

// setSupersample is the executor half of the Supersample switch (§17.5). Off,
// the body, projected-shadow, silhouette-shadow and underwater commits resolve
// each pixel from the one texel at its block's top left rather than the four
// under it, so an edge pixel is covered or not and a face boundary inside the
// subject is not averaged. That texel is the sample the native raster takes:
// the lane biases its corners half a texel so a texel is covered exactly when
// the span writer covers its corner point, and a block's top-left corner is
// the native pixel's own. The recorder's half (no doubled lane, retail's
// anchor) makes the corners native too; this half alone still gives whole-pixel
// edges for a host that gates the executor only. The 2× page, the key and
// colour passes and the outline are unchanged.
func (r *Renderer) setSupersample(on bool) { r.modelSingleSample = !on }

// modelSampleLane is the commit lane that selects the single-sample resolve.
func (r *Renderer) modelSampleLane() float32 {
	if r.modelSingleSample {
		return 1
	}
	return 0
}

// setWreckGlow is the executor half of the wreck glow switch (§28, §30). The
// recorder writes a fresh wreck's cooling emission only under the switch, but a
// host may keep its recorder on every effect and apply the player's selection
// here alone — the Nanolathe screen's preview does, so its histories survive a
// compare — so the executor refuses a recorded emission as well: off, the body
// commit composes no cooling colour and the wreck lends the battle light none.
func (r *Renderer) setWreckGlow(on bool) { r.wreckGlowDisabled = !on }

// wreckEmission is a packet's fresh-wreck cooling colour as the executor
// composes it: the recorded emission, or none with the wreck glow off.
func (r *Renderer) wreckEmission(g *drawlist.ModelGeometry) [3]float32 {
	if r.wreckGlowDisabled || g == nil {
		return [3]float32{}
	}
	return g.WreckEmission
}

// EffectStrengthDefault and EffectStrengthMax are the percentages of
// SetGroundLightStrength and SetBlastRingStrength (§30): 100 is the tuned look,
// 0 draws none, and 200 is the most a player may ask for. The scale is linear.
const (
	EffectStrengthDefault = 100
	EffectStrengthMax     = 200
)

// effectStrengthOffset is a strength percentage as its linear scale of the
// tuned look less one, so the zero value of the field it is stored in is the
// tuned look: a renderer that is never handed a strength draws the default.
func effectStrengthOffset(percent int) float32 {
	return float32(min(max(percent, 0), EffectStrengthMax))/EffectStrengthDefault - 1
}

// SetGroundLightStrength scales the ground light pools and the short terrain
// flash (§31.3, §31.6) as a percentage of the tuned look, clamped to
// 0..EffectStrengthMax; 0 draws no pool and skips the pass, as the ground light
// switch off does (§30). It multiplies each pool's terrain gain and nothing
// else, so models, smoke and the glow layer are untouched. Like the glow
// strength it is set by the host beside SetEffects on every present, and a
// source reset keeps it.
func (r *Renderer) SetGroundLightStrength(percent int) {
	if r == nil {
		return
	}
	r.lighting.groundStrengthOffset = effectStrengthOffset(percent)
}

// SetBlastRingStrength scales the blast ring distortion amplitude (§25) as a
// percentage of the tuned look, clamped to 0..EffectStrengthMax; 0 admits no
// ring, as the blast rings switch off does (§30). Each ring keeps its own
// radius, width and shot-dependent strength; this is a multiplier on top.
func (r *Renderer) SetBlastRingStrength(percent int) {
	if r == nil {
		return
	}
	r.distortion.ringStrengthOffset = effectStrengthOffset(percent)
}

// Effects returns the last selection SetEffects applied. New applies the
// all-on construction default, so this is never the zero value on a renderer
// the constructor produced.
func (r *Renderer) Effects() drawlist.Effects {
	if r == nil {
		return drawlist.Effects{}
	}
	return r.effects
}
