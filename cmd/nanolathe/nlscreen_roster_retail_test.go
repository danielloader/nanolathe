//go:build retail

package main

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/nanolathe-gg/nanolathe/internal/gameplay"
	"github.com/nanolathe-gg/nanolathe/internal/orders"
	"github.com/nanolathe-gg/nanolathe/internal/session"
	"github.com/nanolathe-gg/nanolathe/internal/testsupport"
)

func openNLZeroContent(t *testing.T) (Options, *contentSet) {
	t.Helper()
	roots := filepath.SplitList(os.Getenv("NANOLATHE_MOD_ROOTS_ZERO"))
	if len(roots) == 0 {
		t.Skip("installed TA Zero roots not supplied")
	}
	opts := Options{Root: testsupport.RetailRoot(t), Roots: roots, ModConfig: testsupport.ModConfigPath(t, "ta-zero-alpha5-20241224")}
	cs, err := openContent(opts)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cs.Close() })
	return opts, cs
}

func TestNLPreviewZeroAuthoredRoster(t *testing.T) {
	opts, cs := openNLZeroContent(t)
	for _, name := range []string{"armor", "construct", "glow", "naval", "hotwrecks", "metal", "blast", "placement", "arrival"} {
		t.Run(name, func(t *testing.T) {
			st, _, err := stageNLSession(opts, cs, nlPresets[name], gameplay.Modern, "")
			if err != nil {
				t.Fatal(err)
			}
			if st.roster.factions != [2]string{"GOK", "ARM"} {
				t.Fatalf("preview factions %v", st.roster.factions)
			}
			scope := st.assetUnitNames()
			if !slices.IsSorted(scope) || len(scope) == 0 {
				t.Fatalf("invalid asset roots %v", scope)
			}
			for _, unit := range st.s.Units.Iter() {
				if unit == nil || !unit.Alive || unit.Def == nil {
					continue
				}
				if !slices.Contains(scope, strings.ToLower(unit.Def.UnitName)) {
					t.Fatalf("unit %s outside asset scope", unit.Def.UnitName)
				}
				if q := orders.QueueForUnit(unit); q != nil {
					for _, node := range q.Primary() {
						if node.BuildDefKey != "" && !slices.Contains(st.roster.products(unit.Def), node.BuildDefKey) {
							t.Fatalf("%s queued unauthored product %s", unit.Def.UnitName, node.BuildDefKey)
						}
					}
				}
			}
			if name == "placement" && st.placementUnitName() == "" {
				t.Fatal("missing compatible placement tower")
			}
			if (name == "placement" || name == "arrival") && st.previewLimit() != "" {
				t.Fatalf("ordinary armed tower caused an incidental limit: %s", st.previewLimit())
			}
			if name == "construct" {
				for tick := 1; tick <= nlPresets[name].scene.PreTicks; tick++ {
					nlScriptTick(nlPresets[name], st.events, st.s, tick)
					for end := st.s.Clock.GlobalTick + 1; st.s.State != session.StatePostBattle && st.s.Clock.GlobalTick < end; {
						st.s.Step(st.s.Clock.ScaledAnchor + 1)
					}
				}
				working := false
				for _, u := range st.s.Units.Iter() {
					if u != nil && u.Alive && u.Remaining > 0 && u.Remaining < 1 {
						working = true
					}
				}
				if !working {
					t.Fatal("TA Zero worksite has no construction after its lead-in")
				}
			}
			t.Logf("roots=%v; limit=%s", scope, st.previewLimit())
		})
	}
}

func TestNLPreviewStockRosterPreferences(t *testing.T) {
	_, cs := openNLTestContent(t)
	cat, err := cs.nlPreviewCatalog()
	if err != nil {
		t.Fatal(err)
	}
	r := newNLRoster(cat, nil)
	if name := r.pick("armllt", 0, nlTower, nil); name != "armllt" {
		t.Errorf("stock placement tower replaced by %s", name)
	}
	var names []string
	for _, preset := range nlPresets {
		if preset.scene.PerSide > 0 {
			roster, err := filmRoster(preset.scene.Roster)
			if err != nil {
				t.Fatal(err)
			}
			names = append(names, roster[0]...)
			names = append(names, roster[1]...)
		}
		if preset.scene.Air > 0 {
			names = append(names, filmAircraft[0]...)
			names = append(names, filmAircraft[1]...)
		}
	}
	names = append(names, nlMetalUnits...)
	names = append(names, nlHotWreckUnits...)
	for _, name := range names {
		if got := r.resolve(name); got != name {
			t.Errorf("stock %s replaced by %s", name, got)
		}
	}
}
