package main

import (
	"testing"

	"github.com/nanolathe-gg/nanolathe/internal/audio"
	"github.com/nanolathe-gg/nanolathe/internal/client"
	"github.com/nanolathe-gg/nanolathe/internal/gui"
	"github.com/nanolathe-gg/nanolathe/internal/input"
)

func musicOptionsAuditShell(t *testing.T) (*gameShell, *client.Client, *soundOptionsAuditOutput) {
	t.Helper()
	g, cl, out := soundOptionsAuditShell(t, 3)
	g.audioPrefs.CDMode = 2
	g.audioOwner.Music.Configure(audio.ModeRandom, 0)
	g.audioOwner.Music.SetRequestedTrack(1)
	g.openRetailOptionsPage("music")
	return g, cl, out
}

func musicOptionsAuditPass(g *gameShell, cl *client.Client, battle bool) {
	if battle {
		b := &battleSession{shell: g}
		b.serviceBattleOptionsWidgets(optionsPanel, cl.Input())
	} else {
		g.menuInput(cl)
	}
}

// The two production adapters poll even without an action. Logical next can
// be nonzero while CurTrack is zero after a stop [07 R-FE-01 §6].
func TestMusicOptionsAuditIdlePollUsesLogicalTrack(t *testing.T) {
	for _, battle := range []bool{false, true} {
		t.Run(map[bool]string{false: "frontend", true: "battle"}[battle], func(t *testing.T) {
			g, cl, _ := musicOptionsAuditShell(t)
			c := g.audioOwner.Music
			c.Open(3)
			optionsState.track = 3
			optionsPanel.SetText("TRACKNUM", "3")
			musicOptionsAuditPass(g, cl, battle)
			if optionsState.track != 0 || optionsPanel.TextOf("TRACKNUM") != retailNoDiscText {
				t.Fatal("post-open zero logical track did not reach the page")
			}
			c.Stop()
			musicOptionsAuditPass(g, cl, battle)
			if c.CurTrack() != 0 || optionsState.track != 1 || optionsPanel.TextOf("TRACKNUM") != "1" {
				t.Fatal("stopped logical track was replaced by audible-track state")
			}
		})
	}
}

func TestMusicOptionsAuditDisplayEqualityAndEditedMismatch(t *testing.T) {
	for _, battle := range []bool{false, true} {
		t.Run(map[bool]string{false: "frontend", true: "battle"}[battle], func(t *testing.T) {
			g, cl, out := musicOptionsAuditShell(t)
			c := g.audioOwner.Music
			c.Play(2)
			g.audioPrefs.CDMode = 3
			optionsState.track = 3
			for _, text := range []string{" +2tail", "4294967298end", "-4294967294"} {
				optionsPanel.SetText("TRACKNUM", text)
				musicOptionsAuditPass(g, cl, battle)
				if optionsState.track != 3 || c.RequestedTrack() != 1 || optionsPanel.TextOf("TRACKNUM") != text {
					t.Fatal("equal displayed decimal prefix changed selection or request")
				}
			}
			optionsPanel.SetText("TRACKNUM", "2")
			i := optionsPanel.Index("TRACKNUM")
			optionsPanel.Window.Gadgets[i].MaxChars = 12
			if !optionsPanel.FocusEditor(i) {
				t.Fatal("authored track editor did not capture")
			}
			cl.Input().EnqueueToken(input.Token{Kind: input.TokenEdit, Key: input.KeyBackspace})
			cl.Input().EnqueueToken(input.Token{Kind: input.TokenText, Rune: '3'})
			opens := out.musicOpens
			musicOptionsAuditPass(g, cl, battle)
			if optionsState.track != 2 || c.RequestedTrack() != 2 || optionsPanel.TextOf("TRACKNUM") != "2" || cl.Input().PendingTokens() != 0 {
				t.Fatal("track poll did not follow the same pass's text edit")
			}
			if out.musicOpens != opens {
				t.Fatal("detail refresh unexpectedly started playback")
			}
		})
	}
}

func TestMusicOptionsAuditMissingCategoryStillWritesRequest(t *testing.T) {
	g, cl, _ := musicOptionsAuditShell(t)
	c := g.audioOwner.Music
	c.Play(2)
	g.audioPrefs.CDMode = 3
	optionsPanel.Window.Gadgets[optionsPanel.Index("TRACKTYPE")].Name = "ABSENT"
	optionsPanel.SetText("TRACKNUM", "3")
	musicOptionsAuditPass(g, cl, false)
	if optionsState.track != 2 || c.RequestedTrack() != 2 || optionsPanel.TextOf("TRACKNUM") != "3" {
		t.Fatal("missing category control did not isolate the label-write branch")
	}
}

func TestMusicOptionsAuditRepeatEntryAndAdvancedModeStage(t *testing.T) {
	for _, battle := range []bool{false, true} {
		t.Run(map[bool]string{false: "frontend", true: "battle"}[battle], func(t *testing.T) {
			g, cl, out := musicOptionsAuditShell(t)
			c := g.audioOwner.Music
			c.Play(2)
			g.audioPrefs.CDMode = 3
			optionsState.track = 3
			opens := out.musicOpens
			g.openRetailOptionsPage("music")
			if optionsState.track != 1 || optionsPanel.TextOf("TRACKNUM") != "1" || out.musicOpens != opens {
				t.Fatal("Repeat entry failed to restore request without playback")
			}
			g.audioPrefs.CDMode = 2
			g.syncRetailMusicPage()
			r := optionsPanel.Window.PlacedRect(optionsPanel.Index("TRACKMODE"))
			mouse := cl.Input().Mouse
			mouse.SetPosition(float32(r.X+r.W/2), float32(r.Y+r.H/2))
			mouse.SetButton(input.MouseButtonLeft, true)
			musicOptionsAuditPass(g, cl, battle)
			mouse.ResetEdges()
			// Force the release pass to refresh details after the staged button
			// advances. A full-page refresh would erase that new stage.
			c.Play(3)
			opens = out.musicOpens
			mouse.SetButton(input.MouseButtonLeft, false)
			musicOptionsAuditPass(g, cl, battle)
			mouse.ResetEdges()
			if g.audioPrefs.CDMode != 3 || optionsState.track != 1 || c.RequestedTrack() != 1 || out.musicOpens != opens {
				t.Fatal("pre-action poll erased advanced mode stage or ran after Repeat")
			}
			musicOptionsAuditPass(g, cl, battle)
			if optionsState.track != 3 || c.RequestedTrack() != 3 {
				t.Fatal("later no-action pass failed to synchronize Repeat request")
			}
		})
	}
}

func TestMusicOptionsAuditDisabledPollAndMalformedFallback(t *testing.T) {
	g, cl, _ := musicOptionsAuditShell(t)
	c := g.audioOwner.Music
	g.audioPrefs.CDMode, g.audioPrefs.MusicMode = 3, 0
	c.SetEnabled(false)
	optionsState.track = 3
	optionsPanel.SetText("TRACKNUM", "3")
	musicOptionsAuditPass(g, cl, false)
	if optionsState.track != 1 || c.RequestedTrack() != 1 || c.IsPlaying() {
		t.Fatal("disabled music suppressed the page poll or started playback")
	}
	for _, kind := range []gui.Kind{gui.KindSurface, gui.KindTextBox} {
		i := optionsPanel.Index("TRACKNUM")
		optionsPanel.Window.Gadgets[i].Kind = kind
		if kind == gui.KindTextBox {
			optionsPanel.Window.Gadgets[i].Name = "ABSENT"
		}
		optionsState.track = 3
		musicOptionsAuditPass(g, cl, false)
		if optionsState.track != 3 || c.RequestedTrack() != 1 {
			t.Fatal("malformed control fallback changed selection or request")
		}
	}
}
