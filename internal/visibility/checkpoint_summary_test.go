package visibility

import (
	"reflect"
	"testing"

	"github.com/nanolathe-gg/nanolathe/internal/sim/checkpoint"
)

func TestCheckpointVisibilitySummaryVectorAndBlindSpots(t *testing.T) {
	s := &Service{mode: ModeHistoryEnabled | ModeTerrainRay | ModeFogCacheValid,
		local: 3, team: [10]uint16{1, 0x8000}, viewerDefeated: true,
		wordMask: []uint16{0xffff, 7}}
	s.byteGrids[0], s.byteGrids[2] = []uint8{0, 255}, []uint8{9}
	var got checkpoint.Summary
	got.Word(19)
	if err := s.AppendCheckpointSummary(&got); err != nil {
		t.Fatal(err)
	}
	words := []uint64{19, 5, 3, 1, 0x8000, 0, 0, 0, 0, 0, 0, 0, 0, 1,
		2, 0xffff, 7, 2, 0, 255, 0, 1, 9, 0, 0, 0, 0, 0, 0, 0}
	var expected uint64
	for i, word := range words {
		expected += uint64(i+1) * word
	}
	if count, sum := got.Result(); count != uint64(len(words)) || sum != expected {
		t.Fatalf("got (%d,%x), want (%d,%x)", count, sum, len(words), expected)
	}
	// Presentation cache state and full-only footprints/dimensions do not
	// enter the cheap row, even when a full binding validator would refuse.
	s.mode &^= ModeFogCacheValid
	s.W, s.H = -1, 999
	s.footprints = map[ObserverID]footprint{1: {cx: 99}}
	s.Community.SetAllied(func(PlayerID, PlayerID) bool { panic("callback") })
	var blind checkpoint.Summary
	blind.Word(19)
	if err := s.AppendCheckpointSummary(&blind); err != nil || blind != got {
		t.Fatal("unselected visibility state changed summary", err)
	}
	s.byteGrids[0][1]--
	var selected checkpoint.Summary
	selected.Word(19)
	if err := s.AppendCheckpointSummary(&selected); err != nil || selected == got {
		t.Fatal("selected refcount change lost", err)
	}
}

func TestCheckpointVisibilitySummaryNoAllocationOrMutation(t *testing.T) {
	s := &Service{wordMask: []uint16{7, 9}, byteGrids: [10][]uint8{{1, 2}}}
	before := &Service{wordMask: []uint16{7, 9}, byteGrids: [10][]uint8{{1, 2}}}
	if allocations := testing.AllocsPerRun(20, func() {
		var summary checkpoint.Summary
		if err := s.AppendCheckpointSummary(&summary); err != nil {
			panic(err)
		}
	}); allocations != 0 || !reflect.DeepEqual(s, before) {
		t.Fatalf("summary allocated or mutated: %g", allocations)
	}
	var summary checkpoint.Summary
	summary.Word(11)
	saved := summary
	if err := (*Service)(nil).AppendCheckpointSummary(&summary); err == nil || summary != saved {
		t.Fatal("missing service changed the accumulator")
	}
	if err := s.AppendCheckpointSummary(nil); err == nil {
		t.Fatal("missing summary accepted")
	}
}
