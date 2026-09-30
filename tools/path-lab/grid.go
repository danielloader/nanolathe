package main

import (
	"container/heap"
	"math"
	"path/filepath"
	"strings"
	"sync"
)

// mapGrid is a map's static ground as cmd/nanolathe-pathlab exports it:
// heights and, per movement class, whether a footprint may stand anchored
// at each cell (0 blocked, 1 steep, 2 clear) [04 §6.1].
type mapGrid struct {
	Map      string              `json:"map"`
	CellW    int32               `json:"cell_w"`
	CellH    int32               `json:"cell_h"`
	Sea      uint8               `json:"sea"`
	Heights  []byte              `json:"heights"`
	Blocking []byte              `json:"blocking"`
	Classes  map[string][]byte   `json:"classes"`
	Foot     map[string][2]int32 `json:"foot"`
	ImageW   int                 `json:"image_w"`
	ImageH   int                 `json:"image_h"`

	mu        sync.Mutex
	clearance map[string][]uint8
}

// mapStore loads exported grids on demand, by map name.
type mapStore struct {
	dir string
	mu  sync.Mutex
	got map[string]*mapGrid
}

func mapDirName(name string) string {
	return strings.ReplaceAll(strings.ToLower(strings.TrimSpace(name)), " ", "_")
}

func (s *mapStore) grid(name string) *mapGrid {
	if s == nil || s.dir == "" {
		return nil
	}
	k := mapDirName(name)
	s.mu.Lock()
	defer s.mu.Unlock()
	if g, ok := s.got[k]; ok {
		return g
	}
	if s.got == nil {
		s.got = map[string]*mapGrid{}
	}
	var g mapGrid
	if err := readJSONMaybeGz(filepath.Join(s.dir, k, "grid.json.gz"), &g); err != nil {
		s.got[k] = nil
		return nil
	}
	g.clearance = map[string][]uint8{}
	s.got[k] = &g
	return &g
}

// layer is the class's passability, or the most common two-cell vehicle
// class when the unit's own is not in the grid.
func (g *mapGrid) layer(class string) ([]byte, string) {
	if l, ok := g.Classes[class]; ok {
		return l, class
	}
	for _, k := range []string{"TANKSH2", "TANKSH3", "KBOTSS2"} {
		if l, ok := g.Classes[k]; ok {
			return l, k
		}
	}
	return nil, ""
}

// clear is each anchor cell's distance, in cells and by the chessboard
// metric, to the nearest anchor the class cannot stand at, capped at 255.
// It is the half-width of the free ground around the cell: a unit in a
// corridor n cells wide reads about n/2.
func (g *mapGrid) clear(class string) []uint8 {
	l, k := g.layer(class)
	if l == nil {
		return nil
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	if c, ok := g.clearance[k]; ok {
		return c
	}
	w, h := int(g.CellW), int(g.CellH)
	d := make([]uint8, w*h)
	for i := range d {
		if l[i] != 0 {
			d[i] = 255
		}
	}
	// Two-pass chamfer for the chessboard distance; the map edge counts as
	// blocked.
	at := func(x, z int) uint8 {
		if x < 0 || z < 0 || x >= w || z >= h {
			return 0
		}
		return d[z*w+x]
	}
	relax := func(x, z int, n uint8) {
		if n < 255 && n+1 < d[z*w+x] {
			d[z*w+x] = n + 1
		}
	}
	for z := 0; z < h; z++ {
		for x := 0; x < w; x++ {
			if d[z*w+x] == 0 {
				continue
			}
			relax(x, z, at(x-1, z))
			relax(x, z, at(x, z-1))
			relax(x, z, at(x-1, z-1))
			relax(x, z, at(x+1, z-1))
		}
	}
	for z := h - 1; z >= 0; z-- {
		for x := w - 1; x >= 0; x-- {
			if d[z*w+x] == 0 {
				continue
			}
			relax(x, z, at(x+1, z))
			relax(x, z, at(x, z+1))
			relax(x, z, at(x+1, z+1))
			relax(x, z, at(x-1, z+1))
		}
	}
	g.clearance[k] = d
	return d
}

// anchor is the footprint anchor cell of a unit of the class centred at
// world x, z.
func (g *mapGrid) anchor(class string, x, z float64) (int, int) {
	f := g.Foot[class]
	if f[0] == 0 {
		f = [2]int32{2, 2}
	}
	return int(math.Floor((x - float64(f[0])*8) / 16)), int(math.Floor((z - float64(f[1])*8) / 16))
}

// clearanceAt is the class's clearance at a world position, -1 off the grid.
func (g *mapGrid) clearanceAt(class string, x, z float64) int {
	c := g.clear(class)
	if c == nil {
		return -1
	}
	_, k := g.layer(class)
	ax, az := g.anchor(k, x, z)
	if ax < 0 || az < 0 || ax >= int(g.CellW) || az >= int(g.CellH) {
		return -1
	}
	return int(c[az*int(g.CellW)+ax])
}

type pqItem struct {
	cell int32
	f    float64
}
type pq []pqItem

func (p pq) Len() int           { return len(p) }
func (p pq) Less(i, j int) bool { return p[i].f < p[j].f }
func (p pq) Swap(i, j int)      { p[i], p[j] = p[j], p[i] }
func (p *pq) Push(x any)        { *p = append(*p, x.(pqItem)) }
func (p *pq) Pop() any          { o := *p; n := len(o); x := o[n-1]; *p = o[:n-1]; return x }

// shortest is the length, in world units, of the shortest eight-connected
// path over the class's static layer between two world positions, and
// whether one was found within the expansion limit. It is a yardstick for
// the route a unit took, not the engine's search: unweighted, exact, and
// blind to structures and other units.
func (g *mapGrid) shortest(class string, x0, z0, x1, z1 float64, limit int) (float64, bool) {
	l, k := g.layer(class)
	if l == nil {
		return 0, false
	}
	w, h := int(g.CellW), int(g.CellH)
	sx, sz := g.anchor(k, x0, z0)
	gx, gz := g.anchor(k, x1, z1)
	in := func(x, z int) bool { return x >= 0 && z >= 0 && x < w && z < h }
	if !in(sx, sz) || !in(gx, gz) {
		return 0, false
	}
	// A goal on blocked ground is approached, not entered: accept the
	// nearest standable anchor within three cells of it.
	if l[gz*w+gx] == 0 {
		best, bx, bz := math.Inf(1), -1, -1
		for dz := -3; dz <= 3; dz++ {
			for dx := -3; dx <= 3; dx++ {
				if x, z := gx+dx, gz+dz; in(x, z) && l[z*w+x] != 0 {
					if d := math.Hypot(float64(dx), float64(dz)); d < best {
						best, bx, bz = d, x, z
					}
				}
			}
		}
		if bx < 0 {
			return 0, false
		}
		gx, gz = bx, bz
	}
	h2 := func(x, z int) float64 {
		dx, dz := math.Abs(float64(x-gx)), math.Abs(float64(z-gz))
		return 16 * (math.Max(dx, dz) + (math.Sqrt2-1)*math.Min(dx, dz))
	}
	dist := map[int32]float64{}
	start := int32(sz*w + sx)
	dist[start] = 0
	q := &pq{{start, h2(sx, sz)}}
	done := map[int32]bool{}
	n := 0
	for q.Len() > 0 {
		it := heap.Pop(q).(pqItem)
		if done[it.cell] {
			continue
		}
		done[it.cell] = true
		cx, cz := int(it.cell)%w, int(it.cell)/w
		if cx == gx && cz == gz {
			return dist[it.cell], true
		}
		if n++; n > limit {
			return 0, false
		}
		for dz := -1; dz <= 1; dz++ {
			for dx := -1; dx <= 1; dx++ {
				if dx == 0 && dz == 0 {
					continue
				}
				x, z := cx+dx, cz+dz
				if !in(x, z) || (l[z*w+x] == 0 && !(x == sx && z == sz)) {
					continue
				}
				if dx != 0 && dz != 0 && (l[cz*w+x] == 0 || l[z*w+cx] == 0) {
					continue // no cutting a blocked corner
				}
				c := int32(z*w + x)
				step := 16.0
				if dx != 0 && dz != 0 {
					step = 16 * math.Sqrt2
				}
				nd := dist[it.cell] + step
				if old, ok := dist[c]; !ok || nd < old {
					dist[c] = nd
					heap.Push(q, pqItem{c, nd + h2(x, z)})
				}
			}
		}
	}
	return 0, false
}
