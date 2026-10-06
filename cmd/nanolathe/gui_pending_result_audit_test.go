package main

import (
	"testing"

	"github.com/nanolathe-gg/nanolathe/internal/gui"
	"github.com/nanolathe-gg/nanolathe/internal/input"
	"github.com/nanolathe-gg/nanolathe/internal/ui"
)

// A linked non-button must reach the production screen callback using the
// target record's name, not merely move focus [07 R-WGT-01 §7].
func TestLinkedNonButtonReachesFrontendCallback(t *testing.T) {
	g, _, cl := editorMenuShell(t)
	p := ui.NewPanel(&gui.Window{Rect: gui.Rect{W: 64, H: 48}, Gadgets: []gui.Gadget{
		{Kind: gui.KindPanel},
		{Kind: gui.KindLabel, Name: "LABEL", Active: 1, QuickKey: 'L', Link: "SINGLE"},
		{Kind: gui.KindSurface, Name: "SINGLE", Active: 1},
	}})
	g.frontend.Panels.Replace(p)
	cl.Input().EnqueueToken(input.Token{Kind: input.TokenText, Rune: 'l'})
	g.menuInput(cl)
	if g.frontend.Mode != modeMenuSingle || cl.Input().PendingTokens() != 0 {
		t.Fatalf("linked target callback: mode=%v tokens=%d", g.frontend.Mode, cl.Input().PendingTokens())
	}
}

func TestLinkedEditorResultClosesProductionModal(t *testing.T) {
	g, underlying, cl := editorMenuShell(t)
	p := ui.NewPanel(&gui.Window{Rect: gui.Rect{W: 64, H: 48}, Gadgets: []gui.Gadget{
		{Kind: gui.KindPanel},
		{Kind: gui.KindLabel, Name: "LABEL", Active: 1, QuickKey: 'L', Link: "EDIT"},
		{Kind: gui.KindTextBox, Name: "EDIT", Active: 1, MaxChars: 8},
	}})
	p.SetFocus(1)
	g.frontend.Panels.PushModal(p)
	cl.Input().EnqueueToken(input.Token{Kind: input.TokenText, Rune: 'l'})
	g.menuInput(cl)
	if g.frontend.Panels.Modal() != nil || g.frontend.Panels.Top() != underlying || g.frontend.Panels.Len() != 1 {
		t.Fatal("linked editor result did not close the modal over its saved parent")
	}
	if p.Focused() != 2 || cl.Input().PendingTokens() != 0 {
		t.Fatalf("focus/token result=%d/%d", p.Focused(), cl.Input().PendingTokens())
	}
}

// The screen receives the first pointer result after the matrix, rather than
// the earlier default answer [07 R-WGT-01 §§1-2].
func TestFirstPointerOverridesMatrixInFrontendCallback(t *testing.T) {
	g, _, cl := editorMenuShell(t)
	p := ui.NewPanel(&gui.Window{Rect: gui.Rect{W: 64, H: 48}, Header: gui.Header{CrDefault: "DEFAULT"}, Gadgets: []gui.Gadget{
		{Kind: gui.KindPanel},
		{Kind: gui.KindButton, Name: "SINGLE", Active: 1, Attribs: 0x100, Rect: gui.Rect{W: 10, H: 10}},
		{Kind: gui.KindButton, Name: "DEFAULT", Active: 1},
	}})
	g.frontend.Panels.Replace(p)
	cl.Input().Mouse.SetPosition(2, 2)
	cl.Input().Mouse.SetButton(input.MouseButtonLeft, true)
	cl.Input().EnqueueToken(input.Token{Kind: input.TokenEdit, Key: input.KeyEnter})
	g.menuInput(cl)
	if g.frontend.Mode != modeMenuSingle {
		t.Fatalf("screen received matrix result instead of pointer result: mode=%v", g.frontend.Mode)
	}
}
