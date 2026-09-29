package main

import (
	"path/filepath"
	"testing"

	"github.com/nanolathe-gg/nanolathe/internal/audio"
	"github.com/nanolathe-gg/nanolathe/internal/gui"
	"github.com/nanolathe-gg/nanolathe/internal/input"
	"github.com/nanolathe-gg/nanolathe/internal/settings"
	"github.com/nanolathe-gg/nanolathe/vfs"
)

// The selected track's type is an editable list entry, not the desired battle
// category [03 R-AUD-01 §4]. Mode changes must preserve that distinction.
func TestMusicOptionsModeKeepsBattleCategory(t *testing.T) {
	prior := optionsState
	t.Cleanup(func() { optionsState = prior })
	fs := vfs.New()
	defer fs.Close()
	svc := audio.NewService(fs)
	svc.Music.Open(16)
	svc.Music.Configure(audio.ModeSequential, 0)
	shell := &gameShell{cs: testContentSet(fs), audioOwner: svc, audioPrefs: settings.DefaultAudio()}
	optionsState = &retailOptionsState{track: 1}
	optionsState.categories[0] = 3
	shell.applyRetailMusicMode()
	if svc.Music.DesiredCategory() != 0 {
		t.Fatal("mode change replaced Building with selected track type")
	}
	if svc.Music.TrackCategory(1) != 3 {
		t.Fatal("category edits did not reach playback controller")
	}
}

func TestMusicPreferencesApplyToAnExistingFrontendOwner(t *testing.T) {
	defer audio.ConfigureOutput(audio.OutputConfig{MasterEnabled: true, EffectsVolume: 1, SoundMode: audio.SoundModeMono})
	svc := audio.NewService(nil)
	svc.Music.Configure(audio.ModeCategoryShuffle, 4)
	shell := &gameShell{setup: newSkirmishMenuConfig(""), audioOwner: svc}
	stored := settings.Defaults()
	stored.Audio.MusicVol = 12
	stored.Audio.MusicMode = 0
	shell.applySettings(stored)
	if svc.Music.Volume() != 12 || svc.Music.IsEnabled() || svc.Music.DesiredCategory() != 4 {
		t.Fatal("saved music preferences lost after frontend audio initialization")
	}
}

// NOTRAK is a staged `Off|On` button, so it shows the music bit in its
// current-stage byte, and a click sets the value it then shows
// [03 R-AUD-01 §4][07 R-WGT-01 §3]. The regression wrote the bit into the
// down-state word, so the caption stayed on stage 0: with music on the switch
// read `Off`, and setting it back to `Off` turned the music on again.
func TestMusicSwitchShowsAndSetsTheMusicBit(t *testing.T) {
	fs := vfs.New()
	defer fs.Close()
	svc := audio.NewService(fs)
	svc.Music.Open(16)
	g, panel, cl := syntheticOptionsPage(t, "music", func(w *gui.Window) error {
		w.Gadgets = append(w.Gadgets, gui.Gadget{Kind: gui.KindButton, Name: "NOTRAK", Active: 1, Rect: gui.Rect{X: 200, Y: 40, W: 80, H: 20}, Stages: 2, Text: "Off|On"})
		return nil
	})
	g.cs, g.audioOwner = testContentSet(fs), svc
	notrak := panel.Index("NOTRAK")
	caption := func() string { return retailGadgetText(panel, notrak, panel.Window.Gadgets[notrak]) }
	g.syncRetailMusicPage()
	if caption() != "On" || panel.DownAt(notrak) != 0 {
		t.Fatalf("with music on the page opened showing %q (down %d); want On, released", caption(), panel.DownAt(notrak))
	}
	for _, want := range []struct {
		caption string
		mode    int
	}{{"Off", 0}, {"On", 1}, {"Off", 0}} {
		clickRowGadget(t, g, panel, cl, "NOTRAK", input.MouseButtonLeft)
		if g.audioPrefs.MusicMode != want.mode || svc.Music.IsEnabled() != (want.mode != 0) || caption() != want.caption {
			t.Fatalf("NOTRAK shows %q with musicmode %d and the controller enabled=%v; want %q and %d",
				caption(), g.audioPrefs.MusicMode, svc.Music.IsEnabled(), want.caption, want.mode)
		}
	}
	// A reopened page starts from a fresh stage byte and shows the stored bit.
	panel.SetStageAt(notrak, 1)
	g.syncRetailMusicPage()
	if caption() != "Off" {
		t.Fatalf("with music off the page opened showing %q; want Off", caption())
	}
}

// The same contract on the authored MUSIC page, through real clicks: the switch
// opens showing the stored bit, a click turns the music off and says so, a page
// reopen keeps showing Off, and OK saves it [03 R-AUD-01 §4][07 R-FE-01 §6].
// Skipped when the retail assets are not opted in.
func TestMusicSwitchRetailPage(t *testing.T) {
	g, _, cl := retailAssetShell(t)
	previous := clPtr
	clPtr = cl
	t.Cleanup(func() { clPtr = previous })
	t.Setenv(settings.EnvPath, filepath.Join(t.TempDir(), "settings.json"))
	g.attachSettings()
	t.Cleanup(g.closeRetailOptionsScreen)
	g.openMenu(modeMenuSingle)
	g.activateGadget("Options")
	music := g.retailMusicController()
	caption := func() string {
		i := optionsPanel.Index("NOTRAK")
		return retailGadgetText(optionsPanel, i, optionsPanel.Window.Gadgets[i])
	}
	clickRowGadget(t, g, optionsPanel, cl, "MUSIC", input.MouseButtonLeft)
	if caption() != "On" || !music.IsEnabled() {
		t.Fatalf("with music on the MUSIC page shows %q (enabled=%v); want On", caption(), music.IsEnabled())
	}
	clickRowGadget(t, g, optionsPanel, cl, "NOTRAK", input.MouseButtonLeft)
	clickRowGadget(t, g, optionsPanel, cl, "SOUND", input.MouseButtonLeft)
	clickRowGadget(t, g, optionsPanel, cl, "MUSIC", input.MouseButtonLeft)
	if caption() != "Off" || music.IsEnabled() || g.audioPrefs.MusicMode != 0 {
		t.Fatalf("after turning music off the reopened page shows %q (enabled=%v, musicmode %d); want Off", caption(), music.IsEnabled(), g.audioPrefs.MusicMode)
	}
	clickRowGadget(t, g, optionsPanel, cl, "PREV", input.MouseButtonLeft)
	saved, err := settings.Load()
	if err != nil {
		t.Fatal(err)
	}
	if saved.Audio.MusicMode != 0 {
		t.Fatalf("OK saved musicmode %d; want 0", saved.Audio.MusicMode)
	}
}
