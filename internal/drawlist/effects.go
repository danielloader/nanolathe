package drawlist

// Effects is the player's Enhanced presentation switch set
// (docs/DESIGN_GPU_RENDERER.md §30). It is a plain value so the recorder and
// the modern executor can hold and compare the same selection without either
// importing the settings package; the shell converts the persisted integers.
//
// Every switch is independent and toggles exactly one visible treatment: no
// switch contains another, and none is off because some other switch is. The
// fields mirror the persisted switches one for one. The two methods below are
// not switches: they name the inputs several treatments share, so the recorder
// and the executor agree on when a shared input has to be produced.
//
// Every field is a modern presentation choice. Classic records and composes the
// same pixels whatever the set says, and no field reaches simulation state [I6].
type Effects struct {
	// WaterSurface is the Enhanced surface shading (§26.3, §32.3): the ripple
	// shade and crest tint with their depth ramp, the damp shoreline band and
	// the shallow tint. Off, the painted water keeps its original colours.
	WaterSurface bool
	// WaterMotion is the moving surface: the drifting, churning field with its
	// gusts and the displacement it applies to the painted seabed, the
	// refraction of submerged hulls (§26.5), and the ripple of reflections and
	// aircraft shadows on water. Off, the surface is still — the field frozen
	// at phase zero, the seabed undisplaced.
	WaterMotion bool
	// WaterFoam is shore foam, building foam and wet foam marks
	// (§26.1, §26.3).
	WaterFoam bool
	// HovercraftLandWash is the existing dry-ground hovercraft spray (§26.3).
	// It uses the shared wet/dry mask, independently of water treatments.
	HovercraftLandWash bool
	// WaterReflections is the screen-space reflections of models and
	// projectiles (§26.4, §26.6) and the reflected explosions of §32.2.
	WaterReflections bool

	// ModelLight is explosion, fire, projectile, wreck and nanolathe light on
	// models and smoke (§23, §31.1).
	ModelLight bool
	// GroundLight is the ground pools that light reaches and the short
	// terrain flash (§31.3, §31.6).
	GroundLight bool

	// Glow amounts scale the player's source families independently (§19.4).
	// WeaponGlowStrength covers beams, lightning and projectile body sprites;
	// ExplosionGlowStrength covers effect and strip sprites, flashes and halos.
	// NanoGlowStrength scales both the spray's glow and its local illumination.
	// Percentages are 0..200, with 100 the tuned look; zero emits nothing.
	WeaponGlowStrength, ExplosionGlowStrength, NanoGlowStrength int

	// Finish is the metal/paint material finishes (§29.1).
	Finish bool
	// Glint is the metallic glint (§23.7).
	Glint bool

	// BlastRings is the explosion distortion rings (§25).
	BlastRings bool
	// FireShimmer is the burning-vegetation heat shimmer (§27).
	FireShimmer bool
	// WreckGlow is the fresh wreck's cooling emission colour — the red tint
	// that fades as it cools — and the wreck light that borrows it (§28, §31.1).
	// The arriving commander's glow reuses it (§36). Both halves gate it: the
	// recorder writes no emission, and the executor composes and lends none.
	// The death's own explosion and fire art are content and follow no switch.
	WreckGlow bool
	// WreckShimmer is the heat-wave distortion plume above a fresh wreck (§28),
	// which the arriving commander's shimmer reuses (§36).
	WreckShimmer bool

	// Scorch is the fading scorch marks and the arrival landing scar that
	// shares their layer (§29.2, §36). The trail layer of footprints and
	// tracks (§15) has no switch here: its strength percentage alone governs
	// it, and zero is off.
	Scorch bool

	// SoftShadows is the aircraft soft shadows of §34. Off, an aircraft takes
	// the ordinary silhouette shadow route every other mobile subject takes.
	SoftShadows bool
	// ShadowSoftness scales the altitude-dependent aircraft filter radius
	// (§34), 0..200, with 100 the tuned look. Zero keeps the ordinary shadow.
	ShadowSoftness int

	// Supersample is the model subjects' 2× raster and coverage resolve of
	// §17. Off, the recorder builds no doubled lane — the subject keeps
	// retail's anchor and native corners, as with the Anti-Alias option off —
	// and the executor commits each pixel from the one texel at its block's
	// top left, the sample the native raster takes, so edges are whole pixels
	// with no coverage blend (§17.5).
	Supersample bool
}

// AllEffects is the default selection: every switch on and every amount at 100.
func AllEffects() Effects {
	return Effects{
		WaterSurface: true, WaterMotion: true, WaterFoam: true, HovercraftLandWash: true, WaterReflections: true,
		ModelLight: true, GroundLight: true,
		WeaponGlowStrength: 100, ExplosionGlowStrength: 100, NanoGlowStrength: 100,
		Finish: true, Glint: true,
		BlastRings: true, FireShimmer: true, WreckGlow: true, WreckShimmer: true,
		Scorch:      true,
		SoftShadows: true, ShadowSoftness: 100,
		Supersample: true,
	}
}

// WaterPhase reports whether any water treatment is on. Every one of them reads
// the recorded water phase and the water-motion history behind it — the
// surface shading's damp-band pulse, the moving field, the foam's clock and
// wind energy, the reflections' ripple and softening — so the phase is
// recorded, and the history kept, whenever one of them is drawn (§26.1).
func (e Effects) WaterPhase() bool {
	return e.WaterSurface || e.WaterMotion || e.WaterFoam || e.WaterReflections
}

// SeabedTreated reports whether the surface pass shades or moves the seabed,
// which is when short submerged decals are promoted beneath it so the pass
// treats them with the terrain they lie on (§26.3). Foam alone leaves them in
// their ordinary order.
func (e Effects) SeabedTreated() bool { return e.WaterSurface || e.WaterMotion }
