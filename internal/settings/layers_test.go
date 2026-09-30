package settings

import (
	"encoding/json"
	"testing"
)

// A mod patch and a player patch layer over the base, and the diff of the
// result against base plus mod patch is exactly the player patch.
func TestLayersRoundTrip(t *testing.T) {
	base := Defaults()
	mod := json.RawMessage(`{"gameplay":"community-3.9","presentation":{"communitySelection":1},"switchAlt":1}`)
	player := json.RawMessage(`{"presentation":{"communitySelection":2,"waterSurface":0}}`)
	eff, err := Layer(base, mod, player)
	if err != nil {
		t.Fatal(err)
	}
	if eff.Presentation.CommunitySelection != 2 || eff.SwitchAlt != 1 || eff.Presentation.WaterSurface != 0 {
		t.Fatalf("layered %+v", eff.Presentation)
	}
	baseline, err := Layer(base, mod)
	if err != nil {
		t.Fatal(err)
	}
	diff, err := Diff(eff, baseline, ModScoped)
	if err != nil {
		t.Fatal(err)
	}
	again, err := Layer(baseline, diff)
	if err != nil {
		t.Fatal(err)
	}
	if again.Presentation != eff.Presentation || again.SwitchAlt != eff.SwitchAlt || again.Gameplay != eff.Gameplay {
		t.Fatalf("diff %s does not reproduce the layered settings", diff)
	}
	var doc map[string]any
	_ = json.Unmarshal(diff, &doc)
	if _, ok := doc["switchAlt"]; ok {
		t.Fatalf("diff carries a value the mod already set: %s", diff)
	}
}

// Atomic subtrees replace whole, so a layer can drop a key binding the base
// had rebound.
func TestLayersReplaceKeyBindingsWhole(t *testing.T) {
	base := Defaults()
	base.KeyBindings = KeyBindings{Bindings: map[string][]string{"attack": {"q"}}}
	eff, err := Layer(base, json.RawMessage(`{"keyBindings":{"profile":"zero"}}`))
	if err != nil {
		t.Fatal(err)
	}
	if eff.KeyBindings.Profile != "zero" || len(eff.KeyBindings.Bindings) != 0 {
		t.Fatalf("key bindings merged instead of replaced: %+v", eff.KeyBindings)
	}
	diff, err := Diff(eff, base, ModScoped)
	if err != nil {
		t.Fatal(err)
	}
	again, err := Layer(base, diff)
	if err != nil {
		t.Fatal(err)
	}
	if again.KeyBindings.Profile != "zero" || len(again.KeyBindings.Bindings) != 0 {
		t.Fatalf("diff %s lost the replacement: %+v", diff, again.KeyBindings)
	}
}

// Restrict drops everything outside the mod-scoped paths.
func TestLayersRestrict(t *testing.T) {
	got, err := Restrict(json.RawMessage(`{"fullscreen":true,"display":{"width":800,"glow":0},"presentation":{"waterSurface":0}}`), ModScoped)
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	_ = json.Unmarshal(got, &doc)
	if _, ok := doc["fullscreen"]; ok || doc["display"].(map[string]any)["width"] != nil || doc["display"].(map[string]any)["glow"] != 0.0 {
		t.Fatalf("restricted %s", got)
	}
}

// Without drops a path and its subtree and leaves the rest.
func TestLayersWithout(t *testing.T) {
	got, err := Without(json.RawMessage(`{"presentation":{"waterSurface":0,"glint":0},"switchAlt":1}`), []string{"presentation.waterSurface", "switchAlt"})
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != `{"presentation":{"glint":0}}` {
		t.Fatalf("without = %s", got)
	}
}
