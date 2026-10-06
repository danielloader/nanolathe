package headless

import (
	"errors"
	"testing"

	"github.com/nanolathe-gg/nanolathe/internal/content"
	"github.com/nanolathe-gg/nanolathe/internal/gameplay"
	"github.com/nanolathe-gg/nanolathe/internal/session"
	"github.com/nanolathe-gg/nanolathe/internal/testsupport/retailcat"
	"github.com/nanolathe-gg/nanolathe/internal/visibility"
)

// TestReportCarriesTheBoundMutators: the report prints the canonical set the
// session bound, and nothing for an unmutated battle
// (docs/DESIGN_MODS_MUTATORS.md §6.6).
func TestReportCarriesTheBoundMutators(t *testing.T) {
	request := Request{Map: "synthetic", SimulationSeed: 17, CRTSeed: 19, TickLimit: 1}
	for _, tc := range []struct {
		m    content.Mutators
		want string
	}{
		{content.Mutators{}, ""},
		{content.Mutators{BuildSpeed: content.Factor{Num: 3, Den: 2}, BuildCost: content.Factor{Num: 4, Den: 1}}, "buildCost=4,buildSpeed=1.5"},
	} {
		sess := syntheticSession(request)
		sess.Mutators = tc.m
		report, err := RunSession(request, sess)
		if !errors.Is(err, ErrTickLimit) {
			t.Fatalf("RunSession error = %v, want tick limit", err)
		}
		if report.Mutators != tc.want {
			t.Fatalf("report mutators = %q, want %q", report.Mutators, tc.want)
		}
	}
}

// TestDisplaylessRequestForwardsMutators: a request's mutators reach the
// session entry, in Strict 3.1 as in every mode, and move the reported
// catalog identity; a request without them runs on the unchanged catalog, so
// the fingerprint locks' catalog is untouched. Skipped without retail assets.
func TestDisplaylessRequestForwardsMutators(t *testing.T) {
	cat, fs := retailcat.Shared(t)
	m := content.Mutators{BuildSpeed: content.Factor{Num: 2, Den: 1}}
	for _, tc := range []struct {
		m       content.Mutators
		mutated bool
	}{{content.Mutators{}, false}, {m, true}} {
		request := Request{Map: "ashap plateau", Gameplay: gameplay.Strict31, SimulationSeed: 7, CRTSeed: 7, TickLimit: 1, Mutators: tc.m}
		report, err := RunWithContent(request, fs, cat)
		if !errors.Is(err, ErrTickLimit) {
			t.Fatalf("RunWithContent error = %v, want tick limit", err)
		}
		if report.Mutators != tc.m.String() || (report.CatalogHash != cat.Hash) != tc.mutated {
			t.Fatalf("mutators %q catalog %q (base %q), want %q mutated=%v", report.Mutators, report.CatalogHash, cat.Hash, tc.m.String(), tc.mutated)
		}
	}
}

// TestSightMutatorReachesTheLOSRasters: Sight scales the definition and extends
// the per-battle raster inputs in every mode. Both True and Circular sight
// grow above identity and shrink below it (docs/DESIGN_MODS_MUTATORS.md §6.5).
// Without mutators the authored caps remain [03 §3.2][03 R-COMP-02 §1].
// Skipped without retail assets.
func TestSightMutatorReachesTheLOSRasters(t *testing.T) {
	cat, fs := retailcat.Shared(t)
	covered := func(mode gameplay.Mode, losType int, sight content.Factor) int {
		t.Helper()
		cfg := session.DirectSkirmishConfig("metal heck")
		cfg.ApplyDefaults()
		cfg.LOSType = losType
		fb, err := ComposeFreshBattle(FreshBattleRequest{
			Kind: ScenarioDirectOTA, Map: cfg.MapName, Skirmish: cfg, Gameplay: mode,
			LocalOwner: -1, SimulationSeed: 7, CRTSeed: 7, FS: fs, Catalog: cat,
			Mutators: content.Mutators{Sight: sight},
		})
		if err != nil {
			t.Fatal(err)
		}
		// Battle entry has published the commanders; only the viewer's own
		// observers reach its byte grid [03 §3.2].
		n := 0
		for _, b := range fb.Session.Vis.ByteGrid(visibility.PlayerID(fb.Session.ViewingOwner)) {
			if b != 0 {
				n++
			}
		}
		return n
	}
	half, one, two := content.Factor{Num: 1, Den: 2}, content.Factor{}, content.Factor{Num: 2, Den: 1}
	for _, mode := range []gameplay.Mode{gameplay.Modern, gameplay.Community39, gameplay.Strict31} {
		for _, losType := range []int{0, 1} {
			h, o, d := covered(mode, losType, half), covered(mode, losType, one), covered(mode, losType, two)
			if h >= o || d <= o {
				t.Fatalf("mode %v LOS %d coverage x0.5/x1/x2 = %d/%d/%d, want increasing coverage", mode, losType, h, o, d)
			}
		}
	}
}
