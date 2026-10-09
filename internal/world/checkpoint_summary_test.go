package world

import (
	"math"
	"slices"
	"testing"

	"github.com/nanolathe-gg/nanolathe/internal/sim/checkpoint"
)

func terrainSummaryFixture() *Terrain {
	return &Terrain{Plot: []PlotCell{
		{0xfe, 0xff, 0xff, 0x7f, 8, 9, 7, 42, 0xfe, 0xff, 0x34, 0x12, 0xff},
		{0xff, 0xff, 0, 0, 5, 6, 4, 0, 0xfb, 0xff, 0x80, 0xff, 0x7c},
	}}
}

func readTerrainSummary(t *testing.T, terrain *Terrain) checkpoint.Summary {
	t.Helper()
	var out checkpoint.Summary
	if err := terrain.AppendCheckpointSummary(&out); err != nil {
		t.Fatal(err)
	}
	return out
}

func TestCheckpointSummaryTerrainLiteralWords(t *testing.T) {
	terrain := terrainSummaryFixture()
	words := []uint64{37, 2, 0xfffffffffffffffe, 32767, 42, 65534, 4660, 131,
		0xffffffffffffffff, 0, 0, 65531, 65408, 0}
	var out checkpoint.Summary
	out.Word(37)
	if err := terrain.AppendCheckpointSummary(&out); err != nil {
		t.Fatal(err)
	}
	var sum uint64
	for i, word := range words {
		sum += uint64(i+1) * word
	}
	if count, got := out.Result(); count != uint64(len(words)) || got != sum {
		t.Fatalf("summary = (%d,%x), want (%d,%x)", count, got, len(words), sum)
	}
}

func TestCheckpointSummaryTerrainSelectedExcludedAndPure(t *testing.T) {
	baseline := readTerrainSummary(t, terrainSummaryFixture())
	for _, tc := range []struct {
		name   string
		change func(*Terrain)
	}{
		{"order", func(s *Terrain) { s.Plot[0], s.Plot[1] = s.Plot[1], s.Plot[0] }},
		{"occupant", func(s *Terrain) { s.Plot[1].SetOccupantB(-7) }},
		{"metal", func(s *Terrain) { s.Plot[1].SetMetal(8) }},
		{"feature", func(s *Terrain) { s.Plot[1].SetFeature(9) }},
		{"anchor", func(s *Terrain) { s.Plot[1].SetAnchorWord(10) }},
		{"unknown flag", func(s *Terrain) { s.Plot[1].SetFlagByte(0xfc) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := terrainSummaryFixture()
			tc.change(s)
			if readTerrainSummary(t, s) == baseline {
				t.Fatal("selected mutation invisible")
			}
		})
	}
	s := terrainSummaryFixture()
	s.CellW, s.CellH, s.SeaLevel = 99, 88, 77
	s.Tidal = math.Float32frombits(0x7fc00001)
	s.Plot[0].SetHeight(100)
	s.Plot[0].SetMinHeight(101)
	s.Plot[0].SetMaxHeight(102)
	s.Plot[0].SetFlagByte(0x83)
	s.Plot[1].SetFlagByte(0)
	s.SetClassRestamp(func(int32, int32, int16, int16) { panic("summary called restamp") })
	before := slices.Clone(s.Plot)
	if readTerrainSummary(t, s) != baseline {
		t.Fatal("unselected geometry/fog/placer changed summary")
	}
	if !slices.Equal(s.Plot, before) || math.Float32bits(s.Tidal) != 0x7fc00001 {
		t.Fatal("summary mutated terrain")
	}
}

func TestCheckpointSummaryTerrainNilEmptyAndNoAlloc(t *testing.T) {
	var out checkpoint.Summary
	out.Word(99)
	before := out
	if err := (*Terrain)(nil).AppendCheckpointSummary(&out); err == nil || out != before {
		t.Fatal("nil terrain accepted or changed summary")
	}
	if err := (&Terrain{}).AppendCheckpointSummary(nil); err == nil {
		t.Fatal("nil accumulator accepted")
	}
	empty := readTerrainSummary(t, &Terrain{})
	if count, sum := empty.Result(); count != 1 || sum != 0 {
		t.Fatalf("empty summary = (%d,%d)", count, sum)
	}
	s := terrainSummaryFixture()
	if got := testing.AllocsPerRun(100, func() {
		out = checkpoint.Summary{}
		if err := s.AppendCheckpointSummary(&out); err != nil {
			panic(err)
		}
	}); got != 0 {
		t.Fatalf("allocations = %v", got)
	}
}
