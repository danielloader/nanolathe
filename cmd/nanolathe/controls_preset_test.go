package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/nanolathe-gg/nanolathe/internal/modlibrary"
	"github.com/nanolathe-gg/nanolathe/internal/settings"
)

// The lock path must cover the setting the assignment actually writes,
// including keyboard subtrees and settings without a Nanolathe-screen card.
func TestControlsPresetAssignmentPaths(t *testing.T) {
	for _, row := range controlsPresetRows {
		g := presetTestShell(t)
		for _, preset := range []string{controlsPresetCommunity, controlsPresetRetail, controlsPresetZero} {
			value := row.presetValue(preset)
			if value == presetUnchanged {
				continue
			}
			before := g.liveSettings()
			row.set(g, value)
			diff, err := settings.Diff(g.liveSettings(), before, settings.ModScoped)
			if err != nil {
				t.Fatal(err)
			}
			var doc map[string]any
			if len(diff) > 0 {
				if err := json.Unmarshal(diff, &doc); err != nil {
					t.Fatal(err)
				}
			}
			collectPaths(doc, "", func(path string) {
				if path != row.path {
					t.Errorf("%s writes %s, but locks %s", row.label, path, row.path)
				}
			})
		}
	}
}

func presetTestShell(t *testing.T) *gameShell {
	t.Helper()
	t.Cleanup(func() { applyRetailAudioOptions(settings.DefaultAudio()) })
	g := &gameShell{presentation: settings.DefaultPresentation(), audioPrefs: settings.DefaultAudio()}
	g.setup.NumPlayers = settings.DefaultNumPlayers
	return g
}

// TestCommunityControlsPresetContents locks ProTA's recommended settings
// (docs/DESIGN_MODS_MUTATORS.md §4.3): the Community host options plus the
// preferences ProTA.ini pins, each written as its own stored value.
func TestCommunityControlsPresetContents(t *testing.T) {
	g := presetTestShell(t)
	g.applyControlsPreset(controlsPresetCommunity)
	p := g.presentation
	for name, value := range map[string]int{
		"communitySelection": p.CommunitySelection, "doubleClickSelection": p.DoubleClickSelection,
		"factoryHundredBatch": p.FactoryHundredBatch, "queuedOrderDrag": p.QueuedOrderDrag, "communityCounters": p.CommunityCounters,
		"reloadBars": p.ReloadBars, "veteranLabels": p.VeteranLabels, "groupNumbers": p.GroupNumbers,
		"weatherReport": p.WeatherReport, "overview": p.Overview, "megamapWheel": p.MegamapWheel,
		"megamapWheelMove": p.MegamapWheelMove, "megamapFlash": p.MegamapFlash, "victoryCue": p.VictoryCue,
		"alliedDotSwatches": p.AlliedDotSwatches,
	} {
		if value != 1 {
			t.Errorf("presentation.%s = %d, want 1", name, value)
		}
	}
	if p.MegamapDoubleClickMove != 0 || p.MegamapRadarMinimum != 0 || p.MegamapSonarMinimum != 0 || p.MegamapSonarJamMinimum != 0 || p.MegamapAntiNukeMinimum != 0 {
		t.Error("the megamap double-click move and ring minimums are not ProTA's zeros")
	}
	if p.PlayerDotColors != [10]int{227, 249, 18, 250, 67, 149, 208, 117, 210, 34} {
		t.Errorf("dot colours = %v, want ProTA.ini's", p.PlayerDotColors)
	}
	if !g.switchAlt || !g.clockVisible {
		t.Errorf("switchAlt %v, clock %v; want both set", g.switchAlt, g.clockVisible)
	}
	if a := g.audioPrefs; a.SoundMode != settings.SoundMode3D || a.MixingBuffers != 128 || a.CDMode != 2 {
		t.Errorf("audio = sound mode %d, mixing buffers %d, cd mode %d; want 2, 128, 2", a.SoundMode, a.MixingBuffers, a.CDMode)
	}
	if g.setup.NumPlayers != settings.MaxPlayers {
		t.Errorf("skirmish rows = %d, want %d", g.setup.NumPlayers, settings.MaxPlayers)
	}
	// Nothing outside the table moves: the player's other choices stay.
	if p.AlliedResources != 0 || p.Renderer != settings.DefaultPresentation().Renderer || g.audioPrefs.FXVol != settings.DefaultFXVol {
		t.Error("the preset wrote a setting outside its table")
	}
	for _, row := range controlsPresetRows {
		if want := row.presetValue(controlsPresetCommunity); want != presetUnchanged && row.get(g) != want {
			t.Errorf("%s = %s after applying, want %s", row.label, row.valueText(row.get(g)), row.valueText(want))
		}
	}
}

// TestRetailControlsPresetKeepsSkirmishRows: the retail preset restores the
// retail defaults but never removes skirmish rows.
func TestRetailControlsPresetKeepsSkirmishRows(t *testing.T) {
	g := presetTestShell(t)
	g.applyControlsPreset(controlsPresetCommunity)
	g.applyControlsPreset(controlsPresetRetail)
	p := g.presentation
	if p.CommunitySelection != 0 || p.FactoryHundredBatch != 0 || p.QueuedOrderDrag != 0 || g.switchAlt || g.clockVisible {
		t.Error("the retail preset left a Community option on")
	}
	if p.GroupNumbers != 1 {
		t.Error("the retail preset suppressed the group digits retail draws")
	}
	if a := g.audioPrefs; a.SoundMode != settings.DefaultSoundMode || a.MixingBuffers != settings.DefaultMixingBuffers || a.CDMode != settings.DefaultCDMode {
		t.Errorf("audio = %+v, want the retail defaults", a)
	}
	if g.setup.NumPlayers != settings.MaxPlayers {
		t.Errorf("the retail preset changed the skirmish rows to %d", g.setup.NumPlayers)
	}
	if p.Overview != settings.OverviewZoom || p.VictoryCue != 0 || p.AlliedDotSwatches != 0 || p.PlayerDotColors != settings.DefaultPlayerDotColors {
		t.Errorf("overview %d, victory cue %d, dot colours %v; want Zoom, off and the draw engine's defaults", p.Overview, p.VictoryCue, p.PlayerDotColors)
	}
	// The megamap's own preferences are the player's, whichever overview.
	g.presentation.MegamapFlash, g.presentation.MegamapRadarMinimum = 0, 64
	g.applyControlsPreset(controlsPresetRetail)
	if g.presentation.MegamapFlash != 0 || g.presentation.MegamapRadarMinimum != 64 {
		t.Error("the retail preset changed a megamap preference")
	}
	p = g.presentation
	g.applyControlsPreset("unknown")
	if g.presentation != p {
		t.Error("an unknown preset changed a setting")
	}
}

// TestDotColoursRowRoundTrip: the dot colour table is one row naming its
// two tables. Each survives a settings round trip, and any other table
// reads as Custom, which the ProTA preset replaces.
func TestDotColoursRowRoundTrip(t *testing.T) {
	var row controlsPresetRow
	for _, r := range controlsPresetRows {
		if r.label == "Dot colours" {
			row = r
		}
	}
	if row.get == nil {
		t.Fatal("no Dot colours row")
	}
	t.Setenv(settings.EnvPath, filepath.Join(t.TempDir(), "settings.json"))
	for _, preset := range []string{controlsPresetCommunity, controlsPresetRetail, controlsPresetZero} {
		g := presetTestShell(t)
		g.applyControlsPreset(preset)
		stored := settings.Defaults()
		stored.Presentation = g.presentation
		if err := stored.Save(); err != nil {
			t.Fatal(err)
		}
		loaded, err := settings.Load()
		if err != nil {
			t.Fatal(err)
		}
		g.presentation = loaded.Presentation
		if got, want := row.get(g), row.presetValue(preset); got != want {
			t.Errorf("%s: reloaded row = %s, want %s", preset, row.valueText(got), row.valueText(want))
		}
	}
	g := presetTestShell(t)
	g.presentation.PlayerDotColors[3] = 1
	if got := row.get(g); got != presetCustom || row.valueText(got) != "Custom" {
		t.Fatalf("an edited table reads %d", got)
	}
}

// Zero's Alpha 5 INI differs from ProTA's in the dot palette and independent
// range thresholds. Applying the recommendation must leave undocumented
// preferences alone (research/extensions/ta-zero-engine.md).
func TestZeroControlsPresetContents(t *testing.T) {
	g := presetTestShell(t)
	g.applyControlsPreset(controlsPresetCommunity)
	g.presentation.MegamapRadarMinimum = 123
	g.presentation.MegamapSonarMinimum = 234
	g.presentation.MegamapSonarJamMinimum = 345
	g.presentation.MegamapAntiNukeMinimum = 456
	g.applyControlsPreset(controlsPresetZero)
	p := g.presentation
	if p.CommunitySelection != 2 || p.FactoryHundredBatch != 1 {
		t.Fatal("Zero selection and independent hundred batch were not offered")
	}
	if p.PlayerDotColors != [10]int{227, 212, 80, 235, 198, 219, 208, 93, 36, 67} {
		t.Fatalf("Zero dot colours = %v", p.PlayerDotColors)
	}
	if p.MegamapRadarMinimum != 0 || p.MegamapSonarMinimum != 500 || p.MegamapSonarJamMinimum != 0 || p.MegamapAntiNukeMinimum != 512 {
		t.Fatal("Zero's independent sensor thresholds were not applied")
	}
	if p.Overview != settings.OverviewMegamap || p.MegamapWheel != 1 || p.MegamapWheelMove != 1 || p.MegamapDoubleClickMove != 0 || p.MegamapFlash != 1 || p.DoubleClickSelection != 1 || !g.switchAlt {
		t.Fatal("Zero's documented selection and megamap settings were not applied")
	}
	if g.audioPrefs.SoundMode != settings.SoundMode3D || g.audioPrefs.MixingBuffers != 128 || g.audioPrefs.CDMode != 2 || g.setup.NumPlayers != 10 {
		t.Fatal("Zero's documented audio and skirmish settings were not applied")
	}
	// Zero's recommendation does not undo the player's independently chosen
	// options merely because its older package did not document them.
	if p.VeteranLabels != 1 || p.WeatherReport != 1 || p.VictoryCue != 1 || p.QueuedOrderDrag != 1 || !g.clockVisible {
		t.Fatal("Zero changed a preference outside its documented recommendation")
	}
	if p.MegamapSonarMinimum != 500 || p.MegamapAntiNukeMinimum != 512 {
		t.Fatalf("Zero ring minimums sonar %d, anti-nuke %d", p.MegamapSonarMinimum, p.MegamapAntiNukeMinimum)
	}
}

// TestStrategicIconDiscoveryOrder: an empty preference searches the running
// mod's directory, or a manual stack from its last root to its first; a root
// without a configuration is passed over silently, the first root holding one
// wins, and an explicit preference always wins.
func TestStrategicIconDiscoveryOrder(t *testing.T) {
	writeConfig := func(root, rel string) string {
		t.Helper()
		path := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("[Option]\nUseDefaultIcon=true\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		return path
	}
	base, first, last := t.TempDir(), t.TempDir(), t.TempDir()
	firstConfig := writeConfig(first, "Icon/iconcfg.ini")

	manual := &contentSet{manualRoots: true, roots: []string{base, first, last}}
	roots := strategicIconSearchRoots(manual)
	if len(roots) != 3 || roots[0] != last || roots[2] != base {
		t.Fatalf("manual stack search order = %q", roots)
	}
	if got, err := automaticStrategicIconConfig(roots); err != nil || got != firstConfig {
		t.Fatalf("found %q/%v, want the only configuration %q", got, err, firstConfig)
	}
	lastConfig := writeConfig(last, "iconcfg.ini")
	if got, err := automaticStrategicIconConfig(roots); err != nil || got != lastConfig {
		t.Fatalf("found %q/%v, want the last root's %q", got, err, lastConfig)
	}
	writeConfig(last, "ZIcon/iconcfg.ini")
	if _, err := automaticStrategicIconConfig(roots); err == nil {
		t.Fatal("an ambiguous root was not reported")
	}

	if got := strategicIconSearchRoots(&contentSet{roots: []string{base}}); got != nil {
		t.Fatalf("the base install alone is searched: %q", got)
	}
	withMod := &contentSet{roots: []string{base, first}, mod: &modlibrary.Mod{Dir: first}}
	if got := strategicIconSearchRoots(withMod); len(got) != 1 || got[0] != first {
		t.Fatalf("mod search roots = %q", got)
	}
	if got, err := automaticStrategicIconConfig([]string{base}); err != nil || got != "" {
		t.Fatalf("a root without a configuration gave %q/%v", got, err)
	}
	if icons, err := battleStrategicIcons(nil, "", []string{base}); icons == nil || err != nil {
		t.Fatalf("none found = %v/%v, want the generated catalog silently", icons, err)
	}
	// The explicit preference is loaded even where discovery would fail.
	if icons, err := battleStrategicIcons(nil, firstConfig, roots); icons == nil || err != nil {
		t.Fatalf("explicit preference = %v/%v", icons, err)
	}
}

// TestRetailPresetColumnIsTheDefaults: each row's retail value is the value a
// fresh settings file holds, so *Restore default settings* restores the
// defaults, and a player who never took a preset has nothing to restore.
func TestRetailPresetColumnIsTheDefaults(t *testing.T) {
	g := presetTestShell(t)
	d := settings.Defaults()
	g.presentation, g.audioPrefs = d.Presentation, d.Audio
	g.switchAlt, g.clockVisible = d.SwitchAltEnabled(), d.ClockEnabled()
	for _, row := range controlsPresetRows {
		if got := row.get(g); row.retail != presetUnchanged && got != row.retail {
			t.Errorf("%s: retail column %s, but the default is %s", row.label, row.valueText(row.retail), row.valueText(got))
		}
	}
}
