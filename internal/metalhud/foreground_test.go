package metalhud

import (
	"fmt"
	"testing"

	"github.com/nanolathe-gg/nanolathe/formats"
	"github.com/nanolathe-gg/nanolathe/internal/camera"
	"github.com/nanolathe-gg/nanolathe/internal/client"
	"github.com/nanolathe-gg/nanolathe/internal/drawlist"
)

type chromeTextStage struct {
	region client.ChromeRegion
	font   *formats.FNT
	art    *formats.GAFFrame
}

func (s chromeTextStage) DrawUI(c *client.Client, _ client.UIFrame) {
	c.BeginChromeRegion(s.region)
	w, h := c.ChromeSize()
	c.UIBlit(s.art, 5, 7)
	c.UIText(s.font, "AA", 5, 8, 1)
	c.UIText(s.font, "A", 3, h-3, 1)
	c.UITextWidthClipped(s.font, "A", 3, 3, 0, 1, 2, 2, 5, 4)
	// One-past edges equal to the virtual/private bound are not admitted
	// [03 R-FONT-01 §3]. Rejection must leave the following art transformed.
	c.UIText(s.font, "A", w-3, 8, 1)
	c.UIText(s.font, "A", 3, h-2, 1)
	c.UITextWidthClipped(s.font, "A", 4, 3, 0, 1, 2, 2, 5, 4)
	c.UIFillRect(4, 5, 3, 2, 1)
	c.UIFillRect(w-1, h-1, 4, 4, 1)
	c.EndChromeRegion()
	c.UIText(s.font, "A", 5, 8, 1)
}

func foregroundTestFont() *formats.FNT {
	font := &formats.FNT{Height: 2, Baseline: 1}
	font.Glyphs['A'] = &formats.FNTGlyph{Width: 3, Height: 2, Bits: []byte{0xfc}}
	return font
}

// The native host replays RecordRetainedForeground. Chrome text uses its
// virtual surface for admission and scales the whole glyph, baseline and
// advance with adjacent sprites (DESIGN_INTERFACE_HUD_INPUT §3.3), then closes cleanly.
func TestProductionForegroundChromeText(t *testing.T) {
	for _, tc := range []struct {
		w, h int
		r    client.ChromeRegion
	}{
		{256, 144, client.ChromeRegion{Scale: 1}},
		{256, 144, client.ChromeRegion{Scale: 1, OffsetX: 129}},
		{256, 144, client.ChromeRegion{Scale: 2}},
		{257, 145, client.ChromeRegion{Scale: 2, OffsetX: 1, OffsetY: 1}},
		{257, 145, client.ChromeRegion{Scale: 3}},
	} {
		t.Run(fmt.Sprint(tc), func(t *testing.T) {
			c, err := client.New(client.Options{Width: tc.w, Height: tc.h})
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(c.Close)
			font := foregroundTestFont()
			c.SetFNT(font)
			c.SetEnhanced(true)
			c.SetUIStage(chromeTextStage{tc.r, font, &formats.GAFFrame{Width: 3, Height: 2, Pixels: []byte{1, 1, 1, 1, 1, 1}}})
			list := c.RecordRetainedForeground()
			f := New(nil, tc.w, tc.h)
			quads, _, _, err := f.Prepare(list)
			if err != nil {
				t.Fatal(err)
			}
			w, h := tc.r.VirtualSize(tc.w, tc.h)
			k := float32(tc.r.Scale)
			tx := func(x, y, width, height int) [4]float32 {
				return [4]float32{float32(x)*k + float32(tc.r.OffsetX), float32(y)*k + float32(tc.r.OffsetY), float32(width) * k, float32(height) * k}
			}
			want := [][4]float32{
				tx(5, 7, 3, 2), tx(5, 7, 3, 2), tx(8, 7, 3, 2),
				tx(3, h-4, 3, 2), tx(3, 2, 3, 2), tx(4, 5, 3, 2), tx(w-1, h-1, 1, 1),
				{5, 7, 3, 2},
			}
			// A ceil-rounded width can extend one virtual pixel past the physical
			// right edge. Native storage clipping must preserve its visible span.
			want[6][2] = min(want[6][2], float32(tc.w)-want[6][0])
			if len(quads) != len(want) {
				t.Fatalf("foreground emitted %d quads, want %d: %+v", len(quads), len(want), quads)
			}
			for i := range want {
				if quads[i].Rect != want[i] {
					t.Errorf("quad %d = %v, want %v", i, quads[i].Rect, want[i])
				}
			}
		})
	}
}

// World labels follow their projected anchor but keep native glyph metrics,
// screen offsets and the transformed private clip (GPU design §16.3). A
// rejected label must restore the next world's sprite transform as well.
func TestWorldTextKeepsNativeGlyphs(t *testing.T) {
	font := foregroundTestFont()
	var list drawlist.List
	list.RecordWorld(drawlist.WorldSpace{Begin: true, Factor: .75, Step: camera.ViewScaleNative, OffsetX: -.375, OffsetY: .625, RecordW: 86, RecordH: 86})
	list.RecordGlyphs(drawlist.Glyphs{Font: font, Text: "AA", X: 32, Y: 32, ScreenOffsetX: -3, ScreenOffsetY: -8,
		HasClip: true, Clip: drawlist.Rect{X: 16, Y: 16, W: 40, H: 40}})
	list.RecordGlyphs(drawlist.Glyphs{Font: font, Text: "AA", X: 80, Y: 32})
	list.RecordFill(drawlist.Fill{Rect: drawlist.Rect{X: 32, Y: 32, W: 8, H: 8}, Style: drawlist.FillSolid})
	list.RecordWorld(drawlist.WorldSpace{})
	list.RecordGlyphs(drawlist.Glyphs{Font: font, Text: "A", X: 5, Y: 8})
	f := New(nil, 64, 64)
	quads, _, _, err := f.Prepare(&list)
	if err != nil {
		t.Fatal(err)
	}
	want := [][4]float32{{21, 16, 3, 2}, {24, 16, 3, 2}, {23.625, 24.625, 6, 6}, {5, 7, 3, 2}}
	if len(quads) != len(want) {
		t.Fatalf("world text emitted %d quads, want %d", len(quads), len(want))
	}
	for i := range want {
		if quads[i].Rect != want[i] {
			t.Errorf("quad %d = %v, want %v", i, quads[i].Rect, want[i])
		}
	}
}

var _ client.UIStage = chromeTextStage{}
