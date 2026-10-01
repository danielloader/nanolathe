package client

import (
	"fmt"
	"testing"

	"github.com/nanolathe-gg/nanolathe/internal/audio"
	"github.com/nanolathe-gg/nanolathe/internal/camera"
	"github.com/nanolathe-gg/nanolathe/internal/frame"
	"github.com/nanolathe-gg/nanolathe/internal/pool"
	"github.com/nanolathe-gg/nanolathe/internal/sim/numeric"
	"github.com/nanolathe-gg/nanolathe/internal/sim/rng"
	"github.com/nanolathe-gg/nanolathe/internal/world"
)

func noticeUnit(x, z int64) frame.UnitView {
	return frame.UnitView{
		Slot: 1, Model: "fixture", Owner: 0, MoverMode: 1,
		X: numeric.FixedFromInt(x), Z: numeric.FixedFromInt(z),
		HullOffsetX: -8 << 16, HullOffsetY: 16 << 16, HullOffsetZ: -8 << 16,
		HullXExtent: 16 << 16, HullYExtent: 16 << 16, HullZExtent: 16 << 16,
	}
}

func publishNoticeFrame(t *testing.T, b *frame.Buffer, tick uint32, u frame.UnitView, warning bool) {
	t.Helper()
	f := b.BeginWrite()
	f.Units = append(f.Units, u)
	if warning {
		f.Events = append(f.Events, frame.EventView{
			Kind: frame.EventKindStatus, Tick: tick, Source: u.Slot,
			StatusKind: uint8(audio.SlotUnderAttack), StatusText: "Under Attack", StatusClass: 1,
		})
	}
	if err := b.Publish(tick); err != nil {
		t.Fatal(err)
	}
}

// Reject before queue insertion: an on-screen victim must consume neither a
// caption, voice request nor a private variant draw [07 R-HUD-03 §14.1].
func TestUnderAttackViewportAdmissionIgnoresSelection(t *testing.T) {
	for _, inView := range []bool{true, false} {
		for _, selected := range []bool{false, true} {
			t.Run(fmt.Sprintf("in view %v, selected %v", inView, selected), func(t *testing.T) {
				b := &frame.Buffer{}
				a := audio.NewService(nil)
				cat := &audio.Category{}
				cat.Rows[audio.SlotUnderAttack].Variants = []string{"warning"}
				a.Queue.Register(1, cat, "Fixture", true)
				seed := rng.NewCRT(23)
				a.BindCRT(&seed)
				c := &Client{buffer: b, width: 640, height: 480, cam: &camera.Camera{}, messages: *frame.NewMessageRing()}
				c.SetAudioService(a)
				plays := 0
				a.Queue.OnPlay(func(alias string, _ audio.Slot, _ pool.Handle) {
					if alias != "warning" {
						t.Fatalf("voice alias = %q", alias)
					}
					plays++
				})
				u := noticeUnit(200, 200)
				if !inView {
					u.X = 900 << 16
				}
				if selected {
					u.Flags |= 0x10
				}
				publishNoticeFrame(t, b, 60, u, false)
				c.TickAudio() // preceding host frame's membership
				publishNoticeFrame(t, b, 61, u, true)
				c.TickAudio()
				want := 1
				expected := seed
				if inView {
					want = 0
				} else {
					expected.Rand()
				}
				if plays != want || len(c.MessageLines()) != want || a.Queue.CRTRandom().State != expected.State {
					t.Fatalf("voices/captions/CRT = %d/%d/%d, want %d/%d/%d", plays, len(c.MessageLines()), a.Queue.CRTRandom().State, want, want, expected.State)
				}
				if inView && (a.Queue.Count != 0 || a.Queue.BaseTime != 0) {
					t.Fatal("rejected warning changed queue arbitration")
				}
				c.TickAudio()
				if plays != want || len(c.MessageLines()) != want {
					t.Fatal("repeated drain replayed the warning")
				}
			})
		}
	}
}

func TestNoticeViewportRetainsInclusiveDefinitionBounds(t *testing.T) {
	for _, tc := range []struct {
		name string
		x, z int64
		want bool
	}{
		{"touch left", 120, 200, true}, {"beyond left", 119, 200, false},
		{"touch right", 647, 200, true}, {"beyond right", 648, 200, false},
		{"touch top", 200, 24, true}, {"beyond top", 200, 23, false},
		{"touch bottom", 200, 463, true}, {"beyond bottom", 200, 464, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := &Client{width: 640, height: 480, cam: &camera.Camera{}}
			c.retainNoticeViewport(&frame.Frame{Units: []frame.UnitView{noticeUnit(tc.x, tc.z)}})
			if got := len(c.noticeOnScreen) != 0; got != tc.want {
				t.Fatalf("viewport membership = %v, want %v [07 R-REV-01 §5]", got, tc.want)
			}
		})
	}
}

func TestNoticeViewportSeparatesHighWordsAndUsesPlotHeight(t *testing.T) {
	c := &Client{width: 640, height: 480, cam: &camera.Camera{}}
	u := noticeUnit(200, 456)
	u.Y = 7 << 14                               // 1.75
	u.HullOffsetY, u.HullYExtent = 3<<14, 3<<14 // 0.75
	c.retainNoticeViewport(&frame.Frame{Units: []frame.UnitView{u}})
	if len(c.noticeOnScreen) != 0 {
		t.Fatal("added fractional height before the producer's separate high-word reads")
	}
	u = noticeUnit(200, 72)
	u.Y = 100 << 16
	c.terrain = &world.Terrain{CellW: 40, CellH: 40, Plot: make([]world.PlotCell, 40*40)}
	cell := c.terrain.PlotAt(12, 4)
	cell.SetMinHeight(200)
	cell.SetMaxHeight(200) // these derived fields must not replace authored height 0
	for _, mode := range []uint8{0, 2} {
		for _, flags := range []uint32{0, 1} {
			u.MoverMode, u.Flags = mode, flags
			c.retainNoticeViewport(&frame.Frame{Units: []frame.UnitView{u}})
			if len(c.noticeOnScreen) != 1 {
				t.Fatalf("mode %d flags %d omitted authored plot-height cap", mode, flags)
			}
		}
	}
	u.MoverMode = 1
	c.retainNoticeViewport(&frame.Frame{Units: []frame.UnitView{u}})
	if len(c.noticeOnScreen) != 0 {
		t.Fatal("ground mode incorrectly applied the plot-height cap")
	}
}

func TestWarningUsesPrecedingViewportAndEntryClearsIt(t *testing.T) {
	b := &frame.Buffer{}
	a := audio.NewService(nil)
	a.Queue.Register(1, nil, "Fixture", true)
	c := &Client{buffer: b, width: 640, height: 480, cam: &camera.Camera{}, messages: *frame.NewMessageRing()}
	c.SetAudioService(a)
	u := noticeUnit(200, 200)
	publishNoticeFrame(t, b, 60, u, false)
	c.TickAudio()
	c.cam.X = 1000 // camera scroll follows the damage batch
	publishNoticeFrame(t, b, 61, u, true)
	c.TickAudio()
	if len(c.MessageLines()) != 0 || len(c.noticeOnScreen) != 0 {
		t.Fatal("warning used refreshed viewport instead of preceding membership")
	}
	publishNoticeFrame(t, b, 62, u, true)
	c.TickAudio()
	if len(c.MessageLines()) != 1 {
		t.Fatal("off-screen victim's next warning was suppressed")
	}
	c.cam.X = 0
	c.retainNoticeViewport(&frame.Frame{Units: []frame.UnitView{u}})
	c.SetSnapshot(&frame.Buffer{})
	if len(c.noticeOnScreen) != 0 {
		t.Fatal("new battle retained old viewport membership")
	}
}
