package main

import (
	"image/color"
	"testing"
	"time"

	"github.com/nanolathe-gg/nanolathe/formats"
	"github.com/nanolathe-gg/nanolathe/internal/client"
	"github.com/nanolathe-gg/nanolathe/internal/palette"
	"github.com/nanolathe-gg/nanolathe/internal/platform/screenkit"
	"github.com/nanolathe-gg/nanolathe/internal/render"
	"github.com/nanolathe-gg/nanolathe/internal/testsupport"
	"github.com/nanolathe-gg/nanolathe/vfs"
)

func TestNLPointerScaledAuthoredHotspot(t *testing.T) {
	f := &formats.GAFFrame{Width: 20, Height: 30, XOffset: 7, YOffset: -3}
	r := nlPointerRect(f, 100, 200, 2)
	if r != (screenkit.Rect{X: 86, Y: 206, W: 40, H: 60}) {
		t.Fatalf("scaled hotspot rectangle = %+v", r)
	}
}

func TestNLArtIndexedTransparency(t *testing.T) {
	f := &formats.GAFFrame{Width: 2, Height: 1, Pixels: []byte{0, 9}, Transparent: []bool{false, true}}
	p := &palette.Tables{}
	p.Base[0] = [4]byte{12, 34, 56, 0}
	rgba := nlGAFImage(f, p)
	if got := rgba.RGBAAt(0, 0); got != (color.RGBA{12, 34, 56, 255}) {
		t.Fatalf("physical palette pixel = %v", got)
	}
	if got := rgba.RGBAAt(1, 0); got.A != 0 {
		t.Fatalf("transparent pixel = %v", got)
	}
	if nlGAFImage(f, nil) != nil {
		t.Fatal("missing palette produced artwork")
	}
}

func TestNLMissingPointerLeavesNativePointer(t *testing.T) {
	s := &nlScreen{open: true}
	if s.OwnsPointer() {
		t.Fatal("absent cursor hid the native pointer")
	}
	if s.pointerShape() != render.CursorNormal {
		t.Fatal("idle screen requested a loading cursor")
	}
	s.reload = &nlReload{}
	if s.pointerShape() != render.CursorHourglass {
		t.Fatal("content loading did not request hourglass")
	}
	s.reload = nil
	s.preview = &nlPreview{loading: true}
	if s.pointerShape() != render.CursorHourglass {
		t.Fatal("scene loading did not request hourglass")
	}
	s.closing = true
	if s.pointerShape() != render.CursorNormal {
		t.Fatal("closing-release wait retained hourglass")
	}
	var a *nlArt
	if a.buttonFrame(screenkit.Rect{W: 80, H: 20}, false, false) != nil {
		t.Fatal("absent button art produced a frame")
	}
}

func TestNLRetailButtonAndIndependentCursor(t *testing.T) {
	fs := vfs.New()
	if err := fs.MountGameDirectories([]string{testsupport.RetailRoot(t)}); err != nil {
		t.Fatal(err)
	}
	defer fs.Close()
	g, err := formats.LoadGAFFile(fs, "anims/commongui.gaf")
	if err != nil {
		t.Fatal(err)
	}
	entry, ok := g.Find("BUTTONS0")
	if !ok {
		t.Fatal("stock button bank is missing")
	}
	a := &nlArt{buttons: entry, buttonPal: &palette.Tables{}}
	up := a.buttonFrame(screenkit.Rect{W: 160, H: 40}, false, false)
	down := a.buttonFrame(screenkit.Rect{W: 160, H: 40}, true, false)
	if up == nil || down == nil || up == down || up.Width != 80 || up.Height != 20 || down.Width != up.Width || down.Height != up.Height {
		t.Fatal("scaled stock family did not preserve its up/down geometry")
	}
	underlying, err := client.LoadCursors(fs)
	if err != nil {
		t.Fatal(err)
	}
	oldShell := &gameShell{cs: &contentSet{fs: fs}, assets: &menuAssets{pal: &palette.Tables{}}}
	s := &nlScreen{open: true, preview: &nlPreview{loading: true}}
	s.bindPointer(oldShell)
	if !s.OwnsPointer() || s.pointer == underlying {
		t.Fatal("screen did not install independent cursor playback")
	}
	start := time.Unix(100, 0)
	s.stepPointer(start)
	first := s.pointer.Frame()
	s.stepPointer(start.Add(time.Second))
	if s.pointer.Index() != render.CursorHourglass || s.pointer.Frame() == first || underlying.Index() != render.CursorNormal {
		t.Fatal("wall-clock playback did not advance independently")
	}
	small := a.buttonFrame(screenkit.Rect{W: 30, H: 20}, false, false)
	if small.Width <= small.Height {
		t.Fatal("narrow key cap used checkbox artwork")
	}
	old := s.pointer
	s.bindPointer(&gameShell{cs: &contentSet{fs: fs}, assets: oldShell.assets})
	if s.pointer == old || s.pointer.Index() != render.CursorNormal {
		t.Fatal("content reload retained old cursor playback")
	}
	s.bindPointer(&gameShell{cs: &contentSet{fs: vfs.New()}, assets: oldShell.assets})
	if s.OwnsPointer() {
		t.Fatal("content without cursors retained software pointer ownership")
	}
	s.open = false
	if s.OwnsPointer() {
		t.Fatal("closed settings screen retained pointer ownership")
	}
}

func TestNLSnapshotPreservesGlowOff(t *testing.T) {
	s := &nlScreen{}
	g := &gameShell{cs: &contentSet{}}
	g.display.GlowStrength = 0
	if d := s.snapshot(g); d.glowStrength != 0 {
		t.Fatalf("explicit glow-off strength became %d", d.glowStrength)
	}
}
