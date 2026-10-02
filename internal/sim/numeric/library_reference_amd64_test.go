//go:build amd64 && !amd64.v2

package numeric

import (
	"math"
	"testing"
)

// Only the amd64/v1 build supplies the agreed live reference. The Go compiler
// may fuse the standard library's arithmetic at higher feature levels [I2].
// Committed vectors and digests independently check every target.
func TestRadiansAgainstAMD64Library(t *testing.T) {
	eachRadiansSample(func(x, y float64) {
		got := [5]float64{SinRadians(x), CosRadians(x), TanRadians(x), Atan2Radians(y, x), AcosRadians(x)}
		want := [5]float64{math.Sin(x), math.Cos(x), math.Tan(x), math.Atan2(y, x), math.Acos(x)}
		for j := range got {
			if math.Float64bits(got[j]) != math.Float64bits(want[j]) {
				t.Fatalf("(%g,%g) function %d: %016x want %016x", x, y, j, math.Float64bits(got[j]), math.Float64bits(want[j]))
			}
		}
	})
}

func TestAngleSinCosAgainstAMD64Library(t *testing.T) {
	for a := 0; a < AngleUnitsTurn; a++ {
		radians := float64(a) * 2 * math.Pi / 65536
		sin, cos := AngleSinCos(Angle(a))
		wantSin, wantCos := math.Sin(radians), math.Cos(radians)
		if math.Float64bits(sin) != math.Float64bits(wantSin) || math.Float64bits(cos) != math.Float64bits(wantCos) {
			t.Fatalf("angle %d: %.17g,%.17g want %.17g,%.17g", a, sin, cos, wantSin, wantCos)
		}
	}
}
