package client

import (
	"testing"
	"time"

	"github.com/nanolathe-gg/nanolathe/internal/frame"
)

func publishTicks(t *testing.T, b *frame.Buffer, ticks ...uint32) {
	t.Helper()
	for _, tick := range ticks {
		b.BeginWrite()
		if err := b.Publish(tick); err != nil {
			t.Fatalf("publish %d: %v", tick, err)
		}
	}
}

// Under the asynchronous simulation presentation reads the pair the host names,
// holds it while the writer publishes on, and presents a named tick that is not
// published yet as the newest frame alone rather than blending toward it
// (DESIGN_GPU_RENDERER §13.13).
func TestPinPresentationFollowsTheNamedTick(t *testing.T) {
	buf := frame.NewBuffer()
	target, named := uint32(0), false
	c, err := New(Options{Buffer: buf, Width: 640, Height: 480,
		PresentationTick: func(time.Duration) (uint32, float32, bool) { return target, 0, named }})
	if err != nil {
		t.Fatal(err)
	}
	c.PinPresentation()
	if c.pin.buf != nil {
		t.Fatal("the synchronous path pinned a pair")
	}
	// Enabling widens the buffer's rotation before the writer runs beside it.
	c.SetAsyncSimulation(true)
	publishTicks(t, buf, 1, 2, 3)

	target, named = 2, true
	c.PinPresentation()
	if cur, prev := c.committedFrame(), c.committedPrevious(); cur == nil || cur.Tick != 2 || prev == nil || prev.Tick != 1 {
		t.Fatalf("named tick 2 pinned %v / %v", cur, prev)
	}
	// The writer publishes on; the pinned pair still reads ticks 1 and 2.
	publishTicks(t, buf, 4, 5, 6, 7, 8, 9, 10)
	if cur, prev := c.committedFrame(), c.committedPrevious(); cur.Tick != 2 || prev.Tick != 1 {
		t.Fatalf("pinned pair moved to %d/%d", cur.Tick, prev.Tick)
	}

	// A named tick the buffer does not hold presents the newest publication
	// alone.
	target = 20
	c.PinPresentation()
	if cur, prev := c.committedFrame(), c.committedPrevious(); cur == nil || cur.Tick != 10 || prev != nil {
		t.Fatalf("unavailable tick presented %v / %v, want tick 10 unblended", cur, prev)
	}

	// A declining producer presents the newest pair, blended.
	named = false
	c.PinPresentation()
	if cur, prev := c.committedFrame(), c.committedPrevious(); cur.Tick != 10 || prev == nil || prev.Tick != 9 {
		t.Fatalf("declined producer presented %v / %v", cur, prev)
	}

	c.SetAsyncSimulation(false)
	if c.pin.buf != nil {
		t.Fatal("leaving the asynchronous simulation kept a pin")
	}
	if cur := c.committedFrame(); cur == nil || cur.Tick != 10 {
		t.Fatalf("synchronous read = %v, want the current publication", cur)
	}
}

// A recording pass that reads an older pinned tick leaves the Enhanced history
// layers where the host's in-order observation put them.
func TestPinnedPassDoesNotRewindObservedHistory(t *testing.T) {
	buf := frame.NewBuffer()
	c, err := New(Options{Buffer: buf, Width: 640, Height: 480})
	if err != nil {
		t.Fatal(err)
	}
	c.SetAsyncSimulation(true)
	c.enhanced = true
	c.scorch.valid, c.scorch.tick = true, 12
	older := &frame.Frame{Tick: 11}
	c.observeScorchMarks(older)
	if !c.scorch.valid || c.scorch.tick != 12 {
		t.Fatalf("an older pinned tick reset the scorch history to %+v", c.scorch.tick)
	}
}

// The host names the blend fraction with the pair, so a Draw's fraction and
// pair are one sample; a pre-record pins the pair the host names for the
// instant it predicts, which may already be the next tick (§13.5, §13.13).
func TestPinnedPairCarriesTheHostFraction(t *testing.T) {
	buf := frame.NewBuffer()
	var asked []time.Duration
	c, err := New(Options{Buffer: buf, Width: 640, Height: 480,
		TickFraction: func() float32 { return 0.5 },
		PresentationTick: func(ahead time.Duration) (uint32, float32, bool) {
			asked = append(asked, ahead)
			if ahead > 0 {
				return 3, 0.25, true
			}
			return 2, 0.75, true
		}})
	if err != nil {
		t.Fatal(err)
	}
	c.SetAsyncSimulation(true)
	defer c.SetAsyncSimulation(false)
	publishTicks(t, buf, 1, 2, 3)

	c.PinPresentation()
	if got := c.ResolveTickFraction(); got != ClampTickFraction16(0.75) {
		t.Fatalf("Draw fraction = %d, want the pinned sample's %d, not the producer's", got, ClampTickFraction16(0.75))
	}
	c.StartPreRecordAt(8*time.Millisecond, ClampTickFraction16(0.1), 0, false)
	c.JoinPreRecord()
	defer c.CancelPreRecord()
	if got := c.pre.recorded; got.Tick != 3 || got.TickFraction16 != ClampTickFraction16(0.25) {
		t.Fatalf("pre-record digest named %d + %d, want the host's prediction 3 + %d", got.Tick, got.TickFraction16, ClampTickFraction16(0.25))
	}
	if len(asked) != 2 || asked[0] != 0 || asked[1] != 8*time.Millisecond {
		t.Fatalf("host sampled at %v, want [0 8ms]", asked)
	}
}
