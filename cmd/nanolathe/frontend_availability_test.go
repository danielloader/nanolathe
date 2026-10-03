package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/nanolathe-gg/nanolathe/internal/gui"
	"github.com/nanolathe-gg/nanolathe/internal/mission"
	"github.com/nanolathe-gg/nanolathe/internal/settings"
	"github.com/nanolathe-gg/nanolathe/internal/ui"
)

// Fixtures author menu records and metadata; no original bytes are copied.
func TestFrontendAvailabilityFollowsMountedContent(t *testing.T) {
	g := demoSaveShell(t, map[string][]byte{
		"guis/msnbrief.gui": nil, "guis/loadlist.gui": demoSaveFixture(0),
		retailOptionsGUI: demoSaveFixture(0), retailOptionsBackdrop: nlTestPCX(1, 1),
	})
	window := func(names ...string) *gui.Window {
		w := &gui.Window{Rect: gui.Rect{W: 640, H: 480}, Gadgets: []gui.Gadget{{Kind: gui.KindPanel, Active: 1}}}
		for i, name := range names {
			w.Gadgets = append(w.Gadgets, gui.Gadget{Kind: gui.KindButton, Name: name, Active: 1,
				Rect: gui.Rect{X: 20, Y: int32(20 + i*30), W: 100, H: 20}})
		}
		if i := w.GadgetIndex("AnyMsn"); i >= 0 {
			w.Gadgets[i].Rect.Y = 300
		}
		return w
	}
	main := window("SINGLE", "INTRO", "Credits", "EXIT")
	main.Gadgets[main.GadgetIndex("INTRO")].GrayedOut = 2   // Only the low bit is grey.
	main.Gadgets[main.GadgetIndex("Credits")].GrayedOut = 1 // Preserve an authored restriction.
	g.assets = &menuAssets{missionLayout: missionLayoutFixedCampaign, panel: map[shellMode]*retailPanelAssets{
		modeMenuMain: {window: main}, modeMenuSingle: {window: window("NewCamp", "Skirmish", "Options", "LoadGame", "AnyMsn", "PrevMenu")},
		modeMenuMission:  {window: newgameTestWindow()},
		modeMenuSkirmish: {window: window("Start")}, modeMenuMap: {window: window("MAPNAMES")},
	}}
	g.campaigns = []mission.Campaign{{Name: "Arm Campaign", Missions: []mission.Stub{{Index: 0}}}}
	assertEntries := func(mode shellMode, enabled, disabled []string) {
		t.Helper()
		g.openMenu(mode)
		p := g.activePanel()
		for _, name := range enabled {
			if !p.Fires(p.Index(name)) {
				t.Fatalf("mode %d disabled supported %s", mode, name)
			}
		}
		for _, name := range disabled {
			i := p.Index(name)
			r := p.Window.PlacedRect(i)
			if p.Fires(i) || p.PressTest(r.X+1, r.Y+1) != -1 {
				t.Fatalf("mode %d unavailable %s accepts activation", mode, name)
			}
			p.SetFocus(i)
			if p.DefaultKeyAction(false).Kind != ui.ActionNone {
				t.Fatalf("mode %d unavailable %s accepts a keyboard default", mode, name)
			}
		}
	}
	assertEntries(modeMenuMain, []string{"SINGLE", "EXIT"}, []string{"INTRO", "Credits"})
	if g.activePanel().Window.Gadgets[g.activePanel().Index("INTRO")].GrayedOut != 3 {
		t.Fatal("disabling Intro changed unrelated grey-word bits")
	}
	assertEntries(modeMenuSingle, []string{"NewCamp", "Options", "LoadGame", "PrevMenu"}, []string{"Skirmish", survivalButton, "AnyMsn"})

	// A Network map without its terrain still cannot start either scenario.
	g.maps = []string{"fixture"}
	assertEntries(modeMenuSingle, []string{"NewCamp"}, []string{"Skirmish", survivalButton})
	root := t.TempDir()
	for _, name := range []string{introPath, creditsMoviePath, "maps/fixture.tnt"} {
		path := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, nil, 0644); err != nil {
			t.Fatal(err)
		}
	}
	if err := g.cs.unmappedMount.MountDirectory(root, 20); err != nil {
		t.Fatal(err)
	}
	assertEntries(modeMenuMain, []string{"INTRO"}, []string{"Credits"})
	if g.activePanel().Window.Gadgets[g.activePanel().Index("INTRO")].GrayedOut != 2 {
		t.Fatal("reopening did not restore Intro's authored grey word")
	}
	assertEntries(modeMenuSingle, []string{"Skirmish", survivalButton}, []string{"AnyMsn"})
	g.assets.panel[modeMenuMap].window = nil
	assertEntries(modeMenuSingle, []string{"NewCamp"}, []string{"Skirmish", survivalButton})
	g.assets.panel[modeMenuMission].window = nil
	assertEntries(modeMenuSingle, []string{"Options", "LoadGame"}, []string{"NewCamp"})
	g.assets.panel[modeMenuSingle].window = nil
	assertEntries(modeMenuMain, []string{"INTRO", "EXIT"}, []string{"SINGLE"})
}

// Availability follows the same missing-only dialog choice as its loader.
func TestFrontendLoadAvailabilityUsesMissingOnlyFallback(t *testing.T) {
	for _, tc := range []struct {
		name      string
		files     map[string][]byte
		available bool
	}{
		{"no dialog", nil, false},
		{"demo fallback", map[string][]byte{"guis/loadlist.gui": nil}, true},
		{"retail needs backdrop", map[string][]byte{retailSaveLoadGUI: nil, "guis/loadlist.gui": nil}, false},
		{"retail complete", map[string][]byte{retailSaveLoadGUI: nil, loadScreenMode.backdrop(): nil}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			g := demoSaveShell(t, tc.files)
			if got := g.frontendLoadDialogAvailable(); got != tc.available {
				t.Fatalf("available=%t, want %t", got, tc.available)
			}
		})
	}
}

func TestFrontendMissingBackdropIsAnUnavailableChild(t *testing.T) {
	for _, corrupt := range []bool{false, true} {
		files := map[string][]byte{"guis/fixture.gui": demoSaveFixture(0)}
		if corrupt {
			files["bitmaps/fixture.pcx"] = []byte("invalid PCX")
		}
		g := demoSaveShell(t, files)
		panel, err := loadRetailPanelStrict(g.cs, "guis/fixture.gui", "bitmaps/fixture.pcx", "", "fixture")
		if err == nil {
			t.Fatal("missing/corrupt backdrop accepted")
		}
		if corrupt {
			if panel != nil {
				t.Fatal("malformed backdrop became an unavailable optional child")
			}
		} else if panel == nil || panel.window != nil || panel.unavailable == nil {
			t.Fatal("missing backdrop retained a partial usable window")
		}
	}
}

func TestOptionsAvailabilityUsesRequiredPageAssets(t *testing.T) {
	g := demoSaveShell(t, map[string][]byte{"guis/music.gui": nil, "guis/soundsrt.gui": nil})
	window := func() *gui.Window {
		return &gui.Window{Gadgets: []gui.Gadget{{Kind: gui.KindPanel},
			{Kind: gui.KindButton, Name: "SOUND", Active: 1}, {Kind: gui.KindButton, Name: "MUSIC", Active: 1}}}
	}
	w := window()
	g.disableUnavailableOptionsPages(w, false)
	if !retailGadgetGreyed(w, "SOUND") || !retailGadgetGreyed(w, "MUSIC") {
		t.Fatal("frontend category admitted an absent GUI or backdrop")
	}
	w = window()
	g.disableUnavailableOptionsPages(w, true)
	if retailGadgetGreyed(w, "SOUND") || !retailGadgetGreyed(w, "MUSIC") {
		t.Fatal("battle category did not use its RT GUI without a backdrop")
	}
}

func TestMusicTransportNeedsAvailableTracks(t *testing.T) {
	previousPanel, previousAssets, previousState := optionsPanel, optionsAssets, optionsState
	t.Cleanup(func() { optionsPanel, optionsAssets, optionsState = previousPanel, previousAssets, previousState })
	w := &gui.Window{Gadgets: []gui.Gadget{{Kind: gui.KindPanel}}}
	for _, name := range []string{"MUSICVOL", "CDPREV", "CDSTOP", "CDPLAY", "CDNEXT", "TRACKMODE", "TRACKTYPE", "NOTRAK", "TRACKNUM"} {
		w.Gadgets = append(w.Gadgets, gui.Gadget{Kind: gui.KindButton, Name: name, Active: 1, Stages: 5})
	}
	optionsPanel = ui.NewPanel(w)
	optionsAssets = &retailPanelAssets{window: w}
	optionsState = &retailOptionsState{}
	for _, tc := range []struct {
		on     bool
		tracks int
	}{{true, 0}, {true, 2}, {false, 2}} {
		optionsState.tracks = tc.tracks
		g := &gameShell{audioPrefs: settings.Audio{MusicMode: boolInt(tc.on), CDMode: settings.MaxCDMode}}
		g.syncRetailMusicPage()
		for _, name := range []string{"CDPREV", "CDSTOP", "CDPLAY", "CDNEXT", "TRACKMODE", "TRACKTYPE"} {
			if got := optionsPanel.Fires(optionsPanel.Index(name)); got != (tc.on && tc.tracks > 0) {
				t.Fatalf("music=%t tracks=%d: %s fires=%t", tc.on, tc.tracks, name, got)
			}
		}
		if optionsPanel.Fires(optionsPanel.Index("MUSICVOL")) != tc.on || !optionsPanel.Fires(optionsPanel.Index("NOTRAK")) {
			t.Fatal("missing tracks disabled usable music settings")
		}
	}
}
