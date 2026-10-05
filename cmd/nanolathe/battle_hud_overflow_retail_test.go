//go:build retail

package main

import (
	"fmt"
	"image/png"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/nanolathe-gg/nanolathe/internal/client"
	"github.com/nanolathe-gg/nanolathe/internal/content"
	"github.com/nanolathe-gg/nanolathe/internal/gui"
	"github.com/nanolathe-gg/nanolathe/internal/session"
	"github.com/nanolathe-gg/nanolathe/internal/testsupport"
	"github.com/nanolathe-gg/nanolathe/internal/units"
)

// Optional installed-content check: the synthetic tests are the portable
// regression. These roots are supplied explicitly and no retail bytes persist.
func TestRetailOversizedModMenus(t *testing.T) {
	for _, mod := range []struct {
		name  string
		sides int
	}{{"prota", 2}, {"zero", 3}, {"twilight", 2}} {
		for side := 0; side < mod.sides; side++ {
			profile := mod.name
			t.Run(fmt.Sprintf("%s/side%d", profile, side), func(t *testing.T) {
				roots := filepath.SplitList(os.Getenv("NANOLATHE_MOD_ROOTS_" + strings.ToUpper(profile)))
				if len(roots) == 0 {
					t.Skip("installed mod roots not supplied")
				}
				opts := Options{Root: testsupport.RetailRoot(t), Roots: append([]string{testsupport.RetailRoot(t)}, roots...), Map: "ashap plateau", Seed: 7, ModConfig: modRootsConfigPath(t, profile)}
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
				var commander *units.Unit
				for _, u := range sess.Units.Iter() {
					if u != nil && u.Owner == sess.LocalOwner && u.Def != nil && u.Def.Builder {
						commander = u
						break
					}
				}
				for step := int32(1); step <= 30; step++ {
					sess.Step(step)
				}
				cl, err := client.New(client.Options{Width: 640, Height: 480})
				if err != nil {
					t.Fatal(err)
				}
				b := &battleSession{sess: sess, cat: cat, cl: cl, local: testSelection(commander)}
				b.hud, err = loadRetailBattleHUD(cs.fs, sess, cat, retailPaletteForTest(t, cs), nil, newBattleWindowContext(cs, nil))
				if err != nil {
					t.Fatal(err)
				}
				f, _ := b.currentSnapshot()
				view, ok := snapshotUnitByHandle(f, f.CommandPage.Builder)
				if !ok {
					t.Fatal("missing selected commander")
				}
				def, _ := cat.Unit(view.DefName)

				want := map[string]int{}
				for page := 1; page < int(f.CommandPage.PageCount); page++ {
					w, _ := b.hud.sidebarBuildPage(cat, def, page)
					if w == nil {
						t.Fatalf("page %d missing", page)
					}
					for _, g := range w.Gadgets {
						if g.CommonAttribs&4 != 0 && !strings.EqualFold(g.Name, "IGPATCH") {
							want[g.Name]++
						}
					}
				}
				var compositeGroups [][]string
				prefix := []string{"arm", "cor", "gok"}[side]
				if profile == "prota" {
					compositeGroups = [][]string{{prefix + "syw", prefix + "sy", prefix + "syn", prefix + "sye"}}
				} else if profile == "twilight" {
					compositeGroups = [][]string{
						{prefix + "labw", prefix + "lab", prefix + "labe"},
						{prefix + "vpw", prefix + "vp", prefix + "vpn", prefix + "vpe"},
						{prefix + "syw", prefix + "sy", prefix + "syn", prefix + "sye"},
					}
				}
				for _, enhanced := range []bool{false, true} {
					for _, height := range []int{480, 768, 1080} {
						t.Run(fmt.Sprintf("modern=%v/height=%d", enhanced, height), func(t *testing.T) {
							cl.SetEnhanced(enhanced)
							cl.Resize(1024, height)
							state, active := b.hud.expandedSidebarPaging(b, f)
							if !active {

								t.Fatal("oversized menu has no safe pager")
							}
							got := map[string]int{}
							type compositeButton struct {
								page   int
								rect   gui.Rect
								source gui.Rect
							}
							compositeButtons := make(map[string]compositeButton)
							for page := 1; page < state.Count; page++ {
								b.hud.selectExpandedSidebarPage(b, f, page)
								w, _, err := b.hud.windowForRequired(b, f)
								if err != nil {
									t.Fatal(err)
								}
								for i, g := range w.Gadgets {
									if i == 0 || g.Kind != gui.KindButton {
										continue
									}
									r := w.PlacedRect(i)
									if r.Y < 128 || r.Y+r.H > int32(height) {
										t.Fatalf("%s page%d outside sidebar: %+v", g.Name, page, r)
									}
									if g.CommonAttribs&4 == 0 || strings.EqualFold(g.Name, "IGPATCH") {
										continue
									}
									got[g.Name]++
									if enhanced && (r.W <= 0 || r.H <= 0 || r.W > 64 || r.H > 64) {
										t.Fatalf("modern product %s outside cell: %+v", g.Name, r)
									}
									if enhanced {
										name := strings.ToLower(g.Name)
										for _, group := range compositeGroups {
											if slices.Contains(group, name) {
												source := b.hud.expandedSidebar.sources[i]
												compositeButtons[name] = compositeButton{page, r, source.window.PlacedRect(source.index)}
											}
										}
									}
									if hit := w.HitTest(r.X+r.W/2, r.Y+r.H/2); hit != i {
										t.Fatalf("%s covered by gadget %d", g.Name, hit)
									}
									if product, ok := cat.Unit(g.Name); ok && product.Builder {
										x, y := r.X+r.W/2, r.Y+r.H/2
										if !hudConsumeClick(b.hud, b, x, y) || b.battleState().Input.BuildDef != content.CanonicalKey(g.Name) {
											t.Fatalf("factory %s did not arm placement: BMCode=%d active=%d grey=%d latch=%s dispatch=%v", g.Name, product.BMCode, g.Active, g.GrayedOut, b.battleState().Input.BuildDef, b.hud.dispatchErr)
										}
									}
									_, a := b.hud.sidebarSource(w, i, nil)
									if b.hud.gadgetArtEntry(g, a) == nil {
										t.Fatalf("%s has no art", g.Name)
									}
								}
								if dir := os.Getenv("NANOLATHE_MENU_SHOTS"); dir != "" && (page == 1 || page == state.Count-1 || profile == "twilight") {
									shotClient, err := client.New(client.Options{Width: 1024, Height: height, Buffer: sess.Snapshot})
									if err != nil {
										t.Fatal(err)
									}
									shotClient.SetEnhanced(enhanced)
									shotClient.SetUIStage(battleHUDUIStage{hud: b.hud, battle: b})
									shotClient.SetSnapshot(sess.Snapshot)
									shotClient.SetPalette(retailPaletteForTest(t, cs))

									file, err := os.Create(filepath.Join(dir, fmt.Sprintf("%s-side%d-modern%v-%d-page%d.png", profile, side, enhanced, height, page)))
									if err != nil {
										t.Fatal(err)
									}
									if err := png.Encode(file, shotClient.ComposeFrame()); err != nil {
										t.Fatal(err)
									}
									file.Close()
								}
							}
							if enhanced {
								for _, group := range compositeGroups {
									west, ok := compositeButtons[group[0]]
									if !ok {
										t.Fatalf("missing composite child %s", group[0])
									}
									for _, name := range group {
										child, ok := compositeButtons[name]
										if !ok || child.page != west.page {
											t.Fatalf("composite split across pages: %v", compositeButtons)
										}
										want := child.source
										want.X += west.rect.X - west.source.X
										want.Y += west.rect.Y - west.source.Y
										if child.rect != want {
											t.Fatalf("%s changed shape/offset: %+v want %+v", name, child.rect, want)
										}
									}
									if west.rect.W != 16 || west.rect.H != 64 {
										t.Fatalf("resolved composite geometry: %v", compositeButtons)
									}
								}
							}
							b.hud.selectExpandedSidebarPage(b, f, 0)
							orders, _, err := b.hud.windowForRequired(b, f)
							if err != nil || orders == nil || b.hud.sidebarPaging.state.Page != 0 {
								t.Fatalf("orders page unavailable: %v", err)
							}
							for i, g := range orders.Gadgets {
								if def.HasPageZeroGUI && (!enhanced || g.CommonAttribs&4 == 0 && !sidebarNavigation(g)) {
									source, _ := b.hud.sidebarSource(orders, i, nil)
									if !strings.EqualFold(source.Name, "guis/"+def.UnitName+"0.gui") {
										t.Fatalf("custom Orders used %s", source.Name)
									}
								}
								if i > 0 && g.Kind == gui.KindButton {
									r := orders.PlacedRect(i)
									if r.Y < 128 || r.Y+r.H > int32(height) {
										t.Fatalf("orders %s outside sidebar: %+v", g.Name, r)
									}
								}
							}
							for name, n := range want {
								if got[name] != n {
									t.Errorf("%s seen %d want %d", name, got[name], n)
								}
							}
							for name := range got {
								if _, ok := cat.Unit(content.CanonicalKey(name)); !ok {
									t.Errorf("missing product %s", name)
								}
							}
						})
					}
				}
			})
		}
	}

}
