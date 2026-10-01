package main

import (
	"fmt"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nanolathe-gg/nanolathe/formats"
	"github.com/nanolathe-gg/nanolathe/internal/client"
	"github.com/nanolathe-gg/nanolathe/internal/drawlist"
	"github.com/nanolathe-gg/nanolathe/internal/frame"
	"github.com/nanolathe-gg/nanolathe/internal/mission"
	"github.com/nanolathe-gg/nanolathe/vfs"
)

type briefingConditionsCollector struct {
	drawlist.Sink
	conditions []drawlist.Glyphs
}

func (*briefingConditionsCollector) Clear()                   {}
func (*briefingConditionsCollector) Terrain(drawlist.Terrain) {}
func (*briefingConditionsCollector) Sprite(drawlist.Sprite)   {}
func (*briefingConditionsCollector) Fill(drawlist.Fill)       {}
func (*briefingConditionsCollector) Line(drawlist.Line)       {}
func (*briefingConditionsCollector) Points(drawlist.Points)   {}
func (*briefingConditionsCollector) Model(drawlist.Model)     {}
func (*briefingConditionsCollector) Fog(drawlist.Fog)         {}
func (*briefingConditionsCollector) Surface(drawlist.Surface) {}
func (*briefingConditionsCollector) Cursor(drawlist.Cursor)   {}
func (*briefingConditionsCollector) Expand()                  {}
func (*briefingConditionsCollector) Flash(drawlist.Flash)     {}
func (*briefingConditionsCollector) Halo(drawlist.Halo)       {}
func (s *briefingConditionsCollector) Glyphs(v drawlist.Glyphs) {
	if strings.HasPrefix(v.Text, "Wind Speed : ") || strings.HasPrefix(v.Text, "Gravity : ") {
		s.conditions = append(s.conditions, v)
	}
}

// PANORAMA paints the conditions of the hidden SOLARSYSTEM gadget before its
// frame guard. The side FNT, raw colors, gravity ratio and inclusive pen width
// must survive both software composition and deferred replay [08 R-CAMP-01 §2].
func TestBriefingConditionsUseHiddenGadgetFontAndClip(t *testing.T) {
	shell, _ := retailShellForTest(t)
	for side, campaignPath := range []string{"camps/arm campaign.tdf", "camps/core campaign.tdf"} {
		t.Run(fmt.Sprintf("side%d", side), func(t *testing.T) {
			shell.missionSide = side
			shell.openMenu(modeMenuMission)
			found := false
			for i, campaign := range shell.campaignOptions {
				if strings.EqualFold(campaign.Path, campaignPath) {
					shell.campaignIdx, found = i, true
					break
				}
			}
			if !found {
				t.Fatalf("reference campaign unavailable: %s", campaignPath)
			}
			shell.missionIdx = 0
			shell.openCampaignBriefing()
			if shell.briefing == nil || shell.briefingPanel == nil {
				t.Fatal("campaign briefing did not open")
			}
			panel, b := shell.briefingPanel, shell.briefing
			index := panel.Index("SOLARSYSTEM")
			if index < 0 || panel.ActiveAt(index) {
				t.Fatal("SOLARSYSTEM must remain hidden; PANORAMA owns its text")
			}
			gadget := panel.Window.Gadgets[index]
			font := shell.windowGadgetFont(panel, gadget)
			c, err := client.New(client.Options{Buffer: &frame.Buffer{}, Width: 640, Height: 480})
			if err != nil {
				t.Fatal(err)
			}
			c.SetFNT(shell.font)
			c.SetPalette(shell.assets.pal)
			c.SetUIStage(painterBindingStage(shell.drawBriefing))
			for _, sample := range []struct {
				raw  int
				text string
			}{{112, "Gravity : 1.0"}, {56, "Gravity : 0.5"}, {0, "Gravity : 0.0"}} {
				// Authored by the test: display reads the header rather than a
				// terrain-derived fallback or the battle's fixed-point gravity.
				doc, err := formats.ParseTDF([]byte(fmt.Sprintf("[GlobalHeader]{gravity=%d;}", sample.raw)))
				if err != nil {
					t.Fatal(err)
				}
				b.mission = &mission.Mission{OTA: &formats.OTA{Global: doc.Root.Section("GlobalHeader")}}
				random := &countingRand{}
				b.crt, b.windSpeed, b.countdown = random, 17, 63
				got := &briefingConditionsCollector{}
				c.RecordFrame().Replay(got)
				if len(got.conditions) != 2 {
					t.Fatalf("conditions = %d, want wind and gravity from the panorama callback", len(got.conditions))
				}
				if gadget.FontNumber != uint8(side+1) || font == nil {
					t.Fatal("loaded briefing text did not select the side FNT")
				}
				for i, text := range []string{"Wind Speed : 17", sample.text} {
					v := got.conditions[i]
					color := []byte{53, 117}[side]
					if v.Text != text || v.Font != font || v.Color != color || v.X != 495 || v.Y != int32(20+i*20) || v.MaxWidth != 144 || !v.HasClip || v.Clip != (drawlist.Rect{X: 415, Y: 0, W: 225, H: 107}) {
						t.Fatalf("condition %d: %+v", i, v)
					}
				}
				if random.n != 0 || b.countdown != 62 {
					t.Fatalf("conditions added random work: draws=%d countdown=%d", random.n, b.countdown)
				}
				if dir := os.Getenv("NANOLATHE_BRIEFING_CONDITIONS_SHOTS"); dir != "" && sample.raw == 112 {
					if err := os.MkdirAll(dir, 0o755); err != nil {
						t.Fatal(err)
					}
					file, err := os.Create(filepath.Join(dir, fmt.Sprintf("side%d.png", side)))
					if err != nil {
						t.Fatal(err)
					}
					err = png.Encode(file, c.ComposeFrame())
					closeErr := file.Close()
					if err != nil || closeErr != nil {
						t.Fatalf("briefing capture: %v / %v", err, closeErr)
					}
				}
			}
			// A bound callback prints conditions before its sequence guard.
			entry, ok := shell.assets.briefing.art.Find(b.planet.Panorama)
			if !ok {
				t.Fatal("stock panorama sequence unavailable")
			}
			frames := entry.Frames
			entry.Frames = nil
			got := &briefingConditionsCollector{}
			c.RecordFrame().Replay(got)
			entry.Frames = frames
			if len(got.conditions) != 2 {
				t.Fatal("the panorama frame guard suppressed the condition text")
			}
			// Callback binding survives loss of the sequence itself, not only
			// an empty frame list [08 R-CAMP-01 §2].
			name := entry.Name
			entry.Name = "missing panorama"
			got = &briefingConditionsCollector{}
			c.RecordFrame().Replay(got)
			entry.Name = name
			if len(got.conditions) != 2 {
				t.Fatal("loss of the bound sequence suppressed the condition text")
			}
		})
	}
}

// Retained artwork cannot install a callback after this visit's load failed.
// A later successful visit must bind again [08 R-CAMP-01 §2].
func TestBriefingConditionsRequireCurrentArtBinding(t *testing.T) {
	shell, _ := retailShellForTest(t)
	shell.openMenu(modeMenuMission)
	shell.campaignIdx, shell.missionIdx = 0, 0
	shell.openCampaignBriefing()
	b := shell.briefing
	if b == nil || shell.briefingPanel == nil {
		t.Fatal("campaign briefing did not open")
	}
	previous := shell.assets.briefing.art
	if _, ok := previous.Find(b.planet.Panorama); !ok {
		t.Fatal("stock panorama sequence unavailable")
	}
	c, err := client.New(client.Options{Buffer: &frame.Buffer{}, Width: 640, Height: 480})
	if err != nil {
		t.Fatal(err)
	}
	c.SetFNT(shell.font)
	c.SetPalette(shell.assets.pal)
	c.SetUIStage(painterBindingStage(shell.drawBriefing))
	assertConditions := func(want int) {
		t.Helper()
		b.crt, b.windSpeed, b.countdown = &countingRand{}, 17, 63
		got := &briefingConditionsCollector{}
		c.RecordFrame().Replay(got)
		if len(got.conditions) != want {
			t.Fatalf("conditions = %d, want %d", len(got.conditions), want)
		}
		if b.crt.(*countingRand).n != 0 {
			t.Fatal("condition drawing added random work")
		}
	}
	assertConditions(2)

	fullFS := shell.cs.fs
	emptyFS := vfs.New()
	t.Cleanup(func() { emptyFS.Close() })
	shell.cs.fs = emptyFS
	shell.briefingPanel = shell.loadBriefingPanel(b.planet)
	shell.cs.fs = fullFS
	if shell.briefingPanel == nil || shell.assets.briefing.art != previous {
		t.Fatal("failed current load must retain the previous artwork")
	}
	assertConditions(0)

	shell.briefingPanel = shell.loadBriefingPanel(b.planet)
	assertConditions(2)
	panorama := b.planet.Panorama
	b.planet.Panorama = "missing panorama"
	shell.briefingPanel = shell.loadBriefingPanel(b.planet)
	assertConditions(0)
	b.planet.Panorama = panorama
	assertConditions(0) // draw-time lookup cannot install the missing callback.
	shell.briefingPanel = shell.loadBriefingPanel(b.planet)
	assertConditions(2)
}
