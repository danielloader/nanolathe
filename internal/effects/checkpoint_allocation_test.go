package effects

import (
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/nanolathe-gg/nanolathe/internal/sim/checkpoint"
)

// Inactive geometry remains a full physical payload, but its paths need no
// formatting on success (DESIGN_MULTIPLAYER §16.3.81).
func TestCheckpointEffectsDenseAllocation(t *testing.T) {
	measure := func(size int) float64 {
		p := &FixedEffectPool{fragments: make([]fragmentGeometry, size)}
		c := NewCheckpointContext(nil, p)
		e := checkpoint.NewEncoder(io.Discard)
		return testing.AllocsPerRun(5, func() {
			if err := p.WriteCheckpoint(e, c); err != nil {
				panic(err)
			}
		})
	}
	small, large := measure(16), measure(4096)
	t.Logf("fragments: 16 rows %.0f allocations; 4096 rows %.0f", small, large)
	if small != 0 || large != 0 {
		t.Fatalf("physical fragment paths allocate: %g versus %g", small, large)
	}
}

type effectsCheckpointFailAt struct {
	remaining int
	cause     error
}

func (w *effectsCheckpointFailAt) Write(p []byte) (int, error) {
	if len(p) > w.remaining {
		return 0, w.cause
	}
	w.remaining -= len(p)
	return len(p), nil
}

func TestCheckpointEffectsDenseErrorPaths(t *testing.T) {
	// Authored fragment framing is 41 prefix bytes, then 217 bytes per row.
	// Both inactive physical rows remain in place before the record count.
	for _, tc := range []struct {
		offset int
		path   string
	}{
		{258, "fragments[1].angles"}, {264, "fragments[1].angularRates"},
		{270, "fragments[1].baseVelocity"}, {282, "fragments[1].live"},
		{283, "fragments[1].vertices"}, {467, "fragments[1].vertices"},
		{488, "records[0].AnimA.Active"}, {489, "records[0].AnimA.Countdown"},
		{493, "records[0].AnimA.Durations"}, {497, "records[0].AnimA.Durations"},
		{501, "records[0].AnimA.Frames"}, {509, "records[0].AnimA.Idx"},
		{513, "records[0].AnimA.Loop"}, {514, "records[0].AnimB.Active"},
		{536, "records[0].ExpiryTick"}, {540, "records[0].FragmentExplodeOnHit"},
		{541, "records[0].FragmentSlot"}, {543, "records[0].Gravity"},
		{551, "records[0].HasModel"}, {552, "records[0].Kind"},
		{556, "records[0].Source"}, {560, "records[0].Target"},
		{564, "records[0].VX"}, {572, "records[0].VY"}, {580, "records[0].VZ"},
		{588, "records[0].X"}, {596, "records[0].Y"}, {604, "records[0].Z"},
	} {
		t.Run(tc.path, func(t *testing.T) {
			p := &FixedEffectPool{fragments: make([]fragmentGeometry, 2), records: []EffectRecord{{AnimA: EffectAnimPlayer{Durations: []int32{-1}}}}}
			cause := errors.New("authored sink failure")
			e := checkpoint.NewEncoder(&effectsCheckpointFailAt{remaining: tc.offset, cause: cause})
			err := p.WriteCheckpoint(e, NewCheckpointContext(nil, p))
			want := "logical path effects.FixedEffectPool." + tc.path + ","
			if !errors.Is(err, cause) || !strings.Contains(err.Error(), want) {
				t.Fatalf("offset %d: error=%v, want %s", tc.offset, err, want)
			}
		})
	}
}
