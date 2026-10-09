package session

import (
	"testing"

	"github.com/nanolathe-gg/nanolathe/internal/content"
)

// Same content and effective Community words do not prove which selected set
// prepared the catalog. Independently supplied freeze metadata must agree.
func TestMatchAdmissionChecksPreparationIdentity(t *testing.T) {
	f := newAdmitFixture(t)
	config := admitConfig(t, DirectSkirmishConfig(admitFixtureMap), SkirmishEntryOptions{}, 0)
	valid := f.freeze(t, config)
	name, base := valid.PreparingRule()
	if name != config.request.RuleName || base != string(RuleSetForMode(DirectSkirmishConfig(admitFixtureMap).Gameplay).Base) {
		t.Fatalf("entry recorded preparing rule %q (%q)", name, base)
	}
	ota, tnt := valid.MapFiles()
	for _, tc := range []struct{ name, rule, base string }{
		{"absent", "", ""}, {"other name", "other-modern-set", base}, {"other base", name, "strict-3.1"},
	} {
		sources, err := content.CaptureSimulationSources(f.fs, admitFixtureMap)
		if err != nil {
			t.Fatal(err)
		}
		inputs, err := content.FreezeSimulationInputs(sources, content.SimulationInputRequest{
			Catalog: valid.Catalog(), MapOTA: ota, MapTNT: tnt, MapSchema: valid.MapSchema(),
			CommunityDigest: valid.CommunityDigest(), PreparingRuleName: tc.rule, PreparingRuleBase: tc.base,
		})
		if err != nil {
			t.Fatal(err)
		}
		requireAdmissionKinds(t, tc.name, ValidateMatchInputs(config, inputs), ErrMatchRulesMismatch)
	}
}

func TestMatchAdmissionChecksClonedAndMissingLimits(t *testing.T) {
	f := newAdmitFixture(t)
	config := admitConfig(t, DirectSkirmishConfig(admitFixtureMap), SkirmishEntryOptions{
		Mutators: content.Mutators{Income: content.Factor{Num: 3, Den: 2}},
	}, 0)
	inputs := f.freeze(t, config)
	if inputs.Catalog() == f.cat || inputs.Catalog().Limits != f.cat.Limits {
		t.Fatal("prepared clone did not preserve compile limits")
	}
	if err := ValidateMatchInputs(config, inputs); err != nil {
		t.Fatal(err)
	}
	r := config.Request()
	r.ContentProfile.Weapons++
	requireAdmissionKinds(t, "different limits after preparation", ValidateMatchInputs(resolveMatch(t, r), inputs), ErrMatchContentMismatch)
	// An authored catalog without compile provenance must not use the old
	// Units==0 bypass. This is a distinct freeze, not mutation of frozen data.
	missing := newAdmitFixture(t)
	missing.cat.Limits = content.Limits{}
	missingInputs := missing.freeze(t, config)
	requireAdmissionKinds(t, "absent compile limits", ValidateMatchInputs(config, missingInputs), ErrMatchContentMismatch)
}

// The initial online configuration cannot quietly admit content which was
// already restricted through the single-player preparation path.
func TestMatchAdmissionRefusesFrozenRestrictions(t *testing.T) {
	f := newAdmitFixture(t)
	config := admitConfig(t, DirectSkirmishConfig(admitFixtureMap), SkirmishEntryOptions{}, 0)
	valid := f.freeze(t, config)
	ota, tnt := valid.MapFiles()
	name, base := valid.PreparingRule()
	sources, err := content.CaptureSimulationSources(f.fs, admitFixtureMap)
	if err != nil {
		t.Fatal(err)
	}
	var restrictions content.Restrictions
	if err := restrictions.Set("authored-unit", 1); err != nil {
		t.Fatal(err)
	}
	// This authored freeze contains no units. The admission requirement is
	// absence of the restriction input itself, not an inference from currently
	// constructed units or from its visible effect on this empty catalog.
	inputs, err := content.FreezeSimulationInputs(sources, content.SimulationInputRequest{
		Catalog: valid.Catalog(), MapOTA: ota, MapTNT: tnt, MapSchema: valid.MapSchema(),
		CommunityDigest: valid.CommunityDigest(), PreparingRuleName: name, PreparingRuleBase: base,
		Restrictions: restrictions,
	})
	if err != nil {
		t.Fatal(err)
	}
	requireAdmissionKinds(t, "frozen restrictions with empty field 12", ValidateMatchInputs(config, inputs), ErrMatchContentMismatch)
}
