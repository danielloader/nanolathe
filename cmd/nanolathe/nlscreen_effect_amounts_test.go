package main

import (
	"encoding/json"
	"path/filepath"
	"slices"
	"testing"

	"github.com/nanolathe-gg/nanolathe/internal/drawlist"
	"github.com/nanolathe-gg/nanolathe/internal/modlibrary"
	"github.com/nanolathe-gg/nanolathe/internal/settings"
)

func TestNLScreenSourceAmountsAndSoftness(t *testing.T) {
	s := newNLScreen(func() *gameShell { return nil })
	base := settings.Defaults()
	zero := base
	zero.Display.GlowStrength = 0
	if d := s.draftOf(zero); d.glowStrength != 0 {
		t.Fatalf("draft replaced explicit zero Overall strength with %d", d.glowStrength)
	}
	for _, card := range s.effectCards() {
		if card.key != "glow" && card.key != "softshadows" {
			continue
		}
		d := s.draftOf(base)
		r := nlRender{effects: presentationEffects(d.pres), glow: true, glowStrength: 100, groundLightStrength: 100}
		for i, part := range card.parts {
			if !part.meter || part.key == "overall" {
				continue
			}
			if part.steps[0] != "Off" || part.steps[len(part.steps)-1] != "200%" {
				t.Fatalf("%s lacks zero/full range", part.key)
			}
			part.set(&d, 1)
			card.render(&d, card.get(&d), &r)
			want := presentationEffects(d.pres)
			if r.effects != want || r.groundLightStrength != d.pres.GroundLightStrength {
				t.Fatalf("%s draft/render mismatch", part.key)
			}
			s.partSel[card.key] = i
			alt, ok := card.compare(&d, card.get(&d))
			if !ok {
				t.Fatalf("%s has no compare", part.key)
			}
			other := d
			card.set(&other, alt)
			if part.get(&other) != 0 {
				t.Fatalf("%s compare kept the selected amount", part.key)
			}
			for j, unchanged := range card.parts {
				if i != j && unchanged.get(&other) != unchanged.get(&d) {
					t.Fatalf("%s compare changed %s", part.key, unchanged.key)
				}
			}
		}
		paths := s.cardPaths(card, base)
		for _, part := range card.parts {
			if part.key == "overall" {
				continue
			}
			if !slices.Contains(paths, "presentation."+part.key) {
				t.Fatalf("%s missing mod lock discovery in %v", part.key, paths)
			}
		}
	}
	p := settings.DefaultPresentation()
	p.WeaponGlowStrength, p.ExplosionGlowStrength, p.NanoGlowStrength, p.ShadowSoftness = 0, 150, 25, 50
	e := presentationEffects(p)
	back := settings.DefaultPresentation()
	storeEffects(&back, e)
	if p != back || e == drawlist.AllEffects() {
		t.Fatal("source amounts lost in presentation conversion")
	}
	graphics, controls := nlGraphicsPaths(), nlControlsPaths()
	for _, key := range []string{"weaponGlowStrength", "explosionGlowStrength", "nanoGlowStrength", "shadowSoftness"} {
		path := "presentation." + key
		if !slices.Contains(graphics, path) || slices.Contains(controls, path) {
			t.Fatalf("%s omitted or misclassified in preset scopes", path)
		}
	}
}

// Apply persists the grouped amounts together with the existing master and
// ground value, on the ordinary live shell route used after a player edit.
func TestNLScreenApplyEffectAmounts(t *testing.T) {
	g, _, cl := retailAssetShell(t)
	previous := clPtr
	clPtr = cl
	t.Cleanup(func() { clPtr = previous })
	t.Setenv(settings.EnvPath, filepath.Join(t.TempDir(), "settings.json"))
	g.attachSettings()
	s := newNLScreen(func() *gameShell { return g })
	s.draft = s.snapshot(g)
	s.draft.pres.WeaponGlowStrength, s.draft.pres.ExplosionGlowStrength, s.draft.pres.NanoGlowStrength = 0, 150, 25
	s.draft.pres.GroundLightStrength, s.draft.pres.ShadowSoftness = 50, 200
	s.draft.glow, s.draft.glowStrength = 1, 150
	s.touched["glow"], s.touched["softshadows"] = true, true
	s.apply()
	saved, err := settings.Load()
	if err != nil {
		t.Fatal(err)
	}
	p := saved.Presentation
	if p.WeaponGlowStrength != 0 || p.ExplosionGlowStrength != 150 || p.NanoGlowStrength != 25 || p.GroundLightStrength != 50 || p.ShadowSoftness != 200 || saved.Display.GlowStrength != 150 {
		t.Fatalf("Apply lost effect amounts: %+v / %+v", p, saved.Display)
	}
}

// Overall at 100% increments only strength, but Off also writes the old glow
// switch. Discovery must see both paths before a locked grouped card is edited.
func TestNLScreenGlowMasterLockAtDefaultStrength(t *testing.T) {
	cfg := &modlibrary.Config{Settings: json.RawMessage(`{"display":{"glow":1,"glowStrength":100}}`), Locks: []string{"display.glow"}}
	mod := &modlibrary.Mod{Metadata: modlibrary.Metadata{ID: "glow-lock", Name: "Glow lock", Config: cfg}}
	g := &gameShell{cs: &contentSet{mod: mod}}
	g.applySettings(settings.Defaults())
	s := newNLScreen(func() *gameShell { return g })
	s.mods = []modlibrary.Mod{*mod}
	s.draft = s.freshDraft(g)
	s.draft.mod = 1
	s.bindSource(g)
	for _, c := range s.effectCards() {
		if c.key != "glow" {
			continue
		}
		paths := s.src.paths[c.key]
		if !slices.Contains(paths, "display.glow") || !slices.Contains(paths, "display.glowStrength") || !s.cardLocked(c) {
			t.Fatalf("glow master missed lock paths: %v", paths)
		}
		d := s.draft
		c.parts[0].set(&d, 0)
		s.setCard(c, c.get(&d))
		if s.dialog != "override" || s.draft.glow != 1 {
			t.Fatal("locked glow master changed without approval")
		}
		return
	}
	t.Fatal("Glow card missing")
}
