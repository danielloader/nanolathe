package movement

import (
	"reflect"
	"testing"

	"github.com/nanolathe-gg/nanolathe/internal/pool"
	"github.com/nanolathe-gg/nanolathe/internal/sim/numeric"
	"github.com/nanolathe-gg/nanolathe/internal/units"
)

func TestRepairRadiusSquaresFullFixedDeltas(t *testing.T) {
	for _, tc := range []struct {
		name                           string
		x, z, radius, targetX, targetZ numeric.Fixed
		want                           bool
	}{
		{"inclusive whole boundary", 0, 0, 230 << 16, 230 << 16, 0, true},
		{"fraction exceeds whole boundary", 1000<<16 | 16384, 0, 230 << 16, 1230<<16 | 49152, 0, false},
		{"square terms truncate separately", 0, 0, 1 << 16, 49152, 49152, true},
		{"full sight admits inside target", 0, 0, 230 << 16, 160 << 16, 0, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			u := &units.Unit{X: tc.targetX, Z: tc.targetZ}
			if got := inUnitScanRadius(int32(tc.x), int32(tc.z), int32(tc.radius), u); got != tc.want {
				t.Fatalf("admitted=%v, want %v [04 R-ORD-02 §4]", got, tc.want)
			}
		})
	}
}

// Sector order differs from both pool order and the collision overlap scan's
// column-first walk; within one bucket the newest relink comes first
// [04 R-ORD-02 §4][04 R-COLL-01 §11].
func TestRepairRadiusUsesSectorRowsAndRelinkOrder(t *testing.T) {
	s, w, first := releaseFixture(t, wiringDef(), 2)
	s.Grid.AttachOverlap(s, func(uint8) uint8 { return 1 })
	s.Grid.StampPlane(PlaneGround, s.Collisions[first].CachedAnchor, 1, 1, int(first))
	add := func(x, z int64, stamp bool) pool.Handle {
		h, err := w.Create(wiringDef(), 0, numeric.FixedFromInt(x), 0, numeric.FixedFromInt(z))
		if err != nil {
			t.Fatal(err)
		}
		if stamp {
			s.EnsureUnit(w.Unit(h))
		}
		return h
	}
	xNext := add(160, 32, true)
	zNext := add(32, 160, true)
	inBucket := add(64, 48, true)
	newest := add(80, 80, true)
	add(-32, -32, true) // off-map sector
	add(48, 64, false)  // no sector link
	var got []pool.Handle
	s.VisitUnitsInRadius(128<<16, 128<<16, 230<<16, func(h pool.Handle, _ *units.Unit) bool {
		got = append(got, h)
		return false
	})
	if want := []pool.Handle{newest, inBucket, first, xNext, zNext}; !reflect.DeepEqual(got, want) {
		t.Fatalf("sector walk %v, want %v", got, want)
	}
	got = nil
	s.VisitUnitsInRadius(128<<16, 128<<16, 230<<16, func(h pool.Handle, _ *units.Unit) bool {
		got = append(got, h)
		return true
	})
	if !reflect.DeepEqual(got, []pool.Handle{newest}) {
		t.Fatalf("stop visitor continued: %v", got)
	}
}

// Each endpoint clamps independently, including an inverted pair from a
// signed-negative sight radius. The circle still squares that radius
// [04 R-ORD-02 §4]; this is arithmetic parity, not a stock content case.
func TestRepairRadiusClampsEverySectorEndpoint(t *testing.T) {
	s, _, h := releaseFixture(t, wiringDef(), 0)
	s.Grid.AttachOverlap(s, func(uint8) uint8 { return 1 })
	s.Grid.StampPlane(PlaneGround, s.Collisions[h].CachedAnchor, 1, 1, int(h))
	var got []pool.Handle
	s.VisitUnitsInRadius(0, 0, -16<<16, func(h pool.Handle, _ *units.Unit) bool {
		got = append(got, h)
		return false
	})
	if !reflect.DeepEqual(got, []pool.Handle{h}) {
		t.Fatalf("clamped edge-sector walk %v, want [%d]", got, h)
	}
}
