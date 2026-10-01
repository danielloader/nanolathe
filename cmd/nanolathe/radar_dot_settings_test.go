package main

import (
	"encoding/json"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/nanolathe-gg/nanolathe/internal/modlibrary"
	"github.com/nanolathe-gg/nanolathe/internal/settings"
)

// The Game card edits only its ordinary presentation preference; the rules
// and renderer determine admission later (interface design "Modern radar dots").
func radarDotsCard(t *testing.T, s *nlScreen) nlCard {
	t.Helper()
	for _, page := range s.pages() {
		if page.key != "game" {
			continue
		}
		for _, c := range page.cards {
			if c.key == "radardots" {
				return c
			}
		}
	}
	t.Fatal("Radar dots is missing from the Game page")
	return nlCard{}
}

func TestRadarDotsCardDraftApplySaveCancel(t *testing.T) {
	for _, style := range []int{settings.RadarDotsNone, settings.RadarDotsAttackable} {
		t.Run([]string{"none", "visible", "attackable"}[style], func(t *testing.T) {
			t.Setenv(settings.EnvPath, filepath.Join(t.TempDir(), "settings.json"))
			g, s := settingsRegressionScreen(nil, settings.Defaults())
			g.settingsWritable = true
			c := radarDotsCard(t, s)
			if c.label != "Radar dots" || !c.enhanced || !slices.Equal(c.steps, []string{"No dots", "Visible dots", "Attackable dots"}) {
				t.Fatal("Radar dots lost its title, choices or renderer scope")
			}
			text := c.desc(&s.draft, style)
			for _, scope := range []string{"Modern gameplay", "Enhanced renderer", "Strict 3.1", "Community 3.9", "minimap"} {
				if !strings.Contains(text, scope) {
					t.Fatalf("Radar dots description omits %q", scope)
				}
			}
			before := s.draft.pres
			s.setCard(c, style)
			want := before
			want.RadarDots = style
			if s.draft.pres != want || g.presentation != before || s.dirty() != 1 {
				t.Fatal("Radar dots changed another preference or reached the live settings before Apply")
			}
			if !slices.Equal(s.cardPaths(c, settings.Defaults()), []string{"presentation.radarDots"}) {
				t.Fatal("Radar dots did not expose its ordinary mod-layer path")
			}
			s.apply()
			stored, err := settings.Load()
			if err != nil || stored.Presentation.RadarDots != style || g.presentation.RadarDots != style || s.dirty() != 0 {
				t.Fatalf("Radar dots Apply/save failed: live %d, stored %d: %v", g.presentation.RadarDots, stored.Presentation.RadarDots, err)
			}
			s.setCard(c, settings.RadarDotsVisible)
			s.hide()
			stored, err = settings.Load()
			if err != nil {
				t.Fatal(err)
			}
			restarted, reopened := settingsRegressionScreen(nil, stored)
			if g.presentation.RadarDots != style || restarted.presentation.RadarDots != style || reopened.draft.pres.RadarDots != style {
				t.Fatal("a cancelled Radar dots draft replaced the saved preference")
			}
		})
	}
}

func TestRadarDotsModRecommendationPlayerLayerAndLock(t *testing.T) {
	mod := &modlibrary.Mod{Metadata: modlibrary.Metadata{ID: "radar-style", Name: "Radar style", Config: &modlibrary.Config{
		Settings: json.RawMessage(`{"presentation":{"radarDots":2}}`),
	}}}
	file := settings.Defaults()
	g, s := settingsRegressionScreen(mod, file)
	if g.presentation.RadarDots != settings.RadarDotsAttackable || s.draft.pres.RadarDots != settings.RadarDotsAttackable {
		t.Fatal("mod recommendation did not reach the Radar dots card")
	}
	s.setCard(radarDotsCard(t, s), settings.RadarDotsNone)
	s.apply()
	file = g.captureSettings()
	if file.Presentation.RadarDots != settings.RadarDotsVisible || string(file.ModSettings[mod.ID]) != `{"presentation":{"radarDots":0}}` {
		t.Fatalf("mod edit reached the base or omitted its explicit zero: %d, %s", file.Presentation.RadarDots, file.ModSettings[mod.ID])
	}
	plain, _ := settingsRegressionScreen(nil, file)
	again, reopened := settingsRegressionScreen(mod, file)
	if plain.presentation.RadarDots != settings.RadarDotsVisible || again.presentation.RadarDots != settings.RadarDotsNone || reopened.draft.pres.RadarDots != settings.RadarDotsNone {
		t.Fatal("restart mixed the base and mod Radar dots choices")
	}
	mod.Config.Locks = []string{"presentation.radarDots"}
	locked, screen := settingsRegressionScreen(mod, file)
	c := radarDotsCard(t, screen)
	if locked.presentation.RadarDots != settings.RadarDotsAttackable || !screen.cardLocked(c) {
		t.Fatal("mod lock did not retain its recommended Radar dots value")
	}
	screen.setCard(c, settings.RadarDotsNone)
	if screen.dialog != "override" || screen.draft.pres.RadarDots != settings.RadarDotsAttackable {
		t.Fatal("a locked Radar dots card changed without the existing override flow")
	}
	file.ModLockOverrides = []string{mod.ID}
	overridden, _ := settingsRegressionScreen(mod, file)
	if overridden.presentation.RadarDots != settings.RadarDotsNone {
		t.Fatal("lock override lost the player's Radar dots choice")
	}
}
