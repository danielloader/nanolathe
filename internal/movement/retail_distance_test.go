package movement

import (
	"github.com/nanolathe-gg/nanolathe/internal/sim/numeric"
	"testing"
)

// Air-order distance retains raw fixed-point units and the helper's rounding
// before truncation [04 R-AIR-01 §8][01 R-DET-01 §7].
func TestAirPlanarDistanceUsesRetailRounding(t *testing.T) {
	if got := airPlanarDistance(20, 99, 0, 0); got != 100 {
		t.Fatalf("raw planar distance = %d, want 100", got)
	}
	if got := airPlanarDistance(20<<16, 99<<16, 0, 0); got != (101<<16)-1 {
		t.Fatalf("scaled raw planar distance = %d, want %d", got, (101<<16)-1)
	}
}

func TestAirPlanarDistanceWrapsRawDeltas(t *testing.T) {
	if got := airPlanarDistance(32767<<16, 0, -32767<<16, 0); got != 2<<16 {
		t.Fatalf("wrapped planar distance = %d, want two world units", got)
	}
	if got := airPlanarDistance(1<<31, 0, 0, 0); got != -1<<31 {
		t.Fatalf("signed low distance word = %d, want negative sign-bit value", got)
	}
}

// The dogfight reads a signed whole-unit high word: 160 plus a fraction is
// still close, 161 installs the intercept, and a sign-bit distance is close.
func TestDogfightRangeReadsSignedHighWord(t *testing.T) {
	for _, tc := range []struct {
		name     string
		distance numeric.Fixed
		marker   bool
	}{
		{"fractional", 160<<16 | 65535, false},
		{"next whole unit", 161 << 16, true},
		{"signed high word", 32768 << 16, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sys, _, u := airFixture(t)
			n := pushAirOrder(t, u, "AirToAir", u.X+tc.distance, u.Z)
			n.Phase, n.Param1 = 1, 0
			before := sys.simRNG(u).Draws()
			if code := sys.legAirToAir(u, n, 0, 700); code != 2 {
				t.Fatalf("code = %d, want hold", code)
			}
			c := sys.FlightCommandFor(u.Handle, u)
			got := c != nil && c.Payload != nil
			if got != tc.marker {
				t.Fatalf("intercept marker = %v, want %v", got, tc.marker)
			}
			if n.Deadline != 745 || n.DynamicGate&airLegGateStrike != airLegGateStrike {
				t.Fatal("range arm must keep its deadline and gate")
			}
			if sys.simRNG(u).Draws() != before {
				t.Fatal("range arm must draw no random value")
			}
		})
	}
}
