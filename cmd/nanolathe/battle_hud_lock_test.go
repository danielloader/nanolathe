package main

import (
	"slices"
	"testing"

	contentprofiles "github.com/nanolathe-gg/nanolathe/internal/content/profiles"
	"github.com/nanolathe-gg/nanolathe/internal/modlibrary"
	"github.com/nanolathe-gg/nanolathe/internal/settings"
)

// The build page lock is a Nanolathe host presentation policy (interface
// design §3.3 "Build page lock"), not a retail contract: the player's settings
// file wins over the mod, the mod over auto-flow, and a locked page never
// spans two authored pages.
func TestExpandedSidebarBuildPageLock(t *testing.T) {
	b, _, sources := sidebarRowsFixture(t, 1080)
	b.shell = &gameShell{presentation: settings.DefaultPresentation()}
	// Leave authored page two one product short.
	sources[1].Gadgets[9].Active = 0
	page := func(lock, userLock, n int) []string {
		t.Helper()
		b.modBuildPageSize, b.shell.presentation.BuildMenuPageSize = lock, userLock
		b.hud.retireExpandedSidebar()
		b.hud.sidebarPaging = sidebarRowPaging{}
		expandedWindow(t, b)
		if n > 1 {
			f, _ := b.currentSnapshot()
			b.hud.selectExpandedSidebarPage(b, f, n)
		}
		return sidebarVisibleProducts(expandedWindow(t, b))
	}

	if got := page(0, 0, 1); len(got) <= 6 {
		t.Fatalf("auto-flow page one = %v, want every row that fits", got)
	}
	if got := page(6, 0, 1); !slices.Equal(got, []string{"product0", "product1", "product2", "product3", "product4", "product5"}) {
		t.Fatalf("mod lock page one = %v", got)
	}
	if got := page(6, 0, 2); !slices.Equal(got, []string{"product6", "product7", "product8", "product9", "product10"}) {
		t.Fatalf("mod lock page two = %v, want the short authored page alone", got)
	}
	if got := page(6, 0, 3); got[0] != "product12" || b.hud.sidebarPaging.state.Count != 5 {
		t.Fatalf("mod lock page three = %v, pages %d", got, b.hud.sidebarPaging.state.Count)
	}
	if got := page(6, 4, 2); !slices.Equal(got, []string{"product4", "product5"}) {
		t.Fatalf("settings lock should override the mod: page two = %v", got)
	}
	// A lock taller than the rail keeps the authored/fitted layout.
	page(1000, 0, 1)
	if len(b.hud.sidebarPaging.cellStarts) != 0 {
		t.Fatalf("oversized lock still flowed %v", b.hud.sidebarPaging.cellStarts)
	}
}

func TestSidebarPageStarts(t *testing.T) {
	cells := make([]sidebarBuildCell, 0, 9)
	for _, p := range []int{1, 1, 1, 1, 2, 2, 2, 3, 3} {
		cells = append(cells, sidebarBuildCell{page: p})
	}
	if got := sidebarPageStarts(cells, 3, false); !slices.Equal(got, []int{0, 3, 6}) {
		t.Fatalf("auto-flow starts %v", got)
	}
	if got := sidebarPageStarts(cells, 3, true); !slices.Equal(got, []int{0, 3, 4, 7}) {
		t.Fatalf("locked starts %v", got)
	}
}

// The running content's lock is the mod's metadata value, else its content
// profile's (interface design §3.3 "Build page lock").
func TestContentBuildMenuPageSize(t *testing.T) {
	cs := &contentSet{presentation: contentprofiles.Presentation{BuildMenuPageSize: 12}}
	if got := cs.buildMenuPageSize(); got != 12 {
		t.Fatalf("profile lock without a mod = %d, want 12", got)
	}
	cs.mod = &modlibrary.Mod{Metadata: modlibrary.Metadata{BuildMenuPageSize: 6}}
	if got := cs.buildMenuPageSize(); got != 6 {
		t.Fatalf("mod lock = %d, want the mod's 6 over the profile's 12", got)
	}
}
