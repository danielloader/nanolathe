package main

import (
	"encoding/json"
	"fmt"
	"slices"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	contentprofiles "github.com/nanolathe-gg/nanolathe/internal/content/profiles"
	"github.com/nanolathe-gg/nanolathe/internal/gameplay"
	"github.com/nanolathe-gg/nanolathe/internal/input"
	"github.com/nanolathe-gg/nanolathe/internal/modlibrary"
	"github.com/nanolathe-gg/nanolathe/internal/platform/screenkit"
	"github.com/nanolathe-gg/nanolathe/internal/settings"
)

// These are host settings transactions and input contracts, independent of
// retail assets (DESIGN_INTERFACE_HUD_INPUT §3.17, DESIGN_MODS_MUTATORS §4.6).
func settingsRegressionMod() *modlibrary.Mod {
	return &modlibrary.Mod{Metadata: modlibrary.Metadata{
		ID: "settings-test", Name: "Settings test", MinimumGameplay: string(gameplay.Community39),
		Config: &modlibrary.Config{Settings: json.RawMessage(`{"presentation":{"glint":0},"keyBindings":{"profile":"zero"}}`),
			Locks: []string{"presentation.glint", "keyBindings"}},
	}}
}

func settingsRegressionScreen(mod *modlibrary.Mod, file settings.Settings) (*gameShell, *nlScreen) {
	g := &gameShell{cs: &contentSet{mod: mod}}
	g.applySettings(file)
	s := newNLScreen(func() *gameShell { return g })
	s.canvasScale = 1
	if mod != nil {
		s.mods = []modlibrary.Mod{*mod}
	}
	s.draft = s.freshDraft(g)
	s.bindSource(g)
	return g, s
}

func TestNLScreenOverrideApplyAndRestart(t *testing.T) {
	mod := settingsRegressionMod()
	g, s := settingsRegressionScreen(mod, settings.Defaults())
	s.draft.override, s.draft.gameplay = true, gameplay.Strict31
	s.touched["rules"] = true
	s.apply()
	if !g.lockOverridden(mod) || g.gameplay != gameplay.Strict31 {
		t.Fatalf("Apply: override %v, rules %s", g.lockOverridden(mod), g.gameplay)
	}
	next, _ := settingsRegressionScreen(mod, g.captureSettings())
	next.enforceModGameplayMinimum()
	if !next.lockOverridden(mod) || next.gameplay != gameplay.Strict31 {
		t.Fatalf("restart: override %v, rules %s", next.lockOverridden(mod), next.gameplay)
	}
}

func TestNLScreenSavedOverridePrecedesSettingsLayers(t *testing.T) {
	mod := settingsRegressionMod()
	file := settings.Defaults()
	file.ModLockOverrides = []string{mod.ID}
	file.ModSettings = map[string]json.RawMessage{mod.ID: json.RawMessage(`{"presentation":{"glint":1}}`)}
	g, _ := settingsRegressionScreen(mod, file)
	if g.presentation.Glint != 1 {
		t.Fatalf("approved player layer was suppressed: glint %d", g.presentation.Glint)
	}
	direct := (&gameShell{cs: &contentSet{mod: mod}}).effectiveSettings(file)
	if direct.Presentation.Glint != 1 {
		t.Fatalf("direct battle suppressed approved player layer: glint %d", direct.Presentation.Glint)
	}
	file.ModLockOverrides = nil
	g.applySettings(file)
	if g.presentation.Glint != 0 {
		t.Fatalf("cleared approval kept player layer: glint %d", g.presentation.Glint)
	}
}

func TestNLScreenBaseSaveKeepsModSettings(t *testing.T) {
	file := settings.Defaults()
	patch := json.RawMessage(`{"presentation":{"glint":0}}`)
	file.ModSettings = map[string]json.RawMessage{"another-mod": patch}
	g, s := settingsRegressionScreen(nil, file)
	s.draft.pres.WaterSurface = 0
	s.touched["water"] = true
	s.apply()
	saved := g.captureSettings()
	if string(saved.ModSettings["another-mod"]) != string(patch) {
		t.Fatalf("base save lost another mod's settings: %v", saved.ModSettings)
	}
	mod := &modlibrary.Mod{Metadata: modlibrary.Metadata{ID: "another-mod"}}
	next, _ := settingsRegressionScreen(mod, saved)
	if next.presentation.Glint != 0 {
		t.Fatalf("switching back lost saved preference: glint %d", next.presentation.Glint)
	}
}

func TestNLScreenApplyingPresetsKeepsLibraries(t *testing.T) {
	for _, mod := range []*modlibrary.Mod{nil, {Metadata: modlibrary.Metadata{ID: "preset-mod"}}} {
		name := "base"
		if mod != nil {
			name = mod.ID
		}
		t.Run(name, func(t *testing.T) {
			file := settings.Defaults()
			file.ModSettings = map[string]json.RawMessage{"another-mod": json.RawMessage(`{"presentation":{"glint":0}}`)}
			file.Presets = []settings.Preset{{Name: "Mine", Settings: json.RawMessage(`{"presentation":{"glint":0}}`)}}
			g, s := settingsRegressionScreen(mod, file)
			s.applyPresetToDraft(nlPresetEntry{name: "Mine", patch: file.Presets[0].Settings}, []bool{false, true, false})
			s.apply()
			saved := g.captureSettings()
			if len(saved.Presets) != 1 || saved.Presets[0].Name != "Mine" || len(saved.ModSettings["another-mod"]) == 0 {
				t.Fatalf("preset Apply lost stored libraries: presets %v, mod settings %v", saved.Presets, saved.ModSettings)
			}
			if g.presentation.Glint != 0 {
				t.Fatalf("preset was not applied: glint %d", g.presentation.Glint)
			}
		})
	}
}

func TestNLScreenKeyEditsWaitForLockApproval(t *testing.T) {
	q := input.Chord{Key: input.KeyQ}
	a := input.Chord{Key: input.KeyA}
	for _, name := range []string{"capture", "backspace", "clear", "reset-row", "reset-all", "profile", "preset"} {
		t.Run(name, func(t *testing.T) {
			g, s := settingsRegressionScreen(settingsRegressionMod(), settings.Defaults())
			s.draft.keys.Rebind("attack", []input.Chord{q})
			before := slices.Clone(s.draft.keys.Keys("attack"))
			want := []input.Chord{a}
			switch name {
			case "capture":
				s.capture = nlCapture{id: "attack", slot: 0}
				s.bindCaptured(a)
			case "backspace":
				s.capture = nlCapture{id: "attack", slot: 0}
				s.updateControls(screenkit.Input{Keys: []ebiten.Key{ebiten.KeyBackspace}})
				want = nil
			case "clear":
				s.clearKey("attack", 0)
				want = nil
			case "reset-row":
				s.resetKey("attack")
			case "reset-all":
				s.resetKeys()
			case "profile":
				s.chooseProfile(1)
				want = before // Choosing a profile keeps authored overrides.
			case "preset":
				s.applyPresetToDraft(nlPresetEntry{name: "Keys", patch: json.RawMessage(`{"keyBindings":{"profile":"retail"}}`)}, []bool{false, false, true})
			}
			if s.dialog != "override" || !slices.Equal(s.draft.keys.Keys("attack"), before) {
				t.Fatalf("edit bypassed lock: dialog %q, keys %v", s.dialog, s.draft.keys.Keys("attack"))
			}
			s.confirmOverride()
			if !slices.Equal(s.draft.keys.Keys("attack"), want) {
				t.Fatalf("confirmed edit lost target: got %v, want %v", s.draft.keys.Keys("attack"), want)
			}
			s.apply()
			if !g.lockOverridden(g.cs.mod) || !slices.Equal(g.liveKeyMap().Keys("attack"), want) {
				t.Fatalf("Apply lost approved edit: override %v, keys %v", g.lockOverridden(g.cs.mod), g.liveKeyMap().Keys("attack"))
			}
		})
	}
}

func TestNLScreenGraphicsPresetIncludesSmoothEdges(t *testing.T) {
	_, s := settingsRegressionScreen(nil, settings.Defaults())
	s.applyPresetToDraft(nlPresetEntry{name: "Edges", patch: json.RawMessage(`{"presentation":{"supersample":0},"switchAlt":1}`)}, []bool{false, true, false})
	if s.draft.pres.Supersample != 0 || s.draft.switchAlt {
		t.Fatalf("graphics scope: smooth edges %d, digit keys %v", s.draft.pres.Supersample, s.draft.switchAlt)
	}
	for _, path := range []string{"presentation.supersample", "presentation.arrival", "presentation.placementWeaponRanges"} {
		if !slices.Contains(nlGraphicsPaths(), path) || slices.Contains(nlControlsPaths(), path) {
			t.Fatalf("graphics preference misclassified: %s", path)
		}
	}
}

func TestNLScreenEffectsWheelChangesSelectedPart(t *testing.T) {
	_, s := settingsRegressionScreen(nil, settings.Defaults())
	for _, card := range s.effectCards() {
		if card.kind != nlGroup {
			continue
		}
		for i, part := range card.parts {
			s.draft = s.draftOf(settings.Defaults())
			s.partSel[card.key] = i
			for _, delta := range []int{1, -1} {
				before := s.draft
				s.step(card, card.get(&s.draft), delta)
				for j, other := range card.parts {
					want := other.get(&before)
					if i == j {
						want = max(0, min(len(part.steps)-1, want+delta))
					}
					if got := other.get(&s.draft); got != want {
						t.Fatalf("%s/%s wheel %d changed %s: got %d want %d", card.key, part.label, delta, other.label, got, want)
					}
				}
			}
		}
	}
}

func TestNLScreenScrollViewsKeepWheelPosition(t *testing.T) {
	oldW, oldH := nlScreenW, nlScreenH
	nlScreenW, nlScreenH = 1560, 900
	t.Cleanup(func() { nlScreenW, nlScreenH = oldW, oldH })
	g, s := settingsRegressionScreen(nil, settings.Defaults())
	s.fonts = screenkit.LoadFonts()
	s.art = &nlArt{}
	s.ctlGroup, s.ctlScroll = 1, 1
	s.drawKeyTable(ebiten.NewImage(900, 500), screenkit.Rect{W: 700, H: 300})
	if s.ctlScroll != 1 {
		t.Fatalf("drawing reset controls scroll: %d", s.ctlScroll)
	}
	s.selectControlRow(len(s.controlActions()) - 1)
	if s.ctlRow >= s.ctlScroll+s.controlVisible(s.ctlTable) {
		t.Fatal("keyboard selection is not visible")
	}
	for i := 0; i < 16; i++ {
		g.presets = append(g.presets, settings.Preset{Name: fmt.Sprintf("Preset %d", i)})
	}
	s.presetTop = 1
	s.drawPresets(ebiten.NewImage(1560, 900))
	if s.presetTop != 1 {
		t.Fatalf("drawing reset presets scroll: %d", s.presetTop)
	}
	s.updatePresets(screenkit.Input{X: s.presetList.X + 20, Y: s.presetList.Y + 20, WheelY: -1})
	if s.presetTop != 2 {
		t.Fatalf("preset wheel did not scroll: %d", s.presetTop)
	}
	s.updatePresets(screenkit.Input{Keys: []ebiten.Key{ebiten.KeyEnd}})
	if s.presetSel != len(s.presetEntries())-1 || s.presetSel >= s.presetTop+s.presetVisible() {
		t.Fatalf("last preset not reachable: selected %d, first visible %d", s.presetSel, s.presetTop)
	}
}

func TestNLScreenMouseScrollMakesLastControlReachable(t *testing.T) {
	oldW, oldH := nlScreenW, nlScreenH
	nlScreenW, nlScreenH = 1560, 900
	t.Cleanup(func() { nlScreenW, nlScreenH = oldW, oldH })
	_, s := settingsRegressionScreen(nil, settings.Defaults())
	s.fonts = screenkit.LoadFonts()
	s.art = &nlArt{}
	rows := s.controlRows()
	s.ctlScroll = len(rows)
	r := screenkit.Rect{W: 700, H: 606}
	s.hits.Begin()
	s.drawMouseRows(ebiten.NewImage(900, 700), r)
	s.hits.End()
	if s.ctlScroll == 0 || s.ctlTable != r {
		t.Fatal("Mouse table did not keep a scrollable viewport")
	}
	last := len(rows) - 1 - s.ctlScroll
	s.hits.Update(screenkit.Input{X: 285, Y: 8 + float64(last)*58 + 26}, 0)
	if s.hits.Hot() != "ctl-snapkey-0" {
		t.Fatalf("last Mouse control is unreachable: hovered %q", s.hits.Hot())
	}
}

func TestModPlacementRangeRecommendationPrecedence(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		mod := &modlibrary.Mod{Metadata: modlibrary.Metadata{Config: &modlibrary.Config{
			Content: contentprofiles.Profile{Presentation: contentprofiles.Presentation{PlacementWeaponRanges: &enabled}},
		}}}
		patch, err := settings.Restrict(modRecommendations(mod), []string{"presentation.placementWeaponRanges"})
		want := fmt.Sprintf(`{"presentation":{"placementWeaponRanges":%d}}`, onOff(enabled))
		if err != nil || string(patch) != want {
			t.Fatalf("content recommendation: %s, %v, want %s", patch, err, want)
		}
		mod.Config.Settings = json.RawMessage(fmt.Sprintf(`{"presentation":{"placementWeaponRanges":%d}}`, onOff(!enabled)))
		patch, err = settings.Restrict(modRecommendations(mod), []string{"presentation.placementWeaponRanges"})
		want = fmt.Sprintf(`{"presentation":{"placementWeaponRanges":%d}}`, onOff(!enabled))
		if err != nil || string(patch) != want {
			t.Fatalf("authored settings precedence: %s, %v, want %s", patch, err, want)
		}
	}
}
