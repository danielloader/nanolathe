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

func modScrollScreen(t *testing.T, w, h int) (*nlScreen, func()) {
	t.Helper()
	oldW, oldH := nlScreenW, nlScreenH
	nlScreenW, nlScreenH = float64(w), float64(h)
	t.Cleanup(func() { nlScreenW, nlScreenH = oldW, oldH })
	_, s := settingsRegressionScreen(nil, settings.Defaults())
	s.fonts, s.art, s.ready = screenkit.LoadFonts(), &nlArt{}, true
	for i := range 8 {
		s.mods = append(s.mods, modlibrary.Mod{Metadata: modlibrary.Metadata{
			ID: fmt.Sprintf("scroll-%d", i), Name: fmt.Sprintf("Mod %d", i), Version: "1",
		}})
	}
	s.installsSeen = modDownload.view().installs
	screen := ebiten.NewImage(w, h)
	t.Cleanup(screen.Deallocate)
	draw := func() {
		card := s.pages()[0].cards[0]
		s.hits.Begin()
		s.heroContent(screen, &card, s.draft.mod, 100*s.u(), 180*s.u(), 1)
		s.hits.End()
	}
	draw()
	return s, draw
}

// Viewport movement must survive redraw independently of selection, while
// keyboard selection follows the visible row order (DESIGN_INTERFACE_HUD_INPUT §3.17).
func TestNLScreenModListKeyboardAndTrackpad(t *testing.T) {
	s, draw := modScrollScreen(t, 1560, 900)
	key := func(k ebiten.Key) {
		s.updateInput(screenkit.Input{Keys: []ebiten.Key{k}}, 0)
		draw()
	}
	key(ebiten.KeyArrowDown)
	if s.draft.mod != 1 {
		t.Fatalf("Down selected row %d, want the next row", s.draft.mod)
	}
	key(ebiten.KeyArrowUp)
	key(ebiten.KeyArrowUp)
	if s.draft.mod != 0 {
		t.Fatal("Up did not stop at the first row")
	}
	wheel := func(delta float64) {
		s.updateInput(screenkit.Input{X: s.contentList.X + 20, Y: s.contentList.Y + 20, WheelY: delta}, 0)
		draw()
	}
	wheel(-0.375)
	wheel(-0.375)
	if s.contentTop != 0 {
		t.Fatal("fractional trackpad travel scrolled before a whole row")
	}
	wheel(-0.375)
	if s.contentTop != 1 || s.draft.mod != 0 {
		t.Fatalf("trackpad lost its viewport or changed selection: top %d, selected %d", s.contentTop, s.draft.mod)
	}
	// The fractional remainder joins the next batch; a batch can span rows.
	wheel(-1.875)
	if s.contentTop != 3 || s.draft.mod != 0 {
		t.Fatalf("wheel batch lost travel: top %d, selected %d", s.contentTop, s.draft.mod)
	}
	wheel(-20)
	if s.contentTop != 4 {
		t.Fatal("wheel did not clamp at the last viewport")
	}
	wheel(1)
	if s.contentTop != 3 {
		t.Fatal("reverse scrolling did not move up")
	}
	key(ebiten.KeyArrowDown)
	if s.draft.mod != 1 || s.contentTop != 1 {
		t.Fatal("keyboard selection did not reveal an offscreen row")
	}
	for range 10 {
		key(ebiten.KeyArrowDown)
	}
	if s.draft.mod != 8 || s.contentTop != 4 || s.shell().cs.mod != nil {
		t.Fatalf("last row unreachable or draft applied: selected %d, top %d", s.draft.mod, s.contentTop)
	}
}

func TestNLScreenModListScrollbarPointer(t *testing.T) {
	for _, size := range [][2]int{{640, 480}, {1560, 900}, {1920, 1080}} {
		t.Run(fmt.Sprintf("%dx%d", size[0], size[1]), func(t *testing.T) {
			s, draw := modScrollScreen(t, size[0], size[1])
			u := s.u()
			list := s.contentList
			x := list.X + list.W - 20*u
			pointer := func(in screenkit.Input) {
				s.updateInput(in, 0)
				draw()
			}
			click := func(y float64, id string) {
				t.Helper()
				pointer(screenkit.Input{X: x, Y: y, Pressed: true, Down: true})
				if s.hits.Hot() != id {
					t.Fatalf("pointer hit %q, want %q", s.hits.Hot(), id)
				}
				pointer(screenkit.Input{X: x, Y: y, Released: true})
			}
			click(list.Y+list.H-20*u, "content-down")
			if s.contentTop != 1 {
				t.Fatal("down arrow scroll was undone by redraw")
			}
			click(list.Y+20*u, "content-up")
			if s.contentTop != 0 {
				t.Fatal("up arrow did not scroll back")
			}
			// Grab near the thumb's top, then redraw during the held drag.
			y := list.Y + 50*u
			pointer(screenkit.Input{X: x, Y: y, Pressed: true, Down: true})
			pointer(screenkit.Input{X: x, Y: y, Down: true})
			if s.hits.Active() != "content-track" || s.contentTop != 0 {
				t.Fatal("grabbing the thumb jumped or lost capture")
			}
			pointer(screenkit.Input{X: x + 100*u, Y: list.Y + list.H + 50*u, Down: true})
			if s.contentTop != 4 || s.draft.mod != 0 {
				t.Fatal("drag outside the track did not reach the bottom independently of selection")
			}
			pointer(screenkit.Input{X: x, Y: list.Y - 50*u, Down: true})
			pointer(screenkit.Input{Released: true})
			if s.contentTop != 0 {
				t.Fatal("drag outside the track did not clamp to the top")
			}
			click(list.Y+list.H-50*u, "content-track")
			if s.contentTop != 4 {
				t.Fatalf("track click below the thumb did not reach the bottom: top %d", s.contentTop)
			}
			pointer(screenkit.Input{X: x, Y: list.Y + list.H/2, WheelY: 1})
			if s.contentTop != 3 || s.draft.mod != 0 {
				t.Fatal("wheel over the scrollbar did not scroll without selecting")
			}
			// Refreshing the library can shrink the viewport's range.
			s.mods = s.mods[:2]
			draw()
			if s.contentTop != 0 {
				t.Fatal("shortened list retained an invalid viewport")
			}
		})
	}
}

func TestNLScreenModListRefreshSelection(t *testing.T) {
	base, lib := modFixture(t)
	for i := range 6 {
		id := fmt.Sprintf("refresh-%d", i)
		installFixtureMod(t, lib, id, &modlibrary.Metadata{Schema: 1, ID: id, Name: fmt.Sprintf("Mod %d", i), Version: "1"})
	}
	g, s := settingsRegressionScreen(nil, settings.Defaults())
	g.cs.baseRoots = []string{base}
	s.reloadMods(g)
	s.draft.mod = len(s.mods)
	s.revealContent(s.draft.mod)
	s.removeMod(*s.modAt(s.draft.mod))
	if s.draft.mod != 0 || s.contentTop != 0 {
		t.Fatal("removing the selected mod did not reveal the original game")
	}
	s.draft.mod = len(s.mods)
	chosen := *s.modAt(s.draft.mod)
	installFixtureMod(t, lib, "earlier", &modlibrary.Metadata{Schema: 1, ID: "earlier", Name: "A new mod", Version: "1"})
	s.installsSeen = -1
	s.pollInstalls()
	if !sameMod(s.modAt(s.draft.mod), &chosen) || s.draft.mod >= s.contentTop+nlContentVisible {
		t.Fatal("install refresh lost or hid the selection after reordering")
	}
	// An unchanged refresh preserves browsing away from the selection.
	s.contentTop, s.installsSeen = 0, -1
	s.pollInstalls()
	if s.contentTop != 0 || !sameMod(s.modAt(s.draft.mod), &chosen) {
		t.Fatal("unchanged refresh reset the viewport or selected another mod")
	}
}
