package main

import (
	"slices"
	"testing"

	"github.com/nanolathe-gg/nanolathe/internal/camera"
	"github.com/nanolathe-gg/nanolathe/internal/client"
	"github.com/nanolathe-gg/nanolathe/internal/drawlist"
	"github.com/nanolathe-gg/nanolathe/internal/frame"
	"github.com/nanolathe-gg/nanolathe/internal/palette"
	"github.com/nanolathe-gg/nanolathe/internal/sim/numeric"
	"github.com/nanolathe-gg/nanolathe/internal/world"
)

// The two pictures share one timeline: rendering Off must not erase earlier
// tracks or stop observing the next tick (interface design §3.17, GPU §15).
func TestNLPreviewTrailCompareKeepsCommittedHistory(t *testing.T) {
	cl, err := client.New(client.Options{Width: 320, Height: 240})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(cl.Close)
	cl.SetPalette(&palette.Tables{})
	cl.SetTerrain(&world.Terrain{CellW: 64, CellH: 64, TileIndices: make([]uint16, 32*32), TileSet: make([][1024]byte, 1)})
	cl.SetCamera(&camera.Camera{Scale: camera.ViewScaleNative, ViewW: 320, ViewH: 240})
	cl.SetEnhanced(true)

	read := func(strength int) []drawlist.Trail {
		t.Helper()
		list := recordNLPreviewFrame(cl, strength)
		if list == nil {
			t.Fatal("preview did not record a frame")
		}
		collector := &nlPreviewTrailCollector{}
		list.Replay(collector)
		if collector.terrain != 1 {
			t.Fatal("trail comparison removed the terrain")
		}
		return collector.marks
	}

	previous := 0
	for tick := uint32(1); tick <= 5; tick++ {
		f := cl.Buffer().BeginWrite()
		f.Tick = tick
		f.Units = append(f.Units[:0], frame.UnitView{
			Slot: 1, BMCode: true, MoverMode: 1, FootX: 2,
			X: numeric.FixedFromInt(40 + int64(tick)*24), Z: numeric.FixedFromInt(50),
		})
		if err := cl.Buffer().Publish(tick); err != nil {
			t.Fatal(err)
		}
		cl.ObserveCommittedTick()
		on := read(50)
		if tick > 1 && len(on) <= previous {
			t.Fatalf("tick %d kept %d tracks after %d: comparison lost the shared history", tick, len(on), previous)
		}
		if off := read(0); len(off) != 0 {
			t.Fatalf("Off comparison still draws %d tracks", len(off))
		}
		if again := read(50); !slices.Equal(on, again) {
			t.Fatalf("tick %d changed the same committed trails after rendering Off", tick)
		}
		previous = len(on)
		// The alternate picture is last in the real preview. Tick observation
		// must still run after it, even when no primary repaint intervenes.
		read(0)
	}
}

type nlPreviewTrailCollector struct {
	nlPreviewHideTrails
	marks   []drawlist.Trail
	terrain int
}

func (c *nlPreviewTrailCollector) Terrain(drawlist.Terrain) { c.terrain++ }
func (c *nlPreviewTrailCollector) Trails(batch drawlist.Trails) {
	for _, mark := range batch.Marks {
		if mark.Strength > 0 {
			c.marks = append(c.marks, mark)
		}
	}
}
