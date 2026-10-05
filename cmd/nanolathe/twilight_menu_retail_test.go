//go:build retail

package main

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/nanolathe-gg/nanolathe/internal/frame"
	"github.com/nanolathe-gg/nanolathe/internal/session"
	"github.com/nanolathe-gg/nanolathe/internal/testsupport"
)

// Installed Beta 98 acceptance, including constructor and factory pages that
// inherit download records from the base install. No mod bytes are fixtures.
func TestRetailTwilightBuilderMenuSources(t *testing.T) {
	roots := filepath.SplitList(os.Getenv("NANOLATHE_MOD_ROOTS_TWILIGHT"))
	if len(roots) == 0 {
		t.Skip("installed Twilight roots not supplied")
	}
	opts := Options{Root: testsupport.RetailRoot(t), Roots: append([]string{testsupport.RetailRoot(t)}, roots...), Map: "ashap plateau", Seed: 7, ModConfig: modRootsConfigPath(t, "twilight")}
	cs, err := openContent(opts)
	if err != nil {
		t.Fatal(err)
	}
	defer cs.Close()
	for _, logical := range []string{"guis/armlab2.gui", "guis/corcom4.gui"} {
		info, err := cs.fs.Stat(logical)
		if err != nil || info.Size != 0 {
			t.Fatalf("Beta 98 empty page %s: size=%d error=%v", logical, info.Size, err)
		}
	}
	for side, faction := range []string{"ARM", "CORE"} {
		cfg := session.SkirmishConfig{MapName: opts.Map, NumPlayers: 2}
		cfg.Players[0].Side = side
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
		var keys []string
		for key := range cat.Units {
			keys = append(keys, key)
		}
		slices.Sort(keys)
		for _, key := range keys {
			def := cat.Units[key]
			if !strings.EqualFold(def.Side, faction) || !def.Builder || def.BuildPageCount < 2 {
				continue
			}
			for page := 1; page < int(def.BuildPageCount); page++ {
				var placements []frame.GeneratedProductPlacement
				for _, p := range cat.DownloadPlacementsForPage(key, page) {
					placements = append(placements, frame.GeneratedProductPlacement{ProductKey: p.Product, Button: p.Button})
				}
				name := commandWindowName(sideNamePrefix(h.side), def.UnitName, true, page)
				w, _, err := h.numberedPage(name, page, placements)
				if err != nil || w == nil {
					t.Fatalf("%s page %d: %v", key, page, err)
				}
				for _, p := range placements {
					i := int(p.Button) + 4
					if i < len(w.Gadgets) && sidebarProductSlot(w.Gadgets[i]) && !strings.EqualFold(w.Gadgets[i].Name, p.ProductKey) {
						t.Fatalf("%s page %d lost %s", key, page, p.ProductKey)
					}
				}
			}
		}
	}
}
