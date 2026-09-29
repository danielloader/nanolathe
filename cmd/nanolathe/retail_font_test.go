package main

import (
	"reflect"
	"testing"

	"github.com/nanolathe-gg/nanolathe/formats"
	"github.com/nanolathe-gg/nanolathe/internal/client"
	contentprofiles "github.com/nanolathe-gg/nanolathe/internal/content/profiles"
	"github.com/nanolathe-gg/nanolathe/internal/frame"
	"github.com/nanolathe-gg/nanolathe/internal/gui"
	"github.com/nanolathe-gg/nanolathe/internal/ui"
)

func syntheticRetailGAFFont() *formats.GAFEntry {
	font := &formats.GAFEntry{Frames: make([]formats.GAFFrameRef, 256)}
	font.Frames[' '] = formats.GAFFrameRef{Frame: &formats.GAFFrame{Width: 7, Height: 12}}
	font.Frames['A'] = formats.GAFFrameRef{Frame: &formats.GAFFrame{Width: 8, Height: 12}}
	font.Frames['B'] = formats.GAFFrameRef{Frame: &formats.GAFFrame{Width: 5, Height: 12}}
	font.Frames['I'] = formats.GAFFrameRef{Frame: &formats.GAFFrame{Width: 5, Height: 12}}
	return font
}

func TestRetailGAFTextMetrics(t *testing.T) {
	font := syntheticRetailGAFFont()
	if got := retailGAFTextWidth(font, "A B\x00B"); got != 20 {
		t.Fatalf("width %d, want 20", got)
	}
	// The GAF path skips control bytes instead of applying the FNT newline
	// terminator [07 §4].
	if got := retailGAFTextWidth(font, "A\nB"); got != 13 {
		t.Fatalf("control-byte width %d, want 13", got)
	}
	if got := retailGAFTextHeight(font); got != 14 {
		t.Fatalf("height %d, want I-frame height + 2 = 14", got)
	}
	if got := retailGAFBaselineHeight(font); got != 12 {
		t.Fatalf("baseline normalization %d, want I-frame height 12", got)
	}
	r := gui.Rect{Y: 393, H: 20}
	if got := retailTextPenY(gui.Gadget{Stages: 1}, r, 14); got != 396 {
		t.Fatalf("staged pen y %d, want 396", got)
	}
	if got := retailTextPenY(gui.Gadget{}, r, 14); got != 395 {
		t.Fatalf("unstaged pen y %d, want 395", got)
	}
}

// [07 R-FE-01 §3] halves the primary font's width with integer division,
// and each menu open starts from the authored (initially hidden) label.
func TestMainMenuVersionPlacementOnReopen(t *testing.T) {
	font := syntheticRetailGAFFont()
	for _, code := range []byte("v3.1") {
		font.Frames[code] = formats.GAFFrameRef{Frame: &formats.GAFFrame{Width: 3}}
	}
	font.Frames['v'].Frame.Width = 4 // Total width 13: the shift must be 6.
	window := &gui.Window{Gadgets: []gui.Gadget{
		{Kind: gui.KindPanel},
		{Kind: gui.KindLabel, Name: "DebugString", Text: "authored", Rect: gui.Rect{X: 320, Y: 460, W: 100, H: 20}},
	}}
	shell := &gameShell{
		assets: &menuAssets{
			panel:   map[shellMode]*retailPanelAssets{modeMenuMain: {window: window}},
			gafFont: &formats.GAF{Entries: []formats.GAFEntry{*font}},
		},
		font: fixedWidthFont(10), // Different fallback metrics must not win.
	}
	for open := 0; open < 2; open++ {
		shell.openMenu(modeMenuMain)
		panel := shell.activePanel()
		if !panel.ActiveOf("DebugString") || panel.TextOf("DebugString") != "v3.1" {
			t.Fatalf("open %d: version label active=%t text=%q", open, panel.ActiveOf("DebugString"), panel.TextOf("DebugString"))
		}
		want := window.Gadgets[1].Rect
		want.X = 314
		if got := panel.Window.Gadgets[1].Rect; got != want {
			t.Fatalf("open %d: version rectangle = %+v, want %+v", open, got, want)
		}
	}
	if got := window.Gadgets[1]; got.Rect.X != 320 || got.Active != 0 || got.Text != "authored" {
		t.Fatalf("cached authored label changed: %+v", got)
	}
}

// A content profile's `main_menu_version` replaces the retail literal, and
// the replacement is shifted by its own primary-font width, exactly as
// retail shifts the literal [07 R-FE-01 §3]. The shipped `prota` profile
// names 4.8, the version string ProTA 4.8's patch list configures, and the
// layout is ProTA's: its label sits at x 323 so that `4.8` lands under its
// title art (research/extensions/prota-engine.md "Main-menu version label").
// The retail profile names none, so stock keeps `v3.1`.
func TestMainMenuVersionFromContentProfile(t *testing.T) {
	retail, err := contentprofiles.Lookup(contentprofiles.RetailName)
	if err != nil {
		t.Fatal(err)
	}
	if got := (&gameShell{cs: &contentSet{presentation: retail.Presentation}}).mainMenuVersion(); got != "v3.1" {
		t.Fatalf("retail profile version = %q, want the retail literal", got)
	}
	profile, err := contentprofiles.Lookup("prota")
	if err != nil {
		t.Fatal(err)
	}
	font := syntheticRetailGAFFont()
	for code, width := range map[byte]uint16{'4': 9, '.': 5, '8': 7} {
		font.Frames[code] = formats.GAFFrameRef{Frame: &formats.GAFFrame{Width: width}}
	}
	window := &gui.Window{Gadgets: []gui.Gadget{
		{Kind: gui.KindPanel},
		{Kind: gui.KindLabel, Name: "DebugString", Text: "Debug Build", Rect: gui.Rect{X: 323, Y: 416, W: 300}},
	}}
	shell := &gameShell{
		cs: &contentSet{presentation: profile.Presentation},
		assets: &menuAssets{
			panel:   map[shellMode]*retailPanelAssets{modeMenuMain: {window: window}},
			gafFont: &formats.GAF{Entries: []formats.GAFEntry{*font}},
		},
		font: fixedWidthFont(10),
	}
	shell.openMenu(modeMenuMain)
	panel := shell.activePanel()
	if got := panel.TextOf("DebugString"); got != "4.8" {
		t.Fatalf("version label text = %q, want the prota profile's 4.8", got)
	}
	if got := panel.Window.Gadgets[1].Rect.X; got != 323-21/2 {
		t.Fatalf("version label x = %d, want 323 - trunc(21/2)", got)
	}
	if got := (&gameShell{}).mainMenuVersion(); got != "v3.1" {
		t.Fatalf("version without a profile = %q, want the retail literal", got)
	}
}

// The label painter's pen has no inset: right is `gx + w - tw`, centre is
// `gx + trunc(w/2) - trunc(tw/2)` — two truncations, which differ from one
// when w is even and tw odd — and anything else, DebugString's attribute
// word 80 included, is `gx`. Right wins over centre, and an authored x of -1
// centres on the panel [03 R-FONT-01 §6].
func TestRetailLabelPenX(t *testing.T) {
	r := gui.Rect{X: 100, W: 50}
	for _, tc := range []struct {
		attribs uint32
		width   int
		want    int
	}{
		{0, 21, 100},
		{80, 21, 100},
		{1, 21, 100},
		{4, 21, 129},
		{6, 21, 129},
		{2, 21, 115},
	} {
		if got := retailLabelPenX(nil, gui.Gadget{Attribs: tc.attribs}, r, tc.width); got != tc.want {
			t.Errorf("attribs %d width %d: pen x %d, want %d", tc.attribs, tc.width, got, tc.want)
		}
	}
	window := &gui.Window{Rect: gui.Rect{X: 40, W: 200}}
	if got := retailLabelPenX(window, gui.Gadget{Rect: gui.Rect{RawX: -1}}, r, 20); got != 130 {
		t.Errorf("authored x -1: pen x %d, want the panel-centred 130", got)
	}
}

// The GAF pen's wrapper [03 R-FONT-01 §6]: whole words up to maxW (an exact
// fit stays on the line), carriage returns break, an over-long first word
// yields an empty line and loses one leading byte per line, and the height
// is tested after each line, so the line that exhausts maxH is still drawn.
func TestRetailLabelWrapLines(t *testing.T) {
	measure := func(s string) int { return len(s) }
	for _, tc := range []struct {
		text       string
		maxW, maxH int
		want       []string
	}{
		{"aaa bbb ccc", 7, 100, []string{"aaa bbb", "ccc"}},
		{"aaa bbb", 7, 100, []string{"aaa bbb"}},
		{"ab\rcd", 10, 100, []string{"ab", "cd"}},
		{"aa bb cc dd", 2, 15, []string{"aa", "bb"}},
		{"aa bb", 2, 1, []string{"aa"}},
		{"abcdef gh", 3, 100, []string{"", "", "", "def", "gh"}},
	} {
		if got := retailLabelWrapLines(tc.text, measure, tc.maxW, tc.maxH, 14); !reflect.DeepEqual(got, tc.want) {
			t.Errorf("wrap %q in %dx%d = %q, want %q", tc.text, tc.maxW, tc.maxH, got, tc.want)
		}
	}
}

func TestRetailMainMenuUsesPrimaryGAFGlyphPixels(t *testing.T) {
	root := probeRetail(t)
	cs, err := openContent(Options{Root: root})
	if err != nil {
		t.Skipf("retail assets unavailable: %v", err)
	}
	defer cs.Close()

	assets := loadMenuAssets(cs)
	shell := &gameShell{cs: cs, assets: assets, frontend: ui.NewFrontend(modeMenuMain), font: assets.font}
	shell.openMenu(modeMenuMain)
	if shell.retailGAFTextFont() == nil {
		t.Fatal("primary anims/hattfont12.gaf slot was not loaded")
	}
	cl, err := client.New(client.Options{Buffer: &frame.Buffer{}, Width: 640, Height: 480})
	if err != nil {
		t.Fatal(err)
	}
	cl.SetPalette(assets.pal)
	cl.SetUIStage(gameShellUIStage{shell: shell})
	image := cl.ComposeFrame()

	var single gui.Gadget
	found := false
	for _, gadget := range shell.frontend.Panels.Top().Window.Gadgets {
		if gadget.Name == "SINGLE" {
			single, found = gadget, true
			break
		}
	}
	if !found {
		t.Fatal("SINGLE gadget not found")
	}
	r := single.Rect
	textWidth := shell.retailTextWidth(single.Text)
	x := int(r.X) + (int(r.W)-1-textWidth)/2 + 1
	penY := retailTextPenY(single, r, shell.retailTextHeight())
	frame := retailGAFGlyph(shell.retailGAFTextFont(), 'S')
	if frame == nil {
		t.Fatal("hattfont12 S frame missing")
	}
	// Font loading subtracts I.Height from each frame YOffset, then glyph
	// placement subtracts that normalized offset from the pen [07 §4].
	x -= int(frame.XOffset)
	y := penY - (int(frame.YOffset) - retailGAFBaselineHeight(shell.retailGAFTextFont()))
	if y != penY+1 {
		t.Fatalf("stock hattfont12 draw y %d, want pen y %d + 1", y, penY)
	}
	for py := 0; py < int(frame.Height); py++ {
		for px := 0; px < int(frame.Width); px++ {
			index := py*int(frame.Width) + px
			if frame.Transparent[index] {
				continue
			}
			rr, gg, bb, aa := assets.pal.RGBA(frame.Pixels[index])
			got := image.RGBAAt(x+px, y+py)
			if got.R != rr || got.G != gg || got.B != bb || got.A != aa {
				t.Fatalf("S pixel (%d,%d) = %v, want direct GAF palette pixel (%d,%d,%d,%d)", px, py, got, rr, gg, bb, aa)
			}
		}
	}
}

// The main menu's version label is a label with no font record, so the label
// painter's GAF branch draws it: in GAF slot 1 (`hattfont11`), pen at the
// shifted rectangle's x with no inset and at its own y [03 R-FONT-01 §6]. The
// shift itself measured with slot 0 (`hattfont12`) [07 R-FE-01 §3]. A retail
// capture of the stock menu shows `v3.1` in the small face with its left edge
// at x 307.
func TestRetailMainMenuVersionUsesLabelFace(t *testing.T) {
	root := probeRetail(t)
	cs, err := openContent(Options{Root: root})
	if err != nil {
		t.Skipf("retail assets unavailable: %v", err)
	}
	defer cs.Close()

	assets := loadMenuAssets(cs)
	shell := &gameShell{cs: cs, assets: assets, frontend: ui.NewFrontend(modeMenuMain), font: assets.font}
	shell.openMenu(modeMenuMain)
	small := shell.retailGAFLabelFont()
	if small == nil {
		t.Fatal("secondary anims/hattfont11.gaf slot was not loaded")
	}
	panel := shell.activePanel()
	r := panel.Window.Gadgets[panel.Window.GadgetIndex("DebugString")].Rect
	if want := int32(320 - shell.retailTextWidth("v3.1")/2); r.X != want || r.Y != 300 {
		t.Fatalf("version label at (%d,%d), want (%d,300)", r.X, r.Y, want)
	}
	cl, err := client.New(client.Options{Buffer: &frame.Buffer{}, Width: 640, Height: 480})
	if err != nil {
		t.Fatal(err)
	}
	cl.SetPalette(assets.pal)
	cl.SetUIStage(gameShellUIStage{shell: shell})
	image := cl.ComposeFrame()

	glyph := retailGAFGlyph(small, 'v')
	if glyph == nil {
		t.Fatal("hattfont11 v frame missing")
	}
	x := int(r.X) - int(glyph.XOffset)
	y := int(r.Y) - (int(glyph.YOffset) - retailGAFBaselineHeight(small))
	for py := 0; py < int(glyph.Height); py++ {
		for px := 0; px < int(glyph.Width); px++ {
			index := py*int(glyph.Width) + px
			if glyph.Transparent[index] {
				continue
			}
			rr, gg, bb, aa := assets.pal.RGBA(glyph.Pixels[index])
			if got := image.RGBAAt(x+px, y+py); got.R != rr || got.G != gg || got.B != bb || got.A != aa {
				t.Fatalf("v pixel (%d,%d) = %v, want the hattfont11 glyph pixel (%d,%d,%d,%d)", px, py, got, rr, gg, bb, aa)
			}
		}
	}
}
