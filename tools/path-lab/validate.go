package main

import (
	"flag"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
)

// cmdValidate measures how well the reconstruction places a unit: it
// rebuilds every timeline without the scheduled statuses, asks it where the
// unit was at each status tick, and compares that with the exact position
// the status carries [fmt tad §7 "Scheduled status"].
func cmdValidate(args []string) error {
	fs := flag.NewFlagSet("validate", flag.ExitOnError)
	home, _ := os.UserHomeDir()
	moves := fs.String("moves", filepath.Join(home, "nanolathe-bench/path-redesign/data/moves"), "directory of movement extracts")
	units := fs.String("units", filepath.Join(home, "nanolathe-bench/path-redesign/data/units-ota.json"), "unit table")
	limit := fs.Int("limit", 12, "recordings to read")
	fs.Parse(args)
	defs, err := loadUnitTable(*units)
	if err != nil {
		return err
	}
	_ = defs
	files, _ := filepath.Glob(filepath.Join(*moves, "*/*.json.gz"))
	sort.Strings(files)
	var moving, idle, held []float64
	n := 0
	for _, f := range files {
		var mf movesFile
		if err := readJSONMaybeGz(f, &mf); err != nil || mf.Status != "moves" {
			continue
		}
		if n++; n > *limit {
			break
		}
		for _, p := range mf.Players {
			for _, mu := range p.Units {
				if len(mu.Path) == 0 || len(mu.Pose) == 0 {
					continue
				}
				if mu.Air != nil && *mu.Air {
					continue
				}
				u := &unitTL{Net: mu.NetID, Name: mu.Unit, Def: defs[mu.Unit], Born: int32(mu.Tick), Fin: -1, Died: -1, BX: float64(mu.X), BZ: float64(mu.Z)}
				if mu.FinTick != nil {
					u.Fin = int32(*mu.FinTick)
				}
				if mu.DiedTick != nil {
					u.Died = int32(*mu.DiedTick)
				}
				for _, row := range mu.Path {
					e := entry{T: row[0], B: row[1] != 0, N: int8(row[2])}
					for k := 0; k < int(e.N); k++ {
						e.P[k] = [2]int32{row[3+2*k], row[4+2*k]}
					}
					if m := len(u.Entries); m > 0 && e.T < u.Entries[m-1].T {
						continue
					}
					u.Entries = append(u.Entries, e)
				}
				u.build(int32(mf.LastTick))
				for _, ps := range mu.Pose {
					t := ps[0]
					if !u.alive(t) {
						continue
					}
					x, z, mv, ok := u.pos(t)
					if !ok {
						continue
					}
					d := dist(x, z, float64(ps[1])/65536, float64(ps[2])/65536)
					switch {
					case !mv:
						idle = append(idle, d)
					case u.blockedAt(t):
						held = append(held, d)
					default:
						moving = append(moving, d)
					}
				}
			}
		}
	}
	report := func(name string, v []float64) {
		if len(v) == 0 {
			return
		}
		sort.Float64s(v)
		q := func(p float64) float64 { return v[int(math.Round(p*float64(len(v)-1)))] }
		fmt.Printf("%-8s n=%7d  p50=%6.1f p75=%6.1f p90=%6.1f p95=%6.1f p99=%7.1f max=%8.1f  within16=%.1f%% within32=%.1f%%\n", name, len(v),
			q(.5), q(.75), q(.9), q(.95), q(.99), v[len(v)-1],
			100*float64(sort.SearchFloat64s(v, 16))/float64(len(v)), 100*float64(sort.SearchFloat64s(v, 32))/float64(len(v)))
	}
	fmt.Printf("position error against scheduled statuses, world units (%d recordings)\n", min(n, *limit))
	report("idle", idle)
	report("moving", moving)
	report("held", held)
	return nil
}
