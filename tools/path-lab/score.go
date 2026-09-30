package main

import (
	"flag"
	"fmt"
	"math"
	"os"
	"sort"
	"strings"
)

// A score measures what a snippet's subjects did in one log — the recording
// itself, or a replay of the snippet — with the same definitions for both.
// Every subject is scored on its first order of the window that was not
// already under way when the window opened.

type subjectScore struct {
	Unit   int    `json:"unit"`
	Name   string `json:"name"`
	Found  bool   `json:"found"`
	TOrder int32  `json:"t_order"`
	TUntil int32  `json:"t_until"` // end of the scored span: window end or the unit's next order
	// The goal the subject is judged against. A replay's is the goal its own
	// engine gave the unit (OwnGoal): a rule set that places a group's
	// members differently sends a unit somewhere other than the recording
	// did, and it has arrived when it rests there. RecX and RecZ are the
	// recorded goal.
	GoalX   float64 `json:"goal_x"`
	GoalZ   float64 `json:"goal_z"`
	RecX    float64 `json:"rec_x"`
	RecZ    float64 `json:"rec_z"`
	OwnGoal bool    `json:"own_goal,omitempty"`
	Dist0   float64 `json:"dist0"` // distance to the goal at the order
	// Near is the first tick within nearRadius of the goal, -1 when the
	// subject never came that near in the scored span.
	Near      int32   `json:"near"`
	NearTicks int32   `json:"near_ticks"` // Near - TOrder, the span's length when never near
	Reached   bool    `json:"reached"`
	Died      bool    `json:"died"`
	Latency   int32   `json:"latency"`       // order to first published route, -1 when none
	Blocked   int32   `json:"blocked_ticks"` // blocked ticks before Near (or span end)
	Blocks    int     `json:"blocks"`        // blocked spans before Near
	Routes    int     `json:"routes"`        // route publications before Near
	Length    float64 `json:"length"`        // distance travelled before Near
	Ideal     float64 `json:"ideal_ticks"`   // Dist0 less the radius, over MaxVelocity
	FinalDist float64 `json:"final_dist"`    // distance to the goal at the span's end
	// Shortest is the static shortest path from the order's position to
	// the goal, less the near radius; zero when the map's grid is absent
	// or no path was found.
	Shortest float64 `json:"shortest,omitempty"`
	// Progress is how much nearer the goal the subject ended than it began.
	Progress float64 `json:"progress"`
	Settle   int32   `json:"settle"` // tick the episode's route went inactive for good, -1 when it never did

	// The trip to rest. Crowd is how many subjects were sent to the same
	// place at the same time, this one included, and Radius how near its
	// goal a unit of that crowd counts as there: a crowd cannot stand on
	// one point. Rest is the tick the unit came to rest for good within
	// Radius, -1 when it did not in the scored span; TripTicks is
	// Rest - TOrder, the span's length when it never rested there.
	Crowd     int     `json:"crowd"`
	Radius    float64 `json:"radius"`
	Rest      int32   `json:"rest"`
	Rested    bool    `json:"rested"`
	TripTicks int32   `json:"trip_ticks"`
	RestDist  float64 `json:"rest_dist"` // distance to the goal where it rested, -1 when it never rested
	// Arrive is the tick the unit FIRST came to rest within Radius of its
	// goal, -1 when it never did, and ArriveTicks that tick less TOrder, the
	// span's length when it never did. Unrest is the ticks it spent on a
	// route again after that, in the scored span, and Restarts how many
	// times it set off again: a unit sent aside by a friend, or shuffling
	// for its place, has arrived all the same.
	Arrive      int32 `json:"arrive"`
	Arrived     bool  `json:"arrived"`
	ArriveTicks int32 `json:"arrive_ticks"`
	Unrest      int32 `json:"unrest_ticks"`
	Restarts    int   `json:"restarts"`
	// How the trip looked, a replay only, over the span to Rest: distance
	// moved, turning in circles, stops while holding a route, and distance
	// moved away from the goal.
	Moved     float64 `json:"moved,omitempty"`
	Turns     float64 `json:"turns,omitempty"`
	Stops     int     `json:"stops,omitempty"`
	Away      float64 `json:"away,omitempty"`
	Reversals int     `json:"reversals,omitempty"`
	HasLook   bool    `json:"has_look,omitempty"`
}

type logScore struct {
	Log      string         `json:"log"`
	Rules    string         `json:"rules,omitempty"`
	Jitter   int            `json:"jitter"`
	Subjects []subjectScore `json:"subjects"`
	// Aggregates over the subjects found alive in this log.
	N              int     `json:"n"`
	Reached        int     `json:"reached"`
	MeanNear       float64 `json:"mean_near_ticks"` // censored at the span for those never near
	MedianNear     float64 `json:"median_near_ticks"`
	LastNear       int32   `json:"last_near_ticks"`
	Blocked        int64   `json:"blocked_ticks"`
	Blocks         int     `json:"blocks"`
	MeanLatency    float64 `json:"mean_latency"`
	Length         float64 `json:"length"`
	MeanFinal      float64 `json:"mean_final_dist"`
	Ideal          float64 `json:"ideal_ticks"`
	Shortest       float64 `json:"shortest"`        // summed over subjects with one
	ShortLength    float64 `json:"shortest_length"` // their travelled length, reached subjects only
	Progress       float64 `json:"progress"`        // summed over subjects
	Overlap        int64   `json:"overlap_unit_ticks"`
	OverlapResting int64   `json:"overlap_resting_unit_ticks"`
	OverlapLast    int     `json:"overlap_units_last"`
	MoverTicks     int64   `json:"mover_ticks"`
	Probes         uint64  `json:"probes"`
	Anchors        uint64  `json:"anchors_tested"`
	Searches       int64   `json:"searches"`
	Pops           int64   `json:"pops"`
	// The trip to rest, over the subjects found: censored mean, median and
	// ninetieth percentile of TripTicks, and how many rested at their goal.
	Rested int `json:"rested"`
	// Arrival at rest, first time: how many did, the censored mean, median
	// and ninetieth percentile of ArriveTicks, and the unrest afterwards.
	Arrived      int     `json:"arrived"`
	MeanArrive   float64 `json:"mean_arrive_ticks"`
	MedianArrive float64 `json:"median_arrive_ticks"`
	P90Arrive    float64 `json:"p90_arrive_ticks"`
	Unrest       int64   `json:"unrest_ticks"`
	Restarts     int     `json:"restarts"`
	MeanTrip     float64 `json:"mean_trip_ticks"`
	MedianTrip   float64 `json:"median_trip_ticks"`
	P90Trip      float64 `json:"p90_trip_ticks"`
	// Looks, summed over the subjects of a replay.
	Moved     float64 `json:"moved"`
	Turns     float64 `json:"turns"`
	Stops     int     `json:"stops"`
	Away      float64 `json:"away"`
	Reversals int     `json:"reversals"`
	TickP50   float64 `json:"tick_ms_p50,omitempty"`
	TickP99   float64 `json:"tick_ms_p99,omitempty"`
	TickMax   float64 `json:"tick_ms_max,omitempty"`
}

const nearRadius = 96.0

// A crowd is the subjects ordered within crowdTicks of each other to goals
// within crowdGoal world units of each other. A unit of a crowd of n is at
// its goal within nearRadius plus its footprint times the square root of n:
// the half-width of the block n such units stand in.
const (
	crowdTicks = 8
	crowdGoal  = 320.0
)

// motionAt returns a replay unit's running totals at tick t: the last sample
// at or before it.
func motionAt(m [][6]int32, t int32) (s [6]int32, ok bool) {
	i := sort.Search(len(m), func(i int) bool { return m[i][0] > t })
	if i == 0 {
		return s, false
	}
	return m[i-1], true
}

func scoreLog(s *snippet, r *recording, name string, grid *mapGrid) logScore {
	out := logScore{Log: name}
	if r.File.Engine != nil {
		out.Rules = r.File.Engine.Rules
		out.Jitter = r.File.Engine.Jitter
	}
	subjects := map[int]bool{}
	for _, u := range s.Units {
		if u.Subject {
			subjects[u.ID] = true
		}
	}
	orders := map[int][]snipOrder{}
	for _, o := range s.Orders {
		orders[o.Unit] = append(orders[o.Unit], o)
	}
	ids := make([]int, 0, len(subjects))
	for id := range subjects {
		ids = append(ids, id)
	}
	sort.Ints(ids)
	// scored returns the order a subject is scored on and the end of its
	// span.
	scored := func(id int) (*snipOrder, int32) {
		var first *snipOrder
		until := s.T1
		carried := true
		for i := range orders[id] {
			if !orders[id][i].Carry && orders[id][i].Tick >= s.T0 {
				carried = false
			}
		}
		for i := range orders[id] {
			o := &orders[id][i]
			// A subject whose only order was already under way when the
			// window opened is scored on that order from the window's
			// first tick.
			if (o.Carry || o.Tick < s.T0) && !carried {
				continue
			}
			if first == nil {
				first = o
			} else if o.Tick > first.Tick && (math.Abs(o.GoalX-first.GoalX) > goalUpdateTol || math.Abs(o.GoalZ-first.GoalZ) > goalUpdateTol) {
				until = min(until, o.Tick)
				break
			}
		}
		return first, until
	}
	firsts := map[int]*snipOrder{}
	for _, id := range ids {
		if o, _ := scored(id); o != nil {
			firsts[id] = o
		}
	}
	var nears, trips, arrives []float64
	for _, id := range ids {
		sc := subjectScore{Unit: id, Near: -1, Latency: -1, Settle: -1, Rest: -1, RestDist: -1}
		first, until := scored(id)
		var u *unitTL
		for _, c := range r.Units {
			if c.Net == id && c.Ground && first != nil && c.alive(first.Tick) {
				u = c
				break
			}
		}
		if first == nil || u == nil {
			out.Subjects = append(out.Subjects, sc)
			continue
		}
		sc.Found, sc.Name = true, u.Name
		sc.TOrder, sc.GoalX, sc.GoalZ = max(first.Tick, s.T0), first.GoalX, first.GoalZ
		sc.RecX, sc.RecZ = first.GoalX, first.GoalZ
		if u.Died >= 0 && u.Died < until {
			until = u.Died
			sc.Died = true
		}
		sc.TUntil = until
		// A replay's own goal: the last its engine held for the unit before
		// the span's end, when it lies within the block the order's whole
		// group stands in round the recorded one. Farther off it is not this
		// order's. A goal noted on the span's last tick is the next order's.
		group := 0
		for _, oid := range ids {
			if o := firsts[oid]; o != nil && absInt32(max(o.Tick, s.T0)-sc.TOrder) <= crowdTicks {
				group++
			}
		}
		if i := sort.Search(len(u.Goals), func(i int) bool { return u.Goals[i][0] >= until }); i > 0 {
			gw, gh := u.footprint()
			ox, oz := float64(u.Goals[i-1][1]), float64(u.Goals[i-1][2])
			if dist(ox, oz, first.GoalX, first.GoalZ) <= nearRadius+2*math.Max(gw, gh)*math.Sqrt(float64(max(group, 1))) {
				sc.GoalX, sc.GoalZ, sc.OwnGoal = ox, oz, true
			}
		}
		x0, z0, _, _ := u.pos(sc.TOrder)
		sc.Dist0 = dist(x0, z0, sc.GoalX, sc.GoalZ)
		if v := u.vmax(); v > 0 {
			sc.Ideal = math.Max(0, sc.Dist0-nearRadius) / v
		}
		px, pz := x0, z0
		for t := sc.TOrder; t <= until; t += 3 {
			x, z, _, _ := u.pos(t)
			if sc.Near < 0 {
				sc.Length += dist(px, pz, x, z)
			}
			px, pz = x, z
			if sc.Near < 0 && dist(x, z, sc.GoalX, sc.GoalZ) <= nearRadius {
				sc.Near = t
			}
		}
		stop := until
		if sc.Near >= 0 {
			stop, sc.Reached = sc.Near, true
		}
		sc.NearTicks = stop - sc.TOrder
		for _, b := range u.Blocks {
			t0, t1 := max(b.T0, sc.TOrder), min(b.T1, stop)
			if t1 > t0 {
				sc.Blocked += t1 - t0
				sc.Blocks++
			}
		}
		for i := range u.Entries {
			e := &u.Entries[i]
			if e.T < sc.TOrder || e.T > stop {
				continue
			}
			if u.Kinds[i] == entRoute && (i == 0 || u.Kinds[i-1] != entRoute || e.P[0] != u.Entries[i-1].P[0] || e.P[1] != u.Entries[i-1].P[1]) {
				if i > 0 && u.Kinds[i-1] != entIdle && u.Entries[i-1].N >= 2 && e.P[0] == u.Entries[i-1].P[1] {
					continue // a consumed waypoint, not a publication
				}
				sc.Routes++
				if sc.Latency < 0 {
					sc.Latency = e.T - sc.TOrder
				}
			}
		}
		fx, fz, _, _ := u.pos(until)
		sc.FinalDist = dist(fx, fz, sc.GoalX, sc.GoalZ)
		sc.Progress = sc.Dist0 - sc.FinalDist
		if grid != nil && u.Def != nil && sc.Reached {
			if sp, ok := grid.shortest(u.Def.Class, x0, z0, sc.GoalX, sc.GoalZ, 400000); ok {
				sc.Shortest = math.Max(1, sp-nearRadius)
				out.Shortest += sc.Shortest
				out.ShortLength += sc.Length
			}
		}
		out.Progress += sc.Progress
		for _, sg := range u.Segs {
			if !sg.Moving && sg.T0 >= sc.TOrder && sg.T0 <= until && sg.T1 > until {
				sc.Settle = sg.T0
			}
		}
		// The trip to rest.
		for _, oid := range ids {
			o := firsts[oid]
			if o == nil || absInt32(max(o.Tick, s.T0)-sc.TOrder) > crowdTicks {
				continue
			}
			if dist(o.GoalX, o.GoalZ, sc.GoalX, sc.GoalZ) <= crowdGoal {
				sc.Crowd++
			}
		}
		fw, fh := u.footprint()
		sc.Radius = nearRadius + math.Max(fw, fh)*math.Sqrt(float64(max(sc.Crowd, 1)))
		end := until
		for _, sg := range u.Segs {
			// At rest for good: the stretch of rest that lasts to the
			// span's end. A unit that never moved for its order rested
			// when the order was given.
			if sg.Moving || sg.T1 < until || sg.T0 > until {
				continue
			}
			t := max(sg.T0, sc.TOrder)
			rx, rz, _, _ := u.pos(t)
			sc.RestDist = dist(rx, rz, sc.GoalX, sc.GoalZ)
			if sc.RestDist <= sc.Radius {
				sc.Rest, sc.Rested, end = t, true, t
			}
		}
		sc.TripTicks = end - sc.TOrder
		sc.Arrive, sc.ArriveTicks = -1, until-sc.TOrder
		for _, sg := range u.Segs {
			if sg.T1 <= sc.TOrder || sg.T0 > until {
				continue
			}
			if !sc.Arrived {
				if sg.Moving {
					continue
				}
				t := max(sg.T0, sc.TOrder)
				if ax, az, _, _ := u.pos(t); dist(ax, az, sc.GoalX, sc.GoalZ) <= sc.Radius {
					sc.Arrive, sc.Arrived, sc.ArriveTicks = t, true, t-sc.TOrder
				}
				continue
			}
			if sg.Moving {
				sc.Unrest += min(sg.T1, until) - max(sg.T0, sc.Arrive)
				sc.Restarts++
			}
		}
		arrives = append(arrives, float64(sc.ArriveTicks))
		out.MeanArrive += float64(sc.ArriveTicks)
		out.Unrest += int64(sc.Unrest)
		out.Restarts += sc.Restarts
		if sc.Arrived {
			out.Arrived++
		}
		if len(u.Motion) > 0 {
			// Before its first sample a unit's totals are zero.
			a, _ := motionAt(u.Motion, sc.TOrder)
			if b, ok := motionAt(u.Motion, end); ok && b[0] >= a[0] {
				sc.HasLook = true
				sc.Moved, sc.Turns = float64(b[1]-a[1]), float64(b[2]-a[2])/256
				sc.Stops, sc.Away = int(b[3]-a[3]), float64(b[4]-a[4])
				sc.Reversals = int(b[5] - a[5])
				out.Reversals += sc.Reversals
				out.Moved += sc.Moved
				out.Turns += sc.Turns
				out.Stops += sc.Stops
				out.Away += sc.Away
			}
		}
		trips = append(trips, float64(sc.TripTicks))
		out.MeanTrip += float64(sc.TripTicks)
		if sc.Rested {
			out.Rested++
		}
		out.Subjects = append(out.Subjects, sc)
		out.N++
		if sc.Reached {
			out.Reached++
			out.LastNear = max(out.LastNear, sc.NearTicks)
		}
		nears = append(nears, float64(sc.NearTicks))
		out.MeanNear += float64(sc.NearTicks)
		out.Blocked += int64(sc.Blocked)
		out.Blocks += sc.Blocks
		if sc.Latency >= 0 {
			out.MeanLatency += float64(sc.Latency)
		}
		out.Length += sc.Length
		out.MeanFinal += sc.FinalDist
		out.Ideal += sc.Ideal
	}
	if out.N > 0 {
		n := float64(out.N)
		out.MeanNear /= n
		out.MeanLatency /= n
		out.MeanFinal /= n
		sort.Float64s(nears)
		out.MedianNear = nears[len(nears)/2]
		out.MeanTrip /= n
		sort.Float64s(trips)
		out.MedianTrip = trips[len(trips)/2]
		out.P90Trip = trips[min(len(trips)-1, len(trips)*9/10)]
		out.MeanArrive /= n
		sort.Float64s(arrives)
		out.MedianArrive = arrives[len(arrives)/2]
		out.P90Arrive = arrives[min(len(arrives)-1, len(arrives)*9/10)]
	}
	if e := r.File.Engine; e != nil {
		out.Overlap = e.OverlapTicks
		out.OverlapResting, out.OverlapLast = e.OverlapResting, e.OverlapLast
		out.MoverTicks, out.Probes, out.Anchors = e.MoverTicks, e.Probes, e.Anchors
		out.Searches, out.Pops = e.Searches, e.Pops
		if len(e.TickNS) > 0 {
			t := append([]int64(nil), e.TickNS...)
			sort.Slice(t, func(i, j int) bool { return t[i] < t[j] })
			out.TickP50 = float64(t[len(t)/2]) / 1e6
			out.TickP99 = float64(t[len(t)*99/100]) / 1e6
			out.TickMax = float64(t[len(t)-1]) / 1e6
		}
	}
	return out
}

func absInt32(v int32) int32 {
	if v < 0 {
		return -v
	}
	return v
}

func cmdScore(args []string) error {
	fs := flag.NewFlagSet("score", flag.ExitOnError)
	home, _ := os.UserHomeDir()
	snip := fs.String("snippet", "", "snippet file (required)")
	units := fs.String("units", home+"/nanolathe-bench/path-redesign/data/units-ota.json", "unit table for original TA")
	unitsProTA := fs.String("units-prota", home+"/nanolathe-bench/path-redesign/data/units-prota.json", "unit table for ProTA")
	out := fs.String("out", "", "write the scores as JSON to this file")
	maps := fs.String("maps", home+"/nanolathe-bench/path-redesign/data/maps", "directory of exported map grids, empty for none")
	quiet := fs.Bool("quiet", false, "print nothing")
	fs.Parse(args)
	var s snippet
	if err := readJSONMaybeGz(*snip, &s); err != nil {
		return err
	}
	var tabs tables
	var err error
	if tabs.ota, err = loadUnitTable(*units); err != nil {
		return err
	}
	if tabs.prota, err = loadUnitTable(*unitsProTA); err != nil {
		return err
	}
	if s.Content == "prota" {
		tabs.ota = tabs.prota
	}
	store := &mapStore{dir: *maps}
	grid := store.grid(s.Map)
	var scores []logScore
	for _, f := range fs.Args() {
		r, err := loadRecording(f, tabs)
		if err != nil {
			return err
		}
		name := "recorded"
		if r.File.Engine != nil {
			name = r.File.Engine.Rules
		}
		scores = append(scores, scoreLog(&s, r, name, grid))
	}
	if !*quiet {
		fmt.Printf("snippet %s (%s, %s, ticks %d..%d)\n", s.ID, s.Map, s.Class, s.T0, s.T1)
		fmt.Printf("%-24s %4s %5s %9s %9s %8s %9s %7s %8s %8s %9s %9s %8s\n", "log", "n", "reach", "mean-near", "med-near", "last", "blocked", "blocks", "latency", "length", "final", "overlap", "tick-p99")
		for _, l := range scores {
			fmt.Printf("%-24s %4d %5d %9.0f %9.0f %8d %9d %7d %8.1f %8.0f %9.0f %9d %8.2f\n", l.Log, l.N, l.Reached, l.MeanNear, l.MedianNear, l.LastNear,
				l.Blocked, l.Blocks, l.MeanLatency, l.Length, l.MeanFinal, l.Overlap, l.TickP99)
		}
		if len(scores) > 0 {
			fmt.Printf("ideal mean near ticks (straight line at full speed): %.0f\n", scores[0].Ideal/math.Max(1, float64(scores[0].N)))
		}
	}
	if *out != "" {
		return writeJSONMaybeGz(*out, map[string]any{"snippet": strings.TrimSpace(s.ID), "map": s.Map, "class": s.Class, "t0": s.T0, "t1": s.T1, "scores": scores})
	}
	return nil
}
