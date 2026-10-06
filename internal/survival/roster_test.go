package survival

import (
	"reflect"
	"testing"

	"github.com/nanolathe-gg/nanolathe/internal/content"
	"github.com/nanolathe-gg/nanolathe/internal/sim/rng"
)

func rosterCatalog() *content.Catalog {
	unit := func(key string, metal, energy float32) *content.UnitDef {
		return &content.UnitDef{DefinitionHeader: content.DefinitionHeader{CanonicalKey: key}, UnitName: key, CanMove: true, BMCode: 1, BuildCostMetal: metal, BuildCostEnergy: energy, Weapon1Def: &content.WeaponDef{ID: 1}}
	}
	return &content.Catalog{
		Units:      map[string]*content.UnitDef{"commander": {UnitName: "commander", Builder: true, Commander: true, BMCode: 1, CanMove: true}, "factory": {UnitName: "factory", Builder: true}, "a": unit("a", 100, 1000), "z": unit("z", 400, 2000), "excluded": unit("excluded", 50, 200)},
		Sides:      []*content.SideDef{{Commander: "commander"}},
		BuildMenus: map[string]*content.BuildMenuPage{"commander": {Buttons: []string{"factory"}}, "factory": {Buttons: []string{"a", "z", "excluded"}}},
	}
}

func rosterProducts(m *content.BuildMenuPage) []string {
	if m == nil {
		return nil
	}
	return m.Buttons
}

func TestScenarioPoolAbsentPreservesPoolPlansAndDraws(t *testing.T) {
	cat := rosterCatalog()
	want := BuildPool(cat, rosterProducts)
	got, err := BuildScenarioPool(cat, rosterProducts)
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("absent roster changed pool: %v", err)
	}
	for _, tick := range []uint32{0, 9000, 30000} {
		a, b := rng.NewSimulation(7), rng.NewSimulation(7)
		wa := Plan(3, tick, &want, DefaultTuning(PaceNormal), Options{}, nil, &a)
		wb := Plan(3, tick, &got, DefaultTuning(PaceNormal), Options{}, nil, &b)
		if !reflect.DeepEqual(wa, wb) || a != b {
			t.Fatal("absent roster changed plans or draws")
		}
	}
}

func TestScenarioRosterIsExclusiveSortedAndUsesExistingCosts(t *testing.T) {
	cat := rosterCatalog()
	cat.SurvivalRoster = &content.SurvivalRoster{Units: []content.SurvivalRosterEntry{{Unit: "z", Tier: 3}, {Unit: "a", Tier: 1}}}
	// No builder path is needed for roster content.
	cat.Sides = nil
	cat.BuildMenus = nil
	got, err := BuildScenarioPool(cat, func(*content.BuildMenuPage) []string { t.Fatal("roster walked build tree"); return nil })
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Units) != 2 || got.Units[0].Key != "a" || got.Units[1].Key != "z" || got.Units[0].Tier != 1 || got.MaxTier != 3 {
		t.Fatalf("pool = %+v", got)
	}
	// Ratios 5 and 10 select upper median 10; costs are metal + energy/10.
	if got.RatioE != 1000 || got.RatioM != 100 || got.Units[0].Cost != 200 || got.Units[1].Cost != 600 || got.Tier1Median != 200 {
		t.Fatalf("pricing = %+v", got)
	}
	cat.SurvivalRoster.Units[0].Unit = "missing"
	if _, err := BuildScenarioPool(cat, rosterProducts); err == nil {
		t.Fatal("invalid roster fell back to build tree")
	}
}

func TestScenarioRosterExtendsModBuildTreeAndRepricesUnion(t *testing.T) {
	cat := rosterCatalog()
	alien := *cat.Units["a"]
	alien.UnitName = "alien"
	alien.CanonicalKey = "alien"
	cat.Units["alien"] = &alien
	cat.SurvivalRoster = &content.SurvivalRoster{IncludeBuildTree: true, Units: []content.SurvivalRosterEntry{{Unit: "alien", Tier: 1}, {Unit: "z", Tier: 3}}}
	got, err := BuildScenarioPool(cat, rosterProducts)
	if err != nil {
		t.Fatal(err)
	}
	// Keep every eligible mod build product, add the non-buildable alien, and
	// override the explicit tier without duplicate entries or per-pool pricing.
	want := buildPool(cat, map[string]int{"a": 1, "excluded": 1, "alien": 1, "z": 3})
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("union mismatch: %+v", got)
	}
	for _, key := range []string{"a", "excluded", "alien", "z"} {
		found := false
		for _, u := range got.Units {
			if u.Key == key {
				found = true
			}
		}
		if !found {
			t.Fatalf("missing %s", key)
		}
	}
	cat.SurvivalRoster.Units[0].Unit = "missing"
	if _, err := BuildScenarioPool(cat, rosterProducts); err == nil {
		t.Fatal("additive roster silently skipped invalid content")
	}
}

// Every mode must open with an ordinary attacker. An authored infector is a
// Modern support pick, so a combined tier-1 pool of infectors alone would plan
// empty Modern waves that score as cleared (DESIGN_SURVIVAL §5.1). The check is
// content validation, applied identically in every mode.
func TestScenarioRosterRequiresOrdinaryOpeningAttacker(t *testing.T) {
	infectOnly := func(c *content.Catalog) {
		for _, key := range []string{"a", "excluded", "z"} {
			c.Units[key].NanolatheInfector = true
		}
	}
	// Exclusive roster: rejected while compiling, and again at entry.
	cat := rosterCatalog()
	infectOnly(cat)
	cat.SurvivalRoster = &content.SurvivalRoster{Units: []content.SurvivalRosterEntry{{Unit: "a", Tier: 1}, {Unit: "z", Tier: 3}}}
	if err := cat.ValidateSurvivalRoster(); err == nil {
		t.Fatal("exclusive infector-only opening validated")
	}
	if _, err := BuildScenarioPool(cat, rosterProducts); err == nil {
		t.Fatal("exclusive infector-only opening entered Survival")
	}
	cat.Units["a"].NanolatheInfector = false
	if _, err := BuildScenarioPool(cat, rosterProducts); err != nil {
		t.Fatalf("ordinary exclusive opening refused: %v", err)
	}

	// Build-tree roster: the explicit infector is valid content, and the
	// combined pool decides at entry.
	cat = rosterCatalog()
	infectOnly(cat)
	cat.SurvivalRoster = &content.SurvivalRoster{IncludeBuildTree: true, Units: []content.SurvivalRosterEntry{{Unit: "a", Tier: 1}}}
	if err := cat.ValidateSurvivalRoster(); err != nil {
		t.Fatalf("build-tree roster rejected before its tree was known: %v", err)
	}
	if _, err := BuildScenarioPool(cat, rosterProducts); err == nil {
		t.Fatal("combined infector-only opening entered Survival")
	}
	cat.Units["excluded"].NanolatheInfector = false
	if _, err := BuildScenarioPool(cat, rosterProducts); err != nil {
		t.Fatalf("build tree's ordinary opening refused: %v", err)
	}
}
