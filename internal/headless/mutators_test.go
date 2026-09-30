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

// TestSightMutatorReachesTheLOSRasters: Sight scales the sightdistance both
// rasters quantize when a unit publishes, through the battle's catalog clone,
// in Strict 3.1 as in every mode. Circular line of sight grows with it. True
// line of sight stops at the last reachable LOS.TDF table, TABLE8 or
// sightdistance 256 with the stock nine, which the Arm commander's 290
// already selects, so doubling it changes nothing there while halving it
// shrinks the footprint [03 §3.2][03 R-COMP-02 §1]
// (docs/DESIGN_MODS_MUTATORS.md §6.5). Skipped without retail assets.
func TestSightMutatorReachesTheLOSRasters(t *testing.T) {
	cat, fs := retailcat.Shared(t)
	covered := func(losType int, sight content.Factor) int {
		t.Helper()
		cfg := session.DirectSkirmishConfig("metal heck")
		cfg.ApplyDefaults()
		cfg.LOSType = losType
		fb, err := ComposeFreshBattle(FreshBattleRequest{
			Kind: ScenarioDirectOTA, Map: cfg.MapName, Skirmish: cfg, Gameplay: gameplay.Strict31,
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
	if c1, c2 := covered(0, one), covered(0, two); c2 <= c1 {
		t.Fatalf("Circular coverage %d at Sight x2, want more than %d at x1", c2, c1)
	}
	if t1, t2, th := covered(1, one), covered(1, two), covered(1, half); t2 != t1 || th >= t1 {
		t.Fatalf("True coverage x0.5/x1/x2 = %d/%d/%d, want the x2 footprint equal to x1 (both at the top table) and x0.5 smaller", th, t1, t2)
	}
}
