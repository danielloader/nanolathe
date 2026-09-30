package path

import "testing"

// twoGaps is a wall across column ten with a gap on the direct line (row
// five) and another four rows along.
func twoGaps(c Cell) uint8 {
	if c.X == 10 && c.Z != 5 && c.Z != 9 {
		return 0
	}
	return 3
}

// crossing is the row at which a route crosses the column, in cells.
func crossing(t *testing.T, points []Point, column int32) int32 {
	t.Helper()
	x := column * 16
	for i := 0; i+1 < len(points); i++ {
		a, b := points[i], points[i+1]
		if (a.X <= x) == (b.X <= x) && a.X != x {
			continue
		}
		if b.X == a.X {
			return a.Z / 16
		}
		return (a.Z + (b.Z-a.Z)*(x-a.X)/(b.X-a.X)) / 16
	}
	t.Fatalf("route %v never crosses column %d", points, column)
	return 0
}

// An extra cost on the anchors of the nearer gap sends the route through the
// farther one; without it, and with a cost of nothing, the route takes the
// nearer. The search is the retail one, which reads the cost only when a
// request carries it.
func TestExtraCostTurnsARouteAside(t *testing.T) {
	cfg := SearchConfig{
		Start:         Cell{0, 5},
		StartDir:      DirE,
		Goal:          PointGoal(Cell{20, 5}, 0),
		Scale:         0x18000,
		HasBounds:     true,
		Bounds:        Rect{Min: Cell{0, 0}, Max: Cell{20, 12}},
		PassableValue: twoGaps,
	}
	plain := RunSearch(cfg)
	if got := crossing(t, plain.Points, 10); got != 5 {
		t.Fatalf("without a cost the route crosses at row %d, want 5: %v", got, plain.Points)
	}
	cfg.CostDir = func(Cell, uint8) int32 { return 0 }
	free := RunSearch(cfg)
	if len(free.Points) != len(plain.Points) || free.Popped != plain.Popped {
		t.Fatalf("a cost of nothing changed the search: %v (%d pops), want %v (%d pops)", free.Points, free.Popped, plain.Points, plain.Popped)
	}
	for i := range plain.Points {
		if free.Points[i] != plain.Points[i] {
			t.Fatalf("a cost of nothing changed the route: %v, want %v", free.Points, plain.Points)
		}
	}
	cfg.CostDir = func(c Cell, _ uint8) int32 {
		if c.X >= 8 && c.X <= 12 && c.Z >= 4 && c.Z <= 6 {
			return 48
		}
		return 0
	}
	dear := RunSearch(cfg)
	if got := crossing(t, dear.Points, 10); got != 9 {
		t.Fatalf("with the near gap dear the route crosses at row %d, want 9: %v", got, dear.Points)
	}
}

// The pass over a finished route takes no leg across ground dearer than the
// dearest the route itself crosses.
func TestSmoothKeepsToGroundTheSearchAccepted(t *testing.T) {
	route := routeCells(Cell{0, 0}, Cell{10, 10}, Cell{30, 10})
	for _, tc := range []struct {
		name   string
		dear   Cell // on the straight leg from the first point to the last
		paid   Cell // on the route, as dear
		points int
	}{
		{"a leg over dearer ground is refused", Cell{15, 5}, Cell{-1, -1}, 3},
		{"a leg over ground as dear as the stretch it replaces is taken", Cell{15, 5}, Cell{20, 10}, 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := &smoothSearch{Search: &cannedSearch{route: route}, cfg: SearchConfig{FootPrintX: 1, FootPrintZ: 1,
				PassableValue: func(Cell) uint8 { return 3 },
				CostDir: func(c Cell, _ uint8) int32 {
					if c == tc.dear || c == tc.paid {
						return 16
					}
					return 0
				},
			}}
			s.Resume(100)
			if out, _, _ := s.Resume(100); len(out) != tc.points {
				t.Fatalf("smoothed to %v, want %d points", out, tc.points)
			}
		})
	}
}

// Octant names the sector nearest a direction, in the search's numbering.
func TestOctant(t *testing.T) {
	for _, tc := range []struct {
		dx, dz int64
		want   uint8
	}{
		{0, -5, DirN}, {-5, -5, DirNW}, {-5, 0, DirW}, {-5, 5, DirSW}, {0, 5, DirS}, {5, 5, DirSE}, {5, 0, DirE}, {5, -5, DirNE},
		{2, -5, DirN}, {3, -5, DirNE}, {-100, 41, DirW}, {-100, 42, DirSW}, {0, 0, DirNone},
	} {
		if got := Octant(tc.dx, tc.dz); got != tc.want {
			t.Errorf("Octant(%d, %d) = %d, want %d", tc.dx, tc.dz, got, tc.want)
		}
	}
	for d := uint8(0); d < 8; d++ {
		if got := Octant(int64(dirDelta[d].X), int64(dirDelta[d].Z)); got != d {
			t.Errorf("Octant of sector %d's own step = %d", d, got)
		}
	}
}
