package main

import (
	"flag"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// pick chooses snippets from mined recordings: hard cases by recorded
// difficulty and comparison samples by lottery, spread over recordings and
// maps, on maps whose grid is exported (so the engine can replay them).

type pickCandidate struct {
	rec    string
	mapN   string
	class  string
	weight float64
	t0, t1 int32
	cohort int   // cohort index, -1 for none
	units  []int // subject net IDs
	x, z   float64
	note   string
}

func cmdPick(args []string) error {
	fs := flag.NewFlagSet("pick", flag.ExitOnError)
	home, _ := os.UserHomeDir()
	mined := fs.String("mined", "", "directory mine wrote (required)")
	moves := fs.String("moves", filepath.Join(home, "nanolathe-bench/path-redesign/data/moves"), "directory of movement extracts")
	units := fs.String("units", home+"/nanolathe-bench/path-redesign/data/units-ota.json", "unit table for original TA")
	unitsProTA := fs.String("units-prota", home+"/nanolathe-bench/path-redesign/data/units-prota.json", "unit table for ProTA")
	maps := fs.String("maps", filepath.Join(home, "nanolathe-bench/path-redesign/data/maps"), "directory of exported map grids")
	out := fs.String("out", "", "snippet output directory (required)")
	perClass := fs.Int("per-class", 12, "snippets per class")
	perRec := fs.Int("per-recording", 2, "snippets of one class from one recording")
	maxTicks := fs.Int("max-ticks", 3600, "longest window")
	maxUnits := fs.Int("max-units", 400, "most staged units")
	fs.Parse(args)
	if *mined == "" || *out == "" {
		return fmt.Errorf("pick: -mined and -out are required")
	}
	store := &mapStore{dir: *maps}
	files, _ := filepath.Glob(filepath.Join(*mined, "mined", "*", "*.json.gz"))
	sort.Strings(files)
	var cands []pickCandidate
	for _, f := range files {
		var m minedFile
		if err := readJSONMaybeGz(f, &m); err != nil {
			return err
		}
		if store.grid(m.Map) == nil || m.Engine != nil {
			continue
		}
		cands = append(cands, candidatesOf(&m, int32(*maxTicks))...)
	}
	byClass := map[string][]pickCandidate{}
	for _, c := range cands {
		byClass[c.class] = append(byClass[c.class], c)
	}
	var tabs tables
	var err error
	if tabs.ota, err = loadUnitTable(*units); err != nil {
		return err
	}
	if tabs.prota, err = loadUnitTable(*unitsProTA); err != nil {
		return err
	}
	if err := os.MkdirAll(*out, 0o755); err != nil {
		return err
	}
	var chosen []pickCandidate
	for _, class := range sortedKeys(byClass) {
		cs := byClass[class]
		sort.SliceStable(cs, func(i, j int) bool { return cs[i].weight > cs[j].weight })
		perR, perM := map[string]int{}, map[string]int{}
		n := 0
		// Two passes: first at most three per map, then fill.
		for pass := 0; pass < 2 && n < *perClass; pass++ {
			for _, c := range cs {
				if n >= *perClass {
					break
				}
				if perR[c.rec] >= *perRec || (pass == 0 && perM[c.mapN] >= 3) {
					continue
				}
				dup := false
				for _, o := range chosen {
					if o.rec == c.rec && o.class == c.class && o.t0 < c.t1 && c.t0 < o.t1 {
						dup = true
					}
				}
				if dup {
					continue
				}
				perR[c.rec]++
				perM[c.mapN]++
				n++
				chosen = append(chosen, c)
			}
		}
	}
	byRec := map[string][]pickCandidate{}
	for _, c := range chosen {
		byRec[c.rec] = append(byRec[c.rec], c)
	}
	written := map[string]int{}
	for _, rec := range sortedKeys(byRec) {
		r, err := loadRecording(filepath.Join(*moves, rec+".json.gz"), tabs)
		if err != nil {
			return err
		}
		var cohorts []cohortRec
		for i, c := range byRec[rec] {
			spec := snippetSpec{class: c.class, note: c.note, t0: c.t0, t1: c.t1, margin: 192, subjects: map[*unitTL]bool{}}
			if c.cohort >= 0 {
				if cohorts == nil {
					cohorts = r.cohorts()
				}
				for _, u := range r.Units {
					for ei := range u.Episodes {
						if u.Episodes[ei].Cohort == c.cohort {
							spec.subjects[u] = true
						}
					}
				}
			}
			for _, id := range c.units {
				for _, u := range r.Units {
					if u.Net == id && u.Ground && u.alive(c.t0+1) {
						spec.subjects[u] = true
					}
				}
			}
			if len(spec.subjects) == 0 {
				continue
			}
			spec.region = r.bounds(spec.subjects, spec.t0, spec.t1)
			spec.id = fmt.Sprintf("%s-%s-%d-%d", c.class, strings.ReplaceAll(rec, "/", "-"), c.t0, i)
			s := r.cut(spec)
			if len(s.Units) > *maxUnits {
				continue
			}
			if err := writeJSONMaybeGz(filepath.Join(*out, spec.id+".json"), s); err != nil {
				return err
			}
			written[c.class]++
		}
	}
	for _, k := range sortedKeys(written) {
		fmt.Printf("%-14s %3d snippets (of %d candidates)\n", k, written[k], len(byClass[k]))
	}
	return nil
}

// lottery is a fixed pseudo-random weight in [0, 1) for a candidate, so a
// random class is the same draw on every run.
func lottery(rec string, n int) float64 {
	h := uint64(1469598103934665603)
	for i := 0; i < len(rec); i++ {
		h = (h ^ uint64(rec[i])) * 1099511628211
	}
	h = (h ^ uint64(uint32(n))) * 1099511628211
	h ^= h >> 29
	h *= 0xbf58476d1ce4e5b9
	h ^= h >> 32
	return float64(h>>11) / float64(1<<53)
}

// sampleWindow observes a fixed number of ticks beginning just before the
// command, capped by the recording's end when known. Neither admission nor
// the observation window depends on whether or when a unit arrived.
func sampleWindow(tick, lastTick, maxTicks int32) (int32, int32, bool) {
	from := tick - 1
	to := from + maxTicks
	if lastTick > 0 {
		to = min(to, lastTick)
	}
	return from, to, maxTicks > 0 && to > from
}

func candidatesOf(m *minedFile, maxTicks int32) []pickCandidate {
	var out []pickCandidate
	for i := range m.Cohorts {
		c := &m.Cohorts[i]
		t0, t1, sample := sampleWindow(c.T0, m.LastTick, maxTicks)
		// Large groups however the recorded game went for them — under fire,
		// with losses, arriving or not. A replay runs without combat, so
		// such a snippet compares rule sets on a real army in a real place
		// and says nothing about the recording.
		if c.N >= 32 && c.Distance >= 300 && sample {
			out = append(out, pickCandidate{rec: m.Rec, mapN: m.Map, class: "army", weight: lottery(m.Rec, c.Index+104729),
				t0: t0, t1: t1, cohort: c.Index,
				note: fmt.Sprintf("group order of %d (%s), %.0f world units, drawn at random whatever became of it", c.N, c.Kinds, c.Distance)})
		}
		if c.N < 6 || c.Distance < 300 || c.Combat > int32(c.N)*3 || c.Died > 0 {
			continue
		}
		// Calm recording comparisons retain the combat and loss gates,
		// but do not select on the recorded arrival share or time.
		if sample {
			out = append(out, pickCandidate{rec: m.Rec, mapN: m.Map, class: "group-random", weight: lottery(m.Rec, c.Index),
				t0: t0, t1: t1, cohort: c.Index,
				note: fmt.Sprintf("group order of %d (%s), %.0f world units, drawn at random", c.N, c.Kinds, c.Distance)})
			if c.N >= 32 {
				out = append(out, pickCandidate{rec: m.Rec, mapN: m.Map, class: "group-large", weight: lottery(m.Rec, c.Index+7919),
					t0: t0, t1: t1, cohort: c.Index,
					note: fmt.Sprintf("group order of %d (%s), %.0f world units, drawn at random", c.N, c.Kinds, c.Distance)})
			}
		}
		// Deliberately selected hard cases keep their original filters and
		// observation window, including at least 60% recorded arrival.
		if c.Arrived*10 < c.N*6 {
			continue
		}
		span := c.LastArr + 60
		if span <= 0 || span > maxTicks {
			continue
		}
		perUnit := float64(c.BlockTicks) / float64(c.N)
		class := "group"
		if c.SharedGoal*2 >= c.N {
			class = "group-shared"
		}
		if c.Mixed {
			class = "group-mixed"
		}
		// Weight by blocked time per unit, tempered so huge groups do not
		// take every place.
		out = append(out, pickCandidate{rec: m.Rec, mapN: m.Map, class: class, weight: perUnit * math.Sqrt(float64(c.N)),
			t0: c.T0 - 1, t1: c.T0 + span, cohort: c.Index,
			note: fmt.Sprintf("group order of %d (%s), %.0f world units, recorded blocked %.0f ticks per unit", c.N, c.Kinds, c.Distance, perUnit)})
	}
	epOf := map[[2]int]*episodeRec{}
	for i := range m.Episodes {
		e := &m.Episodes[i]
		epOf[[2]int{e.Unit, int(e.T0)}] = e
	}
	for i := range m.Blocks {
		b := &m.Blocks[i]
		if b.Combat || b.Carried || b.Dur < 90 || b.Episode < 0 {
			continue
		}
		class := ""
		switch b.Class {
		case "head-on", "crossing", "parked-friend", "structure", "static":
			class = b.Class
		default:
			continue
		}
		if b.Clearance > 0 && b.Clearance <= 2 && (b.Class == "head-on" || b.Class == "crossing" || b.Class == "same-way") {
			class = "choke"
		}
		c := pickCandidate{rec: m.Rec, mapN: m.Map, class: class, weight: float64(min(b.Dur, 600)), cohort: -1, units: []int{b.Unit}, x: b.X, z: b.Z,
			t0: b.T0 - 240, t1: b.T0 + min(b.Dur, 900) + 450,
			note: fmt.Sprintf("%s held %d ticks by %s", b.Name, b.Dur, b.Class)}
		if b.Blocker != nil && b.Blocker.State != "structure" && b.Blocker.State != "frame" {
			c.units = append(c.units, b.Blocker.Unit)
			c.note += " " + b.Blocker.Name
		}
		if c.t1-c.t0 > maxTicks {
			continue
		}
		out = append(out, c)
	}
	for i := range m.Episodes {
		e := &m.Episodes[i]
		distance := dist(e.X0, e.Z0, e.GoalX, e.GoalZ)
		if e.Purpose == "move" && e.HasGoal && distance >= 800 && e.Combat == 0 && e.End != "died" && e.Cohort < 0 {
			if t0, t1, ok := sampleWindow(e.T0, m.LastTick, maxTicks); ok {
				out = append(out, pickCandidate{rec: m.Rec, mapN: m.Map, class: "trip-random", weight: lottery(m.Rec, e.Unit*131+int(e.T0)), cohort: -1, units: []int{e.Unit},
					t0: t0, t1: t1,
					note: fmt.Sprintf("%s alone toward a goal %.0f world units away, drawn at random", e.Name, distance)})
			}
		}
		if e.Shortest > 0 && e.Length/e.Shortest >= 1.5 && e.TEnd-e.T0 <= maxTicks && e.Combat == 0 {
			out = append(out, pickCandidate{rec: m.Rec, mapN: m.Map, class: "detour", weight: e.Length - e.Shortest, cohort: -1, units: []int{e.Unit},
				t0: e.T0 - 1, t1: e.TEnd + 60,
				note: fmt.Sprintf("%s travelled %.0f where the static shortest path is %.0f", e.Name, e.Length, e.Shortest)})
		}
	}
	return out
}
