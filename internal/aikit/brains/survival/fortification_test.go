package survival

import (
	"testing"

	"github.com/nanolathe-gg/nanolathe/internal/aikit"
	"github.com/nanolathe-gg/nanolathe/internal/aikit/core"
	"github.com/nanolathe-gg/nanolathe/internal/content"
	"github.com/nanolathe-gg/nanolathe/internal/pool"
)

// These authored fixtures lock the Modern AI policy in DESIGN_SURVIVAL
// §16.8. Their names and statistics describe no retail unit.
func (st *state) pickTower(b *core.Board, builder *aikit.UnitInfo, aa bool) *aikit.UnitInfo {
	return st.pickTowerBudget(b, builder, aa, 1<<62)
}

func fortificationTable(units ...*aikit.UnitInfo) table {
	for i, u := range units {
		u.Index = int32(i)
	}
	var tab table
	tab.build(&aikit.Kit{Table: &aikit.Table{Units: units}})
	return tab
}

func fortificationTower(metal, energy, hp, dps, reach int32, aa bool) *aikit.UnitInfo {
	w := &content.WeaponDef{ID: 1, DamageDefault: dps, Burst: 1, ReloadTime: 30, Range: reach, ToAirWeapon: aa}
	u := &aikit.UnitInfo{Def: &content.UnitDef{Weapon1Def: w}, Role: aikit.RoleDefense,
		Metal: metal, Energy: energy, Value: metal + energy/aikit.EnergyPerMetal, HP: hp, DPS: dps, Range: reach, FootX: 2, FootZ: 2}
	if aa {
		u.AirDPS = dps
	}
	return u
}

func fortificationWall() *aikit.UnitInfo {
	return &aikit.UnitInfo{Def: &content.UnitDef{IsFeature: true}, Value: 20, Metal: 20, FootX: 2, FootZ: 2,
		FinishedFeature: &content.FeatureDef{Blocking: true, FootprintX: 2, FootprintZ: 2, Damage: 2000}}
}

func TestTowerProgressionRespectsEachResourceAndConfiguredDebt(t *testing.T) {
	light := fortificationTower(80, 6000, 600, 50, 400, false)
	heavy := fortificationTower(800, 36000, 5000, 220, 800, false)
	unaffordable := fortificationTower(3000, 60000, 18000, 1000, 1200, false)
	st := &state{tab: fortificationTable(light, heavy, unaffordable)}
	builder := &aikit.UnitInfo{Builds: []*aikit.UnitInfo{light, heavy, unaffordable}}
	b := &core.Board{Metal: aikit.Res{Income: 2}, Energy: aikit.Res{Income: 120}}
	if got := st.pickTower(b, builder, false); got != light {
		t.Fatalf("small economy picked %p, want affordable light tower", got)
	}
	b.Metal.Income, b.Energy.Income = 12, 600
	if got := st.pickTower(b, builder, false); got != heavy {
		t.Fatalf("developed economy picked %p, want affordable heavy tower", got)
	}
	if got := st.pickTowerBudget(b, builder, false, int64(light.Value)); got != light {
		t.Fatal("heavy upgrade postponed the smaller coverage owed by the tower share")
	}
	// Abundant energy cannot pay a metal cost, and abundant metal cannot
	// pay an energy cost. The affordable light tower still supplies coverage.
	b.Metal.Income, b.Energy.Income = 1, 10000
	if got := st.pickTower(b, builder, false); got != light {
		t.Fatal("energy income funded a metal-heavy upgrade")
	}
	b.Metal.Income, b.Energy.Income = 100, 100
	if got := st.pickTower(b, builder, false); got != light {
		t.Fatal("metal income funded an energy-heavy upgrade")
	}
	b.Metal, b.Energy = aikit.Res{Stock: 1000}, aikit.Res{Stock: 48000}
	if got := st.pickTower(b, builder, false); got != heavy {
		t.Fatal("banked resources did not admit the affordable upgrade")
	}
}

func TestTowerSelectionValuesRangeAndAirCoverage(t *testing.T) {
	short := fortificationTower(100, 0, 1000, 100, 300, false)
	long := fortificationTower(100, 0, 1000, 100, 900, false)
	aa := fortificationTower(100, 0, 1000, 200, 700, true)
	st := &state{tab: fortificationTable(short, long, aa)}
	b := &core.Board{Metal: aikit.Res{Stock: 1000}}
	builder := &aikit.UnitInfo{Builds: []*aikit.UnitInfo{short, long, aa}}
	if got := st.pickTower(b, builder, false); got != long {
		t.Fatal("equal-cost ground towers did not prefer longer range")
	}
	if got := st.pickTower(b, builder, true); got != aa {
		t.Fatal("air coverage did not select the authored anti-air tower")
	}
}

func TestWallClassificationRequiresSolidFinishedFeature(t *testing.T) {
	wall := fortificationWall()
	decor := fortificationWall()
	decor.FinishedFeature = &content.FeatureDef{FootprintX: 2, FootprintZ: 2, Damage: 2000}
	missing := fortificationWall()
	missing.FinishedFeature = nil
	nonsolid := fortificationWall()
	nonsolid.FinishedFeature = &content.FeatureDef{Blocking: true, Damage: 2000}
	utility := fortificationWall()
	utility.Role = aikit.RoleJammer
	water := fortificationWall()
	water.Def.MinWaterDepth = 1
	st := &state{tab: fortificationTable(wall, decor, missing, nonsolid, utility, water)}
	for _, u := range []*aikit.UnitInfo{decor, missing, nonsolid, utility, water} {
		if st.tab.of(u).wall {
			t.Fatalf("non-wall definition %+v admitted as defensive wall", u)
		}
	}
	if !st.tab.of(wall).wall || st.wallPiece(&aikit.UnitInfo{Builds: []*aikit.UnitInfo{decor, missing, wall}}) != wall {
		t.Fatal("authored solid wall was not selected from its build menu")
	}
}

func fortificationState(t *testing.T) (*state, *core.Board, *aikit.OwnUnit) {
	t.Helper()
	st, b := allyState(t, &aikit.Obs{})
	tower, wall := fortificationTower(100, 0, 1000, 100, 500, false), fortificationWall()
	st.tab = fortificationTable(tower, wall)
	b.O.Own = []aikit.OwnUnit{{Info: tower, X: st.cx + towerRoom, Z: st.cz, Built: true}}
	b.Metal.Stock = 1000
	st.tick = towerStart
	st.weight[0] = 6000
	st.towers[0] = 1
	u := &aikit.OwnUnit{Info: &aikit.UnitInfo{Builds: []*aikit.UnitInfo{wall}}}
	return st, b, u
}

func TestFirstWallSegmentNeedsCompletedGroundCoverage(t *testing.T) {
	st, b, u := fortificationState(t)
	if s, _, ok := st.nextWall(b, u); !ok || s != 0 {
		t.Fatal("one completed ground tower did not admit its first wall segment")
	}
	pts := st.wallSite(b, 0, nil)
	if len(pts) != wallPieces || pts[1] != [2]int32{st.cx + towerRoom + wallAhead, st.cz} {
		t.Fatalf("first segment %v does not stand in front of its tower", pts)
	}
	b.O.Own[0].Built = false
	if _, _, ok := st.nextWall(b, u); ok {
		t.Fatal("unfinished tower admitted a wall")
	}
	b.O.Own[0].Built = true
	st.tab.of(b.O.Own[0].Info).grange = wallAhead - 1
	if _, _, ok := st.nextWall(b, u); ok {
		t.Fatal("tower out of range admitted a wall")
	}
	st.tab.of(b.O.Own[0].Info).grange = 500
	st.defense.wallN[0] = 1
	if _, _, ok := st.nextWall(b, u); ok {
		t.Fatal("one tower admitted a second segment")
	}
	st.defense.wallN[0] = 0
	st.p.Walls = 0
	if _, _, ok := st.nextWall(b, u); ok {
		t.Fatal("disabled wall policy still planned a segment")
	}
}

func TestWallSegmentsPreserveCorridorsAndAlliedFactoryLanes(t *testing.T) {
	st, b, _ := fortificationState(t)
	st.defense.wallFootX, st.defense.wallFootZ = 2, 2
	st.defense.wallBuildX, st.defense.wallBuildZ = 2, 2
	// A centre off the construction grid exercises the final snapped sites.
	st.cx += 7
	st.cz += 9
	tower := b.O.Own[0].Info
	b.O.Own = nil
	for _, d := range sectorDir {
		b.O.Own = append(b.O.Own, aikit.OwnUnit{Info: tower, X: st.cx + int32(d[0]*towerRoom/1000), Z: st.cz + int32(d[1]*towerRoom/1000), Built: true})
	}
	for s := range sectorDir {
		for n := int32(0); n < maxWallSegments; n++ {
			st.defense.wallN[s] = n
			pts := st.wallSite(b, s, nil)
			if len(pts) != wallPieces {
				t.Fatalf("sector %d segment %d has %d pieces", s, n, len(pts))
			}
			for k, p := range pts {
				if p[0]%16 != 0 || p[1]%16 != 0 {
					t.Fatal("wall geometry checked an unsnapped construction centre")
				}
				for _, prev := range pts[:k] {
					if absI(int64(p[0]-prev[0])) < 32 && absI(int64(p[1]-prev[1])) < 32 {
						t.Fatalf("sector %d has overlapping wall pieces %v and %v", s, prev, p)
					}
				}
			}
			neighbor := (s + 1) % numSectors
			for nn := int32(0); nn < maxWallSegments; nn++ {
				st.defense.wallN[neighbor] = nn
				for _, a := range pts {
					for _, c := range st.wallSite(b, neighbor, nil) {
						// Both axis-aligned footprints have a 23 wu bounding
						// radius: a 64 wu gap between those circles is open
						// between the wall pieces in every orientation.
						if aikit.Dist2(a[0], a[1], c[0], c[1]) < (64+2*23)*(64+2*23) {
							t.Fatalf("neighboring segments close the corridor: %v and %v", a, c)
						}
					}
				}
			}
		}
	}
	// The centre may move around an allied factory, but each piece keeps
	// clear of its exit lane after that move.
	st.defense.wallN[0] = 0
	st.ally.boxes = []keepOut{{x0: st.cx + 700, z0: st.cz - 64, x1: st.cx + 900, z1: st.cz + 64, lane: true}}
	for _, p := range st.wallSite(b, 0, nil) {
		if !st.outOfLanes(p[0], p[1]) {
			t.Fatal("wall piece was planned inside an allied factory exit")
		}
	}
}

func TestAlliedTowerReservationHasNoFallback(t *testing.T) {
	st, b, _ := fortificationState(t)
	tower := b.O.Own[0].Info
	b.O.Allies = []aikit.AllyUnit{{Info: tower, X: st.cx + 780, Z: st.cz, Built: true}}
	// An ordinary-building fallback cannot bypass a tower reservation.
	st.ally.boxes = []keepOut{{x0: 0, z0: 0, x1: 4096, z1: 4096}}
	if _, _, ok := st.siteOnBearing(b, 0, 800, 0); ok {
		t.Fatal("hemmed-in allied tower reservation admitted a fallback site")
	}
	// A clear centre is insufficient when the candidate's footprint reaches
	// the margin. The outward firing lane stays free past that margin too.
	if st.clearOfAllyTowers(b, st.cx+800, st.cz+150, 2, 2) {
		t.Fatal("full candidate footprint overlapped the tower margin")
	}
	if st.clearOfAllyTowers(b, st.cx+1000, st.cz, 2, 2) {
		t.Fatal("candidate occupied the allied tower's outward firing lane")
	}
	if !st.clearOfAllyTowers(b, st.cx+800, st.cz+200, 2, 2) {
		t.Fatal("clear candidate was rejected beside the tower reservation")
	}
}

func TestAirOnlyBuilderDoesNotPostponeGroundCoverage(t *testing.T) {
	st, b, _ := fortificationState(t)
	ground := b.O.Own[0].Info
	aa := fortificationTower(100, 0, 1000, 100, 500, true)
	st.tab = fortificationTable(ground, aa)
	st.air = true
	st.towers[0] = 0
	st.want[0] = 500
	b.O.Own = []aikit.OwnUnit{
		{H: pool.Handle(1), Gen: 1, Built: true, Info: &aikit.UnitInfo{Role: aikit.RoleBuilder, Builds: []*aikit.UnitInfo{aa}}},
		{H: pool.Handle(2), Gen: 1, Built: true, Info: &aikit.UnitInfo{Role: aikit.RoleBuilder, Builds: []*aikit.UnitInfo{ground}}},
	}
	b.Builders = []int32{0, 1}
	if !st.planOne(b) || len(st.jobs.list) != 1 || st.jobs.list[0].prod != ground || st.jobs.list[0].who.h != 2 {
		t.Fatal("air-only constructor postponed the available ground tower")
	}
}

func TestWallCorridorsUseAnchoredFinishedFootprints(t *testing.T) {
	st, b, _ := fortificationState(t)
	st.cx += 7
	st.cz += 9
	st.defense.wallFootX, st.defense.wallFootZ = 4, 2
	st.defense.wallBuildX, st.defense.wallBuildZ = 1, 1
	tower := b.O.Own[0].Info
	b.O.Own = nil
	for _, d := range sectorDir {
		b.O.Own = append(b.O.Own, aikit.OwnUnit{Info: tower, X: st.cx + int32(d[0]*towerRoom/1000), Z: st.cz + int32(d[1]*towerRoom/1000), Built: true})
	}
	var any bool
	for s := range sectorDir {
		for n := int32(0); n < maxWallSegments; n++ {
			st.defense.wallN[s] = n
			for _, a := range st.wallSite(b, s, nil) {
				any = true
				if a[0]%16 != 8 || a[1]%16 != 8 {
					t.Fatal("odd construction footprint lost grid alignment")
				}
				neighbor := (s + 1) % numSectors
				for nn := int32(0); nn < maxWallSegments; nn++ {
					st.defense.wallN[neighbor] = nn
					for _, c := range st.wallSite(b, neighbor, nil) {
						// Blocking rectangles share the unit anchor, extending
						// 64×32 wu toward +X/+Z. Rectangle distance is exact.
						xgap := max(absI(int64(a[0]-c[0]))-64, 0)
						zgap := max(absI(int64(a[1]-c[1]))-32, 0)
						if xgap*xgap+zgap*zgap < 64*64 {
							t.Fatalf("finished footprints narrowed a corridor: %v, %v", a, c)
						}
					}
				}
			}
		}
	}
	if !any {
		t.Fatal("mismatched authored footprints admitted no wall pieces")
	}
}
