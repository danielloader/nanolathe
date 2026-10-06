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
		// Consumer-width fixtures, not claims that ordinary unit histories
		// reach these coordinate pairs [08 R-TRIG-01 §5].
		{"positive minus negative wraps", -1 << 31, 0, 0, 1<<31 - 1, 0, true},
		{"negative minus positive wraps", 1<<31 - 1, 0, 0, -1 << 31, 0, true},
		{"wrapped one pixel remains outside zero radius", -1<<31 + 65535, 0, 0, 1<<31 - 1, 0, false},
		{"distance sum wraps before signed comparison", 0, 0, 0, -1 << 31, -1 << 31, true},
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

// The overlap representation may retain attached entries. Radius consumers
// must skip them before invoking even a stopping visitor, including a factory
// product whose mode still writes ground occupancy [04 R-COLL-01 §11].
func TestRepairRadiusSkipsAttachedBucketHead(t *testing.T) {
	s, w, carrier := releaseFixture(t, wiringDef(), 2)
	s.Grid.AttachOverlap(s, func(uint8) uint8 { return 1 })
	s.Grid.StampPlane(PlaneGround, s.Collisions[carrier].CachedAnchor, 1, 1, int(carrier))
	cargo, err := w.Create(wiringDef(), 0, 64<<16, 0, 48<<16)
	if err != nil {
		t.Fatal(err)
	}
	s.EnsureUnit(w.Unit(cargo))
	if !AttachFactoryProduct(w, carrier, cargo, -1) {
		t.Fatal("attach refused")
	}
	for _, stop := range []bool{false, true} {
		var got []pool.Handle
		s.VisitUnitsInRadius(64<<16, 64<<16, 128<<16, func(h pool.Handle, _ *units.Unit) bool {
			got = append(got, h)
			return stop
		})
		if !reflect.DeepEqual(got, []pool.Handle{carrier}) {
			t.Fatalf("stop=%v: got %v want carrier only", stop, got)
		}
	}
}
