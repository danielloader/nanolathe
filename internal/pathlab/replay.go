package pathlab

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/nanolathe-gg/nanolathe/internal/content"
	"github.com/nanolathe-gg/nanolathe/internal/gameplay"
	"github.com/nanolathe-gg/nanolathe/internal/headless"
	"github.com/nanolathe-gg/nanolathe/internal/movement"
	"github.com/nanolathe-gg/nanolathe/internal/orders"
	"github.com/nanolathe-gg/nanolathe/internal/pool"
	"github.com/nanolathe-gg/nanolathe/internal/session"
	"github.com/nanolathe-gg/nanolathe/internal/sim/numeric"
	"github.com/nanolathe-gg/nanolathe/internal/units"
	"github.com/nanolathe-gg/nanolathe/internal/world"
)

// ReplayOptions selects how a snippet is replayed.
type ReplayOptions struct {
	// Rules is the gameplay rule set's name.
	Rules string
	// Seed seeds both deterministic streams.
	Seed uint32
	// Timing records the wall time of every tick.
	Timing bool
	// Frames, when positive, records every staged unit's position every
	// that many ticks for rendering.
	Frames int
	// ExtraTicks runs the replay past the snippet's window.
	ExtraTicks int
	// Jitter, when positive, moves every staged mobile unit by up to two
	// world units, by a fixed function of its ID and this number. Crowds are
	// chaotic: one trajectory per rule set cannot rank two rule sets, so a
	// comparison replays each under the same few jitters.
	Jitter int
}

// Frame is every staged unit's state at one tick, for rendering.
type Frame struct {
	Tick  int32      `json:"tick"`
	Units [][6]int32 `json:"units"` // id, x, z, heading, blocked, moving
}

// ReplayResult is a replay's log and frames.
type ReplayResult struct {
	Log    *MovesLog `json:"log"`
	Frames []Frame   `json:"frames,omitempty"`
}

type staged struct {
	su      *SnipUnit
	u       *units.Unit
	h       uint16
	log     *MovesUnit
	lastB   bool
	lastN   int8
	lastP   [3][2]int32
	hasLast bool
	mobile  bool
	dead    bool
	fx, fz  int16
	// Running totals behind MovesUnit.Motion, in 256ths of a world unit
	// and in heading units.
	hasPrev      bool
	prevX, prevZ int64
	prevHeading  uint16
	prevSpeed    int32
	goalX, goalZ int64
	prevGoalD    int64
	dist, away   int64
	turn         int64
	stops        int32
	// The turn in progress and the one before it, for reversals: the way
	// it goes, how far it has gone in heading units, and the tick it last
	// grew.
	runSign           int8
	runTurn, prevTurn int64
	runTick, prevTick int32
	reversals         int32
	now               int32
	counted           bool
	// The goal last written to MovesUnit.Goals.
	hasGoal            bool
	logGoalX, logGoalZ int32
}

// goal notes the goal of the order at the head of the unit's queue when it
// has changed.
func (st *staged) goal(now int32) {
	q := orders.QueueOfUnit(st.u)
	if q == nil {
		return
	}
	head := q.Head()
	if head == nil || head.ID != orders.Lookup("Move_Ground") {
		return
	}
	gx, gz := int32(head.GoalX>>16), int32(head.GoalZ>>16)
	if st.hasGoal && gx == st.logGoalX && gz == st.logGoalZ {
		return
	}
	st.hasGoal, st.logGoalX, st.logGoalZ = true, gx, gz
	st.log.Goals = append(st.log.Goals, [3]int32{now, gx, gz})
}

// A reversal is a turn of reversalTurn heading units or more one way followed
// within reversalTicks by one as large the other way.
const (
	reversalTurn  = 0x800 // an eighth of a quarter turn
	reversalTicks = 45
)

// isqrt is the integer square root.
func isqrt(v int64) int64 {
	if v <= 0 {
		return 0
	}
	x := int64(1) << 31
	for {
		y := (x + v/x) >> 1
		if y >= x {
			return x
		}
		x = y
	}
}

// motion advances a staged unit's running totals by one tick; routed
// reports that it holds an active route.
func (st *staged) motion(routed bool) {
	x, z := int64(st.u.X), int64(st.u.Z)
	heading, speed := st.u.Move.Heading, int32(st.u.Move.Speed)
	if st.hasPrev {
		dx, dz := (x-st.prevX)>>8, (z-st.prevZ)>>8
		st.dist += isqrt(dx*dx + dz*dz)
		t := int64(int16(heading - st.prevHeading))
		sign := int8(1)
		if t < 0 {
			t, sign = -t, -1
		}
		st.turn += t
		if t != 0 {
			if sign != st.runSign {
				// The turn in progress ends; one the other way begins.
				st.prevTurn, st.prevTick = st.runTurn, st.runTick
				st.runSign, st.runTurn = sign, 0
				st.counted = false
			}
			st.runTurn += t
			st.runTick = st.now
			if !st.counted && st.runTurn >= reversalTurn && st.prevTurn >= reversalTurn && st.now-st.prevTick <= reversalTicks {
				st.reversals++
				st.counted = true
			}
		}
		if st.prevSpeed > 0 && speed == 0 && routed {
			st.stops++
		}
	}
	goalD := int64(-1)
	if routed {
		if q := orders.QueueOfUnit(st.u); q != nil {
			if head := q.Head(); head != nil {
				gx, gz := int64(head.GoalX), int64(head.GoalZ)
				dx, dz := (x-gx)>>8, (z-gz)>>8
				goalD = isqrt(dx*dx + dz*dz)
				if st.prevGoalD >= 0 && gx == st.goalX && gz == st.goalZ && goalD > st.prevGoalD {
					st.away += goalD - st.prevGoalD
				}
				st.goalX, st.goalZ = gx, gz
			}
		}
	}
	st.prevGoalD = goalD
	st.hasPrev, st.prevX, st.prevZ, st.prevHeading, st.prevSpeed = true, x, z, heading, speed
}

// Replay stages the snippet in a fresh session on its map and runs it under
// the named rule set. The session is an ordinary skirmish composition with
// every computer player passive, every unit told to hold fire, and the whole
// map known to every player; commanders the snippet does not name are parked
// away from its region.
func Replay(c *Content, s *Snippet, opt ReplayOptions) (*ReplayResult, error) {
	if c == nil || c.Catalog == nil || s == nil {
		return nil, fmt.Errorf("nanolathe: replay needs content and a snippet: logical path <replay>, providers searched [], expected both")
	}
	mapHeader := c.Catalog.Maps[content.CanonicalKey(s.Map)]
	if mapHeader == nil {
		return nil, fmt.Errorf("nanolathe: snippet map is not installed: logical path maps/%s, providers searched [%s], expected the map the recording was played on", s.Map, strings.Join(c.Roots, ", "))
	}
	mode, err := gameplay.Parse(opt.Rules)
	if err != nil {
		return nil, err
	}
	// Session slots: one per recording player with staged units, and a
	// hostile bystander when the snippet names a single player, because a
	// skirmish needs an opponent.
	slotOf := map[int]uint8{}
	for _, p := range s.Players {
		slotOf[p] = uint8(len(slotOf))
	}
	n := len(slotOf)
	if n < 2 {
		n = 2
	}
	if n > 10 {
		return nil, fmt.Errorf("nanolathe: snippet names too many players: logical path %s, providers searched [snippet], expected at most 10", s.ID)
	}
	sides := map[uint8]int{}
	for i := range s.Units {
		if strings.HasPrefix(strings.ToUpper(s.Units[i].Name), "COR") {
			sides[slotOf[s.Units[i].Player]]++
		} else {
			sides[slotOf[s.Units[i].Player]]--
		}
	}
	cfg := session.SkirmishConfig{MapName: mapHeader.Name, NumPlayers: n}
	cfg.ApplyDefaults()
	for i := 0; i < n; i++ {
		p := session.SkirmishPlayer{Controller: session.SkirmishControllerComputer, AllyGroup: i % 5, Color: i, Metal: 1000, Energy: 1000}
		if i == 0 {
			p.Controller = session.SkirmishControllerHuman
		}
		if sides[uint8(i)] > 0 {
			p.Side = 1
		}
		if i >= 5 {
			p.AllyGroup = 5
		}
		cfg.Players[i] = p
	}
	cfg.Location = 1
	cfg.CommanderDeath = int(session.CommanderDeathContinues)
	cfg.UnitLimit = 1500
	cfg.Mapping, cfg.LineOfSight = 0, 0
	fb, err := headless.ComposeFreshBattle(headless.FreshBattleRequest{
		Kind: headless.ScenarioDirectOTA, Map: cfg.MapName, LocalOwner: -1, Gameplay: mode, Difficulty: 1,
		Skirmish: cfg, SimulationSeed: opt.Seed, CRTSeed: opt.Seed, FS: c.View, Catalog: c.Catalog,
		CommunitySources: session.CommunitySources{Content: c.Features},
	})
	if err != nil {
		return nil, err
	}
	sess := fb.Session
	sess.Snapshot = nil
	for i := range sess.AI {
		if sess.AI[i] != nil {
			sess.AI[i].Passive = true
		}
	}
	sess.Movement.BindWorld(sess.Units)
	sess.Movement.CountRefusals()

	info := &EngineInfo{Rules: opt.Rules, Snippet: s.ID}
	res := &ReplayResult{Log: &MovesLog{Source: "sim", ID: s.ID + "@" + opt.Rules, Map: s.Map, MaxUnits: 1500, Status: "moves", Engine: info}}
	info.Jitter = opt.Jitter
	maxPlayer := 0
	for _, p := range s.Players {
		maxPlayer = max(maxPlayer, p)
	}
	res.Log.Players = make([]MovesPlayer, maxPlayer+1)
	for i := range res.Log.Players {
		res.Log.Players[i] = MovesPlayer{Number: i + 1, Units: []*MovesUnit{}}
	}

	r := &replay{sess: sess, s: s, info: info, slotOf: slotOf, byID: map[int]*staged{}, jitter: opt.Jitter}
	if err := r.parkCommanders(); err != nil {
		return nil, err
	}
	for _, cell := range s.Reclaimed {
		sess.Features.ReclaimAt(int(cell[0]), int(cell[1]))
	}
	// Structures first so every mover is checked against their stamps.
	order := make([]int, 0, len(s.Units))
	for i := range s.Units {
		if s.Units[i].Structure {
			order = append(order, i)
		}
	}
	for i := range s.Units {
		if !s.Units[i].Structure {
			order = append(order, i)
		}
	}
	for _, i := range order {
		if s.Units[i].Spawn <= s.Start {
			r.stage(&s.Units[i], res.Log)
		}
	}
	// The first step only finishes loading.
	for sess.Clock.GlobalTick == 0 && sess.State != session.StatePostBattle {
		before := sess.Clock.GlobalTick
		sess.Step(sess.Clock.ScaledAnchor + 1)
		if sess.Clock.GlobalTick != before {
			break
		}
		if r.guard++; r.guard > 8 {
			break
		}
	}
	// A follower's route request is admitted only sixty ticks after its last
	// admitted one, and a unit that has never requested carries a stamp of
	// zero [04 R-MOV-01 §7], so nothing is admitted in a session's first
	// sixty ticks. The staged units stand through those before the window
	// opens; a recorded game is long past them.
	for k := 0; k < warmupTicks; k++ {
		before := sess.Clock.GlobalTick
		for sess.Clock.GlobalTick == before && sess.State != session.StatePostBattle {
			sess.Step(sess.Clock.ScaledAnchor + 1)
		}
	}
	window := int(s.T1-s.Start) + opt.ExtraTicks
	oi, gi := 0, 0
	sort.SliceStable(s.Orders, func(i, j int) bool { return s.Orders[i].Tick < s.Orders[j].Tick })
	r.grouped = make([]bool, len(s.Groups))
	if opt.Timing {
		info.TickNS = make([]int64, 0, window)
	}
	for k := 1; k <= window; k++ {
		now := s.Start + int32(k) // the recording tick this step simulates
		for _, i := range order {
			su := &s.Units[i]
			if su.Spawn == now && su.Spawn > s.Start {
				r.stage(su, res.Log)
			}
			if su.Die == now {
				r.kill(su, now)
			}
		}
		for gi < len(s.Groups) && s.Groups[gi].Tick <= now {
			r.group(&s.Groups[gi])
			gi++
		}
		for oi < len(s.Orders) && s.Orders[oi].Tick <= now {
			if o := &s.Orders[oi]; o.Group == 0 || !r.grouped[o.Group-1] {
				r.order(o)
			}
			oi++
		}
		t0 := time.Now()
		before := sess.Clock.GlobalTick
		for sess.Clock.GlobalTick == before {
			if sess.State == session.StatePostBattle {
				info.Notes = append(info.Notes, fmt.Sprintf("battle ended at window tick %d", k))
				break
			}
			sess.Step(sess.Clock.ScaledAnchor + 1)
		}
		if opt.Timing {
			info.TickNS = append(info.TickNS, time.Since(t0).Nanoseconds())
		}
		if sess.State == session.StatePostBattle {
			break
		}
		r.observe(now, opt, res)
		res.Log.LastTick = int64(now)
	}
	return res, nil
}

// warmupTicks is how long the staged scene stands before the window opens.
const warmupTicks = 64

type replay struct {
	sess   *session.Session
	s      *Snippet
	info   *EngineInfo
	slotOf map[int]uint8
	byID   map[int]*staged
	all    []*staged
	guard  int
	jitter int
	// grouped marks the group commands that were issued as such; the
	// orders of one that could not be are given singly.
	grouped []bool
}

// parkCommanders moves each player's composed commander to the map corner
// farthest from the snippet's region, unless the snippet stages that
// commander itself, in which case the composed one takes its place.
func (r *replay) parkCommanders() error {
	sess := r.sess
	cx, cz := (r.s.Region[0]+r.s.Region[2])/2, (r.s.Region[1]+r.s.Region[3])/2
	w, h := float64(sess.World.CellW)*16, float64(sess.World.CellH)*16
	type corner struct{ x, z float64 }
	corners := []corner{{96, 96}, {w - 96, 96}, {96, h - 96}, {w - 96, h - 96}}
	sort.SliceStable(corners, func(i, j int) bool {
		far := func(c corner) float64 { return (c.x-cx)*(c.x-cx) + (c.z-cz)*(c.z-cz) }
		return far(corners[i]) > far(corners[j])
	})
	slot := 0
	for _, u := range sess.Units.IterSliced() {
		if u == nil || !u.Alive || u.Def == nil || !u.Def.Commander {
			continue
		}
		placed := false
		// Spiral out from the corner over a coarse lattice.
		for ring := 0; ring < 40 && !placed; ring++ {
			for dz := -ring; dz <= ring && !placed; dz++ {
				for dx := -ring; dx <= ring && !placed; dx++ {
					if max(abs(dx), abs(dz)) != ring {
						continue
					}
					x := corners[0].x + float64(dx)*48 + float64(slot)*0
					z := corners[0].z + float64(dz)*48
					if x < 64 || z < 64 || x > w-64 || z > h-64 {
						continue
					}
					fx, fz := numeric.FixedFromInt(int64(x)), numeric.FixedFromInt(int64(z))
					if !r.free(u.Def, fx, fz, u) {
						continue
					}
					if sess.Movement.PlaceUnit(orders.PlaceRequest{Unit: u.Handle, X: fx, Y: sess.World.HeightAt(fx, fz), Z: fz}) {
						placed = true
					}
				}
			}
		}
		if !placed {
			r.info.Notes = append(r.info.Notes, fmt.Sprintf("commander of slot %d left at its start position", u.Owner))
		}
		holdFire(u)
		slot++
	}
	return nil
}

// fixed converts a world coordinate read from a snippet to 16.16. It is
// staging input, not authoritative arithmetic [I2].
func fixed(v float64) numeric.Fixed { return numeric.Fixed(int64(math.Round(v * 65536))) }

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

// holdFire sets the unit's standing fire order to hold fire, so a replay
// measures movement and no weapon changes it.
func holdFire(u *units.Unit) {
	u.Flags &^= uint32(units.StandingFieldMask) << units.StandingFireShift
}

// profileOf resolves the movement profile a definition's units take.
func (r *replay) profileOf(def *content.UnitDef) movement.Profile {
	if def.MovementClass != "" {
		if mc := r.sess.Catalog.Movement[content.CanonicalKey(def.MovementClass)]; mc != nil {
			return movement.NewProfile(mc)
		}
	}
	return movement.NewScratchProfile(def)
}

// free reports whether a mobile unit of def may stand at x, z: every cell
// of its footprint is in bounds, statically passable for its profile, and
// held by no unit other than self.
func (r *replay) free(def *content.UnitDef, x, z numeric.Fixed, self *units.Unit) bool {
	p := r.profileOf(def)
	cs := movement.CollisionState{X: int32(x.Raw()), Z: int32(z.Raw()), FootPrintX: int16(p.FootPrintX), FootPrintZ: int16(p.FootPrintZ), Mode: 1}
	a := cs.ProposedAnchor(1)
	fx, fz := int32(p.FootPrintX), int32(p.FootPrintZ)
	if a.X < 0 || a.Z < 0 || a.X+fx > r.sess.World.CellW || a.Z+fz > r.sess.World.CellH {
		return false
	}
	if !p.IsPassableFootprint(r.sess.World, a.X, a.Z) {
		return false
	}
	for dz := int32(0); dz < fz; dz++ {
		for dx := int32(0); dx < fx; dx++ {
			if occ, ok := r.sess.Movement.Grid.OccupantAtPlane(movement.PlaneGround, movement.Cell{X: a.X + dx, Z: a.Z + dz}); ok {
				if self == nil || occ != int(self.Handle) {
					return false
				}
			}
		}
	}
	return true
}

// stage creates one snippet unit. A mobile unit whose recorded position is
// not free — the reconstruction places a unit within about a cell — is
// moved to the nearest free position within three cells, and counted.
func (r *replay) stage(su *SnipUnit, log *MovesLog) {
	sess := r.sess
	def, ok := sess.Catalog.Unit(su.Name)
	if !ok || def == nil {
		r.info.Dropped++
		r.info.Notes = append(r.info.Notes, fmt.Sprintf("unit %d: no definition named %s", su.ID, su.Name))
		return
	}
	owner := r.slotOf[su.Player]
	x, z := fixed(su.X), fixed(su.Z)
	st := &staged{su: su, mobile: def.BMCode != 0}
	if def.Commander {
		// The composed commander stands in for the recorded one.
		for _, u := range sess.Units.IterSliced() {
			if u != nil && u.Alive && u.Owner == owner && u.Def != nil && u.Def.Commander {
				if r.place(def, &x, &z, u) && sess.Movement.PlaceUnit(orders.PlaceRequest{Unit: u.Handle, X: x, Y: sess.World.HeightAt(x, z), Z: z}) {
					st.u = u
				}
				break
			}
		}
		if st.u == nil {
			r.info.Dropped++
			return
		}
	} else if !st.mobile {
		ax, az := world.PlacementAnchor(x, z, def.FootprintX, def.FootprintZ)
		x, z = world.PlacementCenter(ax, az, def.FootprintX, def.FootprintZ)
		h, err := sess.Units.Create(def, owner, x, sess.World.HeightAt(x, z), z)
		if err != nil {
			r.info.Dropped++
			r.info.Notes = append(r.info.Notes, fmt.Sprintf("structure %d %s: %v", su.ID, su.Name, err))
			return
		}
		st.u = sess.Units.Unit(h)
		sess.Movement.EnsureUnit(st.u)
	} else {
		if r.jitter > 0 {
			x += numeric.FixedFromInt(int64((su.ID*7+r.jitter*13)%5 - 2))
			z += numeric.FixedFromInt(int64((su.ID*11+r.jitter*17)%5 - 2))
		}
		if !r.place(def, &x, &z, nil) {
			r.info.Dropped++
			r.info.Notes = append(r.info.Notes, fmt.Sprintf("unit %d %s: no free position near %.0f,%.0f", su.ID, su.Name, su.X, su.Z))
			return
		}
		h, err := sess.Units.Create(def, owner, x, sess.World.HeightAt(x, z), z)
		if err != nil {
			r.info.Dropped++
			r.info.Notes = append(r.info.Notes, fmt.Sprintf("unit %d %s: %v", su.ID, su.Name, err))
			return
		}
		st.u = sess.Units.Unit(h)
		sess.Movement.EnsureUnit(st.u)
		if su.HX != 0 || su.HZ != 0 {
			// The mover's records may already exist, so the heading is
			// written to each of them.
			hd := movement.HeadingFromDelta(int64(su.HX*4096), int64(su.HZ*4096))
			st.u.Move.Heading = hd
			if int(h) < len(sess.Movement.Steers) && sess.Movement.Steers[h] != nil {
				sess.Movement.Steers[h].Heading, sess.Movement.Steers[h].PendingHeading = hd, hd
			}
			if int(h) < len(sess.Movement.Collisions) && sess.Movement.Collisions[h] != nil {
				sess.Movement.Collisions[h].Heading = hd
			}
		}
		sess.BindStagedOrderQueue(st.u)
	}
	holdFire(st.u)
	st.h = uint16(st.u.Handle)
	if _, fx, fz, ok := sess.Movement.CommittedFootprint(st.u.Handle); ok {
		st.fx, st.fz = fx, fz
	}
	tick := int64(su.Spawn)
	st.log = &MovesUnit{NetID: su.ID, Unit: strings.ToUpper(su.Name), Tick: tick, FinTick: &tick,
		X: int(st.u.X.Raw() >> 16), Z: int(st.u.Z.Raw() >> 16)}
	log.Players[su.Player].Units = append(log.Players[su.Player].Units, st.log)
	r.byID[su.ID] = st
	r.all = append(r.all, st)
}

// place finds a free position for def at or near *x, *z and reports
// whether one exists; it counts a move.
func (r *replay) place(def *content.UnitDef, x, z *numeric.Fixed, self *units.Unit) bool {
	if r.free(def, *x, *z, self) {
		return true
	}
	for ring := 1; ring <= 6; ring++ {
		best := -1 // squared distance in steps, none yet
		var bx, bz numeric.Fixed
		for dz := -ring; dz <= ring; dz++ {
			for dx := -ring; dx <= ring; dx++ {
				if max(abs(dx), abs(dz)) != ring {
					continue
				}
				cx, cz := *x+numeric.FixedFromInt(int64(dx*8)), *z+numeric.FixedFromInt(int64(dz*8))
				if !r.free(def, cx, cz, self) {
					continue
				}
				if d := dx*dx + dz*dz; best < 0 || d < best {
					best, bx, bz = d, cx, cz
				}
			}
		}
		if best >= 0 {
			*x, *z = bx, bz
			r.info.Nudged++
			return true
		}
	}
	return false
}

// kill removes a unit the recording shows dying, by the ordinary death
// path so it leaves what its definition leaves.
func (r *replay) kill(su *SnipUnit, now int32) {
	st := r.byID[su.ID]
	if st == nil || st.dead || st.u == nil || !st.u.Alive {
		return
	}
	st.u.Health = -1
	r.sess.Units.DestroyBy(st.u.Handle, units.DeathKilled, 0)
	st.dead = true
	t := int64(now)
	st.log.DiedTick = &t
}

// group issues a recorded group move through the command boundary's own
// arithmetic. The clicked point is not in a recording; the displacement
// every formation member received is, and the point that gives the staged
// members that displacement is their centre plus it.
func (r *replay) group(g *SnipGroup) {
	idx := -1
	for i := range r.s.Groups {
		if &r.s.Groups[i] == g {
			idx = i
		}
	}
	var handles []pool.Handle
	var sx, sz, n int32
	for _, id := range g.Units {
		st := r.byID[id]
		if st == nil || st.dead || !st.mobile || st.u == nil || !st.u.Alive {
			continue
		}
		handles = append(handles, st.u.Handle)
		sx += int32(st.u.X) >> 16
		sz += int32(st.u.Z) >> 16
		n++
	}
	if n < 2 || idx < 0 {
		return
	}
	cx, cz := numeric.Fixed(int64(sx/n)<<16), numeric.Fixed(int64(sz/n)<<16)
	r.sess.StageGroupMove(r.slotOf[g.Player], handles, cx+fixed(g.DX), cz+fixed(g.DZ), g.Count)
	r.grouped[idx] = true
}

// order gives a staged unit a move to the recorded goal.
func (r *replay) order(o *SnipOrder) {
	st := r.byID[o.Unit]
	if st == nil || st.dead || !st.mobile || st.u == nil || !st.u.Alive {
		return
	}
	sess := r.sess
	q := orders.QueueForUnit(st.u)
	if q == nil {
		return
	}
	q.PurgeUnprotected()
	q.DropLeadingAutoOps()
	id := orders.Lookup("Move_Ground")
	gx, gz := fixed(o.GoalX), fixed(o.GoalZ)
	q.Push(id, orders.NewNodeForOrder(id, 0, gx, sess.World.HeightAt(gx, gz), gz, sess.Clock.GlobalTick+1, st.u.Handle, false))
}

// observe appends what changed in each staged unit's follower state, the
// way a unit-sync record shows it: the blocked bit and the first three
// retained route points of an active route [fmt tad §7].
func (r *replay) observe(now int32, opt ReplayOptions, res *ReplayResult) {
	mv := r.sess.Movement
	var frame *Frame
	if opt.Frames > 0 && int(now-r.s.Start)%opt.Frames == 0 {
		res.Frames = append(res.Frames, Frame{Tick: now})
		frame = &res.Frames[len(res.Frames)-1]
	}
	type rect struct{ x0, z0, x1, z1 int32 }
	rects := make([]rect, 0, len(r.all))
	resting := make([]bool, 0, len(r.all))
	for _, st := range r.all {
		if st.dead || st.u == nil {
			continue
		}
		if !st.u.Alive || uint16(st.u.Handle) != st.h {
			st.dead = true
			if st.log.DiedTick == nil {
				t := int64(now)
				st.log.DiedTick = &t
			}
			continue
		}
		if !st.mobile {
			continue
		}
		h := st.u.Handle
		var b bool
		var n int8
		var p [3][2]int32
		if int(h) < len(mv.Collisions) && mv.Collisions[h] != nil {
			b = mv.Collisions[h].Blocked
		}
		if int(h) < len(mv.Routes) && mv.Routes[h] != nil && mv.Routes[h].Active {
			rt := mv.Routes[h]
			n = int8(min(int(rt.Count), 3))
			for i := 0; i < int(n); i++ {
				p[i] = [2]int32{rt.Points[i].X, rt.Points[i].Z}
			}
		}
		if !st.hasLast || b != st.lastB || n != st.lastN || p != st.lastP {
			row := []int32{now, 0, int32(n)}
			if b {
				row[1] = 1
			}
			for i := 0; i < int(n); i++ {
				row = append(row, p[i][0], p[i][1])
			}
			st.log.Path = append(st.log.Path, row)
			st.hasLast, st.lastB, st.lastN, st.lastP = true, b, n, p
		}
		st.now = now
		st.motion(n > 0)
		st.goal(now)
		if int(now-r.s.Start)%30 == 0 {
			st.log.Pose = append(st.log.Pose, [5]int32{now, int32(st.u.X.Raw()), int32(st.u.Z.Raw()), int32(st.u.Move.Heading), int32(st.u.Move.Speed.Raw())})
			st.log.Motion = append(st.log.Motion, [6]int32{now, int32(st.dist >> 8), int32(st.turn >> 8), st.stops, int32(st.away >> 8), st.reversals})
		}
		if frame != nil {
			bl, mvg := int32(0), int32(0)
			if b {
				bl = 1
			}
			if n > 0 {
				mvg = 1
			}
			frame.Units = append(frame.Units, [6]int32{int32(st.su.ID), int32(st.u.X.Raw() >> 16), int32(st.u.Z.Raw() >> 16), int32(st.u.Move.Heading), bl, mvg})
		}
		if a, fx, fz, ok := mv.CommittedFootprint(h); ok {
			rects = append(rects, rect{a.X, a.Z, a.X + int32(fx), a.Z + int32(fz)})
			resting = append(resting, n == 0)
			if n > 0 {
				r.info.MoverTicks++
			}
		}
	}
	pairs := 0
	hit := make([]bool, len(rects))
	for i := range rects {
		for j := i + 1; j < len(rects); j++ {
			if rects[i].x0 < rects[j].x1 && rects[j].x0 < rects[i].x1 && rects[i].z0 < rects[j].z1 && rects[j].z0 < rects[i].z1 {
				pairs++
				hit[i], hit[j] = true, true
			}
		}
	}
	r.info.OverlapLast = 0
	for i, v := range hit {
		if v {
			r.info.OverlapTicks++
			r.info.OverlapLast++
			if resting[i] {
				r.info.OverlapResting++
			}
		}
	}
	r.info.OverlapPairs = max(r.info.OverlapPairs, pairs)
	ls := mv.LabStats()
	r.info.Steers, r.info.Probes, r.info.Anchors = ls.Visits, ls.Probes, ls.Anchors
	if ls.Searches > 0 {
		r.info.Searches, r.info.Pops = int64(ls.Searches), int64(ls.Pops)
	}
	if ls.Visits > 0 {
		r.info.SteerStarts, r.info.SteerTicks = ls.SteerStarts[:], ls.SteerTicks[:]
	}
	r.info.Refused, r.info.RefusedNear = ls.Refused[:], ls.RefusedNear[:]
}
