package utility

import (
	"github.com/nanolathe-gg/nanolathe/internal/aikit"
	"github.com/nanolathe-gg/nanolathe/internal/aikit/core"
	"github.com/nanolathe-gg/nanolathe/internal/content"
	"github.com/nanolathe-gg/nanolathe/internal/pool"
	"github.com/nanolathe-gg/nanolathe/internal/sim/numeric"
)

// Strategic weapons are controller policy, not sustained firepower or a
// gameplay rule (DESIGN_SESSIONS_AI_SAVE "Modern AI stockpile weapons").
// The ordinary engine still owns production, launch and interception.
const (
	strategicReady    = 15 * 1800 // proactive investment after fifteen minutes
	strategicFinance  = 180       // income horizon for a launcher and its first round
	stockpileFinance  = 120       // income horizon for a replacement round
	strategicRetarget = 1800      // rest a deliberate ground target for one minute
)

func strategicWeapons(u *aikit.UnitInfo) [3]*content.WeaponDef {
	if u == nil || u.Def == nil {
		return [3]*content.WeaponDef{}
	}
	return [3]*content.WeaponDef{u.Def.Weapon1Def, u.Def.Weapon2Def, u.Def.Weapon3Def}
}

func strategicWeapon(w *content.WeaponDef) bool {
	return w != nil && !content.IsWeaponInactive(w) && w.Stockpile
}

func roundCosts(w *content.WeaponDef) (metal, energy int64) {
	return max64(int64(numeric.TruncateFloat64ToLow32(w.MetalPerShot)), 0),
		max64(int64(numeric.TruncateFloat64ToLow32(w.EnergyPerShot)), 0)
}

// funded keeps a fifth of capacity available to ordinary work. Large
// savings can fund a purchase without income; otherwise gross income must
// cover it over the horizon and the resource must not currently be stalling.
func strategicFunded(r aikit.Res, cost int64, horizon int64) bool {
	reserve := max64(int64(r.Cap)/5, 0)
	stock := int64(r.Stock)
	if stock >= cost+reserve {
		return true
	}
	return stock >= reserve && r.Income > 0 && r.Income >= r.Expense && int64(r.Income)*horizon >= cost
}

func stockpileThreat(o *aikit.Obs) bool {
	for _, r := range o.Memory {
		if int(r.Owner) >= len(o.Allied) || o.Allied[r.Owner] || r.Owner == o.Me {
			continue
		}
		for _, w := range strategicWeapons(r.Info) {
			if strategicWeapon(w) && !w.Interceptor && w.Targetable {
				return true
			}
		}
	}
	return false
}

// strategicCount includes nanoframes and this think's reservations, not
// just finished units. One launcher and one home interceptor are the
// initial policy; losses make their replacements eligible again.
func (s *shared) strategicCount(role aikit.Role) int32 {
	var n int32
	for _, u := range s.k.Table.Units {
		if u.Role.Has(role) {
			n += s.count[u.Index]
		}
	}
	return n
}

func (e *Economy) evalStrategic(b *core.Board, builder *aikit.OwnUnit, p *aikit.UnitInfo) cand {
	c := cand{kind: cStrategic, prod: p, spot: -1, spacing: 3}
	anti := p.Role.Has(aikit.RoleInterceptor)
	role := aikit.RoleStockpile
	if anti {
		role = aikit.RoleInterceptor
	}
	if e.s.strategicCount(role) != 0 || (anti && e.s.interceptorQueued) || (b.Tick < strategicReady && !(anti && stockpileThreat(b.O))) {
		return c
	}
	travel, threat, ok := e.place(b, builder, &c)
	if !ok {
		return c
	}
	if anti && !e.s.info[p.Index].water {
		// Defend the home economy rather than a front-line tower position.
		c.x, c.z = b.HomeX, b.HomeZ
		travel = half(int64(aikit.Dist(builder.X, builder.Z, c.x, c.z))/speedOf(builder.Info), int64(e.s.p.TravelHalf))
		threat = half(int64(b.Threat.At(c.x, c.z)), int64(e.s.p.ThreatHalf))
	}
	for _, w := range strategicWeapons(p) {
		if !strategicWeapon(w) || w.Interceptor != anti || (anti && w.Coverage <= 0) {
			continue
		}
		m, en := roundCosts(w)
		if !strategicFunded(b.Metal, int64(p.Metal)+m, strategicFinance) || !strategicFunded(b.Energy, int64(p.Energy)+en, strategicFinance) {
			continue
		}
		if !anti {
			if _, _, ok := strategicTarget(b.O, c.x, c.z, w, p); !ok {
				continue
			}
		}
		weight := int64(1200)
		if anti && stockpileThreat(b.O) {
			weight = 3000
		}
		c.score = mul(mul(mul(weight, e.afford(p)), travel), threat)
		c.f = [4]int64{weight, e.afford(p), travel, threat}
		break
	}
	return c
}

// Mobile interceptors are a factory alternative when no currently completed
// builder offers a stationary system. A pending factory request reserves the
// same one-system budget, even before its nanoframe exists.
func (pr *Production) interceptorScore(b *core.Board, p *aikit.UnitInfo) int64 {
	s := pr.s
	if s.strategicCount(aikit.RoleInterceptor) != 0 || s.interceptorQueued || (b.Tick < strategicReady && !stockpileThreat(b.O)) {
		return 0
	}
	for _, i := range b.Builders {
		for _, q := range b.O.Own[i].Info.Builds {
			if q.Role.Has(aikit.RoleInterceptor) && !q.Role.Has(aikit.RoleMobile) && !s.info[q.Index].water && s.prodBlock[q.Index] <= s.tick {
				return 0
			}
		}
	}
	for _, w := range strategicWeapons(p) {
		if !strategicWeapon(w) || !w.Interceptor || w.Coverage <= 0 {
			continue
		}
		m, en := roundCosts(w)
		if strategicFunded(b.Metal, int64(p.Metal)+m, strategicFinance) && strategicFunded(b.Energy, int64(p.Energy)+en, strategicFinance) {
			if stockpileThreat(b.O) {
				return 3000
			}
			return 1200
		}
	}
	return 0
}

func (pr *Production) hasQueuedInterceptor(b *core.Board) bool {
	for _, i := range b.Factories {
		f := &b.O.Own[i]
		r := pr.regOf(f)
		if f.QueueLen > 0 && ((f.Target != nil && f.Target.Role.Has(aikit.RoleInterceptor)) || (r.prod != nil && r.prod.Role.Has(aikit.RoleInterceptor))) {
			return true
		}
	}
	return false
}

// strategicTarget uses identified, remembered buildings only. Radar blips
// and guessed starts cannot identify an expensive target. Ties retain the
// observation's deterministic order. This is a deliberate targeting policy;
// it does not replace the engine's ordinary autonomous acquisition.
func strategicTarget(o *aikit.Obs, x, z int32, w *content.WeaponDef, carrier *aikit.UnitInfo) (int32, int32, bool) {
	if !strategicWeapon(w) || w.Interceptor || w.Range <= 0 {
		return 0, 0, false
	}
	m, en := roundCosts(w)
	minimum := max64((m+en/aikit.EnergyPerMetal)/2, 100)
	var best *aikit.Remembered
	var value int64
	for i := range o.Memory {
		r := &o.Memory[i]
		if r.Info == nil || !r.Building || r.Owner == o.Me || int(r.Owner) >= len(o.Allied) || o.Allied[r.Owner] {
			continue
		}
		v := int64(r.Info.Value)
		if v < minimum || v <= value || aikit.Dist2(x, z, r.X, r.Z) > int64(w.Range)*int64(w.Range) {
			continue
		}
		if !strategicSafe(o, r.X, r.Z, carrier) {
			continue
		}
		best, value = r, v
	}
	if best == nil {
		return 0, 0, false
	}
	return best.X, best.Z, true
}

func strategicSafe(o *aikit.Obs, x, z int32, carrier *aikit.UnitInfo) bool {
	// Ordinary unit area damage uses the halved authored area [06 §9.3].
	// Add a footprint pad and 64 world units for movement during reaction;
	// this is a conservative planning margin, not altered damage geometry.
	// Ordinary ground attack binds slots zero AND one [04 R-ORD-01 §3].
	// Cover the larger blast even if the companion weapon is not a stockpile.
	var area int32
	weapons := strategicWeapons(carrier)
	for _, w := range weapons[:2] {
		if w != nil && !content.IsWeaponInactive(w) && w.AreaOfEffect > area {
			area = w.AreaOfEffect
		}
	}
	radius := int64(area/2) + 64
	near := func(ux, uz int32, info *aikit.UnitInfo) bool {
		pad := int64(0)
		if info != nil {
			pad = int64(info.FootX+info.FootZ) * 8
		}
		r := radius + pad
		return aikit.Dist2(x, z, ux, uz) <= r*r
	}
	for _, u := range o.Own {
		if near(u.X, u.Z, u.Info) {
			return false
		}
	}
	for _, u := range o.Allies {
		if near(u.X, u.Z, u.Info) {
			return false
		}
	}
	return true
}

type strategicOrder struct {
	def    *aikit.UnitInfo
	gen    uint32
	issued bool
	x, z   int32
	tick   uint32
	ammo   int32
}

type strategicPlan struct {
	orders []strategicOrder
}

// service runs before new building assignments, with the same action
// budget. It also works without a living constructor, and after a load:
// stock and outstanding work come from detached observations.
func (p *strategicPlan) service(b *core.Board, budget int) int {
	used := 0
	var one [1]pool.Handle
	for i := range b.O.Own {
		u := &b.O.Own[i]
		if !u.Built || !u.Info.Role.Any(aikit.RoleStockpile|aikit.RoleInterceptor) {
			continue
		}
		if used >= budget {
			break
		}
		p.orders = handleSlots(p.orders, u.H, 0)
		r := &p.orders[u.H]
		if r.def != u.Info || r.gen != u.Gen {
			*r = strategicOrder{def: u.Info, gen: u.Gen}
		}
		one[0] = u.H
		for slot, w := range strategicWeapons(u.Info) {
			if !strategicWeapon(w) || used >= budget {
				continue
			}
			// Plan deliberate fire for a primary stockpile; ordinary ground
			// attack also binds slot one, whose blast is included in safety.
			if slot == 0 && !w.Interceptor {
				x, z, target := strategicTarget(b.O, u.X, u.Z, w, u.Info)
				if r.issued && u.Order != aikit.OrderAttack {
					// A rejected or completed order must not retain the intent
					// forever. Its normal retry cooldown still bounds reissues.
					r.issued = false
				}
				if r.issued && (u.Ammo[slot] < r.ammo || !target || x != r.x || z != r.z) {
					b.K.Stop(one[:])
					r.issued = false
					used++
				} else if !r.issued && target && u.Ammo[slot] > 0 && (r.tick == 0 || b.Tick-r.tick >= strategicRetarget) {
					b.K.AttackPos(one[:], x, z, false)
					r.issued, r.x, r.z, r.tick, r.ammo = true, x, z, b.Tick, u.Ammo[slot]
					used++
				}
			}
			want := int32(1)
			if w.Interceptor {
				want = 3
			}
			if used < budget && u.StockpileQueued[slot] == 0 && u.Ammo[slot] < want {
				m, en := roundCosts(w)
				if strategicFunded(b.Metal, m, stockpileFinance) && strategicFunded(b.Energy, en, stockpileFinance) {
					b.K.Stockpile(u.H, int32(slot), 1)
					used++
				}
			}
		}
		// A mobile interceptor waits near the economy it protects; never
		// send it into an army wave or repeatedly replace an ongoing move.
		if used < budget && u.Info.Role.Has(aikit.RoleInterceptor|aikit.RoleMobile) && u.Order == aikit.OrderIdle {
			for _, w := range strategicWeapons(u.Info) {
				if !strategicWeapon(w) || !w.Interceptor || w.Coverage <= 0 {
					continue
				}
				r := int64(w.Coverage) / 2
				if aikit.Dist2(u.X, u.Z, b.HomeX, b.HomeZ) > r*r {
					b.K.Move(one[:], b.HomeX, b.HomeZ, false)
					used++
				}
				break
			}
		}
	}
	return used
}
