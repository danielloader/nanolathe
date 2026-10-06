package ui

import (
	"testing"

	"github.com/nanolathe-gg/nanolathe/internal/gui"
	"github.com/nanolathe-gg/nanolathe/internal/input"
)

// The stop test follows record 1 even when the matrix already fired. Hidden
// records still reach that test [07 R-WGT-01 §§1-2].
func TestMatrixResultVisitsFirstRecordBeforeFinalFocus(t *testing.T) {
	for _, active := range []uint8{0, 1} {
		p := NewPanel(&gui.Window{Rect: gui.Rect{W: 80, H: 20}, Header: gui.Header{CrDefault: "DEFAULT"}, Gadgets: []gui.Gadget{
			{Kind: gui.KindPanel},
			{Kind: gui.KindSurface, Active: active, Rect: gui.Rect{W: 10, H: 10}},
			{Kind: gui.KindButton, Name: "DEFAULT", Active: 1},
			{Kind: gui.KindSurface, Active: 1},
		}})
		p.SetFocus(3)
		visits := 0
		r := p.ServiceFrame(WidgetFrame{TokenMode: true, KeyNavigation: true, Tokens: []input.Token{{Kind: input.TokenEdit, Key: input.KeyEnter}}}, WidgetHooks{Surface: func(index int) {
			visits++
			if index != 1 || p.Focused() != 3 {
				t.Fatalf("matrix bypassed first visit or focused early: index=%d focus=%d", index, p.Focused())
			}
		}})
		if visits != int(active) || !r.Fired || r.FiredIndex != 2 || p.Focused() != 2 {
			t.Fatalf("active=%d visits=%d result=%+v focus=%d", active, visits, r, p.Focused())
		}
	}
}

func TestFirstPointerResultReplacesMatrixResult(t *testing.T) {
	p := NewPanel(&gui.Window{Rect: gui.Rect{W: 80, H: 20}, Header: gui.Header{CrDefault: "DEFAULT"}, Gadgets: []gui.Gadget{
		{Kind: gui.KindPanel},
		{Kind: gui.KindButton, Name: "POINTER", Active: 1, Attribs: 0x100, Rect: gui.Rect{W: 10, H: 10}},
		{Kind: gui.KindButton, Name: "DEFAULT", Active: 1},
	}})
	r := p.ServiceFrame(WidgetFrame{PointerX: 2, PointerY: 2, HeldButtons: 1,
		PointerEvents: []input.PointerEvent{{Kind: input.LeftDown, X: 2, Y: 2}},
		TokenMode:     true, KeyNavigation: true, Tokens: []input.Token{{Kind: input.TokenEdit, Key: input.KeyEnter}},
	}, WidgetHooks{ArtFrames: func(int) int { return 2 }})
	if !r.Fired || r.FiredIndex != 1 || r.FiredButton != 1 || r.ConsumedTokens != 1 || p.Focused() != 1 {
		t.Fatalf("pointer did not replace pending matrix result: %+v focus=%d", r, p.Focused())
	}
}

func TestMatrixResultRetiresFirstRecordCapture(t *testing.T) {
	p := NewPanel(&gui.Window{Rect: gui.Rect{W: 80, H: 20}, Header: gui.Header{CrDefault: "DEFAULT"}, Gadgets: []gui.Gadget{
		{Kind: gui.KindPanel},
		{Kind: gui.KindButton, Name: "PRESSED", Active: 1, Rect: gui.Rect{W: 10, H: 10}},
		{Kind: gui.KindButton, Name: "DEFAULT", Active: 1},
	}})
	r := p.ServiceFrame(WidgetFrame{PointerX: 2, PointerY: 2, HeldButtons: 1,
		PointerEvents: []input.PointerEvent{{Kind: input.LeftDown, X: 2, Y: 2}},
		TokenMode:     true, KeyNavigation: true, Tokens: []input.Token{{Kind: input.TokenEdit, Key: input.KeyEnter}},
	}, WidgetHooks{})
	if !r.Fired || r.FiredIndex != 2 || p.StatusAt(1) != 1 || p.CaptureIndex() != -1 {
		t.Fatalf("first press was skipped or retained capture past firing: %+v status=%d capture=%d", r, p.StatusAt(1), p.CaptureIndex())
	}
}

func TestMatrixStageAdvanceBelongsToSurvivingResult(t *testing.T) {
	for _, pointer := range []bool{false, true} {
		p := NewPanel(&gui.Window{Rect: gui.Rect{W: 80, H: 20}, Gadgets: []gui.Gadget{
			{Kind: gui.KindPanel},
			{Kind: gui.KindButton, Name: "POINTER", Active: 1, Attribs: 0x100, Rect: gui.Rect{W: 10, H: 10}},
			{Kind: gui.KindButton, Name: "FOCUSED", Active: 1, Stages: 3},
		}})
		p.SetFocus(2)
		frame := WidgetFrame{TokenMode: true, KeyNavigation: true, Tokens: []input.Token{{Kind: input.TokenText, Rune: ' '}}}
		if pointer {
			frame.PointerX, frame.PointerY, frame.HeldButtons = 2, 2, 1
			frame.PointerEvents = []input.PointerEvent{{Kind: input.LeftDown, X: 2, Y: 2}}
		}
		r := p.ServiceFrame(frame, WidgetHooks{ArtFrames: func(int) int { return 2 }})
		want := 2
		if pointer {
			want = 1
		}
		if !r.Fired || r.FiredIndex != want || r.StageAdvanced == pointer || p.StageAt(2) != 1 {
			t.Fatalf("pointer=%t result=%+v original stage=%d", pointer, r, p.StageAt(2))
		}
	}
}

// A rejected link erases the pending result, allowing later records; an absent
// name keeps the label as the result [07 R-WGT-01 §7].
func TestLabelResolutionControlsPendingMatrixResult(t *testing.T) {
	for _, missing := range []bool{false, true} {
		link := "INACTIVE"
		if missing {
			link = "ABSENT"
		}
		p := NewPanel(&gui.Window{Rect: gui.Rect{W: 80, H: 20}, Header: gui.Header{CrDefault: "DEFAULT"}, Gadgets: []gui.Gadget{
			{Kind: gui.KindPanel},
			{Kind: gui.KindLabel, Name: "LABEL", Active: 1, Link: link, Rect: gui.Rect{W: 10, H: 10}},
			{Kind: gui.KindSurface, Name: "LATER", Active: 1},
			{Kind: gui.KindButton, Name: "DEFAULT", Active: 1},
			{Kind: gui.KindButton, Name: "INACTIVE"},
		}})
		p.ServiceFrame(WidgetFrame{PointerX: 2, PointerY: 2, HeldButtons: 1, PointerEvents: []input.PointerEvent{{Kind: input.LeftDown, X: 2, Y: 2}}}, WidgetHooks{})
		visits := 0
		r := p.ServiceFrame(WidgetFrame{PointerX: 2, PointerY: 2,
			PointerEvents: []input.PointerEvent{{Kind: input.LeftUp, X: 2, Y: 2}},
			TokenMode:     true, KeyNavigation: true, Tokens: []input.Token{{Kind: input.TokenEdit, Key: input.KeyEnter}},
		}, WidgetHooks{Surface: func(int) { visits++ }})
		if missing {
			if !r.Fired || r.FiredIndex != 1 || visits != 0 {
				t.Fatalf("missing target did not fire label: %+v visits=%d", r, visits)
			}
		} else if r.Fired || visits != 1 {
			t.Fatalf("rejected link retained pending result: %+v visits=%d", r, visits)
		}
	}
}
