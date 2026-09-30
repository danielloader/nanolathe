//go:build retail

package main

import (
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nanolathe-gg/nanolathe/internal/camera"
	"github.com/nanolathe-gg/nanolathe/internal/client"
	"github.com/nanolathe-gg/nanolathe/internal/frame"
	"github.com/nanolathe-gg/nanolathe/internal/gameplay"
	"github.com/nanolathe-gg/nanolathe/internal/settings"
	"github.com/nanolathe-gg/nanolathe/internal/testsupport"
)

// presetRetailShell opens a windowed-style shell on opts with a client bound
// for captures, with its settings attached and writable.
func presetRetailShell(t *testing.T, opts Options) (*gameShell, *client.Client) {
	t.Helper()
	cs, err := openContent(opts)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cs.Close() })
	opts.Root, opts.Roots = cs.root, cs.roots
	shell, err := newGameShell(opts, cs)
	if err != nil {
		t.Fatal(err)
	}
	shell.attachSettings()
	shell.enforceModGameplayMinimum()
	shell.cam = &camera.Camera{ViewW: retailScreenW, ViewH: retailScreenH, MapW: retailScreenW, MapH: retailScreenH}
	cl, err := client.New(client.Options{Buffer: &frame.Buffer{}, Width: retailScreenW, Height: retailScreenH})
	if err != nil {
		t.Fatal(err)
	}
	saved := clPtr
	clPtr = cl
	t.Cleanup(func() { clPtr = saved })
	if shell.assets != nil && shell.assets.pal != nil {
		cl.SetPalette(shell.assets.pal)
	}
	if shell.font != nil {
		cl.SetFNT(shell.font)
	}
	cl.SetUIStage(gameShellUIStage{shell: shell})
	return shell, cl
}

func presetCapture(t *testing.T, cl *client.Client, name string) {
	t.Helper()
	dir := os.Getenv("NANOLATHE_MODS_CAPTURE")
	if dir == "" {
		return
	}
	f, err := os.Create(filepath.Join(dir, name+".png"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, cl.ComposeFrame()); err != nil {
		t.Fatal(err)
	}
}

// TestProTARecommendedSettingsAreItsLayer installs the ProTA package with its
// repository config, as a download or a drop of the hosted zip does. The
// config's settings, keys and recommended rules are ProTA's settings layer
// (DESIGN_MODS_MUTATORS §4.6): a start with --mod plays them with nothing
// offered, a change made while ProTA runs is kept for ProTA alone, and the
// original game keeps the base settings. The loading screen names the unit
// limit ProTA's feature table sets.
func TestProTARecommendedSettingsAreItsLayer(t *testing.T) {
	prota := os.Getenv("NANOLATHE_MOD_ROOTS_PROTA")
	if prota == "" {
		t.Skip("NANOLATHE_MOD_ROOTS_PROTA is unset")
	}
	retail := testsupport.RetailRoot(t)
	settingsPath := filepath.Join(t.TempDir(), "settings.json")
	t.Setenv(settings.EnvPath, settingsPath)
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Cleanup(func() { applyRetailAudioOptions(settings.DefaultAudio()) })
	devnull, _ := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	defer devnull.Close()
	if err := runInstallMod(Options{Root: retail, InstallMod: stageModPackage(t, filepath.SplitList(prota)[0], "prota-4.8")}, devnull); err != nil {
		t.Fatal(err)
	}
	lib, err := openModLibrary()
	if err != nil {
		t.Fatal(err)
	}
	installed, err := lib.Installed()
	if err != nil || len(installed) != 1 {
		t.Fatalf("installed %v, %v", installed, err)
	}
	id := installed[0].ID

	shell, cl := presetRetailShell(t, Options{Root: retail, Mod: id, ModSet: true})
	if shell.cs.mod == nil || shell.cs.mod.Controls != "community" || shell.cs.mod.MinimumGameplay != string(gameplay.Community39) {
		t.Fatalf("the mounted package = %+v", shell.cs.mod)
	}
	if shell.presentation.CommunitySelection != 1 || !shell.switchAlt || shell.audioPrefs.SoundMode != settings.SoundMode3D || shell.gameplay != gameplay.Community39 {
		t.Fatalf("ProTA's recommendations are not its layer: selection %d, switchAlt %v, sound %d, gameplay %s",
			shell.presentation.CommunitySelection, shell.switchAlt, shell.audioPrefs.SoundMode, shell.gameplay)
	}
	if shell.liveKeyMap().Profile() != "community" {
		t.Fatalf("keyboard profile %q, want ProTA's community", shell.liveKeyMap().Profile())
	}
	shell.presentation.CommunitySelection = 2
	shell.saveSettings()
	stored, err := settings.Load()
	if err != nil {
		t.Fatal(err)
	}
	if stored.Presentation.CommunitySelection != 0 || stored.SwitchAlt != 0 || len(stored.ModSettings[id]) == 0 {
		t.Fatalf("the base took ProTA's settings: selection %d, switchAlt %d, patch %s", stored.Presentation.CommunitySelection, stored.SwitchAlt, stored.ModSettings[id])
	}
	plain, _ := presetRetailShell(t, Options{Root: retail, Mod: "none", ModSet: true})
	if plain.presentation.CommunitySelection != 0 || plain.switchAlt {
		t.Fatal("the original game plays ProTA's settings")
	}
	shell.openMenu(modeMenuMain)
	presetCapture(t, cl, "prota-menu")

	shell.loading = newLoadingState("Comet Catcher")
	shell.frontend.SetMode(modeLoading)
	if line := shell.loadingSelectionLines()[0]; !strings.Contains(line, "Unit limit 1500 (set by ") {
		t.Fatalf("loading line %q does not name the effective unit limit", line)
	}
	presetCapture(t, cl, "loading")
	shell.setGameplay(gameplay.Strict31)
	shell.loading = newLoadingState("Comet Catcher")
	if line := shell.loadingSelectionLines()[0]; strings.Contains(line, "set by") {
		t.Fatalf("Strict 3.1 ignores the feature table, but the loading line reads %q", line)
	}
}
