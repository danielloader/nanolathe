package main

import (
	"testing"

	"github.com/nanolathe-gg/nanolathe/internal/gui"
)

// A builder with nine authored pages pages through retail's three-bit field:
// `.` from page 7 shows the zero field (the orders window with the page still
// shown), the next `.` reaches page 1, and `,` from the zero field reaches page
// 7. Page 8 is never selectable and nothing sticks [07 R-HUD-03 §6].
func TestNinePageKeysCycleThroughShownZeroField(t *testing.T) {
	b, _, _ := paletteCallbackFactory(t, []gui.Gadget{{Kind: gui.KindButton, Name: "ARMNEXT", Active: 1, QuickKey: 'n'}}, nil, 0, 9)
	state := func() (int, bool) {
		f, _ := b.currentSnapshot()
		return int(f.CommandPage.Page), commandPageIsPaged(f)
	}
	for want := 1; want <= 7; want++ {
		b.nextBuildPage()
		if page, paged := state(); page != want || !paged {
			t.Fatalf("`.` reached page %d shown=%v, want %d shown", page, paged, want)
		}
	}
	b.nextBuildPage()
	if page, paged := state(); page != 0 || !paged {
		t.Fatalf("`.` from page 7 of 9 reached page %d shown=%v, want the shown zero field", page, paged)
	}
	if got := commandWindowName("ARM", "ARMFAV", true, 0); got != "armgen" {
		t.Fatalf("shown zero field opens %q, want the orders window", got)
	}
	b.prevBuildPage()
	if page, paged := state(); page != 7 || !paged {
		t.Fatalf("`,` from the shown zero field reached page %d shown=%v, want 7", page, paged)
	}
	b.nextBuildPage()
	b.nextBuildPage()
	if page, paged := state(); page != 1 || !paged {
		t.Fatalf("`.` past the shown zero field reached page %d shown=%v, want 1", page, paged)
	}
}
