package community

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

// TestMainlineTableIdentity locks the one table the engine carries. Its
// digest predates the removal of the per-mod tables, so it proves the
// mainline value did not move when they left (DESIGN_COMMUNITY_PATCH §3.3).
func TestMainlineTableIdentity(t *testing.T) {
	prota, err := Table(Mainline)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := prota.Digest(), "936c51eec9338d1ea3e8edcfd27682bf423644433a8224d384730e350de0ca81"; got != want {
		t.Fatalf("Table(%q).Digest() = %s, want %s", Mainline, got, want)
	}
	if !prota.AreaDamageOverflow || !prota.GridClaimTieBreak || prota.OffMapAircraftMarginTiles != 1 {
		t.Fatalf("prota matrix flags = %+v", prota)
	}
	if prota.ProjectileCapacity != 3000 || prota.ExplosionCapacity != 3000 || prota.DebrisCapacity != 1000 {
		t.Fatalf("prota pool capacities = %d/%d/%d", prota.ProjectileCapacity, prota.ExplosionCapacity, prota.DebrisCapacity)
	}
	if prota.PathStepAllowance != 66650 || prota.UnitLimit != 1500 {
		t.Fatalf("prota preference defaults = path %d, units %d", prota.PathStepAllowance, prota.UnitLimit)
	}
	// The shipped preference file's SfxLimit, not the code's absent-key
	// default of 16000 (community-patch-engine.md §4.1, CP-LIM-2).
	if prota.SfxLimit != 20480 {
		t.Fatalf("prota SfxLimit = %d, want the shipped 20480", prota.SfxLimit)
	}
	// A mod's table is its own config now; the engine names no other.
	for _, name := range []string{"escalation", "tazero", "mayhem", "ota", "bta", "twilight", ""} {
		if _, err := Table(name); err == nil {
			t.Errorf("Table(%q) resolved; only the mainline table is carried", name)
		}
	}
}

func TestResolvePrecedenceAndStrictIdentity(t *testing.T) {
	falseValue := false
	zero := 0
	configuredLimit := 1000
	authored := mainline
	authored.AirCorpseFall = true
	authored.RepairRate = RepairRate{Enabled: true, RepairMultiplier: 3, SelfHealMultiplier: 3}
	got, err := Resolve(false,
		Overrides{Base: &authored, UnitLimit: &configuredLimit},
		Overrides{AreaDamageOverflow: &falseValue, UnitLimit: &zero},
	)
	if err != nil {
		t.Fatal(err)
	}
	if got.AreaDamageOverflow || got.UnitLimit != 0 {
		t.Fatalf("later explicit false/zero did not win: %+v", got)
	}
	if !got.AirCorpseFall || !got.RepairRate.Enabled {
		t.Fatalf("base replacement did not preserve the authored values: %+v", got)
	}
	// A later table starts again from the mainline value, discarding the
	// base, exactly as a later base discards an earlier table.
	if got, err := Resolve(false, Overrides{Base: &authored}, Overrides{Table: Mainline}); err != nil || got != mainline {
		t.Fatalf("a later table = %+v, %v; want the mainline table", got, err)
	}
	if _, err := Resolve(false, Overrides{Table: Mainline, Base: &authored}); err == nil {
		t.Fatal("a source naming both a table and a base resolved")
	}

	strict, err := Resolve(true, Overrides{Table: "does-not-exist", UnitLimit: &configuredLimit})
	if err != nil {
		t.Fatalf("Strict source was not ignored: %v", err)
	}
	if strict != (Features{}) {
		t.Fatalf("Strict features = %+v, want zero", strict)
	}
}

func TestOverridesJSONPreservesFalseAndZero(t *testing.T) {
	var got Overrides
	if err := json.Unmarshal([]byte(`{"areaDamageOverflow":false,"unitLimit":0,"repairRate":{"enabled":false}}`), &got); err != nil {
		t.Fatal(err)
	}
	if got.AreaDamageOverflow == nil || *got.AreaDamageOverflow || got.UnitLimit == nil || *got.UnitLimit != 0 {
		t.Fatalf("decoded pointers = %+v", got)
	}
	if got.RepairRate == nil || got.RepairRate.Enabled == nil || *got.RepairRate.Enabled {
		t.Fatalf("decoded repair override = %+v", got.RepairRate)
	}
	for _, raw := range []string{
		`{"areaDamageOverflwo":true}`,
		`{"repairRate":{"multipler":3}}`,
		`{} {}`,
	} {
		if err := json.Unmarshal([]byte(raw), &got); err == nil {
			t.Errorf("json.Unmarshal(%q) succeeded", raw)
		}
	}
}

func TestSnapRadiiClampToSelectedTableMaxima(t *testing.T) {
	nine := 9
	for _, tc := range []struct {
		name               string
		mexMax, wreckMax   int
		wantMex, wantWreck int
	}{
		{"mainline", 3, 1, 3, 1},
		{"mex snap off", 0, 1, 0, 1},
		{"both one", 1, 1, 1, 1},
		{"both off", 0, 0, 0, 0},
	} {
		base := mainline
		base.MexSnapRadius, base.MexSnapRadiusMax = tc.mexMax, tc.mexMax
		base.WreckSnapRadius, base.WreckSnapRadiusMax = tc.wreckMax, tc.wreckMax
		got, err := Resolve(false, Overrides{Base: &base}, Overrides{
			MexSnapRadius:   &nine,
			WreckSnapRadius: &nine,
		})
		if err != nil {
			t.Fatalf("Resolve(%q): %v", tc.name, err)
		}
		if got.MexSnapRadius != tc.wantMex || got.WreckSnapRadius != tc.wantWreck {
			t.Errorf("Resolve(%q) snap radii = %d/%d, want %d/%d", tc.name, got.MexSnapRadius, got.WreckSnapRadius, tc.wantMex, tc.wantWreck)
		}
	}
}

func TestParseOverrideAndValidation(t *testing.T) {
	parsed, err := ParseOverride("repairRate.repairMultiplier=3")
	if err != nil || parsed.RepairRate == nil || parsed.RepairRate.RepairMultiplier == nil || *parsed.RepairRate.RepairMultiplier != 3 {
		t.Fatalf("ParseOverride repair multiplier = %+v, %v", parsed, err)
	}
	parsed, err = ParseOverride("areaDamageOverflow=false")
	if err != nil || parsed.AreaDamageOverflow == nil || *parsed.AreaDamageOverflow {
		t.Fatalf("ParseOverride explicit false = %+v, %v", parsed, err)
	}
	parsed, err = ParseOverride("sfxLimit=400")
	if err != nil || parsed.SfxLimit == nil || *parsed.SfxLimit != 400 {
		t.Fatalf("ParseOverride sfxLimit = %+v, %v", parsed, err)
	}
	for _, malformed := range []string{
		"missing-equals",
		"unknown=true",
		"areaDamageOverflow=1",
		"unitLimit=twenty",
		"unitLimit=19",
		"repairRate.repairMultiplier=101",
		"pathStepAllowance=-1",
		"pathStepAllowance=2147483648",
		"projectileCapacity=32768",
		"explosionCapacity=65536",
		"sfxLimit=-1",
		"sfxLimit=214748365",
		"table=unknown",
		"table=",
	} {
		if _, err := ParseOverride(malformed); err == nil {
			t.Errorf("ParseOverride(%q) succeeded", malformed)
		}
	}

	tooHigh := 101
	if _, err := Resolve(false, Overrides{RepairRate: &RepairRateOverrides{RepairMultiplier: &tooHigh}}); err == nil {
		t.Error("repair multiplier above source bound succeeded")
	}
	tooFewUnits := 19
	if _, err := Resolve(false, Overrides{UnitLimit: &tooFewUnits}); err == nil {
		t.Error("unit limit below approved settings bound succeeded")
	}
	negative := -1
	if _, err := Resolve(false, Overrides{PathStepAllowance: &negative}); err == nil {
		t.Error("negative path allowance succeeded")
	}
}

func TestResolveDefaultsToMainlineAndDigestStable(t *testing.T) {
	got, err := Resolve(false)
	if err != nil {
		t.Fatal(err)
	}
	want, _ := Table(Mainline)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Resolve(false) = %+v, want mainline %+v", got, want)
	}
	const wantDigest = "936c51eec9338d1ea3e8edcfd27682bf423644433a8224d384730e350de0ca81"
	firstDigest := got.Digest()
	secondDigest := got.Digest()
	if firstDigest != secondDigest {
		t.Fatal("digest changed between calls")
	}
	if firstDigest != wantDigest {
		t.Fatalf("mainline digest = %s, want %s", firstDigest, wantDigest)
	}
}

// TestProTAPackageSwitchesOffInEveryTableAndOverridable locks the selection
// policy of DESIGN_COMMUNITY_PATCH §4.7: the ProTA 4.8 package switches are
// false in the mainline table, so retail content keeps its AI under
// Community 3.9 and Modern, and they are reachable only through a gameplay
// source, which Strict ignores.
func TestProTAPackageSwitchesOffInEveryTableAndOverridable(t *testing.T) {
	f, err := Table(Mainline)
	if err != nil {
		t.Fatal(err)
	}
	if f.AIDifficultyIncome || f.AIStockpileProducts || f.TargetLockRelease || f.AIApplianceEnergy || f.AIBuilderStopThreshold ||
		f.WorkingWeaponsAutonomous || f.AttackSingleSlotTake || f.MapFeatureOwnerEleven || f.ResurrectionTextFix {
		t.Fatalf("the mainline table enables a ProTA package switch: %+v", f)
	}
	var profile Overrides
	if err := json.Unmarshal([]byte(`{"table":"prota","aiDifficultyIncome":true,"aiStockpileProducts":true,"targetLockRelease":true,"aiApplianceEnergy":true,"aiBuilderStopThreshold":true}`), &profile); err != nil {
		t.Fatal(err)
	}
	got, err := Resolve(false, profile)
	if err != nil {
		t.Fatal(err)
	}
	if !got.AIDifficultyIncome || !got.AIStockpileProducts || !got.TargetLockRelease || !got.AIApplianceEnergy || !got.AIBuilderStopThreshold {
		t.Fatalf("a content source did not enable the package switches: %+v", got)
	}
	mainline, _ := Table(Mainline)
	if got.Digest() == mainline.Digest() {
		t.Fatal("enabled package switches did not enter the digest")
	}
	off, err := ParseOverride("aiDifficultyIncome=false")
	if err != nil {
		t.Fatal(err)
	}
	if got, _ = Resolve(false, profile, off); got.AIDifficultyIncome || !got.AIStockpileProducts {
		t.Fatalf("command-line override did not win field by field: %+v", got)
	}
	if strict, _ := Resolve(true, profile); strict != (Features{}) {
		t.Fatalf("Strict resolved package switches: %+v", strict)
	}
}

// ParseFeatures is how a mod config's communityFeatures becomes a value
// (docs/DESIGN_MODS_MUTATORS.md §4.2): decoded over the mainline table,
// closed, bounded, and the one place a snap maximum may be declared.
func TestParseFeatures(t *testing.T) {
	got, err := ParseFeatures([]byte(`{"airCorpseFall":true,"mexSnap":false,"mexSnapRadius":0,"mexSnapRadiusMax":0,"repairRate":{"enabled":true}}`), "<test>")
	if err != nil {
		t.Fatal(err)
	}
	want := mainline
	want.AirCorpseFall, want.MexSnap, want.MexSnapRadius, want.MexSnapRadiusMax = true, false, 0, 0
	want.RepairRate.Enabled = true
	if got != want {
		t.Fatalf("ParseFeatures = %+v, want the mainline value with the four fields changed", got)
	}
	if got, err := ParseFeatures([]byte(`{}`), "<test>"); err != nil || got != mainline {
		t.Fatalf("an empty document = %+v, %v; want the mainline table", got, err)
	}
	for name, body := range map[string]string{
		"unknown key":      `{"airCorpseFal":true}`,
		"trailing value":   `{} {}`,
		"multiplier bound": `{"repairRate":{"repairMultiplier":0}}`,
		"unit limit bound": `{"unitLimit":19}`,
		"radius over max":  `{"mexSnapRadius":4}`,
		"negative max":     `{"wreckSnapRadius":0,"wreckSnapRadiusMax":-1}`,
		"table key":        `{"table":"prota"}`,
	} {
		if _, err := ParseFeatures([]byte(body), "<test>"); err == nil || !strings.Contains(err.Error(), "logical path <test>") {
			t.Errorf("%s: ParseFeatures(%s) = %v, want a refusal naming the document", name, body, err)
		}
	}
	// A base value carries its maxima through resolution, and a later
	// field-level radius is clamped to them, never past them.
	five := 5
	resolved, err := Resolve(false, Overrides{Base: &got}, Overrides{MexSnapRadius: &five})
	if err != nil || resolved.MexSnapRadius != 0 || resolved.MexSnapRadiusMax != 0 {
		t.Fatalf("base maxima through resolution = %+v, %v", resolved, err)
	}
	bad := mainline
	bad.RepairRate.RepairMultiplier = 0
	if _, err := Resolve(false, Overrides{Base: &bad}); err == nil {
		t.Fatal("an out-of-bounds base resolved")
	}
	var decoded Overrides
	if err := json.Unmarshal([]byte(`{"base":{"airCorpseFall":true}}`), &decoded); err != nil || decoded.Base == nil || !decoded.Base.AirCorpseFall {
		t.Fatalf("a base source did not survive a JSON round trip (a save sidecar): %+v, %v", decoded, err)
	}
}
