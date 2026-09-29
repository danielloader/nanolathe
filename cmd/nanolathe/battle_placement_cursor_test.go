package main

import (
	"testing"

	"github.com/nanolathe-gg/nanolathe/internal/client"
	"github.com/nanolathe-gg/nanolathe/internal/drawlist"
	"github.com/nanolathe-gg/nanolathe/internal/render"
)

// placementCursorCollector keeps the cursor command of one replayed list.
type placementCursorCollector struct {
	cursors []drawlist.Cursor
}

func (s *placementCursorCollector) Clear()                   {}
func (s *placementCursorCollector) Terrain(drawlist.Terrain) {}
func (s *placementCursorCollector) Sprite(drawlist.Sprite)   {}
func (s *placementCursorCollector) Glyphs(drawlist.Glyphs)   {}
func (s *placementCursorCollector) Fill(drawlist.Fill)       {}
func (s *placementCursorCollector) Line(drawlist.Line)       {}
func (s *placementCursorCollector) Points(drawlist.Points)   {}
func (s *placementCursorCollector) Flash(drawlist.Flash)     {}
func (s *placementCursorCollector) Halo(drawlist.Halo)       {}
func (s *placementCursorCollector) Model(drawlist.Model)     {}
func (s *placementCursorCollector) Fog(drawlist.Fog)         {}
func (s *placementCursorCollector) Surface(drawlist.Surface) {}
func (s *placementCursorCollector) Cursor(c drawlist.Cursor) { s.cursors = append(s.cursors, c) }
func (s *placementCursorCollector) Expand()                  {}

// A Modern sweep across the map with a building armed. Each host step snaps
// the ghost from its pointer [07 §9]; the presented frame then receives a
// fresher pointer 40 pixels further on, as a fast mouse delivers between host
// steps. The cursor recorded with the ghost — the stock `cursorfindsite`
// reticle on a legal site, the stock `cursortoofar` crosshair on an illegal
// one [07 §8] — must stay on the ghost's pointer, inside the footprint and no
// more than half a cell from its centre on this flat fixture (the retail
// round-to-nearest snap). Without the ghost the same cursor takes the fresh
// pointer again. Presentation only: no order, RNG or simulation input changes.
func TestPlacementCursorStaysOnGhostDuringModernSweep(t *testing.T) {
	cs, err := openContent(Options{Root: probeRetail(t)})
	if err != nil {
		t.Fatal(err)
	}
	defer cs.Close()
	cursors, err := client.LoadCursors(cs.fs)
	if err != nil {
		t.Fatal(err)
	}
	b, s, _ := placeClickFixture(t, 64, 64)
	cl, err := client.New(client.Options{Buffer: s.Snapshot, Width: 640, Height: 480})
	if err != nil {
		t.Fatal(err)
	}
	cl.SetCamera(b.cam)
	cl.SetEnhanced(true)
	cl.SetCursors(cursors)
	cl.SetUIStage(battleHUDUIStage{hud: &retailBattleHUD{}, battle: b})
	b.armPlacement(b.cat.Units["armsolar"])
	state := &b.battleState().Input

	const lead = 40
	present := func(x, y int32, illegal bool) (drawlist.Cursor, int, int) {
		t.Helper()
		cl.Input().Mouse.SetPosition(float32(x), float32(y))
		state.PointerX, state.PointerY = x, y
		b.updatePlacement(x, y)
		if b.battleState().PlacementArmed() && !state.BuildOK {
			// The open, fully visible fixture has no illegal site of its own.
			t.Fatalf("pointer %d,%d: fixture site is illegal", x, y)
		}
		if illegal {
			// Stand in for any rejection; only the cursor's shape and pin
			// are under test here, not the placement predicate.
			state.BuildOK = false
		}
		b.updateCursor(cl)
		list := cl.RecordModernFrame()
		cl.PositionPresentationCursor(list, int(x)+lead, int(y))
		var got placementCursorCollector
		list.Replay(&got)
		if len(got.cursors) != 1 || got.cursors[0].Frame == nil {
			t.Fatalf("pointer %d,%d recorded %d cursors", x, y, len(got.cursors))
		}
		c := got.cursors[0]
		// The pixel that marks the pointer: the reticle's centre, which
		// Nanolathe centres on the pointer, or the crosshair's authored hotspot.
		px, py := int(c.HotX)+(int(c.Frame.Width)-1)/2, int(c.HotY)+(int(c.Frame.Height)-1)/2
		if !c.CenterOnPointer {
			px, py = int(c.HotX)+int(c.Frame.XOffset), int(c.HotY)+int(c.Frame.YOffset)
		}
		return c, px, py
	}

	for i, x := range []int32{250, 262, 274, 286, 298, 310, 322, 334} {
		illegal := i%2 == 1
		want := render.CursorFindSite
		if illegal {
			want = render.CursorTooFar
		}
		c, px, py := present(x, 300, illegal)
		if got := cl.Cursors().Index(); got != want {
			t.Fatalf("pointer %d,300 installed cursor %d, want %d", x, got, want)
		}
		if !c.Pinned || int32(px) != x || py != 300 {
			t.Fatalf("pointer %d,300: cursor pinned=%v at %d,%d, want pinned at the ghost's pointer", x, c.Pinned, px, py)
		}
		l, top, r, btm := b.placementRect()
		if int32(px) < l || int32(px) >= r || int32(py) < top || int32(py) >= btm {
			t.Fatalf("pointer %d,300: cursor %d,%d outside ghost %d,%d..%d,%d", x, px, py, l, top, r, btm)
		}
		if cx, cy := (l+r)/2, (top+btm)/2; abs32(int32(px)-cx) > 8 || abs32(int32(py)-cy) > 8 {
			t.Fatalf("pointer %d,300: cursor %d,%d more than half a cell from ghost centre %d,%d", x, px, py, cx, cy)
		}
		if fresh := x + lead; fresh >= l && fresh < r {
			t.Fatalf("pointer %d,300: the fresh pointer %d still lies on the ghost; the sweep proves nothing", x, fresh)
		}
	}

	// Placement ends: nothing pins the next recording, and the idle pointer
	// takes the fresh position again.
	b.disarmPlacement()
	c, px, _ := present(334, 300, false)
	if c.Pinned || px != 334+lead {
		t.Fatalf("after placement the cursor stayed pinned=%v at x %d, want the fresh pointer %d", c.Pinned, px, 334+lead)
	}
}

func abs32(v int32) int32 {
	if v < 0 {
		return -v
	}
	return v
}
