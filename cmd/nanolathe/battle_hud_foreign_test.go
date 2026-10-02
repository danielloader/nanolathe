package main

import (
	"slices"
	"testing"

	"github.com/nanolathe-gg/nanolathe/internal/gui"
	"github.com/nanolathe-gg/nanolathe/internal/input"
	"github.com/nanolathe-gg/nanolathe/internal/settings"
)

// Foreign native pages and the local generated/Orders pages name the same
// commands with different prefixes [07 R-HUD-03 §6].
func foreignSidebarFixture(t *testing.T) (*battleSession, []*gui.Window) {
	t.Helper()
	b, _, sources := expandedSidebarFixture(t)
	for page, source := range sources {
		prefix := "ARM"
		if page == 0 {
			prefix = "COR"
		}
		for i := range source.Gadgets {
			g := &source.Gadgets[i]
			if commandButtonName(g.Name) != "" || g.Name == "NEXT" || g.Name == "PREV" {
				g.Name = prefix + g.Name
			}
		}
	}
	// Shared commands use Orders' shortcut even when a native page differs.
	sources[0].Gadgets[expandedIndex(t, sources[0], "CORMOVE")].QuickKey = 'q'
	sources[2].Gadgets[expandedIndex(t, sources[2], "ARMMOVE")].QuickKey = 'm'
	b.shell = &gameShell{presentation: settings.DefaultPresentation()}
	return b, sources
}

func TestExpandedSidebarForeignCommandsKeepPreferencesAndSources(t *testing.T) {
	b, sources := foreignSidebarFixture(t)
	originals := make([][]gui.Gadget, len(sources))
	for i, source := range sources {
		originals[i] = slices.Clone(source.Gadgets)
	}
	for _, limit := range []int{6, 12, 0} {
		b.shell.presentation.BuildMenuPageSize = limit
		for _, orders := range []int{0, 1} {
			b.shell.presentation.SidebarOrders = orders
			w := expandedWindow(t, b)
			if !b.hud.expandedSidebar.key.flat || b.hud.expandedSidebar.key.inlineOrders != (orders != 0) {
				t.Fatalf("foreign scaffold ignored preferences %d/%d", limit, orders)
			}
			want := 12
			if limit == 6 {
				want = 6
			}
			if got := len(sidebarVisibleProducts(w)); got != want {
				t.Fatalf("limit %d has %d products, want %d", limit, got, want)
			}
			for _, name := range []string{"BUILD", "ORDERS", "MOVE", "ATTACK"} {
				count := 0
				for _, g := range w.Gadgets {
					if commandButtonName(g.Name) == name {
						count++
						if g.Name != "ARM"+name {
							t.Fatalf("%s lost the Orders source name", name)
						}
					}
				}
				if count != 1 {
					t.Fatalf("foreign scaffold has %d copies of %s", count, name)
				}
			}
			if w.Gadgets[expandedIndex(t, w, "ARMMOVE")].QuickKey != 'm' {
				t.Fatal("shared command lost the canonical Orders shortcut")
			}
			if expandedIndex(t, w, "CORPREV") < 1 || expandedIndex(t, w, "CORNEXT") < 1 {
				t.Fatal("native navigation source lost")
			}
		}
	}
	f, _ := b.currentSnapshot()
	d, _ := b.cat.Unit("armfav")
	c := b.hud.sidebarProductCatalog(b.cat, d, int(f.CommandPage.PageCount))
	group := c.groups[sidebarAssociation{sources[2], 1}]
	for _, source := range sources[:2] {
		if c.groups[sidebarAssociation{source, 1}] != group {
			t.Fatal("foreign shared commands did not connect their source groups")
		}
	}
	for i, source := range sources {
		for j, g := range source.Gadgets {
			old := originals[i][j]
			if g.Name != old.Name || g.QuickKey != old.QuickKey || g.Rect != old.Rect || g.Assoc != old.Assoc || g.ButtonArt != old.ButtonArt {
				t.Fatal("composition rewrote a source control")
			}
		}
	}
	b.viewerStep(0, b.cl)
	paletteCallbackToken(t, b, b.cl, 'm')
	if b.battleState().Input.Latch != input.LatchMove {
		t.Fatal("foreign scaffold lost command activation")
	}
	b.cl.SetEnhanced(false)
	if expandedWindow(t, b) != sources[0] {
		t.Fatal("Classic did not retain the authored foreign page")
	}
}

func TestExpandedSidebarForeignCommandsKeepSafetyChecks(t *testing.T) {
	for _, mismatch := range []string{"unknown control", "command attributes", "shared groups"} {
		t.Run(mismatch, func(t *testing.T) {
			b, sources := foreignSidebarFixture(t)
			switch mismatch {
			case "unknown control":
				for i, source := range sources[:2] {
					name := []string{"CORMODCONTROL", "ARMMODCONTROL"}[i]
					source.Gadgets = append(source.Gadgets, gui.Gadget{Kind: gui.KindButton, Name: name, Active: 1, Rect: gui.Rect{X: 64, Y: 282, W: 55, H: 31}})
				}
			case "command attributes":
				sources[1].Gadgets[expandedIndex(t, sources[1], "ARMMOVE")].Attribs ^= 0x10
			case "shared groups":
				sources[2].Gadgets[expandedIndex(t, sources[2], "ARMMOVE")].Assoc = 11
			}
			if expandedWindow(t, b) != sources[0] {
				t.Fatalf("composition hid %s", mismatch)
			}
		})
	}
}
