package aikit

import (
	"testing"

	"github.com/nanolathe-gg/nanolathe/internal/ai"
	"github.com/nanolathe-gg/nanolathe/internal/content"
	"github.com/nanolathe-gg/nanolathe/internal/world"
)

// Clearance belongs to the final footprint, not just the requested centre;
// exact contact with the reserved edge is allowed (DESIGN_SURVIVAL §16.8).
func TestAlliedTowerSpaceCoversFootprintsAndFiringLane(t *testing.T) {
	tower := &UnitInfo{Role: RoleDefense, FootX: 2, FootZ: 2}
	e := &executor{mapInfo: &MapInfo{HomeX: 256, HomeZ: 1024}, obs: &Obs{Allies: []AllyUnit{{Info: tower, X: 1024, Z: 1024}}}}
	e.prepareTowerSpace()
	for _, tc := range []struct {
		name         string
		x, z, fx, fz int32
		blocked      bool
	}{
		{"adjacent", 65, 63, 2, 2, true},
		{"centre clear footprint overlaps", 52, 63, 4, 2, true},
		{"reserved edge", 53, 63, 2, 2, false},
		{"outward firing lane", 77, 63, 2, 2, true},
		{"past firing lane", 81, 63, 2, 2, false},
		{"beside firing lane", 77, 70, 2, 2, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := e.blocksAllyTower(tc.x, tc.z, tc.fx, tc.fz); got != tc.blocked {
				t.Fatalf("blocked=%v, want %v", got, tc.blocked)
			}
		})
	}
	// The human's unfinished tower also reserves its eventual firing space.
	e.obs.Allies[0].Built = false
	e.prepareTowerSpace()
	if !e.blocksAllyTower(65, 63, 2, 2) {
		t.Fatal("nanoframe lost its reservation")
	}
	e.obs.Allies = nil
	e.prepareTowerSpace()
	if e.blocksAllyTower(65, 63, 2, 2) {
		t.Fatal("gone tower retained its reservation")
	}
}

func TestFinalPlacementCannotBypassAlliedTowerSpace(t *testing.T) {
	_, ter := pocketMap(new(Rand), 160, 160, 0)
	ex, _ := world.NewFootprintExtent(4, 4)
	yard, _ := world.ParseYardMap("oooooooooooooooo", 4, 4)
	p := &placeDef{ok: true, extent: ex, yard: yard, footX: 4, footZ: 4, rules: world.PlacementRules{MaxSlope: 100, MinWaterDepth: -10000, MaxWaterDepth: 10000}}
	info := &UnitInfo{FootX: 4, FootZ: 4}
	e := &executor{m: &ai.Manager{Terrain: ter}, mapInfo: &MapInfo{HomeX: 256, HomeZ: 1024}, places: []*placeDef{p}, obs: &Obs{Allies: []AllyUnit{{Info: &UnitInfo{Role: RoleDefense, FootX: 2, FootZ: 2}, X: 1024, Z: 1024}}}}
	x, z, ok := e.findSite(info, 1056, 1024, 2, nil, 1)
	if !ok {
		t.Fatal("no alternate site on an open map")
	}
	if e.blocksAllyTower(x, z, 4, 4) {
		t.Fatalf("searched site (%d,%d) entered tower space", x, z)
	}
	// Row placement and extractor searches use this same final predicate.
	if e.validAt(p, 64, 62) {
		t.Fatal("canonical final predicate admitted reserved footprint")
	}
}

func TestFinishedFeatureComesFromAuthoredConversion(t *testing.T) {
	wall := &content.FeatureDef{Blocking: true, Reclaimable: true, FootprintX: 2, FootprintZ: 2}
	unit := &content.UnitDef{IsFeature: true, Corpse: "Wall", MaxDamage: 100}
	cat := &content.Catalog{Units: map[string]*content.UnitDef{"wall": unit}, Features: map[string]*content.FeatureDef{"wall": wall}}
	tab := BuildTable(cat, nil)
	if tab.Lookup("wall").FinishedFeature != wall || !tab.defensiveFeatures[wall] {
		t.Fatal("authored wall conversion was not preserved")
	}
}
