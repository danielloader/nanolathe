package survival

import (
	"slices"
	"testing"

	"github.com/nanolathe-gg/nanolathe/internal/aikit"
	"github.com/nanolathe-gg/nanolathe/internal/aikit/core"
	"github.com/nanolathe-gg/nanolathe/internal/content"
	"github.com/nanolathe-gg/nanolathe/internal/pool"
)

func cleanupWorld(t *testing.T, n int) (*Economy, *core.Board) {
	t.Helper()
	con := &aikit.UnitInfo{Index: 0, Role: aikit.RoleBuilder | aikit.RoleMobile | aikit.RoleAir, FootX: 2, FootZ: 2,
		Def: &content.UnitDef{CanReclamate: true, CanFly: true}}
	o := &aikit.Obs{Tick: 1000}
	for i := range n {
		o.Own = append(o.Own, aikit.OwnUnit{H: pool.Handle(i + 1), Gen: 1, Info: con, X: 2300, Z: 2048, Built: true})
	}
	st, b := allyState(t, o)
	b.EnemyValue = aikit.NewGrid(b.K.Map)
	b.K.Table.Units = []*aikit.UnitInfo{con}
	b.K.Budget = -1
	b.Update(b.K, o)
	st.p = Params{}
	return &Economy{st: st}, b
}

func obstacle(x, z, metal int32) aikit.Feature {
	return aikit.Feature{X: x, Z: z, FootX: 2, FootZ: 2, Metal: metal, Blocking: true, Reclaimable: true}
}

// Own and observed allied exits precede corridor blockers, which precede
// wreck metal. The nearest blocker wins within a priority, independent of
// a richer pile nearby; authored defensive wall features are excluded.
func TestCleanupPrioritizesLanesAndKeepsWalls(t *testing.T) {
	e, b := cleanupWorld(t, 4)
	factory := &aikit.UnitInfo{Role: aikit.RoleFactory, FootX: 6, FootZ: 6}
	b.O.Allies = []aikit.AllyUnit{{H: 40, Gen: 1, Info: factory, Built: true, X: 2500, Z: 2200}}
	lane := obstacle(2500, 2300, 0)
	corridor := obstacle(2350, 2048, 0)
	wreck := obstacle(2250, 1980, 1000)
	wall := obstacle(2500, 2280, 5000)
	wall.Defensive = true
	b.O.Features = []aikit.Feature{wall, wreck, corridor, lane}
	e.planCleanup(b)
	if len(e.cleanup.jobs) != 1 || e.cleanup.jobs[0].x != lane.X || e.cleanup.jobs[0].z != lane.Z || e.cleanup.jobs[0].rank != 3 {
		t.Fatalf("jobs %+v: want the allied lane obstacle", e.cleanup.jobs)
	}
	e.cleanup.jobs = nil
	b.O.Allies = nil
	b.O.Own = append(b.O.Own, aikit.OwnUnit{H: 40, Gen: 1, Info: factory, Built: true, X: 2500, Z: 2200})
	b.Factories = []int32{4}
	e.planCleanup(b)
	if len(e.cleanup.jobs) != 1 || e.cleanup.jobs[0].rank != 3 || e.cleanup.jobs[0].x != lane.X || e.cleanup.jobs[0].z != lane.Z {
		t.Fatal("own factory approach did not receive the same cleanup priority")
	}
	e.cleanup.jobs = nil
	b.O.Features = []aikit.Feature{wall, wreck, corridor}
	e.planCleanup(b)
	if len(e.cleanup.jobs) != 1 || e.cleanup.jobs[0].rank != 2 || e.cleanup.jobs[0].x != corridor.X {
		t.Fatalf("jobs %+v: want the corridor before wreck metal", e.cleanup.jobs)
	}
	e.cleanup.jobs = nil
	b.O.Features = []aikit.Feature{wall, wreck}
	e.planCleanup(b)
	if len(e.cleanup.jobs) != 1 || e.cleanup.jobs[0].rank != 1 || e.cleanup.jobs[0].x != wreck.X {
		t.Fatalf("jobs %+v: want the wreck, preserving the authored wall", e.cleanup.jobs)
	}
	if !cleanupLane(&lane, factory, 2500, 2200) || cleanupLane(&corridor, factory, 2500, 2200) {
		t.Fatal("factory approach footprint test missed its boundaries")
	}
}

// Cleanup takes only idle constructors, reserves economic and defensive
// work and neither doubles up on a target nor overlaps two Clear areas.
func TestCleanupReservesBuildersAndTargets(t *testing.T) {
	for _, n := range []int{1, 2, 3, 8} {
		e, b := cleanupWorld(t, n)
		b.O.Features = []aikit.Feature{obstacle(2400, 2048, 20), obstacle(2490, 2048, 20), obstacle(2800, 2200, 100)}
		e.planCleanup(b)
		want := 0
		if n >= 3 {
			want = max(n/4, 1)
		}
		if len(e.cleanup.jobs) != want {
			t.Fatalf("%d constructors: %d cleanup jobs, want %d", n, len(e.cleanup.jobs), want)
		}
		if want > 1 {
			a, z := e.cleanup.jobs[0], e.cleanup.jobs[1]
			if a.who == z.who || aikit.Dist2(a.x, a.z, z.x, z.z) <= 4*cleanupRadius*cleanupRadius {
				t.Fatalf("duplicate/overlapping jobs %+v", e.cleanup.jobs)
			}
		}
	}
	e, b := cleanupWorld(t, 4)
	b.O.Features = []aikit.Feature{obstacle(2400, 2048, 20)}
	b.O.Own[0].Order = aikit.OrderBuild
	b.O.Own[1].Order = aikit.OrderRepair // useful assistance also stays at work
	b.O.Own[2].Info = &aikit.UnitInfo{Role: aikit.RoleCommander | aikit.RoleBuilder | aikit.RoleMobile, Def: &content.UnitDef{CanReclamate: true}}
	e.st.jobs.list = []job{{kind: jobTower, who: handleGen{b.O.Own[3].H, b.O.Own[3].Gen}}}
	e.planCleanup(b)
	if len(e.cleanup.jobs) != 0 {
		t.Fatalf("productive/defensive builders taken: %+v", e.cleanup.jobs)
	}
}

// The economy sees neither a cleanup constructor nor its assigned feature
// area; the original observation and builder list are restored afterwards.
func TestCleanupHidesClaimsFromEconomy(t *testing.T) {
	e, b := cleanupWorld(t, 4)
	b.O.Features = []aikit.Feature{obstacle(2400, 2048, 20), obstacle(2900, 2100, 100)}
	original := b.O
	builders := slices.Clone(b.Builders)
	inner := &cleanupObserver{}
	e.inner = inner
	e.Plan(b)
	if len(e.cleanup.jobs) != 1 || len(inner.builders) != 3 || len(inner.features) != 1 {
		t.Fatalf("jobs %+v, economy builders %v features %+v", e.cleanup.jobs, inner.builders, inner.features)
	}
	j := e.cleanup.jobs[0]
	for _, i := range inner.builders {
		if b.O.Own[i].H == j.who.h {
			t.Fatal("economy saw the cleanup constructor")
		}
	}
	if b.O != original || !slices.Equal(b.Builders, builders) || len(b.O.Features) != 2 {
		t.Fatal("temporary economy filtering changed the observation or board")
	}
	if len(e.view.Own) != 0 || len(e.view.Features) != 0 {
		t.Fatal("economy retained borrowed observation slices between thinks")
	}
	b.O.Own[b.Index(j.who.h)].Order = aikit.OrderReclaim
	b.O.Tick++
	b.Update(b.K, b.O)
	e.Plan(b)
	if len(e.cleanup.jobs) != 1 || e.cleanup.jobs[0].who != j.who {
		t.Fatal("active cleanup assignment was duplicated or taken by another layer")
	}
}

// When defensive work grows, cleanup yields enough builders to preserve
// an economic constructor rather than holding its earlier larger share.
func TestCleanupYieldsWhenDefenseNeedsBuilders(t *testing.T) {
	e, b := cleanupWorld(t, 8)
	b.O.Features = []aikit.Feature{obstacle(2400, 2048, 20), obstacle(2800, 2200, 100)}
	e.planCleanup(b)
	if len(e.cleanup.jobs) != 2 {
		t.Fatal("fixture did not assign two cleanup constructors")
	}
	for _, i := range b.Builders {
		u := &b.O.Own[i]
		if !e.cleanupClaimed(u) {
			e.st.jobs.list = append(e.st.jobs.list, job{kind: jobTower, who: handleGen{u.H, u.Gen}})
		}
	}
	e.planCleanup(b)
	if len(e.cleanup.jobs) != 1 {
		t.Fatalf("cleanup held %d builders beside %d defense jobs; no constructor left for economy", len(e.cleanup.jobs), len(e.st.jobs.list))
	}
}

type cleanupObserver struct {
	builders []int32
	features []aikit.Feature
}

func (*cleanupObserver) Init(*core.Board) {}
func (p *cleanupObserver) Plan(b *core.Board) {
	p.builders = slices.Clone(b.Builders)
	p.features = slices.Clone(b.O.Features)
}

// A threat at the area or on its approach blocks assignment; an active
// reclaim is stopped when one appears. Reused handles and subsequently
// assigned construction are released without stopping useful work.
func TestCleanupStopsThreatenedWork(t *testing.T) {
	e, b := cleanupWorld(t, 4)
	f := obstacle(2800, 2048, 100)
	b.O.Features = []aikit.Feature{f}
	b.Threat.AddDisc(2550, 2048, 80, 100)
	e.planCleanup(b)
	if len(e.cleanup.jobs) != 0 {
		t.Fatal("cleanup crossed an observed threat")
	}
	b.Threat.Clear()
	e.planCleanup(b)
	if len(e.cleanup.jobs) != 1 {
		t.Fatal("safe wreck did not get a constructor")
	}
	e.emitCleanup(b)
	j := e.cleanup.jobs[0]
	b.O.Own[b.Index(j.who.h)].Order = aikit.OrderReclaim
	b.Threat.AddDisc(f.X, f.Z, 80, 100)
	e.refreshCleanup(b)
	if len(e.cleanup.jobs) != 0 || e.cleanup.stops != 1 {
		t.Fatalf("threatened cleanup jobs %+v, stops %d", e.cleanup.jobs, e.cleanup.stops)
	}
	if !e.cleanupTaken(f.X, f.Z, b.Tick) {
		t.Fatal("threatened target was immediately offered again")
	}
	b.Threat.Clear()
	b.Tick++
	b.O.Own[0].Order = aikit.OrderBuild
	e.cleanup.jobs = []cleanupJob{j}
	e.refreshCleanup(b)
	if len(e.cleanup.jobs) != 0 || e.cleanup.stops != 1 {
		t.Fatal("later productive construction was stopped")
	}
	b.O.Own[0].Gen++
	e.cleanup.jobs = []cleanupJob{j}
	e.refreshCleanup(b)
	if len(e.cleanup.jobs) != 0 || e.cleanup.stops != 1 {
		t.Fatal("a reused handle inherited cleanup")
	}
}

// A pending Clear preserves action room for army and production.
func TestCleanupKeepsActionReserve(t *testing.T) {
	e, b := cleanupWorld(t, 4)
	b.O.Features = []aikit.Feature{obstacle(2400, 2048, 20)}
	e.planCleanup(b)
	b.K.Budget = 2
	e.emitCleanup(b)
	if e.cleanup.jobs[0].issued {
		t.Fatal("cleanup spent the army/production action reserve")
	}
	b.K.Budget = 3
	e.emitCleanup(b)
	if !e.cleanup.jobs[0].issued {
		t.Fatal("cleanup did not use an available spare action")
	}
}
