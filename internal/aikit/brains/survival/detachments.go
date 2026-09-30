package survival

import (
	"fmt"
	"strconv"

	"github.com/nanolathe-gg/nanolathe/internal/aikit"
	"github.com/nanolathe-gg/nanolathe/internal/aikit/brains/tactics"
	"github.com/nanolathe-gg/nanolathe/internal/aikit/core"
)

// These are Modern AI tuning, not retail mechanics (DESIGN_SURVIVAL §16.8).
// Three approaches cover the circle; a fourth policy retains the mobile
// support and a fifth of the ground units, reinforcing without moving the
// other detachments. Existing troops never change their approach on a warning.
const (
	fronts          = 3
	reserve         = fronts
	detachmentCount = fronts + 1
	reserveShare    = 20
	stageEvery      = 300
	stageRoom       = 64
	stageBlobMax    = 192
	refugeGap       = 500 // beyond tactics' retreat-arrival radius
)

type detOwner struct {
	gen  uint32
	info *aikit.UnitInfo
	id   int
	set  bool
}

type detReach struct {
	mc aikit.MoveClass
	r  *aikit.Reach
}

type detachment struct {
	army  *tactics.Army
	board core.Board
	obs   aikit.Obs
	// sector stays fixed for fronts; the reserve may move toward pressure.
	sector, facing int
	x, z           int32
	homeX, homeZ   int32 // a clear rear refuge, distinct from the waiting stage
	stageSet       bool
	stageAt        uint32
	n, ground      int32
	value          int64
	pressure       int64
	warning        int64
}

type detachments struct {
	enabled, placed bool
	d               [detachmentCount]detachment
	owners          []detOwner // handle plus generation, independent of tactics' role tags
	reach           []detReach
	// New units are allocated after the surviving memberships are counted,
	// so observation ordering cannot make reinforcements forget existing strength.
	fresh []int32
	// Instrumentation only.
	allocated int64
	stages    int64
}

func (ds *detachments) init(b *core.Board, inner core.Policy) {
	a, ok := inner.(*tactics.Army)
	if !ok {
		return
	}
	ds.enabled = true
	// The supplied policy was initialized first, including its temper.
	// Copy its resulting configuration rather than drawing a new personality.
	ds.d[reserve].army = a
	for i := 0; i < fronts; i++ {
		ds.d[i].army = tactics.New(a.P)
		ds.d[i].army.Init(b)
	}
	k := b.K
	ds.reach = make([]detReach, len(k.Table.Units))
	if k.Map.CellW <= 0 {
		return // a fixture with no terrain
	}
	for i, u := range k.Table.Units {
		if u.Side != k.Side || !u.Role.Has(aikit.RoleMobile) {
			continue
		}
		mc, ok := aikit.MoveClassOf(u)
		if !ok {
			continue
		}
		// Match the tactics and utility reach cache's footprint cap.
		lim := int32(3)
		if mc.MinDepth > 0 {
			lim = 4
		}
		mc.FootX, mc.FootZ = min(mc.FootX, lim), min(mc.FootZ, lim)
		ds.reach[i] = detReach{mc: mc, r: k.Map.Reach(mc)}
	}
}

func (ds *detachments) owner(u *aikit.OwnUnit) *detOwner {
	for len(ds.owners) <= int(u.H) {
		ds.owners = append(ds.owners, detOwner{})
	}
	m := &ds.owners[u.H]
	if m.gen != u.Gen || m.info != u.Info {
		*m = detOwner{gen: u.Gen, info: u.Info}
	}
	return m
}

// actor is what the army operates; builders and commanders remain the
// economy's and Survival guard's actors. Unarmed scouts have no Survival
// scouting task: nothing at a distant start position needs discovering.
func detActor(u *aikit.OwnUnit) bool {
	return u.Built && u.Info.Role.Has(aikit.RoleCombat) &&
		!u.Info.Role.Any(aikit.RoleBuilder|aikit.RoleCommander)
}

func detSupport(u *aikit.OwnUnit) bool {
	return u.Info.Role.Any(aikit.RoleAir | aikit.RoleNaval)
}

func (ds *detachments) front(s int) int {
	best, dist := 0, numSectors
	for i := 0; i < fronts; i++ {
		if d := ringDist(s, ds.d[i].sector); d < dist {
			best, dist = i, d
		}
	}
	return best
}

func (ds *detachments) plan(b *core.Board, st *state, near *aikit.Obs) {
	if !ds.placed {
		out := sectorOf(st.outX, st.outZ)
		for i := 0; i < fronts; i++ {
			ds.d[i].sector = (out + (i*numSectors+fronts/2)/fronts) % numSectors
			ds.d[i].facing = ds.d[i].sector
		}
		ds.d[reserve].sector, ds.d[reserve].facing = out, out
		ds.placed = true
	}
	for i := range ds.d {
		d := &ds.d[i]
		d.n, d.ground, d.value, d.pressure, d.warning = 0, 0, 0, 0, 0
	}
	// Pressure comes from observed attackers, not remembered positions.
	for i := range near.Enemy {
		c := &near.Enemy[i]
		if c.Info != nil && (c.Info.DPS <= 0 || !c.Info.Role.Has(aikit.RoleMobile)) {
			continue
		}
		s := sectorOf(int64(c.X-st.cx), int64(c.Z-st.cz))
		v := int64(100) // an untyped blip warrants a modest response
		if c.Info != nil {
			v = int64(max(c.Info.Value, 100))
		}
		ds.d[ds.front(s)].pressure += v
	}
	for _, g := range st.live {
		s := sectorOfAngle(g.Angle)
		ds.d[ds.front(s)].warning += 1
	}
	ds.fresh = ds.fresh[:0]
	var ground int32
	for i := range near.Own {
		u := &near.Own[i]
		if !detActor(u) {
			continue
		}
		if !detSupport(u) {
			ground++
		}
		m := ds.owner(u)
		if !m.set {
			ds.fresh = append(ds.fresh, int32(i))
			continue
		}
		ds.count(u, m.id)
	}
	// A reserve moves only its share of the army. A front's home bearing
	// stays fixed even when the other side has the strongest warning.
	best, score := reserve, int64(0)
	for i := 0; i < fronts; i++ {
		d := &ds.d[i]
		need := d.pressure*2 + d.warning*200 - d.value
		if need > score {
			best, score = i, need
		}
	}
	if best != reserve {
		ds.d[reserve].facing = ds.d[best].sector
	}
	for i := range ds.d {
		d := &ds.d[i]
		if d.stageAt == 0 || b.Tick-d.stageAt >= stageEvery {
			ds.stage(b, st, near, i)
		}
	}
	for _, idx := range ds.fresh {
		u := &near.Own[idx]
		id := reserve
		if !detSupport(u) && ds.d[reserve].ground >= max(1, (ground*reserveShare+99)/100) {
			id = ds.assign(b, st, u)
		}
		m := ds.owner(u)
		m.set, m.id = true, id
		ds.count(u, id)
		ds.allocated++
	}
	// Every policy runs once every think, including empty groups: each
	// budget mirror sees the same preceding batch and reaction window and
	// subtracts all commands earlier policies emitted through this shared Kit.
	// Observed pressure orders the fronts before the reserve and quiet fronts.
	var order [detachmentCount]int
	for i := range order {
		order[i] = i
	}
	for i := 0; i < detachmentCount; i++ {
		for j := i + 1; j < detachmentCount; j++ {
			if ds.d[order[j]].pressure > ds.d[order[i]].pressure {
				order[i], order[j] = order[j], order[i]
			}
		}
	}
	for _, id := range order {
		ds.picture(b, st, near, id)
		d := &ds.d[id]
		d.army.Plan(&d.board)
	}
}

func (ds *detachments) count(u *aikit.OwnUnit, id int) {
	d := &ds.d[id]
	d.n++
	d.value += int64(u.Info.Value)
	if !detSupport(u) {
		d.ground++
	}
}

func (ds *detachments) unitReach(u *aikit.OwnUnit) *detReach {
	if i := u.Info.Index; i >= 0 && int(i) < len(ds.reach) && ds.reach[i].r != nil {
		return &ds.reach[i]
	}
	return nil
}

func (ds *detachments) reaches(u *aikit.OwnUnit, x, z int32) bool {
	if rc := ds.unitReach(u); rc != nil {
		reg := rc.r.At(u.X, u.Z)
		return reg != 0 && rc.r.At(x, z) == reg
	}
	return true
}

func (ds *detachments) assign(b *core.Board, st *state, u *aikit.OwnUnit) int {
	best, score := reserve, int64(1<<62)
	for i := 0; i < fronts; i++ {
		d := &ds.d[i]
		x, z, hx, hz := d.x, d.z, d.homeX, d.homeZ
		if !d.stageSet {
			hx, hz = st.dx, st.dz
			ex, ez := ds.facingPoint(b, st, i)
			x, z = hx+int32(int64(ex-hx)*450/1000), hz+int32(int64(ez-hz)*450/1000)
		}
		if !ds.reaches(u, x, z) || !ds.reaches(u, hx, hz) {
			continue
		}
		// Weighted shares reinforce public warnings and present pressure.
		// A small distance preference avoids needless travel when shares tie.
		weight := int64(400) + d.pressure + d.warning*400
		if st.mine[d.sector] {
			weight += 200
		}
		v := (d.value+int64(u.Info.Value))*1000/weight + int64(aikit.Dist(u.X, u.Z, x, z))/16
		if v < score {
			best, score = i, v
		}
	}
	return best
}

// stage searches nearby bearings and radial rows for an open waiting disc
// with a separate rear refuge. Tactics uses Home for retreat and damaged-unit
// withdrawal, so making the forward stage Home would strand a losing army.
func (ds *detachments) stage(b *core.Board, st *state, o *aikit.Obs, id int) {
	d := &ds.d[id]
	d.stageAt = b.Tick
	room := int32(min(stageBlobMax, max(stageRoom, int32(aikit.ISqrt64(int64(d.ground)))*27)))
	if d.stageSet && (!ds.stageClear(st, o, d.x, d.z, room) || !ds.stageGround(b, o, id, d.x, d.z, room) ||
		!ds.stageClear(st, o, d.homeX, d.homeZ, room) || !ds.stageGround(b, o, id, d.homeX, d.homeZ, room)) {
		d.stageSet = false
	}
	for _, off := range [...]int{0, -1, 1, -2, 2} {
		s := (d.facing + off + numSectors) % numSectors
		r := max(towerRoom, st.radius[s]-128)
		if id == reserve {
			r = max(humanRoom+room+32, r-256)
		}
		for _, dr := range [...]int32{0, 128, -128, 256, -256, 384} {
			rr := clampI(int64(r+dr), int64(humanRoom+room+32), maxPerimeter)
			x := clampWorld(int64(st.cx)+sectorDir[s][0]*rr/1000, b.K.Map.WorldW)
			z := clampWorld(int64(st.cz)+sectorDir[s][1]*rr/1000, b.K.Map.WorldH)
			if !ds.stageClear(st, o, x, z, room) || !ds.stageGround(b, o, id, x, z, room) {
				continue
			}
			hx, hz, ok := ds.refuge(b, st, o, id, x, z, room)
			if !ok {
				continue
			}
			if !d.stageSet || aikit.Dist2(x, z, d.x, d.z) > 64*64 {
				ds.stages++
			}
			d.x, d.z = x, z
			d.homeX, d.homeZ = hx, hz
			d.stageSet = true
			return
		}
	}
}

func (ds *detachments) facingPoint(b *core.Board, st *state, id int) (int32, int32) {
	s := ds.d[id].facing
	return clampWorld(int64(st.dx)+sectorDir[s][0]*threatDist/1000, b.K.Map.WorldW),
		clampWorld(int64(st.dz)+sectorDir[s][1]*threatDist/1000, b.K.Map.WorldH)
}

// refuge searches inward first, then adjacent rear ground. The core must
// stay outside the straight segment as well as both waiting discs; ordinary
// movement remains responsible for routes around terrain and buildings.
func (ds *detachments) refuge(b *core.Board, st *state, o *aikit.Obs, id int, x, z, room int32) (int32, int32, bool) {
	vx, vz := int64(x-st.cx), int64(z-st.cz)
	r := int32(aikit.ISqrt64(vx*vx + vz*vz))
	s := sectorOf(vx, vz)
	for _, off := range [...]int{0, -1, 1, -2, 2, -3, 3, -4, 4} {
		bearing := (s + off + numSectors) % numSectors
		for _, back := range [...]int32{512, 384, 256, 640} {
			rr := max(humanRoom+room+32, r-back)
			if rr >= r {
				continue
			}
			hx := clampWorld(int64(st.cx)+sectorDir[bearing][0]*int64(rr)/1000, b.K.Map.WorldW)
			hz := clampWorld(int64(st.cz)+sectorDir[bearing][1]*int64(rr)/1000, b.K.Map.WorldH)
			if aikit.Dist2(x, z, hx, hz) < refugeGap*refugeGap || !refugeOutsideCore(st, x, z, hx, hz, room) ||
				!ds.stageClear(st, o, hx, hz, room) || !ds.stageGround(b, o, id, hx, hz, room) {
				continue
			}
			return hx, hz, true
		}
	}
	return 0, 0, false
}

func refugeOutsideCore(st *state, x, z, hx, hz, room int32) bool {
	dx, dz := int64(hx-x), int64(hz-z)
	den := dx*dx + dz*dz
	if den == 0 {
		return false
	}
	u := clampI(int64(st.cx-x)*dx+int64(st.cz-z)*dz, 0, den)
	px, pz := int64(x)+dx*u/den, int64(z)+dz*u/den
	cx, cz := px-int64(st.cx), pz-int64(st.cz)
	r := int64(humanRoom + room)
	return cx*cx+cz*cz >= r*r
}

func (ds *detachments) stageClear(st *state, o *aikit.Obs, x, z, room int32) bool {
	if aikit.Dist2(x, z, st.cx, st.cz) < int64(humanRoom+room)*int64(humanRoom+room) {
		return false
	}
	building := func(info *aikit.UnitInfo, bx, bz int32) bool {
		if info.Role.Has(aikit.RoleMobile) {
			return true
		}
		hx, hz := info.FootX*8+room+stageRoom, info.FootZ*8+room+stageRoom
		if x >= bx-hx && x <= bx+hx && z >= bz-hz && z <= bz+hz {
			return false
		}
		return !info.Role.Has(aikit.RoleFactory) || x < bx-hx || x > bx+hx || z < bz-room || z > bz+info.FootZ*8+allyLane+room
	}
	for i := range o.Own {
		u := &o.Own[i]
		if !building(u.Info, u.X, u.Z) {
			return false
		}
	}
	for i := range o.Allies {
		u := &o.Allies[i]
		if !building(u.Info, u.X, u.Z) {
			return false
		}
	}
	for i := range o.Features {
		f := &o.Features[i]
		if f.Blocking && absStage(x-f.X) <= f.FootX*8+room && absStage(z-f.Z) <= f.FootZ*8+room {
			return false
		}
	}
	return true
}

func absStage(v int32) int32 {
	if v < 0 {
		return -v
	}
	return v
}

func (ds *detachments) stageGround(b *core.Board, o *aikit.Obs, id int, x, z, room int32) bool {
	var all, in int64
	// Many units share one movement region. Test its waiting disc once per
	// candidate, not once per member; bounded scratch stays on the stack.
	var cache [32]struct {
		r      *aikit.Reach
		region int32
		open   bool
	}
	ncache := 0
	for i := range o.Own {
		u := &o.Own[i]
		if !detActor(u) || detSupport(u) {
			continue
		}
		m := ds.owner(u)
		if m.set && m.id != id {
			continue
		}
		w := int64(max(u.Info.Value, 1))
		all += w
		if !ds.reaches(u, x, z) {
			continue
		}
		if rc := ds.unitReach(u); rc != nil {
			region := rc.r.At(u.X, u.Z)
			cached := false
			open := true
			for j := 0; j < ncache; j++ {
				if cache[j].r == rc.r && cache[j].region == region {
					cached, open = true, cache[j].open
					break
				}
			}
			if !cached {
				for _, p := range [...]struct{ x, z int32 }{{0, 0}, {-room, 0}, {room, 0}, {0, -room}, {0, room}} {
					px, pz := x+p.x, z+p.z
					if rc.r.At(px, pz) != region || b.K.Map.InPassage(&rc.mc, px/16-rc.mc.FootX/2, pz/16-rc.mc.FootZ/2, rc.mc.FootX, rc.mc.FootZ) {
						open = false
						break
					}
				}
				if ncache < len(cache) {
					cache[ncache].r, cache[ncache].region, cache[ncache].open = rc.r, region, open
					ncache++
				}
			}
			if !open {
				continue
			}
		}
		in += w
	}
	return all == 0 || in*2 > all
}

func (ds *detachments) contact(st *state, id int, info *aikit.UnitInfo, x, z int32) bool {
	if id == reserve || ds.front(sectorOf(int64(x-st.cx), int64(z-st.cz))) == id {
		return true
	}
	// Adjacent threats whose weapons reach a detachment still enter its
	// danger picture, even across the front boundary.
	r := int64(768)
	if info != nil {
		r = max(r, int64(info.Range+192))
	}
	d := &ds.d[id]
	return aikit.Dist2(x, z, d.x, d.z) <= r*r
}

func (ds *detachments) picture(b *core.Board, st *state, o *aikit.Obs, id int) {
	d := &ds.d[id]
	v := &d.obs
	own, enemy, memory := v.Own[:0], v.Enemy[:0], v.Memory[:0]
	*v = *o
	for i := range o.Own {
		u := &o.Own[i]
		if detActor(u) {
			if ds.owner(u).id != id {
				continue
			}
		} else if u.Info.Role.Has(aikit.RoleMobile) && !u.Info.Role.Any(aikit.RoleBuilder|aikit.RoleCommander) {
			continue
		}
		copy := *u
		// An armed scout is a defender here, not a tour of enemy starts.
		if copy.Info.Role.Has(aikit.RoleScout) && (copy.Tag == 0 || copy.Tag == 20) {
			copy.Tag = 1
		}
		own = append(own, copy)
	}
	for i := range o.Enemy {
		c := &o.Enemy[i]
		if ds.contact(st, id, c.Info, c.X, c.Z) {
			enemy = append(enemy, *c)
		}
	}
	for i := range o.Memory {
		m := &o.Memory[i]
		if ds.contact(st, id, m.Info, m.X, m.Z) {
			memory = append(memory, *m)
		}
	}
	v.Own, v.Enemy, v.Memory = own, enemy, memory
	x, z := d.homeX, d.homeZ
	ex := x + int32(int64(d.x-x)*1000/450)
	ez := z + int32(int64(d.z-z)*1000/450)
	if !d.stageSet {
		// A cramped map or growing blob may refuse the open pair. Keep the
		// full group reactive through ordinary tactics instead of suspending
		// its actors. Each front retains its own bearing; only the reserve
		// follows pressure. Reach, passage and retreat still belong to tactics.
		x, z = st.dx, st.dz
		ex, ez = ds.facingPoint(b, st, id)
	}
	d.board.HomeX, d.board.HomeZ = x, z
	d.board.Update(b.K, v)
	if d.board.HomeX != x || d.board.HomeZ != z {
		// The first commander sighting initializes Board's home once.
		// Rebuild that first picture at the detachment's actual home.
		d.board.HomeX, d.board.HomeZ = x, z
		d.board.Update(b.K, v)
	}
	// With no local contact tactics gathers as far as 45% along its facing
	// line. End that search at the open stage while keeping Home behind it.
	// Observed enemies still drive its danger, combat and reach decisions.
	d.board.EnemyX, d.board.EnemyZ = ex, ez
	d.board.EnemyKnown = false
	d.board.RallyX, d.board.RallyZ = d.x, d.z
	if !d.stageSet {
		d.board.RallyX, d.board.RallyZ = x, z
	}
	d.board.Posture = b.Posture
}

func (ds *detachments) explain(x *aikit.Explain) {
	for i := range ds.d {
		d := &ds.d[i]
		if d.board.O == nil {
			continue
		}
		x.Notes = append(x.Notes, fmt.Sprintf("survival detachment %d: sector %d, %d units (%d ground), value %d, pressure %d, warnings %d, stage (%d,%d), refuge (%d,%d), safe %v", i, d.sector, d.n, d.ground, d.value, d.pressure, d.warning, d.x, d.z, d.homeX, d.homeZ, d.stageSet))
		if !d.stageSet {
			x.Notes = append(x.Notes, fmt.Sprintf("survival detachment %d: fallback at defence centre (%d,%d), facing sector %d", i, d.board.HomeX, d.board.HomeZ, d.facing))
		}
		nsq, ng, ngoal := len(x.Squads), len(x.Grids), len(x.Goals)
		d.army.Explain(&d.board, x)
		prefix := "detachment " + strconv.Itoa(i) + ": "
		for j := nsq; j < len(x.Squads); j++ {
			x.Squads[j].ID += int32(i) * 32
			x.Squads[j].Task = prefix + x.Squads[j].Task
		}
		for j := ng; j < len(x.Grids); j++ {
			x.Grids[j].Name = prefix + x.Grids[j].Name
		}
		for j := ngoal; j < len(x.Goals); j++ {
			x.Goals[j].Label = prefix + x.Goals[j].Label
		}
	}
}

func (ds *detachments) report(add func(string, int64)) {
	add("sv_detachments", detachmentCount)
	add("sv_det_allocated", ds.allocated)
	add("sv_det_stages", ds.stages)
	for i := range ds.d {
		d := &ds.d[i]
		prefix := "sv_det_" + strconv.Itoa(i) + "_"
		add(prefix+"units", int64(d.n))
		add(prefix+"ground", int64(d.ground))
		add(prefix+"value", d.value)
		add(prefix+"pressure", d.pressure)
		add(prefix+"warnings", d.warning)
		add(prefix+"sector", int64(d.sector))
		add(prefix+"facing", int64(d.facing))
		add(prefix+"home_x", int64(d.board.HomeX))
		add(prefix+"home_z", int64(d.board.HomeZ))
		add(prefix+"stage_x", int64(d.x))
		add(prefix+"stage_z", int64(d.z))
		add(prefix+"refuge_x", int64(d.homeX))
		add(prefix+"refuge_z", int64(d.homeZ))
		safe := int64(0)
		if d.stageSet {
			safe = 1
		}
		add(prefix+"stage_safe", safe)
		add(prefix+"fallback", 1-safe)
		d.army.Report(func(name string, v int64) { add(prefix+name, v) })
	}
}
