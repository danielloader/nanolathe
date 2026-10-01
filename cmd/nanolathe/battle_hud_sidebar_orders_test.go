package main

import (
	"slices"
	"testing"

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
