package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nanolathe-gg/nanolathe/internal/client"
	"github.com/nanolathe-gg/nanolathe/internal/frame"
	"github.com/nanolathe-gg/nanolathe/internal/gui"
	"github.com/nanolathe-gg/nanolathe/internal/input"
	"github.com/nanolathe-gg/nanolathe/internal/settings"
	"github.com/nanolathe-gg/nanolathe/internal/ui"
)

func TestCommunityHUDOptionsTransaction(t *testing.T) {
	g, _, cl := retailAssetShell(t)
	previous := clPtr
	clPtr = cl
	t.Cleanup(func() { clPtr = previous })
	t.Setenv(settings.EnvPath, filepath.Join(t.TempDir(), "settings.json"))
	g.attachSettings()
	t.Cleanup(g.closeRetailOptionsScreen)
	g.openMenu(modeMenuSingle)
	g.activateGadget("Options")
	g.activateRetailOptionsGadget("COMMUNITYHUD")
	if optionsState.page != "communityhud" {
		t.Fatal("HUD page did not open")
	}
	if optionsPanel.Index("HELPTEXT") < 0 {
		t.Fatal("HUD applicability hints have no visible help target")
	}
	for _, name := range []string{"NHEALTH", "NCOUNTERS", "NRELOAD", "NVETERAN", "NGROUPS", "NALLIES", "NWEATHER", "NVICTORY"} {
		if optionsPanel.HelpOf(name) == "" {
			t.Fatalf("%s has no applicability hint", name)
		}
	}
	if client.DamageBars() || optionsPanel.StageAt(optionsPanel.Index("NHEALTH")) != 0 {
		t.Fatal("health bars should open at the saved default")
	}
	reload := optionsPanel.Window.PlacedRect(optionsPanel.Index("NRELOAD"))
	g.updateHoverHelp(reload.X+1, reload.Y+1)
	if got, want := optionsPanel.TextOf("HELPTEXT"), optionsPanel.HelpOf("NRELOAD"); got != want {
		t.Fatalf("reload hint = %q, want %q", got, want)
	}
	if dir := os.Getenv("NANOLATHE_OPTIONS_SHOT"); dir != "" {
		writeShellShot(t, cl, filepath.Join(dir, "community-hud.png"))
	}
	g.activateRetailOptionsGadget("NCOUNTERS")
	g.activateRetailOptionsGadget("NGROUPS")
	g.activateRetailOptionsGadget("NHEALTH")
	if !cl.CommunityHUDOptions().Counters || !cl.CommunityHUDOptions().DisableGroupNumbers || !client.DamageBars() {
		t.Fatal("live HUD preferences not applied")
	}
	g.activateRetailOptionsGadget("UNDO")
	if cl.CommunityHUDOptions().Counters || cl.CommunityHUDOptions().DisableGroupNumbers || client.DamageBars() {
		t.Fatal("undo changed default HUD settings")
	}
	g.activateRetailOptionsGadget("NHEALTH")
	g.activateRetailOptionsGadget("NVICTORY")
	g.activateRetailOptionsGadget("NWEATHER")
	g.activateRetailOptionsGadget("PREV")
	saved, err := settings.Load()
	if err != nil {
		t.Fatal(err)
	}
	if saved.Presentation.WeatherReport != 1 || saved.Presentation.VictoryCue != 1 || !saved.DamageBarsEnabled() {
		t.Fatal("HUD preference not persisted")
	}
	if err := g.openRetailOptionsScreen(true); err != nil {
		t.Fatal(err)
	}
	g.activateRetailOptionsGadget("COMMUNITYHUD")
	counters := optionsPanel.Window.PlacedRect(optionsPanel.Index("NCOUNTERS"))
	g.updateHoverHelp(counters.X+1, counters.Y+1)
	if dir := os.Getenv("NANOLATHE_OPTIONS_SHOT"); dir != "" {
		writeShellShot(t, cl, filepath.Join(dir, "community-hud-battle.png"))
	}
	g.activateRetailOptionsGadget("RESTORE")
	if client.DamageBars() || g.presentation.WeatherReport != 0 || g.presentation.VictoryCue != 0 {
		t.Fatal("restore did not reset the HUD page defaults")
	}
	g.activateRetailOptionsGadget("CANCEL")
	if g.presentation.WeatherReport != 1 || g.presentation.VictoryCue != 1 || !client.DamageBars() {
		t.Fatal("cancel did not restore the entry HUD preferences")
	}
}

// syntheticOptionsPage opens one options page built on an authored-by-us
// template (a staged SHADING button and a label) with the options state the
// dispatcher requires, and returns the shell, panel and a client whose input
// drives the ordinary front-end widget pass. No retail asset is read.
func syntheticOptionsPage(t *testing.T, page string, build func(*gui.Window) error) (*gameShell, *ui.Panel, *client.Client) {
	t.Helper()
	window := &gui.Window{Rect: gui.Rect{W: 640, H: 480}, Gadgets: []gui.Gadget{
		{Kind: gui.KindPanel, Active: 1, Rect: gui.Rect{W: 640, H: 480}},
		{Kind: gui.KindLabel, Name: "CAPTION", Active: 1, Rect: gui.Rect{X: 20, Y: 20, W: 120, H: 12}},
		{Kind: gui.KindButton, Name: "SHADING", Active: 1, Rect: gui.Rect{X: 20, Y: 40, W: 120, H: 20}, Stages: 2},
	}}
	if build != nil {
		if err := build(window); err != nil {
			t.Fatal(err)
		}
	}
	for i := range window.Gadgets {
		if window.Gadgets[i].Stages != 0 {
			window.Gadgets[i].Labels = strings.Split(window.Gadgets[i].Text, "|")
		}
	}
	g := &gameShell{frontend: ui.NewFrontend(modeMenuSingle), presentation: settings.DefaultPresentation(), audioPrefs: settings.DefaultAudio()}
	panel := ui.NewPanel(window)
	g.frontend.Panels.Push(panel)
	priorPanel, priorAssets, priorState := optionsPanel, optionsAssets, optionsState
	optionsPanel, optionsAssets = panel, &retailPanelAssets{window: window}
	optionsState = &retailOptionsState{page: page, sliders: map[int]*retailSliderState{}, serviceStageIndex: -1}
	t.Cleanup(func() { optionsPanel, optionsAssets, optionsState = priorPanel, priorAssets, priorState })
	cl, err := client.New(client.Options{Buffer: &frame.Buffer{}, Width: 640, Height: 480})
	if err != nil {
		t.Fatal(err)
	}
	return g, panel, cl
}

// Every HUD switch is a live control: a click writes its own preference and
// nothing else, the page shows what was written, and the click plays only the
// page's ordinary `Options` cue (DESIGN_INTERFACE_HUD_INPUT §3.16). The
// regression is the Victory cue switch, which had no callback: a click moved
// only its drawn stage, and the next click on any other switch redrew every
// row from the preferences and turned it back off.
func TestCommunityHUDEverySwitchCommits(t *testing.T) {
	g, panel, cl := syntheticOptionsPage(t, "communityhud", communityHUDOptionsPage)
	g.syncCommunityHUDOptions()
	// The Victory cue switch goes first, so every other switch is clicked
	// after it, as in the report.
	rows := []string{"NVICTORY"}
	for _, row := range communityHUDSwitches(&settings.Presentation{}) {
		if row.name != "NVICTORY" {
			rows = append(rows, row.name)
		}
	}
	for _, name := range append([]string{"NHEALTH"}, rows...) {
		if got := retailOptionsCue(retailOptionsCueKey(name)); got != "Options" {
			t.Errorf("%s plays %q; want the page's Options cue and no preview", name, got)
		}
	}
	for _, name := range rows {
		before := g.presentation
		clickRowGadget(t, g, panel, cl, name, input.MouseButtonLeft)
		after := g.presentation
		changed := communityHUDSwitches(&after)
		unchanged := communityHUDSwitches(&before)
		for i, row := range changed {
			want := *unchanged[i].value
			if row.name == name {
				want = 1 - want
			}
			if *row.value != want {
				t.Errorf("clicking %s left %s at %d; want %d", name, row.name, *row.value, want)
			}
			if got := panel.StageAt(panel.Index(row.name)); got != *row.value {
				t.Errorf("after clicking %s the %s switch shows %d; its preference is %d", name, row.name, got, *row.value)
			}
		}
	}
	if g.presentation.VictoryCue != 1 {
		t.Fatalf("the Victory cue switch did not stay on while the other switches changed: %d", g.presentation.VictoryCue)
	}
	if got := g.captureSettings().Presentation.VictoryCue; got != 1 {
		t.Fatalf("the saved block carries victoryCue %d; want the switch's 1", got)
	}
}
