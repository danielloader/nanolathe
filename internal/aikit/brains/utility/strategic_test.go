package utility

import (
	"testing"

	"github.com/nanolathe-gg/nanolathe/internal/aikit"
	"github.com/nanolathe-gg/nanolathe/internal/aikit/core"
	"github.com/nanolathe-gg/nanolathe/internal/content"
)

func strategicFixture() (*Economy, *core.Board, *aikit.UnitInfo, *aikit.UnitInfo) {
	mk := func(key string, role aikit.Role, w *content.WeaponDef) *aikit.UnitInfo {
		return &aikit.UnitInfo{Key: key, Role: role, Metal: 2000, Energy: 40000, Value: 2666, HP: 1000, FootX: 4, FootZ: 4,
			Def: &content.UnitDef{Weapon1Def: w}}
	}
	silo := mk("renamed_launcher", aikit.RoleStockpile, &content.WeaponDef{ID: 1, Stockpile: true, CommandFire: true, Targetable: true,
		Range: 10000, AreaOfEffect: 400, MetalPerShot: 1000, EnergyPerShot: 60000})
	anti := mk("renamed_interceptor", aikit.RoleInterceptor, &content.WeaponDef{ID: 2, Stockpile: true, Interceptor: true, Coverage: 2000,
		MetalPerShot: 100, EnergyPerShot: 1000})
	con := &aikit.UnitInfo{Role: aikit.RoleBuilder | aikit.RoleMobile, Speed: 30, BuildPower: 100, Builds: []*aikit.UnitInfo{silo, anti}}
	tab := &aikit.Table{Units: []*aikit.UnitInfo{con, silo, anti}}
	for i, u := range tab.Units {
		u.Index = int32(i)
	}
	k := &aikit.Kit{Table: tab, Map: &aikit.MapInfo{WorldW: 10000, WorldH: 10000}, Persona: aikit.PersonaHard}
	s := &shared{p: DefaultParams()}
	s.setup(k)
	s.tick, s.safeX, s.safeZ = strategicReady, 500, 500
	s.supplyM, s.supplyE = 100000, 5000000
	b := &core.Board{K: k, Tick: strategicReady, HomeX: 500, HomeZ: 500,
		Threat: aikit.NewGrid(&aikit.MapInfo{SectorW: 20, SectorH: 20}),
		Metal:  aikit.Res{Stock: 10000, Cap: 10000, Income: 100},
		Energy: aikit.Res{Stock: 200000, Cap: 200000, Income: 5000}}
	b.O = &aikit.Obs{Tick: b.Tick, Metal: b.Metal, Energy: b.Energy,
		Own:    []aikit.OwnUnit{{H: 1, Gen: 1, Info: con, X: 500, Z: 500, Built: true}},
		Memory: []aikit.Remembered{{H: 100, Owner: 1, Info: silo, X: 6000, Z: 6000, Building: true}}}
	b.Builders = []int32{0}
	return &Economy{s: s}, b, silo, anti
}

func TestStrategicConstructionNeedsFundsAndInformation(t *testing.T) {
	e, b, silo, anti := strategicFixture()
	con := &b.O.Own[0]
	if c := e.evalStrategic(b, con, silo); c.score <= minScore || c.kind != cStrategic {
		t.Fatalf("funded launcher with a known target: %+v", c)
	}
	if c := e.evalStrategic(b, con, anti); c.score <= minScore || c.x != b.HomeX || c.z != b.HomeZ {
		t.Fatalf("funded home defense: %+v", c)
	}
	e.s.count[silo.Index] = 1 // another builder's commitment or a nanoframe
	if c := e.evalStrategic(b, con, silo); c.score != 0 {
		t.Fatal("duplicate launcher")
	}
	e.s.count[silo.Index] = 0
	b.Tick = strategicReady - 1
	if c := e.evalStrategic(b, con, silo); c.score != 0 {
		t.Fatal("offensive early investment")
	}
	if c := e.evalStrategic(b, con, anti); c.score == 0 {
		t.Fatal("observed nuclear threat did not admit early defense")
	}
	b.O.Memory = nil
	if c := e.evalStrategic(b, con, anti); c.score != 0 {
		t.Fatal("early defense without a threat")
	}
	b.Tick = strategicReady
	if c := e.evalStrategic(b, con, silo); c.score != 0 {
		t.Fatal("guessed enemy start bought a nuke")
	}
	b.Energy = aikit.Res{Stock: 1, Cap: 1000, Income: 10, Expense: 100}
	if c := e.evalStrategic(b, con, anti); c.score != 0 {
		t.Fatal("energy stall bought an interceptor")
	}
}

func TestStrategicTargetUsesKnownHostilesAndFriendlyClearance(t *testing.T) {
	_, b, silo, _ := strategicFixture()
	w := silo.Def.Weapon1Def
	check := func(want bool) {
		t.Helper()
		_, _, got := strategicTarget(b.O, 500, 500, w, silo)
		if got != want {
			t.Fatalf("target=%v want %v", got, want)
		}
	}
	check(true)
	b.O.Allied[1] = true
	check(false)
	b.O.Allied[1] = false
	b.O.Memory[0].Building = false
	check(false)
	b.O.Memory[0].Building = true
	w.Range = 100
	check(false)
	w.Range = 10000
	b.O.Allies = []aikit.AllyUnit{{X: 6000, Z: 6000}}
	check(false)
	b.O.Allies = nil
	// Radius is half the authored 400 plus the policy's 64. The boundary
	// is rejected; moving one whole world unit beyond it permits the aim.
	b.O.Own = []aikit.OwnUnit{{X: 6264, Z: 6000}}
	check(false)
	b.O.Own[0].X++
	check(true)
	silo.Def.Weapon2Def = &content.WeaponDef{ID: 3, Stockpile: true, AreaOfEffect: 1000}
	check(false) // ordinary ground attack also binds the larger companion
	silo.Def.Weapon2Def = nil
	b.O.Memory = nil
	b.O.Enemy = []aikit.Contact{{X: 6000, Z: 6000}}
	check(false) // an untyped blip supplies no target value
}

func TestStrategicFundingKeepsReserve(t *testing.T) {
	r := aikit.Res{Stock: 1200, Cap: 1000}
	if !strategicFunded(r, 1000, 120) {
		t.Fatal("savings at exact boundary refused")
	}
	r.Stock--
	if strategicFunded(r, 1000, 120) {
		t.Fatal("reserve spent")
	}
	r.Stock, r.Income, r.Expense = 200, 10, 10
	if !strategicFunded(r, 1200, 120) {
		t.Fatal("exact income horizon refused")
	}
	if strategicFunded(r, 1201, 120) {
		t.Fatal("unfunded income horizon admitted")
	}
	r.Expense++
	if strategicFunded(r, 1000, 120) {
		t.Fatal("falling economy admitted a new drain")
	}
}

func TestStrategicUpkeepReservesAndRecycledHandles(t *testing.T) {
	e, b, silo, anti := strategicFixture()
	b.O.Own = []aikit.OwnUnit{own(3, silo, 500, 500), own(4, anti, 600, 500)}
	if n := e.strategic.service(b, 1); n != 1 {
		t.Fatalf("one-action budget used %d", n)
	}
	b.O.Own[0].StockpileQueued[0] = 1
	b.O.Own[1].StockpileQueued[0] = 1
	if n := e.strategic.service(b, 10); n != 0 {
		t.Fatalf("duplicated pending rounds: %d", n)
	}
	b.O.Own[0].StockpileQueued[0], b.O.Own[1].StockpileQueued[0] = 0, 0
	b.O.Own[0].Ammo[0], b.O.Own[1].Ammo[0] = 1, 3
	if n := e.strategic.service(b, 10); n != 1 {
		t.Fatalf("loaded launcher should get a deliberate target, got %d", n)
	}
	b.O.Own[0].Order = aikit.OrderAttack
	if n := e.strategic.service(b, 10); n != 0 {
		t.Fatalf("repeated standing attack: %d", n)
	}
	b.O.Own[0].Ammo[0] = 0
	if n := e.strategic.service(b, 10); n != 2 {
		t.Fatalf("spent round should stop old ground order and replenish, got %d", n)
	}
	b.O.Own[0].Ammo[0] = 1
	if n := e.strategic.service(b, 10); n != 0 {
		t.Fatal("same target retried during cooldown")
	}
	b.O.Own[0].Gen++
	if n := e.strategic.service(b, 10); n != 1 {
		t.Fatal("new allocation inherited target cooldown")
	}
	b.O.Own[0].Built = false
	b.O.Own[1].Built = false
	if n := e.strategic.service(b, 10); n != 0 {
		t.Fatal("unfinished units serviced")
	}
}

func TestMobileInterceptorIsAReservedFallback(t *testing.T) {
	e, b, _, anti := strategicFixture()
	mobile := *anti
	mobile.Role |= aikit.RoleMobile
	pr := &Production{s: e.s}
	if pr.interceptorScore(b, &mobile) != 0 {
		t.Fatal("mobile system displaced an available stationary one")
	}
	b.Builders = nil
	if pr.interceptorScore(b, &mobile) == 0 {
		t.Fatal("factory-only defense unavailable")
	}
	e.s.count[anti.Index] = 1
	if pr.interceptorScore(b, &mobile) != 0 {
		t.Fatal("existing system did not reserve defense budget")
	}
}

func TestQueuedInterceptorReservesConstructionAcrossThinks(t *testing.T) {
	e, b, _, anti := strategicFixture()
	pr := &Production{s: e.s}
	fac := &aikit.UnitInfo{Role: aikit.RoleFactory}
	b.O.Own = append(b.O.Own, aikit.OwnUnit{H: 2, Gen: 1, Info: fac, Built: true, QueueLen: 2})
	b.Factories = []int32{1}
	pr.regOf(&b.O.Own[1]).prod = anti // pending behind a different first product
	e.s.interceptorQueued = pr.hasQueuedInterceptor(b)
	if c := e.evalStrategic(b, &b.O.Own[0], anti); c.score != 0 {
		t.Fatal("a newly available constructor duplicated the queued interceptor")
	}
	b.O.Own[1].QueueLen = 0 // failed/cancelled production releases the reservation
	e.s.interceptorQueued = pr.hasQueuedInterceptor(b)
	if c := e.evalStrategic(b, &b.O.Own[0], anti); c.score == 0 {
		t.Fatal("empty factory queue still held the defense budget")
	}
}

func TestStrategicCandidatesReachTheBuilderDecision(t *testing.T) {
	e, b, silo, anti := strategicFixture()
	var d decision
	e.evalBuildings(b, &b.O.Own[0], &builderState{blockSpot: -1}, &d)
	if d.n != 2 || d.top[0].kind != cStrategic || d.top[1].kind != cStrategic {
		t.Fatalf("stockpile products did not enter the ordinary builder choice: %+v", d)
	}
	e.s.count[anti.Index]++ // earlier builder reserved the defender
	d = decision{}
	e.evalBuildings(b, &b.O.Own[0], &builderState{blockSpot: -1}, &d)
	if d.n != 1 || d.top[0].prod != silo {
		t.Fatal("same-think reservation did not leave only the offensive launcher")
	}
}
