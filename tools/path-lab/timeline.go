package main

import (
	"math"
	"sort"
)

// A ground movement entry is the route follower's visible state: the blocked
// bit and the first three retained route points [fmt tad §7 "Ground movement
// entry"]. The follower keeps points[0] as the point just left and points[1]
// as the point being steered to, and consumes points[1] when the unit's
// integer position is within five world units of it [04 R-MOV-01 §3]. The
// goal installer writes a two-point line from the unit's own integer position
// to the goal query's position [04 R-PATH-01 §8]; a published search result
// instead holds cell anchors, 16·cell + 8·footprint per axis
// [04 R-PATH-01 §7]. Those three facts are what this file reads a unit's
// movement back from.

type entry struct {
	T int32
	B bool
	N int8
	P [3][2]int32
}

type entryKind uint8

const (
	entIdle    entryKind = iota // no active route
	entInstall                  // goal installation: (unit position, goal)
	entRoute                    // a published route's prefix
)

type fixKind uint8

const (
	fixExact    fixKind = iota // install start or scheduled status
	fixWaypoint                // a consumed waypoint: within 5 world units
	fixCell                    // a published route's first point: the unit's cell
	fixBirth                   // creation position
	fixEnd                     // a route's last point, reached when it emptied
)

type fix struct {
	T    int32
	X, Z float64
	Kind fixKind
}

// segment is a maximal interval in which the unit either holds an active
// route (Moving) or does not. An idle unit does not move, so an idle
// segment has one position.
type segment struct {
	T0, T1 int32 // [T0, T1)
	Moving bool
	X, Z   float64 // idle position; valid when Known
	Known  bool
}

type blockSpan struct {
	T0, T1 int32
	// Carried marks a span whose blocked bit was already set when the route
	// became active: the bit is stale across an idle period, so its start is
	// not a fresh refusal.
	Carried bool
	// End is how the span ended: 0 the bit cleared, 1 the route went
	// inactive, 2 the unit died or the log ended.
	End   uint8
	Entry int // index of the entry that started it
}

type episode struct {
	T0       int32 // first goal installation (or first route, when none was seen)
	TRoute   int32 // first route publication after T0, -1 when none
	TEnd     int32
	X0, Z0   float64
	GoalX    float64 // last installed goal
	GoalZ    float64
	Goal0X   float64 // first installed goal
	Goal0Z   float64
	HasGoal  bool
	EndX     float64
	EndZ     float64
	EndKnown bool
	// End: "arrived", "short" (idle away from the goal), "superseded",
	// "died", "open".
	End         string
	Reinstalls  int // same goal installed again
	GoalUpdates int // goal moved a little while active (pursuit, refresh)
	Routes      int // route publications
	Waypoints   int // waypoints consumed
	Blocks      int
	BlockTicks  int32
	MaxBlock    int32
	WaitTicks   int32 // active, no route: waiting for a search result
	Length      float64
	First       bool // the unit's first episode (factory egress or first order)
	Cohort      int  // index into the recording's cohorts, -1 when none
	firstEntry  int
	lastEntry   int
	idleSince   int32
	idle        bool
}

type unitTL struct {
	Rec    *recording
	Player int // index into the recording's players
	Net    int
	Name   string
	Def    *unitDef
	Born   int32
	Fin    int32 // -1 when never finished
	Died   int32 // -1 when alive at the end
	BX, BZ float64
	Ground bool // a mobile ground (or surface) unit with movement entries
	// Builder is the net ID of the unit that started this one, zero when
	// the recording names none.
	Builder int

	Entries  []entry
	Kinds    []entryKind
	Fixes    []fix
	Segs     []segment
	Blocks   []blockSpan
	Episodes []episode
	// unblocked[i] is the count of moving, unblocked ticks before tick
	// ubT[i]; pos() interpolates in that clock so a unit does not drift
	// while it is held.
	ubT []int32
	ubV []int32
	// Hurt and Fire are combat buckets [tick, amount].
	Hurt [][2]int32
	Fire [][2]int32
	// Motion is a replay's running totals, sampled with its poses: tick,
	// distance, turning in 256ths of a circle, stops, distance moved away
	// from the goal. A recording has none.
	Motion [][6]int32
	// Goals is a replay's own goal for the unit, a row each time the goal of
	// its head order changed: tick, x, z.
	Goals [][3]int32

	alignX, alignZ int32
	hasAlign       bool
}

func (u *unitTL) aligned(p [2]int32) bool {
	return ((p[0]%16)+16)%16 == u.alignX && ((p[1]%16)+16)%16 == u.alignZ
}

func dist(ax, az, bx, bz float64) float64 { return math.Hypot(ax-bx, az-bz) }

const (
	sameGoalTol    = 2.0
	goalUpdateTol  = 200.0
	idleEpisodeGap = 90
	arriveRadius   = 48.0
)

// build reads the unit's entries into fixes, segments, blocked spans and
// episodes. end is the recording's last tick.
func (u *unitTL) build(end int32) {
	if len(u.Entries) == 0 {
		return
	}
	// Lattice alignment: every point of a three-point entry is a cell
	// anchor, so their residues modulo 16 name the class footprint's bias.
	cnt := map[[2]int32]int{}
	for i := range u.Entries {
		e := &u.Entries[i]
		if e.N == 3 {
			for k := 0; k < 3; k++ {
				cnt[[2]int32{((e.P[k][0] % 16) + 16) % 16, ((e.P[k][1] % 16) + 16) % 16}]++
			}
		}
	}
	best := 0
	for k, n := range cnt {
		if n > best || (n == best && (k[0] < u.alignX || (k[0] == u.alignX && k[1] < u.alignZ))) {
			best, u.alignX, u.alignZ, u.hasAlign = n, k[0], k[1], true
		}
	}
	if !u.hasAlign && u.Def != nil {
		fx, fz := u.Def.ClassFX, u.Def.ClassFZ
		if fx == 0 {
			fx, fz = u.Def.FX, u.Def.FZ
		}
		if fx > 0 {
			u.alignX, u.alignZ, u.hasAlign = int32(8*fx%16), int32(8*fz%16), true
		}
	}
	u.Kinds = make([]entryKind, len(u.Entries))
	for i := range u.Entries {
		e := &u.Entries[i]
		switch {
		case e.N == 0:
			u.Kinds[i] = entIdle
		case e.N == 2 && u.hasAlign && !(u.aligned(e.P[0]) && u.aligned(e.P[1])):
			u.Kinds[i] = entInstall
		default:
			u.Kinds[i] = entRoute
		}
	}

	last := end
	if u.Died >= 0 && u.Died < last {
		last = u.Died
	}
	u.segments(last)
	u.blocks(last)
	u.clock(last)
	u.episodes(last)
}

// segments walks the entries into moving and idle intervals and collects
// the position fixes.
func (u *unitTL) segments(last int32) {
	start := u.Born
	if u.Fin >= 0 {
		start = u.Fin
	}
	if len(u.Entries) > 0 && u.Entries[0].T < start {
		start = u.Entries[0].T
	}
	u.Fixes = append(u.Fixes, fix{T: start, X: u.BX, Z: u.BZ, Kind: fixBirth})
	cur := segment{T0: start, Moving: false, X: u.BX, Z: u.BZ, Known: true}
	var prev *entry
	prevKind := entIdle
	for i := range u.Entries {
		e := &u.Entries[i]
		k := u.Kinds[i]
		if e.T > last {
			break
		}
		moving := k != entIdle
		if moving != cur.Moving {
			cur.T1 = e.T
			if cur.Moving && prev != nil && prevKind != entIdle {
				// The route emptied. With two points left the unit was
				// steering to the last one and consumed it; with three it
				// was stopped on the way and its position is not known here.
				if prev.N == 2 && prevKind == entRoute {
					u.Fixes = append(u.Fixes, fix{T: e.T, X: float64(prev.P[1][0]), Z: float64(prev.P[1][1]), Kind: fixEnd})
				}
			}
			if cur.T1 > cur.T0 {
				u.Segs = append(u.Segs, cur)
			}
			cur = segment{T0: e.T, Moving: moving}
		}
		switch k {
		case entInstall:
			u.Fixes = append(u.Fixes, fix{T: e.T, X: float64(e.P[0][0]), Z: float64(e.P[0][1]), Kind: fixExact})
		case entRoute:
			switch {
			case prev != nil && prevKind != entIdle && prev.N >= 2 && e.P[0] == prev.P[1] && e.P[0] != prev.P[0]:
				u.Fixes = append(u.Fixes, fix{T: e.T, X: float64(e.P[0][0]), Z: float64(e.P[0][1]), Kind: fixWaypoint})
			case prev != nil && prevKind == entRoute && e.N == prev.N && e.P == prev.P:
				// Only the blocked bit changed.
			case prev != nil && prevKind == entInstall && e.T-prev.T <= 2:
				// The search result for the installation just seen: its
				// first point is the cell of the exact position already
				// recorded, so it adds nothing.
			default:
				u.Fixes = append(u.Fixes, fix{T: e.T, X: float64(e.P[0][0]), Z: float64(e.P[0][1]), Kind: fixCell})
			}
		}
		prev, prevKind = e, k
	}
	cur.T1 = last + 1
	if cur.T1 > cur.T0 {
		u.Segs = append(u.Segs, cur)
	}
	sort.SliceStable(u.Fixes, func(i, j int) bool { return u.Fixes[i].T < u.Fixes[j].T })
	u.dropStaleCells()

	// An idle unit stands still, so any exact fix inside an idle segment or
	// at the tick the next route starts is that segment's position.
	fi := 0
	for si := range u.Segs {
		s := &u.Segs[si]
		if s.Moving {
			continue
		}
		for fi < len(u.Fixes) && u.Fixes[fi].T < s.T0 {
			fi++
		}
		bestRank := -1
		for j := fi; j < len(u.Fixes) && u.Fixes[j].T <= s.T1; j++ {
			f := u.Fixes[j]
			rank := 0
			switch f.Kind {
			case fixExact, fixBirth:
				rank = 3
			case fixEnd, fixWaypoint:
				rank = 2
			case fixCell:
				rank = 1
			}
			// A fix at T1 belongs to the next route's start; only an exact
			// one (the installer's own position) describes this segment.
			if f.T == s.T1 && f.Kind != fixExact {
				continue
			}
			if rank > bestRank {
				bestRank, s.X, s.Z, s.Known = rank, f.X, f.Z, true
			}
		}
	}
	// Scheduled statuses are exact positions wherever they fall.
}

// dropStaleCells removes the cell fixes the unit cannot have been at. A
// published route's first point is the cell the search started from, which
// is where the unit stood when the request was admitted; by the time a slow
// search publishes, the unit has moved on, and the fix would pull it back.
// A cell fix is kept only when the unit could have come from the fix before
// it and gone on to the fix after it at no more than twice its speed.
func (u *unitTL) dropStaleCells() {
	v := 3.0
	if u.Def != nil && u.Def.VMax > 0 {
		v = u.Def.VMax
	}
	ok := func(a, b fix) bool {
		dt := float64(b.T - a.T)
		if dt < 0 {
			dt = -dt
		}
		return dist(a.X, a.Z, b.X, b.Z) <= 2*v*dt+24
	}
	out := u.Fixes[:0]
	for i, f := range u.Fixes {
		if f.Kind == fixCell {
			if n := len(out); n > 0 && !ok(out[n-1], f) {
				continue
			}
			// The next fix that is not itself a cell fix.
			stale := false
			for j := i + 1; j < len(u.Fixes); j++ {
				if u.Fixes[j].Kind != fixCell {
					stale = !ok(f, u.Fixes[j])
					break
				}
			}
			if stale {
				continue
			}
		}
		out = append(out, f)
	}
	u.Fixes = out
}

// addPoses merges scheduled-status positions into the fixes. It runs before
// build so segments can use them.
func (u *unitTL) addPoses(pose [][5]int32, pos [][6]int32) {
	for _, p := range pose {
		u.Fixes = append(u.Fixes, fix{T: p[0], X: float64(p[1]) / 65536, Z: float64(p[2]) / 65536, Kind: fixExact})
	}
	if len(pose) == 0 {
		for _, p := range pos {
			u.Fixes = append(u.Fixes, fix{T: p[0], X: float64(p[1]), Z: float64(p[2]), Kind: fixExact})
		}
	}
}

func (u *unitTL) blocks(last int32) {
	open := -1
	carried := false
	active := false
	for i := range u.Entries {
		e := &u.Entries[i]
		if e.T > last {
			break
		}
		k := u.Kinds[i]
		nowActive := k != entIdle
		switch {
		case open >= 0 && (!e.B || !nowActive):
			end := uint8(0)
			if !nowActive {
				end = 1
			}
			u.Blocks = append(u.Blocks, blockSpan{T0: u.Entries[open].T, T1: e.T, Carried: carried, End: end, Entry: open})
			open = -1
		case open < 0 && e.B && nowActive:
			open = i
			// The bit was set before this route was active: not a fresh
			// refusal at this tick.
			carried = !active || (i > 0 && u.Entries[i-1].B)
		}
		active = nowActive
	}
	if open >= 0 {
		u.Blocks = append(u.Blocks, blockSpan{T0: u.Entries[open].T, T1: last, Carried: carried, End: 2, Entry: open})
	}
}

// clock builds the moving-and-unblocked tick counter.
func (u *unitTL) clock(last int32) {
	type ev struct {
		t int32
		d int8 // +1 starts counting, -1 stops
	}
	var evs []ev
	for _, s := range u.Segs {
		if s.Moving {
			evs = append(evs, ev{s.T0, 1}, ev{s.T1, -1})
		}
	}
	for _, b := range u.Blocks {
		evs = append(evs, ev{b.T0, -1}, ev{b.T1, 1})
	}
	sort.SliceStable(evs, func(i, j int) bool { return evs[i].t < evs[j].t })
	level, v := int8(0), int32(0)
	var t0 int32
	if len(evs) > 0 {
		t0 = evs[0].t
	}
	u.ubT = append(u.ubT, t0)
	u.ubV = append(u.ubV, 0)
	for _, e := range evs {
		if e.t > t0 {
			if level > 0 {
				v += e.t - t0
			}
			t0 = e.t
			u.ubT = append(u.ubT, t0)
			u.ubV = append(u.ubV, v)
		}
		level += e.d
	}
}

// unblocked is the number of moving, unblocked ticks before tick t.
func (u *unitTL) unblocked(t int32) float64 {
	n := len(u.ubT)
	if n == 0 {
		return 0
	}
	i := sort.Search(n, func(i int) bool { return u.ubT[i] > t }) - 1
	if i < 0 {
		return 0
	}
	if i >= n-1 {
		return float64(u.ubV[n-1])
	}
	span := float64(u.ubT[i+1] - u.ubT[i])
	if span <= 0 {
		return float64(u.ubV[i])
	}
	return float64(u.ubV[i]) + float64(u.ubV[i+1]-u.ubV[i])*float64(t-u.ubT[i])/span
}

func (u *unitTL) segAt(t int32) *segment {
	n := len(u.Segs)
	i := sort.Search(n, func(i int) bool { return u.Segs[i].T1 > t })
	if i >= n {
		if n == 0 {
			return nil
		}
		return &u.Segs[n-1]
	}
	return &u.Segs[i]
}

// pos estimates the unit's position at tick t and reports whether it holds
// an active route then. An idle unit is where its segment says; a moving
// unit is between the fixes around t, at the share of their unblocked
// moving time that has passed.
func (u *unitTL) pos(t int32) (x, z float64, moving, ok bool) {
	s := u.segAt(t)
	if s == nil {
		return u.BX, u.BZ, false, true
	}
	if !s.Moving {
		if s.Known {
			return s.X, s.Z, false, true
		}
	}
	n := len(u.Fixes)
	j := sort.Search(n, func(i int) bool { return u.Fixes[i].T > t })
	var a, b *fix
	if j > 0 {
		a = &u.Fixes[j-1]
	}
	if j < n {
		b = &u.Fixes[j]
	}
	switch {
	case a == nil && b == nil:
		return u.BX, u.BZ, s.Moving, false
	case a == nil:
		return b.X, b.Z, s.Moving, true
	case b == nil:
		return a.X, a.Z, s.Moving, true
	}
	if !s.Moving {
		// Idle with no fix of its own: the unit stopped between a and b.
		// The fix after it, if the unit did not move before it, is where
		// it stood; otherwise the last fix before.
		return a.X, a.Z, false, true
	}
	ua, ub, ut := u.unblocked(a.T), u.unblocked(b.T), u.unblocked(t)
	f := 0.0
	if ub > ua {
		f = (ut - ua) / (ub - ua)
	} else if b.T > a.T {
		f = float64(t-a.T) / float64(b.T-a.T)
	}
	if f < 0 {
		f = 0
	} else if f > 1 {
		f = 1
	}
	return a.X + (b.X-a.X)*f, a.Z + (b.Z-a.Z)*f, true, true
}

// heading is the unit's direction of travel at tick t as a unit vector: to
// the point it is steering at, or zero when it holds no route.
func (u *unitTL) heading(t int32) (hx, hz float64, ok bool) {
	n := len(u.Entries)
	i := sort.Search(n, func(i int) bool { return u.Entries[i].T > t }) - 1
	if i < 0 || u.Kinds[i] == entIdle {
		return 0, 0, false
	}
	e := &u.Entries[i]
	x, z, _, _ := u.pos(t)
	dx, dz := float64(e.P[1][0])-x, float64(e.P[1][1])-z
	d := math.Hypot(dx, dz)
	if d < 1 {
		dx, dz = float64(e.P[1][0]-e.P[0][0]), float64(e.P[1][1]-e.P[0][1])
		d = math.Hypot(dx, dz)
		if d < 1 {
			return 0, 0, false
		}
	}
	return dx / d, dz / d, true
}

// blockedAt reports whether tick t lies in one of the unit's blocked spans.
func (u *unitTL) blockedAt(t int32) bool {
	n := len(u.Blocks)
	i := sort.Search(n, func(i int) bool { return u.Blocks[i].T1 > t })
	return i < n && u.Blocks[i].T0 <= t
}

// blockAt returns the blocked span tick t lies in, nil for none.
func (u *unitTL) blockAt(t int32) *blockSpan {
	n := len(u.Blocks)
	i := sort.Search(n, func(i int) bool { return u.Blocks[i].T1 > t })
	if i < n && u.Blocks[i].T0 <= t {
		return &u.Blocks[i]
	}
	return nil
}

func (u *unitTL) episodes(last int32) {
	var ep *episode
	closeEp := func(t int32, why string) {
		if ep == nil {
			return
		}
		if ep.idle {
			ep.TEnd = ep.idleSince
		} else {
			ep.TEnd = t
			ep.End = why
		}
		u.Episodes = append(u.Episodes, *ep)
		ep = nil
	}
	start := func(i int, e *entry, goal bool) {
		ep = &episode{T0: e.T, TRoute: -1, Cohort: -1, firstEntry: i, lastEntry: i, First: len(u.Episodes) == 0}
		if goal {
			ep.X0, ep.Z0 = float64(e.P[0][0]), float64(e.P[0][1])
			ep.GoalX, ep.GoalZ = float64(e.P[1][0]), float64(e.P[1][1])
			ep.Goal0X, ep.Goal0Z, ep.HasGoal = ep.GoalX, ep.GoalZ, true
		} else {
			ep.X0, ep.Z0 = float64(e.P[0][0]), float64(e.P[0][1])
			ep.TRoute = e.T
		}
	}
	var prev *entry
	prevKind := entIdle
	for i := range u.Entries {
		e := &u.Entries[i]
		if e.T > last {
			break
		}
		switch u.Kinds[i] {
		case entIdle:
			if ep != nil && !ep.idle {
				ep.idle, ep.idleSince = true, e.T
			}
		case entInstall:
			gx, gz := float64(e.P[1][0]), float64(e.P[1][1])
			switch {
			case ep == nil:
				start(i, e, true)
			case ep.idle && e.T-ep.idleSince > idleEpisodeGap:
				closeEp(e.T, "")
				start(i, e, true)
			case !ep.HasGoal:
				ep.GoalX, ep.GoalZ, ep.Goal0X, ep.Goal0Z, ep.HasGoal = gx, gz, gx, gz, true
				ep.idle = false
			case dist(gx, gz, ep.GoalX, ep.GoalZ) <= sameGoalTol:
				ep.Reinstalls++
				ep.idle = false
			case !ep.idle && dist(gx, gz, ep.GoalX, ep.GoalZ) <= goalUpdateTol:
				ep.GoalUpdates++
				ep.GoalX, ep.GoalZ = gx, gz
			default:
				closeEp(e.T, "superseded")
				start(i, e, true)
			}
		case entRoute:
			if ep == nil || (ep.idle && e.T-ep.idleSince > idleEpisodeGap) {
				closeEp(e.T, "")
				start(i, e, false)
			}
			ep.idle = false
			switch {
			case prev != nil && prevKind != entIdle && prev.N >= 2 && e.P[0] == prev.P[1] && e.P[0] != prev.P[0]:
				ep.Waypoints++
			case prev != nil && prevKind == entRoute && e.N == prev.N && e.P == prev.P:
			default:
				ep.Routes++
				if ep.TRoute < 0 {
					ep.TRoute = e.T
				}
			}
		}
		if ep != nil {
			ep.lastEntry = i
		}
		prev, prevKind = e, u.Kinds[i]
	}
	if ep != nil {
		why := "open"
		if u.Died >= 0 && u.Died <= last {
			why = "died"
		}
		closeEp(last, why)
	}

	// Measure each episode from the reconstructed timeline.
	bi := 0
	for k := range u.Episodes {
		ep := &u.Episodes[k]
		ex, ez, _, ok := u.pos(ep.TEnd)
		// The position just after the end, when the unit went idle there.
		if s := u.segAt(ep.TEnd); s != nil && !s.Moving && s.Known {
			ex, ez, ok = s.X, s.Z, true
		}
		ep.EndX, ep.EndZ, ep.EndKnown = ex, ez, ok
		if ep.End == "" {
			ep.End = "short"
			if ep.HasGoal && ok && dist(ex, ez, ep.GoalX, ep.GoalZ) <= arriveRadius {
				ep.End = "arrived"
			}
			if !ep.HasGoal {
				ep.End = "ended"
			}
		}
		// Length over the position knots inside the episode.
		px, pz := ep.X0, ep.Z0
		j := sort.Search(len(u.Fixes), func(i int) bool { return u.Fixes[i].T > ep.T0 })
		for ; j < len(u.Fixes) && u.Fixes[j].T <= ep.TEnd; j++ {
			f := u.Fixes[j]
			ep.Length += dist(px, pz, f.X, f.Z)
			px, pz = f.X, f.Z
		}
		if ok {
			ep.Length += dist(px, pz, ex, ez)
		}
		for bi < len(u.Blocks) && u.Blocks[bi].T1 <= ep.T0 {
			bi++
		}
		for j := bi; j < len(u.Blocks) && u.Blocks[j].T0 < ep.TEnd; j++ {
			b := u.Blocks[j]
			t0, t1 := max(b.T0, ep.T0), min(b.T1, ep.TEnd)
			if t1 > t0 {
				ep.Blocks++
				ep.BlockTicks += t1 - t0
				ep.MaxBlock = max(ep.MaxBlock, t1-t0)
			}
		}
		// Waiting: active episode ticks inside idle segments (no route yet).
		for _, s := range u.Segs {
			if s.Moving || s.T1 <= ep.T0 || s.T0 >= ep.TEnd {
				continue
			}
			ep.WaitTicks += min(s.T1, ep.TEnd) - max(s.T0, ep.T0)
		}
	}
}
