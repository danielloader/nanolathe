package features

import (
	"slices"
	"testing"

	"github.com/nanolathe-gg/nanolathe/internal/content"
	"github.com/nanolathe-gg/nanolathe/internal/sim/checkpoint"
)

func readFeatureSummary(t *testing.T, s *Service) checkpoint.Summary {
	t.Helper()
	var out checkpoint.Summary
	if err := s.AppendCheckpointSummary(&out); err != nil {
		t.Fatal(err)
	}
	return out
}

func TestCheckpointSummaryFeatureLiteralWordsAndBlindSpots(t *testing.T) {
	inst := &Instance{ReclaimProgress: 99, IsBurning: true}
	s := &Service{instances: map[int]*Instance{3: inst, 7: nil}, cursor: -1, arenaHeld: 4}
	// Count stored records, including nil values, without looking them up.
	words := []uint64{37, 2, 0xffffffffffffffff, 4}
	var out checkpoint.Summary
	out.Word(37)
	if err := s.AppendCheckpointSummary(&out); err != nil {
		t.Fatal(err)
	}
	var sum uint64
	for i, word := range words {
		sum += uint64(i+1) * word
	}
	if count, got := out.Result(); count != uint64(len(words)) || got != sum {
		t.Fatalf("summary = (%d,%x), want (%d,%x)", count, got, len(words), sum)
	}
	baseline := readFeatureSummary(t, s)
	inst.ReclaimProgress++
	inst.IsBurning = false
	// Even invalid excluded list/cache state is not traversed or repaired by
	// this leaf. Session owns the completed-tick boundary (§16.3.77).
	inst.nextActive = inst
	s.activeHead, s.activeWalking = inst, true
	s.instanceKeys, s.instanceKeysStale = []int{7, 3, -1}, true
	s.instanceValues = []*Instance{nil}
	s.LastReproIdx = 100
	s.SetSequenceFrames(func(*content.FeatureDef, uint8) []int32 { panic("summary queried sequence") })
	delete(s.instances, 7)
	s.instances[8] = &Instance{}
	keys := slices.Clone(s.instanceKeys)
	if readFeatureSummary(t, s) != baseline {
		t.Fatal("unselected state changed summary")
	}
	if !slices.Equal(s.instanceKeys, keys) || !s.instanceKeysStale || !s.activeWalking || inst.nextActive != inst || s.instances[8] == nil {
		t.Fatal("summary mutated source")
	}
	for _, tc := range []struct {
		name   string
		change func(*Service)
	}{
		{"record count", func(s *Service) { s.instances = map[int]*Instance{0: nil} }},
		{"cursor", func(s *Service) { s.cursor-- }},
		{"arena", func(s *Service) { s.arenaHeld++ }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			copy := *s
			tc.change(&copy)
			if readFeatureSummary(t, &copy) == baseline {
				t.Fatal("selected mutation invisible")
			}
		})
	}
}

func TestCheckpointSummaryFeatureNilEmptyAndNoAlloc(t *testing.T) {
	var out checkpoint.Summary
	out.Word(99)
	before := out
	if err := (*Service)(nil).AppendCheckpointSummary(&out); err == nil || out != before {
		t.Fatal("nil service accepted or changed summary")
	}
	if err := (&Service{}).AppendCheckpointSummary(nil); err == nil {
		t.Fatal("nil accumulator accepted")
	}
	empty := readFeatureSummary(t, &Service{})
	if count, sum := empty.Result(); count != 3 || sum != 0 {
		t.Fatalf("empty summary = (%d,%d)", count, sum)
	}
	s := &Service{instances: map[int]*Instance{0: nil}, cursor: -1, arenaHeld: -2}
	if got := testing.AllocsPerRun(100, func() {
		out = checkpoint.Summary{}
		if err := s.AppendCheckpointSummary(&out); err != nil {
			panic(err)
		}
	}); got != 0 {
		t.Fatalf("allocations = %v", got)
	}
}
