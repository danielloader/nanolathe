package construction

import (
	"testing"

	"github.com/nanolathe-gg/nanolathe/internal/sim/numeric"
)

// Construction's approach uses the same distance and pad rounding as work
// orders, including the inclusive compare [05 R-WORK-01 §2, §12][01 R-DET-01 §7].
func TestNanoRangeUsesRetailDistanceAtIntegerBoundary(t *testing.T) {
	svc, builder, _ := approachFixture(t, 10, 10)
	builder.X, builder.Z = 0, 0
	builder.Def.FootprintX, builder.Def.FootprintZ = 0, 0
	builder.Def.BuildDistance = 100
	if !svc.isWithinNanoRange(builder, numeric.FixedFromInt(20), numeric.FixedFromInt(99), 0, 0) {
		t.Fatal("raw distance just below 101 world units must read whole distance 100")
	}
	builder.Def.BuildDistance = 99
	if svc.isWithinNanoRange(builder, numeric.FixedFromInt(20), numeric.FixedFromInt(99), 0, 0) {
		t.Fatal("whole distance 100 must exceed reach 99")
	}
	if got := nanoFootprintPad(20, 99); got != 807 {
		t.Fatalf("scaled pad = %d, want 807 after multiplying the helper result by eight", got)
	}
}

// Large coordinate differences formerly reached a separate saturating root.
// Retail wraps each raw difference before widening, so this remains bounded
// and reads a two-world-unit magnitude [05 R-WORK-01 §2].
func TestNanoRangeWrapsRawDeltasBeforeDistance(t *testing.T) {
	svc, builder, _ := approachFixture(t, 10, 10)
	builder.X, builder.Z = numeric.FixedFromInt(32767), 0
	builder.Def.FootprintX, builder.Def.FootprintZ = 0, 0
	builder.Def.BuildDistance = 1
	if svc.isWithinNanoRange(builder, numeric.FixedFromInt(-32767), 0, 0, 0) {
		t.Fatal("wrapped distance two must exceed reach one")
	}
	builder.Def.BuildDistance = 2
	if !svc.isWithinNanoRange(builder, numeric.FixedFromInt(-32767), 0, 0, 0) {
		t.Fatal("wrapped distance two must meet inclusive reach two")
	}
	if nanoFootprintPad(65536+20, 99) != 807 {
		t.Fatal("footprints must narrow to signed words before distance")
	}
}
