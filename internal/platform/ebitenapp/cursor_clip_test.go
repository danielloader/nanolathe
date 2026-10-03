package ebitenapp

import (
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/nanolathe-gg/nanolathe/internal/client"
)

type cursorClipScreen struct{ active bool }

func (s *cursorClipScreen) Active() bool     { return s.active }
func (*cursorClipScreen) Update()            {}
func (*cursorClipScreen) Draw(*ebiten.Image) {}

// The host screen fills the display, so the menu's 4:3 confinement must not
// keep the pointer away from its controls (DESIGN_PRESENTATION_CLIENT §2.1).
func TestCursorClipFollowsPresentedScreenLayout(t *testing.T) {
	for _, tc := range []struct {
		name               string
		outsideW, outsideH int
		scale              float64
	}{
		{"wide", 1920, 1080, 1},
		{"ultrawide", 3440, 1440, 1},
		{"scaled", 1536, 864, 1.25},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, err := client.New(client.Options{Width: 640, Height: 480})
			if err != nil {
				t.Fatal(err)
			}
			screen := &cursorClipScreen{}
			a := app{c: c, options: RunOptions{Screen: screen}, screenScale: screenScale{
				monitorScaleF: func() float64 { return tc.scale },
			}}
			cw, ch := int(float64(tc.outsideW)*tc.scale), int(float64(tc.outsideH)*tc.scale)
			check := func(fullWindow bool) {
				t.Helper()
				w, h := a.Layout(tc.outsideW, tc.outsideH)
				clipW, clipH := a.cursorClipSize()
				if clipW != w || clipH != h {
					t.Fatalf("clip uses %dx%d, presented canvas is %dx%d", clipW, clipH, w, h)
				}
				left, top, right, bottom, ok := presentedCursorRect(cw, ch, clipW, clipH)
				if !ok {
					t.Fatal("no confinement rectangle")
				}
				if fullWindow {
					if left != 0 || top != 0 || right != int32(cw) || bottom != int32(ch) {
						t.Fatalf("host screen clipped to [%d,%d)x[%d,%d)", left, right, top, bottom)
					}
				} else if left <= 0 || right >= int32(cw) {
					t.Fatal("4:3 client canvas did not restore its pillarbox confinement")
				}
			}
			check(false)
			screen.active = true
			check(true)
			screen.active = false
			check(false)
			c.Resize(800, 600)
			check(false)
			screen.active = true
			check(true)
		})
	}
}

// The confinement rectangle must admit exactly the device pixels Ebitengine
// reports as logical 0..w-1 (truncating toward zero), so both exact camera
// edges stay reachable and no pixel reports a coordinate off the canvas.
func TestPresentedCursorRectAdmitsExactlyTheCanvas(t *testing.T) {
	for _, tc := range []struct{ cw, ch, w, h int }{
		{3440, 1440, 1024, 768}, // the reported ultrawide pillarbox
		{3440, 1440, 640, 480},
		{1920, 1080, 800, 600},
		{1280, 1024, 1024, 768}, // letterbox top and bottom
		{1024, 768, 1024, 768},  // exact fit
		{2560, 1600, 1600, 1200},
	} {
		left, top, right, bottom, ok := presentedCursorRect(tc.cw, tc.ch, tc.w, tc.h)
		if !ok {
			t.Fatalf("%v: no rectangle", tc)
		}
		s := min(float64(tc.cw)/float64(tc.w), float64(tc.ch)/float64(tc.h))
		ox := (float64(tc.cw) - float64(tc.w)*s) / 2
		oy := (float64(tc.ch) - float64(tc.h)*s) / 2
		logical := func(p int32, o float64) int { return int((float64(p) - o) / s) }
		inside := func(p int32, o float64, n int) bool {
			v := (float64(p) - o) / s
			return v > -1 && int(v) < n
		}
		if logical(left, ox) != 0 || logical(right-1, ox) != tc.w-1 ||
			logical(top, oy) != 0 || logical(bottom-1, oy) != tc.h-1 {
			t.Fatalf("%v: rect [%d,%d)x[%d,%d) misses a canvas edge", tc, left, right, top, bottom)
		}
		if left > 0 && inside(left-1, ox, tc.w) || right < int32(tc.cw) && inside(right, ox, tc.w) ||
			top > 0 && inside(top-1, oy, tc.h) || bottom < int32(tc.ch) && inside(bottom, oy, tc.h) {
			t.Fatalf("%v: rect [%d,%d)x[%d,%d) excludes a canvas pixel", tc, left, right, top, bottom)
		}
		if left > 0 && !inside(left, ox, tc.w) || right < int32(tc.cw) && inside(right, ox, tc.w) {
			t.Fatalf("%v: rect admits a bar pixel", tc)
		}
	}
	if _, _, _, _, ok := presentedCursorRect(0, 1440, 1024, 768); ok {
		t.Fatal("empty client produced a rectangle")
	}
}
