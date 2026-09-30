package main

import (
	"fmt"
	"math"
	"sort"
	"strings"
)

// blockerRec describes the unit most likely to have refused a mover's step.
type blockerRec struct {
	Unit   int    `json:"unit"`
	Name   string `json:"name"`
	Player int    `json:"player"`
	// Rel is "own" for the mover's player and "other" for any other.
	Rel string `json:"rel"`
	// State is moving, held (moving but itself blocked), parked, structure
	// or frame (a structure still under construction).
	State       string  `json:"state"`
	Gap         float64 `json:"gap"`
	Bearing     float64 `json:"bearing"`      // degrees off the mover's travel direction
	HeadingDiff float64 `json:"heading_diff"` // degrees between the two travel directions, -1 unknown
	SameCohort  bool    `json:"same_cohort,omitempty"`
	Slower      bool    `json:"slower,omitempty"`
}

// blockRec is one blocked span with its context.
type blockRec struct {
	Rec       string      `json:"rec"`
	Unit      int         `json:"unit"`
	Name      string      `json:"name"`
	Kind      string      `json:"kind,omitempty"`
	Player    int         `json:"player"`
	T0        int32       `json:"t0"`
	Dur       int32       `json:"dur"`
	End       uint8       `json:"end"`
	Carried   bool        `json:"carried,omitempty"`
	X         float64     `json:"x"`
	Z         float64     `json:"z"`
	HX        float64     `json:"hx"`
	HZ        float64     `json:"hz"`
	Episode   int         `json:"episode"`
	GoalDist  float64     `json:"goal_dist"`
	StartDist float64     `json:"start_dist"`
	Egress    bool        `json:"egress,omitempty"`
	Cohort    int         `json:"cohort"`
	Blocker   *blockerRec `json:"blocker,omitempty"`
	NearMob   int         `json:"near_mobile"`
	NearStr   int         `json:"near_struct"`
	Combat    bool        `json:"combat,omitempty"`
	Class     string      `json:"class"`
	// Clearance is the static free half-width around the unit in cells,
	// zero when the map's grid is not exported.
	Clearance int `json:"clearance,omitempty"`
	// Root names what held the front of the queue this unit stood in: the
	// class of the first block, followed from blocker to blocker, whose
	// blocker is not a held friend going the same way. It is "free-leader"
	// when the unit ahead was moving freely, "mutual" when the chain comes
	// back to a unit already in it, and the block's own class when the
	// block is not a same-way one. Chain is how many units were followed.
	Root  string `json:"root,omitempty"`
	Chain int    `json:"chain,omitempty"`
}

type episodeRec struct {
	Rec    string  `json:"rec"`
	Unit   int     `json:"unit"`
	Name   string  `json:"name"`
	Kind   string  `json:"kind,omitempty"`
	Player int     `json:"player"`
	Index  int     `json:"index"`
	VMax   float64 `json:"vmax"`
	episodeOut
	Ideal  float64 `json:"ideal_ticks"`
	Ratio  float64 `json:"ratio"`
	Detour float64 `json:"detour"`
	Combat int32   `json:"combat"`
	// Purpose is build, reclaim, combat, follow, egress, move or unknown.
	Purpose string `json:"purpose"`
	// Settle is the ticks from first coming within 160 world units of the
	// goal to the episode's end; Shortest the static shortest path's
	// length from start to end. Zero when not measured.
	Settle   int32   `json:"settle,omitempty"`
	Shortest float64 `json:"shortest,omitempty"`
}

type episodeOut struct {
	T0          int32   `json:"t0"`
	TRoute      int32   `json:"t_route"`
	TEnd        int32   `json:"t_end"`
	X0          float64 `json:"x0"`
	Z0          float64 `json:"z0"`
	GoalX       float64 `json:"goal_x"`
	GoalZ       float64 `json:"goal_z"`
	HasGoal     bool    `json:"has_goal"`
	EndX        float64 `json:"end_x"`
	EndZ        float64 `json:"end_z"`
	End         string  `json:"end"`
	Reinstalls  int     `json:"reinstalls"`
	GoalUpdates int     `json:"goal_updates"`
	Routes      int     `json:"routes"`
	Waypoints   int     `json:"waypoints"`
	Blocks      int     `json:"blocks"`
	BlockTicks  int32   `json:"block_ticks"`
	MaxBlock    int32   `json:"max_block"`
	WaitTicks   int32   `json:"wait_ticks"`
	Length      float64 `json:"length"`
	First       bool    `json:"first,omitempty"`
	Cohort      int     `json:"cohort"`
}

func (e *episode) out() episodeOut {
	return episodeOut{T0: e.T0, TRoute: e.TRoute, TEnd: e.TEnd, X0: e.X0, Z0: e.Z0, GoalX: e.GoalX, GoalZ: e.GoalZ,
		HasGoal: e.HasGoal, EndX: e.EndX, EndZ: e.EndZ, End: e.End, Reinstalls: e.Reinstalls, GoalUpdates: e.GoalUpdates,
		Routes: e.Routes, Waypoints: e.Waypoints, Blocks: e.Blocks, BlockTicks: e.BlockTicks, MaxBlock: e.MaxBlock,
		WaitTicks: e.WaitTicks, Length: e.Length, First: e.First, Cohort: e.Cohort}
}

type cohortMember struct {
	Unit       int     `json:"unit"`
	Name       string  `json:"name"`
	VMax       float64 `json:"vmax"`
	Episode    int     `json:"episode"`
	X0         float64 `json:"x0"`
	Z0         float64 `json:"z0"`
	GoalX      float64 `json:"goal_x"`
	GoalZ      float64 `json:"goal_z"`
	End        string  `json:"end"`
	TEnd       int32   `json:"t_end"`
	Length     float64 `json:"length"`
	Blocks     int     `json:"blocks"`
	BlockTicks int32   `json:"block_ticks"`
	SelfTicks  int32   `json:"self_block_ticks"`
	WaitTicks  int32   `json:"wait_ticks"`
	Ideal      float64 `json:"ideal_ticks"`
}

type cohortRec struct {
	Rec         string         `json:"rec"`
	Index       int            `json:"index"`
	Player      int            `json:"player"`
	T0          int32          `json:"t0"`
	N           int            `json:"n"`
	Members     []cohortMember `json:"members"`
	StartRadius float64        `json:"start_radius"`
	GoalRadius  float64        `json:"goal_radius"`
	SharedGoal  int            `json:"shared_goal"` // members whose goal another member also holds
	Distance    float64        `json:"distance"`    // centroid to centroid
	Mixed       bool           `json:"mixed_speed"`
	Kinds       string         `json:"kinds"`
	Arrived     int            `json:"arrived"`
	Short       int            `json:"short"`
	Superseded  int            `json:"superseded"`
	Died        int            `json:"died"`
	BlockTicks  int32          `json:"block_ticks"`
	SelfTicks   int32          `json:"self_block_ticks"`
	WaitTicks   int32          `json:"wait_ticks"`
	FirstArr    int32          `json:"first_arrival"`
	LastArr     int32          `json:"last_arrival"`
	MedianArr   int32          `json:"median_arrival"`
	IdealMedian float64        `json:"ideal_median"`
	Combat      int32          `json:"combat"`
}

func (u *unitTL) episodeAt(t int32) int {
	n := len(u.Episodes)
	i := sort.Search(n, func(i int) bool { return u.Episodes[i].TEnd > t })
	if i < n && u.Episodes[i].T0 <= t {
		return i
	}
	return -1
}

func (u *unitTL) kind() string {
	if u.Def != nil && u.Def.Kind != "" {
		return u.Def.Kind
	}
	return "?"
}

func (u *unitTL) vmax() float64 {
	if u.Def != nil {
		return u.Def.VMax
	}
	return 0
}

func (u *unitTL) combat(t0, t1 int32) int32 {
	return sumBuckets(u.Hurt, t0, t1) + sumBuckets(u.Fire, t0, t1)
}

const (
	blockerReach = 28.0 // footprint gap that still counts as touching, world units
	crowdRadius  = 96.0
	nearGoal     = 96.0
)

func deg(x float64) float64 { return x * 180 / math.Pi }

// cohortWindow is how many ticks apart the goal installations of one group
// order may be observed. The order is applied on one tick, but a unit-sync
// record carries a bounded number of movement entries, so a large group's
// entries reach the recording over several consecutive ticks
// [08 "Unit-sync ownership and body structure"].
const (
	cohortWindow  = 8
	cohortDispTol = 12.0
	cohortGoalTol = 3.0
)

// cohorts groups the episodes one group order started. A group move keeps
// each unit's offset from the group's centre, so every member's goal lies
// at the same displacement from its start; a unit beyond the formation
// cutoff, and every member of a group given one target, takes the shared
// point instead [04 R-STANCE-01 §5]. Episodes of one player observed within
// cohortWindow ticks that share a displacement or a goal are one cohort.
func (r *recording) cohorts() []cohortRec {
	type ref struct {
		u      *unitTL
		ei     int
		t      int32
		dx, dz float64
		gx, gz float64
	}
	byPlayer := map[int][]ref{}
	for _, u := range r.Units {
		if !u.Ground {
			continue
		}
		for ei := range u.Episodes {
			e := &u.Episodes[ei]
			if !e.HasGoal || u.Kinds[e.firstEntry] != entInstall {
				continue
			}
			byPlayer[u.Player] = append(byPlayer[u.Player], ref{u, ei, e.T0, e.Goal0X - e.X0, e.Goal0Z - e.Z0, e.Goal0X, e.Goal0Z})
		}
	}
	type cluster struct {
		members []ref
		last    int32
	}
	var out []cohortRec
	players := make([]int, 0, len(byPlayer))
	for p := range byPlayer {
		players = append(players, p)
	}
	sort.Ints(players)
	for _, p := range players {
		refs := byPlayer[p]
		sort.SliceStable(refs, func(i, j int) bool {
			if refs[i].t != refs[j].t {
				return refs[i].t < refs[j].t
			}
			return refs[i].u.Net < refs[j].u.Net
		})
		var open []*cluster
		var done []*cluster
		for _, e := range refs {
			// Retire clusters the window has passed.
			k := 0
			for _, c := range open {
				if e.t-c.last > cohortWindow {
					done = append(done, c)
				} else {
					open[k] = c
					k++
				}
			}
			open = open[:k]
			var home *cluster
			for _, c := range open {
				for _, m := range c.members {
					if m.u == e.u {
						continue
					}
					if (math.Abs(m.dx-e.dx) <= cohortDispTol && math.Abs(m.dz-e.dz) <= cohortDispTol) ||
						(math.Abs(m.gx-e.gx) <= cohortGoalTol && math.Abs(m.gz-e.gz) <= cohortGoalTol) {
						home = c
						break
					}
				}
				if home != nil {
					break
				}
			}
			if home == nil {
				home = &cluster{}
				open = append(open, home)
			}
			home.members = append(home.members, e)
			home.last = e.t
		}
		done = append(done, open...)
		sort.SliceStable(done, func(i, j int) bool { return done[i].members[0].t < done[j].members[0].t })
		for _, cl := range done {
			g := cl.members
			if len(g) < 2 {
				continue
			}
			sort.Slice(g, func(i, j int) bool { return g[i].u.Net < g[j].u.Net })
			c := cohortRec{Rec: r.Key, Index: len(out), Player: p, T0: g[0].t, N: len(g)}
			var sx, sz, gx, gz float64
			for _, m := range g {
				e := &m.u.Episodes[m.ei]
				e.Cohort = c.Index
				c.T0 = min(c.T0, m.t)
				sx, sz, gx, gz = sx+e.X0, sz+e.Z0, gx+e.Goal0X, gz+e.Goal0Z
			}
			n := float64(len(g))
			sx, sz, gx, gz = sx/n, sz/n, gx/n, gz/n
			c.Distance = dist(sx, sz, gx, gz)
			vmin, vmax := math.Inf(1), 0.0
			kinds := map[string]int{}
			goals := map[[2]int32]int{}
			for _, m := range g {
				e := &m.u.Episodes[m.ei]
				c.StartRadius = math.Max(c.StartRadius, dist(e.X0, e.Z0, sx, sz))
				c.GoalRadius = math.Max(c.GoalRadius, dist(e.Goal0X, e.Goal0Z, gx, gz))
				if v := m.u.vmax(); v > 0 {
					vmin, vmax = math.Min(vmin, v), math.Max(vmax, v)
				}
				kinds[m.u.kind()]++
				goals[[2]int32{int32(e.Goal0X), int32(e.Goal0Z)}]++
			}
			for _, m := range g {
				e := &m.u.Episodes[m.ei]
				if goals[[2]int32{int32(e.Goal0X), int32(e.Goal0Z)}] > 1 {
					c.SharedGoal++
				}
			}
			c.Mixed = vmax > 0 && vmin < vmax*0.8
			var ks []string
			for _, kn := range sortedKeys(kinds) {
				ks = append(ks, fmt.Sprintf("%s:%d", kn, kinds[kn]))
			}
			c.Kinds = strings.Join(ks, ",")
			out = append(out, c)
		}
	}
	return out
}

// purpose guesses what an episode's order was for, from what the recording
// shows happened at its goal: a structure this unit started there, a
// feature its player reclaimed there, or a shot the unit fired as it
// ended. Anything else is a move.
func (r *recording) purpose(u *unitTL, e *episode) string {
	if !e.HasGoal {
		return "unknown"
	}
	for _, b := range r.builtBy[u.Net] {
		if b.Born >= e.T0 && b.Born <= e.TEnd+90 {
			w, h := b.footprint()
			if math.Abs(b.BX-e.GoalX) <= w/2+96 && math.Abs(b.BZ-e.GoalZ) <= h/2+96 {
				return "build"
			}
		}
	}
	if u.Def != nil && u.Def.Builder {
		for _, rc := range r.File.Players[u.Player].Reclaims {
			if rc[0] >= e.T0 && rc[0] <= e.TEnd+90 &&
				math.Abs(float64(rc[1])*16+8-e.GoalX) <= 64 && math.Abs(float64(rc[2])*16+8-e.GoalZ) <= 64 {
				return "reclaim"
			}
		}
	}
	if sumBuckets(u.Fire, e.TEnd-60, e.TEnd+90) > 0 {
		return "combat"
	}
	if e.GoalUpdates > 0 {
		return "follow"
	}
	if e.First && u.Builder != 0 {
		return "egress"
	}
	return "move"
}

// rootChain is how many blockers a same-way block is followed through.
const rootChain = 8

// classifyBlock finds the likely blocker of one blocked span, names the
// situation, and follows a queue to what held its front.
func (r *recording) classifyBlock(u *unitTL, b *blockSpan) blockRec {
	rec, o := r.classifyBlockOnce(u, b)
	rec.Root = rec.Class
	if rec.Class != "same-way" {
		return rec
	}
	seen := []*unitTL{u}
	cur, at := rec, o
	for {
		if cur.Blocker == nil || at == nil || cur.Blocker.State != "held" {
			rec.Root = "free-leader"
			return rec
		}
		for _, v := range seen {
			if v == at {
				rec.Root = "mutual"
				return rec
			}
		}
		span := at.blockAt(b.T0)
		if span == nil || rec.Chain >= rootChain {
			rec.Root = "unknown"
			return rec
		}
		seen = append(seen, at)
		rec.Chain++
		var next *unitTL
		cur, next = r.classifyBlockOnce(at, &blockSpan{T0: b.T0, T1: span.T1, End: span.End, Carried: span.Carried})
		if cur.Class != "same-way" {
			rec.Root = cur.Class
			return rec
		}
		at = next
	}
}

func (r *recording) classifyBlockOnce(u *unitTL, b *blockSpan) (blockRec, *unitTL) {
	x, z, _, _ := u.pos(b.T0)
	hx, hz, hok := u.heading(b.T0)
	rec := blockRec{Rec: r.Key, Unit: u.Net, Name: u.Name, Kind: u.kind(), Player: u.Player, T0: b.T0, Dur: b.T1 - b.T0,
		End: b.End, Carried: b.Carried, X: x, Z: z, HX: hx, HZ: hz, Episode: u.episodeAt(b.T0), GoalDist: -1, StartDist: -1, Cohort: -1}
	if rec.Episode >= 0 {
		e := &u.Episodes[rec.Episode]
		if e.HasGoal {
			rec.GoalDist = dist(x, z, e.GoalX, e.GoalZ)
		}
		rec.StartDist = dist(x, z, e.X0, e.Z0)
		rec.Egress = e.First
		rec.Cohort = e.Cohort
	}
	w, h := u.footprint()
	ns := r.near(u, b.T0, x, z, w, h, crowdRadius)
	var best *neighbour
	bestScore := math.Inf(1)
	for i := range ns {
		n := &ns[i]
		if n.U.mobile() {
			rec.NearMob++
		} else {
			rec.NearStr++
		}
		if n.Gap > blockerReach {
			continue
		}
		// Prefer what lies ahead: the refused step is along the heading.
		score := n.Gap
		if hok {
			dx, dz := n.X-x, n.Z-z
			d := math.Hypot(dx, dz)
			if d > 0 {
				c := (dx*hx + dz*hz) / d
				score += (1 - c) * 24
			}
		}
		if score < bestScore {
			bestScore, best = score, n
		}
	}
	rec.Combat = u.combat(b.T0-90, b.T1+90) > 0
	if best == nil {
		rec.Class = "static"
		return rec, nil
	}
	o := best.U
	br := &blockerRec{Unit: o.Net, Name: o.Name, Player: o.Player, Rel: "other", Gap: best.Gap, HeadingDiff: -1}
	if o.Player == u.Player {
		br.Rel = "own"
	}
	if hok {
		dx, dz := best.X-x, best.Z-z
		if d := math.Hypot(dx, dz); d > 0 {
			br.Bearing = deg(math.Acos(math.Max(-1, math.Min(1, (dx*hx+dz*hz)/d))))
		}
	}
	switch {
	case !o.mobile():
		br.State = "structure"
		if o.Fin < 0 || b.T0 < o.Fin {
			br.State = "frame"
		}
	case best.Moving:
		br.State = "moving"
		if o.blockedAt(b.T0) {
			br.State = "held"
		}
		if ox, oz, ok := o.heading(b.T0); ok && hok {
			br.HeadingDiff = deg(math.Acos(math.Max(-1, math.Min(1, ox*hx+oz*hz))))
		}
	default:
		br.State = "parked"
	}
	if oe := o.episodeAt(b.T0); oe >= 0 && rec.Cohort >= 0 && o.Player == u.Player && o.Episodes[oe].Cohort == rec.Cohort {
		br.SameCohort = true
	}
	if o.mobile() && o.vmax() > 0 && u.vmax() > 0 && o.vmax() < u.vmax()*0.9 {
		br.Slower = true
	}
	rec.Blocker = br
	switch {
	case br.State == "structure" || br.State == "frame":
		rec.Class = "structure"
	case br.Rel == "other":
		rec.Class = "other-player"
	case br.State == "parked":
		rec.Class = "parked-friend"
	case br.HeadingDiff < 0:
		rec.Class = "moving-friend"
	case br.HeadingDiff <= 60:
		rec.Class = "same-way"
	case br.HeadingDiff >= 120:
		rec.Class = "head-on"
	default:
		rec.Class = "crossing"
	}
	return rec, o
}
