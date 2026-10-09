package content

import "testing"

func TestCatalogCloneKeepsEffectiveLimits(t *testing.T) {
	original := &Catalog{Limits: Limits{Units: 17, Weapons: 31, TNTBytes: 4096, LOSBytes: 8192}}
	clone := original.Clone()
	if clone.Limits != original.Limits {
		t.Fatalf("clone lost effective limits: %+v, want %+v", clone.Limits, original.Limits)
	}
	clone.Limits.Units++
	if original.Limits.Units != 17 {
		t.Fatal("clone shares mutable limit storage")
	}
}

func TestFrozenPreparationIdentityIsAdmissionMetadata(t *testing.T) {
	f := newFrozenFixture(t)
	legacy := f.freeze(t, SimulationInputRequest{})
	request := SimulationInputRequest{PreparingRuleName: "authored-set", PreparingRuleBase: "modern"}
	admitted := f.freeze(t, request)
	request.PreparingRuleName = "changed-after-freeze"
	if name, base := admitted.PreparingRule(); name != "authored-set" || base != "modern" {
		t.Fatalf("preparing identity %q %q", name, base)
	}
	if legacy.Digest() != admitted.Digest() {
		t.Fatal("admission metadata changed the semantic content digest")
	}
	for _, absent := range []*SimulationInputs{nil, legacy} {
		if name, base := absent.PreparingRule(); name != "" || base != "" {
			t.Fatal("missing preparing identity was defaulted")
		}
	}
}
