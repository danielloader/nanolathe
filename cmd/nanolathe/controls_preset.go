package main

// Controls presets: a mod's recommended settings, offered once and never
// forced (docs/DESIGN_MODS_MUTATORS.md §4.3, D13, P10). The preset's
// contents live in controlsPresetRows alone; the design document's §4.3 table
// lists the same rows.

import (
	"strconv"

	"github.com/nanolathe-gg/nanolathe/internal/audio"
	contentprofiles "github.com/nanolathe-gg/nanolathe/internal/content/profiles"
	"github.com/nanolathe-gg/nanolathe/internal/modlibrary"
	"github.com/nanolathe-gg/nanolathe/internal/settings"
)

// The preset names mod metadata and content profiles may carry.
const (
	controlsPresetCommunity = contentprofiles.ControlsCommunity
	controlsPresetRetail    = contentprofiles.ControlsRetail
	controlsPresetZero      = contentprofiles.ControlsZero
)

// presetUnchanged is a row value the preset leaves alone.
const presetUnchanged = -1

// presetCustom is what a row that stands for several stored values reads
// when they match none of its named choices; no preset writes it.
const presetCustom = -2

// controlsPresetRow is one existing host option a preset assigns. Each value
// is the option's own stored integer. A row owns no new behaviour: it writes
// a setting the player can already change on an options page or with a chat
// command.
type controlsPresetRow struct {
	label     string
	community int
	retail    int
	zero      int
	// names displays a value; nil displays Off/On for 0/1, and numbers
	// otherwise.
	names []string
	get   func(g *gameShell) int
	set   func(g *gameShell, value int)
}

var onOffNames = []string{"Off", "On"}

// The Dot colours row's named tables.
const (
	playerColoursDefault = 0
	playerColoursProTA   = 1
	playerColoursZero    = 2
)

// presentationRow assigns one field of the presentation block.
func presentationRow(label string, community, retail, zero int, field func(*settings.Presentation) *int) controlsPresetRow {
	return controlsPresetRow{
		label: label, community: community, retail: retail, zero: zero, names: onOffNames,
		get: func(g *gameShell) int {
			p := g.presentation
			return *field(&p)
		},
		set: func(g *gameShell, value int) {
			p := g.presentation
			*field(&p) = value
			g.setPresentation(p)
		},
	}
}

// minimumRow keeps independently authored range thresholds visible in the offer.
func minimumRow(label string, zero int, field func(*settings.Presentation) *int) controlsPresetRow {
	row := presentationRow(label, 0, presetUnchanged, zero, field)
	row.names = nil
	return row
}

func selectionPresetRow() controlsPresetRow {
	row := presentationRow("Selection keys", 1, 0, 2, func(p *settings.Presentation) *int { return &p.CommunitySelection })
	row.names = []string{"Retail", "Community", "Zero"}
	return row
}

// keyboardPresetRow selects the keyboard profile of the same name and keeps
// every action the player rebound (DESIGN_INTERFACE_HUD_INPUT §3.6
// "Rebinding"). Community's keys are retail's, since ProTA's controls change
// what Ctrl+B/F/S do rather than which keys do it
// (research/extensions/prota-engine.md, "Shipped selection and hotkey
// audit"); Zero adds its documented Z for the previous build page.
func keyboardPresetRow() controlsPresetRow {
	return controlsPresetRow{
		label: "Keyboard", community: 1, retail: 0, zero: 2,
		names: []string{"Retail", "Community", "Zero"},
		get:   func(g *gameShell) int { return g.keyProfileIndex() },
		set: func(g *gameShell, value int) {
			if value >= 0 && value < len(keyProfiles) {
				g.setKeyProfile(keyProfiles[value])
			}
		},
	}
}

// controlsPresetRows is the whole content of the presets. The `community`
// column is ProTA 4.8's recommended settings: the Community host options
// (DESIGN_COMMUNITY_PATCH §7), the preferences ProTA's `ProTA.ini` pins
// through its `[REG]` block (research/extensions/community-patch-engine.md
// §4.1), its draw-engine megamap keys, and the victory cue its renderer
// always plays. The `retail` column is the retail default of each; a row
// the retail preset leaves alone says presetUnchanged. Zero assigns only
// settings documented by Alpha 5 TAZero.ini and the author’s controls page,
// retaining other preferences
// (research/extensions/ta-zero-engine.md, "Documented engine-level behavior").
var controlsPresetRows = []controlsPresetRow{
	selectionPresetRow(),
	keyboardPresetRow(),
	// The pinned source's quick-key handler, which the Community selection
	// row already follows, makes Shift's factory step 100 while Ctrl is held;
	// ProTA 4.8's recorder lists "Queue 100 units" among the interface-upgrade
	// functions it enables (DESIGN_INTERFACE_HUD_INPUT §3.13).
	presentationRow("Factory Ctrl+Shift 100", 1, 0, 1, func(p *settings.Presentation) *int { return &p.FactoryHundredBatch }),
	presentationRow("Double-click select", 1, 0, 1, func(p *settings.Presentation) *int { return &p.DoubleClickSelection }),
	presentationRow("Order drag", 1, 0, presetUnchanged, func(p *settings.Presentation) *int { return &p.QueuedOrderDrag }),
	{
		label: "Digit keys", community: 1, retail: settings.DefaultSwitchAlt, zero: 1,
		names: []string{"Pages", "Groups"},
		get:   func(g *gameShell) int { return boolInt(g.switchAlt) },
		set:   func(g *gameShell, value int) { g.switchAlt = value != 0 },
	},
	presentationRow("Counters", 1, 0, presetUnchanged, func(p *settings.Presentation) *int { return &p.CommunityCounters }),
	presentationRow("Reload bars", 1, 0, presetUnchanged, func(p *settings.Presentation) *int { return &p.ReloadBars }),
	presentationRow("Veterancy", 1, 0, presetUnchanged, func(p *settings.Presentation) *int { return &p.VeteranLabels }),
	// Retail draws a unit's group digit [03 R-FX-01 §6]; the option only
	// suppresses it, so its retail value, like its default, is On
	// (DESIGN_INTERFACE_HUD_INPUT §3.14).
	presentationRow("Group digits", 1, 1, presetUnchanged, func(p *settings.Presentation) *int { return &p.GroupNumbers }),
	presentationRow("Wind/tide readout", 1, 0, presetUnchanged, func(p *settings.Presentation) *int { return &p.WeatherReport }),
	// The megamap rows are ProTA.ini's draw-engine keys
	// (DESIGN_INTERFACE_HUD_INPUT §3.15). The retail preset returns the
	// overview to Zoom and leaves the megamap's own preferences alone.
	{
		label: "Overview", community: settings.OverviewMegamap, retail: settings.OverviewZoom, zero: settings.OverviewMegamap,
		names: []string{"Zoom", "Megamap"},
		get:   func(g *gameShell) int { return g.presentation.Overview },
		set: func(g *gameShell, value int) {
			p := g.presentation
			p.Overview = value
			g.setPresentation(p)
		},
	},
	presentationRow("Megamap wheel", 1, presetUnchanged, 1, func(p *settings.Presentation) *int { return &p.MegamapWheel }),
	presentationRow("Wheel out moves camera", 1, presetUnchanged, 1, func(p *settings.Presentation) *int { return &p.MegamapWheelMove }),
	presentationRow("Megamap double-click move", 0, presetUnchanged, 0, func(p *settings.Presentation) *int { return &p.MegamapDoubleClickMove }),
	presentationRow("Under-attack flash", 1, presetUnchanged, 1, func(p *settings.Presentation) *int { return &p.MegamapFlash }),
	minimumRow("Megamap radar minimum", 0, func(p *settings.Presentation) *int { return &p.MegamapRadarMinimum }),
	minimumRow("Megamap sonar minimum", 500, func(p *settings.Presentation) *int { return &p.MegamapSonarMinimum }),
	minimumRow("Megamap sonar jammer minimum", 0, func(p *settings.Presentation) *int { return &p.MegamapSonarJamMinimum }),
	minimumRow("Megamap anti-nuke minimum", 512, func(p *settings.Presentation) *int { return &p.MegamapAntiNukeMinimum }),
	{
		// The ten-entry dot colour table as one choice: the draw engine's
		// defaults (retail) or ProTA.ini's palette.
		label: "Dot colours", community: playerColoursProTA, retail: playerColoursDefault, zero: playerColoursZero,
		names: []string{"Default", "ProTA", "TA Zero"},
		get: func(g *gameShell) int {
			switch g.presentation.PlayerDotColors {
			case settings.DefaultPlayerDotColors:
				return playerColoursDefault
			case settings.ProTAPlayerDotColors:
				return playerColoursProTA
			case settings.ZeroPlayerDotColors:
				return playerColoursZero
			}
			return presetCustom
		},
		set: func(g *gameShell, value int) {
			p := g.presentation
			p.PlayerDotColors = settings.DefaultPlayerDotColors
			if value == playerColoursProTA {
				p.PlayerDotColors = settings.ProTAPlayerDotColors
			} else if value == playerColoursZero {
				p.PlayerDotColors = settings.ZeroPlayerDotColors
			}
			g.setPresentation(p)
		},
	},
	// ProTA's draw engine squares each allied resource row in the player's
	// dot colour (DESIGN_INTERFACE_HUD_INPUT §3.15).
	presentationRow("Allied dot swatches", 1, 0, presetUnchanged, func(p *settings.Presentation) *int { return &p.AlliedDotSwatches }),
	{
		label: "Game clock", community: 1, retail: settings.DefaultClock, zero: presetUnchanged, names: onOffNames,
		get: func(g *gameShell) int { return boolInt(g.clockVisible) },
		set: func(g *gameShell, value int) { g.clockVisible = value != 0 },
	},
	{
		label: "Sound", community: settings.SoundMode3D, retail: settings.DefaultSoundMode, zero: settings.SoundMode3D,
		names: []string{"Off", "Mono", "3D"},
		get:   func(g *gameShell) int { return g.audioPrefs.SoundMode },
		set:   func(g *gameShell, value int) { g.audioPrefs.SoundMode = value },
	},
	presentationRow("Victory cue", 1, 0, presetUnchanged, func(p *settings.Presentation) *int { return &p.VictoryCue }),
	{
		// 128 voices is more than the mixer's 32 tracked slots, so no sound
		// is cut off for the voice limit [03 R-AUD-01 §1].
		label: "Sound voices", community: 128, retail: settings.DefaultMixingBuffers, zero: 128,
		get: func(g *gameShell) int { return g.audioPrefs.MixingBuffers },
		set: func(g *gameShell, value int) { g.audioPrefs.MixingBuffers = value },
	},
	{
		label: "Music", community: 2, retail: settings.DefaultCDMode, zero: 2,
		names: []string{"", "Play all", "Random", "Repeat", "Custom"},
		get:   func(g *gameShell) int { return g.audioPrefs.CDMode },
		set:   func(g *gameShell, value int) { g.audioPrefs.CDMode = value },
	},
	{
		// The skirmish screen's row count, `NumSkirmishPlayers`, as the
		// `*X` selector sets it [08 R-SKIR-01 §1]. Only the rows shown
		// change: each row keeps its controller, so the players a skirmish
		// starts with stay the same. The retail preset never removes rows.
		label: "Skirmish rows", community: settings.MaxPlayers, retail: presetUnchanged, zero: settings.MaxPlayers,
		get: func(g *gameShell) int { return g.setup.NumPlayers },
		set: func(g *gameShell, value int) {
			if !g.survivalMenu {
				g.setup.NumPlayers = value
			}
		},
	},
}

// presetValue is the row's value under a preset, or presetUnchanged.
func (r controlsPresetRow) presetValue(preset string) int {
	switch preset {
	case controlsPresetCommunity:
		return r.community
	case controlsPresetRetail:
		return r.retail
	case controlsPresetZero:
		return r.zero
	}
	return presetUnchanged
}

func (r controlsPresetRow) valueText(value int) string {
	if value == presetCustom {
		return "Custom"
	}
	if value >= 0 && value < len(r.names) && r.names[value] != "" {
		return r.names[value]
	}
	return strconv.Itoa(value)
}

// knownControlsPreset reports whether name is one of the presets.
func knownControlsPreset(name string) bool {
	return name == controlsPresetCommunity || name == controlsPresetRetail || name == controlsPresetZero
}

// applyControlsPreset writes a named assignment of existing host options
// once (§4.3, P10). Later changes by the player stick; only a switch away
// from the content offers them back (restoreControlsPreset).
func (g *gameShell) applyControlsPreset(name string) {
	if !knownControlsPreset(name) {
		return
	}
	for _, row := range controlsPresetRows {
		if value := row.presetValue(name); value != presetUnchanged {
			row.set(g, value)
		}
	}
	g.finishControlsPreset()
}

// finishControlsPreset brings the live audio in line with rows a preset wrote.
func (g *gameShell) finishControlsPreset() {
	g.audioPrefs.Normalize()
	g.applyRetailAudioOptions()
	if g.audioOwner != nil && g.audioOwner.Music != nil {
		music := g.audioOwner.Music
		music.Configure(audio.PlayMode(g.audioPrefs.CDMode), music.DesiredCategory())
	}
}

// modRecommendations is a mod's controls preset and gameplay minimum as the
// mount will see them: its metadata, else its content profile (§4.3).
func (s *modsScreen) modRecommendations(mod *modlibrary.Mod) *modlibrary.Mod {
	if s == nil || mod == nil {
		return mod
	}
	key := mod.ID + "@" + mod.Version
	if resolved, ok := s.recommended[key]; ok {
		return &resolved
	}
	if s.recommended == nil {
		s.recommended = map[string]modlibrary.Mod{}
	}
	resolved := modlibrary.ResolveProfileDefaults(s.baseRoots, *mod)
	s.recommended[key] = resolved
	return &resolved
}
