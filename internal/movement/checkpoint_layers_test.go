package movement

import (
	"bytes"
	"testing"

	"github.com/nanolathe-gg/nanolathe/internal/sim/checkpoint"
)

func TestMovementCheckpointLayerRegistryOrder(t *testing.T) {
	build := func(reverse bool) (*System, *CheckpointContext) {
		s := &System{Grid: &OccupancyGrid{}}
		a, b := &ClassLayer{watermark: 1}, &ClassLayer{watermark: 2}
		r := &ClassLayers{byName: make(map[string]*ClassLayer), names: []string{"z", "a"}, grid: s.Grid, anchors: s, movers: s, ticks: s}
		if reverse {
			r.byName["z"] = b
			r.byName["a"] = a
		} else {
			r.byName["a"] = a
			r.byName["z"] = b
		}
		s.layerRegistry = r
		c := movementCheckpointContext()
		collectMovementCheckpoint(t, s, c)
		return s, c
	}
	s, c := build(false)
	if rows := c.Layers.Values(); rows[0].watermark != 1 || rows[1].watermark != 2 {
		t.Fatal("membership discovery not key sorted")
	}
	got := movementCheckpointWrite(t, func(e *checkpoint.Encoder) { writeMovementLayers(e, c, s, s.layerRegistry, "registry") })
	want := movementCheckpointVector(t, uint8(1), uint32(2), uint32(1), uint8('a'), uint16(5), uint32(1), uint32(1), uint8('z'), uint16(5), uint32(2),
		uint8(1), uint8(0), uint8(1), uint32(2), uint32(1), uint8('z'), uint32(1), uint8('a'), uint8(0), uint8(1), uint8(0))
	if !bytes.Equal(got, want) {
		t.Fatalf("registry\ngot  %x\nwant %x", got, want)
	}
	a := movementCheckpointBytes(t, s)
	other, _ := build(true)
	if b := movementCheckpointBytes(t, other); !bytes.Equal(a, b) {
		t.Fatal("map allocation/insertion changed bytes")
	}
	s.layerRegistry.names[0], s.layerRegistry.names[1] = s.layerRegistry.names[1], s.layerRegistry.names[0]
	if b := movementCheckpointBytes(t, s); bytes.Equal(a, b) {
		t.Fatal("allocation-order names were sorted away")
	}
}

func TestMovementCheckpointLayerVectorAndScratch(t *testing.T) {
	s := &System{Grid: &OccupancyGrid{}}
	c := movementCheckpointContext()
	l := &ClassLayer{Grid: s.Grid, H: 2, W: 3, cells: []uint32{4, 5}, commits: []commitWord{{set: true, tick: 6}, {}}, watermark: 7, movers: s}
	got := movementCheckpointWrite(t, func(e *checkpoint.Encoder) { writeMovementLayer(e, c, s, l, "layer") })
	want := movementCheckpointVector(t, uint8(1), int32(2), [16]uint8{}, uint8(0), int32(3), uint32(2), uint32(4), uint32(5), uint32(2), uint8(1), uint32(6), uint8(0), uint32(0), uint8(0), uint8(1), uint32(7))
	if !bytes.Equal(got, want) {
		t.Fatalf("layer\ngot  %x\nwant %x", got, want)
	}
	l.stampScratch = []uint8{9}
	l.stampRows = []uint8{8}
	l.restampTiers = []uint8{7}
	l.restampW = 6
	l.fullStamps = 5
	if after := movementCheckpointWrite(t, func(e *checkpoint.Encoder) { writeMovementLayer(e, c, s, l, "layer") }); !bytes.Equal(got, after) {
		t.Fatal("layer scratch changed bytes")
	}
	l.commits[1].tick = 10
	if after := movementCheckpointWrite(t, func(e *checkpoint.Encoder) { writeMovementLayer(e, c, s, l, "layer") }); bytes.Equal(got, after) {
		t.Fatal("unset commit residual disappeared")
	}
}
