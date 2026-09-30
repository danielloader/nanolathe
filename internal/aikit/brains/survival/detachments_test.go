package survival

import (
	"strings"
	"testing"

	"github.com/nanolathe-gg/nanolathe/formats"
	"github.com/nanolathe-gg/nanolathe/internal/ai"
	"github.com/nanolathe-gg/nanolathe/internal/aikit"
	"github.com/nanolathe-gg/nanolathe/internal/aikit/brains/tactics"
	"github.com/nanolathe-gg/nanolathe/internal/aikit/core"
	"github.com/nanolathe-gg/nanolathe/internal/cob"
	"github.com/nanolathe-gg/nanolathe/internal/content"
	"github.com/nanolathe-gg/nanolathe/internal/economy"
	"github.com/nanolathe-gg/nanolathe/internal/orders"
	"github.com/nanolathe-gg/nanolathe/internal/pool"
	"github.com/nanolathe-gg/nanolathe/internal/sim/numeric"
	"github.com/nanolathe-gg/nanolathe/internal/units"
	"github.com/nanolathe-gg/nanolathe/internal/world"
)

func detFixture(n int) (*Army, *core.Board, *aikit.UnitInfo) {
	u := &aikit.UnitInfo{Index: 0, Role: aikit.RoleMobile | aikit.RoleCombat, Side: "ARM", Value: 100, HP: 500, DPS: 25, Range: 200, Speed: 40, FootX: 2, FootZ: 2}
	m := &aikit.MapInfo{WorldW: 4096, WorldH: 4096, SectorW: 32, SectorH: 32}
	k := &aikit.Kit{Table: &aikit.Table{Units: []*aikit.UnitInfo{u}}, Map: m, Side: "ARM", Persona: aikit.PersonaHard}
	p := tactics.DefaultParams()
	p.Raid, p.Harass, p.Probe, p.Tour = false, false, false, false
	a := &Army{st: &state{ready: true, cx: 2048, cz: 2048, dx: 2048, dz: 2048, outX: 1000}, inner: tactics.New(p)}
	for i := range a.st.radius {
		a.st.radius[i], a.st.mine[i] = 800, true
	}
	o := &aikit.Obs{Tick: 1000}
	for i := 0; i < n; i++ {
		o.Own = append(o.Own, aikit.OwnUnit{H: pool.Handle(i + 1), Gen: 1, Info: u, X: 2540 + int32(i%4)*40, Z: 1900 + int32(i/4)*40, HP: 500, MaxHP: 500, Built: true})
	}
	b := &core.Board{K: k, Posture: core.Posture{AttackValue: 1 << 30}}
	a.Init(b)
	b.Update(k, o)
	return a, b, u
}

// A new warning and observation reordering keep every surviving actor in
// its original detachment; a recycled slot gets a new ownership decision.
func TestDetachmentsKeepInstancesAcrossWarnings(t *testing.T) {
	a, b, u := detFixture(30)
	a.Plan(b)
	before := append([]detOwner(nil), a.force.owners...)
	for i, j := 0, len(b.O.Own)-1; i < j; i, j = i+1, j-1 {
		b.O.Own[i], b.O.Own[j] = b.O.Own[j], b.O.Own[i]
	}
	a.st.live = []Approach{{Angle: 8 * sectorAngle, Other: true}}
	b.O.Tick += stageEvery
	b.Update(b.K, b.O)
	a.Plan(b)
	for i := range b.O.Own {
		u := &b.O.Own[i]
		if got := a.force.owners[u.H]; got != before[u.H] {
			t.Fatalf("warning changed instance %d's owner from %+v to %+v", u.H, before[u.H], got)
		}
	}
	// Replacement is a support aircraft, so its old ground-front ownership
	// must not survive slot reuse.
	idx := -1
	for i := range b.O.Own {
		if a.force.owners[b.O.Own[i].H].id != reserve {
			idx = i
			break
		}
	}
	if idx < 0 {
		t.Fatal("fixture has no front member")
	}
	air := *u
	air.Role |= aikit.RoleAir
	b.O.Own[idx].Gen++
	b.O.Own[idx].Info = &air
	b.O.Tick++
	b.Update(b.K, b.O)
	a.Plan(b)
	if m := a.force.owners[b.O.Own[idx].H]; m.id != reserve || m.gen != b.O.Own[idx].Gen {
		t.Fatalf("recycled support instance retained old ownership: %+v", m)
	}
}

// Simultaneous public approaches allocate strength to both fronts while
// preserving a reserve. Each tactics board has only its assigned combat
// actors and rebuilt indices, values and threat grids from its own view.
func TestDetachmentsCoverSimultaneousApproachesWithConsistentBoards(t *testing.T) {
	a, b, u := detFixture(36)
	a.st.live = []Approach{{Angle: 0, Other: true}, {Angle: 5 * sectorAngle, Other: true}}
	b.O.Enemy = []aikit.Contact{{H: 90, Gen: 1, Info: u, X: 3400, Z: 2048, Visible: true}, {H: 91, Gen: 1, Info: u, X: 1460, Z: 3470, Visible: true}, {H: 92, Gen: 1, Info: u, X: 64, Z: 64, Visible: true}}
	for _, c := range b.O.Enemy {
		b.O.Memory = append(b.O.Memory, aikit.Remembered{H: c.H, Gen: c.Gen, Info: c.Info, X: c.X, Z: c.Z, LastSeen: b.Tick})
	}
	b.Update(b.K, b.O)
	hx, hz, ex, ez, av := b.HomeX, b.HomeZ, b.EnemyX, b.EnemyZ, b.ArmyValue
	a.Plan(b)
	if d := &a.force.d[reserve]; d.ground < int32((len(b.O.Own)*reserveShare+99)/100) {
		t.Fatalf("reserve has %d ground units", d.ground)
	}
	for _, i := range []int{a.force.front(0), a.force.front(5)} {
		d := &a.force.d[i]
		if d.n == 0 || d.warning == 0 || d.pressure == 0 {
			t.Fatalf("simultaneous front %d not allocated: %+v", i, d)
		}
	}
	for id := range a.force.d {
		d := &a.force.d[id]
		var value int32
		for _, i := range d.board.Combat {
			u := &d.obs.Own[i]
			if a.force.owners[u.H].id != id || d.board.Index(u.H) != i {
				t.Fatalf("detachment %d has foreign actor or bad index %d", id, u.H)
			}
			value += u.Info.Value
		}
		if d.board.ArmyValue != value {
			t.Fatalf("detachment %d board value %d, assigned value %d", id, d.board.ArmyValue, value)
		}
		var check core.Board
		check.HomeX, check.HomeZ = d.board.HomeX, d.board.HomeZ
		check.Update(b.K, &d.obs)
		if check.EnemyArmyValue != d.board.EnemyArmyValue || check.NearHomeThreat != d.board.NearHomeThreat {
			t.Fatalf("detachment %d has global enemy totals", id)
		}
		for i, v := range check.Threat.V {
			if d.board.Threat.V[i] != v {
				t.Fatalf("detachment %d retains unfiltered threat in cell %d", id, i)
			}
		}
		for _, c := range d.obs.Enemy {
			if c.H == 92 {
				t.Fatal("a distant straggler entered the defensive picture")
			}
		}
	}
	if b.HomeX != hx || b.HomeZ != hz || b.EnemyX != ex || b.EnemyZ != ez || b.ArmyValue != av {
		t.Fatal("detachments mutated the production board")
	}
}

func detTerrain(split bool) *world.Terrain {
	const w = 256
	attrs := make([]formats.TNTAttribute, w*w)
	for z := 0; z < w; z++ {
		for x := 0; x < w; x++ {
			h := uint8(60)
			if split && x >= 176 && x < 200 {
				h = 10
			}
			attrs[z*w+x] = formats.TNTAttribute{Height: h, Feature: world.PlotFeatureNone}
		}
	}
	return &world.Terrain{CellW: w, CellH: w, SeaLevel: 50, Plot: world.ExpandPlot(attrs, w, w)}
}

// Stages use the actual ground region, stay out of narrow terrain and
// leave the whole gathering disc clear of buildings, features and lanes.
func TestDetachmentStagesUseOpenReachableGround(t *testing.T) {
	a, b, u := detFixture(24)
	u.Def = &content.UnitDef{CanMove: true, MinWaterDepth: -10000, MaxWaterDepth: 0, MaxSlope: 10}
	b.K.Map = aikit.AnalyzeMap(detTerrain(true), nil, 2, 2, 2400, 2048)
	a.Init(b)
	fac := &aikit.UnitInfo{Role: aikit.RoleFactory, FootX: 8, FootZ: 8, Value: 1000}
	b.O.Allies = []aikit.AllyUnit{{Info: fac, X: 2560, Z: 2048, Built: true}}
	b.O.Features = []aikit.Feature{{X: 2688, Z: 2192, FootX: 8, FootZ: 8, Blocking: true}}
	b.Update(b.K, b.O)
	a.Plan(b)
	for id := range a.force.d {
		d := &a.force.d[id]
		if !d.stageSet {
			t.Fatalf("detachment %d found no open site on the fixture", id)
		}
		if !a.force.stageClear(a.st, b.O, d.x, d.z, stageRoom) {
			t.Fatalf("detachment %d stage (%d,%d) blocks the core or a lane", id, d.x, d.z)
		}
		if !a.force.stageClear(a.st, b.O, d.homeX, d.homeZ, stageRoom) ||
			aikit.Dist2(d.x, d.z, d.homeX, d.homeZ) < refugeGap*refugeGap ||
			!refugeOutsideCore(a.st, d.x, d.z, d.homeX, d.homeZ, stageRoom) {
			t.Fatalf("detachment %d has no separate clear rear refuge", id)
		}
		if d.board.HomeX != d.homeX || d.board.HomeZ != d.homeZ {
			t.Fatalf("detachment %d uses its forward stage as tactics Home", id)
		}
		for _, i := range d.board.Combat {
			actor := &d.obs.Own[i]
			if !a.force.reaches(actor, d.x, d.z) {
				t.Fatalf("detachment %d put actor %d across the water", id, actor.H)
			}
			if !a.force.reaches(actor, d.homeX, d.homeZ) {
				t.Fatalf("detachment %d put actor %d's refuge across the water", id, actor.H)
			}
		}
	}
	// Test the region and passage verdict separately from candidate order.
	if a.force.stageGround(b, b.O, reserve, 3500, 2048, stageRoom) {
		t.Fatal("stage on the other island admitted ground members")
	}
}

func TestDetachmentsRefuseBlockedStagesAndReuseBuffers(t *testing.T) {
	a, b, _ := detFixture(40)
	a.Plan(b)
	owners := append([]detOwner(nil), a.force.owners...)
	if got := testing.AllocsPerRun(5, func() { a.Plan(b) }); got != 0 {
		t.Fatalf("steady detachment planning allocates %g objects per think", got)
	}
	beforeLoss := make(map[string]int64)
	a.Report(func(name string, v int64) { beforeLoss[name] = v })
	block := &aikit.UnitInfo{Role: aikit.RoleFactory, FootX: 512, FootZ: 512}
	b.O.Allies = append(b.O.Allies, aikit.AllyUnit{Info: block, X: 2048, Z: 2048})
	b.O.Tick += stageEvery
	b.Update(b.K, b.O)
	a.Plan(b)
	for i := range a.force.d {
		d := &a.force.d[i]
		if d.stageSet {
			t.Fatalf("detachment %d retained a stage covered by an allied footprint", i)
		}
		if d.board.HomeX != a.st.dx || d.board.HomeZ != a.st.dz || int32(len(d.board.Combat)) != d.n {
			t.Fatalf("refused detachment %d lost its reactive actors or defence-centre Home", i)
		}
	}
	for i := range b.O.Own {
		u := &b.O.Own[i]
		if a.force.owners[u.H] != owners[u.H] {
			t.Fatalf("refusing unsafe ground changed actor %d's ownership", u.H)
		}
	}
	counts := make(map[string]int64)
	a.Report(func(name string, v int64) { counts[name] = v })
	if counts["sv_detachments"] != detachmentCount || counts["sv_det_3_ground"] == 0 || counts["sv_det_0_stage_safe"] != 0 || counts["sv_det_0_fallback"] != 1 {
		t.Fatalf("missing allocation and stage diagnostics: %v", counts)
	}
	for name, value := range counts {
		if strings.Contains(name, "_tac_lost_") && value != beforeLoss[name] {
			t.Fatalf("refused staging fabricated loss history: %s = %d", name, value)
		}
	}
	// A warning alone preserves each front's facing geometry, and new
	// ground units still reinforce fronts instead of all piling into reserve.
	var enemy [fronts][2]int32
	for i := range enemy {
		enemy[i] = [2]int32{a.force.d[i].board.EnemyX, a.force.d[i].board.EnemyZ}
	}
	a.st.live = []Approach{{Angle: 8 * sectorAngle, Other: true}}
	u := b.O.Own[0].Info
	for i := 0; i < 20; i++ {
		b.O.Own = append(b.O.Own, aikit.OwnUnit{H: pool.Handle(41 + i), Gen: 1, Info: u, X: 2540, Z: 2048, HP: 500, MaxHP: 500, Built: true})
	}
	b.O.Tick += 30
	b.Update(b.K, b.O)
	a.Plan(b)
	for i, point := range enemy {
		if d := &a.force.d[i]; point != [2]int32{d.board.EnemyX, d.board.EnemyZ} {
			t.Fatalf("warning turned refused front %d away from its own approach", i)
		}
	}
	if n := a.force.d[reserve].ground; n > (int32(len(b.O.Own))*reserveShare+99)/100 {
		t.Fatalf("refused stages pushed every reinforcement into reserve: %d of %d ground actors", n, len(b.O.Own))
	}
}

type detRetreatBrain struct {
	detBudgetBrain
	contact bool
	refuse  bool
	enemy   aikit.UnitInfo
}

func (br *detRetreatBrain) Init(k *aikit.Kit) {
	// This commanderless fixture supplies its authored side explicitly.
	k.Side = "ARM"
	br.detBudgetBrain.Init(k)
}

func (br *detRetreatBrain) Think(k *aikit.Kit, o *aikit.Obs) {
	if br.contact {
		d := &br.a.force.d[0]
		br.enemy = *k.Table.Units[0]
		br.enemy.Value, br.enemy.HP, br.enemy.DPS = 100, 5000, 2500
		// An observed, cheap but overpowering attacker triggers ordinary
		// tactics retreat without meeting the main incident-response threshold.
		c := aikit.Contact{H: 60, Gen: 1, Info: &br.enemy, X: d.x + 160, Z: d.z, Built: true, Visible: true, HPPct: 100}
		o.Enemy = append(o.Enemy, c)
		o.Memory = append(o.Memory, aikit.Remembered{H: c.H, Gen: c.Gen, Info: c.Info, X: c.X, Z: c.Z, LastSeen: o.Tick})
	}
	if br.refuse {
		block := &aikit.UnitInfo{Role: aikit.RoleFactory, FootX: 512, FootZ: 512}
		o.Allies = append(o.Allies, aikit.AllyUnit{Info: block, X: 2048, Z: 2048})
	}
	br.detBudgetBrain.Think(k, o)
}

// detStagedHost gives each front a real waiting order and places its actors
// at that stage before the next ordinary think and stage revalidation.
func detStagedHost(t *testing.T) (*aikit.Host, *units.World, *economy.Service, *detRetreatBrain, []pool.Handle) {
	t.Helper()
	wdef := &content.WeaponDef{ID: 1, DamageDefault: 25, ReloadTime: 30, Range: 200}
	def := &content.UnitDef{DefinitionHeader: content.DefinitionHeader{CanonicalKey: "defender"}, UnitName: "defender", Side: "ARM", CanMove: true, BMCode: 1, MaxDamage: 500, BuildCostMetal: 100, FootprintX: 2, FootprintZ: 2, Weapon1Def: wdef, MinWaterDepth: -10000, MaxSlope: 10}
	def.Script = &cob.Program{Code: []uint32{0x10065000}}
	cat := &content.Catalog{Units: map[string]*content.UnitDef{"defender": def}}
	w := units.NewSliced(64, cat)
	for i := 0; i < 20; i++ {
		_, err := w.Create(def, 0, numeric.FixedFromInt(2480+int64(i%6)*32), 0, numeric.FixedFromInt(1850+int64(i/6)*32))
		if err != nil {
			t.Fatal(err)
		}
	}
	e := &economy.Service{}
	e.Players[0].Exists, e.Players[0].ControllerState = true, 2
	m := &ai.Manager{Player: 0, Catalog: cat, Terrain: detTerrain(false)}
	p := aikit.PersonaHard
	p.APM, p.ThinkEvery, p.Reaction = 0, stageEvery, 3
	br := &detRetreatBrain{}
	host := aikit.NewHost(m, br, p)
	t.Cleanup(host.Close)
	for tick := uint32(1); tick <= stageEvery+3; tick++ {
		host.Step(tick, w, e)
	}
	host.Join()
	d := &br.a.force.d[0]
	if d.n == 0 || !d.stageSet {
		t.Fatal("fixture has no staged front actors")
	}
	var front []pool.Handle
	for _, actor := range w.IterSliced() {
		if br.a.force.owners[actor.Handle].id == 0 {
			front = append(front, actor.Handle)
			q := orders.QueueOfUnit(actor)
			if q == nil || q.Head() == nil || q.Head().ID != orders.Lookup("Move_Ground") ||
				aikit.Dist2(int32(q.Head().GoalX>>16), int32(q.Head().GoalZ>>16), d.x, d.z) > 2*2 {
				t.Fatalf("front actor %d did not receive a forward Move to stage (%d,%d)", actor.Handle, d.x, d.z)
			}
			actor.X, actor.Z = numeric.FixedFromInt(int64(d.x)), numeric.FixedFromInt(int64(d.z))
		}
	}
	return host, w, e, br, front
}

// A losing army already at its waiting stage must receive an actual Move
// to the rear refuge through the host, rather than a Move to that same stage.
func TestDetachmentRetreatOrderTargetsRearRefuge(t *testing.T) {
	host, w, e, br, front := detStagedHost(t)
	d := &br.a.force.d[0]
	br.contact = true
	for tick := uint32(stageEvery + 4); tick <= 2*stageEvery+3; tick++ {
		host.Step(tick, w, e)
	}
	host.Join()
	x := &aikit.Explain{}
	d.army.Explain(&d.board, x)
	retreat := false
	for _, s := range x.Squads {
		retreat = retreat || strings.Contains(s.Task, "main:retreat")
	}
	if !retreat {
		t.Fatalf("the overpowering contact did not trigger ordinary tactics retreat: %+v", x.Squads)
	}
	for _, h := range front {
		q := orders.QueueOfUnit(w.Unit(h))
		if q == nil || q.Head() == nil || q.Head().ID != orders.Lookup("Move_Ground") ||
			q.Head().GoalX != numeric.FixedFromInt(int64(d.homeX)) || q.Head().GoalZ != numeric.FixedFromInt(int64(d.homeZ)) {
			t.Fatalf("front actor %d did not receive an actual Move to refuge (%d,%d)", h, d.homeX, d.homeZ)
		}
	}
	if aikit.Dist2(d.x, d.z, d.homeX, d.homeZ) < refugeGap*refugeGap {
		t.Fatal("retreat destination collapsed into the forward waiting stage")
	}
	if host.Stats().Failed != 0 || host.Stats().DroppedAPM != 0 {
		t.Fatalf("host rejected the ordinary retreat orders: %+v", host.Stats())
	}
}

// Refusing a larger waiting pair cannot silence existing defenders. The
// ordinary contact prediction still retreats them with a host-applied Move.
func TestDetachmentRefusedPairKeepsReactiveOrders(t *testing.T) {
	host, w, e, br, front := detStagedHost(t)
	before := append([]detOwner(nil), br.a.force.owners...)
	beforeLoss := make(map[string]int64)
	br.a.Report(func(name string, v int64) { beforeLoss[name] = v })
	br.contact, br.refuse = true, true
	for tick := uint32(stageEvery + 4); tick <= 2*stageEvery+3; tick++ {
		host.Step(tick, w, e)
	}
	host.Join()
	d := &br.a.force.d[0]
	if d.stageSet || len(d.board.Combat) == 0 || d.board.HomeX != br.a.st.dx || d.board.HomeZ != br.a.st.dz {
		t.Fatal("refused pair did not preserve ordinary actors and defence-centre Home")
	}
	x := &aikit.Explain{}
	d.army.Explain(&d.board, x)
	retreat := false
	for _, s := range x.Squads {
		retreat = retreat || strings.Contains(s.Task, "main:retreat")
	}
	if !retreat {
		t.Fatalf("refused pair disabled the ordinary response to a losing contact: %+v", x.Squads)
	}
	for _, h := range front {
		if br.a.force.owners[h] != before[h] {
			t.Fatalf("refused pair reassigned actor %d", h)
		}
		q := orders.QueueOfUnit(w.Unit(h))
		if q == nil || q.Head() == nil || q.Head().ID != orders.Lookup("Move_Ground") ||
			q.Head().GoalX != numeric.FixedFromInt(int64(br.a.st.dx)) || q.Head().GoalZ != numeric.FixedFromInt(int64(br.a.st.dz)) {
			t.Fatalf("refused front actor %d received no reactive retreat Move", h)
		}
	}
	br.a.Report(func(name string, value int64) {
		if strings.Contains(name, "_tac_lost_") && value != beforeLoss[name] {
			t.Fatalf("fallback invented destroyed units: %s = %d", name, value)
		}
	})
	if host.Stats().Failed != 0 || host.Stats().DroppedAPM != 0 {
		t.Fatalf("host rejected the fallback's ordinary reactive orders: %+v", host.Stats())
	}
}

type detTemper struct{ calls int }

func (t *detTemper) ArmyTemper() tactics.Temper {
	t.calls++
	return tactics.Temper{Engage: 40, RaidPct: 100, HarassPct: 100}
}

func TestDetachmentsPreserveTacticsConfigurationAndTemper(t *testing.T) {
	a, b, _ := detFixture(0)
	src := &detTemper{}
	inner := a.inner.(*tactics.Army)
	inner.P.Temper = src
	inner.P.Micro, inner.P.EngageAdj, inner.P.NavalAdj = false, 17, 31
	a.Init(b)
	if src.calls != 1 {
		t.Fatalf("detachment initialization read the temper %d times", src.calls)
	}
	for i := range a.force.d {
		if a.force.d[i].army.P != inner.P {
			t.Fatalf("detachment %d discarded configured tactics parameters", i)
		}
	}
}

type detBudgetBrain struct {
	a    *Army
	b    core.Board
	emit []int
}

func (*detBudgetBrain) Name() string { return "survival-budget-fixture" }
func (br *detBudgetBrain) Init(k *aikit.Kit) {
	st := &state{ready: true, cx: 2048, cz: 2048, dx: 2048, dz: 2048, outX: 1000}
	for i := range st.radius {
		st.radius[i], st.mine[i] = 800, true
	}
	p := tactics.DefaultParams()
	p.Raid, p.Harass, p.Probe, p.Tour = false, false, false, false
	br.a = &Army{st: st, inner: tactics.New(p)}
	br.b.K = k
	br.a.Init(&br.b)
}
func (br *detBudgetBrain) Think(k *aikit.Kit, o *aikit.Obs) {
	br.b.Update(k, o)
	br.b.Posture.AttackValue = 1 << 30
	br.a.Plan(&br.b)
	br.emit = append(br.emit, k.Emitted())
}

// All four mirrors share one real host batch. With one burst action the
// first think cannot issue one action per detachment, nor repeatedly fill
// four independent buckets on later thinks.
func TestDetachmentsShareTheHostActionBudget(t *testing.T) {
	wdef := &content.WeaponDef{ID: 1, DamageDefault: 25, ReloadTime: 30, Range: 200}
	def := &content.UnitDef{DefinitionHeader: content.DefinitionHeader{CanonicalKey: "defender"}, UnitName: "defender", Side: "ARM", CanMove: true, BMCode: 1, MaxDamage: 500, BuildCostMetal: 100, FootprintX: 2, FootprintZ: 2, Weapon1Def: wdef, MinWaterDepth: -10000, MaxSlope: 10}
	cat := &content.Catalog{Units: map[string]*content.UnitDef{"defender": def}}
	w := units.NewSliced(64, cat)
	def.Script = &cob.Program{Code: []uint32{0x10065000}}
	for i := 0; i < 24; i++ {
		_, err := w.Create(def, 0, numeric.FixedFromInt(2480+int64(i%6)*32), 0, numeric.FixedFromInt(1850+int64(i/6)*32))
		if err != nil {
			t.Fatal(err)
		}
	}
	e := &economy.Service{}
	e.Players[0].Exists, e.Players[0].ControllerState = true, 2
	m := &ai.Manager{Player: 0, Catalog: cat, Terrain: detTerrain(false)}
	p := aikit.PersonaHard
	p.Burst, p.APM, p.ThinkEvery, p.Reaction = 1, 1, 10, 3
	br := &detBudgetBrain{}
	host := aikit.NewHost(m, br, p)
	defer host.Close()
	for tick := uint32(1); tick < 150; tick++ {
		host.Step(tick, w, e)
	}
	host.Join()
	var total int
	for _, n := range br.emit {
		total += n
	}
	if len(br.emit) < 2 || total != 1 || host.Stats().DroppedAPM != 0 {
		t.Fatalf("budget mirrors emitted %v, total %d, drops %d, info %+v, fronts %d/%d/%d/%d", br.emit, total, host.Stats().DroppedAPM, host.Kit().Table.Units[0], br.a.force.d[0].n, br.a.force.d[1].n, br.a.force.d[2].n, br.a.force.d[3].n)
	}
}
