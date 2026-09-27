package main

import (
	"strings"

	"github.com/nanolathe-gg/nanolathe/internal/content"
)

// prepareFirstUseArt builds, at battle entry, the host art a player's first
// use of a panel would otherwise build on the game goroutine mid-battle, where
// the frame waits for it. Traced in play (docs/BATTLE_BENCHMARK.md "Live window
// trace"), the Megamap's icon bank cost an 88 ms freeze on the first Tab and a
// builder's first build page 7-9 ms. Entry already spends a long frame
// composing the battle, and these reads change nothing a later use would not;
// they only move when it happens. Art a battle cannot reach from entry — the
// Megamap switched on mid-battle, a captured builder of another side — still
// loads on first use.
func (b *battleSession) prepareFirstUseArt() {
	if b == nil {
		return
	}
	if b.megamapMode() {
		b.megamapIconBank()
	}
	b.hud.preloadBuildPages(b.cat)
}

// preloadBuildPages loads every numbered build page the viewing side's
// builders can open, with its page art, into the caches the side rail reads
// [07 R-HUD-03 §6]. A page that does not exist as a file is left to the
// generated-page path, which composes it when it is opened.
func (h *retailBattleHUD) preloadBuildPages(cat *content.Catalog) {
	if h == nil || h.fs == nil || h.side == nil || cat == nil {
		return
	}
	for _, u := range cat.UnitRecords() {
		if u == nil || u.BuildPageCount <= 1 || !strings.EqualFold(u.Side, h.side.Name) {
			continue
		}
		for page := 1; page < int(u.BuildPageCount); page++ {
			name := commandWindowName(h.side.NamePrefix, u.UnitName, true, page)
			if _, err := h.fs.Stat("guis/" + name + ".gui"); err != nil {
				continue
			}
			h.loadWindow(name)
		}
	}
}
