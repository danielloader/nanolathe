package main

import (
	"path/filepath"
	"slices"
	"testing"

	"github.com/nanolathe-gg/nanolathe/internal/drawlist"
	"github.com/nanolathe-gg/nanolathe/internal/settings"
)

func TestNLScreenHovercraftLandWashSelection(t *testing.T) {
	s := newNLScreen(func() *gameShell { return nil })
	base := settings.Defaults()
	for _, c := range s.effectCards() {
		if c.key != "hovercraftLandWash" {
			continue
		}
		d := s.draftOf(base)
		if !c.enhanced || c.scene(&d, c.get(&d)) != "metal" || c.get(&d) != 1 {
			t.Fatal("wash card lost default, indication or dry preview")
		}
		c.set(&d, 0)
		want := drawlist.AllEffects()
		want.HovercraftLandWash = false
		if presentationEffects(d.pres) != want {
			t.Fatal("wash card reached a water switch")
		}
		r := nlRender{effects: drawlist.AllEffects()}
		c.render(&d, 0, &r)
		if r.effects != want {
			t.Fatal("wash preview did not isolate executor selection")
		}
		if alt, ok := c.compare(&d, 0); !ok || alt != 1 {
			t.Fatal("wash card lacks independent compare")
		}
		if !slices.Contains(s.cardPaths(c, base), "presentation.hovercraftLandWash") {
			t.Fatal("wash card missing mod-lock path")
		}
		if !slices.Contains(nlGraphicsPaths(), "presentation.hovercraftLandWash") || slices.Contains(nlControlsPaths(), "presentation.hovercraftLandWash") {
			t.Fatal("wash missing graphics preset scope")
		}
		return
	}
	t.Fatal("land wash card missing")
}

func TestNLScreenApplyHovercraftLandWash(t *testing.T) {
	g, _, cl := retailAssetShell(t)
	previous := clPtr
	clPtr = cl
	t.Cleanup(func() { clPtr = previous })
	t.Setenv(settings.EnvPath, filepath.Join(t.TempDir(), "settings.json"))
	g.attachSettings()
	s := newNLScreen(func() *gameShell { return g })
	s.draft = s.snapshot(g)
	s.draft.pres.HovercraftLandWash = 0
	s.touched["hovercraftLandWash"] = true
	s.apply()
	saved, err := settings.Load()
	if err != nil {
		t.Fatal(err)
	}
	if saved.Presentation.HovercraftLandWash != 0 || saved.Presentation.WaterFoam != 1 || presentationEffects(g.presentation).HovercraftLandWash {
		t.Fatalf("Apply lost independent land-wash Off: stored wash=%d foam=%d, host wash=%v", saved.Presentation.HovercraftLandWash, saved.Presentation.WaterFoam, presentationEffects(g.presentation).HovercraftLandWash)
	}
}
