package main

import (
	"fmt"
	"strings"

	"github.com/nanolathe-gg/nanolathe/internal/content"
	"github.com/nanolathe-gg/nanolathe/internal/gameplay"
	"github.com/nanolathe-gg/nanolathe/internal/input"
	"github.com/nanolathe-gg/nanolathe/internal/settings"
)

// The Nanolathe screen's catalogue: pages of cards, each card one setting
// with its values, words, preview scene and the draft field it edits
// (docs/DESIGN_INTERFACE_HUD_INPUT.md §3.17). A new switch is a new card
// here; the layout takes any number of them.

// nlDraft is the screen's working copy. Nothing reaches the shell until
// Apply, except that the live preview always shows the draft.
type nlDraft struct {
	gameplay     gameplay.Mode
	mod          int // 0 is the original game, then the installed mods in order
	override     bool
	pres         settings.Presentation
	glow         int
	glowStrength int
	mutators     content.Mutators
	unitLimit    int
	fullscreen   bool
	switchAlt    bool
	controls     int // index into nlControlsPresets; 0 keeps the current rows
	// keys is the draft keyboard map the Controls page edits; Apply hands a
	// copy to the shell (DESIGN_INTERFACE_HUD_INPUT §3.6 "Rebinding").
	keys          *input.KeyMap
	interfaceType int
}

type nlKind int

const (
	nlMeter   nlKind = iota // a row of lamps; the first is Off
	nlSwitch                // an Off/On throw switch
	nlGroup                 // several related settings on one card
	nlStepper               // ◄ value ► with a notch per value
	nlLayers                // the three rule layers
	nlHalves                // two or three big choices side by side
	nlContent               // the mod chooser
)

type nlCard struct {
	key   string
	label string
	pics  []string // unit pictures for the card, first that exists wins
	kind  nlKind
	steps []string
	subs  []string // per-step sub caption for halves and layers
	get   func(d *nlDraft) int
	set   func(d *nlDraft, v int)
	desc  func(d *nlDraft, v int) string
	chips []string
	// scene is the preview preset for value v.
	scene func(d *nlDraft, v int) string
	// render applies value v to a frame's parameters.
	render func(d *nlDraft, v int, r *nlRender)
	// compare is the value the split compare shows beside v.
	compare func(d *nlDraft, v int) (int, bool)
	// usesRules and usesMutators put the draft's rules and mutators into
	// the scene, because they change the simulation.
	usesRules    bool
	usesMutators bool
	enhanced     bool // shows nothing under Classic
	// parts are a grouped card's settings (groupCard).
	parts []nlPart
	// details lists, for value v, what it changes, drawn under the
	// description in two columns.
	details func(d *nlDraft, v int) []string
	// demo names the pointer demonstration drawn over the preview
	// (nlscreen_demo.go), for controls a battle alone cannot show.
	demo string
	// blink shows the compare as the whole frame alternating between the
	// two values instead of a split, for an effect too fine to find by
	// looking from one half to the other.
	blink bool
}

type nlPage struct {
	key, title string
	cards      []nlCard
}

var nlControlsPresets = []struct{ label, preset, sub string }{
	{"As set", "", "Keeps every control as it is now."},
	{"Retail 3.1", controlsPresetRetail, "The original game's controls and interface."},
	{"Community", controlsPresetCommunity, "The TA community patch layout ProTA recommends."},
	{"TA Zero", controlsPresetZero, "TA Zero's selection rules and megamap."},
}

func onOff(v bool) int {
	if v {
		return 1
	}
	return 0
}

func (s *nlScreen) pages() []nlPage {
	return []nlPage{
		{key: "game", title: "Game", cards: s.gameCards()},
		{key: "mutators", title: "Mutators", cards: s.mutatorCards()},
		{key: "graphics", title: "Graphics", cards: s.graphicsCards()},
		{key: "effects", title: "Effects", cards: s.effectCards()},
		{key: "controls", title: "Controls", cards: s.controlCards()},
	}
}

// ---------------------------------------------------------------- GAME

var nlRuleModes = []gameplay.Mode{gameplay.Strict31, gameplay.Community39, gameplay.Modern}

func (s *nlScreen) gameCards() []nlCard {
	modNames := []string{"Total Annihilation"}
	for _, m := range s.mods {
		modNames = append(modNames, m.Name+" "+m.Version)
	}
	return []nlCard{
		{
			key: "content", label: "Content", pics: []string{"armcom", "corcom"}, kind: nlContent, steps: modNames,
			get: func(d *nlDraft) int { return d.mod },
			set: func(d *nlDraft, v int) {
				d.mod = v
				d.override = s.shell().lockOverridden(s.modAt(v))
				if min, ok := modMinimumGameplay(s.modAt(v)); ok && gameplayBelow(d.gameplay, min) && !d.override {
					d.gameplay = min
				}
			},
			desc: func(d *nlDraft, v int) string {
				m := s.modAt(v)
				if m == nil {
					return "The original game with no mod mounted. Every rule set is open. Switching content reloads the game data."
				}
				text := m.Summary
				if text == "" {
					text = m.Name
				}
				if min, ok := modMinimumGameplay(m); ok {
					text += " Needs " + gameplayLabel(min) + " rules or newer."
				}
				if m.Controls != "" {
					text += " Brings its recommended controls unless you choose a profile on the Controls page."
				}
				return text + " Switching content reloads the game data."
			},
			scene: func(*nlDraft, int) string { return "armor" },
		},
		{
			key: "rules", label: "Rules", pics: []string{"armck", "armcv"}, kind: nlLayers,
			steps: []string{"Strict 3.1", "Community 3.9", "Modern"},
			subs:  []string{"Retail TotalA.exe, exactly", "Adds the community patch fixes", "Adds Nanolathe's own policies"},
			get: func(d *nlDraft) int {
				return gameplayOptionStage(d.gameplay)
			},
			set: func(d *nlDraft, v int) { d.gameplay = nlRuleModes[v] },
			desc: func(d *nlDraft, v int) string {
				return [...]string{
					"The original game exactly as TotalA.exe 3.1 plays it, known faults included.",
					"The original game plus the TA community patch's fixes and larger limits, as ProTA, TA Zero and Escalation play.",
					"Community 3.9 plus Nanolathe's own fixes for movement, building and targeting. Each layer includes the ones beneath it.",
				}[v]
			},
			// DESIGN_COMMUNITY_PATCH §2–§4 and the Modern policies listed in
			// CLAUDE.md, in player terms.
			details: func(d *nlDraft, v int) []string {
				return [][]string{{
					"Original limits: short paths, 300 shots",
					"No building where your own units stand",
					"Structures always face one way",
					"Shots can hit a hill in the way",
					"Units meeting head-on stop and wait",
					"The reference for every test",
				}, {
					"Long paths, far more shots and effects",
					"Build under units: they step aside",
					"Guarding builders hold position",
					"Structures can rotate",
					"Splash hits every unit in a crowd",
					"New weapon keys and veterancy",
				}, {
					"No shots wasted into terrain",
					"Hold Fire always holds fire",
					"Units clear factory exits and sites",
					"Group moves spread, on straight paths",
					"Friendly traffic jams always clear",
					"Aircraft queue for repair pads",
				}}[v]
			},
			scene:     func(*nlDraft, int) string { return "march" },
			usesRules: true,
		},
		{
			key: "unitlimit", label: "Unit limit", pics: []string{"armfav", "armflash"}, kind: nlStepper,
			steps: []string{"Auto", "250", "500", "1000", "1500", "2000", "3000"},
			get: func(d *nlDraft) int {
				for i, v := range nlUnitLimits {
					if v == d.unitLimit {
						return i
					}
				}
				return 0
			},
			set: func(d *nlDraft, v int) { d.unitLimit = nlUnitLimits[v] },
			desc: func(d *nlDraft, v int) string {
				if v == 0 {
					limit, source := s.shell().effectiveUnitLimit()
					if source != "" {
						return fmt.Sprintf("The most units each player may have. Auto uses %d, set by %s.", limit, source)
					}
					return fmt.Sprintf("The most units each player may have. Auto uses %d.", settings.DefaultUnitLimit)
				}
				return "The most units each player may have. Your choice wins over a mod's own limit."
			},
			scene: func(*nlDraft, int) string { return "armor" },
		},
	}
}

var nlUnitLimits = []int{0, 250, 500, 1000, 1500, 2000, 3000}

// ---------------------------------------------------------------- MUTATORS

var nlMutatorPics = map[string][]string{
	"buildSpeed":   {"armnanotc", "armck"},
	"buildCost":    {"armmoho", "armmex"},
	"income":       {"armsolar", "armwin"},
	"salvage":      {"armck", "armcv"},
	"health":       {"corkrog", "armzeus"},
	"damage":       {"armbrtha", "armguard"},
	"areaOfEffect": {"armsilo", "corsilo"},
	"fireRate":     {"armpw", "armllt"},
	"unitSpeed":    {"armflash", "armfast"},
	"sight":        {"armrad", "armpeep"},
	"radar":        {"armarad", "armrad"},
}

var nlMutatorScene = map[string]string{
	"buildSpeed": "construct", "buildCost": "construct", "income": "construct", "salvage": "salvage",
	"health": "armor", "damage": "armor", "fireRate": "armor", "areaOfEffect": "blast",
	"unitSpeed": "march", "sight": "sight", "radar": "sight",
}

func (s *nlScreen) mutatorCards() []nlCard {
	var cards []nlCard
	labels := make([]string, len(content.MutatorSteps))
	for i, f := range content.MutatorSteps {
		labels[i] = "×" + f.String()
	}
	for _, info := range content.MutatorCatalog() {
		cards = append(cards, nlCard{
			key: "mut-" + info.Key, label: info.Label, pics: nlMutatorPics[info.Key], kind: nlStepper, steps: labels,
			get: func(d *nlDraft) int {
				f, _ := d.mutators.Factor(info.Key)
				for i, step := range content.MutatorSteps {
					if f == step || (f.IsIdentity() && step.IsIdentity()) {
						return i
					}
				}
				return 3
			},
			set: func(d *nlDraft, v int) { _ = d.mutators.SetFactor(info.Key, content.MutatorSteps[v]) },
			// Compare sets the factor beside ×1: two scenes, side by side.
			compare: func(d *nlDraft, v int) (int, bool) {
				for i, step := range content.MutatorSteps {
					if step.IsIdentity() {
						return i, v != i
					}
				}
				return 0, false
			},
			desc: func(*nlDraft, int) string {
				return info.Description + " Mutators apply under every rule set, Strict 3.1 included."
			},
			chips:        []string{info.Group},
			scene:        func(*nlDraft, int) string { return nlMutatorScene[info.Key] },
			usesMutators: true,
		})
	}
	return cards
}

// ---------------------------------------------------------------- GRAPHICS

var nlFPS = []int{30, 60, 120, 144, 0}

func (s *nlScreen) graphicsCards() []nlCard {
	return []nlCard{
		{
			key: "renderer", label: "Renderer", pics: []string{"armrad", "armarad"}, kind: nlHalves,
			steps: []string{"Classic", "Enhanced"}, subs: []string{"8-bit · 30 fps · pixel-exact", "True colour · smooth · effects"},
			get: func(d *nlDraft) int { return onOff(d.pres.Renderer == "modern") },
			set: func(d *nlDraft, v int) {
				d.pres.Renderer = "classic"
				if v == 1 {
					d.pres.Renderer = "modern"
				}
			},
			desc: func(d *nlDraft, v int) string {
				if v == 0 {
					return "The original 8-bit picture, pixel for pixel, at the original 30 frames a second. Effects are off."
				}
				return "True colour, motion drawn at your display's rate, 2× detail art and everything on the Effects page."
			},
			scene:   func(*nlDraft, int) string { return "closeup" },
			render:  func(d *nlDraft, v int, r *nlRender) { r.classic = v == 0 },
			compare: func(d *nlDraft, v int) (int, bool) { return 1 - v, true },
		},
		{
			key: "fps", label: "Frame rate", pics: []string{"armflash", "armpw"}, kind: nlStepper,
			steps: []string{"30", "60", "120", "144", "Display"},
			get: func(d *nlDraft) int {
				for i, f := range nlFPS {
					if f == d.pres.FPS {
						return i
					}
				}
				return 1
			},
			set: func(d *nlDraft, v int) { d.pres.FPS = nlFPS[v] },
			desc: func(d *nlDraft, v int) string {
				return "How often Enhanced draws a frame. The battle always runs at 30 ticks a second; faster rates draw the motion in between."
			},
			scene:    func(*nlDraft, int) string { return "armor" },
			render:   func(d *nlDraft, v int, r *nlRender) { r.fps = nlFPS[v] },
			enhanced: true,
		},
		{
			key: "sidebar", label: "Sidebar", pics: []string{"armlab", "armvp"}, kind: nlHalves,
			steps: []string{"Original", "12 per page", "Free flow"},
			subs:  []string{"Authored pages", "Twelve per page", "Fills the window"},
			// Original is the authored panel; the other two are the expanded
			// sidebar, locked to twelve-cell pages or flowing freely
			// (DESIGN_INTERFACE_HUD_INPUT §3.3 "Build page lock").
			get: func(d *nlDraft) int {
				switch {
				case d.pres.ExpandedSidebar == 0:
					return 0
				case d.pres.BuildMenuPageSize == 12:
					return 1
				}
				return 2
			},
			set: func(d *nlDraft, v int) {
				d.pres.ExpandedSidebar = onOff(v != 0)
				d.pres.BuildMenuPageSize = [...]int{d.pres.BuildMenuPageSize, 12, 0}[v]
			},
			desc: func(_ *nlDraft, v int) string {
				return [...]string{
					"The original side panel: each builder's authored build pages, six to a page.",
					"The taller side panel with build pages of twelve, the layout ProTA, TA Zero and Escalation author. A mod's own page size can still set it.",
					"The taller side panel fills every row the window has room for, so fewer page turns on a big display.",
				}[v]
			},
			scene:    func(*nlDraft, int) string { return "armor" },
			enhanced: true,
			demo:     "sidebar",
		},
		{
			key: "fullscreen", label: "Fullscreen", pics: []string{"armmark", "armrad"}, kind: nlHalves,
			steps: []string{"Windowed", "Fullscreen"}, subs: []string{"A window you can move", "The whole display"},
			get: func(d *nlDraft) int { return onOff(d.fullscreen) },
			set: func(d *nlDraft, v int) { d.fullscreen = v == 1 },
			desc: func(*nlDraft, int) string {
				return "Alt+Enter switches too, anywhere in the game."
			},
			scene: func(*nlDraft, int) string { return "armor" },
			demo:  "fullscreen",
		},
	}
}

// ---------------------------------------------------------------- EFFECTS

func (s *nlScreen) effectCards() []nlCard {
	type field = func(p *settings.Presentation) *int
	// sw is an Off/On part for one switch.
	sw := func(key, label, sub string, f field, apply func(r *nlRender, on bool)) nlPart {
		return nlPart{key: key, label: label, sub: sub, steps: []string{"Off", "On"},
			get:    func(d *nlDraft) int { return onOff(*f(&d.pres) != 0) },
			set:    func(d *nlDraft, v int) { *f(&d.pres) = v },
			render: func(r *nlRender, v int) { apply(r, v != 0) }}
	}
	strengths := []int{25, 50, 100, 150, 200}
	// strength is a percentage part: its steps, not an off, since the
	// switch beside it turns the treatment off.
	strength := func(key, label, sub string, f field, apply func(r *nlRender, pct int)) nlPart {
		return nlPart{key: key, label: label, sub: sub, meter: true, steps: []string{"25%", "50%", "100%", "150%", "200%"},
			get: func(d *nlDraft) int {
				v := *f(&d.pres)
				best := 2
				for i, st := range strengths {
					if abs(st-v) < abs(strengths[best]-v) {
						best = i
					}
				}
				return best
			},
			set:    func(d *nlDraft, v int) { *f(&d.pres) = strengths[v] },
			render: func(r *nlRender, v int) { apply(r, strengths[v]) }}
	}
	pres := func(get func(p *settings.Presentation) *int) field { return get }
	return []nlCard{
		s.groupCard("water", "Water", []string{"armsub", "corsub", "armship"}, "naval",
			"The Enhanced water, part by part. Compare shows the part you pick switched off beside it on.",
			sw("waterSurface", "Surface", "Depth tint, damp shores and the shallows", pres(func(p *settings.Presentation) *int { return &p.WaterSurface }),
				func(r *nlRender, on bool) { r.effects.WaterSurface = on }),
			sw("waterMotion", "Motion", "Drift, churn and hulls wavering below", pres(func(p *settings.Presentation) *int { return &p.WaterMotion }),
				func(r *nlRender, on bool) { r.effects.WaterMotion = on }),
			sw("waterFoam", "Foam", "Shore foam, wakes and hovercraft spray", pres(func(p *settings.Presentation) *int { return &p.WaterFoam }),
				func(r *nlRender, on bool) { r.effects.WaterFoam = on }),
			sw("waterReflections", "Reflections", "Ships, aircraft and blasts mirrored", pres(func(p *settings.Presentation) *int { return &p.WaterReflections }),
				func(r *nlRender, on bool) { r.effects.WaterReflections = on }),
		),
		s.groupCard("lighting", "Lighting", []string{"armllt", "corllt"}, "lighting",
			"Light from explosions, fire, shots, hot wrecks and the nano spray, on units and on the ground.",
			sw("modelLight", "Unit light", "On units and smoke", pres(func(p *settings.Presentation) *int { return &p.ModelLight }),
				func(r *nlRender, on bool) { r.effects.ModelLight = on }),
			sw("groundLight", "Ground light", "Pools and flashes on the terrain", pres(func(p *settings.Presentation) *int { return &p.GroundLight }),
				func(r *nlRender, on bool) { r.effects.GroundLight = on }),
			strength("groundLightStrength", "Ground light strength", "How bright the pools are", pres(func(p *settings.Presentation) *int { return &p.GroundLightStrength }),
				func(r *nlRender, pct int) { r.groundLightStrength = pct }),
		),
		blinking(s.groupCard("metal", "Metal", []string{"armmex", "armmoho"}, "metal",
			"How unit hulls take the light.",
			sw("finish", "Finish", "Metal and paint read as metal and paint", pres(func(p *settings.Presentation) *int { return &p.Finish }),
				func(r *nlRender, on bool) { r.effects.Finish = on }),
			sw("glint", "Glint", "Highlights running across metal as it turns", pres(func(p *settings.Presentation) *int { return &p.Glint }),
				func(r *nlRender, on bool) { r.effects.Glint = on }),
		)),
		{
			key: "supersample", label: "Smooth edges", pics: []string{"armbull", "corgol"}, kind: nlSwitch, steps: []string{"Off", "On"},
			get: func(d *nlDraft) int { return onOff(d.pres.Supersample != 0) },
			set: func(d *nlDraft, v int) { d.pres.Supersample = v },
			desc: func(*nlDraft, int) string {
				return "Units, buildings and their shadows drawn at twice the detail and blended where their edges cross a pixel. Off keeps the hard pixel edges of the original art."
			},
			// The magnified turning hulls of the Metal card, where edges step
			// or blend as each unit turns; blinking, since an edge pixel is
			// too small to compare across a split.
			scene:    func(*nlDraft, int) string { return "metal" },
			render:   func(d *nlDraft, v int, r *nlRender) { r.effects.Supersample = v != 0 },
			compare:  func(d *nlDraft, v int) (int, bool) { return 0, v != 0 },
			enhanced: true,
			blink:    true,
		},
		{
			key: "glow", label: "Glow", pics: []string{"armanni", "corhlt"}, kind: nlMeter, steps: []string{"Off", "50%", "100%", "150%", "200%"},
			get: func(d *nlDraft) int {
				if d.glow == 0 {
					return 0
				}
				return max(1, min(4, (d.glowStrength+25)/50))
			},
			set: func(d *nlDraft, v int) {
				d.glow = onOff(v != 0)
				if v != 0 {
					d.glowStrength = v * 50
				}
			},
			desc: func(*nlDraft, int) string {
				return "A bloom halo on lasers, lightning, fire, blasts, shots and the nano spray, from half to twice the tuned strength."
			},
			chips:    []string{"Lasers", "Nano spray", "Fire", "Blasts"},
			scene:    func(*nlDraft, int) string { return "glow" },
			render:   func(d *nlDraft, v int, r *nlRender) { r.glow, r.glowStrength = v != 0, v*50 },
			compare:  func(d *nlDraft, v int) (int, bool) { return 0, v != 0 },
			enhanced: true,
		},
		s.groupCard("heat", "Heat", []string{"corpyro", "armbrtha"}, "blast",
			"Hot air bending the picture, and wrecks that glow as they cool.",
			withScene(sw("blastRings", "Blast rings", "The ring of hot air round every blast", pres(func(p *settings.Presentation) *int { return &p.BlastRings }),
				func(r *nlRender, on bool) { r.effects.BlastRings = on }), "blast"),
			withScene(strength("blastRingStrength", "Ring strength", "More or less bend for the same shot", pres(func(p *settings.Presentation) *int { return &p.BlastRingStrength }),
				func(r *nlRender, pct int) { r.blastRingStrength = pct }), "blast"),
			withScene(sw("fireShimmer", "Fire shimmer", "Heat haze over burning trees", pres(func(p *settings.Presentation) *int { return &p.FireShimmer }),
				func(r *nlRender, on bool) { r.effects.FireShimmer = on }), "fireshimmer"),
			withScene(sw("wreckGlow", "Wreck glow", "The red of a fresh wreck, fading as it cools", pres(func(p *settings.Presentation) *int { return &p.WreckGlow }),
				func(r *nlRender, on bool) { r.effects.WreckGlow = on }), "hotwrecks"),
			withScene(sw("wreckShimmer", "Wreck heat wave", "The haze rising over a hot wreck", pres(func(p *settings.Presentation) *int { return &p.WreckShimmer }),
				func(r *nlRender, on bool) { r.effects.WreckShimmer = on }), "hotwrecks"),
		),
		s.groupCard("marks", "Marks", []string{"armbull", "corgol"}, "crater",
			"What battles leave on the ground.",
			withScene(sw("scorch", "Scorch", "Blast scars that fade over time", pres(func(p *settings.Presentation) *int { return &p.Scorch }),
				func(r *nlRender, on bool) { r.effects.Scorch = on }), "crater"),
			withScene(nlPart{key: "trails", label: "Trails", sub: "Footprints and tracks, and how dark they lie", meter: true,
				steps: []string{"Off", "25%", "50%", "100%"},
				get: func(d *nlDraft) int {
					switch v := d.pres.TrailStrength; {
					case v <= 0:
						return 0
					case v <= 25:
						return 1
					case v <= 50:
						return 2
					}
					return 3
				},
				set:    func(d *nlDraft, v int) { d.pres.TrailStrength = [...]int{0, 25, 50, 100}[v] },
				render: func(r *nlRender, v int) { r.trailStrength = [...]int{0, 25, 50, 100}[v] }}, "trails"),
		),
		{
			key: "softshadows", label: "Soft shadows", pics: []string{"armhawk", "armthund"}, kind: nlSwitch, steps: []string{"Off", "On"},
			get: func(d *nlDraft) int { return onOff(d.pres.SoftShadows != 0) },
			set: func(d *nlDraft, v int) { d.pres.SoftShadows = v },
			desc: func(*nlDraft, int) string {
				return "Aircraft shadows that soften and spread the higher a plane flies, and waver on water. Off gives aircraft the plain shadow every other unit has."
			},
			scene:    func(*nlDraft, int) string { return "air" },
			render:   func(d *nlDraft, v int, r *nlRender) { r.effects.SoftShadows = v != 0 },
			compare:  func(d *nlDraft, v int) (int, bool) { return 0, v != 0 },
			enhanced: true,
		},
	}
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

// nlPart is one setting of a grouped card: its values, what it writes and
// how a frame shows it.
type nlPart struct {
	key, label, sub string
	steps           []string
	meter           bool // a strength or level rather than an Off/On switch
	get             func(d *nlDraft) int
	set             func(d *nlDraft, v int)
	render          func(r *nlRender, v int)
	scene           string // the preview for this part, when not the card's
}

func withScene(p nlPart, scene string) nlPart { p.scene = scene; return p }

// groupCard is a card that holds several related settings shown together,
// one scene for them all; its value packs every part's so the draft, the
// change count and Apply treat it as one card. Compare shows the selected
// part at its lowest value beside the draft.
// blinking makes a card's compare a blink (nlCard.blink).
func blinking(c nlCard) nlCard {
	c.blink = true
	return c
}

func (s *nlScreen) groupCard(key, label string, pics []string, scene, text string, parts ...nlPart) nlCard {
	pack := func(d *nlDraft) int {
		code, radix := 0, 1
		for _, p := range parts {
			code += p.get(d) * radix
			radix *= len(p.steps)
		}
		return code
	}
	unpack := func(code int) []int {
		out := make([]int, len(parts))
		for i, p := range parts {
			out[i] = code % len(p.steps)
			code /= len(p.steps)
		}
		return out
	}
	selected := func() int { return max(0, min(s.partSel[key], len(parts)-1)) }
	return nlCard{
		key: key, label: label, pics: pics, kind: nlGroup, parts: parts, steps: []string{""},
		get: pack,
		set: func(d *nlDraft, code int) {
			for i, v := range unpack(code) {
				parts[i].set(d, v)
			}
		},
		desc: func(*nlDraft, int) string { return text },
		scene: func(d *nlDraft, v int) string {
			if p := parts[selected()]; p.scene != "" {
				return p.scene
			}
			return scene
		},
		render: func(d *nlDraft, code int, r *nlRender) {
			for i, v := range unpack(code) {
				parts[i].render(r, v)
			}
		},
		compare: func(d *nlDraft, code int) (int, bool) {
			vals := unpack(code)
			i := selected()
			if vals[i] == 0 {
				return 0, false
			}
			alt := 0
			radix := 1
			for j, p := range parts {
				v := vals[j]
				if j == i {
					v = 0
				}
				alt += v * radix
				radix *= len(p.steps)
			}
			return alt, true
		},
		enhanced: true,
	}
}

// ---------------------------------------------------------------- CONTROLS

var nlSnapKeys = []string{"alt", "ctrl", "shift"}

func (s *nlScreen) controlCards() []nlCard {
	presetSteps := make([]string, len(nlControlsPresets))
	presetSubs := make([]string, len(nlControlsPresets))
	for i, p := range nlControlsPresets {
		presetSteps[i], presetSubs[i] = p.label, p.sub
	}
	half := func(key, label string, pics []string, steps, subs []string, field func(p *settings.Presentation) *int, text string) nlCard {
		return nlCard{
			key: key, label: label, pics: pics, kind: nlHalves, steps: steps, subs: subs,
			get:   func(d *nlDraft) int { return *field(&d.pres) },
			set:   func(d *nlDraft, v int) { *field(&d.pres) = v },
			desc:  func(*nlDraft, int) string { return text },
			scene: func(*nlDraft, int) string { return "controls" },
		}
	}
	return []nlCard{
		{
			key: "profile", label: "Profile", pics: []string{"armcom", "corcom"}, kind: nlStepper, steps: presetSteps, subs: presetSubs,
			get: func(d *nlDraft) int { return d.controls },
			set: func(d *nlDraft, v int) { d.controls = v },
			desc: func(d *nlDraft, v int) string {
				return nlControlsPresets[v].sub + " A profile sets the switches on this page and the Community pages of Options together; change any of them after."
			},
			scene: func(*nlDraft, int) string { return "controls" },
		},
		{
			key: "interface", label: "Mouse buttons", pics: []string{"armpw", "armjeth"}, kind: nlHalves,
			steps: []string{"Left-click", "Right-click"}, subs: []string{"Right button deselects", "Right button gives orders"},
			get: func(d *nlDraft) int { return d.interfaceType },
			set: func(d *nlDraft, v int) { d.interfaceType = v },
			desc: func(*nlDraft, int) string {
				return "The original game's Interface Type: with left-click, the right button deselects and cancels; with right-click, an idle right press gives the contextual order."
			},
			scene: func(*nlDraft, int) string { return "controls" },
		},
		{
			key: "selection", label: "Selection", pics: []string{"armpw", "armjeth"}, kind: nlHalves,
			steps: []string{"Retail", "Community", "Zero"}, subs: []string{"Original rules", "Community patch cycling", "TA Zero rules"},
			get: func(d *nlDraft) int { return d.pres.CommunitySelection },
			set: func(d *nlDraft, v int) { d.pres.CommunitySelection = v },
			desc: func(*nlDraft, int) string {
				return "How selection keys and clicks behave: the original rules, the community patch's cycling, or TA Zero's."
			},
			scene: func(*nlDraft, int) string { return "armor" },
		},
		half("dblclick", "Double-click", []string{"armham", "armrock"}, []string{"Off", "On"}, []string{"Retail", "Selects that type on screen"},
			func(p *settings.Presentation) *int { return &p.DoubleClickSelection },
			"Double-clicking a unit selects every unit of that type on screen, as the community patch does."),
		half("batch", "Factory ×100", []string{"armlab", "armvp"}, []string{"Off", "On"}, []string{"Alt queues 20", "Ctrl+Shift queues 100"},
			func(p *settings.Presentation) *int { return &p.FactoryHundredBatch },
			"Ctrl+Shift on a factory's build button queues 100 at once. Alt keeps its batch of 20."),
		{
			key: "digits", label: "Digit keys", pics: []string{"armcom", "armck"}, kind: nlHalves,
			steps: []string{"Build pages", "Groups"}, subs: []string{"1–9 pick build pages", "1–9 recall groups"},
			get: func(d *nlDraft) int { return onOff(d.switchAlt) },
			set: func(d *nlDraft, v int) { d.switchAlt = v == 1 },
			desc: func(*nlDraft, int) string {
				return "Whether the plain digit keys pick build pages or recall control groups; Alt+digit does the other."
			},
			scene: func(*nlDraft, int) string { return "controls" },
		},
		half("orderdrag", "Order drag", []string{"armfav", "armflash"}, []string{"Off", "On"}, []string{"Queued orders stay", "Drag queued waypoints"},
			func(p *settings.Presentation) *int { return &p.QueuedOrderDrag },
			"Drag a queued move or patrol waypoint to a new spot with the mouse."),
		{
			key: "builddrag", label: "Build drag", pics: []string{"armsolar", "armwin"}, kind: nlHalves,
			steps: []string{"Off", "On"}, subs: []string{"One site per click", "Drag a row, Alt a grid"},
			get: func(d *nlDraft) int { return d.pres.BuildDrag },
			set: func(d *nlDraft, v int) { d.pres.BuildDrag = v },
			desc: func(*nlDraft, int) string {
				return "Drag while placing a structure to lay a row of them; hold Alt to lay a grid. Shift appends, and a Shift row over queued sites takes them off."
			},
			scene:    func(*nlDraft, int) string { return "controls" },
			enhanced: true,
		},
		half("tab", "Tab key", []string{"armrad", "armmark"}, []string{"Options", "Megamap"}, []string{"Opens the battle menu", "Opens the megamap"},
			func(p *settings.Presentation) *int { return &p.Overview },
			"What Tab does in battle: the retail options menu, or the community megamap overview."),
		{
			key: "snapkey", label: "Snap override", pics: []string{"armmex", "armmoho"}, kind: nlStepper, steps: []string{"Alt", "Ctrl", "Shift"},
			get: func(d *nlDraft) int {
				for i, k := range nlSnapKeys {
					if strings.EqualFold(k, d.pres.ClickSnapOverrideKey) {
						return i
					}
				}
				return 0
			},
			set: func(d *nlDraft, v int) { d.pres.ClickSnapOverrideKey = nlSnapKeys[v] },
			desc: func(*nlDraft, int) string {
				return "Hold this key to place a structure exactly where you click instead of snapping to the nearest metal or wreck."
			},
			scene: func(*nlDraft, int) string { return "controls" },
		},
	}
}

// parseMutatorPairs reads Mutators.String's "key=value,..." spelling back.
func parseMutatorPairs(text string) map[string]string {
	out := map[string]string{}
	for _, part := range strings.Split(text, ",") {
		key, value, ok := strings.Cut(strings.TrimSpace(part), "=")
		if ok {
			out[key] = value
		}
	}
	return out
}
