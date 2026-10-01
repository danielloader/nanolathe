package main

import (
	"slices"
	"testing"

	"github.com/nanolathe-gg/nanolathe/internal/gui"
	"github.com/nanolathe-gg/nanolathe/internal/input"
	"github.com/nanolathe-gg/nanolathe/internal/settings"
)

func TestSidebarHiddenOrdersKeepKeysAndDedicatedPage(t *testing.T) {
	b, cl, _ := sidebarRowsFixture(t, 480)
	b.shell = &gameShell{presentation: settings.DefaultPresentation()}
	b.shell.presentation.BuildMenuPageSize = 6
	w := expandedWindow(t, b)
	if len(sidebarVisibleProducts(w)) != 6 || b.hud.expandedSidebar.key.inlineOrders {
		t.Fatal("short fixed page did not show six products without inline orders")
	}
	for _, name := range []string{"MOVE", "ATTACK"} {
		g := w.Gadgets[expandedIndex(t, w, name)]
		if g.Active == 0 || g.Rect.W == 0 || g.Rect.H == 0 {
			t.Fatalf("common command %s hidden with supplementary Orders", name)
		}
	}
	repair := expandedIndex(t, w, "REPAIR")
	if w.Gadgets[repair].Active != 0 || w.PlacedRect(repair).W != 0 || w.PlacedRect(repair).H != 0 {
		t.Fatal("hidden order retained a draw or pointer target")
	}
	paletteCallbackToken(t, b, cl, 'r')
	if b.battleState().Input.Latch != input.LatchRepair || w.Gadgets[repair].Active != 0 {
		t.Fatal("hidden order lost its keyboard action or became visible")
	}
	f, _ := b.currentSnapshot()
	before := sidebarVisibleProducts(w)
	b.hud.selectExpandedSidebarPage(b, f, 0)
	w = expandedWindow(t, b)
	if len(sidebarVisibleProducts(w)) != 0 || w.Gadgets[expandedIndex(t, w, "REPAIR")].Active == 0 {
		t.Fatal("dedicated Orders page did not expose its controls")
	}
	b.hud.selectExpandedSidebarPage(b, f, b.hud.sidebarPaging.state.Remembered)
	if !slices.Equal(sidebarVisibleProducts(expandedWindow(t, b)), before) {
		t.Fatal("BUILD did not restore the remembered products")
	}
}

// These authored records distinguish the always-visible common rows from the
// optional supplementary panel, and lock fixed-count placement (HUD §3.3).
func TestSidebarCommonCommandsStayVisibleAndFixedControlsFollowBuild(t *testing.T) {
	b, cl, sources := sidebarRowsFixture(t, 1080)
	for _, source := range sources {
		for _, item := range []struct {
			name string
			x, y int32
		}{
			{"STOP", 64, 247}, {"DEFEND", 5, 282}, {"PATROL", 64, 282}, {"BLAST", 64, 317},
		} {
			source.Gadgets = append(source.Gadgets, gui.Gadget{Kind: gui.KindButton, Name: item.name, Active: 1, Rect: gui.Rect{X: item.x, Y: item.y, W: 55, H: 31}})
		}
	}
	b.shell = &gameShell{presentation: settings.DefaultPresentation()}
	for _, count := range []int{6, 12} {
		b.shell.presentation.BuildMenuPageSize = count
		for _, orders := range []int{0, 1} {
			b.shell.presentation.SidebarOrders = orders
			var tallTop int32
			for _, height := range []int{480, 800, 1080} {
				cl.Resize(1280, height)
				w := expandedWindow(t, b)
				for _, name := range []string{"MOVE", "STOP", "DEFEND", "PATROL", "ATTACK", "BLAST"} {
					i := expandedIndex(t, w, name)
					g, r := w.Gadgets[i], w.PlacedRect(i)
					if g.Active == 0 || r.W == 0 || r.H == 0 || r.Y+r.H > int32(height) || w.HitTest(r.X+1, r.Y+1) != i {
						t.Fatalf("common command %s unavailable at %d/%d/%d: %+v", name, count, orders, height, r)
					}
				}
				f, _ := b.currentSnapshot()
				def, _ := b.cat.Unit("armfav")
				c := b.hud.sidebarProductCatalog(b.cat, def, int(f.CommandPage.PageCount))
				p := c.sidebarLayout(height, count, orders != 0)
				gridEnd := int32(128) + c.upperHeight + int32((p.capacity+1)/2)*64
				for _, gap := range p.spacing {
					if gap.at < 0 {
						gridEnd += gap.pixels
					}
				}
				if p.commandTop != gridEnd {
					t.Fatal("fixed commands were not anchored to the reserved build area")
				}
				if height >= 800 && orders == 0 {
					top := w.PlacedRect(expandedIndex(t, w, "MOVE")).Y
					if height == 800 {
						tallTop = top
					} else if top != tallTop {
						t.Fatal("fixed command rows moved with spare screen height")
					}
				}
			}
		}
	}
}

func TestSidebarOrdersPreferenceRetiresCaptureAndKeepsProducts(t *testing.T) {
	b, _, _ := sidebarRowsFixture(t, 1080)
	b.shell = &gameShell{presentation: settings.DefaultPresentation()}
	b.shell.presentation.BuildMenuPageSize = 6
	w := expandedWindow(t, b)
	if !b.hud.expandedSidebar.key.inlineOrders {
		t.Fatal("orders should fit on the tall surface")
	}
	before := sidebarVisibleProducts(w)
	p := b.hud.palettePanel(w)
	p.SetPressed(expandedIndex(t, w, "REPAIR"))
	b.shell.presentation.SidebarOrders = 0
	next := expandedWindow(t, b)
	if next == w || p.CaptureIndex() != -1 || b.hud.expandedSidebar.key.inlineOrders || !slices.Equal(sidebarVisibleProducts(next), before) {
		t.Fatal("orders preference retained capture or changed a fixed build page")
	}
}

func TestSidebarHiddenOrdersPreserveSharedShortcutPrecedence(t *testing.T) {
	for _, view := range []string{"inline", "never", "dedicated"} {
		t.Run(view, func(t *testing.T) {
			b, cl, sources := sidebarRowsFixture(t, 1080)
			orders := sources[len(sources)-1]
			move, repair := expandedIndex(t, orders, "MOVE"), expandedIndex(t, orders, "REPAIR")
			orders.Gadgets[move].QuickKey, orders.Gadgets[repair].QuickKey = 'r', 'r'
			// The source's earlier accepting record owns this shared key,
			// even when its supplementary button is hidden [07 R-WGT-01 §3].
			orders.Gadgets[move], orders.Gadgets[repair] = orders.Gadgets[repair], orders.Gadgets[move]
			b.shell = &gameShell{presentation: settings.DefaultPresentation()}
			b.shell.presentation.BuildMenuPageSize = 6
			if view != "inline" {
				b.shell.presentation.SidebarOrders = 0
			}
			expandedWindow(t, b)
			if view == "dedicated" {
				f, _ := b.currentSnapshot()
				b.hud.selectExpandedSidebarPage(b, f, 0)
			}
			// Opening a different command window flushes the old token ring;
			// finish that transition before submitting the new owner's key.
			b.viewerStep(0, cl)
			paletteCallbackToken(t, b, cl, 'r')
			if b.battleState().Input.Latch != input.LatchRepair {
				t.Fatalf("orders visibility changed the shared shortcut winner: %v", b.battleState().Input.Latch)
			}
		})
	}
}
