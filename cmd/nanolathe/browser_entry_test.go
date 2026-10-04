package main

import (
	"fmt"
	"io"
	"path/filepath"
	"testing"

	"github.com/nanolathe-gg/nanolathe/internal/gameplay"
	"github.com/nanolathe-gg/nanolathe/internal/mission"
	"github.com/nanolathe-gg/nanolathe/internal/platform/ebitenapp"
	"github.com/nanolathe-gg/nanolathe/internal/settings"
)

// Browser mission launch uses the ordinary direct windowed entry. Exercise
// that composition without running a window: the authored campaign side and
// requested difficulty must reach the shell while its saved skirmish setup
// and presentation preferences remain available (DESIGN_BROWSER_HOST §1).
func TestDirectMissionEntryKeepsShellIdentityAndPreferences(t *testing.T) {
	root := probeRetail(t)
	cs, err := openContent(Options{Root: root, Mod: "none", ModSet: true})
	if err != nil {
		t.Fatal(err)
	}
	defer cs.Close()
	census, err := censusSkirmishMaps(cs.unmappedMount)
	if err != nil || len(census.names) == 0 {
		t.Fatalf("reference install's skirmish map list unavailable: %v", err)
	}

	for _, tc := range []struct {
		name       string
		side       int
		difficulty int
		renderer   string
	}{
		{name: "Arm", side: 0, difficulty: 0, renderer: "modern"},
		{name: "Core", side: 1, difficulty: 2, renderer: "classic"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv(settings.EnvPath, filepath.Join(t.TempDir(), "settings.json"))
			saved := settings.Defaults()
			saved.Difficulty = 1
			saved.Gameplay = gameplay.Strict31
			saved.UnitLimit = 777
			saved.Skirmish.Map = census.names[len(census.names)-1]
			saved.Skirmish.NumPlayers = 3
			saved.Skirmish.Players[0].Side = 1 - tc.side
			saved.Skirmish.Players[0].Color = 4
			saved.Display.Width, saved.Display.Height = 800, 600
			saved.ScrollSpeed = 7
			saved.SwitchAlt = 1
			saved.Fullscreen = true
			saved.Presentation.Renderer = "classic"
			if tc.renderer == "classic" {
				saved.Presentation.Renderer = "modern"
			}
			saved.Presentation.FPS = 120
			if err := saved.Save(); err != nil {
				t.Fatal(err)
			}
			opts, err := parseFlags([]string{
				"--root=" + root, "--mod=none", "--fullscreen=false", "--auto-remaster=false",
				"--mission=camps/" + tc.name + " Campaign.tdf:MISSION0",
				fmt.Sprintf("--difficulty=%d", tc.difficulty), "--seed=7",
				"--renderer=" + tc.renderer, "--fps=60",
			}, io.Discard)
			if err != nil {
				t.Fatal(err)
			}
			previousClient := clPtr
			defer func() { clPtr = previousClient }()
			shell, cl, err := newDirectBattleView(opts, cs)
			if err != nil {
				t.Fatal(err)
			}
			defer shell.releaseAudio()
			defer shell.teardownBattle(cl)
			if shell.battle == nil || shell.frontend.Mode != modeBattle {
				t.Fatal("direct mission did not enter the shell's battle state")
			}
			sess := shell.battle.sess
			if sess.Mission == nil || sess.Mission.Type != mission.TypeCampaign || sess.Mission.Difficulty != tc.difficulty || shell.missionDifficulty() != tc.difficulty {
				t.Fatalf("mission difficulty did not reach the campaign and shell: mission=%+v shell=%d", sess.Mission, shell.missionDifficulty())
			}
			if side, known := sess.SideForOwner(int(sess.LocalOwner)); !known || side != tc.side || shell.missionSide != side || shell.battle.hud.side != sess.Catalog.Sides[side] {
				t.Fatalf("campaign side disagrees across session, shell and HUD: side=%d known=%t shell=%d", side, known, shell.missionSide)
			}
			if sess.Gameplay != saved.Gameplay || shell.gameplay != saved.Gameplay {
				t.Fatalf("mission gameplay=%q shell=%q, want saved %q", sess.Gameplay, shell.gameplay, saved.Gameplay)
			}
			if shell.setup.MapName != saved.Skirmish.Map || shell.setup.NumPlayers != saved.Skirmish.NumPlayers || shell.setup.UnitLimit != saved.UnitLimit || shell.setup.Players[0].Side != saved.Skirmish.Players[0].Side || shell.setup.Players[0].Color != saved.Skirmish.Players[0].Color {
				t.Fatalf("mission replaced the retained skirmish preferences: %+v", shell.setup)
			}
			if w, h := cl.Size(); w != saved.Display.Width || h != saved.Display.Height || shell.scrollSpeed != saved.ScrollSpeed || !shell.battle.switchAlt {
				t.Fatalf("mission lost display/input preferences: size=%dx%d scroll=%d switchAlt=%t", w, h, shell.scrollSpeed, shell.battle.switchAlt)
			}
			window := shell.windowOptions()
			mode, fps := window.PresentationSettings()
			wantMode := ebitenapp.RendererModern
			if tc.renderer == "classic" {
				wantMode = ebitenapp.RendererClassic
			}
			if mode != wantMode || fps != 60 || window.Fullscreen || shell.opts.AutoRemaster {
				t.Fatalf("launch overrides lost: renderer=%v fps=%d fullscreen=%t remaster=%t", mode, fps, window.Fullscreen, shell.opts.AutoRemaster)
			}
			shell.battle.returnToMenu(cl)
			if shell.battle != nil || shell.frontend.Mode != modeMenuMain || shell.captureSettings().Skirmish.Map != saved.Skirmish.Map || shell.setup.UnitLimit != saved.UnitLimit {
				t.Fatal("direct mission did not return through the ordinary shell lifecycle with preferences intact")
			}
		})
	}
}
