package main

import (
	"fmt"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/nanolathe-gg/nanolathe/internal/modlibrary"
	"github.com/nanolathe-gg/nanolathe/internal/platform/screenkit"
	"github.com/nanolathe-gg/nanolathe/internal/settings"
)

// The remove cap must receive the whole click even when badges move it into
// the row's selection region (DESIGN_MODS_MUTATORS §8.2).
func TestNLScreenModRemovePointer(t *testing.T) {
	oldW, oldH := nlScreenW, nlScreenH
	nlScreenW, nlScreenH = 1560, 900
	t.Cleanup(func() { nlScreenW, nlScreenH = oldW, oldH })
	fonts := screenkit.LoadFonts()
	screen := ebiten.NewImage(1560, 900)
	defer screen.Deallocate()
	for _, badges := range []struct{ name, config string }{
		{"none", ""},
		{"rules", `,"rules":{"minimumGameplay":"community-3.9"}`},
		{"controls", `,"keys":{"profile":"zero"}`},
		{"both", `,"rules":{"minimumGameplay":"community-3.9"},"keys":{"profile":"zero"}`},
	} {
		t.Run(badges.name, func(t *testing.T) {
			base, lib := modFixture(t)
			mod := installFixtureConfig(t, lib, "old-mod", fmt.Sprintf(`{"schema":2,"id":"sample","name":"Sample","version":"1+nanolathe.1"%s}`, badges.config))
			g, s := settingsRegressionScreen(nil, settings.Defaults())
			g.cs.baseRoots = []string{base}
			s.fonts, s.art, s.mods = fonts, &nlArt{}, []modlibrary.Mod{mod}
			card := s.gameCards()[0]
			draw := func() {
				s.hits.Begin()
				s.heroContent(screen, &card, s.draft.mod, 0, 0, 1)
				if s.dialog == "remove" {
					s.drawRemoveConfirm(screen)
				}
				s.hits.End()
			}
			click := func(x, y float64) {
				s.hits.Update(screenkit.Input{X: x, Y: y, Pressed: true}, 0)
				s.hits.Update(screenkit.Input{X: x, Y: y, Released: true}, 0)
			}
			draw()
			// Find the visible cap through pointer routing, so this checks its
			// reachable width without reproducing the badge placement formula.
			var capPixels []float64
			for x := 0.5; x < s.contentList.W; x++ {
				s.hits.Update(screenkit.Input{X: x, Y: 73}, 0)
				if s.hits.Hot() == s.ui.id("content-remove", "", 1, -1) {
					capPixels = append(capPixels, x)
				}
			}
			if len(capPixels) != 24 {
				t.Fatalf("only %d of the remove cap's 24 pixels receive clicks", len(capPixels))
			}
			x := capPixels[len(capPixels)/2]
			click(x, 73)
			if s.dialog != "remove" || !sameMod(&s.removing, &mod) || s.draft.mod != 0 {
				t.Fatalf("remove click changed the selection or missed confirmation: dialog %q, draft %d", s.dialog, s.draft.mod)
			}
			draw()
			click(823, 515) // Keep
			if _, ok, err := lib.Lookup(mod.ID, mod.Version); err != nil || !ok || s.dialog != "" {
				t.Fatalf("Keep did not preserve the installed mod: %v, %v, dialog %q", ok, err, s.dialog)
			}
			draw()
			click(x, 73)
			draw()
			click(973, 515) // Remove
			if _, ok, err := lib.Lookup(mod.ID, mod.Version); err != nil || ok || len(s.mods) != 0 || s.dialog != "" {
				t.Fatalf("confirmed removal left the mod installed or listed: %v, %v, rows %d, dialog %q", ok, err, len(s.mods), s.dialog)
			}
		})
	}
}
