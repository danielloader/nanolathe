package main

import (
	"strconv"
	"strings"
	"testing"

	"github.com/nanolathe-gg/nanolathe/formats"
	"github.com/nanolathe-gg/nanolathe/internal/gui"
	"github.com/nanolathe-gg/nanolathe/internal/input"
	"github.com/nanolathe-gg/nanolathe/internal/ui"
)

// Authored ordinary button art preserves the authored stage count. The
// production pointer pass advances it before the options callback stores the
// byte-sized setting [07 R-FE-01 §6].
func TestOptionsAuditCaptionSettingUsesByteProduct(t *testing.T) {
	for _, stages := range []int{3, 53} {
		t.Run(strconv.Itoa(stages)+" stages", func(t *testing.T) {
			g, panel, cl := syntheticOptionsPage(t, "speeds", func(w *gui.Window) error {
				art := &formats.GAFEntry{Frames: make([]formats.GAFFrameRef, stages+2)}
				for i := range art.Frames {
					art.Frames[i].Frame = &formats.GAFFrame{Width: 80, Height: 20, Pixels: make([]byte, 80*20)}
				}
				w.Gadgets = w.Gadgets[:1]
				w.Gadgets = append(w.Gadgets, gui.Gadget{
					Kind: gui.KindButton, Name: "UNITCHAT", SourceName: retailOptionsPageSource + "UNITCHAT",
					Active: 1, Rect: gui.Rect{X: 20, Y: 40, W: 80, H: 20},
					Stages: uint8(stages), Text: strings.TrimSuffix(strings.Repeat("x|", stages), "|"),
					ButtonArtResolved: true, ButtonArt: art,
				})
				return nil
			})
			g.messages.UnitChatText = 5
			g.refreshRetailOptionsPage()
			for stage := 2; stage < stages; stage++ {
				clickRowGadget(t, g, panel, cl, "UNITCHAT", input.MouseButtonLeft)
			}
			want := 10
			if stages == 53 {
				want = 4
			}
			if got := g.messages.UnitChatText; got != want {
				t.Fatalf("stored caption setting = %d, want %d", got, want)
			}
			if got := panel.StageAt(panel.Index("UNITCHAT")); got != stages-1 {
				t.Fatalf("live stage = %d, want %d", got, stages-1)
			}
			g.frontend.Panels.Pop()
			optionsPanel = ui.NewPanel(panel.Window)
			g.frontend.Panels.Push(optionsPanel)
			g.refreshRetailOptionsPage()
			if got := optionsPanel.StageAt(optionsPanel.Index("UNITCHAT")); got != want/5 {
				t.Fatalf("reopened stage = %d, want %d", got, want/5)
			}
		})
	}
}

// These no-art rectangles pass through the production slider builder and
// page-entry refresh. SCREEN retains the reciprocal's fraction before the
// ceiling and runs its callback even when the seeded knob exceeds the last
// pointer position [07 R-FE-01 §6].
func TestOptionsAuditScreenEntryReciprocal(t *testing.T) {
	for _, value := range []int{32, 65} {
		t.Run(strconv.Itoa(value), func(t *testing.T) {
			g, panel, _ := syntheticOptionsPage(t, "speeds", func(w *gui.Window) error {
				w.Gadgets = w.Gadgets[:1]
				w.Gadgets = append(w.Gadgets,
					gui.Gadget{Kind: gui.KindScrollBar, Name: "SCREEN", SourceName: retailOptionsPageSource + "SCREEN", Active: 1, Attribs: 1, Rect: gui.Rect{X: 20, Y: 40, W: 72, H: 10}},
					gui.Gadget{Kind: gui.KindScrollBar, Name: "TXTSCROL", SourceName: retailOptionsPageSource + "TXTSCROL", Active: 1, Attribs: 1, Rect: gui.Rect{X: 20, Y: 60, W: 13, H: 7}})
				return nil
			})
			g.buildRetailOptionsSliders(panel.Window)
			g.scrollSpeed, g.messages.TextScroll = value, 12
			g.refreshRetailOptionsPage()
			s := retailOptionsSliderNamed(g, "SCREEN")
			if s.travel != 66 || s.knob != value+1 || panel.SliderKnobAt(panel.Index("SCREEN")) != value+1 {
				t.Fatalf("entry slider = %+v, want travel66 and knob%d", s, value+1)
			}
			if g.scrollSpeed != value+1 {
				t.Fatalf("entry scroll setting = %d, want %d", g.scrollSpeed, value+1)
			}
			if g.messages.TextScroll != 13 {
				t.Fatalf("SPEEDS omitted the other slider callback: text scroll = %d", g.messages.TextScroll)
			}
		})
	}
}

// Both visual-page arms seed the same retained knob without feeding its
// quantised read-out back into gamma. Explicit knob changes still commit
// normally [07 R-FE-01 §6].
func TestOptionsAuditVisualEntryDoesNotCommitGamma(t *testing.T) {
	for _, inBattle := range []bool{false, true} {
		t.Run("inBattle="+strconv.FormatBool(inBattle), func(t *testing.T) {
			g, panel, _ := syntheticOptionsPage(t, "visuals", func(w *gui.Window) error {
				w.Gadgets = w.Gadgets[:1]
				w.Gadgets = append(w.Gadgets, gui.Gadget{Kind: gui.KindScrollBar, Name: "GAMMA", SourceName: retailOptionsPageSource + "GAMMA", Active: 1, Attribs: 1, Rect: gui.Rect{X: 20, Y: 40, W: 13, H: 7}})
				return nil
			})
			optionsState.inBattle = inBattle
			g.buildRetailOptionsSliders(panel.Window)
			g.display.Gamma = 12
			g.refreshRetailOptionsPage()
			s := retailOptionsSliderNamed(g, "GAMMA")
			if s.travel != 7 || s.knob != 4 || g.display.Gamma != 12 {
				t.Fatalf("visual entry slider=%+v gamma=%d, want travel7 knob4 gamma12", s, g.display.Gamma)
			}
			moveRetailSliderNamed(g, "GAMMA", s, 3)
			moveRetailSliderNamed(g, "GAMMA", s, 4)
			if g.display.Gamma != 13 {
				t.Fatalf("explicit knob change left gamma=%d, want13", g.display.Gamma)
			}
		})
	}
}
