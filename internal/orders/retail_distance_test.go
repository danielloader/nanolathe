package orders

import (
	"testing"

	"github.com/nanolathe-gg/nanolathe/internal/content"
	"github.com/nanolathe-gg/nanolathe/internal/sim/numeric"
	"github.com/nanolathe-gg/nanolathe/internal/units"
)

// The distance routine rounds just below 101 for (20, 99); its truncation
// must precede the inclusive leash comparison [01 R-DET-01 §7][04 R-STANCE-01 §4].
func TestLeashUsesRetailDistanceBeforeComparison(t *testing.T) {
	u := &units.Unit{X: numeric.FixedFromInt(20), Z: numeric.FixedFromInt(99)}
	n := &Node{Param3: 101}
	if leashBroken(u, n) {
		t.Fatal("rounded distance 100 must stay inside leash 101")
	}
	n.Param3 = 100
	if !leashBroken(u, n) {
		t.Fatal("distance equal to leash must abandon")
	}
	n.Param3 = 0
	if leashBroken(u, n) {
		t.Fatal("zero leash must stay unlimited")
	}
}

// Both distance truncations occur after the helper: before the signed high
// word read for centres, and after multiplication for pads [05 R-WORK-01 §2].
func TestBuildRangeUsesRetailDistanceAtIntegerBoundary(t *testing.T) {
	builder := &units.Unit{Def: &content.UnitDef{BuildDistance: 100}}
	if !inBuildRange(builder, numeric.FixedFromInt(20), numeric.FixedFromInt(99), 0, 0) {
		t.Fatal("raw distance just below 101 world units must read whole distance 100")
	}
	builder.Def.BuildDistance = 99
	if inBuildRange(builder, numeric.FixedFromInt(20), numeric.FixedFromInt(99), 0, 0) {
		t.Fatal("whole distance 100 must exceed reach 99")
	}
	if got := footprintPad(20, 99); got != 807 {
		t.Fatalf("scaled pad = %d, want 807 after multiplying the helper result by eight", got)
	}
}

// The stored high words are signed even where the host's fixed type is wider,
// and a nonzero negative leash participates in the signed compare.
func TestLeashSignedWords(t *testing.T) {
	u := &units.Unit{X: numeric.FixedFromInt(32768)}
	n := &Node{GuardX: -32768, Param3: 1}
	if leashBroken(u, n) {
		t.Fatal("matching signed high words must have distance zero")
	}
	n.Param3 = ^uint32(0)
	if !leashBroken(u, n) {
		t.Fatal("zero distance must reach a signed negative leash")
	}
}

func TestBuildRangeWrapsRawDeltasBeforeDistance(t *testing.T) {
	builder := &units.Unit{X: numeric.FixedFromInt(32767), Def: &content.UnitDef{BuildDistance: 1}}
	if inBuildRange(builder, numeric.FixedFromInt(-32767), 0, 0, 0) {
		t.Fatal("wrapped delta has magnitude two world units, outside reach one")
	}
	builder.Def.BuildDistance = 2
	if !inBuildRange(builder, numeric.FixedFromInt(-32767), 0, 0, 0) {
		t.Fatal("wrapped delta must meet the inclusive reach of two")
	}
	if footprintPad(65536+20, 99) != 807 {
		t.Fatal("footprints must narrow to signed words before distance")
	}
}
