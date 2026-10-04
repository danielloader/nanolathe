//go:build retail

package profiles_test

import (
	"strings"
	"testing"

	"github.com/nanolathe-gg/nanolathe/internal/content"
)

// Twilight Beta 98 keeps the retail tree names but ships a LOS table larger
// than the retail read cap. Admission must come from its package config:
// research/extensions/twilight-engine.md, "Authored content and settings".
func TestTwilightContentSetCompilesUnderItsConfig(t *testing.T) {
	fs := mountWithMod(t, "twilight")
	profile := modContent(t, fs, "twilight-2.0-beta98")
	view := profile.Layout().Apply(fs)
	if _, err := content.CompileLOSTables(view, content.RetailLimits()); err == nil {
		t.Fatal("retail LOS read cap admitted Twilight's enlarged table")
	} else if !strings.Contains(err.Error(), "exceeds read limit") {
		t.Fatalf("retail LOS refusal is not a read-cap error: %v", err)
	}
	catalog, err := content.CompileWithOptions(view, content.Options{Limits: content.LimitsFromProfile(profile.Limits)})
	if err != nil {
		t.Fatalf("compile Twilight: %v", err)
	}
	assertLargeLOSTableCompiled(t, catalog)
	for _, side := range catalog.Sides {
		commander := catalog.Units[content.CanonicalKey(side.Commander)]
		if commander == nil || commander.Script == nil {
			t.Fatalf("side %s has no linked commander program", side.Name)
		}
		if commander.Provenance.ProviderID != "rev31.gp3" {
			t.Fatalf("side %s commander came from %s", side.Name, commander.Provenance.ProviderID)
		}
		page := catalog.BuildMenus[commander.CanonicalKey]
		if page == nil || len(page.AuthoredButtons) == 0 {
			t.Fatalf("side %s commander has no authored build membership", side.Name)
		}
		for _, name := range page.AuthoredButtons {
			if catalog.Units[content.CanonicalKey(name)] == nil {
				t.Fatalf("commander %s product %s did not link", commander.UnitName, name)
			}
		}
	}
}
