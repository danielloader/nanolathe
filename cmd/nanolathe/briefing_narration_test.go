package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/nanolathe-gg/nanolathe/formats"
	"github.com/nanolathe-gg/nanolathe/internal/audio"
	"github.com/nanolathe-gg/nanolathe/internal/client"
	"github.com/nanolathe-gg/nanolathe/internal/gui"
	"github.com/nanolathe-gg/nanolathe/internal/input"
	"github.com/nanolathe-gg/nanolathe/internal/ui"
	"github.com/nanolathe-gg/nanolathe/vfs"
)

type briefingCallbackOutput struct{ events []string }

func (o *briefingCallbackOutput) PlaySample(s *audio.Sample, _, _ float64) error {
	o.events = append(o.events, s.Alias)
	return nil
}
func (o *briefingCallbackOutput) PlayStream(*audio.Sample, float64) error {
	o.events = append(o.events, "stream start")
	return nil
}
func (o *briefingCallbackOutput) StopStream() { o.events = append(o.events, "stream stop") }

func TestBriefingCallbackCueAndStopOrder(t *testing.T) {
	for _, name := range []string{"SHUTUP", "PrevMenu", "Start"} {
		t.Run(name, func(t *testing.T) {
			fs := authoredVoiceFS(t, "prior")
			svc := audio.NewService(fs)
			svc.Init(nil)
			for _, alias := range []string{"Options", "SmallButton", "Previous", "BigButton"} {
				if _, err := svc.Cache.Put(alias, bytes.Repeat([]byte{0x80}, 64)); err != nil {
					t.Fatal(err)
				}
			}
			o := &briefingCallbackOutput{}
			previous := audio.GlobalOutput()
			audio.SetGlobalOutput(o)
			t.Cleanup(func() { audio.SetGlobalOutput(previous) })
			svc.StartStream("sounds/prior.wav", 0, 0, 0)
			svc.TickStream(0)
			if !slices.Equal(o.events, []string{"stream start"}) {
				t.Fatalf("prior stream: %v", o.events)
			}
			o.events = nil
			p := ui.NewPanel(&gui.Window{Rect: gui.Rect{W: 200, H: 100}, Gadgets: []gui.Gadget{
				{Kind: gui.KindPanel, Active: 1},
				{Kind: gui.KindButton, Name: name, Active: 1, Stages: 2, Rect: gui.Rect{X: 10, Y: 10, W: 80, H: 20}},
			}})
			p.SetStageAt(1, 1)
			// No mission narration key: these callbacks still operate on a
			// stream which was already running [07 R-FE-01 §4].
			b := NewCampaignBriefingController(nil, 0, nil, func() (freshBattleRequest, error) {
				o.events = append(o.events, "validate")
				return freshBattleRequest{}, errors.New("authored start rejection")
			})
			g := &gameShell{cs: testContentSet(fs), frontend: ui.NewFrontend(modeMenuMission), briefing: b, briefingPanel: p, audioOwner: svc, frontendAliasesBound: true}
			if name == "Start" {
				g.assets = &menuAssets{message: &retailPanelAssets{window: &gui.Window{Gadgets: []gui.Gadget{
					{Kind: gui.KindPanel, Active: 1}, {Kind: gui.KindButton, Name: "OK", Active: 1, Rect: gui.Rect{W: 30, H: 15}},
				}}}}
				g.font = &formats.FNT{Height: 8}
			}
			cl, err := client.New(client.Options{Width: 200, Height: 100})
			if err != nil {
				t.Fatal(err)
			}
			cl.Input().Mouse.SetPosition(20, 20)
			cl.Input().Mouse.SetButton(input.MouseButtonLeft, true)
			g.briefingInput(cl)
			if len(o.events) != 0 {
				t.Fatalf("press emitted callback: %v", o.events)
			}
			cl.Input().Mouse.SetButton(input.MouseButtonLeft, false)
			g.briefingInput(cl)
			want := []string{"Options", "stream stop", "SmallButton"}
			switch name {
			case "PrevMenu":
				want = []string{"stream stop", "Previous"}
				if b.State() != BriefingClosed || g.briefing != nil {
					t.Fatal("PrevMenu did not close briefing")
				}
			case "Start":
				want = []string{"BigButton", "validate"}
				if b.State() != BriefingOpen {
					t.Fatal("rejected Start closed briefing")
				}
			}
			if !slices.Equal(o.events, want) {
				t.Fatalf("callback events=%v, want %v", o.events, want)
			}
		})
	}
}

func TestBriefingStartWithoutNarrationStopsOnlyAfterAdmission(t *testing.T) {
	admitted := false
	b := NewCampaignBriefingController(nil, 0, nil, func() (freshBattleRequest, error) {
		if !admitted {
			return freshBattleRequest{}, errors.New("authored rejection")
		}
		return freshBattleRequest{}, nil
	})
	if event, err := b.Dispatch(BriefingActionStart); err == nil || len(event.Audio) != 0 || b.State() != BriefingOpen {
		t.Fatalf("rejected start: event=%#v error=%v state=%v", event, err, b.State())
	}
	admitted = true
	event, err := b.Dispatch(BriefingActionStart)
	if err != nil || !event.Valid || len(event.Audio) != 1 || event.Audio[0].Kind != BriefingAudioStop || b.State() != BriefingClosed {
		t.Fatalf("admitted start: event=%#v error=%v state=%v", event, err, b.State())
	}
}

type briefingStreamOutput struct {
	plays, stops int
}

func (*briefingStreamOutput) PlaySample(*audio.Sample, float64, float64) error { return nil }
func (o *briefingStreamOutput) PlayStream(*audio.Sample, float64) error        { o.plays++; return nil }
func (o *briefingStreamOutput) StopStream()                                    { o.stops++ }

func installBriefingStreamOutput(t *testing.T) *briefingStreamOutput {
	t.Helper()
	old := audio.GlobalOutput()
	o := &briefingStreamOutput{}
	audio.SetGlobalOutput(o)
	t.Cleanup(func() { audio.SetGlobalOutput(old) })
	return o
}

// A complete click must deliver one staged callback, not a second toggle on
// the initial down edge [07 R-WGT-01 §3][03 R-AUD-02 §1].
func TestBriefingNarrationClickStopsAndRestartsOnce(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "camps", "briefs"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "camps", "briefs", "voice.wav"), loopTestWAV(), 0644); err != nil {
		t.Fatal(err)
	}
	fs := vfs.New()
	if err := fs.MountDirectory(root, 1); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { fs.Close() })
	o := installBriefingStreamOutput(t)
	p := ui.NewPanel(&gui.Window{Rect: gui.Rect{W: 200, H: 100}, Gadgets: []gui.Gadget{
		{Kind: gui.KindPanel, Active: 1},
		{Kind: gui.KindButton, Name: "SHUTUP", Active: 1, Stages: 2, Rect: gui.Rect{X: 10, Y: 10, W: 80, H: 20}},
	}})
	p.SetStageAt(1, 1)
	g := &gameShell{briefing: NewCampaignBriefingController(briefingMission(t, "Lava"), 0, &countingRand{}, nil), briefingPanel: p, audioOwner: audio.NewService(fs)}
	cl, err := client.New(client.Options{Width: 200, Height: 100})
	if err != nil {
		t.Fatal(err)
	}
	g.consumeBriefingAudio(g.briefing.OpeningAudio())
	g.audioOwner.TickStream(60)
	if o.plays != 1 {
		t.Fatal("opening narration never played")
	}
	g.briefingNowMS = 2000
	cl.Input().Mouse.SetPosition(20, 20)
	cl.Input().Mouse.SetButton(input.MouseButtonLeft, true)
	g.briefingInput(cl)
	if !g.briefing.NarrationOn() || o.stops != 0 {
		t.Fatal("press fired narration before release")
	}
	cl.Input().Mouse.SetButton(input.MouseButtonLeft, false)
	g.briefingInput(cl)
	if g.briefing.NarrationOn() || p.StageAt(1) != 0 || o.stops != 1 {
		t.Fatal("release did not stop narration exactly once")
	}
	g.audioOwner.TickStream(120)
	if o.plays != 1 {
		t.Fatal("stop click also scheduled a restart")
	}
	cl.Input().Mouse.SetButton(input.MouseButtonLeft, true)
	g.briefingInput(cl)
	cl.Input().Mouse.SetButton(input.MouseButtonLeft, false)
	g.briefingInput(cl)
	if !g.briefing.NarrationOn() || p.StageAt(1) != 1 {
		t.Fatal("second click did not enable narration")
	}
	g.audioOwner.TickStream(119)
	if o.plays != 1 {
		t.Fatal("restart ignored the authored delay")
	}
	g.audioOwner.TickStream(120)
	if o.plays != 2 || o.stops != 1 {
		t.Fatalf("restart plays/stops=%d/%d", o.plays, o.stops)
	}
	cl.Input().Mouse.SetButton(input.MouseButtonLeft, true)
	g.briefingInput(cl)
	cl.Input().Mouse.SetPosition(150, 70)
	cl.Input().Mouse.SetButton(input.MouseButtonLeft, false)
	g.briefingInput(cl)
	if !g.briefing.NarrationOn() || p.StageAt(1) != 1 || o.stops != 1 {
		t.Fatal("release outside narration button changed its state")
	}
}

// An authored third stage requests narration again before the opening delay
// expires; the controller does not turn a nonzero-to-nonzero change into stop
// [07 R-FE-01 §4]. Timer overlap itself belongs to the audio owner.
func TestBriefingNarrationThreeStageRelease(t *testing.T) {
	for _, hasNarration := range []bool{true, false} {
		t.Run(map[bool]string{true: "narration", false: "absent-key"}[hasNarration], func(t *testing.T) {
			root := t.TempDir()
			if err := os.MkdirAll(filepath.Join(root, "camps", "briefs"), 0755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(root, "camps", "briefs", "voice.wav"), loopTestWAV(), 0644); err != nil {
				t.Fatal(err)
			}
			fs := vfs.New()
			if err := fs.MountDirectory(root, 1); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { fs.Close() })
			o := installBriefingStreamOutput(t)
			p := ui.NewPanel(&gui.Window{Rect: gui.Rect{W: 200, H: 100}, Gadgets: []gui.Gadget{
				{Kind: gui.KindPanel, Active: 1},
				{Kind: gui.KindButton, Name: "SHUTUP", Text: "Quiet|Read|Replay", Active: 1, Stages: 3, Rect: gui.Rect{X: 10, Y: 10, W: 80, H: 20}},
			}})
			p.SetStageAt(1, 1)
			m := briefingMission(t, "Lava")
			if !hasNarration {
				m = nil
			}
			b := NewCampaignBriefingController(m, 0, &countingRand{}, nil)
			g := &gameShell{briefing: b, briefingPanel: p, audioOwner: audio.NewService(fs)}
			cl, err := client.New(client.Options{Width: 200, Height: 100})
			if err != nil {
				t.Fatal(err)
			}
			g.consumeBriefingAudio(b.OpeningAudio())
			g.briefingNowMS = 333 // presentation tick 9, before the opening delay
			cl.Input().Mouse.SetPosition(20, 20)
			click := func() {
				cl.Input().Mouse.SetButton(input.MouseButtonLeft, true)
				g.briefingInput(cl)
				cl.Input().Mouse.SetButton(input.MouseButtonLeft, false)
				g.briefingInput(cl)
			}
			click()
			if p.StageAt(1) != 2 || !b.NarrationOn() || o.stops != 0 || o.plays != 0 {
				t.Fatalf("first release: stage=%d on=%t plays/stops=%d/%d", p.StageAt(1), b.NarrationOn(), o.plays, o.stops)
			}
			// Both requested deadlines have elapsed. The exact overlap cadence
			// is tested by the audio owner, independently of this callback.
			g.audioOwner.TickStream(69)
			if hasNarration && o.plays == 0 || !hasNarration && o.plays != 0 {
				t.Fatalf("narration key=%t, starts=%d", hasNarration, o.plays)
			}
			if !hasNarration {
				// An absent mission key does not exempt an existing stream
				// from the zero-stage stop request.
				g.audioOwner.StartStream("camps/briefs/voice.wav", 0, 0, 69)
				g.audioOwner.TickStream(69)
			}
			click()
			if p.StageAt(1) != 0 || b.NarrationOn() || o.stops != 1 {
				t.Fatalf("second release: stage=%d on=%t stops=%d", p.StageAt(1), b.NarrationOn(), o.stops)
			}
		})
	}
}

func TestBriefingNarrationStageRequests(t *testing.T) {
	b := NewCampaignBriefingController(briefingMission(t, "Lava"), 0, &countingRand{}, nil)
	for _, stage := range []int{2, 2, 1} {
		got := b.DispatchNarrationStage(stage)
		if len(got) != 1 || got[0].Kind != BriefingAudioStart || got[0].Path != "camps/briefs/voice.wav" || got[0].Delay != 60 || got[0].Volume != 0 {
			t.Fatalf("stage %d request = %#v", stage, got)
		}
	}
	b.narrationPath = ""
	if got := b.DispatchNarrationStage(2); len(got) != 0 {
		t.Fatalf("absent narration key requested start: %#v", got)
	}
	if got := b.DispatchNarrationStage(0); len(got) != 1 || got[0].Kind != BriefingAudioStop {
		t.Fatalf("absent narration key suppressed stop: %#v", got)
	}
	b.state = BriefingClosed
	if got := b.DispatchNarrationStage(1); len(got) != 0 {
		t.Fatalf("closed briefing dispatched narration: %#v", got)
	}
}

// Reopening a briefing starts its clock before scheduling the delayed stream;
// a previous visit's elapsed time must not lengthen the delay [08 R-CAMP-01 §2].
func TestRetailBriefingNarrationReopenDelay(t *testing.T) {
	g, _ := retailShellForTest(t)
	o := installBriefingStreamOutput(t)
	g.missionSide = 0
	g.openMenu(modeMenuMission)
	found := false
	for i, campaign := range g.campaignOptions {
		if campaign.Path == "camps/arm campaign.tdf" {
			g.campaignIdx, found = i, true
			break
		}
	}
	if !found {
		t.Fatal("Arm campaign missing")
	}
	g.missionIdx = 0
	g.briefingNowMS = 50000
	g.openCampaignBriefing()
	if g.briefing == nil || g.briefingPanel == nil {
		t.Fatal("briefing did not open")
	}
	idx := g.briefingPanel.Index("SHUTUP")
	if idx < 0 || g.briefingPanel.StageAt(idx) != 1 {
		t.Fatal("narration toggle did not start enabled")
	}
	g.audioOwner.TickStream(59)
	if o.plays != 0 {
		t.Fatal("narration started before its delay")
	}
	g.audioOwner.TickStream(60)
	if o.plays != 1 {
		t.Fatalf("reopened narration plays=%d; path=%q", o.plays, g.briefing.narrationPath)
	}
	cl, err := client.New(client.Options{Width: 640, Height: 480})
	if err != nil {
		t.Fatal(err)
	}
	r := g.briefingPanel.Window.PlacedRect(idx)
	cl.Input().Mouse.SetPosition(float32(r.X+r.W/2), float32(r.Y+r.H/2))
	cl.Input().Mouse.SetButton(input.MouseButtonLeft, true)
	g.briefingInput(cl)
	cl.Input().Mouse.SetButton(input.MouseButtonLeft, false)
	g.briefingInput(cl)
	if g.briefing.NarrationOn() || g.briefingPanel.StageAt(idx) != 0 || o.stops != 1 {
		t.Fatal("stock narration button did not stop the decoded stream once")
	}
}
