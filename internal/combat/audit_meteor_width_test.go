package combat

import (
	"testing"

	"github.com/nanolathe-gg/nanolathe/internal/sim/rng"
)

// These are authored arithmetic-domain vectors, not a stock-map census
// [06 §6.5]. The large-radius difference survives the later fixed-point shift.
func TestMeteorRandomScalingWidensBeforeDivision(t *testing.T) {
	for _, tc := range []struct{ radius, want int32 }{
		{300001, 69433}, {-300001, -69433}, {2147483647, 497025023},
	} {
		crt := rng.NewCRT(12345)
		radius, angle := MeteorRadiusAndAngle(&crt, tc.radius)
		if radius != tc.want || angle != 38328 || crt.Draws() != 2 {
			t.Fatalf("radius %d: got (%d,%d), draws %d", tc.radius, radius, angle, crt.Draws())
		}
	}
	crt := rng.NewCRT(12345)
	x, z := MeteorTarget(&crt, 300001, 300001)
	if x != -21156 || z != 3897 || crt.Draws() != 2 {
		t.Fatalf("signed cell targets = (%d,%d), draws %d", x, z, crt.Draws())
	}
}

func TestMeteorStoredCoordinatesAndVelocityWrap(t *testing.T) {
	crt := rng.NewCRT(12345)
	x, z := MeteorOrigin(&crt, 32767, -32768)
	if x != -32767 || z != 32755 || crt.Draws() != 2 {
		t.Fatalf("wrapped origin = (%d,%d), draws %d", x, z, crt.Draws())
	}
	vx, vz := MeteorVelocity(0, 3000, 3000, 0)
	if vx.Raw() != 12769325 || vz.Raw() != -12769325 {
		t.Fatalf("wrapped velocities = (%d,%d)", vx.Raw(), vz.Raw())
	}
	crt = rng.NewCRT(12345)
	px, py, pz := MeteorEntryPos(&crt, 2048, -2049, 0)
	if px.Raw() != -2147483648 || pz.Raw() != 2146435072 || py != MeteorHeightFixed || crt.Draws() != 2 {
		t.Fatalf("wrapped entry = (%d,%d,%d), draws %d", px.Raw(), py.Raw(), pz.Raw(), crt.Draws())
	}
}
