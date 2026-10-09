package client

import (
	"testing"

	"github.com/nanolathe-gg/nanolathe/formats"
	"github.com/nanolathe-gg/nanolathe/internal/audio"
	"github.com/nanolathe-gg/nanolathe/internal/camera"
	"github.com/nanolathe-gg/nanolathe/internal/frame"
)

func TestCommittedStatusEventReachesMessageLines(t *testing.T) {
	b := &frame.Buffer{}
	f := b.BeginWrite()
	f.Events = append(f.Events, frame.EventView{
		Kind: frame.EventKindStatus, Tick: 7, Source: 3,
		StatusKind: 7, StatusText: "Can't build", StatusClass: 1,
	})
	if err := b.Publish(7); err != nil {
		t.Fatal(err)
	}
	a := audio.NewService(nil)
	a.Queue.Register(3, nil, "ARMADA", true)
	c := &Client{buffer: b, messages: *frame.NewMessageRing(), screenChat: 0}
	c.SetAudioService(a)
	c.TickAudio()
	lines := c.MessageLines()
	if len(lines) != 1 || lines[0].Text != "ARMADA: Can't build" || lines[0].SourceUnit != 3 {
		t.Fatalf("message lines = %#v", lines)
	}
	c.TickAudio() // drawing the same committed frame must not duplicate it
	if got := len(c.MessageLines()); got != 1 {
		t.Fatalf("duplicate committed event produced %d lines", got)
	}
}

// The online overlay reserves the physical message column, including the
// battle's active scaled rail and top strip.
func TestMessageColumnBottomMatchesDrawnChromeGeometry(t *testing.T) {
	for _, chrome := range []camera.ChromeInsets{{}, {Left: 257}, {Left: 257, Top: 64}} {
		c := &Client{cam: &camera.Camera{Chrome: chrome}, messageFNT: &formats.FNT{Height: 12}, messages: *frame.NewMessageRing()}
		if c.MessageColumnBottom() != 0 {
			t.Fatal("empty message column reserved overlay space")
		}
		c.messages.Append("first", 1, 0, 10, 0)
		c.messages.Append("second", 1, 0, 10, 0)
		c.drawMessageLines()
		var recorded messageGlyphCollector
		c.list.Replay(&recorded)
		left, top, _ := c.cam.ChromeInset()
		if len(recorded.runs) != 2 || recorded.runs[0].X != left+10 || recorded.runs[0].Y != top+20 {
			t.Fatalf("chrome %+v: drawn message column = %+v", chrome, recorded.runs)
		}
		wantBottom := int(recorded.runs[1].Y) + int(c.messageFNT.Height)
		if got := c.MessageColumnBottom(); got != wantBottom {
			t.Fatalf("chrome %+v: overlay boundary %d, drawn column bottom %d", chrome, got, wantBottom)
		}
	}
}
