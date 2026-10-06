package visibility

import (
	"testing"

	"github.com/nanolathe-gg/nanolathe/internal/sim/numeric"
)

// The callback squares the complete adjusted radius at signed32 width; the
// visitor still searches the unbonused authored radius [03 R-VIS-01 §4].
func TestRadarAdjustedSquareKeepsPromotedWidth(t *testing.T) {
	for _, tc := range []struct {
		name                     string
		radius, height, distance int32
		seen                     bool
	}{
		{"ordinary", 100, 20, 99, true},
		{"strict callback edge", 100, 0, 100, false},
		{"visitor still caps bonus", 100, 20, 101, false},
		{"crosses signed word", 32600, 200, 32550, true},
		{"square wraps signed32", 32600, 7000, 1, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := New(flatTerrain(16, 0), ModeHistoryEnabled|ModeCurrentEnabled)
			var emitterStatus, targetStatus uint32
			units := []SensorUnit{
				{ID: 1, Owner: 0, Status: &emitterStatus, Alive: true, Active: true, X: numeric.FixedFromInt(100), Y: numeric.FixedFromInt(int64(tc.height)), RadarDistance: tc.radius},
				{ID: 2, Owner: 1, Status: &targetStatus, Alive: true, Hidden: true, X: numeric.FixedFromInt(int64(100 + tc.distance))},
			}
			s.SensorTick(1, 2, units)
			if got := targetStatus&SeenBit != 0; got != tc.seen {
				t.Fatalf("Seen=%v, want %v (radius=%d height=%d distance=%d)", got, tc.seen, tc.radius, tc.height, tc.distance)
			}
		})
	}
}
