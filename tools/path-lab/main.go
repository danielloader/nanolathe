// Command path-lab is the pathfinding laboratory's offline half: it reads
// movement extracts of recorded games (tools/tad-extract -moves) and the
// same-shaped logs the engine's replay runner writes, reconstructs what each
// ground unit did, and measures it — order episodes, blocked spans and who
// caused them, group orders, stalls — with one set of definitions for both.
//
// Recordings and everything derived from them are local research data and
// never enter the repository. docs/PATHFINDING_LAB.md describes the method.
package main

import (
	"flag"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
)

func main() {
	if len(os.Args) < 2 {
		usage()
	}
	var err error
	switch os.Args[1] {
	case "mine":
		err = cmdMine(os.Args[2:])
	case "show":
		err = cmdShow(os.Args[2:])
	case "validate":
		err = cmdValidate(os.Args[2:])
	case "snippet":
		err = cmdSnippet(os.Args[2:])
	case "score":
		err = cmdScore(os.Args[2:])
	case "frames":
		err = cmdFrames(os.Args[2:])
	case "pick":
		err = cmdPick(os.Args[2:])
	default:
		usage()
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "nanolathe: path-lab:", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: path-lab mine|show|validate|snippet|score|frames|pick [flags]")
	os.Exit(2)
}

// minedFile is the per-recording output of mine.
type minedFile struct {
	Rec        string       `json:"rec"`
	Map        string       `json:"map"`
	LastTick   int32        `json:"last_tick"`
	Players    int          `json:"players"`
	Combatants int          `json:"combatants"`
	Engine     *engineInfo  `json:"engine,omitempty"`
	Ground     int          `json:"ground_units"`
	Blocks     []blockRec   `json:"blocks"`
	Episodes   []episodeRec `json:"episodes"`
	Cohorts    []cohortRec  `json:"cohorts"`
	Tally      tally        `json:"tally"`
	Recorder   string       `json:"recorder,omitempty"`
	EngineKind string       `json:"engine_kind"`
	Content    string       `json:"content"`
	Moving     int64        `json:"moving_ticks"`
	BlockedT   int64        `json:"blocked_ticks"`
	// Heat lists blocked unit-ticks per 64-world-unit map cell, by class.
	Heat []heatCell `json:"heat,omitempty"`
}

type mineOptions struct {
	minBlock   int32
	minCohort  int
	allRecords bool
	maps       *mapStore
	detours    int // episodes per recording measured against the static shortest path
}

// engineOf names the executable family a recorder string implies: the 3.9
// community patches carry their own recorder and the raised search
// allowance [community-patch-engine.md]; older standalone recorders sat on
// the retail executable.
func engineOf(recorder string) string {
	r := strings.ToLower(strings.TrimSpace(recorder))
	switch {
	case r == "":
		return "sim"
	case strings.HasPrefix(r, "3.9"):
		return "3.9"
	case strings.HasPrefix(r, "taf"):
		return "taf"
	}
	return "3.1"
}

// heat is blocked unit-ticks per coarse map cell and class.
type heatCell struct {
	X, Z  int32
	Ticks map[string]int64
}

const heatCellSize = 64.0

func mineRecording(r *recording, opt mineOptions) *minedFile {
	out := &minedFile{Rec: r.Key, Map: r.File.Map, LastTick: r.Last, Players: r.Players, Combatants: r.Combatants,
		Engine: r.File.Engine, Tally: tally{}, Blocks: []blockRec{}, Episodes: []episodeRec{}, Cohorts: []cohortRec{},
		Recorder: r.File.Recorder, EngineKind: engineOf(r.File.Recorder), Content: "ota"}
	if strings.HasSuffix(strings.ToLower(r.File.File), ".pro") || strings.HasPrefix(r.File.Source, "v4.") {
		out.Content = "prota"
	}
	eng := out.EngineKind
	grid := opt.maps.grid(r.File.Map)
	heat := map[[2]int32]*heatCell{}
	detoursLeft := opt.detours
	t := out.Tally
	cohorts := r.cohorts()
	selfTicks := map[*episode]int32{} // ticks an episode was blocked by a member of its own cohort
	for _, u := range r.Units {
		if !u.Ground {
			continue
		}
		out.Ground++
		kind := u.kind()
		life := r.Last
		if u.Died >= 0 {
			life = min(life, u.Died)
		}
		from := u.Born
		if u.Fin >= 0 {
			from = u.Fin
		}
		var moving, blocked int32
		for _, s := range u.Segs {
			if s.Moving {
				moving += s.T1 - s.T0
			}
		}
		for i := range u.Blocks {
			b := &u.Blocks[i]
			dur := b.T1 - b.T0
			blocked += dur
			rec := r.classifyBlock(u, b)
			where := "mid-route"
			switch {
			case rec.GoalDist >= 0 && rec.GoalDist <= nearGoal:
				where = "near-goal"
			case rec.StartDist >= 0 && rec.StartDist <= nearGoal:
				where = "near-start"
			}
			d := float64(dur)
			db := bucket(d, 1, 5, 15, 30, 60, 150, 300, 900)
			t.add(key("block", "class", rec.Class, "dur", db), d)
			t.add(key("block_class", "class", rec.Class), d)
			t.add(key("block_root", "class", rec.Class, "root", rec.Root), d)
			if rec.Class == "same-way" {
				t.add(key("block_chain", "root", rec.Root, "chain", bucket(float64(rec.Chain), 1, 2, 3, 5)), d)
			}
			t.add(key("block_where", "class", rec.Class, "where", where), d)
			t.add(key("block_kind", "kind", kind, "class", rec.Class), d)
			t.add(key("block_combat", "class", rec.Class, "combat", yes(rec.Combat)), d)
			t.add(key("block_crowd", "class", rec.Class, "near", bucket(float64(rec.NearMob), 0, 1, 3, 7, 15)), d)
			t.add(key("block_egress", "egress", yes(rec.Egress), "class", rec.Class), d)
			t.add(key("block_engine", "engine", eng, "class", rec.Class), d)
			if !rec.Carried {
				hk := [2]int32{int32(math.Floor(rec.X / heatCellSize)), int32(math.Floor(rec.Z / heatCellSize))}
				hc := heat[hk]
				if hc == nil {
					hc = &heatCell{X: hk[0], Z: hk[1], Ticks: map[string]int64{}}
					heat[hk] = hc
				}
				hc.Ticks[rec.Class] += int64(dur)
			}
			if grid != nil && u.Def != nil {
				if c := grid.clearanceAt(u.Def.Class, rec.X, rec.Z); c >= 0 {
					rec.Clearance = c
					t.add(key("block_clearance", "class", rec.Class, "cells", bucket(float64(c), 1, 2, 3, 5, 8, 16)), d)
					t.add(key("block_clearance_all", "cells", bucket(float64(c), 1, 2, 3, 5, 8, 16)), d)
				}
			}
			if rec.Blocker != nil {
				br := rec.Blocker
				t.add(key("block_state", "rel", br.Rel, "state", br.State), d)
				if rec.Class == "same-way" || rec.Class == "head-on" || rec.Class == "crossing" || rec.Class == "moving-friend" {
					t.add(key("block_cohort", "class", rec.Class, "same_cohort", yes(br.SameCohort)), d)
					t.add(key("block_slower", "class", rec.Class, "slower", yes(br.Slower)), d)
					t.add(key("block_held", "class", rec.Class, "blocker_held", yes(br.State == "held")), d)
				}
				if br.SameCohort && rec.Episode >= 0 {
					selfTicks[&u.Episodes[rec.Episode]] += min(b.T1, u.Episodes[rec.Episode].TEnd) - b.T0
				}
			}
			if dur >= opt.minBlock || opt.allRecords {
				out.Blocks = append(out.Blocks, rec)
			}
		}
		t.add(key("time", "kind", kind, "what", "moving"), float64(moving))
		t.add(key("time", "kind", kind, "what", "blocked"), float64(blocked))
		t.add(key("time_engine", "engine", eng, "what", "moving"), float64(moving))
		t.add(key("time_engine", "engine", eng, "what", "blocked"), float64(blocked))
		out.Moving += int64(moving)
		out.BlockedT += int64(blocked)
		if grid != nil && u.Def != nil {
			// Where units travel, by clearance, so blocked time per
			// clearance can be read against exposure.
			for ts := from; ts < life; ts += 30 {
				if x, z, mv, ok := u.pos(ts); ok && mv {
					if c := grid.clearanceAt(u.Def.Class, x, z); c >= 0 {
						t.add(key("moving_clearance", "cells", bucket(float64(c), 1, 2, 3, 5, 8, 16)), 30)
					}
				}
			}
		}
		if life > from {
			t.add(key("time", "kind", kind, "what", "alive"), float64(life-from))
		}
		for ei := range u.Episodes {
			e := &u.Episodes[ei]
			er := episodeRec{Rec: r.Key, Unit: u.Net, Name: u.Name, Kind: kind, Player: u.Player, Index: ei, VMax: u.vmax(), episodeOut: e.out()}
			dur := float64(e.TEnd - e.T0)
			if v := u.vmax(); v > 0 && e.Length > 0 {
				er.Ideal = e.Length / v
				er.Ratio = dur / er.Ideal
			}
			if e.HasGoal {
				if d0 := dist(e.X0, e.Z0, e.GoalX, e.GoalZ); d0 > 0 {
					er.Detour = e.Length / d0
				}
			}
			er.Combat = u.combat(e.T0, e.TEnd+1)
			er.Purpose = r.purpose(u, e)
			lb := bucket(e.Length, 100, 300, 1000, 3000)
			t.add(key("purpose", "purpose", er.Purpose, "end", e.End), dur)
			t.add(key("episode", "end", e.End, "len", lb), dur)
			t.add(key("episode_kind", "kind", kind, "end", e.End), dur)
			t.add(key("episode_blocked", "len", lb, "blocks", bucket(float64(e.Blocks), 0, 1, 3, 10, 30)), float64(e.BlockTicks))
			t.add(key("episode_blockticks", "len", lb), float64(e.BlockTicks))
			t.add(key("episode_length", "len", lb), e.Length)
			t.add(key("episode_routes", "len", lb, "routes", bucket(float64(e.Routes), 1, 2, 4, 8, 16)), float64(e.Routes))
			if e.TRoute >= 0 && e.HasGoal {
				t.add(key("latency", "ticks", bucket(float64(e.TRoute-e.T0), 1, 3, 10, 30, 90, 300)), float64(e.TRoute-e.T0))
				t.add(key("latency_engine", "engine", eng, "ticks", bucket(float64(e.TRoute-e.T0), 1, 3, 10, 30, 90, 300)), float64(e.TRoute-e.T0))
				// A goal installed within ten ticks of the unit's last
				// admitted route request keeps that stamp, and the
				// follower then waits out its sixty-tick throttle
				// [04 R-PATH-01 §8].
				quick := "n"
				if ei > 0 && e.T0-u.Episodes[ei-1].T0 <= 10 {
					quick = "y"
				}
				t.add(key("latency_reorder", "engine", eng, "reordered_within_10", quick, "ticks", bucket(float64(e.TRoute-e.T0), 1, 10, 30, 55, 65, 90, 300)), float64(e.TRoute-e.T0))
			}
			// Settling: from first coming within 160 world units of the
			// goal to the end of the episode, for plain moves that ended
			// there. In free flow that takes 160/vmax ticks.
			if er.Purpose == "move" && e.HasGoal && (e.End == "arrived" || e.End == "short") && e.Length >= 300 && u.vmax() > 0 &&
				dist(e.EndX, e.EndZ, e.GoalX, e.GoalZ) <= 160 {
				first := int32(-1)
				for ts := e.T0; ts <= e.TEnd; ts += 5 {
					if x, z, _, ok := u.pos(ts); ok && dist(x, z, e.GoalX, e.GoalZ) <= 160 {
						first = ts
						break
					}
				}
				if first >= 0 {
					settle := float64(e.TEnd - first)
					free := 160 / u.vmax()
					t.add(key("settle", "cohort", bucket(float64(cohortSize(cohorts, e.Cohort)), 1, 3, 7, 15, 31), "ratio", bucket(settle/free, 1.25, 2, 3, 5, 10)), settle)
					er.Settle = int32(settle)
				}
			}
			if grid != nil && u.Def != nil && detoursLeft > 0 && er.Purpose == "move" && e.End == "arrived" && e.Length >= 600 && er.Combat == 0 {
				if sp, ok := grid.shortest(u.Def.Class, e.X0, e.Z0, e.EndX, e.EndZ, 400000); ok && sp > 0 {
					detoursLeft--
					er.Shortest = sp
					t.add(key("detour_static", "len", lb, "ratio", bucket(e.Length/sp, 1.05, 1.15, 1.3, 1.5, 2, 3)), e.Length)
					t.add(key("detour_static_sum", "what", "travelled"), e.Length)
					t.add(key("detour_static_sum", "what", "shortest"), sp)
				}
			}
			if e.End == "arrived" && e.Length >= 300 && er.Ideal > 0 {
				calm := yes(er.Combat == 0)
				t.add(key("ratio", "calm", calm, "len", lb, "ratio", bucket(er.Ratio, 1.1, 1.25, 1.5, 2, 3, 5)), dur)
				t.add(key("detour", "calm", calm, "len", lb, "detour", bucket(er.Detour, 1.05, 1.2, 1.5, 2, 3)), e.Length)
				t.add(key("arrived_time", "calm", calm, "len", lb, "what", "actual"), dur)
				t.add(key("arrived_time", "calm", calm, "len", lb, "what", "ideal"), er.Ideal)
				t.add(key("arrived_time", "calm", calm, "len", lb, "what", "blocked"), float64(e.BlockTicks))
				t.add(key("arrived_time", "calm", calm, "len", lb, "what", "waiting"), float64(e.WaitTicks))
			}
			if e.End == "short" && e.HasGoal {
				t.add(key("short", "purpose", er.Purpose, "dist", bucket(dist(e.EndX, e.EndZ, e.GoalX, e.GoalZ), 64, 96, 160, 320, 640)), dur)
			}
			notable := er.Shortest > 0 || e.MaxBlock >= 150 || (e.End == "short" && e.Length >= 100) ||
				(e.TRoute >= 0 && e.TRoute-e.T0 >= 60) || (e.End == "arrived" && e.Length >= 300 && er.Ratio >= 2)
			if notable || opt.allRecords {
				out.Episodes = append(out.Episodes, er)
			}
		}
	}
	for ci := range cohorts {
		c := &cohorts[ci]
		var arr []int32
		var ideals []float64
		for _, u := range r.Units {
			for ei := range u.Episodes {
				e := &u.Episodes[ei]
				if e.Cohort != c.Index || !u.Ground {
					continue
				}
				m := cohortMember{Unit: u.Net, Name: u.Name, VMax: u.vmax(), Episode: ei, X0: e.X0, Z0: e.Z0, GoalX: e.Goal0X, GoalZ: e.Goal0Z,
					End: e.End, TEnd: e.TEnd, Length: e.Length, Blocks: e.Blocks, BlockTicks: e.BlockTicks, WaitTicks: e.WaitTicks,
					SelfTicks: selfTicks[e]}
				if m.VMax > 0 {
					m.Ideal = e.Length / m.VMax
				}
				c.Members = append(c.Members, m)
				c.BlockTicks += e.BlockTicks
				c.SelfTicks += m.SelfTicks
				c.WaitTicks += e.WaitTicks
				c.Combat += u.combat(e.T0, e.TEnd+1)
				switch e.End {
				case "arrived":
					c.Arrived++
					arr = append(arr, e.TEnd-e.T0)
					ideals = append(ideals, m.Ideal)
				case "short":
					c.Short++
				case "superseded":
					c.Superseded++
				case "died":
					c.Died++
				}
			}
		}
		sort.Slice(c.Members, func(i, j int) bool { return c.Members[i].Unit < c.Members[j].Unit })
		if len(arr) > 0 {
			sort.Slice(arr, func(i, j int) bool { return arr[i] < arr[j] })
			sort.Float64s(ideals)
			c.FirstArr, c.LastArr, c.MedianArr = arr[0], arr[len(arr)-1], arr[len(arr)/2]
			c.IdealMedian = ideals[len(ideals)/2]
		}
		nb := bucket(float64(c.N), 2, 4, 8, 16, 32, 64)
		t.add(key("cohort", "n", nb), float64(c.N))
		t.add(key("cohort_block", "n", nb, "what", "blocked"), float64(c.BlockTicks))
		t.add(key("cohort_block", "n", nb, "what", "self"), float64(c.SelfTicks))
		t.add(key("cohort_shared_goal", "n", nb, "shared", yes(c.SharedGoal > 0)), float64(c.SharedGoal))
		t.add(key("cohort_mixed", "n", nb, "mixed", yes(c.Mixed)), float64(c.BlockTicks))
		if c.N >= opt.minCohort || opt.allRecords {
			out.Cohorts = append(out.Cohorts, *c)
		}
	}
	keys := make([][2]int32, 0, len(heat))
	for k := range heat {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i][1] != keys[j][1] {
			return keys[i][1] < keys[j][1]
		}
		return keys[i][0] < keys[j][0]
	})
	for _, k := range keys {
		out.Heat = append(out.Heat, *heat[k])
	}
	return out
}

func cohortSize(cs []cohortRec, i int) int {
	if i < 0 || i >= len(cs) {
		return 1
	}
	return cs[i].N
}

func cmdMine(args []string) error {
	fs := flag.NewFlagSet("mine", flag.ExitOnError)
	home, _ := os.UserHomeDir()
	moves := fs.String("moves", filepath.Join(home, "nanolathe-bench/path-redesign/data/moves"), "directory of movement extracts, <source>/<id>.json.gz")
	units := fs.String("units", filepath.Join(home, "nanolathe-bench/path-redesign/data/units-ota.json"), "unit table for original TA recordings")
	unitsProTA := fs.String("units-prota", filepath.Join(home, "nanolathe-bench/path-redesign/data/units-prota.json"), "unit table for ProTA recordings")
	outDir := fs.String("out", "", "output directory (required)")
	workers := fs.Int("workers", max(1, runtime.NumCPU()/3), "parallel recordings")
	minBlock := fs.Int("min-block", 60, "shortest blocked span written in detail, ticks")
	minCohort := fs.Int("min-cohort", 3, "smallest group order written in detail")
	all := fs.Bool("all", false, "write every span, episode and group in detail")
	maps := fs.String("maps", filepath.Join(home, "nanolathe-bench/path-redesign/data/maps"), "directory of exported map grids, empty for none")
	detours := fs.Int("detours", 400, "episodes per recording measured against the static shortest path")
	only := fs.String("only", "", "substring a recording key must contain")
	fs.Parse(args)
	if *outDir == "" {
		return fmt.Errorf("mine: -out is required")
	}
	var tabs tables
	var err error
	if tabs.ota, err = loadUnitTable(*units); err != nil {
		return err
	}
	if *unitsProTA != "" {
		if tabs.prota, err = loadUnitTable(*unitsProTA); err != nil {
			return err
		}
	}
	var files []string
	for _, pat := range []string{"*/*.json.gz", "*/*.json", "*.json.gz", "*.json"} {
		m, _ := filepath.Glob(filepath.Join(*moves, pat))
		files = append(files, m...)
	}
	sort.Strings(files)
	opt := mineOptions{minBlock: int32(*minBlock), minCohort: *minCohort, allRecords: *all, maps: &mapStore{dir: *maps}, detours: *detours}
	type result struct {
		key  string
		m    *minedFile
		err  error
		skip bool
	}
	jobs := make(chan string)
	results := make(chan result)
	var wg sync.WaitGroup
	for w := 0; w < *workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for f := range jobs {
				r, err := loadRecording(f, tabs)
				if err != nil {
					results <- result{key: f, err: err, skip: strings.Contains(err.Error(), "carries no movement")}
					continue
				}
				m := mineRecording(r, opt)
				p := filepath.Join(*outDir, "mined", r.Key+".json.gz")
				err = os.MkdirAll(filepath.Dir(p), 0o755)
				if err == nil {
					err = writeJSONMaybeGz(p, m)
				}
				results <- result{key: r.Key, m: m, err: err}
			}
		}()
	}
	go func() {
		for _, f := range files {
			if *only != "" && !strings.Contains(f, *only) {
				continue
			}
			jobs <- f
		}
		close(jobs)
		wg.Wait()
		close(results)
	}()
	total := tally{}
	type row struct {
		Rec      string `json:"rec"`
		Map      string `json:"map"`
		LastTick int32  `json:"last_tick"`
		Ground   int    `json:"ground_units"`
		Blocks   int64  `json:"blocks"`
		Cohorts  int    `json:"cohorts"`
		Engine   string `json:"engine_kind"`
		Content  string `json:"content"`
		Players  int    `json:"combatants"`
		Moving   int64  `json:"moving_ticks"`
		Blocked  int64  `json:"blocked_ticks"`
		HasGrid  bool   `json:"has_grid"`
	}
	var rows []row
	skipped, failed := 0, 0
	for res := range results {
		switch {
		case res.skip:
			skipped++
		case res.err != nil:
			failed++
			fmt.Fprintln(os.Stderr, res.err)
		default:
			total.merge(res.m.Tally)
			var nb int64
			for _, k := range res.m.Tally.table("block_class") {
				nb += res.m.Tally[k].N
			}
			rows = append(rows, row{res.m.Rec, res.m.Map, res.m.LastTick, res.m.Ground, nb, len(res.m.Cohorts),
				res.m.EngineKind, res.m.Content, res.m.Combatants, res.m.Moving, res.m.BlockedT, opt.maps.grid(res.m.Map) != nil})
		}
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].Rec < rows[j].Rec })
	fmt.Fprintf(os.Stderr, "mined %d recordings (%d without movement, %d failed)\n", len(rows), skipped, failed)
	return writeJSONMaybeGz(filepath.Join(*outDir, "summary.json"), map[string]any{"recordings": rows, "tally": total})
}

// cmdShow prints one table of a summary.
func cmdShow(args []string) error {
	fs := flag.NewFlagSet("show", flag.ExitOnError)
	in := fs.String("in", "", "summary.json")
	fs.Parse(args)
	var s struct {
		Tally tally `json:"tally"`
	}
	if err := readJSONMaybeGz(*in, &s); err != nil {
		return err
	}
	names := fs.Args()
	if len(names) == 0 {
		seen := map[string]bool{}
		for k := range s.Tally {
			seen[strings.SplitN(k, "|", 2)[0]] = true
		}
		fmt.Println(strings.Join(sortedKeys(seen), " "))
		return nil
	}
	for _, name := range names {
		var tn int64
		var ts float64
		for _, k := range s.Tally.table(name) {
			tn += s.Tally[k].N
			ts += s.Tally[k].Sum
		}
		fmt.Printf("== %s: n=%d sum=%.0f\n", name, tn, ts)
		for _, k := range s.Tally.table(name) {
			c := s.Tally[k]
			fmt.Printf("  %-64s n=%9d (%5.1f%%)  sum=%13.0f (%5.1f%%)  mean=%8.1f\n", strings.TrimPrefix(k, name+"|"),
				c.N, 100*float64(c.N)/float64(max(tn, 1)), c.Sum, 100*c.Sum/nonzero(ts), c.Sum/float64(max(c.N, 1)))
		}
	}
	return nil
}

func nonzero(x float64) float64 {
	if x == 0 {
		return 1
	}
	return x
}
