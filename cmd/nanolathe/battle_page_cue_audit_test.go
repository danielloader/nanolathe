package main

import (
	"testing"

	"github.com/nanolathe-gg/nanolathe/internal/input"
	"github.com/nanolathe-gg/nanolathe/internal/units"
)

// Named UI cues use authored samples and an accepting output here. The
// assertions lock producer requests, not availability or playback policy
// [07 R-HUD-03 §6].
func TestPageCycleCueWithoutLiveOwner(t *testing.T) {
	for _, released := range []bool{false, true} {
		t.Run(map[bool]string{false: "no-selection", true: "released-owner"}[released], func(t *testing.T) {
			b := newTestBattle(testCatalogON05(), testWorldON05(40, 40))
			if released {
				u := placeUnit(b, "armcons", 200<<16, 120<<16)
				replaceSelectionForTest(t, b, u)
				b.sess.Units.Destroy(u.Handle, units.DeathKilled)
			}
			applyPendingBattleCommands(b)
			f, ok := b.currentSnapshot()
			if !ok || f.CommandPage.Builder != 0 {
				t.Fatal("fixture retained a live page owner")
			}
			spy := paletteCallbackCues(t, b, cueNextBuildMenu)
			for _, key := range []input.Key{input.KeyPeriod, input.KeyComma} {
				pressKeys(b, key)
			}
			if len(spy.aliases) != 2 {
				t.Fatalf("ownerless next/previous requests=%v, want two", spy.aliases)
			}
			pressKeys(b, input.Key1)
			if len(spy.aliases) != 2 {
				t.Fatal("ownerless digit request emitted a cue")
			}
		})
	}
}

func TestPageDigitCueAdmissionAndRegularTransitions(t *testing.T) {
	b := newTestBattle(testCatalogON05(), testWorldON05(40, 40))
	b.cat.Units["armcons"].BuildPageCount = 3
	u := placeUnit(b, "armcons", 200<<16, 120<<16)
	replaceSelectionForTest(t, b, u)
	spy := paletteCallbackCues(t, b, cueNextBuildMenu)
	cases := []struct {
		key  input.Key
		page uint16
		cues int
	}{
		{input.Key2, 1, 1}, // valid, already shown
		{input.Key2, 1, 2}, // a second accepted request still cues
		{input.Key9, 1, 2}, // unavailable digit page is silent
		{input.Key1, 0, 3},
		{input.KeyPeriod, 1, 4},
		{input.KeyComma, 0, 5},
	}
	for _, tc := range cases {
		pressKeys(b, tc.key)
		f, _ := b.currentSnapshot()
		if f.CommandPage.Page != tc.page || len(spy.aliases) != tc.cues {
			t.Fatalf("key=%v page=%d cues=%v, want page=%d cues=%d", tc.key, f.CommandPage.Page, spy.aliases, tc.page, tc.cues)
		}
		if len(b.sess.PendingHumanCommands()) != 0 {
			t.Fatal("page input entered the authoritative command queue")
		}
	}
}

func TestPageCuePreservesAdaptiveSidebarChangePolicy(t *testing.T) {
	b, _ := sidebarNavigationFixture(t)
	spy := paletteCallbackCues(t, b, cueNextBuildMenu)
	// The adaptive view already shows page 1. Its local host policy remains
	// change-only even though the authored-page path cues an accepted repeat.
	b.switchBuildPage(2)
	if len(spy.aliases) != 0 {
		t.Fatal("unchanged adaptive page requested a cue")
	}
	b.switchBuildPage(1)
	if len(spy.aliases) != 1 {
		t.Fatal("changed adaptive page did not request its cue")
	}
}
