package main

import (
	"fmt"
	"math"
	"sort"
	"strings"
)

// recording is one movement extract read into unit timelines.
type recording struct {
	File    *movesFile
	Key     string // "<source>/<id>"
	Units   []*unitTL
	ByNet   map[int]*unitTL // last unit to hold each net ID
	Last    int32
	Players int
	// Combatants is the number of players that own at least one unit.
	Combatants int
	// builtBy lists, per builder net ID, the units it started.
	builtBy map[int][]*unitTL
	idx     *spaceIndex
}

// tables chooses a unit table for a recording: ProTA's for a ProTA
// recording, the stock one otherwise.
type tables struct {
	ota, prota unitTable
}

func (t tables) pick(mf *movesFile) unitTable {
	if t.prota != nil && (strings.HasSuffix(strings.ToLower(mf.File), ".pro") || strings.HasPrefix(mf.Source, "v4.")) {
		return t.prota
	}
	return t.ota
}

func loadRecording(path string, tabs tables) (*recording, error) {
	var mf movesFile
	if err := readJSONMaybeGz(path, &mf); err != nil {
		return nil, err
	}
	defs := tabs.pick(&mf)
	if mf.Status != "moves" {
		return nil, fmt.Errorf("%s/%s: extract status %q carries no movement", mf.Source, mf.ID, mf.Status)
	}
	r := &recording{File: &mf, Key: mf.Source + "/" + mf.ID, ByNet: map[int]*unitTL{}, Last: int32(mf.LastTick), Players: len(mf.Players),
		builtBy: map[int][]*unitTL{}}
	for pi, p := range mf.Players {
		if len(p.Units) > 0 {
			r.Combatants++
		}
		for _, mu := range p.Units {
			u := &unitTL{Rec: r, Player: pi, Net: mu.NetID, Name: mu.Unit, Def: defs[mu.Unit],
				Born: int32(mu.Tick), Fin: -1, Died: -1, BX: float64(mu.X), BZ: float64(mu.Z),
				Hurt: mu.Hurt, Fire: mu.Fire, Builder: mu.Builder}
			if mu.Builder != 0 {
				r.builtBy[mu.Builder] = append(r.builtBy[mu.Builder], u)
			}
			if mu.FinTick != nil {
				u.Fin = int32(*mu.FinTick)
			}
			if mu.DiedTick != nil {
				u.Died = int32(*mu.DiedTick)
			}
			air := mu.Air != nil && *mu.Air
			if u.Def != nil && u.Def.Air {
				air = true
			}
			u.Ground = !air && len(mu.Path) > 0
			if u.Ground {
				u.Entries = make([]entry, 0, len(mu.Path))
				for _, row := range mu.Path {
					if len(row) < 3 {
						continue
					}
					e := entry{T: row[0], B: row[1] != 0, N: int8(row[2])}
					for k := 0; k < int(e.N) && 4+2*k < len(row); k++ {
						e.P[k] = [2]int32{row[3+2*k], row[4+2*k]}
					}
					// Entries arrive in capture order; a rewound sender tick
					// would break every search below, so it is dropped.
					if n := len(u.Entries); n > 0 && e.T < u.Entries[n-1].T {
						continue
					}
					u.Entries = append(u.Entries, e)
				}
				u.addPoses(mu.Pose, mu.Pos)
				// A log written before reversals were counted has five
				// numbers a sample.
				for _, m := range mu.Motion {
					var row [6]int32
					copy(row[:], m)
					u.Motion = append(u.Motion, row)
				}
				u.Goals = mu.Goals
			}
			u.build(r.Last)
			r.Units = append(r.Units, u)
			r.ByNet[u.Net] = u
		}
	}
	return r, nil
}

// footprint is the unit's footprint in world units, a default of two cells
// when its definition is unknown.
func (u *unitTL) footprint() (w, h float64) {
	if u.Def != nil && u.Def.FX > 0 && u.Def.FZ > 0 {
		return float64(u.Def.FX) * 16, float64(u.Def.FZ) * 16
	}
	return 32, 32
}

func (u *unitTL) mobile() bool {
	if u.Def != nil {
		return u.Def.Mobile
	}
	return len(u.Entries) > 0
}

// alive reports whether the unit exists as an obstacle at tick t: from its
// creation for a structure (a frame under construction blocks), from its
// completion for a mobile unit (a factory's product is inside the yard
// until then), to its death.
func (u *unitTL) alive(t int32) bool {
	from := u.Born
	if u.mobile() && u.Fin >= 0 {
		from = u.Fin
	}
	if t < from {
		return false
	}
	return u.Died < 0 || t < u.Died
}

// spaceIndex answers "which units are near this point at this tick". Units
// are filed by coarse cell per time slice at the slice's middle tick; a
// query reads the three-by-three cells around the point in the tick's slice
// and evaluates exact positions for those.
type spaceIndex struct {
	slice int32
	cell  float64
	// moving[slice][cellKey] -> unit indices filed from a moving estimate
	buckets []map[int64][]int32
	// static units (structures and never-moving units) filed once
	static map[int64][]int32
}

func cellKey(cx, cz int64) int64 { return cx<<32 ^ (cz & 0xffffffff) }

func (r *recording) index() *spaceIndex {
	if r.idx != nil {
		return r.idx
	}
	ix := &spaceIndex{slice: 64, cell: 256, static: map[int64][]int32{}}
	n := int(r.Last/ix.slice) + 2
	ix.buckets = make([]map[int64][]int32, n)
	for ui, u := range r.Units {
		if len(u.Entries) == 0 {
			// Never moved: one position for its whole life.
			k := cellKey(int64(math.Floor(u.BX/ix.cell)), int64(math.Floor(u.BZ/ix.cell)))
			ix.static[k] = append(ix.static[k], int32(ui))
			continue
		}
		from := u.Born
		if u.Fin >= 0 {
			from = u.Fin
		}
		to := r.Last
		if u.Died >= 0 {
			to = min(to, u.Died)
		}
		for s := from / ix.slice; s <= to/ix.slice && int(s) < n; s++ {
			mid := s*ix.slice + ix.slice/2
			x, z, _, _ := u.pos(mid)
			k := cellKey(int64(math.Floor(x/ix.cell)), int64(math.Floor(z/ix.cell)))
			if ix.buckets[s] == nil {
				ix.buckets[s] = map[int64][]int32{}
			}
			ix.buckets[s][k] = append(ix.buckets[s][k], int32(ui))
		}
	}
	r.idx = ix
	return ix
}

type neighbour struct {
	U      *unitTL
	X, Z   float64
	Moving bool
	Gap    float64 // separation of the two footprints' rectangles, world units
	Dist   float64 // centre distance
}

// near lists the units other than self whose footprint lies within reach of
// the point at tick t, nearest gap first. w, h is the asking unit's
// footprint.
func (r *recording) near(self *unitTL, t int32, x, z, w, h, reach float64) []neighbour {
	ix := r.index()
	var out []neighbour
	cx, cz := int64(math.Floor(x/ix.cell)), int64(math.Floor(z/ix.cell))
	visit := func(ids []int32) {
		for _, id := range ids {
			o := r.Units[id]
			if o == self || !o.alive(t) {
				continue
			}
			if o.Def != nil && o.Def.Air {
				continue
			}
			ox, oz, mv, _ := o.pos(t)
			ow, oh := o.footprint()
			gx := math.Abs(ox-x) - (w+ow)/2
			gz := math.Abs(oz-z) - (h+oh)/2
			gap := math.Max(math.Max(gx, gz), 0)
			if gap > reach {
				continue
			}
			out = append(out, neighbour{U: o, X: ox, Z: oz, Moving: mv, Gap: gap, Dist: dist(x, z, ox, oz)})
		}
	}
	s := int(t / ix.slice)
	for dx := int64(-1); dx <= 1; dx++ {
		for dz := int64(-1); dz <= 1; dz++ {
			k := cellKey(cx+dx, cz+dz)
			visit(ix.static[k])
			if s >= 0 && s < len(ix.buckets) && ix.buckets[s] != nil {
				visit(ix.buckets[s][k])
			}
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Gap != out[j].Gap {
			return out[i].Gap < out[j].Gap
		}
		return out[i].Dist < out[j].Dist
	})
	return out
}

// combatNear sums a unit's Hurt or Fire buckets within ticks of t.
func sumBuckets(rows [][2]int32, t0, t1 int32) int32 {
	i := sort.Search(len(rows), func(i int) bool { return rows[i][0]+combatBucket > t0 })
	var s int32
	for ; i < len(rows) && rows[i][0] < t1; i++ {
		s += rows[i][1]
	}
	return s
}

const combatBucket = 30
