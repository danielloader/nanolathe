package content

import (
	"strings"
	"testing"

	"github.com/nanolathe-gg/nanolathe/internal/cob"
	"github.com/nanolathe-gg/nanolathe/internal/sim/checkpoint"
	"github.com/nanolathe-gg/nanolathe/vfs"
)

// M3-C3: equal names do not collapse record identity, and a foreign object
// cannot claim an admitted key merely by carrying that name or its Hash.
func TestCheckpointKeysPreserveAdmittedRecordIdentity(t *testing.T) {
	f := newFrozenFixture(t)
	cat := f.catalog(t)
	original := cat.unitRecords[1]
	duplicate := *original
	duplicate.UnitDefID = 3
	cat.unitRecords = append(cat.unitRecords, &duplicate)
	weapon := &WeaponDef{DefinitionHeader: DefinitionHeader{CanonicalKey: "pulse", Hash: "authored-pulse"}, ID: 7, ReloadTime: 4}
	cat.Weapons = map[string]*WeaponDef{"pulse": weapon}
	inputs := f.freeze(t, SimulationInputRequest{Catalog: cat})
	keys, err := inputs.CheckpointKeys()
	if err != nil {
		t.Fatal(err)
	}
	check := func(got checkpoint.Definition, err error, want checkpoint.Definition) {
		t.Helper()
		if err != nil || got != want {
			t.Fatalf("reference = %+v, %v; want %+v", got, err, want)
		}
	}
	u, err := keys.Unit(original)
	check(u, err, checkpoint.Definition{Family: 1, Ordinal: 2, Key: "unit/testunit"})
	u, err = keys.Unit(&duplicate)
	check(u, err, checkpoint.Definition{Family: 1, Ordinal: 3, Key: "unit/testunit"})
	p, err := keys.ProgramForUnit(&duplicate, duplicate.Script)
	check(p, err, checkpoint.Definition{Family: 2, Ordinal: 3, Key: "unit/testunit"})
	w, err := keys.Weapon(weapon)
	check(w, err, checkpoint.Definition{Family: 1, Ordinal: 7, Key: "weapon/pulse"})
	tree := cat.Features["tree1"]
	ref, err := keys.Feature(tree)
	check(ref, err, checkpoint.Definition{Family: 1, Key: "feature/tree1"})
	mdl, ok := inputs.Model("fixture")
	if !ok {
		t.Fatal("fixture model absent")
	}
	ref, err = keys.Model(mdl)
	check(ref, err, checkpoint.Definition{Family: 3, Key: "objects3d/fixture.3do"})

	foreignUnit, foreignWeapon, foreignFeature, foreignModel := *original, *weapon, *tree, *mdl
	for name, resolve := range map[string]func() (checkpoint.Definition, error){
		"unit":    func() (checkpoint.Definition, error) { return keys.Unit(&foreignUnit) },
		"weapon":  func() (checkpoint.Definition, error) { return keys.Weapon(&foreignWeapon) },
		"feature": func() (checkpoint.Definition, error) { return keys.Feature(&foreignFeature) },
		"model":   func() (checkpoint.Definition, error) { return keys.Model(&foreignModel) },
		"program owner": func() (checkpoint.Definition, error) {
			return keys.ProgramForUnit(&foreignUnit, original.Script)
		},
	} {
		t.Run(name, func(t *testing.T) {
			if ref, err := resolve(); err == nil || ref != (checkpoint.Definition{}) {
				t.Fatalf("foreign object resolved: %+v, %v", ref, err)
			}
		})
	}
	// The result is a detached value; callers cannot rewrite a retained key.
	u.Key = "changed"
	again, err := keys.Unit(&duplicate)
	check(again, err, checkpoint.Definition{Family: 1, Ordinal: 3, Key: "unit/testunit"})
}

// The definition loader preserves an existing extension, while the strict
// binder appends its extension. This authored fixture reaches the real frozen
// COB fallback path without inventing a manifest entry.
func TestCheckpointProgramValidatesFrozenFallbackWithoutReadingFiles(t *testing.T) {
	f := newFrozenFixture(t)
	cat := f.catalog(t)
	unit := *cat.unitRecords[1]
	unit.CanonicalKey, unit.UnitName, unit.Script = "fallback", "fallback.cob", nil
	cat.unitRecords = []*UnitDef{&unit}
	cat.Units = firstUnitNames(cat.unitRecords)
	programBytes := frozenFixtureCOB([]uint32{frozenFixtureCreate}, []string{"Create"}, []uint32{0}, []string{"base"})
	f.write(t, "scripts/fallback.cob.cob", programBytes)
	f.fs = mountFrozenFixture(t, f.root)
	inputs := f.freeze(t, SimulationInputRequest{Catalog: cat})
	entry, ok := manifestEntry(inputs, SimulationFamilyCOB, "unit/fallback")
	if !ok || entry.Presence != SimulationInputFallback {
		t.Fatalf("expected admitted fallback: %+v", entry)
	}
	first, err := cob.Load(programBytes)
	if err != nil {
		t.Fatal(err)
	}
	second, err := cob.Load(programBytes)
	if err != nil || first == second {
		t.Fatalf("independent program load: %v", err)
	}
	f.write(t, "scripts/fallback.cob.cob", []byte("changed after admission"))
	// Even a broken live provider cannot affect construction/lookup of keys.
	// Neither routine may call the frozen filesystem either.
	inputs.view = nil
	if _, err := inputs.CheckpointKeys(); err == nil {
		t.Fatal("incomplete frozen inputs accepted")
	}
	// A non-nil wrapper whose promoted methods panic if any filesystem
	// operation is attempted. Reference validation must never reach them.
	inputs.view = struct{ vfs.FSOps }{}
	keys, err := inputs.CheckpointKeys()
	if err != nil {
		t.Fatal(err)
	}
	want := checkpoint.Definition{Family: 2, Ordinal: 1, Key: "unit/fallback"}
	for _, program := range []*cob.Program{first, second} {
		if got, err := keys.ProgramForUnit(&unit, program); err != nil || got != want {
			t.Fatalf("fallback program = %+v, %v; want %+v", got, err, want)
		}
	}
	second.Code[0]++
	if _, err := keys.ProgramForUnit(&unit, second); err == nil {
		t.Fatal("changed program with old source checksum accepted")
	}
}

// M3-C3: two different records cannot be represented by one ambiguous pointer.
func TestCheckpointKeysRejectAmbiguousAndChangedAdmissions(t *testing.T) {
	t.Run("aliased records", func(t *testing.T) {
		f := newFrozenFixture(t)
		cat := f.catalog(t)
		cat.unitRecords = append(cat.unitRecords, cat.unitRecords[1])
		inputs := f.freeze(t, SimulationInputRequest{Catalog: cat})
		if _, err := inputs.CheckpointKeys(); err == nil || !strings.Contains(err.Error(), "unambiguous") {
			t.Fatalf("ambiguous records accepted: %v", err)
		}
	})
	t.Run("changed catalog", func(t *testing.T) {
		inputs := newFrozenFixture(t).freeze(t, SimulationInputRequest{})
		inputs.catalog.unitRecords[0].Limit++
		if _, err := inputs.CheckpointKeys(); err == nil {
			t.Fatal("changed unit semantics accepted under old admission")
		}
	})
	t.Run("changed model", func(t *testing.T) {
		inputs := newFrozenFixture(t).freeze(t, SimulationInputRequest{})
		mdl, _ := inputs.Model("fixture")
		mdl.Pieces[0].Vertices[0][0]++
		if _, err := inputs.CheckpointKeys(); err == nil {
			t.Fatal("changed model semantics accepted under old admission")
		}
	})
}

// Normalization is conditional on malformed identity/footprints. In
// particular it must not clamp negative pools on an otherwise valid record.
func TestCheckpointFeatureNormalizationPreservesItsExactBoundary(t *testing.T) {
	f := newFrozenFixture(t)
	cat := f.catalog(t)
	base := &FeatureDef{
		DefinitionHeader: DefinitionHeader{CanonicalKey: "malformed", Hash: "authored-malformed"},
		FootprintX:       0, FootprintZ: -2, Damage: -3, Metal: -4, Energy: 9,
		Blocking: true, FeatureDeadDef: cat.Features["tree1"], Unknown: map[string]string{"unused": "value"},
	}
	good := &FeatureDef{DefinitionHeader: DefinitionHeader{CanonicalKey: "valid"}, FootprintX: 2, FootprintZ: 3, Metal: -4}
	cat.Features["malformed"], cat.Features["valid"] = base, good
	inputs := f.freeze(t, SimulationInputRequest{Catalog: cat})
	keys, err := inputs.CheckpointKeys()
	if err != nil {
		t.Fatal(err)
	}
	fixed := *base
	fixed.FootprintX, fixed.FootprintZ, fixed.Damage, fixed.Metal = 1, 1, 0, 0
	// Provenance remains local; it is neither admission identity nor behavior.
	fixed.Provenance.ProviderID = "another-host"
	got, err := keys.NormalizedFeature(base, &fixed)
	want := CheckpointFeature{Variant: 2, Base: checkpoint.Definition{Family: 1, Key: "feature/malformed"}, FootprintX: 1, FootprintZ: 1, Energy: 9}
	if err != nil || got != want {
		t.Fatalf("normalized reference = %+v, %v; want %+v", got, err, want)
	}
	if base.FootprintX != 0 || base.Metal != -4 || fixed.Metal != 0 {
		t.Fatal("reference validation mutated definitions")
	}
	if _, err := keys.Feature(&fixed); err == nil {
		t.Fatal("normalized copy resolved without its base relation")
	}
	for name, edit := range map[string]func(*FeatureDef){
		"wrong clamp":        func(v *FeatureDef) { v.Damage = 1 },
		"other scalar":       func(v *FeatureDef) { v.Blocking = false },
		"sequence":           func(v *FeatureDef) { v.SeqNameBurn = "other" },
		"successor identity": func(v *FeatureDef) { copy := *v.FeatureDeadDef; v.FeatureDeadDef = &copy },
		"unknown keys":       func(v *FeatureDef) { v.Unknown = map[string]string{"unused": "changed"} },
	} {
		t.Run(name, func(t *testing.T) {
			changed := fixed
			edit(&changed)
			if _, err := keys.NormalizedFeature(base, &changed); err == nil {
				t.Fatal("change outside the existing normalization accepted")
			}
		})
	}
	got, err = keys.NormalizedFeature(good, good)
	if err != nil || got.Variant != 1 || good.Metal != -4 {
		t.Fatalf("well-formed base should stay unchanged: %+v, %v", got, err)
	}
	clamped := *good
	clamped.Metal = 0
	if _, err := keys.NormalizedFeature(good, &clamped); err == nil {
		t.Fatal("resource clamp without malformed gate accepted")
	}
	if _, err := keys.NormalizedFeature(base, nil); err == nil {
		t.Fatal("nil normalized value accepted")
	}
}

func TestCheckpointKeysRejectMissingObjects(t *testing.T) {
	var inputs *SimulationInputs
	if _, err := inputs.CheckpointKeys(); err == nil {
		t.Fatal("nil inputs accepted")
	}
	var keys *CheckpointKeys
	for _, resolve := range []func() (checkpoint.Definition, error){
		func() (checkpoint.Definition, error) { return keys.Unit(nil) },
		func() (checkpoint.Definition, error) { return keys.Weapon(nil) },
		func() (checkpoint.Definition, error) { return keys.Feature(nil) },
		func() (checkpoint.Definition, error) { return keys.Model(nil) },
		func() (checkpoint.Definition, error) { return keys.ProgramForUnit(nil, nil) },
	} {
		if got, err := resolve(); err == nil || got != (checkpoint.Definition{}) {
			t.Fatalf("nil resolver/object accepted: %+v, %v", got, err)
		}
	}
	if _, err := keys.NormalizedFeature(nil, nil); err == nil {
		t.Fatal("nil base accepted")
	}
}
