package session

import (
	"reflect"
	"testing"

	"github.com/nanolathe-gg/nanolathe/internal/construction"
	"github.com/nanolathe-gg/nanolathe/internal/content"
	"github.com/nanolathe-gg/nanolathe/internal/economy"
	"github.com/nanolathe-gg/nanolathe/internal/survival"
)

func TestSurvivalRosterInEveryModeAndAttackerOwner(t *testing.T) {
	for _, rules := range []RuleSet{StrictRuleSet(), ModernRuleSet()} {
		t.Run(rules.Name, func(t *testing.T) {
			cat := &content.Catalog{Units: map[string]*content.UnitDef{"alien": {DefinitionHeader: content.DefinitionHeader{CanonicalKey: "alien"}, UnitName: "alien", CanMove: true, BMCode: 1, BuildCostMetal: 100, Weapon1Def: &content.WeaponDef{ID: 1}}}, SurvivalRoster: &content.SurvivalRoster{Units: []content.SurvivalRosterEntry{{Unit: "alien", Tier: 1}}}}
			s := &Session{Catalog: cat, Build: &construction.Service{}, Econ: &economy.Service{}}
			s.BindRules(rules)
			s.Econ.Players[0].Stock = [2]float32{31, 42}
			sim, crt, stock := *s.SimRNG(), *s.CrtRNG(), s.Econ.Players[0].Stock
			cfg := SurvivalConfigFor("fixture", []SkirmishPlayer{{Color: 1}, {Color: 0}}, SurvivalOptions{})
			if err := s.initSurvival(cfg); err != nil {
				t.Fatal(err)
			}
			if owner, ok := s.SurvivalAttacker(); !ok || owner != 2 {
				t.Fatalf("attacker %d,%v", owner, ok)
			}
			if len(s.Survival.pool.Units) != 1 || s.Survival.pool.Units[0].Key != "alien" {
				t.Fatal("roster not selected")
			}
			if *s.SimRNG() != sim || *s.CrtRNG() != crt || s.Econ.Players[0].Stock != stock {
				t.Fatal("roster setup spent RNG or resources")
			}
		})
	}
}

func TestSurvivalRosterOutsideScenarioAndFailureAreInert(t *testing.T) {
	s := &Session{Catalog: &content.Catalog{SurvivalRoster: &content.SurvivalRoster{Units: []content.SurvivalRosterEntry{{Unit: "missing", Tier: 1}}}}}
	if err := s.initSurvival(SkirmishConfig{}); err != nil {
		t.Fatal(err)
	}
	if _, ok := s.SurvivalAttacker(); ok {
		t.Fatal("ordinary session has attacker")
	}
	if err := s.initSurvival(SurvivalSkirmishConfig("fixture", 0, SurvivalOptions{})); err == nil {
		t.Fatal("invalid roster accepted")
	}
	if s.Survival != nil {
		t.Fatal("failed setup published director")
	}
	var nilSession *Session
	if _, ok := nilSession.SurvivalAttacker(); ok {
		t.Fatal("nil session has attacker")
	}
}

func TestSurvivalAbsentRosterUsesBoundBuildProducts(t *testing.T) {
	cat := minimalCatalogForStrict()
	cat.Units["armcom"].Builder = true
	cat.Units["armcom"].CanMove = true
	cat.Units["factory"] = &content.UnitDef{UnitName: "factory", Builder: true}
	cat.Units["soldier"] = &content.UnitDef{DefinitionHeader: content.DefinitionHeader{CanonicalKey: "soldier"}, UnitName: "soldier", CanMove: true, BMCode: 1, BuildCostMetal: 10, Weapon1Def: &content.WeaponDef{ID: 1}}
	cat.BuildMenus = map[string]*content.BuildMenuPage{"armcom": {Buttons: []string{"factory"}}, "factory": {Buttons: []string{"soldier"}}}
	s := &Session{Catalog: cat}
	if err := s.initSurvival(SurvivalSkirmishConfig("fixture", 0, SurvivalOptions{})); err != nil {
		t.Fatal(err)
	}
	want := survival.BuildPool(cat, func(m *content.BuildMenuPage) []string { return construction.BuildProducts(nil, m) })
	if !reflect.DeepEqual(s.Survival.pool, want) {
		t.Fatal("default director pool changed")
	}
}
