package ai

import (
	"bytes"
	"maps"
	"math"
	"reflect"
	"testing"

	"github.com/nanolathe-gg/nanolathe/internal/sim/checkpoint"
	"github.com/nanolathe-gg/nanolathe/internal/units"
)

func TestCheckpointStrategicVector(t *testing.T) {
	s := Strategic{BuildCapable: -1, CenterX: -2, CenterY: 1<<40 + 3, CenterZ: 4,
		ClassVectors: map[string]ClassVector{"z": {-1, 2, -3}, "A": {4, -5, 6}}, Counts: map[string]int32{"z": 8, "A": -7}, InitVectors: map[string]int8{"z": -9},
		LandRegion: PlacementRegion{CellH: 12, CellW: 11, OffsetX: -13, OffsetZ: 14}, LastRefreshTick: 15,
		MetalSpots: []MetalSpot{{-1, 2, math.Float32frombits(0x80000000)}, {3, -4, math.Float32frombits(0x7f800000)}}, Radius: -16, SingleVectors: map[string]int8{"A": -17},
		WaterRegion: PlacementRegion{CellH: -19, CellW: 18, OffsetX: -20, OffsetZ: 21}, liveUnitCount: 65530, maxWind: -22, maxWindBound: true, setupDrawsReady: true, unitLimit: 65531, unitLimitBound: true}
	var out bytes.Buffer
	e := checkpoint.NewEncoder(&out)
	s.writeCheckpoint(e)
	if err := e.Err(); err != nil {
		t.Fatal(err)
	}
	want := aiCheckpointHex(t, "ffffffff00feffffffffffffff0300000000010000040000000000000002000000010000004104fb06010000007aff02fd020000000100000041f9ffffff010000007a0800000001000000010000007af70c000b00f3ff0e000f00000002000000ffff0200000000800300fcff0000807ff0ffffff010000000100000041efedff1200ecff150000faffeaffffff010001fbff01")
	if !bytes.Equal(out.Bytes(), want) {
		t.Fatalf("got %x\nwant %x", out.Bytes(), want)
	}
}

func TestCheckpointStrategicRetainedStateAndPurity(t *testing.T) {
	c := aiCheckpointContext(t)
	baseline := aiCheckpointBytes(t, &Manager{}, c)
	for name, edit := range map[string]func(*Strategic){
		"ready": func(s *Strategic) { s.setupDrawsReady = true }, "limit bound": func(s *Strategic) { s.unitLimitBound = true }, "wind bound": func(s *Strategic) { s.maxWindBound = true },
		"limit": func(s *Strategic) { s.unitLimit = 65535 }, "wind": func(s *Strategic) { s.maxWind = -1 }, "live": func(s *Strategic) { s.liveUnitCount = 65535 },
		"build capable": func(s *Strategic) { s.BuildCapable = -1 }, "refresh": func(s *Strategic) { s.LastRefreshTick = 1 }, "radius": func(s *Strategic) { s.Radius = -1 },
		"land": func(s *Strategic) { s.LandRegion.OffsetX = -1 }, "water": func(s *Strategic) { s.WaterRegion.CellW = 1 }, "centre": func(s *Strategic) { s.CenterY = 1 << 40 },
		"counts": func(s *Strategic) { s.Counts = map[string]int32{"unused": -1} }, "class": func(s *Strategic) { s.ClassVectors = map[string]ClassVector{"unused": {C2: -1}} },
		"init": func(s *Strategic) { s.InitVectors = map[string]int8{"unused": -1} }, "single": func(s *Strategic) { s.SingleVectors = map[string]int8{"unused": -1} },
		"spots": func(s *Strategic) { s.MetalSpots = []MetalSpot{{CellX: -1, CellZ: 2, Metal: 3}} },
	} {
		t.Run(name, func(t *testing.T) {
			m := &Manager{}
			edit(&m.Strategic)
			if bytes.Equal(baseline, aiCheckpointBytes(t, m, c)) {
				t.Fatal("retained mutation vanished")
			}
		})
	}
	m := &Manager{Strategic: Strategic{MetalSpots: []MetalSpot{{1, 2, 3}, {4, 5, 6}}, Counts: map[string]int32{"z": 9, "a": 1}}}
	before := m.Strategic
	before.Counts = maps.Clone(m.Strategic.Counts)
	before.MetalSpots = append([]MetalSpot(nil), m.Strategic.MetalSpots...)
	first := aiCheckpointBytes(t, m, c)
	if !reflect.DeepEqual(before, m.Strategic) {
		t.Fatal("capture mutated strategic state")
	}
	m.Strategic.Counts = map[string]int32{"a": 1, "z": 9}
	if !bytes.Equal(first, aiCheckpointBytes(t, m, c)) {
		t.Fatal("map insertion order entered bytes")
	}
	m.Strategic.MetalSpots[0], m.Strategic.MetalSpots[1] = m.Strategic.MetalSpots[1], m.Strategic.MetalSpots[0]
	if bytes.Equal(first, aiCheckpointBytes(t, m, c)) {
		t.Fatal("metal spot order collapsed")
	}
	baseline = aiCheckpointBytes(t, m, c)
	m.Strategic.countsWalk = []*units.Unit{{Handle: 7}}
	m.Strategic.setupDraws = [8]uint32{1, 2, 3, 4, 5, 6, 7, 8}
	m.Strategic.negRegionW = 99
	m.Strategic.negRegionH = 98
	m.Strategic.posRegionW = 97
	m.Strategic.posRegionH = 96
	if !bytes.Equal(baseline, aiCheckpointBytes(t, m, c)) {
		t.Fatal("excluded draw ledger retained")
	}
	for _, bits := range []uint32{0x7fc00000, 0xff800001} {
		m.Strategic.MetalSpots[0].Metal = math.Float32frombits(bits)
		if _, err := m.CollectCheckpointReferences(c); err == nil {
			t.Fatal("NaN collected")
		}
		var out bytes.Buffer
		if err := m.WriteCheckpoint(checkpoint.NewEncoder(&out), c); err == nil {
			t.Fatal("NaN encoded")
		}
	}
}
