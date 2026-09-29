package client

import (
	"reflect"
	"testing"

	"github.com/nanolathe-gg/nanolathe/internal/input"
	"github.com/nanolathe-gg/nanolathe/internal/render"
)

// placementCursorFixture installs a placement cursor whose single opaque pixel
// marks the point that lands on the pointer: the centre of cursorfindsite's
// stock 21x23 frame, whose authored offset would put it down-right of the
// pointer [03 R-FX-01 §5], or the authored centre hotspot of cursortoofar's
// stock 29x29 crosshair.
func placementCursorFixture(t *testing.T, idx int) *Client {
	t.Helper()
	c, err := New(Options{Width: 80, Height: 70})
	if err != nil {
		t.Fatal(err)
	}
	cs := snapshotCursor(99)
	cs.idx = idx
	f := cs.Frame()
	f.Width, f.Height = 21, 23
	f.XOffset, f.YOffset = -15, -3
	mark := 11*int(f.Width) + 10
	if idx == render.CursorTooFar {
		f.Width, f.Height = 29, 29
		f.XOffset, f.YOffset = 14, 14
		mark = 14*int(f.Width) + 14
	}
	f.Pixels = make([]byte, int(f.Width)*int(f.Height))
	f.Transparent = make([]bool, len(f.Pixels))
	for i := range f.Transparent {
		f.Transparent[i] = true
	}
	f.Pixels[mark] = 99
	f.Transparent[mark] = false
	c.SetCursors(cs)
	return c
}

// Without a ghost in the recording — the reticle over the minimap, where the
// ghost is neither updated nor drawn [07 §9] — the placement reticle takes
// the fresh presentation pointer like every other cursor, and stays centred
// on it rather than taking the stock GAF offset [03 R-FX-01 §5].
func TestPlacementReticleCenteredAfterLatePosition(t *testing.T) {
	c := placementCursorFixture(t, render.CursorFindSite)
	c.in.Mouse.SetPosition(30, 30)
	c.drawCursor()
	c.PositionPresentationCursor(&c.list, 40, 35)
	c.replayForTest()
	if got := c.indexed[35*c.width+40]; got != 99 {
		t.Fatalf("late positioned reticle centre = %d, want 99", got)
	}
	if got := c.indexed[30*c.width+30]; got != 0 {
		t.Fatalf("reticle remained at old pointer: %d", got)
	}
}

// A cursor recorded with the build ghost keeps the host-step pointer the
// ghost was snapped from, whichever validity shape it has [07 §8][07 §9]:
// moving it to a fresher pointer would carry it ahead of the ghost while the
// mouse sweeps.
func TestPlacementCursorPinnedWithGhostAfterLatePosition(t *testing.T) {
	for _, tc := range []struct {
		name string
		idx  int
	}{{"legal site reticle", render.CursorFindSite}, {"illegal site crosshair", render.CursorTooFar}} {
		t.Run(tc.name, func(t *testing.T) {
			c := placementCursorFixture(t, tc.idx)
			c.in.Mouse.SetPosition(30, 30)
			c.PinCursorToRecord()
			c.drawCursor()
			c.PositionPresentationCursor(&c.list, 40, 35)
			c.replayForTest()
			if got := c.indexed[30*c.width+30]; got != 99 {
				t.Fatalf("cursor centre at the ghost's pointer = %d, want 99", got)
			}
			if got := c.indexed[35*c.width+40]; got != 0 {
				t.Fatalf("cursor followed the late pointer away from the ghost: %d", got)
			}
		})
	}
}

// The pin belongs to one recording: a frame that draws no ghost starts
// unpinned, so the cursor returns to low-latency positioning as soon as
// placement ends.
func TestCursorPinLastsOneRecording(t *testing.T) {
	c := placementCursorFixture(t, render.CursorFindSite)
	c.in.UpdatePointerMotion(input.PointerEvent{X: 30, Y: 30})
	c.in.PublishPointer()
	c.PinCursorToRecord()
	list := c.RecordModernFrame()
	c.PositionPresentationCursor(list, 40, 35)
	list.Replay(classicSink{c: c})
	if got := c.indexed[35*c.width+40]; got != 99 {
		t.Fatalf("a recording with no ghost kept an earlier pin: late pointer pixel = %d, want 99", got)
	}
}

// Host late positioning must affect only the presented pointer, preserve the
// GAF hotspot [07 §8], and never move the pointer used by commands [I6].
func TestPresentationCursorMovesWithoutPublishingInput(t *testing.T) {
	c, err := New(Options{Width: 8, Height: 8})
	if err != nil {
		t.Fatal(err)
	}
	c.SetCursors(snapshotCursor(19))
	c.cursors.Frame().XOffset, c.cursors.Frame().YOffset = 1, 2
	c.in.UpdatePointerMotion(input.PointerEvent{X: 2, Y: 3})
	c.in.PublishPointer()
	before, _ := c.in.PointerSample()
	c.StartPreRecord(0, 0, false)
	c.JoinPreRecord()
	list, hit := c.TakePreRecord(c.PresentationDigest(), 0)
	if !hit {
		t.Fatal("unchanged presentation did not consume its pre-record")
	}
	c.PositionPresentationCursor(list, 6, 6)
	list.Replay(classicSink{c: c})
	if c.indexed[4*8+5] != 19 || c.indexed[1*8+1] == 19 {
		t.Fatal("cursor was not moved from its recorded location with its authored hotspot")
	}
	if after, _ := c.in.PointerSample(); !reflect.DeepEqual(after, before) {
		t.Fatal("presentation positioning changed command input")
	}
}

func TestPresentationCursorCaptureAndRestore(t *testing.T) {
	c, err := New(Options{Width: 8, Height: 8})
	if err != nil {
		t.Fatal(err)
	}
	c.SetCursors(snapshotCursor(19))
	c.in.UpdatePointerMotion(input.PointerEvent{X: 2, Y: 3})
	c.in.PublishPointer()
	c.SetPointerCaptured(true)
	list := c.RecordModernFrame()
	c.PositionPresentationCursor(list, 6, 6)
	list.Replay(classicSink{c: c})
	for _, pixel := range c.indexed {
		if pixel == 19 {
			t.Fatal("captured pointer was drawn")
		}
	}
	c.SetPointerCaptured(false)
	list = c.RecordModernFrame()
	c.PositionPresentationCursor(list, 6, 6)
	list.Replay(classicSink{c: c})
	if c.indexed[3*8+2] != 19 || c.indexed[6*8+6] == 19 {
		t.Fatal("late positioning replaced the capture-release restore position")
	}
}
