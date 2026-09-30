package main

import (
	"flag"
	"fmt"
	"math"
	"os"
	"sort"
	"strconv"
	"strings"
)

// A snippet is a piece of a recorded game cut out for replay: the units and
// structures that stood in a region during a window of ticks, where they
// were when the window opened, and the goals the recording shows them being
// given. It is a staging description, not a save: headings of idle units,
// fog, features destroyed by weapons and everything outside the region are
// not in a recording and are not invented here — the replay runner documents
// what it substitutes (docs/PATHFINDING_LAB.md).
type snippet struct {
	Schema int    `json:"schema"`
	ID     string `json:"id"`
	Rec    string `json:"rec"`
	Map    string `json:"map"`
	// Content is "ota" or "prota": which unit definitions the recording
	// was played with.
	Content string `json:"content"`
	Class   string `json:"class,omitempty"`
	Note    string `json:"note,omitempty"`
	T0      int32  `json:"t0"`
	T1      int32  `json:"t1"`
	// Start is the tick the units are staged at, a lead before T0: a unit
	// that was under way at T0 is staged where it was at Start and given
	// the order it was carrying out, so that at T0 it is moving as the
	// recording shows. A unit staged standing reads as parked to every
	// search for as long as it stands [04 R-PATH-01 §14].
	Start   int32       `json:"start"`
	Region  [4]float64  `json:"region"` // x0, z0, x1, z1 in world units
	Players []int       `json:"players"`
	Units   []snipUnit  `json:"units"`
	Orders  []snipOrder `json:"orders"`
	Groups  []snipGroup `json:"groups,omitempty"`
	// Reclaimed lists feature anchor cells the recording shows reclaimed
	// before the window opened [fmt tad §6.4].
	Reclaimed [][2]int32 `json:"reclaimed,omitempty"`
}

type snipUnit struct {
	ID     int     `json:"id"` // the recording's net ID
	Name   string  `json:"name"`
	Player int     `json:"player"`
	X      float64 `json:"x"`
	Z      float64 `json:"z"`
	// HX, HZ is the direction the unit was travelling or last travelled,
	// zero when the recording shows none.
	HX        float64 `json:"hx,omitempty"`
	HZ        float64 `json:"hz,omitempty"`
	Structure bool    `json:"structure,omitempty"`
	// Spawn is the tick the unit appears, T0 for one present at the start.
	Spawn int32 `json:"spawn"`
	// Die is the tick the recording shows the unit's death, -1 for none in
	// the window.
	Die     int32 `json:"die"`
	Subject bool  `json:"subject,omitempty"`
}

// snipGroup is one recorded group move: its members and the displacement
// the command gave every unit that kept its formation offset
// [04 R-STANCE-01 §5].
type snipGroup struct {
	Tick   int32   `json:"tick"`
	Player int     `json:"player"`
	DX     float64 `json:"dx"`
	DZ     float64 `json:"dz"`
	Units  []int   `json:"units"`
	// Count is the least size the recorded selection can have had: a
	// member keeps its formation offset only within a squared distance of
	// 3000 per selected unit from the selection's centre
	// [04 R-STANCE-01 §5], so the farthest member that kept its offset
	// bounds the selection from below.
	Count int32 `json:"count"`
}

type snipOrder struct {
	Tick  int32   `json:"tick"`
	Unit  int     `json:"unit"`
	GoalX float64 `json:"goal_x"`
	GoalZ float64 `json:"goal_z"`
	// Carry marks an order already under way when the window opened.
	Carry bool `json:"carry,omitempty"`
	// Derived marks a goal the recording never showed installed: the
	// episode began with a published route, and the goal is where the
	// unit's episode ended.
	Derived bool `json:"derived,omitempty"`
	// Group is one more than the index of the group command the order came
	// from, zero for an order of its own.
	Group int `json:"group,omitempty"`

	cohort int
	dx, dz float64
	x0, z0 float64
}

// carryNear is how close to its goal a unit with an order under way may be
// for the snippet to stage it standing instead of giving it the order
// again.
const carryNear = 128.0

// carrySoon is how soon after staging a unit's next order may come for the
// snippet to skip the order it was still carrying out: a goal installed
// within ten ticks of an admitted request keeps that request's stamp.
const carrySoon = 11

// snippetLead is how long before its window a snippet is staged.
const snippetLead = 30

type snippetSpec struct {
	id, class, note string
	t0, t1          int32
	region          [4]float64
	margin          float64
	subjects        map[*unitTL]bool
}

func inRect(x, z float64, r [4]float64, grow float64) bool {
	return x >= r[0]-grow && x <= r[2]+grow && z >= r[1]-grow && z <= r[3]+grow
}

// cut builds the snippet for spec from a recording.
func (r *recording) cut(spec snippetSpec) *snippet {
	start := spec.t0 - snippetLead
	s := &snippet{Schema: 1, ID: spec.id, Rec: r.Key, Map: r.File.Map, Class: spec.class, Note: spec.note,
		T0: spec.t0, T1: spec.t1, Start: start, Region: spec.region, Content: "ota", Units: []snipUnit{}, Orders: []snipOrder{}}
	if strings.HasSuffix(strings.ToLower(r.File.File), ".pro") || strings.HasPrefix(r.File.Source, "v4.") {
		s.Content = "prota"
	}
	players := map[int]bool{}
	for _, u := range r.Units {
		if u.Def != nil && u.Def.Air {
			continue
		}
		if u.Def == nil && !u.Ground {
			continue // neither a known structure nor a ground mover
		}
		from := u.Born
		if u.mobile() && u.Fin >= 0 {
			from = u.Fin
		}
		if from > spec.t1 || (u.Died >= 0 && u.Died <= start) {
			continue
		}
		if u.mobile() && u.Fin < 0 && len(u.Entries) == 0 {
			continue // never finished: a product still inside its factory
		}
		// Present in the region at any sampled tick of the window.
		in := false
		grow := spec.margin
		if !u.mobile() {
			w, h := u.footprint()
			grow += math.Max(w, h) / 2
		}
		for t := max(start, from); t <= spec.t1; t += 15 {
			if u.Died >= 0 && t >= u.Died {
				break
			}
			x, z, _, _ := u.pos(t)
			if inRect(x, z, spec.region, grow) {
				in = true
				break
			}
		}
		if !in && !spec.subjects[u] {
			continue
		}
		su := snipUnit{ID: u.Net, Name: u.Name, Player: u.Player, Structure: !u.mobile(), Spawn: max(start, from), Die: -1, Subject: spec.subjects[u]}
		if u.Died >= 0 && u.Died <= spec.t1 {
			su.Die = u.Died
		}
		su.X, su.Z, _, _ = u.pos(su.Spawn)
		if hx, hz, ok := u.heading(su.Spawn); ok {
			su.HX, su.HZ = hx, hz
		} else if hx, hz, ok := u.lastHeading(su.Spawn); ok {
			su.HX, su.HZ = hx, hz
		}
		players[u.Player] = true
		s.Units = append(s.Units, su)
		if !u.Ground {
			continue
		}
		for ei := range u.Episodes {
			e := &u.Episodes[ei]
			if e.TEnd <= start || e.T0 > spec.t1 {
				continue
			}
			o := snipOrder{Tick: max(e.T0, su.Spawn), Unit: u.Net, GoalX: e.Goal0X, GoalZ: e.Goal0Z, Carry: e.T0 < start,
				cohort: -1, dx: e.Goal0X - e.X0, dz: e.Goal0Z - e.Z0, x0: e.X0, z0: e.Z0}
			if e.HasGoal && e.T0 >= start && e.Cohort >= 0 {
				o.cohort = e.Cohort
			}
			if e.GoalUpdates > 0 && e.T0 < start {
				o.GoalX, o.GoalZ = e.GoalX, e.GoalZ
			}
			if o.Carry && e.HasGoal && dist(su.X, su.Z, o.GoalX, o.GoalZ) <= carryNear {
				// Already at its destination's crowd when the window
				// opens: staged standing, not re-ordered.
				continue
			}
			if o.Carry && ei+1 < len(u.Episodes) && u.Episodes[ei+1].T0 <= start+carrySoon {
				// About to be re-ordered. Giving it the old order first
				// would stamp a route request moments before the new
				// goal, and a request stamped within ten ticks of a goal
				// installation makes the follower wait out its sixty-tick
				// throttle [04 R-PATH-01 §8]: a delay the recorded unit,
				// whose request was older, did not have.
				continue
			}
			if !e.HasGoal {
				if !e.EndKnown || e.End == "superseded" || e.End == "died" || e.End == "open" {
					continue
				}
				o.GoalX, o.GoalZ, o.Derived = e.EndX, e.EndZ, true
			}
			s.Orders = append(s.Orders, o)
		}
	}
	s.groupOrders()
	for p := range players {
		s.Players = append(s.Players, p)
	}
	sort.Ints(s.Players)
	sort.SliceStable(s.Units, func(i, j int) bool {
		if s.Units[i].Spawn != s.Units[j].Spawn {
			return s.Units[i].Spawn < s.Units[j].Spawn
		}
		if s.Units[i].Player != s.Units[j].Player {
			return s.Units[i].Player < s.Units[j].Player
		}
		return s.Units[i].ID < s.Units[j].ID
	})
	sort.SliceStable(s.Orders, func(i, j int) bool {
		if s.Orders[i].Tick != s.Orders[j].Tick {
			return s.Orders[i].Tick < s.Orders[j].Tick
		}
		return s.Orders[i].Unit < s.Orders[j].Unit
	})
	for _, p := range r.File.Players {
		for _, rc := range p.Reclaims {
			if rc[0] <= start && inRect(float64(rc[1])*16+8, float64(rc[2])*16+8, spec.region, spec.margin+256) {
				s.Reclaimed = append(s.Reclaimed, [2]int32{rc[1], rc[2]})
			}
		}
	}
	return s
}

// groupOrders finds, among the orders of each cohort, the displacement most
// of them share — the formation's — and records the cohort as one group
// command. A cohort with no shared displacement was given one target or one
// point and stays a set of single orders.
func (s *snippet) groupOrders() {
	by := map[int][]int{}
	var ids []int
	for i := range s.Orders {
		if c := s.Orders[i].cohort; c >= 0 {
			if _, ok := by[c]; !ok {
				ids = append(ids, c)
			}
			by[c] = append(by[c], i)
		}
	}
	sort.Ints(ids)
	player := map[int]int{}
	for _, u := range s.Units {
		player[u.ID] = u.Player
	}
	for _, c := range ids {
		m := by[c]
		if len(m) < 2 {
			continue
		}
		best, bestN := -1, 0
		for _, i := range m {
			n := 0
			for _, j := range m {
				if math.Abs(s.Orders[i].dx-s.Orders[j].dx) <= cohortDispTol && math.Abs(s.Orders[i].dz-s.Orders[j].dz) <= cohortDispTol {
					n++
				}
			}
			if n > bestN {
				best, bestN = i, n
			}
		}
		if bestN < 2 || bestN*2 < len(m) {
			continue
		}
		// The members' mean displacement, over those that share it.
		var dx, dz float64
		for _, j := range m {
			if math.Abs(s.Orders[best].dx-s.Orders[j].dx) <= cohortDispTol && math.Abs(s.Orders[best].dz-s.Orders[j].dz) <= cohortDispTol {
				dx += s.Orders[j].dx
				dz += s.Orders[j].dz
			}
		}
		g := snipGroup{Tick: s.Orders[m[0]].Tick, Player: player[s.Orders[m[0]].Unit], DX: math.Round(dx / float64(bestN)), DZ: math.Round(dz / float64(bestN))}
		var cx, cz float64
		for _, j := range m {
			cx, cz = cx+s.Orders[j].x0, cz+s.Orders[j].z0
		}
		cx, cz = cx/float64(len(m)), cz/float64(len(m))
		far := 0.0
		for _, j := range m {
			if math.Abs(s.Orders[best].dx-s.Orders[j].dx) <= cohortDispTol && math.Abs(s.Orders[best].dz-s.Orders[j].dz) <= cohortDispTol {
				ox, oz := s.Orders[j].x0-cx, s.Orders[j].z0-cz
				far = math.Max(far, ox*ox+oz*oz)
			}
		}
		// A margin of a tenth for the centre the missing members moved.
		g.Count = max(int32(len(m)), int32(math.Ceil(far*1.1/3000)))
		for _, j := range m {
			g.Tick = min(g.Tick, s.Orders[j].Tick)
			g.Units = append(g.Units, s.Orders[j].Unit)
			s.Orders[j].Group = len(s.Groups) + 1
		}
		sort.Ints(g.Units)
		s.Groups = append(s.Groups, g)
	}
	sort.SliceStable(s.Groups, func(i, j int) bool { return s.Groups[i].Tick < s.Groups[j].Tick })
	// Sorting moved the groups; renumber the orders that name them.
	index := map[[2]int]int{}
	for gi, g := range s.Groups {
		for _, u := range g.Units {
			index[[2]int{u, int(g.Tick)}] = gi + 1
		}
	}
	for i := range s.Orders {
		o := &s.Orders[i]
		if o.Group == 0 {
			continue
		}
		o.Group = 0
		for gi, g := range s.Groups {
			for _, u := range g.Units {
				if u == o.Unit && o.Tick >= g.Tick && o.Tick <= g.Tick+cohortWindow && o.cohort >= 0 {
					o.Group = gi + 1
				}
			}
		}
	}
	_ = index
}

// lastHeading is the direction of the last route the unit held before t.
func (u *unitTL) lastHeading(t int32) (hx, hz float64, ok bool) {
	n := len(u.Entries)
	i := sort.Search(n, func(i int) bool { return u.Entries[i].T > t }) - 1
	for ; i >= 0; i-- {
		e := &u.Entries[i]
		if u.Kinds[i] == entIdle || e.N < 2 {
			continue
		}
		dx, dz := float64(e.P[1][0]-e.P[0][0]), float64(e.P[1][1]-e.P[0][1])
		if d := math.Hypot(dx, dz); d >= 1 {
			return dx / d, dz / d, true
		}
	}
	return 0, 0, false
}

func cmdSnippet(args []string) error {
	fs := flag.NewFlagSet("snippet", flag.ExitOnError)
	home, _ := os.UserHomeDir()
	moves := fs.String("moves", "", "movement extract of the recording (required)")
	units := fs.String("units", home+"/nanolathe-bench/path-redesign/data/units-ota.json", "unit table for original TA recordings")
	unitsProTA := fs.String("units-prota", home+"/nanolathe-bench/path-redesign/data/units-prota.json", "unit table for ProTA recordings")
	t0 := fs.Int("t0", 0, "first tick")
	t1 := fs.Int("t1", 0, "last tick")
	region := fs.String("region", "", "x0,z0,x1,z1 in world units; omitted takes the subjects' bounding box")
	margin := fs.Float64("margin", 160, "world units around the region whose units are staged too")
	cohort := fs.Int("cohort", -1, "cohort index whose members are the subjects")
	subj := fs.String("subjects", "", "comma-separated net IDs of the subjects")
	id := fs.String("id", "", "snippet ID")
	class := fs.String("class", "", "problem class label")
	out := fs.String("out", "-", "output file")
	fs.Parse(args)
	var tabs tables
	var err error
	if tabs.ota, err = loadUnitTable(*units); err != nil {
		return err
	}
	if tabs.prota, err = loadUnitTable(*unitsProTA); err != nil {
		return err
	}
	r, err := loadRecording(*moves, tabs)
	if err != nil {
		return err
	}
	spec := snippetSpec{id: *id, class: *class, t0: int32(*t0), t1: int32(*t1), margin: *margin, subjects: map[*unitTL]bool{}}
	if *cohort >= 0 {
		cs := r.cohorts()
		if *cohort >= len(cs) {
			return fmt.Errorf("snippet: cohort %d of %d", *cohort, len(cs))
		}
		for _, u := range r.Units {
			for ei := range u.Episodes {
				if u.Episodes[ei].Cohort == *cohort {
					spec.subjects[u] = true
					if spec.t0 == 0 {
						spec.t0 = cs[*cohort].T0 - 1
					}
					spec.t1 = max(spec.t1, u.Episodes[ei].TEnd+30)
				}
			}
		}
	}
	for _, f := range strings.Split(*subj, ",") {
		if f = strings.TrimSpace(f); f == "" {
			continue
		}
		n, err := strconv.Atoi(f)
		if err != nil {
			return err
		}
		// The unit holding that net ID when the window opens.
		for _, u := range r.Units {
			if u.Net == n && u.alive(spec.t0) {
				spec.subjects[u] = true
			}
		}
	}
	if *region != "" {
		p := strings.Split(*region, ",")
		if len(p) != 4 {
			return fmt.Errorf("snippet: -region wants x0,z0,x1,z1")
		}
		for i := range p {
			if spec.region[i], err = strconv.ParseFloat(strings.TrimSpace(p[i]), 64); err != nil {
				return err
			}
		}
	} else {
		spec.region = r.bounds(spec.subjects, spec.t0, spec.t1)
	}
	if spec.id == "" {
		spec.id = fmt.Sprintf("%s@%d", strings.ReplaceAll(r.Key, "/", "-"), spec.t0)
	}
	return writeJSONMaybeGz(*out, r.cut(spec))
}

// bounds is the bounding box of the subjects' positions and goals over the
// window.
func (r *recording) bounds(subjects map[*unitTL]bool, t0, t1 int32) [4]float64 {
	b := [4]float64{math.Inf(1), math.Inf(1), math.Inf(-1), math.Inf(-1)}
	add := func(x, z float64) {
		b[0], b[1], b[2], b[3] = math.Min(b[0], x), math.Min(b[1], z), math.Max(b[2], x), math.Max(b[3], z)
	}
	for u := range subjects {
		for t := t0; t <= t1; t += 15 {
			if !u.alive(t) {
				continue
			}
			x, z, _, _ := u.pos(t)
			add(x, z)
		}
		for ei := range u.Episodes {
			e := &u.Episodes[ei]
			if e.HasGoal && e.TEnd > t0 && e.T0 <= t1 {
				add(e.Goal0X, e.Goal0Z)
			}
		}
	}
	if math.IsInf(b[0], 1) {
		return [4]float64{}
	}
	return b
}
