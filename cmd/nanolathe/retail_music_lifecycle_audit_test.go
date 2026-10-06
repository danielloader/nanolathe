package main

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/nanolathe-gg/nanolathe/formats"
	"github.com/nanolathe-gg/nanolathe/internal/audio"
	"github.com/nanolathe-gg/nanolathe/internal/client"
	"github.com/nanolathe-gg/nanolathe/internal/gui"
	"github.com/nanolathe-gg/nanolathe/internal/input"
	"github.com/nanolathe-gg/nanolathe/vfs"
)

type musicLifecycleOutput struct{ cue func() }

func (o *musicLifecycleOutput) PlaySample(*audio.Sample, float64, float64) error {
	if o.cue != nil {
		o.cue()
	}
	return nil
}

// Authored roots and pages exercise real loading, rebuilding and both widget
// adapters. The output deliberately has no music device: queries are supplied
// through the controller's existing host seam, with no audibility assertion.
func musicLifecycleShell(t *testing.T, battle bool) (*gameShell, *client.Client, *musicLifecycleOutput) {
	t.Helper()
	g, _, cl := syntheticOptionsPage(t, "", func(w *gui.Window) error { w.Gadgets = w.Gadgets[:1]; return nil })
	g.closeRetailOptionsScreen()
	root := t.TempDir()
	write := func(path string, data []byte) {
		t.Helper()
		path = filepath.Join(root, path)
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, data, 0644); err != nil {
			t.Fatal(err)
		}
	}
	rootGUI := "[GADGET0]{[COMMON]{id=0; name=ROOT; width=220; height=300;}}\n"
	for i, name := range []string{"MUSIC", "SOUND", "SPEEDS", "VISUALS", "CANCEL", "PREV"} {
		rootGUI += fmt.Sprintf("[GADGET%d]{[COMMON]{id=1; name=%s; xpos=10; ypos=%d; width=80; height=20; active=1;} text=%s;}\n", i+1, name, 20+i*35, name)
	}
	write("guis/startopt.gui", []byte(strings.Replace(rootGUI, "width=220", "width=640", 1)))
	write("guis/prefs.gui", []byte(rootGUI))
	musicGUI := `[GADGET0]{[COMMON]{id=0; name=PAGE; xpos=220; width=300; height=240;}}
[GADGET1]{[COMMON]{id=1; name=TRACKMODE; xpos=20; ypos=20; width=80; height=20; active=1;} stages=4; text=All|Random|Repeat|Custom;}
[GADGET2]{[COMMON]{id=3; name=TRACKNUM; xpos=120; ypos=20; width=80; height=20; active=1;}}
[GADGET3]{[COMMON]{id=1; name=TRACKTYPE; xpos=20; ypos=50; width=80; height=20; active=1;} stages=5; text=Building|Battle|Victory|Defeat|Unused;}
[GADGET4]{[COMMON]{id=1; name=NOTRAK; xpos=120; ypos=50; width=80; height=20; active=1;} stages=2; text=Off|On;}
[GADGET5]{[COMMON]{id=1; name=CDPLAY; xpos=20; ypos=90; width=80; height=20; active=1;} text=Play;}
[GADGET6]{[COMMON]{id=1; name=CDNEXT; xpos=120; ypos=90; width=80; height=20; active=1;} text=Next;}
[GADGET7]{[COMMON]{id=1; name=CDSTOP; xpos=20; ypos=120; width=80; height=20; active=1;} text=Stop;}
[GADGET8]{[COMMON]{id=1; name=CDPREV; xpos=120; ypos=120; width=80; height=20; active=1;} text=Prev;}
[GADGET9]{[COMMON]{id=1; name=RESTORE; xpos=20; ypos=160; width=80; height=20; active=1;} text=Restore;}
[GADGET10]{[COMMON]{id=1; name=UNDO; xpos=120; ypos=160; width=80; height=20; active=1;} text=Undo;}`
	for _, name := range []string{"music", "musicrt"} {
		write("guis/"+name+".gui", []byte(musicGUI))
	}
	for _, name := range []string{"sounds", "soundsrt", "speeds", "speedsrt", "visuals", "visualrt"} {
		write("guis/"+name+".gui", []byte("[GADGET0]{[COMMON]{id=0; name=PAGE; xpos=220; width=300; height=240;}}"))
	}
	for _, name := range []string{"options4x", "optmusic4x", "optsound4x", "optinterface4x", "optvisual4x"} {
		write("bitmaps/"+name+".pcx", nlTestPCX(1, 2))
	}
	for i := 1; i <= 3; i++ {
		write(fmt.Sprintf("music/%d.wav", i), []byte("authored media"))
	}
	write("sounds/options.wav", []byte{128, 129, 127, 128})
	fs := vfs.New()
	if err := fs.MountDirectory(root, 1); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { fs.Close() })
	g.cs = testContentSet(fs)
	g.assets = &menuAssets{common: &formats.GAF{}}
	for _, name := range []string{"MUSIC", "SOUND", "SPEEDS", "VISUALS", "CANCEL", "PREV", "TRACKMODE", "TRACKTYPE", "NOTRAK", "CDPLAY", "CDNEXT", "CDSTOP", "CDPREV", "RESTORE", "UNDO"} {
		entry := formats.GAFEntry{Name: name, Frames: make([]formats.GAFFrameRef, 9)}
		for i := range entry.Frames {
			entry.Frames[i].Frame = &formats.GAFFrame{Width: 80, Height: 20, Pixels: make([]byte, 1600)}
		}
		g.assets.common.Entries = append(g.assets.common.Entries, entry)
	}
	g.audioOwner = audio.NewService(fs)
	g.audioOwner.ConfigureMusic(false)
	g.audioOwner.Music.Configure(audio.ModeSequential, 0)
	g.audioPrefs.CDMode = 1
	g.audioOwner.Registry.RegisterPath("Options", "sounds/options.wav")
	g.audioOwner.Registry.RegisterPath("Previous", "sounds/options.wav")
	g.frontendAliasesBound = true
	out := &musicLifecycleOutput{}
	old := audio.GlobalOutput()
	audio.SetGlobalOutput(out)
	t.Cleanup(func() { out.cue = nil; g.audioOwner.Music.Stop(); audio.SetGlobalOutput(old) })
	if err := g.openRetailOptionsScreen(battle); err != nil {
		t.Fatal(err)
	}
	g.openRetailOptionsPage("music")
	if optionsPanel.Index("CDPLAY") < 1 {
		t.Fatal("authored music page did not open")
	}
	return g, cl, out
}

func musicLifecycleClick(t *testing.T, g *gameShell, cl *client.Client, battle bool, name string) {
	t.Helper()
	i := optionsPanel.Index(name)
	if i < 1 {
		t.Fatalf("missing button %s", name)
	}
	r := optionsPanel.Window.PlacedRect(i)
	mouse := cl.Input().Mouse
	mouse.SetPosition(float32(r.X+r.W/2), float32(r.Y+r.H/2))
	mouse.SetButton(input.MouseButtonLeft, true)
	musicOptionsAuditPass(g, cl, battle)
	mouse.ResetEdges()
	mouse.SetButton(input.MouseButtonLeft, false)
	musicOptionsAuditPass(g, cl, battle)
	mouse.ResetEdges()
}

func TestMusicLifecycleTransport(t *testing.T) {
	for _, battle := range []bool{false, true} {
		for _, paused := range []bool{false, true} {
			t.Run(fmt.Sprintf("battle=%v/paused=%v", battle, paused), func(t *testing.T) {
				g, cl, _ := musicLifecycleShell(t, battle)
				c := g.audioOwner.Music
				if paused {
					c.Play(1)
					c.Pause(true)
				} else {
					c.SelectTrack(1)
				}
				g.audioPrefs.CDMode = 3
				c.SetRequestedTrack(3)
				optionsState.track = 1
				optionsPanel.SetText("TRACKNUM", "1")
				musicLifecycleClick(t, g, cl, battle, "CDNEXT")
				wantStatus := audio.StatusIdle
				if paused {
					wantStatus = audio.StatusPaused
				}
				if optionsState.track != 2 || c.NextTrack() != 2 || c.RequestedTrack() != 2 || c.Status() != wantStatus {
					t.Fatalf("next selection=%d next=%d request=%d status=%v", optionsState.track, c.NextTrack(), c.RequestedTrack(), c.Status())
				}
				musicLifecycleClick(t, g, cl, battle, "CDSTOP")
				if optionsState.track != 1 || c.NextTrack() != 1 || c.RequestedTrack() != 1 || c.Status() != audio.StatusIdle {
					t.Fatal("stop did not select and refresh one")
				}
				musicLifecycleClick(t, g, cl, battle, "CDPREV")
				if optionsState.track != 3 || c.NextTrack() != 3 || c.RequestedTrack() != 3 {
					t.Fatal("previous did not wrap and write Repeat request")
				}
			})
		}
	}
}

func TestMusicLifecyclePlayDoesNotRefreshDetails(t *testing.T) {
	for _, selected := range []int{0, 3} {
		t.Run(fmt.Sprint(selected), func(t *testing.T) {
			g, cl, _ := musicLifecycleShell(t, false)
			c := g.audioOwner.Music
			logical, text, wantCurrent := 2, "2", 3
			if selected == 0 {
				logical, text, wantCurrent = 0, retailNoDiscText, 1
			}
			c.SelectTrack(logical)
			g.audioPrefs.CDMode = 3
			c.SetRequestedTrack(1)
			optionsState.track = selected
			optionsPanel.SetText("TRACKNUM", text) // Equality leaves the distinct selection alone.
			musicLifecycleClick(t, g, cl, false, "CDPLAY")
			if c.CurTrack() != wantCurrent || c.RequestedTrack() != 1 || optionsPanel.TextOf("TRACKNUM") != text {
				t.Fatal("Play skipped zero, ran a detail refresh or used displayed text as selection")
			}
		})
	}
}

func TestMusicLifecyclePageSwitchDepartsOnceBeforeCue(t *testing.T) {
	for _, battle := range []bool{false, true} {
		t.Run(fmt.Sprint(battle), func(t *testing.T) {
			g, cl, out := musicLifecycleShell(t, battle)
			c := g.audioOwner.Music
			c.Play(2)
			optionsState.track = 2
			optionsPanel.SetText("TRACKNUM", "2")
			queries := 0
			c.SetPlaybackPoll(func() bool { queries++; return true })
			wantQueries := 0
			if battle {
				wantQueries = 1
			}
			cues := 0
			out.cue = func() {
				cues++
				if queries != wantQueries || !battle && c.Status() != audio.StatusIdle {
					t.Fatal("page-switch cue preceded music departure")
				}
			}
			musicLifecycleClick(t, g, cl, battle, "SOUND")
			out.cue = nil
			if queries != wantQueries || cues != 1 || optionsState.page != "sound" {
				t.Fatalf("page switch queries=%d cues=%d page=%s", queries, cues, optionsState.page)
			}
		})
	}
}

func TestMusicLifecycleRootRetentionAndDeparture(t *testing.T) {
	for _, battle := range []bool{false, true} {
		t.Run(fmt.Sprint(battle), func(t *testing.T) {
			g, cl, out := musicLifecycleShell(t, battle)
			c := g.audioOwner.Music
			if optionsState.track != 0 {
				t.Fatal("initial root selection is not zero")
			}
			c.Play(3)
			c.Configure(audio.ModeIdle, 0)
			optionsState.track = 3
			optionsPanel.SetText("TRACKNUM", "3")
			queries := 0
			c.SetPlaybackPoll(func() bool {
				queries++
				if c.Status() != audio.StatusIdle {
					t.Fatal("departure query preceded idle status clear")
				}
				return false
			})
			out.cue = func() {
				if c.Status() != audio.StatusIdle {
					t.Fatal("root cue preceded music departure")
				}
			}
			musicLifecycleClick(t, g, cl, battle, "PREV")
			out.cue = nil
			wantQueries, wantNext := 0, 1
			if battle {
				wantQueries, wantNext = 1, 3
			}
			if queries != wantQueries || c.NextTrack() != wantNext || optionsState != nil {
				t.Fatalf("departure queries=%d next=%d state=%v", queries, c.NextTrack(), optionsState)
			}
			if err := g.openRetailOptionsScreen(battle); err != nil {
				t.Fatal(err)
			}
			g.openRetailOptionsPage("music")
			if optionsState.track != 3 || optionsPanel.TextOf("TRACKNUM") != "3" {
				t.Fatal("root reopen discarded retained selection before its first poll")
			}
			musicOptionsAuditPass(g, cl, battle)
			if optionsState.track != wantNext {
				t.Fatal("later poll did not adopt logical next")
			}
		})
	}
}

func TestMusicLifecycleCancelAndUndoOrder(t *testing.T) {
	for _, battle := range []bool{false, true} {
		for _, action := range []string{"CANCEL", "UNDO"} {
			t.Run(fmt.Sprintf("battle=%v/%s", battle, action), func(t *testing.T) {
				g, cl, _ := musicLifecycleShell(t, battle)
				c := g.audioOwner.Music
				g.closeRetailOptionsScreen()
				g.audioPrefs.MusicMode = 0
				c.SetEnabled(false)
				c.SetRequestedTrack(2)
				if err := g.openRetailOptionsScreen(battle); err != nil {
					t.Fatal(err)
				}
				g.audioPrefs.MusicMode = 1
				c.SetEnabled(true)
				c.Configure(audio.ModeIdle, 0)
				c.Play(3)
				c.SetRequestedTrack(3)
				g.openRetailOptionsPage("music")
				optionsState.track = 3
				optionsPanel.SetText("TRACKNUM", "3")
				type observed struct {
					status        audio.StatusMode
					pref, request int
				}
				var seen []observed
				c.SetPlaybackPoll(func() bool {
					seen = append(seen, observed{c.Status(), g.audioPrefs.MusicMode, c.RequestedTrack()})
					return !(battle && action == "CANCEL" && len(seen) == 1)
				})
				musicLifecycleClick(t, g, cl, battle, action)
				want := []observed{{audio.StatusPlaying, 1, 3}}
				if action == "CANCEL" {
					want = []observed{{audio.StatusIdle, 1, 3}}
				}
				if battle {
					if action == "CANCEL" {
						want = append(want, observed{audio.StatusIdle, 1, 3})
					} else {
						want = append(want, observed{audio.StatusPlaying, 0, 2})
					}
				}
				if !reflect.DeepEqual(seen, want) {
					t.Fatalf("query state=%v want=%v", seen, want)
				}
				if g.audioPrefs.MusicMode != 0 || !c.IsEnabled() || c.RequestedTrack() != 2 {
					t.Fatal("restore conflated preference enable, live enable or request")
				}
				if action == "CANCEL" && optionsState != nil {
					t.Fatal("Cancel did not close root")
				}
				if action == "UNDO" && (optionsState == nil || optionsState.page != "music") {
					t.Fatal("Undo did not rebuild music")
				}
			})
		}
	}
}

func TestMusicLifecycleRestoreKeepsLiveModeAndEnable(t *testing.T) {
	for _, battle := range []bool{false, true} {
		t.Run(fmt.Sprint(battle), func(t *testing.T) {
			g, cl, _ := musicLifecycleShell(t, battle)
			c := g.audioOwner.Music
			c.SetEnabled(false)
			g.audioPrefs.MusicMode, g.audioPrefs.CDMode = 0, 2
			g.syncRetailMusicPage()
			queries := 0
			c.SetPlaybackPoll(func() bool {
				queries++
				if g.audioPrefs.MusicMode != 1 || g.audioPrefs.CDMode != 4 || c.IsEnabled() {
					t.Fatal("Restore query saw wrong preference/live state")
				}
				return false
			})
			musicLifecycleClick(t, g, cl, battle, "RESTORE")
			wantQueries, wantNext := 1, 1
			if battle {
				wantQueries, wantNext = 2, 3
			}
			if queries != wantQueries || c.NextTrack() != wantNext || c.IsEnabled() || g.audioPrefs.CDMode != 4 || g.audioPrefs.MusicMode != 1 {
				t.Fatalf("Restore queries=%d next=%d enabled=%v pref=%+v", queries, c.NextTrack(), c.IsEnabled(), g.audioPrefs)
			}
		})
	}
}

func TestMusicLifecycleSnapshotRequestRejectsOverCount(t *testing.T) {
	g, cl, _ := musicLifecycleShell(t, false)
	c := g.audioOwner.Music
	g.closeRetailOptionsScreen()
	c.SetRequestedTrack(3)
	if err := g.openRetailOptionsScreen(false); err != nil {
		t.Fatal(err)
	}
	g.openRetailOptionsPage("music")
	c.SetNumTracks(1)
	c.SetRequestedTrack(1)
	musicLifecycleClick(t, g, cl, false, "UNDO")
	if c.RequestedTrack() != 1 {
		t.Fatal("Undo assigned saved request above current track count")
	}
}
