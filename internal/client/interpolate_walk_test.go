package client

import (
	"reflect"
	"testing"

	"github.com/nanolathe-gg/nanolathe/internal/frame"
	"github.com/nanolathe-gg/nanolathe/internal/pool"
)

// Authored fixture: one axis holds each gait key for three ticks while the
// weapon aims every tick. Enhanced smooths only the held axis (§13.5).
func walkFrame(tick uint32, slots ...pool.Handle) *frame.Frame {
	f := &frame.Frame{Tick: tick}
	for _, slot := range slots {
		u := unitAt(slot, wu(int64(tick)), 0, 0)
		u.InstanceID, u.MoverMode = uint64(slot), 1
		u.Pieces = []frame.PieceView{{Tx: wu(int64(tick/3)*6 + int64(slot-1)*20), RotY: uint16(tick * 100)}}
		f.Units = append(f.Units, u)
	}
	return f
}

func warmWalk(in *interpolator) (*frame.Frame, *frame.Frame) {
	var prev, cur *frame.Frame
	for tick := uint32(1); tick <= 8; tick++ {
		prev, cur = walkFrame(tick-1, 1), walkFrame(tick, 1)
		in.blend(prev, cur, int64(fractionOne)/2)
	}
	return prev, cur
}

func TestWalkHeldGaitKeepsRootAndAimTiming(t *testing.T) {
	var in interpolator
	prev, cur := warmWalk(&in)
	before := cur.Units[0].Pieces[0]
	u := in.blend(prev, cur, int64(fractionOne)/2).Units[0]
	if u.X != wu(7)+wu(1)/2 || u.Pieces[0].RotY != 750 || (u.Pieces[0].Tx < wu(11)-6 || u.Pieces[0].Tx > wu(11)) {
		t.Fatalf("held gait blend: root=%d aim=%d gait=%d", u.X, u.Pieces[0].RotY, u.Pieces[0].Tx)
	}
	if cur.Units[0].Pieces[0] != before {
		t.Fatal("committed pose changed")
	}
	wantPiece := u.Pieces[0]
	// Same tick redraws consume the same retained keys, including a rejected
	// speculative fraction followed by the correct fraction.
	for range 4 {
		in.blend(prev, cur, int64(fractionOne)/4)
		got := in.blend(prev, cur, int64(fractionOne)/2).Units[0]
		if got.Pieces[0] != wantPiece {
			t.Fatal("redraw advanced history")
		}
	}
	if allocs := testing.AllocsPerRun(10, func() { in.blend(prev, cur, int64(fractionOne)/2) }); allocs != 0 {
		t.Fatalf("steady walk redraw allocated %v times", allocs)
	}
}

func TestWalkHistoryResetsAtLifecycleBoundaries(t *testing.T) {
	cases := []struct {
		name string
		edit func(*frame.Frame, *frame.Frame)
	}{
		{"stop", func(p, c *frame.Frame) { c.Units[0].X = p.Units[0].X }},
		{"building", func(p, c *frame.Frame) { c.Units[0].IsBuilding = true }},
		{"nanoframe", func(p, c *frame.Frame) { c.Units[0].BuildRemaining = 0.5 }},
		{"airborne", func(p, c *frame.Frame) { c.Units[0].MoverMode = 2 }},
		{"transport", func(p, c *frame.Frame) { c.Units[0].Carrier = 3 }},
		{"death", func(p, c *frame.Frame) { c.Units = nil }},
		{"capture", func(p, c *frame.Frame) { c.Units[0].Owner++ }},
		{"reuse", func(p, c *frame.Frame) { c.Units[0].InstanceID++ }},
		{"teleport", func(p, c *frame.Frame) { c.Units[0].X = wu(1000) }},
		{"missing publication", func(p, c *frame.Frame) { p.Tick = 7 }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var in interpolator
			warmWalk(&in)
			p, c := walkFrame(8, 1), walkFrame(9, 1)
			tc.edit(p, c)
			in.blend(p, c, int64(fractionOne)/2)
			if len(in.walk) != 0 {
				t.Fatal("ineligible history retained")
			}
		})
	}
}

func TestWalkHistoryIdentityAndTimeline(t *testing.T) {
	for _, name := range []string{"rollback", "gap", "shared tick replaced", "both new subject", "unseen teleport", "unseen stop"} {
		t.Run(name, func(t *testing.T) {
			var in interpolator
			warmWalk(&in)
			p, c := walkFrame(8, 1), walkFrame(9, 1)
			switch name {
			case "rollback":
				p, c = walkFrame(1, 1), walkFrame(2, 1)
			case "gap":
				p, c = walkFrame(12, 1), walkFrame(13, 1)
			case "shared tick replaced":
				p.Units[0].InstanceID = 44
				c.Units[0].InstanceID = 44
			case "both new subject":
				p, c = walkFrame(9, 1), walkFrame(10, 1)
				p.Units[0].Owner = 2
				c.Units[0].Owner = 2
			case "unseen teleport":
				p, c = walkFrame(9, 1), walkFrame(10, 1)
				p.Units[0].X += wu(1000)
				c.Units[0].X += wu(1000)
			case "unseen stop":
				p, c = walkFrame(9, 1), walkFrame(10, 1)
				p.Units[0].X = wu(8)
			}
			var fresh interpolator
			want := fresh.blend(p, c, int64(fractionOne)/2).Units[0]
			got := in.blend(p, c, int64(fractionOne)/2).Units[0]
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("old history crossed %s", name)
			}
		})
	}
	// Skipping one presented pair is safe when the next pair supplies both
	// otherwise unseen committed ticks. No publication is synthesized.
	var full, skipped interpolator
	for tick := uint32(1); tick <= 8; tick++ {
		p, c := walkFrame(tick-1, 1), walkFrame(tick, 1)
		full.blend(p, c, int64(fractionOne)/2)
		if tick != 7 {
			skipped.blend(p, c, int64(fractionOne)/2)
		}
	}
	if !reflect.DeepEqual(full.units, skipped.units) {
		t.Fatal("available previous publication was not consumed")
	}
}

func TestWalkHistoryFollowsSlotsAcrossParallelReordering(t *testing.T) {
	const n = 300
	slots := make([]pool.Handle, n)
	for i := range slots {
		slots[i] = pool.Handle(i + 1)
	}
	var seq, par interpolator
	p := newRecordPool(4)
	defer p.close()
	par.each = p.forEachFn
	for tick := uint32(1); tick <= 9; tick++ {
		prev, cur := walkFrame(tick-1, slots...), walkFrame(tick, slots...)
		for i, j := 0, len(cur.Units)-1; i < j; i, j = i+1, j-1 {
			cur.Units[i], cur.Units[j] = cur.Units[j], cur.Units[i]
		}
		want := seq.blend(prev, cur, int64(fractionOne)/2)
		got := par.blend(prev, cur, int64(fractionOne)/2)
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("parallel blend differs at tick %d", tick)
		}
	}
	for _, u := range par.units {
		want := wu(13 + int64(u.Slot-1)*20)
		if got := u.Pieces[0].Tx; got < want-6 || got > want {
			t.Fatalf("slot %d borrowed another history: %d, want near %d", u.Slot, got, want)
		}
	}
	par.blend(walkFrame(9, slots...), walkFrame(10, 1), int64(fractionOne)/2)
	if len(par.walk) != 1 {
		t.Fatalf("removed histories retained: %d", len(par.walk))
	}
}

func TestWalkClientPauseSwitchAndPreRecord(t *testing.T) {
	c, buf := pipelineClient(t)
	publish := func(tick uint32) {
		f := buf.BeginWrite()
		f.Units = append(f.Units, walkFrame(tick, 1).Units...)
		if err := buf.Publish(tick); err != nil {
			t.Fatal(err)
		}
	}
	for tick := uint32(3); tick <= 8; tick++ {
		publish(tick)
		c.SetTickFraction(0.5)
		c.presentationFrame()
	}
	c.SetPresentationPaused(true)
	want := c.presentationFrame().Units[0].Pieces[0]
	for range 3 {
		if got := c.presentationFrame().Units[0].Pieces[0]; got != want {
			t.Fatal("pause changed pose")
		}
	}
	c.SetPresentationPaused(false)
	c.StartPreRecord(ClampTickFraction16(0.25), 0, false)
	c.JoinPreRecord()
	c.SetTickFraction(0.5)
	if _, ok := c.TakePreRecord(c.PresentationDigest(), 0); ok {
		t.Fatal("mismatched speculative fraction accepted")
	}
	if got := c.presentationFrame().Units[0].Pieces[0]; got != want {
		t.Fatal("discarded record advanced pose history")
	}
	c.SetInterpolation(false)
	if len(c.interp.walk) != 0 {
		t.Fatal("Original retained smoothing history")
	}
	if got := c.presentationFrame(); got != buf.Current() {
		t.Fatal("Original used blended frame")
	}
	c.SetInterpolation(true)
	var fresh interpolator
	expected := fresh.blend(buf.Previous(), buf.Current(), int64(fractionOne)/2).Units[0].Pieces[0]
	if got := c.presentationFrame().Units[0].Pieces[0]; got != expected {
		t.Fatal("reenabling reused old history")
	}
	c.buffer = frame.NewBuffer()
	if c.presentationFrame() != nil || len(c.interp.walk) != 0 {
		t.Fatal("missing current retained history")
	}
}

func TestWalkPublicationSourceBoundaries(t *testing.T) {
	for _, name := range []string{"replacement buffer", "missing previous"} {
		t.Run(name, func(t *testing.T) {
			c := &Client{}
			c.SetInterpolation(true)
			c.SetTickFraction(0.5)
			publish := func(buf *frame.Buffer, tick uint32) {
				f := buf.BeginWrite()
				f.Units = append(f.Units, walkFrame(tick, 1).Units...)
				if err := buf.Publish(tick); err != nil {
					t.Fatal(err)
				}
			}
			c.buffer = frame.NewBuffer()
			for tick := uint32(0); tick <= 8; tick++ {
				publish(c.buffer, tick)
				c.presentationFrame()
			}
			if len(c.interp.walk) == 0 {
				t.Fatal("fixture did not acquire history")
			}
			c.buffer = frame.NewBuffer()
			if name == "replacement buffer" {
				publish(c.buffer, 7)
			}
			publish(c.buffer, 8)
			got := c.presentationFrame()
			if name == "missing previous" {
				if got != c.buffer.Current() || len(c.interp.walk) != 0 {
					t.Fatal("missing previous borrowed history")
				}
			} else {
				var fresh interpolator
				want := fresh.blend(c.buffer.Previous(), c.buffer.Current(), int64(fractionOne)/2)
				if !reflect.DeepEqual(got, want) {
					t.Fatal("replacement buffer borrowed old history")
				}
			}
		})
	}
}
