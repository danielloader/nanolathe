package movement

import (
	"testing"

	"github.com/nanolathe-gg/nanolathe/internal/path"
)

// claimsFixture is the traffic fixture with both units on searched routes
// east along row ten: the mover from cell three, the friend from cell eight,
// both to cell seventeen. slow makes the friend a slow-turning unit.
func claimsFixture(t *testing.T, p ClaimsPilot, slow bool) (sys *System, cost func(h int, c Cell, dir uint8) int32) {
	t.Helper()
	sys, w, a, b, _ := trafficFixture(t, Traffic{Pilot: p}, 0)
	if slow {
		def := *w.Unit(b).Def
		def.TurnRate = 400
		w.Unit(b).Def = &def
	}
	anchor := func(c int32) Point { return Point{X: c*16 + 8, Z: 10*16 + 8} }
	handleRow(sys.Routes, a).PublishAtRevision([]Point{anchor(3), anchor(17)}, 0)
	handleRow(sys.Routes, b).PublishAtRevision([]Point{anchor(8), anchor(17)}, 0)
	sys.BeginTick(1)
	cost = func(h int, c Cell, dir uint8) int32 {
		cfg := path.SearchConfig{FootPrintX: 1, FootPrintZ: 1}
		req := path.Request{Unit: a}
		if h == 1 {
			req.Unit = b
		}
		p.Search(sys, req, &cfg)
		if cfg.CostDir == nil {
			return 0
		}
		return cfg.CostDir(path.Cell{X: c.X, Z: c.Z}, dir)
	}
	return sys, cost
}

// A search pays for the ground a friend's route crosses, and nothing for the
// ground its own route crosses or for ground no route crosses.
func TestClaimsChargesForAFriendsRoute(t *testing.T) {
	_, cost := claimsFixture(t, ClaimsPilot{}, false)
	for _, tc := range []struct {
		name string
		unit int
		cell Cell
		want int32
	}{
		{"the mover on the friend's route ahead of it", 0, Cell{X: 12, Z: 10}, 8},
		{"the mover on its own route alone", 0, Cell{X: 5, Z: 10}, 0},
		{"the friend on the mover's route behind it", 1, Cell{X: 5, Z: 10}, 8},
		{"the friend where both routes run", 1, Cell{X: 12, Z: 10}, 8},
		{"ground no route crosses", 0, Cell{X: 12, Z: 16}, 0},
	} {
		if got := cost(tc.unit, tc.cell, path.DirE); got != tc.want {
			t.Errorf("%s: cost %d, want %d", tc.name, got, tc.want)
		}
	}
}

// With Rank a slow-turning unit pays nothing for a nimble unit's route, and
// the nimble unit pays for the slow one's.
func TestClaimsRankExemptsSlowTurningUnits(t *testing.T) {
	_, cost := claimsFixture(t, ClaimsPilot{Rank: true}, true)
	if got := cost(1, Cell{X: 5, Z: 10}, path.DirE); got != 0 {
		t.Errorf("the slow-turning friend pays %d for the nimble mover's route, want 0", got)
	}
	if got := cost(0, Cell{X: 12, Z: 10}, path.DirE); got != 8 {
		t.Errorf("the nimble mover pays %d for the slow-turning friend's route, want 8", got)
	}
}

// The goal installer's straight line from where a unit stands to its goal
// claims nothing: it is not a way the unit will go.
func TestClaimsIgnoresTheStraightLineFallback(t *testing.T) {
	p := ClaimsPilot{}
	sys, _, a, b, _ := trafficFixture(t, Traffic{Pilot: p}, 0)
	handleRow(sys.Routes, b).PublishAtRevision([]Point{{X: 8*16 + 3, Z: 10*16 + 6}, {X: 17*16 + 1, Z: 10*16 + 6}}, 0)
	sys.BeginTick(1)
	cfg := path.SearchConfig{FootPrintX: 1, FootPrintZ: 1}
	p.Search(sys, path.Request{Unit: a}, &cfg)
	if cfg.CostDir != nil {
		if got := cfg.CostDir(path.Cell{X: 12, Z: 10}, path.DirE); got != 0 {
			t.Fatalf("a straight-line fallback was charged %d", got)
		}
	}
}

// With Oncoming a step against a friend's claim costs more than a step along
// it, and a step across it costs what a step along it does.
func TestClaimsChargeMoreAgainstTheWay(t *testing.T) {
	_, cost := claimsFixture(t, ClaimsPilot{Oncoming: 24}, false)
	for _, tc := range []struct {
		name string
		dir  uint8
		want int32
	}{
		{"along", path.DirE, 8},
		{"across", path.DirN, 8},
		{"against", path.DirW, 24},
		{"against at an angle", path.DirSW, 24},
	} {
		if got := cost(0, Cell{X: 12, Z: 10}, tc.dir); got != tc.want {
			t.Errorf("a step %s the friend's way costs %d, want %d", tc.name, got, tc.want)
		}
	}
}
