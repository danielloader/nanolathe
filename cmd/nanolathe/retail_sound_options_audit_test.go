package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nanolathe-gg/nanolathe/formats"
	"github.com/nanolathe-gg/nanolathe/internal/audio"
	"github.com/nanolathe-gg/nanolathe/internal/client"
	"github.com/nanolathe-gg/nanolathe/internal/gui"
	"github.com/nanolathe-gg/nanolathe/internal/input"
	"github.com/nanolathe-gg/nanolathe/internal/settings"
	"github.com/nanolathe-gg/nanolathe/vfs"
)

type soundOptionsAuditOutput struct {
	config     audio.OutputConfig
	cue        func()
	musicOpens int
}

func (o *soundOptionsAuditOutput) ConfigureOutput(c audio.OutputConfig) { o.config = c }
func (o *soundOptionsAuditOutput) PlaySample(*audio.Sample, float64, float64) error {
	if o.cue != nil {
		o.cue()
	}
	return nil
}
func (o *soundOptionsAuditOutput) NewMusicPlayer(io.ReadSeeker, string) (audio.MusicPlayer, error) {
	o.musicOpens++
	return &exitMusicPlayer{}, nil
}

// The real page loader, art constructor, widget service and audio alias path
// consume these independently authored files. No retail assets are required.
func soundOptionsAuditShell(t *testing.T, speechStages int) (*gameShell, *client.Client, *soundOptionsAuditOutput) {
	t.Helper()
	g, _, cl := syntheticOptionsPage(t, "sound", func(w *gui.Window) error {
		w.Gadgets = w.Gadgets[:1]
		return nil
	})
	root := t.TempDir()
	files := []struct{ path, text string }{
		{"guis/soundsrt.gui", fmt.Sprintf(`
[GADGET0] { [COMMON] { id=0; name=PAGE; width=320; height=240; } }
[GADGET1] { [COMMON] { id=1; name=MODE; xpos=20; ypos=20; width=80; height=20; active=1; } stages=3; text=Off|Mono|3D; }
[GADGET2] { [COMMON] { id=1; name=SPEECH; xpos=20; ypos=50; width=80; height=20; active=1; } stages=%d; text=%s; }
[GADGET3] { [COMMON] { id=5; name=VOLTEXT; xpos=20; ypos=80; width=80; height=20; active=1; } text=Volume; }
[GADGET4] { [COMMON] { id=1; name=RESTORE; xpos=20; ypos=110; width=80; height=20; active=1; } text=Restore; }
[GADGET5] { [COMMON] { id=1; name=UNDO; xpos=120; ypos=110; width=80; height=20; active=1; } text=Undo; }
`, speechStages, strings.TrimSuffix(strings.Repeat("x|", speechStages), "|"))},
		{"guis/musicrt.gui", `
[GADGET0] { [COMMON] { id=0; name=PAGE; width=320; height=240; } }
[GADGET1] { [COMMON] { id=1; name=TRACKMODE; xpos=20; ypos=20; width=80; height=20; active=1; } stages=4; text=All|Random|Repeat|Custom; }
[GADGET2] { [COMMON] { id=3; name=TRACKNUM; xpos=20; ypos=50; width=80; height=20; active=1; } }
[GADGET3] { [COMMON] { id=1; name=TRACKTYPE; xpos=20; ypos=80; width=80; height=20; active=1; } stages=5; text=Building|Battle|Victory|Defeat|Unused; }
`},
		{"sounds/options.wav", string([]byte{128, 129, 127, 128})},
		{"music/1.wav", "authored fake driver media"},
		{"music/2.wav", "authored fake driver media"},
		{"music/3.wav", "authored fake driver media"},
	}
	for _, file := range files {
		path := filepath.Join(root, file.path)
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(file.text), 0644); err != nil {
			t.Fatal(err)
		}
	}
	fs := vfs.New()
	if err := fs.MountDirectory(root, 1); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { fs.Close() })
	g.cs = testContentSet(fs)
	g.assets = &menuAssets{common: &formats.GAF{}}
	for _, button := range []struct {
		name   string
		stages int
	}{{"MODE", 3}, {"SPEECH", speechStages}, {"TRACKMODE", 4}, {"TRACKTYPE", 5}, {"RESTORE", 0}, {"UNDO", 0}} {
		entry := formats.GAFEntry{Name: button.name, Frames: make([]formats.GAFFrameRef, button.stages+4)}
		for i := range entry.Frames {
			entry.Frames[i].Frame = &formats.GAFFrame{Width: 80, Height: 20, Pixels: make([]byte, 80*20)}
		}
		g.assets.common.Entries = append(g.assets.common.Entries, entry)
	}
	g.audioOwner = audio.NewService(fs)
	g.audioOwner.ConfigureMusic(false)
	g.audioOwner.Registry.RegisterPath("Options", "sounds/options.wav")
	g.frontendAliasesBound = true
	oldOutput := audio.GlobalOutput()
	out := &soundOptionsAuditOutput{}
	audio.SetGlobalOutput(out)
	t.Cleanup(func() {
		g.audioOwner.Music.Stop()
		audio.ConfigureOutput(audio.OutputConfig{MasterEnabled: true, EffectsVolume: 1, SoundMode: audio.SoundModeMono, MixingBuffers: 8})
		audio.SetGlobalOutput(oldOutput)
	})
	g.applyRetailAudioOptions()
	optionsState.inBattle = true
	optionsState.tracks = 3
	optionsState.snapshot.audio = g.audioPrefs
	g.openRetailOptionsPage("sound")
	if optionsPanel.Index("SPEECH") < 1 {
		t.Fatal("authored sound page failed to open")
	}
	return g, cl, out
}

func TestSoundOptionsAuditSpeechRetainsStageAcrossByteWrap(t *testing.T) {
	for _, stages := range []int{3, 53} {
		t.Run(fmt.Sprint(stages), func(t *testing.T) {
			g, cl, _ := soundOptionsAuditShell(t, stages)
			g.audioPrefs.SpeechFX, g.audioPrefs.UnitChat = 1, 5
			g.openRetailOptionsPage("sound")
			for stage := 2; stage < stages; stage++ {
				clickRowGadget(t, g, optionsPanel, cl, "SPEECH", input.MouseButtonLeft)
			}
			want := 10
			if stages == 53 {
				want = 4
			}
			if g.audioPrefs.UnitChat != want || g.audioPrefs.SpeechFX != 1 || optionsPanel.StageAt(optionsPanel.Index("SPEECH")) != stages-1 {
				t.Fatalf("level/enable/stage = %d/%d/%d", g.audioPrefs.UnitChat, g.audioPrefs.SpeechFX, optionsPanel.StageAt(optionsPanel.Index("SPEECH")))
			}
			g.openRetailOptionsPage("sound")
			if got := optionsPanel.StageAt(optionsPanel.Index("SPEECH")); got != want/5 {
				t.Fatalf("reopened speech stage = %d, want %d", got, want/5)
			}
		})
	}
}

func TestSoundOptionsAuditRepeatRestoresRequestWithoutPlaying(t *testing.T) {
	g, cl, out := soundOptionsAuditShell(t, 3)
	g.audioPrefs.CDMode = 2
	g.openRetailOptionsPage("music")
	c := g.audioOwner.Music
	c.Configure(audio.ModeRandom, 0)
	c.SetRequestedTrack(1)
	if !c.Play(2) {
		t.Fatal("authored media did not enter playing state")
	}
	optionsState.track = 3
	g.syncRetailMusicPage()
	opens := out.musicOpens
	clickRowGadget(t, g, optionsPanel, cl, "TRACKMODE", input.MouseButtonLeft)
	if g.audioPrefs.CDMode != 3 || optionsState.track != 1 || optionsPanel.TextOf("TRACKNUM") != "1" || c.RequestedTrack() != 1 {
		t.Fatalf("Repeat mode/selection/label/request = %d/%d/%q/%d", g.audioPrefs.CDMode, optionsState.track, optionsPanel.TextOf("TRACKNUM"), c.RequestedTrack())
	}
	if c.CurTrack() != 2 || c.NextTrack() != 2 || out.musicOpens != opens {
		t.Fatalf("Repeat immediately changed playback: current=%d next=%d opens=%d, want 2/2/%d", c.CurTrack(), c.NextTrack(), out.musicOpens, opens)
	}
	// The subsequent ordinary tick remains the playback owner. No page pump
	// runs between this callback assertion and the tick [03 R-AUD-01 §4].
	c.Tick(true)
	if c.CurTrack() != 1 || out.musicOpens != opens+1 {
		t.Fatal("later Repeat tick no longer plays the retained request")
	}
}

func TestSoundOptionsAuditModeAndSpeechCueOrder(t *testing.T) {
	g, cl, out := soundOptionsAuditShell(t, 3)
	for _, tc := range []struct{ from, to int }{{0, 1}, {2, 0}} {
		g.audioPrefs.SoundMode = tc.from
		g.applyRetailAudioOptions()
		g.syncRetailSoundPage()
		requests := 0
		out.cue = func() {
			requests++
			if g.audioPrefs.SoundMode != tc.to || out.config.MasterEnabled != (tc.to != 0) || optionsPanel.ActiveOf("VOLTEXT") != (tc.to != 0) || retailGadgetGreyed(optionsAssets.window, "SPEECH") != (tc.to == 0) {
				t.Fatalf("MODE cue preceded its setting, backend or control enable changes")
			}
		}
		clickRowGadget(t, g, optionsPanel, cl, "MODE", input.MouseButtonLeft)
		if requests != 1 {
			t.Fatalf("MODE emitted %d cue requests, want 1", requests)
		}
	}
	g.audioPrefs.SoundMode = 1
	g.audioPrefs.SpeechFX, g.audioPrefs.UnitChat = 1, 5
	g.applyRetailAudioOptions()
	g.syncRetailSoundPage()
	requests := 0
	out.cue = func() {
		requests++
		if g.audioPrefs.UnitChat != 5 {
			t.Fatal("SPEECH wrote its level before requesting the cue")
		}
	}
	clickRowGadget(t, g, optionsPanel, cl, "SPEECH", input.MouseButtonLeft)
	if requests != 1 || g.audioPrefs.UnitChat != 10 {
		t.Fatalf("SPEECH requests/level = %d/%d, want 1/10", requests, g.audioPrefs.UnitChat)
	}
}

func TestSoundOptionsAuditRestoreUndoCueAfterReopen(t *testing.T) {
	for _, action := range []string{"RESTORE", "UNDO"} {
		t.Run(action, func(t *testing.T) {
			g, cl, out := soundOptionsAuditShell(t, 3)
			optionsState.snapshot.audio = settings.DefaultAudio()
			g.audioPrefs.SoundMode, g.audioPrefs.UnitChat = 0, 0
			g.applyRetailAudioOptions()
			g.syncRetailSoundPage()
			previous := optionsPanel
			requests := 0
			out.cue = func() {
				requests++
				if optionsPanel == previous || g.audioPrefs.SoundMode != 1 || g.audioPrefs.UnitChat != 10 || !out.config.MasterEnabled || optionsPanel.StageAt(optionsPanel.Index("SPEECH")) != 2 {
					t.Fatal("sound-page cue preceded restored state or reconstructed controls")
				}
			}
			clickRowGadget(t, g, previous, cl, action, input.MouseButtonLeft)
			if requests != 1 {
				t.Fatalf("%s emitted %d requests, want 1", action, requests)
			}
		})
	}
}
