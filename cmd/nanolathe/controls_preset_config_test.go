package main

import (
	"slices"
	"testing"

	"github.com/nanolathe-gg/nanolathe/internal/modlibrary"
	"github.com/nanolathe-gg/nanolathe/internal/settings"
)

// presetSettingsPaths are the settings-document paths the preset rows write;
// the keyboard row writes the config's keys section instead.
var presetSettingsPaths = []string{
	"audio.cdMode", "audio.mixingBuffers", "audio.soundMode", "clock",
	"presentation.alliedDotSwatches", "presentation.communityCounters", "presentation.communitySelection",
	"presentation.doubleClickSelection", "presentation.factoryHundredBatch", "presentation.groupNumbers",
	"presentation.megamapAntiNukeMinimum", "presentation.megamapDoubleClickMove", "presentation.megamapFlash",
	"presentation.megamapRadarMinimum", "presentation.megamapSonarJamMinimum", "presentation.megamapSonarMinimum",
	"presentation.megamapWheel", "presentation.megamapWheelMove", "presentation.overview",
	"presentation.playerDotColors", "presentation.queuedOrderDrag", "presentation.reloadBars",
	"presentation.veteranLabels", "presentation.victoryCue", "presentation.weatherReport",
	"skirmish.numPlayers", "switchAlt",
}

// TestShippedConfigSettingsAreThePresetRows proves the ProTA and TA Zero
// configs' settings and keys sections are exactly their presets' columns of
// controlsPresetRows: applied over the defaults, every row the preset
// assigns reads the preset's value, every row it leaves alone keeps the
// default, and the document sets nothing else. Escalation and Mayhem name
// no preset and carry neither section (docs/DESIGN_MODS_MUTATORS.md §4.2,
// §4.3).
func TestShippedConfigSettingsAreThePresetRows(t *testing.T) {
	for _, tc := range []struct{ dir, preset string }{
		{"prota-4.8", controlsPresetCommunity},
		{"ta-zero-alpha5-20241224", controlsPresetZero},
	} {
		t.Run(tc.dir, func(t *testing.T) {
			meta, err := modlibrary.ReadConfigFile(shippedConfigPath(t, tc.dir))
			if err != nil {
				t.Fatal(err)
			}
			config := meta.Config
			if meta.Controls != tc.preset || config.Keys == nil || config.Keys.Profile != tc.preset || len(config.Keys.Bindings) != 0 {
				t.Fatalf("controls %q, keys %+v; want the %s keyboard profile and nothing rebound", meta.Controls, config.Keys, tc.preset)
			}
			s := settings.Defaults()
			if err := config.ApplySettings(&s); err != nil {
				t.Fatal(err)
			}
			s.KeyBindings = *config.Keys
			g := presetTestShell(t)
			g.applySettings(s)
			fresh := presetTestShell(t)
			fresh.applySettings(settings.Defaults())

			assigned := 0
			for _, row := range controlsPresetRows {
				want := row.presetValue(tc.preset)
				if want == presetUnchanged {
					if got, def := row.get(g), row.get(fresh); got != def {
						t.Errorf("%s: config sets %d, but the %s preset leaves it at the default %d", row.label, got, tc.preset, def)
					}
					continue
				}
				assigned++
				if got := row.get(g); got != want {
					t.Errorf("%s = %d, want the %s preset's %d", row.label, got, tc.preset, want)
				}
			}
			paths := config.SettingsPaths()
			for _, path := range paths {
				if !slices.Contains(presetSettingsPaths, path) {
					t.Errorf("config sets %s, which no preset row writes", path)
				}
			}
			// One row, Keyboard, is the keys section rather than a setting.
			if len(paths) != assigned-1 {
				t.Errorf("config sets %d settings for the %d rows the %s preset assigns besides Keyboard: %v", len(paths), assigned-1, tc.preset, paths)
			}
		})
	}
	for _, dir := range []string{"escalation-10.2.0", "mayhem-11.3.0"} {
		meta, err := modlibrary.ReadConfigFile(shippedConfigPath(t, dir))
		if err != nil {
			t.Fatal(err)
		}
		if meta.Controls != "" || meta.Config.Settings != nil || meta.Config.Keys != nil {
			t.Errorf("%s names settings or keys: controls %q", dir, meta.Controls)
		}
	}
}
