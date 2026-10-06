package main

import (
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nanolathe-gg/nanolathe/formats"
	"github.com/nanolathe-gg/nanolathe/internal/client"
	"github.com/nanolathe-gg/nanolathe/internal/frame"
	"github.com/nanolathe-gg/nanolathe/internal/gui"
	"github.com/nanolathe-gg/nanolathe/internal/ui"
	"github.com/nanolathe-gg/nanolathe/vfs"
)

func labelAuditFS(t *testing.T, name, data string) *vfs.FS {
	t.Helper()
	root := t.TempDir()
	path := filepath.Join(root, name)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(data), 0644); err != nil {
		t.Fatal(err)
	}
	fs := vfs.New()
	if err := fs.MountDirectory(root, 1); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = fs.Close() })
	return fs
}

func labelAuditFont() *formats.GAF {
	font := formats.GAFEntry{Frames: make([]formats.GAFFrameRef, 256)}
	for code := 32; code < 127; code++ {
		f := &formats.GAFFrame{Width: 10, Height: 7, YOffset: 7, Pixels: make([]byte, 70), Transparent: make([]bool, 70)}
		for i := range f.Pixels {
			x, y := i%10, i/10
			f.Pixels[i] = 230
			f.Transparent[i] = !(x == 1 || x == 7 || y == 0 || y == 3) || code == 32
		}
		font.Frames[code].Frame = f
	}
	return &formats.GAF{Entries: []formats.GAFEntry{font}}
}

// Actual open/fill precedes drawing: the authored caption, including empty,
// determines the retained X, not the selected map name [07 R-WGT-01 §7].
func TestRetailLabelSkirmishInitialPaintLifetime(t *testing.T) {
	for _, initial := range []string{"AA", ""} {
		t.Run(fmt.Sprintf("initial-length-%d", len(initial)), func(t *testing.T) {
			fs := labelAuditFS(t, "guis/skirmish.gui", fmt.Sprintf(`
[GADGET0] { [COMMON] { id=0; name=ROOT; xpos=40; ypos=20; width=200; height=100; active=1; } }
[GADGET1] { [COMMON] { id=5; name=MapName; xpos=-1; ypos=10; width=100; height=12; active=1; } text=%s; }
[GADGET2] { [COMMON] { id=5; name=HELPTEXT; xpos=-1; ypos=35; width=100; height=12; active=1; } text=AA; }
[GADGET3] { [COMMON] { id=1; name=StartLocation; xpos=5; ypos=70; width=15; height=12; active=1; } }
`, initial))
			w, err := gui.LoadWithTranslation(fs, "guis/skirmish.gui", nil)
			if err != nil {
				t.Fatal(err)
			}
			g := &gameShell{cs: testContentSet(fs), assets: &menuAssets{panel: map[shellMode]*retailPanelAssets{modeMenuSkirmish: {window: w}}, gafFontSmall: labelAuditFont()}}
			g.setup.MapName = "AAAAAA"
			g.openMenu(modeMenuSkirmish)
			p := g.activePanel()
			i := p.Index("MapName")
			want := int32(90)
			if initial == "" {
				want = 100
			}
			if p.TextAt(i) != "AAAAAA" || p.Window.Gadgets[i].Rect.X != want {
				t.Fatalf("filled label text=%q x=%d, want selected caption at %d", p.TextAt(i), p.Window.Gadgets[i].Rect.X, want)
			}
			if w.Gadgets[i].Rect.X != -1 {
				t.Fatal("opening changed cached authored window")
			}
			c, err := client.New(client.Options{Buffer: &frame.Buffer{}, Width: 280, Height: 160})
			if err != nil {
				t.Fatal(err)
			}
			c.Input().Mouse.SetPosition(46, 91)
			g.menuInput(c)
			help := p.Index("HELPTEXT")
			if p.TextAt(help) == "AA" || p.TextAt(help) == "" {
				t.Fatalf("production hover did not replace help: %q", p.TextAt(help))
			}
			c.SetUIStage(painterBindingStage(func(c *client.Client) {
				c.UIFillRect(0, 0, 280, 160, 14)
				for _, idx := range []int{i, help} {
					g.drawRetailTextState(c, p, idx, p.Window.Gadgets[idx], p.Window.PlacedRect(idx))
				}
			}))
			snap := c.ComposeFrameSnapshot()
			if p.Window.Gadgets[i].Rect.X != want || p.Window.Gadgets[help].Rect.X != 90 {
				t.Fatal("caption repaint moved resolved label")
			}
			x, y := 40+int(want), 30
			if snap.Indexed[y*snap.Width+x] != 230 || snap.Indexed[y*snap.Width+x-1] != 14 {
				t.Fatal("paint did not use retained X plus panel origin")
			}
			if dir := os.Getenv("NANOLATHE_LABEL_CAPTURE"); dir != "" {
				if err := os.MkdirAll(dir, 0755); err != nil {
					t.Fatal(err)
				}
				img := image.NewGray(image.Rect(0, 0, snap.Width*2, snap.Height*2))
				for y := 0; y < img.Rect.Dy(); y++ {
					for x := 0; x < img.Rect.Dx(); x++ {
						img.Pix[y*img.Stride+x] = snap.Indexed[(y/2)*snap.Width+x/2]
					}
				}
				f, err := os.Create(filepath.Join(dir, fmt.Sprintf("initial-%d.png", len(initial))))
				if err != nil {
					t.Fatal(err)
				}
				err = png.Encode(f, img)
				closeErr := f.Close()
				if err != nil {
					t.Fatal(err)
				}
				if closeErr != nil {
					t.Fatal(closeErr)
				}
			}
		})
	}
}

func TestRetailLabelAppendPlacementPrecedesCentering(t *testing.T) {
	for _, dx := range []int{0, 31} {
		t.Run(fmt.Sprint(dx), func(t *testing.T) {
			g, _, _ := syntheticOptionsPage(t, "sound", func(w *gui.Window) error { w.Gadgets = w.Gadgets[:1]; w.Rect.W = 200; return nil })
			fs := labelAuditFS(t, "guis/soundsrt.gui", fmt.Sprintf(`[GADGET0]{[COMMON]{id=0;name=PAGE;xpos=%d;width=100;height=100;}}
[GADGET1]{[COMMON]{id=5;name=APPENDED;xpos=-1;ypos=4;width=100;height=12;active=1;}text=AA;}`, dx))
			g.cs = testContentSet(fs)
			g.assets = &menuAssets{gafFontSmall: labelAuditFont()}
			optionsState.inBattle = true
			g.openRetailOptionsPage("sound")
			i := optionsPanel.Index("APPENDED")
			if i < 0 {
				t.Fatal("page did not append")
			}
			want := int32(90)
			if dx != 0 {
				want = int32(dx - 1)
			}
			if got := optionsPanel.Window.Gadgets[i].Rect.X; got != want {
				t.Fatalf("translated label x=%d, want %d", got, want)
			}
			optionsPanel.SetTextAt(i, "AAAAAA")
			g.resolveRetailLabel(optionsPanel.Window, i, optionsPanel.TextAt(i))
			if got := optionsPanel.Window.Gadgets[i].Rect.X; got != want {
				t.Fatalf("changed caption re-centered translated label: %d", got)
			}
		})
	}
}

func TestRetailLabelSignedStoreAndPersistentSentinel(t *testing.T) {
	w := &gui.Window{Rect: gui.Rect{W: 200}, Gadgets: []gui.Gadget{{}, {Kind: gui.KindLabel, Rect: gui.Rect{X: -1}}}}
	resolveLabelX(w, 1, 202)
	if w.Gadgets[1].Rect.X != -1 {
		t.Fatal("negative half did not remain sentinel")
	}
	resolveLabelX(w, 1, 20)
	if w.Gadgets[1].Rect.X != 90 {
		t.Fatal("sentinel result stopped being eligible")
	}
	w.Gadgets[1].Rect.X = -1
	resolveLabelX(w, 1, 200+65538)
	if w.Gadgets[1].Rect.X != 32767 {
		t.Fatal("label position did not narrow to signed 16 bits")
	}
}

func TestBattleLabelInitialPaintAndEmptyRepaint(t *testing.T) {
	for _, initial := range []string{"AA", ""} {
		w := &gui.Window{Rect: gui.Rect{X: 40, W: 200, H: 50}, OriginX: 40, Gadgets: []gui.Gadget{{Kind: gui.KindPanel}, {Kind: gui.KindLabel, Name: "TITLE", Active: 1, Text: initial, Rect: gui.Rect{X: -1, RawX: -1, Y: 5, W: 100, H: 12}}}}
		bank := labelAuditFont()
		h := &retailBattleHUD{modalFontSmall: &bank.Entries[0]}
		h.installWindow(w, nil)
		want := int32(90)
		if initial == "" {
			want = 100
		}
		p := ui.NewPanel(w)
		c, err := client.New(client.Options{Buffer: &frame.Buffer{}, Width: 280, Height: 80})
		if err != nil {
			t.Fatal(err)
		}
		c.SetUIStage(painterBindingStage(func(c *client.Client) { h.drawGUIWindowState(c, w, nil, "AAAAAA", p, nil) }))
		c.ComposeFrameSnapshot()
		if w.Gadgets[1].Rect.X != want {
			t.Fatalf("battle dynamic title x=%d want %d", w.Gadgets[1].Rect.X, want)
		}
		// A label hidden during construction first resolves even when its next
		// paint has no glyphs. The later nonempty caption keeps that position.
		w.Gadgets[1].Rect.X = -1
		w.Gadgets[1].Text = ""
		c.SetUIStage(painterBindingStage(func(c *client.Client) { h.drawGUIWindowState(c, w, nil, "", p, nil) }))
		c.ComposeFrameSnapshot()
		if w.Gadgets[1].Rect.X != 100 {
			t.Fatal("battle empty paint skipped position write")
		}
	}
}

func TestRetailLabelMessageResizeRetainsInitialX(t *testing.T) {
	bank := labelAuditFont()
	g := &gameShell{assets: &menuAssets{gafFontSmall: bank, gafFont: bank}}
	w := &gui.Window{Rect: gui.Rect{W: 200, H: 50}, Gadgets: []gui.Gadget{
		{Kind: gui.KindPanel},
		{Kind: gui.KindButton, Name: "OK", Rect: gui.Rect{W: 20, H: 12}},
		{Kind: gui.KindLabel, Name: "TITLE", Active: 1, Text: "AA", Attribs: 4, Rect: gui.Rect{X: -1, W: 100, H: 12}},
		{Kind: gui.KindLabel, Name: "LINKED", Link: "OK", Active: 0, Text: "AA", Attribs: 0x14, Rect: gui.Rect{X: 7, W: 30, H: 12}},
	}}
	built := g.buildRetailMessageWindow(w, strings.Repeat("A", 6))
	if built.Rect.W == w.Rect.W || built.Gadgets[2].Rect.X != 90 {
		t.Fatalf("resized message width=%d label x=%d", built.Rect.W, built.Gadgets[2].Rect.X)
	}
	// Widening covers authored and appended labels, even hidden ones. It
	// replaces prior alignment/inert bits; the final build restores inert
	// only for an empty link [07 R-FE-01 §9][07 R-WGT-01 §7].
	for _, gad := range built.Gadgets {
		if gad.Kind != gui.KindLabel {
			continue
		}
		want := uint32(2)
		if gad.Link == "" {
			want |= gui.AttribInert
		}
		if gad.Rect.W != built.Rect.W || gad.Attribs != want {
			t.Fatalf("label %q width=%d attributes=%d", gad.Name, gad.Rect.W, gad.Attribs)
		}
	}
	if built.Gadgets[3].Rect.X != 7 || w.Gadgets[2].Rect.W != 100 || w.Gadgets[2].Attribs != 4 {
		t.Fatal("widening changed retained X or cached authored records")
	}
}
