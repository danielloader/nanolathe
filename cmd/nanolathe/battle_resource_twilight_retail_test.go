//go:build retail

package main

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/nanolathe-gg/nanolathe/internal/gameplay"
	"github.com/nanolathe-gg/nanolathe/internal/session"
	"github.com/nanolathe-gg/nanolathe/internal/testsupport"
)

// Beta 98 reserves bonus extractors in CANBUILD without putting their buttons
// on the human pages (research/extensions/twilight-engine.md "Extractor bonuses
// and human shortcut admission"). The input gesture must preserve that boundary.
func TestTwilightResourceShortcutKeepsHumanExtractorIdentity(t *testing.T) {
	roots := filepath.SplitList(os.Getenv("NANOLATHE_MOD_ROOTS_TWILIGHT"))
	if len(roots) == 0 {
		t.Skip("installed Twilight roots not supplied")
	}
	opts := Options{Root: testsupport.RetailRoot(t), Roots: append([]string{testsupport.RetailRoot(t)}, roots...), Map: "ashap plateau", Seed: 7, ModConfig: modRootsConfigPath(t, "twilight"), Gameplay: gameplay.Community39, GameplaySet: true}
	cs, err := openContent(opts)
	if err != nil {
		t.Fatal(err)
	}
	defer cs.Close()
	cfg := session.SkirmishConfig{MapName: opts.Map, NumPlayers: 2, Gameplay: gameplay.Community39}
	cfg.Players[1].Controller = session.SkirmishControllerComputer
	cfg.ApplyDefaults()
	sess, cat, err := newBattleSessionWithConfig(opts, cs, cfg)
	if err != nil {
		t.Fatal(err)
	}
	h, err := loadRetailBattleHUD(cs.fs, sess, cat, retailPaletteForTest(t, cs), nil, newBattleWindowContext(cs, nil))
	if err != nil {
		t.Fatal(err)
	}
	b := &battleSession{sess: sess, cat: cat, hud: h}
	for _, side := range []struct{ builder, bonus string }{{"armcom", "aaimexx"}, {"corcom", "caimexx"}} {
		t.Run(side.builder, func(t *testing.T) {
			menu := cat.BuildMenus[side.builder]
			if menu == nil || !slices.ContainsFunc(menu.Buttons, func(key string) bool { return strings.EqualFold(key, side.bonus) }) {
				t.Fatal("fixture lost AI extractor membership")
			}
			bonus, _ := cat.Unit(side.bonus)
			if bonus == nil || bonus.EnergyMake != 500 {
				t.Fatal("fixture lost authored AI energy bonus")
			}
			mex, _, _ := b.resourceProducts(side.builder)
			if mex == nil || mex.CanonicalKey == side.bonus || mex.EnergyMake != 0 {
				t.Fatal("human resource shortcut did not choose an ordinary zero-energy extractor")
			}
		})
	}
}
