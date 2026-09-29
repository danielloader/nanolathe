package main

import (
	"testing"

	"github.com/nanolathe-gg/nanolathe/internal/gui"
	"github.com/nanolathe-gg/nanolathe/internal/hud"
	"github.com/nanolathe-gg/nanolathe/internal/input"
	"github.com/nanolathe-gg/nanolathe/internal/sim/numeric"
	"github.com/nanolathe-gg/nanolathe/vfs"
)

// Exercise the retained product callback, not only its count helper: the
// optional host preference must reach the ordinary signed factory command.
func TestZeroFactoryBatchReachesPaletteCommand(t *testing.T) {
	buttons := []gui.Gadget{{Kind: gui.KindButton, Name: "ARMFAV", Active: 1, Rect: gui.Rect{X: 1, Y: 1, W: 20, H: 12}}}
	b, cl, factory := paletteCallbackFactory(t, buttons, []string{"armfav"}, 0, 1)
	if b.shell == nil {
		b.shell = &gameShell{}
	}
	w := b.hud.windows["armfav1"]
	cl.Input().Kbd.SetKey(input.KeyCtrl, true)
	for _, tc := range []struct {
		enabled int
		right   bool
		want    int
	}{{0, false, 5}, {1, false, 100}, {1, true, -100}} {
		b.shell.presentation.FactoryHundredBatch = tc.enabled
		before := len(paletteFactoryCommands(b))
		paletteCallbackClick(t, b, cl, w, 1, tc.right, true)
		pending := paletteFactoryCommands(b)
		if len(pending) != before+1 {
			t.Fatalf("product click enqueued %d commands, want one", len(pending)-before)
		}
		got := pending[before].FactoryBuild
		if got.Builder != factory || got.Product != "armfav" || got.Count != tc.want {
			t.Fatalf("factory batch command = %+v, want count %d", got, tc.want)
		}
	}
}

// A ProTA player's Ctrl+Shift+click on a mobile product in a factory's menu
// queues a hundred once the Community (ProTA) preset is applied; without the
// preference it stays retail's Shift batch of five [07 R-P0-11 §1]. The
// signed count reaches the ordinary counted producer, whose handling of a
// count is the session's (DESIGN_INTERFACE_HUD_INPUT §3.13).
func TestCommunityPresetFactoryHundredBatchForMobileProduct(t *testing.T) {
	resetUnitInfoState(t)
	b := newTestBattle(testCatalogON05(), testWorldON05(20, 20))
	b.millisSource = &fakeMillisSource{}
	w := &gui.Window{Rect: gui.Rect{W: 128, H: 80}, Gadgets: []gui.Gadget{{Kind: gui.KindPanel}, {Kind: gui.KindButton, Name: "ARMFAV", Active: 1, Rect: gui.Rect{X: 1, Y: 1, W: 20, H: 12}}}}
	b.hud = &retailBattleHUD{fs: vfs.New(), cat: b.cat, windows: map[string]*gui.Window{"gen": w, "armfac1": w}}
	b.shell = presetTestShell(t)
	b.cat.Units["armfac"].BuildPageCount = 2 // one authored build page after orders
	fac := placeUnit(b, "armfac", numeric.Fixed(96<<16), numeric.Fixed(96<<16))
	if product, _ := b.cat.Unit("armfav"); hud.ProductArmsPlacement(product) || fac.Def.BMCode != 0 || fac.Def.CanMove {
		t.Fatal("fixture must be a mobile product in a structure factory")
	}
	replaceSelectionForTest(t, b, fac)
	cl := b.cl
	b.viewerStep(0, cl)
	if f, _ := b.currentSnapshot(); f.CommandPage.Builder != fac.Handle {
		t.Fatalf("factory command page not published: %+v", f.CommandPage)
	}
	cl.Input().Kbd.SetKey(input.KeyCtrl, true)
	click := func(right bool) int {
		t.Helper()
		before := len(paletteFactoryCommands(b))
		paletteCallbackClick(t, b, cl, w, 1, right, true)
		pending := paletteFactoryCommands(b)
		if len(pending) != before+1 || pending[before].FactoryBuild.Builder != fac.Handle || pending[before].FactoryBuild.Product != "armfav" {
			t.Fatalf("Ctrl+Shift product click enqueued %+v", pending[before:])
		}
		return pending[before].FactoryBuild.Count
	}
	if got := click(false); got != 5 {
		t.Fatalf("default preferences: Ctrl+Shift count %d, want Shift's 5", got)
	}
	b.shell.applyControlsPreset(controlsPresetCommunity)
	if got := click(false); got != 100 {
		t.Fatalf("Community preset: Ctrl+Shift count %d, want 100", got)
	}
	if got := click(true); got != -100 {
		t.Fatalf("Community preset: Ctrl+Shift right count %d, want -100", got)
	}
}
