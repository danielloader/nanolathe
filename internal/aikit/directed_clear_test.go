package aikit

import (
	"testing"

	"github.com/nanolathe-gg/nanolathe/internal/ai"
	"github.com/nanolathe-gg/nanolathe/internal/content"
	"github.com/nanolathe-gg/nanolathe/internal/orders"
	"github.com/nanolathe-gg/nanolathe/internal/pool"
	"github.com/nanolathe-gg/nanolathe/internal/units"
	"github.com/nanolathe-gg/nanolathe/internal/world"
)

// Directed cleanup admits an observed zero-metal blocker, preserves a wall,
// and never adds an unseen blocker to the ordinary reclaim orders.
func TestDirectedCleanupOnlyReclaimsObservedNondefensiveBlockers(t *testing.T) {
	for _, tc := range []struct {
		name                            string
		observed, defensive, wide, want bool
	}{
		{"observed obstacle", true, false, false, true},
		{"unseen obstacle", false, false, false, false},
		{"authored defensive wall", true, true, false, false},
		{"broad resource collection", true, false, true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newGenFixture(t, &countBrain{})
			f.def.CanReclamate = true
			feature := &content.FeatureDef{Blocking: true, Reclaimable: true, FootprintX: 1, FootprintZ: 1}
			ter := &world.Terrain{CellW: 32, CellH: 32, Plot: make([]world.PlotCell, 32*32), FeatureDefs: []*content.FeatureDef{feature}}
			for i := range ter.Plot {
				ter.Plot[i].SetFeature(world.PlotFeatureNone)
			}
			ter.PlotAt(8, 8).SetFeature(0)
			obs := &Obs{}
			if tc.observed {
				obs.Features = []Feature{{X: 136, Z: 136, FootX: 1, FootZ: 1, Blocking: true, Reclaimable: true, Defensive: tc.defensive}}
			}
			tab := &Table{defensiveFeatures: map[*content.FeatureDef]bool{feature: tc.defensive}}
			e := &executor{m: &ai.Manager{Player: 0, Terrain: ter}, mapInfo: &MapInfo{CellW: 32, CellH: 32}, obs: obs, table: tab}
			u := f.w.Unit(f.own)
			b := &batch{actors: []pool.Handle{f.own}, inst: []*units.Unit{u}}
			c := &Command{Kind: CmdClear, first: 0, count: 1, X: 136, Z: 136, Count: 96}
			if tc.wide {
				c.Count = 512
			}
			if got := e.execClear(c, b, 30, f.w); got != tc.want {
				t.Fatalf("issued=%v want=%v", got, tc.want)
			}
			q := orders.QueueOfUnit(u)
			if tc.want && (q == nil || q.Head() == nil || orders.Table()[q.Head().ID].Name != "Reclaim") {
				t.Fatal("cleanup did not use an ordinary Reclaim order")
			}
		})
	}
}
