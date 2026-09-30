package main

import (
	"encoding/json"
	"github.com/nanolathe-gg/nanolathe/internal/community"
	"github.com/nanolathe-gg/nanolathe/internal/save"
	"github.com/nanolathe-gg/nanolathe/internal/session"
	"path/filepath"
	"slices"
	"testing"

	"github.com/nanolathe-gg/nanolathe/internal/gameplay"
	"github.com/nanolathe-gg/nanolathe/internal/input"
	"github.com/nanolathe-gg/nanolathe/internal/modlibrary"
	"github.com/nanolathe-gg/nanolathe/internal/settings"
)

// These lock the Nanolathe screen's transactions, not retail behavioral
// claims (DESIGN_INTERFACE_HUD_INPUT §3.17, DESIGN_MODS_MUTATORS §4.3).

// An overridden rule lock must survive the start-up raise and a restart; a
// cleared one must raise again.
func TestModLockOverrideSurvivesStartupRaise(t *testing.T) {
	mod := &modlibrary.Mod{Metadata: modlibrary.Metadata{ID: "locked", Name: "Locked", MinimumGameplay: string(gameplay.Community39)}}
	g := &gameShell{cs: &contentSet{mod: mod}}
	g.setGameplay(gameplay.Strict31)
	g.setLockOverride(mod, true)
	g.enforceModGameplayMinimum()
	if g.gameplay != gameplay.Strict31 {
		t.Fatalf("overridden lock raised the rules to %s", g.gameplay)
	}
	saved := g.captureSettings()
	if !slices.Equal(saved.ModLockOverrides, []string{"locked"}) {
		t.Fatalf("saved overrides %v", saved.ModLockOverrides)
	}
	next := &gameShell{cs: &contentSet{mod: mod}}
	next.applySettings(saved)
	next.enforceModGameplayMinimum()
	if next.gameplay != gameplay.Strict31 || !next.lockOverridden(mod) {
		t.Fatalf("restart lost the override: %s, overridden %v", next.gameplay, next.lockOverridden(mod))
	}
	next.setLockOverride(mod, false)
	next.enforceModGameplayMinimum()
	if next.gameplay != gameplay.Community39 {
		t.Fatalf("cleared lock left the rules at %s", next.gameplay)
	}
}

// A controls profile applies first; a card the player changed afterwards
// keeps the player's value, and every other row takes the profile's.
func TestNLScreenApplyKeepsTouchedCardsOverProfile(t *testing.T) {
	g, _, cl := retailAssetShell(t)
	previous := clPtr
	clPtr = cl
	t.Cleanup(func() { clPtr = previous })
	t.Setenv(settings.EnvPath, filepath.Join(t.TempDir(), "settings.json"))
	g.attachSettings()
	s := newNLScreen(func() *gameShell { return g })
	s.draft = s.snapshot(g)
	card := func(key string) nlCard {
		for _, c := range s.controlCards() {
			if c.key == key {
				return c
			}
		}
		t.Fatalf("no %s card", key)
		return nlCard{}
	}
	s.setCard(card("profile"), 2) // Community
	s.setCard(card("selection"), 2)
	s.apply()
	p := g.presentation
	if p.CommunitySelection != 2 {
		t.Fatalf("touched selection became %d", p.CommunitySelection)
	}
	if p.FactoryHundredBatch != 1 || p.DoubleClickSelection != 1 || !g.switchAlt {
		t.Fatalf("profile rows not applied: %+v switchAlt %v", p, g.switchAlt)
	}
	saved, err := settings.Load()
	if err != nil {
		t.Fatal(err)
	}
	if saved.Presentation.CommunitySelection != 2 || saved.Presentation.FactoryHundredBatch != 1 {
		t.Fatalf("saved %+v", saved.Presentation)
	}
	if s.dirty() != 0 {
		t.Fatalf("%d cards still differ after Apply", s.dirty())
	}
}

// A key rebound on the Controls page reaches the shell's key map on Apply
// and the settings file keeps only the change.
func TestNLScreenApplyWritesKeyBindings(t *testing.T) {
	g, _, cl := retailAssetShell(t)
	previous := clPtr
	clPtr = cl
	t.Cleanup(func() { clPtr = previous })
	t.Setenv(settings.EnvPath, filepath.Join(t.TempDir(), "settings.json"))
	g.attachSettings()
	s := newNLScreen(func() *gameShell { return g })
	s.draft = s.freshDraft(g)
	q := input.Chord{Key: input.KeyQ}
	s.draft.keys.Rebind("attack", []input.Chord{q})
	s.touched["keys"] = true
	if s.dirty() == 0 {
		t.Fatal("a rebound key does not count as a change")
	}
	s.apply()
	if got := g.liveKeyMap().Keys("attack"); !slices.Equal(got, []input.Chord{q}) {
		t.Fatalf("shell attack keys %v", got)
	}
	saved, err := settings.Load()
	if err != nil {
		t.Fatal(err)
	}
	if got := saved.KeyBindings.Bindings["attack"]; !slices.Equal(got, []string{"q"}) || len(saved.KeyBindings.Bindings) != 1 {
		t.Fatalf("saved bindings %v", saved.KeyBindings.Bindings)
	}
}

// A change made while a mod runs is kept for that mod: the base block keeps
// the original game's value, and loading the base for the original game or
// for the mod gives back each its own.
func TestModSettingsKeepChangesPerMod(t *testing.T) {
	mod := &modlibrary.Mod{Metadata: modlibrary.Metadata{ID: "somemod", Name: "Some mod"}}
	g := &gameShell{cs: &contentSet{mod: mod}}
	g.applySettings(settings.Defaults())
	g.presentation.WaterSurface = 0
	file := g.captureSettings()
	if file.Presentation.WaterSurface != settings.DefaultEffectSwitch {
		t.Fatalf("base block took the mod's change: water %d", file.Presentation.WaterSurface)
	}
	if len(file.ModSettings["somemod"]) == 0 {
		t.Fatal("the mod's change was not kept")
	}
	plain := &gameShell{cs: &contentSet{}}
	plain.applySettings(file)
	if plain.presentation.WaterSurface != settings.DefaultEffectSwitch {
		t.Fatalf("the original game plays the mod's change: water %d", plain.presentation.WaterSurface)
	}
	again := &gameShell{cs: &contentSet{mod: mod}}
	again.applySettings(file)
	if again.presentation.WaterSurface != 0 {
		t.Fatalf("the mod lost its change: water %d", again.presentation.WaterSurface)
	}
	if kept := again.captureSettings(); string(kept.ModSettings["somemod"]) != string(file.ModSettings["somemod"]) {
		t.Fatalf("a save without changes moved the patch: %s vs %s", kept.ModSettings["somemod"], file.ModSettings["somemod"])
	}
}

// A saved preset reaches the settings file, and applying one part of a
// preset moves only that part of the draft.
func TestNLScreenPresetsSaveAndApplyPart(t *testing.T) {
	g, _, cl := retailAssetShell(t)
	previous := clPtr
	clPtr = cl
	t.Cleanup(func() { clPtr = previous })
	t.Setenv(settings.EnvPath, filepath.Join(t.TempDir(), "settings.json"))
	g.attachSettings()
	s := newNLScreen(func() *gameShell { return g })
	s.draft = s.freshDraft(g)
	s.draft.pres.Glint, s.draft.switchAlt = 0, true
	s.savePreset("Mine")
	saved, err := settings.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(saved.Presets) != 1 || saved.Presets[0].Name != "Mine" {
		t.Fatalf("saved presets %+v", saved.Presets)
	}
	// Back to the original game's graphics only: glint returns, the digit
	// keys stay as the player set them.
	var original nlPresetEntry
	for _, e := range s.presetEntries() {
		if e.name == "Original game" {
			original = e
		}
	}
	s.applyPresetToDraft(original, []bool{false, true, false})
	if s.draft.pres.Glint != settings.DefaultEffectSwitch || !s.draft.switchAlt {
		t.Fatalf("graphics part: glint %d switchAlt %v", s.draft.pres.Glint, s.draft.switchAlt)
	}
}

// A mod's recommendation is its settings layer and a locked path keeps the
// mod's value until the player overrides; the screen marks the card as set
// by the mod, and changing a locked card asks first.
func TestModConfigSettingsAndLocks(t *testing.T) {
	cfg := &modlibrary.Config{Settings: json.RawMessage(`{"presentation":{"glint":0}}`), Locks: []string{"presentation.glint"}}
	mod := &modlibrary.Mod{Metadata: modlibrary.Metadata{ID: "lockmod", Name: "Lock mod", Config: cfg}}
	file := settings.Defaults()
	file.ModSettings = map[string]json.RawMessage{"lockmod": json.RawMessage(`{"presentation":{"glint":1}}`)}
	g := &gameShell{cs: &contentSet{mod: mod}}
	g.applySettings(file)
	if g.presentation.Glint != 0 {
		t.Fatalf("a locked setting played the player's value: glint %d", g.presentation.Glint)
	}
	settled := g.captureSettings()
	settled.ModLockOverrides = []string{mod.ID}
	g.applySettings(settled)
	if g.presentation.Glint != 1 {
		t.Fatalf("an overridden lock kept the mod's value: glint %d", g.presentation.Glint)
	}
	settled = g.captureSettings()
	settled.ModLockOverrides = nil
	g.applySettings(settled)

	s := newNLScreen(func() *gameShell { return g })
	s.mods = []modlibrary.Mod{*mod}
	s.draft = s.freshDraft(g)
	s.draft.mod = 1
	s.bindSource(g)
	var metal nlCard
	for _, c := range s.effectCards() {
		if c.key == "metal" {
			metal = c
		}
	}
	if !s.cardLocked(metal) || s.cardSource(metal) != "set" {
		t.Fatalf("metal card: locked %v, source %q", s.cardLocked(metal), s.cardSource(metal))
	}
	d := s.draft
	metal.parts[1].set(&d, 1)
	s.setCard(metal, metal.get(&d))
	if s.dialog != "override" || s.draft.pres.Glint != 0 {
		t.Fatalf("a locked change did not ask first: dialog %q, glint %d", s.dialog, s.draft.pres.Glint)
	}
	s.confirmOverride()
	if s.draft.pres.Glint != 1 || !s.draft.override {
		t.Fatalf("the override did not apply the change: glint %d", s.draft.pres.Glint)
	}
}

// A sidecar that names a per-mod table this build no longer carries restores
// under its recorded entry table.
func TestSidecarRemovedTableUsesEntry(t *testing.T) {
	entry, err := community.Table(community.Mainline)
	if err != nil {
		t.Fatal(err)
	}
	entry.UnitLimit = 1234
	sc := save.Sidecar{Rules: "community-3.9", Gameplay: "community-3.9",
		Community: save.SidecarCommunity{Sources: save.SidecarCommunitySources{Content: []community.Overrides{{Table: "tazero"}}}, Entry: entry}}
	sel, err := resolveSidecarSelection(sc)
	if err != nil {
		t.Fatal(err)
	}
	got := sel.sources.Content
	if len(got) != 1 || got[0].Table != "" || got[0].Base == nil || got[0].Base.UnitLimit != 1234 {
		t.Fatalf("content sources %+v", got)
	}
	resolved, err := session.ResolveCommunity(gameplay.Community39, sel.sources)
	if err != nil || resolved.UnitLimit != 1234 {
		t.Fatalf("resolved %+v, %v", resolved, err)
	}
}
