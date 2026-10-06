package ai

import (
	"bytes"
	"fmt"
	"math"
	"testing"

	"github.com/nanolathe-gg/nanolathe/formats"
	"github.com/nanolathe-gg/nanolathe/internal/content"
	"github.com/nanolathe-gg/nanolathe/internal/mission"
	"github.com/nanolathe-gg/nanolathe/internal/sim/rng"
	"github.com/nanolathe-gg/nanolathe/vfs"
)

// classVectorFor runs the class routine over one definition and returns its
// first-pass coefficient and its class triple. The wind environment is bound to
// a still map so the net-energy query reads only the definition's own fields
// [05 R-PROD-01 §1].
func classVectorFor(t *testing.T, def *content.UnitDef) (int8, ClassVector) {
	t.Helper()
	key := content.CanonicalKey(def.UnitName)
	def.CanonicalKey = key
	s := &Strategic{Catalog: &content.Catalog{Units: map[string]*content.UnitDef{key: def}}}
	s.BindEnergyEnvironment(func() (float32, float32) { return 0, 0 })
	s.Init([]string{key})
	return s.SingleVectors[key], s.ClassVectors[key]
}

// TestWeaponScoreConsumesUnsignedDamageWord locks the consumer width and
// inactive sentinel gate [08 R-P0-05 §5]. Refresh draws only its outer gate;
// recomputing these coefficients adds no draws [08 R-P0-05 §6].
func TestWeaponScoreConsumesUnsignedDamageWord(t *testing.T) {
	for _, tc := range []struct {
		name   string
		damage int32
		want   int8
	}{
		{"wrap to zero", 65536, 7},
		{"negative wraps high", -1, 100},
		{"zero", 0, 7},
		{"one damage addend", 40, 8},
		{"unsigned maximum", 65535, 100},
	} {
		for _, active := range []bool{true, false} {
			name, id, want := "active/", int32(1), tc.want
			if !active {
				name, id, want = "inactive/", 0, 2
			}
			t.Run(name+tc.name, func(t *testing.T) {
				weapon := &content.WeaponDef{ID: id, DamageDefault: tc.damage}
				def := &content.UnitDef{UnitName: "damageword", CanonicalKey: "damageword", MinWaterDepth: -1, Weapon1Def: weapon}
				s := &Strategic{Catalog: &content.Catalog{Units: map[string]*content.UnitDef{def.CanonicalKey: def}}}
				s.BindEnergyEnvironment(func() (float32, float32) { return 0, 0 })
				s.Init([]string{def.CanonicalKey})
				if got := s.SingleVectors[def.CanonicalKey]; got != want {
					t.Fatalf("initial coefficient = %d, want %d", got, want)
				}

				// This authored state makes the refresh gate zero. Replacing
				// the stored coefficient proves that the gated body ran.
				stream := rng.SimulationFromState(30)
				probe := stream
				if draw := probe.Uint32n(30); draw != 0 {
					t.Fatalf("fixture refresh gate = %d, want zero", draw)
				}
				s.SingleVectors[def.CanonicalKey] = -100
				if !s.MaybeRefresh(30, &stream, 0, nil) {
					t.Fatal("refresh did not run")
				}
				if got := s.SingleVectors[def.CanonicalKey]; got != want {
					t.Fatalf("refreshed coefficient = %d, want %d", got, want)
				}
				if stream.State != probe.State || stream.Draws() != probe.Draws() {
					t.Fatalf("class recompute changed RNG beyond the outer gate: state/draws = %d/%d, want %d/%d", stream.State, stream.Draws(), probe.State, probe.Draws())
				}
				if weapon.DamageDefault != tc.damage {
					t.Fatalf("consumer changed authored damage to %d, want %d", weapon.DamageDefault, tc.damage)
				}
			})
		}
	}
}

// TestClassCoefficientWorkingPrecision locks the stored-single/working-wide
// boundaries [08 R-P0-05 §5]. The energy fields accept these authored fractions.
// The fractional metal costs are direct arithmetic fixtures: retail's integer
// cost loader does not produce them, and no gameplay reachability is claimed.
func TestClassCoefficientWorkingPrecision(t *testing.T) {
	for _, tc := range []struct {
		name        string
		def         content.UnitDef
		coefficient int
		want        int8
	}{
		{"authored passive energy", content.UnitDef{EnergyMake: 0.99999994}, 0, 4},
		{"authored energy usage", content.UnitDef{EnergyUse: -19.799999}, 2, 98},
		{"fractional cost arithmetic final sum", content.UnitDef{BuildCostMetal: 50.000008, ExtractsMetal: 1}, 1, 98},
		{"fractional cost arithmetic combined store", content.UnitDef{BuildCostMetal: 1000.00006, MakesMetal: 1}, 1, 4},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tc.def.UnitName = "workingprecision"
			tc.def.MinWaterDepth = -1
			_, cv := classVectorFor(t, &tc.def)
			got := [3]int8{cv.C0, cv.C1, cv.C2}[tc.coefficient]
			if got != tc.want {
				t.Fatalf("coefficient %d = %d, want %d [08 R-P0-05 §5]", tc.coefficient, got, tc.want)
			}
		})
	}
}

// TestClassExtractorUsesStoredSingle exercises loader-admitted decimal values
// that become zero in retail's definition store before all three predicates
// [08 R-P0-05 §5]. The catalog deliberately retains its parser precision.
func TestClassExtractorUsesStoredSingle(t *testing.T) {
	for _, authored := range []string{"1e-46", "-1e-46"} {
		t.Run(authored, func(t *testing.T) {
			fbi := fmt.Sprintf("[UNITINFO]{UnitName=tinyextractor;Copyright=Copyright 1997 Humongous Entertainment. All rights reserved.;ExtractsMetal=%s;MinWaterDepth=-1;}", authored)
			var archive bytes.Buffer
			if err := vfs.WriteArchive(&archive, []vfs.ArchiveFile{{Path: "units/tinyextractor.fbi", Data: []byte(fbi)}}, vfs.ArchiveWriteOptions{}); err != nil {
				t.Fatal(err)
			}
			fs := vfs.New()
			t.Cleanup(func() { _ = fs.Close() })
			if _, err := fs.MountArchiveReader("tinyextractor.ufo", bytes.NewReader(archive.Bytes()), int64(archive.Len()), 1, vfs.ArchiveOptions{}); err != nil {
				t.Fatal(err)
			}
			defs, err := content.CompileUnits(fs)
			if err != nil {
				t.Fatal(err)
			}
			def := defs["tinyextractor"]
			if def == nil || def.ExtractsMetal == 0 || float32(def.ExtractsMetal) != 0 {
				t.Fatal("fixture must retain a nonzero parser value that stores as single-precision zero")
			}
			before := def.ExtractsMetal
			single, cv := classVectorFor(t, def)
			if single != 2 || cv.C0 != 4 || cv.C1 != 0 {
				t.Fatalf("coefficients = %d/%d/%d, want 2/4/0", single, cv.C0, cv.C1)
			}
			if def.ExtractsMetal != before {
				t.Fatal("consumer changed the authored catalog")
			}
		})
	}
}

// These are direct consumer/arithmetic fixtures, not retail-authored NaN
// claims. The query-result fixture uses an explicit zero environment and an
// infinite generator to produce unordered multiplication; NaN EnergyUse would
// exercise a different, producer-owned gate [08 R-P0-05 §5].
func TestClassPredicatesOnUnorderedInputs(t *testing.T) {
	for _, tc := range []struct {
		name           string
		def            content.UnitDef
		single, c0, c1 int8
	}{
		{"extractor NaN", content.UnitDef{ExtractsMetal: math.NaN()}, 2, 4, 0},
		{"query product NaN", content.UnitDef{WindGenerator: math.Inf(1)}, 12, 100, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tc.def.UnitName = "unordered"
			tc.def.MinWaterDepth = -1
			single, cv := classVectorFor(t, &tc.def)
			if single != tc.single || cv.C0 != tc.c0 || cv.C1 != tc.c1 {
				t.Fatalf("coefficients = %d/%d/%d, want %d/%d/%d", single, cv.C0, cv.C1, tc.single, tc.c0, tc.c1)
			}
		})
	}
}

// TestFirstPassAddsTheTwoCostTerms locks [08 R-P0-05 §5]: retail multiplies each
// build cost by a negative constant and subtracts that product, so the net
// effect is +0.01 x metal cost and +0.002 x energy cost. Under the subtracting
// reading an expensive definition pins at -100 instead of +100, which is the
// defect this test exists to catch. Both directions: an expensive definition
// rails high, a cheap one stays low, and the ordering is monotone in cost.
func TestFirstPassAddsTheTwoCostTerms(t *testing.T) {
	for _, tc := range []struct {
		name   string
		metal  float32
		energy float32
		want   int8
	}{
		// acc starts at 1; an unarmed definition's weapon budget is 1.
		{"free", 0, 0, 2},
		// 400 is a multiple of 100, so the exact product lands just under 4 and
		// the working-precision sum truncates to 4, not 5
		// [08 "Arithmetic and clamping"].
		{"cheap metal", 400, 0, 5},         // trunc(4.99999991) = 4, + 1
		{"cheap energy", 0, 2000, 6},       // 1 + 4.0000002 = 5, + 1
		{"both", 400, 2000, 9},             // trunc(4.99999991) = 4, trunc(8.0000002) = 8, + 1
		{"expensive", 100000, 0, 100},      // 1 + 1000 clamps at the upper bound
		{"very expensive", 0, 500000, 100}, // 1 then +1000 clamps at the upper bound
	} {
		t.Run(tc.name, func(t *testing.T) {
			single, _ := classVectorFor(t, &content.UnitDef{
				UnitName:        "cost",
				BuildCostMetal:  tc.metal,
				BuildCostEnergy: tc.energy,
				MinWaterDepth:   -1,
			})
			if single != tc.want {
				t.Fatalf("single coefficient = %d, want %d [08 R-P0-05 §5]", single, tc.want)
			}
		})
	}
}

// TestOtherMixAccumulatorStartsAtOneAndCanAttackReplacesIt locks [08 R-P0-05 §5]:
// the accumulator's initial value is 1, not 0, and can-attack overwrites it with
// 21 rather than adding to it. Both are visible through the zero-count times-four
// multiplier: 1*4 = 4 against 21*4 = 84.
func TestOtherMixAccumulatorStartsAtOneAndCanAttackReplacesIt(t *testing.T) {
	for _, tc := range []struct {
		name      string
		canAttack bool
		want      int8
	}{
		{"plain definition starts at one", false, 4},
		{"can-attack replaces it with 21", true, 84},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, cv := classVectorFor(t, &content.UnitDef{
				UnitName:      "startvalue",
				CanAttack:     tc.canAttack,
				MinWaterDepth: -1,
			})
			if cv.C0 != tc.want {
				t.Fatalf("other-mix = %d, want %d [08 R-P0-05 §5]", cv.C0, tc.want)
			}
		})
	}
}

// TestOtherMixFoldsClampedEnergyMake locks the routine's ninth input
// [08 R-P0-05 §5]: the definition's passive `energymake`, clamped below at zero
// and above at THIRTY, added in floating point before the single truncation, so
// the zero-count times-four multiplier scales the folded sum. The upper bound is
// 30, not 100: a fusion plant authoring 1000 gains exactly 30.
func TestOtherMixFoldsClampedEnergyMake(t *testing.T) {
	for _, tc := range []struct {
		name       string
		energyMake float64
		want       int8
	}{
		{"absent", 0, 4},                   // trunc(1 + 0) * 4
		{"fractional truncates", 2.75, 12}, // trunc(1 + 2.75) = 3, * 4
		{"solar collector", 15, 64},        // trunc(1 + 15) = 16, * 4
		{"at the upper bound", 30, 100},    // trunc(1 + 30) = 31, * 4 = 124, clamped
		{"far above the bound", 1000, 100},
		{"negative clamps to zero", -50, 4},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, cv := classVectorFor(t, &content.UnitDef{
				UnitName:      "energymake",
				EnergyMake:    tc.energyMake,
				MinWaterDepth: -1,
			})
			if cv.C0 != tc.want {
				t.Fatalf("other-mix = %d, want %d [08 R-P0-05 §5]", cv.C0, tc.want)
			}
		})
	}

	// Below the upper bound the term is not saturated, so the coefficient must
	// still separate two producers. 20 and 25 differ; 30 and 1000 do not.
	_, twenty := classVectorFor(t, &content.UnitDef{UnitName: "e20", EnergyMake: 20, MinWaterDepth: -1})
	_, twentyFive := classVectorFor(t, &content.UnitDef{UnitName: "e25", EnergyMake: 25, MinWaterDepth: -1})
	if twenty.C0 == twentyFive.C0 {
		t.Fatalf("energymake 20 and 25 must differ, both %d [08 R-P0-05 §5]", twenty.C0)
	}
	_, thirty := classVectorFor(t, &content.UnitDef{UnitName: "e30", EnergyMake: 30, MinWaterDepth: -1})
	_, thousand := classVectorFor(t, &content.UnitDef{UnitName: "e1000", EnergyMake: 1000, MinWaterDepth: -1})
	if thirty.C0 != thousand.C0 {
		t.Fatalf("energymake 30 and 1000 must agree at the clamp: %d and %d [08 R-P0-05 §5]", thirty.C0, thousand.C0)
	}
}

// TestOtherMixHasNoLowerClamp locks [08 R-P0-05 §5]: `base` is clamped above at
// 100 and not below, so the half-capacity addend can leave it negative and a
// negative `base` does reach the candidate score.
//
// The absence of a lower bound at -100 is contract rather than observable
// arithmetic: the half-capacity addend's range is [-50, +50] and the value it
// joins is at least one, so the pre-store value can never fall below -100 and a
// spurious [-100, 100] clamp would agree here. What this test does forbid is
// the other plausible mistake — a lower bound at zero, matching the energy and
// metal coefficients — which would erase the negative value entirely.
func TestOtherMixHasNoLowerClamp(t *testing.T) {
	// The first-pass coefficient reaches -100 with a large negative authored
	// metal cost; half of it is -50, and the accumulator before it is 1*4 = 4.
	def := &content.UnitDef{UnitName: "nolower", BuildCostMetal: -100000, MinWaterDepth: -1}
	key := content.CanonicalKey(def.UnitName)
	def.CanonicalKey = key
	s := &Strategic{Catalog: &content.Catalog{Units: map[string]*content.UnitDef{key: def}}}
	s.Init([]string{key})
	if got := s.SingleVectors[key]; got != -100 {
		t.Fatalf("fixture: single coefficient = %d, want -100", got)
	}
	s.SetUnitLimit(10)
	s.liveUnitCount = 6
	s.recomputeClassVectors()
	if got := s.ClassVectors[key].C0; got != -46 {
		t.Fatalf("other-mix = %d, want -46 (4 + -100/2): there is no lower clamp [08 R-P0-05 §5]", got)
	}

	// The upper bound is still applied, at 100.
	_, cv := classVectorFor(t, &content.UnitDef{
		UnitName: "upper", CanAttack: true, CanFly: true, ExtractsMetal: 1,
		MakesMetal: 1, RadarDistance: 1, SonarDistance: 1, MinWaterDepth: -1,
	})
	if cv.C0 != 100 {
		t.Fatalf("other-mix = %d, want the upper clamp 100 [08 R-P0-05 §5]", cv.C0)
	}
}

// TestMetalCoefficientAddsTwentyFiveAndClampsAtZero locks [08 R-P0-05 §5]: the
// `makesmetal` term is PLUS 25 and the clamp is [0, 100] in floating point
// before the truncation, so a metal maker scores max(0, 25 - 0.02 x metal cost)
// and can never be negative. The old -25 with a [-100, 100] clamp produced
// deeply negative values, which is what this table forbids.
func TestMetalCoefficientAddsTwentyFiveAndClampsAtZero(t *testing.T) {
	for _, tc := range []struct {
		name          string
		extractsMetal float64
		makesMetal    int32
		metalCost     float32
		want          int8
	}{
		{"plain definition", 0, 0, 500, 0},         // -10 clamps up to 0
		{"maker, free", 0, 1, 0, 25},               // 25
		{"maker, cheap", 0, 1, 500, 15},            // 25 - 10
		{"maker, at the crossover", 0, 1, 1250, 0}, // 25 - 25
		{"maker, dear", 0, 1, 5000, 0},             // 25 - 100 clamps up to 0
		{"extractor, cheap", 1, 0, 500, 90},        // 100 - 10
		{"extractor, dear", 1, 0, 100000, 0},       // 100 - 2000 clamps up to 0
		{"extractor and maker", 1, 1, 0, 100},      // 125 clamps down to 100
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, cv := classVectorFor(t, &content.UnitDef{
				UnitName:       "metal",
				ExtractsMetal:  tc.extractsMetal,
				MakesMetal:     tc.makesMetal,
				BuildCostMetal: tc.metalCost,
				MinWaterDepth:  -1,
			})
			if cv.C1 != tc.want {
				t.Fatalf("metal coefficient = %d, want %d [08 R-P0-05 §5]", cv.C1, tc.want)
			}
		})
	}
}

// TestEnergyCoefficientClampsToZeroAndHundred locks [08 R-P0-05 §5]: the energy
// coefficient's clamp bounds are 0 and 100, applied in floating point before the
// single truncation, not [-100, 100] after it. A consumer therefore reads zero
// rather than a negative value.
func TestEnergyCoefficientClampsToZeroAndHundred(t *testing.T) {
	for _, tc := range []struct {
		name       string
		energyUse  float64
		energyCost float32
		want       int8
	}{
		{"neutral", 0, 0, 0},
		{"consumer clamps up to zero", 3, 514, 0}, // -1.285 - 15
		{"producer", -10, 400, 49},                // -1.0 + 50
		{"strong producer clamps at 100", -100, 0, 100},
		{"expensive definition clamps up to zero", 0, 100000, 0}, // -250
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, cv := classVectorFor(t, &content.UnitDef{
				UnitName:        "energy",
				EnergyUse:       tc.energyUse,
				BuildCostEnergy: tc.energyCost,
				MinWaterDepth:   -1,
			})
			if cv.C2 != tc.want {
				t.Fatalf("energy coefficient = %d, want %d [08 R-P0-05 §5]", cv.C2, tc.want)
			}
		})
	}
}

// TestNaNCoefficientsTakeTheUpperArm locks the unordered path of
// [08 R-P0-05 §5]: retail's upper-bound comparison is unordered-sensitive and
// NaN sets the bit that branch selects, so a NaN energy or metal coefficient
// becomes 100, not 0. These directly injected arithmetic inputs do not claim
// authored reachability: retail loads build costs through an integer accessor.
func TestNaNCoefficientsTakeTheUpperArm(t *testing.T) {
	nan := float32(math.NaN())
	_, cv := classVectorFor(t, &content.UnitDef{
		UnitName:        "nan",
		BuildCostMetal:  nan,
		BuildCostEnergy: nan,
		MinWaterDepth:   -1,
	})
	if cv.C1 != 100 {
		t.Fatalf("metal coefficient on a NaN cost = %d, want 100 [08 R-P0-05 §5]", cv.C1)
	}
	if cv.C2 != 100 {
		t.Fatalf("energy coefficient on a NaN cost = %d, want 100 [08 R-P0-05 §5]", cv.C2)
	}

	// A NaN `energymake` takes the other-mix clamp's lower arm and contributes
	// nothing, matching retail's fall-through to the literal zero.
	_, plain := classVectorFor(t, &content.UnitDef{UnitName: "plain", MinWaterDepth: -1})
	_, nanMake := classVectorFor(t, &content.UnitDef{
		UnitName: "nanmake", EnergyMake: math.NaN(), MinWaterDepth: -1,
	})
	if nanMake.C0 != plain.C0 {
		t.Fatalf("other-mix on a NaN energymake = %d, want %d [08 R-P0-05 §5]", nanMake.C0, plain.C0)
	}
}

// TestBuildOptionTermsTestListPresence locks [08 R-P0-05 §9] and
// [08 R-ENTRY-02 §2]: the `+20` initialization term and the refresh's
// build-capable count test whether the compiled build-option list EXISTS, which
// is exactly the authored `builder` flag. Retail allocates that list for every
// builder, including one whose `CANBUILD` section is absent, so a builder with
// no resolvable entries still qualifies — the case the old non-emptiness test
// dropped.
func TestBuildOptionTermsTestListPresence(t *testing.T) {
	for _, tc := range []struct {
		name    string
		builder bool
		buttons []string
		want    int8
	}{
		{"non-builder with no menu", false, nil, 40},
		{"non-builder with a menu", false, []string{"x"}, 40},
		{"builder with entries", true, []string{"x"}, 60},
		{"builder with an empty menu", true, nil, 60},
	} {
		t.Run(tc.name, func(t *testing.T) {
			key := content.CanonicalKey("bo")
			def := &content.UnitDef{
				DefinitionHeader: content.DefinitionHeader{CanonicalKey: key},
				UnitName:         "bo",
				Builder:          tc.builder,
			}
			cat := &content.Catalog{
				Units:      map[string]*content.UnitDef{key: def},
				BuildMenus: map[string]*content.BuildMenuPage{key: {Buttons: tc.buttons}},
			}
			s := &Strategic{Catalog: cat}
			s.Init([]string{key})
			if got := s.InitVectors[key]; got != tc.want {
				t.Fatalf("initialization byte = %d, want %d [08 R-P0-05 §9]", got, tc.want)
			}
		})
	}
}

// The authored map scalar and definition multiplier keep their single stores,
// but the selected query product stays wide through the class consumer
// [05 R-PROD-01 §1][08 R-P0-05 §5]. No live unit or activation gate applies.
func TestClassTidalQueryRetainsWorkingProduct(t *testing.T) {
	var archive bytes.Buffer
	fbi := "[UNITINFO]{UnitName=fractionaltide;Copyright=Copyright 1997 Humongous Entertainment. All rights reserved.;TidalGenerator=1.98;MinWaterDepth=-1;}"
	if err := vfs.WriteArchive(&archive, []vfs.ArchiveFile{{Path: "units/fractionaltide.fbi", Data: []byte(fbi)}}, vfs.ArchiveWriteOptions{}); err != nil {
		t.Fatal(err)
	}
	fs := vfs.New()
	t.Cleanup(func() { _ = fs.Close() })
	if _, err := fs.MountArchiveReader("fractionaltide.ufo", bytes.NewReader(archive.Bytes()), int64(archive.Len()), 1, vfs.ArchiveOptions{}); err != nil {
		t.Fatal(err)
	}
	defs, err := content.CompileUnits(fs)
	if err != nil {
		t.Fatal(err)
	}
	def := defs["fractionaltide"]
	if def == nil {
		t.Fatal("missing authored definition")
	}
	ota, err := formats.ParseTDF([]byte("[GlobalHeader]{tidalstrength=10;}"))
	if err != nil {
		t.Fatal(err)
	}
	tide := float32(mission.DecodeMissionGlobals(ota.Root.Section("GlobalHeader")).TidalStrength)
	s := &Strategic{Catalog: &content.Catalog{Units: defs}}
	s.BindEnergyEnvironment(func() (float32, float32) { return 0, tide })
	s.Init([]string{"fractionaltide"})
	if got := s.ClassVectors["fractionaltide"].C2; got != 99 {
		t.Fatalf("energy coefficient=%d, want 99 from unrounded query product", got)
	}
	// Refresh consumes only its established outer gate; the changed helper
	// has no draw and does not alter the live-environment access order.
	stream := rng.SimulationFromState(30)
	want := stream
	want.Uint32n(30)
	s.ClassVectors["fractionaltide"] = ClassVector{}
	if !s.MaybeRefresh(30, &stream, 0, nil) || s.ClassVectors["fractionaltide"].C2 != 99 {
		t.Fatal("refresh did not retain the authored tidal coefficient")
	}
	if stream.State != want.State || stream.Draws() != want.Draws() {
		t.Fatal("query changed RNG beyond the refresh gate")
	}
}

// Direct-state fixtures settle branch semantics without claiming a decimal
// NaN writer. The query suppresses unordered definition fields before selecting
// a generator product [05 R-PROD-01 §1].
func TestClassifyUnorderedDefinitionGates(t *testing.T) {
	for _, tc := range []struct {
		name string
		def  content.UnitDef
		want float64
	}{
		{"energy use falls through", content.UnitDef{EnergyUse: math.NaN(), WindGenerator: 8, TidalGenerator: 12}, -4},
		{"wind falls through", content.UnitDef{WindGenerator: math.NaN(), TidalGenerator: 12}, -3},
		{"tidal is rejected", content.UnitDef{TidalGenerator: math.NaN()}, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := classify(&tc.def, .5, .25); got != tc.want {
				t.Fatalf("query=%g, want %g", got, tc.want)
			}
		})
	}
}
