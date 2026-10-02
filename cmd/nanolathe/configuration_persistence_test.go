package main

import (
	"encoding/json"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/nanolathe-gg/nanolathe/internal/input"
	"github.com/nanolathe-gg/nanolathe/internal/modlibrary"
	"github.com/nanolathe-gg/nanolathe/internal/settings"
)

// A saved config is an independent mount input, even while a library mod's
// config takes precedence (DESIGN_MODS_MUTATORS §4.3). Unrelated saves and
// reloads must retain the player's path for the next base-content start.
func TestSettingsRoundTripKeepsContentConfig(t *testing.T) {
	for _, mod := range []*modlibrary.Mod{nil, {Metadata: modlibrary.Metadata{ID: "configured-mod"}}} {
		name := "base"
		if mod != nil {
			name = mod.ID
		}
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "settings.json")
			t.Setenv(settings.EnvPath, path)
			file := settings.Defaults()
			file.ContentProfile = filepath.Join(t.TempDir(), "nanolathe-mod.json")
			g, s := settingsRegressionScreen(mod, file)
			g.settingsWritable = true
			if got := g.liveSettings().ContentProfile; got != file.ContentProfile {
				t.Fatalf("live settings lost saved config: %q", got)
			}
			s.draft.pres.WaterSurface = 0
			s.touched["water"] = true
			s.apply()
			stored, err := settings.Load()
			if err != nil || stored.ContentProfile != file.ContentProfile {
				t.Fatalf("Apply lost saved config: %q, %v", stored.ContentProfile, err)
			}
			next := &gameShell{cs: &contentSet{}}
			next.adoptLiveSettings(g)
			if got := next.captureSettings().ContentProfile; got != file.ContentProfile {
				t.Fatalf("content reload lost saved config: %q", got)
			}
			restarted, _ := settingsRegressionScreen(nil, stored)
			if got := restarted.captureSettings().ContentProfile; got != file.ContentProfile {
				t.Fatalf("restart lost saved config: %q", got)
			}
		})
	}
}

// Saving the draft is a separate library write: it must capture the same
// mod-scoped result Apply will install without installing it itself
// (DESIGN_INTERFACE_HUD_INPUT §3.17, DESIGN_MODS_MUTATORS §4.6).
func checkExportedDraftMatchesApply(t *testing.T, g *gameShell, s *nlScreen) settings.Settings {
	t.Helper()
	t.Setenv(settings.EnvPath, filepath.Join(t.TempDir(), "settings.json"))
	g.settingsWritable = true
	before := g.liveSettings()
	s.savePreset("Pending settings")
	after := g.liveSettings()
	after.Presets = before.Presets
	if !reflect.DeepEqual(before, after) {
		t.Fatal("saving a draft preset changed the live preferences")
	}
	stored, err := settings.Load()
	if err != nil || len(stored.Presets) != 1 {
		t.Fatalf("saved presets: %v, %v", stored.Presets, err)
	}
	exported, err := settings.Layer(settings.Defaults(), stored.Presets[0].Settings)
	if err != nil {
		t.Fatal(err)
	}
	s.apply()
	diff, err := settings.Diff(g.liveSettings(), exported, settings.ModScoped)
	if err != nil || len(diff) != 0 {
		t.Fatalf("saved draft differs from Apply: %s, %v", diff, err)
	}
	return exported
}

func TestNLPresetExportKeepsPendingNonCardValues(t *testing.T) {
	file := settings.Defaults()
	file.Presentation.FPS = 73
	file.Presentation.BuildMenuPageSize = 10
	file.Presentation.GroundLightStrength = 137
	g, s := settingsRegressionScreen(nil, file)
	s.applyPresetToDraft(nlPresetEntry{name: "Rules and sound", patch: json.RawMessage(`{"gameplayFeatures":{"unitLimit":1777},"clock":1,"audio":{"soundMode":2,"mixingBuffers":128,"cdMode":2},"skirmish":{"numPlayers":10}}`)}, []bool{true, false, true})
	// A second, unrelated preset must not forget the first one's invisible
	// rows when the combined draft is saved under a new name.
	s.applyPresetToDraft(nlPresetEntry{name: "Water", patch: json.RawMessage(`{"presentation":{"waterSurface":0}}`)}, []bool{false, true, false})
	exported := checkExportedDraftMatchesApply(t, g, s)
	if exported.GameplayFeatures.UnitLimit == nil || *exported.GameplayFeatures.UnitLimit != 1777 || exported.Audio.MixingBuffers != 128 || exported.Clock != 1 || exported.Skirmish.NumPlayers != 10 {
		t.Fatal("draft export lost pending fields with no card")
	}
	if exported.Presentation.FPS != 73 || exported.Presentation.BuildMenuPageSize != 10 || exported.Presentation.GroundLightStrength != 137 {
		t.Fatal("draft export normalized untouched custom preferences")
	}
}

func TestNLPresetExportMatchesPendingControlsProfile(t *testing.T) {
	for profile := 1; profile < len(nlControlsPresets); profile++ {
		t.Run(nlControlsPresets[profile].label, func(t *testing.T) {
			file := settings.Defaults()
			file.Presentation.ZoomLockPercent = 137
			file.Presentation.MegamapSonarMinimum = 777
			file.Presentation.PlayerDotColors[0] = 42
			file.Clock = 1
			file.Audio.SoundMode, file.Audio.MixingBuffers, file.Audio.CDMode = 0, 33, 3
			file.Skirmish.NumPlayers = 5
			file.KeyBindings.Bindings = map[string][]string{"attack": {"q"}}
			g, s := settingsRegressionScreen(nil, file)
			s.chooseProfile(profile)
			// Explicit card and key changes have precedence over the profile.
			for _, c := range s.controlCards() {
				if c.key == "selection" {
					s.setCard(c, 2)
				}
			}
			s.draft.keys.Rebind("stop", []input.Chord{{Key: input.KeyW}})
			s.touched["keys"] = true
			exported := checkExportedDraftMatchesApply(t, g, s)
			if exported.Presentation.ZoomLockPercent != 137 {
				t.Fatal("profile export changed unassigned zoom preference")
			}
		})
	}
}

func TestNLPresetExportKeepsProfileThenPresetPrecedence(t *testing.T) {
	g, s := settingsRegressionScreen(nil, settings.Defaults())
	s.chooseProfile(2)
	s.applyPresetToDraft(nlPresetEntry{name: "Quiet", patch: json.RawMessage(`{"audio":{"soundMode":0},"clock":0,"presentation":{"communitySelection":2}}`)}, []bool{false, false, true})
	exported := checkExportedDraftMatchesApply(t, g, s)
	if exported.Audio.SoundMode != 0 || exported.Clock != 0 || exported.Presentation.CommunitySelection != 2 || exported.Audio.MixingBuffers != 128 {
		t.Fatal("draft export did not keep profile, preset, then card precedence")
	}
}

func TestNLPresetGraphicsDoNotFreezePendingProfileRows(t *testing.T) {
	g, s := settingsRegressionScreen(nil, settings.Defaults())
	s.chooseProfile(2)
	s.applyPresetToDraft(nlPresetEntry{name: "Water", patch: json.RawMessage(`{"presentation":{"waterSurface":0}}`)}, []bool{false, true, false})
	// A graphics preset does not turn the Community profile's pending rows
	// into explicit card edits that would override a later Retail choice.
	s.chooseProfile(1)
	exported := checkExportedDraftMatchesApply(t, g, s)
	if exported.Presentation.CommunitySelection != 0 || exported.SwitchAlt != 0 || exported.Presentation.FactoryHundredBatch != 0 {
		t.Fatal("graphics preset retained the earlier profile's controls")
	}
}

func TestNLPresetExportResolvesPendingLockOverride(t *testing.T) {
	mod := &modlibrary.Mod{Metadata: modlibrary.Metadata{ID: "locked-audio", Config: &modlibrary.Config{
		Settings: json.RawMessage(`{"audio":{"mixingBuffers":128}}`), Locks: []string{"audio.mixingBuffers"},
	}}}
	file := settings.Defaults()
	file.ModSettings = map[string]json.RawMessage{mod.ID: json.RawMessage(`{"audio":{"mixingBuffers":64}}`)}
	g, s := settingsRegressionScreen(mod, file)
	s.draft.override = true
	exported := checkExportedDraftMatchesApply(t, g, s)
	if exported.Audio.MixingBuffers != 64 {
		t.Fatal("draft export lost the player value restored by lock approval")
	}
}

func TestNLPresetExportUsesPendingContentLayers(t *testing.T) {
	file := settings.Defaults()
	mod := modlibrary.Mod{Metadata: modlibrary.Metadata{ID: "next-content", MinimumGameplay: "community-3.9", Config: &modlibrary.Config{
		Settings: json.RawMessage(`{"presentation":{"glint":0},"audio":{"mixingBuffers":128}}`),
	}}}
	file.ModSettings = map[string]json.RawMessage{mod.ID: json.RawMessage(`{"presentation":{"zoomLockPercent":137},"audio":{"cdMode":2}}`)}
	g, s := settingsRegressionScreen(nil, file)
	s.mods = []modlibrary.Mod{mod}
	s.setCard(s.gameCards()[0], 1)
	s.savePreset("Next content")
	exported, err := settings.Layer(settings.Defaults(), g.presets[0].Settings)
	if err != nil {
		t.Fatal(err)
	}
	if g.contentMod() != nil || g.presentation.Glint != file.Presentation.Glint {
		t.Fatal("saving the target-content preset changed the current content")
	}
	// The reload first adopts settings under the target content's layers,
	// then writes the pending draft through the ordinary Apply transaction.
	next := &gameShell{cs: &contentSet{mod: &mod}, opts: g.opts}
	next.adoptLiveSettings(g)
	s.applyDraft(next, s.draft, s.touched, s.pendingPresets)
	diff, err := settings.Diff(next.liveSettings(), exported, settings.ModScoped)
	if err != nil || len(diff) != 0 {
		t.Fatalf("target-content export differs from reloaded Apply: %s, %v", diff, err)
	}
}

func TestNLCompletePresetsResetAutoLimitAndDefaultKeys(t *testing.T) {
	for _, source := range []string{"Original game", "Saved defaults"} {
		t.Run(source, func(t *testing.T) {
			g, s := settingsRegressionScreen(nil, settings.Defaults())
			s.savePreset("Saved defaults")
			var preset nlPresetEntry
			for _, entry := range s.presetEntries() {
				if entry.name == source {
					preset = entry
				}
			}
			file := settings.Defaults()
			file.UnitLimit = 500
			file.KeyBindings = settings.KeyBindings{Profile: input.ProfileZero, Bindings: map[string][]string{"attack": {"q"}}}
			g.applySettings(file)
			s.draft = s.freshDraft(g)
			s.applyPresetToDraft(preset, []bool{true, true, true})
			s.apply()
			if g.savedUnitLimit != 0 || !keyBindingsSetting(g.keyMap).IsZero() {
				t.Fatalf("preset omitted default resets: unit limit %d, keys %+v", g.savedUnitLimit, keyBindingsSetting(g.keyMap))
			}
		})
	}
}
