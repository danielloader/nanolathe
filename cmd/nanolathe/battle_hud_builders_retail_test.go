//go:build retail

package main

import (
	"fmt"
	"image/png"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/nanolathe-gg/nanolathe/internal/client"
	"github.com/nanolathe-gg/nanolathe/internal/hud"
	"github.com/nanolathe-gg/nanolathe/internal/session"
	"github.com/nanolathe-gg/nanolathe/internal/sim/numeric"
	"github.com/nanolathe-gg/nanolathe/internal/testsupport"
)

// Enter through the completed front-end transition: it preserves authored
// quickkey bytes, including case differences between physical and DL pages.
func TestRetailBuilderMenusAfterTransition(t *testing.T) {
	for side := 0; side < 2; side++ {
		t.Run(fmt.Sprint(side), func(t *testing.T) {
			opts := Options{Root: testsupport.RetailRoot(t), Map: "ashap plateau", Seed: 7}
			cs, err := openContent(opts)
			if err != nil {
				t.Fatal(err)
			}
			defer cs.Close()
			cfg := session.SkirmishConfig{MapName: opts.Map, NumPlayers: 2}
			cfg.Players[0].Side = side
			cfg.Players[1].Controller = session.SkirmishControllerComputer
			cfg.ApplyDefaults()
			sess, cat, err := newBattleSessionWithConfig(opts, cs, cfg)
			if err != nil {
				t.Fatal(err)
			}
			ctx := newBattleWindowContext(cs, nil)
			ctx.completeTransition()
			pal := retailPaletteForTest(t, cs)
			h, err := loadRetailBattleHUD(cs.fs, sess, cat, pal, nil, ctx)
			if err != nil {
				t.Fatal(err)
			}
			cl, err := client.New(client.Options{Width: 1280, Height: 768})
			if err != nil {
				t.Fatal(err)
			}
			cl.SetEnhanced(true)
			b := &battleSession{sess: sess, cat: cat, cl: cl, hud: h}
			var x, y, z numeric.Fixed
			for _, u := range sess.Units.Iter() {
				if u.Owner == sess.LocalOwner {
					x, y, z = u.X, u.Y, u.Z
				}
			}
			var names []string
			for name, d := range cat.Units {
				// The local player can own either faction's builders through
				// capture, production or spawning (issue #85).
				if d.Builder && d.BuildPageCount > 1 {
					names = append(names, name)
				}
			}
			slices.Sort(names)
			if len(names) == 0 {
				t.Fatal("no builders")
			}
			step := int32(1)
			for _, name := range names {
				t.Run(name, func(t *testing.T) {
					for _, u := range sess.Units.Iter() {
						u.Flags &^= hud.SelectionFlag
					}
					d, _ := cat.Unit(name)
					// The settings demonstration must resolve the same cells as
					// the live HUD, rather than inventing six/twelve-item pages
					// from CANBUILD (HUD design §3.17).
					demo := loadNLSidebar(cs.fs, cat, d, cat.Sides[side], nil)
					live := h.sidebarProductCatalog(cat, d, int(d.BuildPageCount))
					if demo == nil || len(demo.cells) != len(live.cells) {
						t.Fatal("settings preview did not resolve the live build cells")
					}
					if demo.backdrop == nil {
						t.Fatal("settings preview did not resolve the native rail artwork")
					}
					for _, items := range [][]sidebarProduct{demo.tabs, demo.commands} {
						for _, item := range items {
							if demo.controls[item.source] == nil {
								t.Fatalf("settings preview lost native control art for %s", item.source.window.Gadgets[item.source.index].Name)
							}
						}
					}
					for i, cell := range live.cells {
						other := demo.cells[i]
						if cell.page != other.page || cell.bounds != other.bounds || len(cell.products) != len(other.products) {
							t.Fatalf("preview cell %d differs from the HUD", i)
						}
						for j, product := range cell.products {
							x, y := product.source, other.products[j].source
							if product.rect != other.products[j].rect || x.window.Gadgets[x.index].Name != y.window.Gadgets[y.index].Name {
								t.Fatalf("preview product %d/%d differs from the HUD", i, j)
							}
						}
					}
					handle, err := sess.Units.Create(d, sess.LocalOwner, x+numeric.FixedFromInt(160), y, z)
					if err != nil {
						t.Fatal(err)
					}
					u := sess.Units.Unit(handle)
					u.Flags = hud.EncodePageBits(u.Flags|hud.SelectionFlag, 1)
					for n := 0; n < 5; n++ {
						sess.Step(step)
						step++
					}
					f := sess.Snapshot.Current()
					for _, height := range []int{480, 768, 1080} {
						cl.Resize(1280, height)
						w, _, err := h.windowForRequired(b, f)
						if err != nil {
							t.Fatal(err)
						}
						if w == nil || !h.expandedSidebar.key.flat {
							t.Fatalf("height%d: builder retained separate authored pages", height)
						}
						state, _ := h.expandedSidebarPaging(b, f)
						controls := map[string]bool{}
						commandCounts := map[string]int{}
						for _, g := range w.Gadgets {
							if command := commandButtonName(g.Name); command != "" {
								commandCounts[command]++
								if commandCounts[command] > 1 {
									t.Fatalf("duplicated combined command %s", command)
								}
							}
							if g.Active != 0 {
								controls[commandButtonName(g.Name)] = true
							}
						}
						for _, name := range []string{"MOVE", "STOP", "DEFEND", "PATROL", "ATTACK", "BLAST", "REPAIR", "FIREORD"} {
							common := name != "REPAIR" && name != "FIREORD"
							if (common || h.expandedSidebar.key.inlineOrders) && !controls[name] {
								t.Fatalf("missing combined command %s", name)
							}
						}
						if len(sidebarVisibleProducts(w)) < 1 {
							t.Fatal("combined page has no build products")
						}
						first := sidebarVisibleProducts(w)
						inline := h.expandedSidebar.key.inlineOrders
						if h.sidebarPaging.capacity < 6 {
							t.Fatal("Free flow capacity fell below six")
						}
						h.selectExpandedSidebarPage(b, f, 0)
						ordersProducts := sidebarVisibleProducts(expandedWindow(t, b))
						if inline && !slices.Equal(ordersProducts, first) || !inline && len(ordersProducts) != 0 {
							t.Fatal("Orders did not match the combined or dedicated layout")
						}
						h.selectExpandedSidebarPage(b, f, state.Remembered)
						if dir := os.Getenv("NANOLATHE_MENU_SHOTS"); dir != "" && height == 768 {
							cl.SetUIStage(battleHUDUIStage{hud: h, battle: b})
							cl.SetSnapshot(sess.Snapshot)
							cl.SetPalette(pal)
							file, err := os.Create(filepath.Join(dir, fmt.Sprintf("ota-%d-%s-%d.png", side, name, height)))
							if err != nil {
								t.Fatal(err)
							}
							err = png.Encode(file, cl.ComposeFrame())
							closeErr := file.Close()
							if err != nil {
								t.Fatal(err)
							}
							if closeErr != nil {
								t.Fatal(closeErr)
							}
						}
					}
				})
			}
		})
	}
}
