package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nanolathe-gg/nanolathe/formats"
	"github.com/nanolathe-gg/nanolathe/internal/mission"
	"github.com/nanolathe-gg/nanolathe/internal/ui"
	"github.com/nanolathe-gg/nanolathe/vfs"
)

// The fallback is content presentation policy; [07 R-FE-01 §4] supplies
// the campaign-count boundary. Fixtures are independently authored.
func TestMissionBackgroundFallback(t *testing.T) {
	for _, tc := range []struct {
		name       string
		campaigns  int
		background string
		playAny    []byte
		layout     missionMenuLayout
		wantError  bool
	}{
		{name: "retail needs no unused campaign art", playAny: nlTestPCX(2, 1)},
		{name: "one campaign", campaigns: 1, background: "newcampaign4x", layout: missionLayoutFixedCampaign},
		{name: "two campaigns remain fixed", campaigns: 2, background: "newcampaign4x", layout: missionLayoutFixedCampaign},
		{name: "three campaigns use a list", campaigns: 3, background: "newcampaign4", layout: missionLayoutCampaign},
		{name: "missing chosen background", campaigns: 1, layout: missionLayoutFixedCampaign, wantError: true},
		{name: "corrupt retail does not fall back", campaigns: 1, playAny: []byte("invalid PCX"), background: "newcampaign4x", wantError: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			write := func(name string, data []byte) {
				t.Helper()
				path := filepath.Join(root, name)
				if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, data, 0644); err != nil {
					t.Fatal(err)
				}
			}
			for i := 0; i < tc.campaigns; i++ {
				side := []string{"ARM", "CORE"}[i%2]
				write(fmt.Sprintf("camps/campaign%d.TDF", i), []byte("[HEADER]{campaignside="+side+";}"))
			}
			// A subdirectory ending in TDF and a non-TDF file do not change
			// the boundary; use different sides above the counting layer.
			write("camps/ignored.tdf/nested.tdf", nil)
			write("camps/readme.txt", nil)
			if tc.playAny != nil {
				write("bitmaps/playanygame4.pcx", tc.playAny)
			}
			if tc.background != "" {
				write("bitmaps/"+tc.background+".pcx", nlTestPCX(1, 2))
			}
			fs := vfs.New()
			defer fs.Close()
			if err := fs.MountDirectory(root, 0); err != nil {
				t.Fatal(err)
			}
			background, layout, err := loadMissionBackground(testContentSet(fs))
			if (err != nil) != tc.wantError || layout != tc.layout {
				t.Fatalf("layout=%d error=%v, want layout=%d error=%v", layout, err, tc.layout, tc.wantError)
			}
			if tc.wantError {
				if tc.playAny == nil && !strings.Contains(err.Error(), "logical path bitmaps/newcampaign4x.pcx") {
					t.Fatalf("missing background lost its logical path: %v", err)
				}
				return
			}
			wantWidth := uint16(1)
			if tc.playAny != nil {
				wantWidth = 2
			}
			if background == nil || background.Width != wantWidth {
				t.Fatal("selected the wrong authored image")
			}
		})
	}
}

func TestCampaignFallbackUsesAuthoredLayout(t *testing.T) {
	campaigns := []mission.Campaign{
		{Name: "Alternative Arm", Document: mustParseCampaignHeader(t, "ARM")},
		{Name: "Arm Campaign", Document: mustParseCampaignHeader(t, "ARM"), Missions: []mission.Stub{{Index: 0}, {Index: 1}}},
		{Name: "Core Campaign", Document: mustParseCampaignHeader(t, "CORE"), Missions: []mission.Stub{{Index: 0}, {Index: 1}}},
	}
	for _, layout := range []missionMenuLayout{missionLayoutCampaign, missionLayoutFixedCampaign} {
		for side := 0; side < 2; side++ {
			window := newgameTestWindow()
			window.Gadgets[1].Rect.Y, window.Gadgets[1].Rect.H = 314, 142
			before := window.Gadgets[1].Rect
			background := &formats.PCX{Width: 1}
			g := &gameShell{frontend: ui.NewFrontend(modeMenuMission), campaigns: campaigns, missionSide: side, missionIdx: 1,
				assets: &menuAssets{missionLayout: layout, missionBackground: background,
					panel: map[shellMode]*retailPanelAssets{modeMenuMission: {window: window}}}}
			g.applyRetailMissionLayout(window)
			g.frontend.Panels.Replace(ui.NewPanel(window))
			g.refreshMissionPanel()
			p := g.activePanel()
			if p.ActiveOf("Missions") || p.ActiveOf("MissionsKnob") || g.missionIdx != 0 {
				t.Fatal("campaign fallback exposed or retained a later mission")
			}
			if p.ActiveOf("Campaign") != (layout == missionLayoutCampaign) || p.ActiveOf("CampaignKnob") != (layout == missionLayoutCampaign) {
				t.Fatal("campaign visibility does not match the selected art")
			}
			if window.Gadgets[1].Rect != before || g.panelBackground() != background {
				t.Fatal("fallback changed authored rectangles or lost its background")
			}
			if layout == missionLayoutFixedCampaign {
				want := []string{"Arm Campaign", "Core Campaign"}[side]
				if len(g.campaignOptions) != 1 || g.campaignOptions[0].Name != want || window.Header.DefaultFocus != "Difficulty" {
					t.Fatalf("fixed side %d selected %+v", side, g.campaignOptions)
				}
			} else if window.Header.DefaultFocus != "Campaign" {
				t.Fatal("visible campaign list lost initial focus")
			}
		}
	}
}
