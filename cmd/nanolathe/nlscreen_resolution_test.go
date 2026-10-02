package main

import (
	"path/filepath"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/nanolathe-gg/nanolathe/internal/platform/screenkit"
	"github.com/nanolathe-gg/nanolathe/internal/settings"
)

func TestNLResolutionDraftCommitAndCancel(t *testing.T) {
	t.Setenv(settings.EnvPath, filepath.Join(t.TempDir(), "settings.json"))
	g := &gameShell{cs: &contentSet{}}
	g.applySettings(settings.Defaults())
	g.settingsWritable = true
	g.commitWindowSize()
	s := newNLScreen(func() *gameShell { return g })
	s.draft = s.snapshot(g)
	s.openResolutionDialog()
	s.resolutionText = [2]string{"1377", "913"}
	s.acceptResolution()
	if s.dialog != "" || s.draft.resolution != (retailDisplayMode{1377, 913}) || !s.touched["resolution"] {
		t.Fatal("custom size was not staged")
	}
	if g.windowSize != (retailDisplayMode{800, 600}) || g.display.Width != 800 {
		t.Fatal("draft resized the window")
	}
	s.openResolutionDialog()
	s.resolutionText = [2]string{"1600", "900"}
	s.updateResolution(screenkit.Input{Keys: []ebiten.Key{ebiten.KeyEscape}})
	if s.draft.resolution != (retailDisplayMode{1377, 913}) {
		t.Fatal("Cancel retained text edits")
	}
	s.openResolutionDialog()
	s.resolutionText = [2]string{"639", "900"}
	s.acceptResolution()
	if s.dialog != "resolution" || s.resolutionError == "" || s.draft.resolution != (retailDisplayMode{1377, 913}) {
		t.Fatal("invalid input changed the draft")
	}
	s.dialog = ""
	s.applyDraft(g, s.draft, s.touched, nil)
	if g.windowSize != (retailDisplayMode{1377, 913}) {
		t.Fatalf("committed window: %+v", g.windowSize)
	}
	saved, err := settings.Load()
	if err != nil {
		t.Fatal(err)
	}
	if saved.Display.Width != 1377 || saved.Display.Height != 913 {
		t.Fatalf("saved dimensions: %+v", saved.Display)
	}
	// Defaults change only missing sizes; an existing small window stays valid.
	saved.Display.Width, saved.Display.Height = 640, 480
	saved.Normalize()
	if saved.Display.Width != 640 || saved.Display.Height != 480 {
		t.Fatal("legacy saved size was replaced")
	}
}

func TestNLResolutionMonitorPresetsKeepExactCustomDimensions(t *testing.T) {
	s := newNLScreen(func() *gameShell { return nil })
	s.draft.resolution = retailDisplayMode{1377, 913}
	s.resolutionModes = retailMonitorDisplayModes(retailDisplayMode{2752, 1152}, retailDisplayMode{3440, 1440}, s.draft.resolution)
	for _, want := range []retailDisplayMode{{3440, 1440}, {2752, 1152}, {1280, 536}, {1600, 670}, {1920, 804}, {1377, 913}} {
		i := retailDisplayModeIndex(s.resolutionModes, want.W, want.H)
		if s.resolutionModes[i] != want {
			t.Fatalf("missing preset %+v", want)
		}
	}
	copied := nlDraft{}
	nlCopyCard(s.resolutionCard(), &copied, &s.draft)
	if copied.resolution != s.draft.resolution {
		t.Fatal("copy rounded custom dimensions")
	}
	s.chooseResolution(retailDisplayMode{1501, 901})
	s.stepResolution(1)
	if s.draft.resolution != (retailDisplayMode{1600, 670}) {
		t.Fatalf("next custom neighbour: %+v", s.draft.resolution)
	}
	s.chooseResolution(retailDisplayMode{1501, 901})
	s.stepResolution(-1)
	if s.draft.resolution != (retailDisplayMode{1377, 913}) {
		t.Fatalf("previous custom neighbour: %+v", s.draft.resolution)
	}
}
