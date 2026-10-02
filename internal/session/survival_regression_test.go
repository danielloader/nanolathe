package session

import (
	"github.com/nanolathe-gg/nanolathe/formats"
	"github.com/nanolathe-gg/nanolathe/internal/pool"
	"github.com/nanolathe-gg/nanolathe/internal/sim/rng"
	"github.com/nanolathe-gg/nanolathe/internal/survival"
	"github.com/nanolathe-gg/nanolathe/internal/units"
	"github.com/nanolathe-gg/nanolathe/internal/world"
	"testing"
)

func TestSurvivalDropsFullWaveTail(t *testing.T) {
	cat := minimalCatalogForStrict()
	def := cat.Units["armcom"]
	def.Limit = -1
	s := &Session{Catalog: cat, World: minimalTerrain(), Units: units.NewSliced(2, cat)}
	r := rng.NewSimulation(7)
	s.Units.SetSimulationRNG(&r)
	for i := 0; i < 2; i++ {
		if _, err := s.Units.Create(def, 1, 0, 0, 0); err != nil {
			t.Fatalf("filling slice: %v", err)
		}
	}
	if _, err := s.Units.Create(def, 1, 0, 0, 0); err == nil {
		t.Fatal("fixture did not fill attacker slice")
	}
	st := &survivalState{attacker: 1, centreX: 16, centreZ: 16, tuning: survival.DefaultTuning(survival.PaceNormal), phase: survivalActive}
	st.pool = survival.Pool{Units: []survival.Unit{{Def: def, Key: "armcom", Domain: survival.Air, Cost: 100}}}
	st.plan = survival.Wave{Groups: []survival.Group{{Domain: survival.Air, Picks: make([]int, 100)}}}
	s.Survival = st
	ex, ez := s.survivalEntryCell(0)
	if _, _, _, ok := s.survivalFindCell(def, nil, 0, ex, ez, survivalSpawnReach); !ok {
		t.Fatal("fixture lacks spawn site")
	}
	draws := r.Draws()
	s.survivalSpawn(100)
	if st.nextG < len(st.plan.Groups) {
		t.Fatalf("full attacker slice retained wave tail: next group=%d pick=%d of %d", st.nextG, st.nextP, len(st.plan.Groups[0].Picks))
	}
	if r.Draws() != draws {
		t.Fatal("discarding a capped wave consumed RNG")
	}
	lo, _, _ := s.Units.SliceForPlayer(1)
	s.Units.FreeImmediate(pool.Handle(lo))
	s.survivalSpawn(101)
	if got := s.Units.LiveCountForPlayer(1); got != 1 {
		t.Fatalf("discarded tail resumed after a casualty: live=%d", got)
	}
}

func TestSurvivalCommanderUsesOwnMovementRegion(t *testing.T) {
	cat := minimalCatalogForStrict()
	human := cat.Units["armcom"]
	buddy := cat.Units["corcom"]
	human.Limit = -1
	buddy.Limit = -1
	land := cat.Movement["testmove"]
	land.MaxSlope = 255
	land.MaxWaterSlope = 255
	amph := *land
	amph.MaxWaterDepth = 10000
	amph.CanonicalKey = "amph"
	cat.Movement["amph"] = &amph
	buddy.MovementClass = "amph"
	attrs := make([]formats.TNTAttribute, 32*32)
	for z := 0; z < 32; z++ {
		for x := 0; x < 32; x++ {
			h := uint8(40)
			if x == 3 || x == 4 {
				h = 0
			}
			attrs[z*32+x] = formats.TNTAttribute{Height: h, Feature: world.PlotFeatureNone}
		}
	}
	ter := &world.Terrain{CellW: 32, CellH: 32, SeaLevel: 30, Plot: world.ExpandPlot(attrs, 32, 32)}
	s := &Session{Catalog: cat, World: ter, Units: units.NewSliced(3, cat)}
	s.Survival = &survivalState{attacker: 2, tuning: survival.DefaultTuning(survival.PaceNormal)}
	s.Survival.tuning.BuddyRing = 3
	cfg := SurvivalSkirmishConfig("fixture", 1, SurvivalOptions{Enabled: true})
	if err := s.survivalChooseSite(cfg); err != nil {
		t.Fatal(err)
	}
	hc, bc := s.survivalClassFor(human), s.survivalClassFor(buddy)
	if hc.regions.Largest() != 2 || bc.regions.Largest() != 1 {
		t.Fatalf("fixture labels human=%d buddy=%d", hc.regions.Largest(), bc.regions.Largest())
	}
	if err := s.placeSurvivalCommanders(cfg); err != nil {
		t.Fatalf("buddy with fully passable movement class cannot be placed because region IDs differ: %v", err)
	}
}

// Reaching the cap on this tick's final allowed creation must discard all
// later directions now, before another tick can free a slot (§6.5).
func TestSurvivalDropsTailOnFinalCreation(t *testing.T) {
	cat := minimalCatalogForStrict()
	def := cat.Units["armcom"]
	def.Limit = -1
	s := &Session{Catalog: cat, World: minimalTerrain(), Units: units.NewSliced(2, cat)}
	st := &survivalState{attacker: 1, centreX: 16, centreZ: 16, tuning: survival.DefaultTuning(survival.PaceNormal), phase: survivalActive}
	st.pool = survival.Pool{Units: []survival.Unit{{Def: def, Key: "armcom", Domain: survival.Air, Cost: 100}}}
	st.plan = survival.Wave{Groups: []survival.Group{{Domain: survival.Air, Picks: []int{0, 0}}, {Domain: survival.Air, Picks: []int{0, 0}}}}
	s.Survival = st
	s.survivalSpawn(100)
	if got := s.Units.LiveCountForPlayer(1); got != 2 {
		t.Fatalf("created %d units, want cap 2", got)
	}
	if st.nextG != len(st.plan.Groups) {
		t.Fatalf("cap reached at tick budget but retained group %d", st.nextG)
	}
}
