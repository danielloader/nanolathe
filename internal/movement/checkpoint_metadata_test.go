package movement

import (
	"math"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/nanolathe-gg/nanolathe/internal/path"
	"github.com/nanolathe-gg/nanolathe/internal/pool"
)

// The production boundary must keep admission operands across budget slices,
// and replace them with the request [04 R-PATH-01 §4, §6]. No callback is run
// to inspect metadata (DESIGN_MULTIPLAYER §16.3.5–§16.3.6).
func TestCheckpointMetadataWorkingSetLifecycle(t *testing.T) {
	for _, kernel := range []path.Kernel{nil, path.RetailKernel{}, path.StraightenKernel{}, path.SmoothKernel{}} {
		t.Run(checkpointKernelName(kernel), func(t *testing.T) {
			s, u, _, _, req := newGroundPathStatusFixture(t, path.Cell{X: 2, Z: 2}, path.Cell{X: 18, Z: 18})
			s.Kernel, s.tick = kernel, 27
			profile := *s.profiles[u.Handle]
			if work := s.searchFunc(req, 65536, 0); work.Done {
				t.Fatalf("fixture did not suspend: %+v", work)
			}
			first := s.sessions[u.Handle]
			roots := first.checkpoint
			if roots == nil || roots.unsupported != "" || roots.cost != nil || roots.leg != nil {
				t.Fatalf("retail roots = %+v", roots)
			}
			wantProfile := [8]int32{int32(profile.FootPrintX), int32(profile.FootPrintZ), profile.MaxWaterDepth, profile.MinWaterDepth,
				int32(profile.MaxSlope), int32(profile.BadSlope), int32(profile.MaxWaterSlope), int32(profile.BadWaterSlope)}
			want := path.CheckpointAccessor{Kind: 1, Requester: uint32(u.Handle), Owner: u.Owner, Profile: wantProfile,
				Tick: 27, FootprintX: int32(profile.FootPrintX), FootprintZ: int32(profile.FootPrintZ)}
			if !reflect.DeepEqual(roots.passable.value, want) {
				t.Fatalf("class operands = %+v, want %+v", roots.passable.value, want)
			}
			rev := roots.revise
			if rev.value.Kind != 8 || rev.value.Profile != wantProfile || rev.value.Requester != uint32(u.Handle) || rev.value.Tick != 27 ||
				rev.registry != s.layerRegistry || rev.layer != roots.passable.layer || rev.class != s.classKeyFor(u.Handle) {
				t.Fatalf("revision operands = %+v", rev)
			}

			// A later tick's live unit/profile are not the suspended captures.
			u.Owner, s.tick = 3, 91
			s.profiles[u.Handle].MaxSlope = 7
			s.Collisions[u.Handle].CachedAnchor = Cell{X: 3, Z: 3}
			if work := s.searchFunc(req, 1, 0); work.Done || s.sessions[u.Handle] != first || first.checkpoint != roots {
				t.Fatalf("resumption replaced metadata or finished: %+v", work)
			}
			if !reflect.DeepEqual(roots.passable.value, want) || rev.value.Tick != 27 {
				t.Fatal("resumption re-read admission operands")
			}
			*s.profiles[u.Handle], u.Owner = profile, 0
			req.Activation++
			if work := s.searchFunc(req, 65536, 0); work.Done {
				t.Fatalf("replacement did not suspend: %+v", work)
			}
			replacement := s.sessions[u.Handle]
			if replacement == first || replacement.checkpoint == roots || replacement.checkpoint.revise.value.Tick != 91 {
				t.Fatal("new activation retained the old metadata")
			}
			s.dropPathSession(int(u.Handle))
			if s.sessions[u.Handle] != nil || s.checkpointSearch != nil {
				t.Fatal("dropped working set left metadata rooted on System")
			}
		})
	}
}

func checkpointKernelName(k path.Kernel) string {
	if k == nil {
		return "nil"
	}
	return reflect.TypeOf(k).String()
}

// This kernel keeps the real cfg without reading its callbacks. It lets each
// construction branch be inspected even when a real setup would finish at once.
// It must remain explicitly unsupported by checkpoint metadata.
type checkpointHoldingKernel struct{ opened int }

func (k *checkpointHoldingKernel) NewSession(cfg path.SearchConfig) path.Search {
	k.opened++
	return &checkpointHoldingSearch{cfg: cfg}
}

type checkpointHoldingSearch struct{ cfg path.SearchConfig }

func (*checkpointHoldingSearch) Resume(int) ([]path.Point, path.Status, bool) { return nil, 0, false }
func (*checkpointHoldingSearch) Notified() path.Status                        { return 0 }
func (*checkpointHoldingSearch) SetupSteps() int                              { return 0 }
func (*checkpointHoldingSearch) Popped() int                                  { return 0 }
func (s *checkpointHoldingSearch) Start() path.Cell                           { return s.cfg.Start }
func (s *checkpointHoldingSearch) Config() path.SearchConfig                  { return s.cfg }
func (*checkpointHoldingSearch) Release()                                     {}

type checkpointOperandRules struct {
	StrictRules
	learned []*LearnedTerrain
	calls   int
	jam     bool
	wedge   bool
}

func (r *checkpointOperandRules) LearnedTerrain(*System) *LearnedTerrain {
	v := r.learned[r.calls]
	r.calls++
	return v
}
func (r *checkpointOperandRules) JamRelease(*System) (uint16, uint32) {
	if r.jam {
		return 1, 10
	}
	return 0, 0
}
func (r *checkpointOperandRules) WedgeEscape(*System) bool { return r.wedge }

func checkpointChain(t *testing.T, n *checkpointAccessorCapture, kinds ...uint8) []*checkpointAccessorCapture {
	t.Helper()
	var chain []*checkpointAccessorCapture
	for _, kind := range kinds {
		if n == nil || n.value.Kind != kind {
			t.Fatalf("chain node = %+v, want kind %d", n, kind)
		}
		chain = append(chain, n)
		if len(n.inputs) == 0 {
			n = nil
		} else {
			n = n.inputs[0]
		}
	}
	if n != nil {
		t.Fatalf("unexpected remaining chain: %+v", n)
	}
	return chain
}

// Each closure captures its own LearnedTerrain answer. Through/jam replace the
// preceding passability view, the finishing leg stays independent, and wedge
// retains the last view plus its separate non-learned override.
func TestCheckpointMetadataViewReplacementAndWedge(t *testing.T) {
	for _, tc := range []struct {
		name            string
		through         uint8
		leg, jam, wedge bool
	}{
		{name: "learned"},
		{name: "leg", leg: true},
		{name: "through movers", through: 1, leg: true},
		{name: "through all friends", through: 7, leg: true},
		{name: "jam replaces through", through: 2, leg: true, jam: true},
		{name: "wedge class", wedge: true},
		{name: "wedge static", through: 2, leg: true, wedge: true},
		{name: "wedge jam", through: 1, leg: true, jam: true, wedge: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rules := &checkpointOperandRules{jam: tc.jam, wedge: tc.wedge,
				learned: []*LearnedTerrain{{}, {}, {}, {}}}
			s, _, h := wedgeFixture(t, rules, 6, 6, 7, 8)
			k := &checkpointHoldingKernel{}
			s.Kernel, s.tick = k, 17
			s.trafficNow = Traffic{LegsThroughMovers: tc.leg}
			setHandleRow(&s.jamReleases, h, jamRelease{until: 100})
			// No metadata operation may probe a composed view.
			for _, name := range s.layerRegistry.names {
				s.layerRegistry.byName[name].mapping = func(int32, int32) (uint16, bool) { panic("metadata called mapping") }
			}
			rules.calls = 0
			req := path.Request{Unit: h, Goal: path.PointGoal(path.Cell{X: 16, Z: 6}, 0)}
			s.searchUnder(req, 65536, 0, tc.through)
			roots := s.sessions[h].checkpoint
			if k.opened != 1 || !strings.Contains(roots.unsupported, "kernel") {
				t.Fatalf("custom kernel was not executed once and refused: %d, %q", k.opened, roots.unsupported)
			}
			calls := 1
			var legLearned *LearnedTerrain
			if tc.leg {
				legLearned = rules.learned[calls]
				calls++
			}
			finalLearned := rules.learned[0]
			if tc.through != 0 {
				finalLearned = rules.learned[calls]
				calls++
			}
			if tc.jam {
				finalLearned = rules.learned[calls]
				calls++
			}
			if rules.calls != calls {
				t.Fatalf("LearnedTerrain called %d times, want existing %d calls", rules.calls, calls)
			}
			view := roots.passable
			if tc.wedge {
				if view.value.Kind != 6 || len(view.inputs) != 2 || view.value.Start != (path.Cell{X: 6, Z: 6}) ||
					view.value.Bounds != (path.Rect{Min: path.Cell{X: 4, Z: 4}, Max: path.Cell{X: 8, Z: 8}}) ||
					view.value.FootprintX != 2 || view.value.FootprintZ != 2 {
					t.Fatalf("wedge operands = %+v", view)
				}
				if tc.through != 0 || tc.jam {
					override := checkpointChain(t, view.inputs[1], 5, 4, 1)
					if override[0].value.Wall != 1 || override[0].system != s {
						t.Fatal("wedge lost the captured wall predicate")
					}
				} else {
					checkpointChain(t, view.inputs[1], 1)
				}
				view = view.inputs[0]
			}
			var base *checkpointAccessorCapture
			switch {
			case tc.jam:
				chain := checkpointChain(t, view, 5, 4, 2, 1)
				if chain[0].value.Wall != 1 || chain[0].system != s || chain[1].layer != chain[3].layer || chain[2].learned != finalLearned {
					t.Fatal("jam did not replace the previous through/learned view")
				}
				base = chain[3]
			case tc.through != 0:
				chain := checkpointChain(t, view, 5, 3, 2, 1)
				wall := uint8(2)
				if tc.through >= 2 {
					wall = 1
				}
				if chain[0].value.Wall != wall || chain[0].system != s || chain[1].value.Through != tc.through || chain[2].learned != finalLearned {
					t.Fatal("through operands were reconstructed or replaced")
				}
				base = chain[3]
			default:
				chain := checkpointChain(t, view, 2, 1)
				if chain[0].learned != finalLearned {
					t.Fatal("lost learned pointer")
				}
				base = chain[1]
			}
			if tc.leg {
				leg := checkpointChain(t, roots.leg, 5, 3, 2, 1)
				if leg[0].value.Wall != 2 || leg[0].system != s || leg[1].value.Through != 1 || leg[2].learned != legLearned || leg[3] != base {
					t.Fatal("finishing leg did not retain its independent view")
				}
			} else if roots.leg != nil {
				t.Fatal("absent leg acquired metadata")
			}
			if base.value.Owner != 0 || base.value.Requester != uint32(h) || base.value.Tick != 17 || roots.revise.layer != base.layer {
				t.Fatal("class/revision lost common captured operands")
			}
			if roots.cost != nil || s.checkpointSearch != nil {
				t.Fatal("unexpected cost or scratch metadata")
			}
		})
	}
}

func TestCheckpointMetadataWedgeKeepsRawFootprintAndOverflow(t *testing.T) {
	n := checkpointWedgeCapture(nil, nil, Cell{X: math.MaxInt32, Z: math.MinInt32}, 1, 2, -3, 0)
	want := path.Rect{Min: path.Cell{X: math.MaxInt32 - 1, Z: math.MaxInt32 - 1}, Max: path.Cell{X: math.MinInt32, Z: math.MinInt32 + 2}}
	if n.value.Bounds != want || n.value.FootprintX != -3 || n.value.FootprintZ != 0 {
		t.Fatalf("strict boundary/raw footprint operands changed: %+v", n.value)
	}
}

func checkpointClaimFixture(t *testing.T, p Pilot) (*System, pool.Handle, pool.Handle) {
	t.Helper()
	s, _, a, b, _ := trafficFixture(t, Traffic{Pilot: p}, 0)
	point := func(x int32) Point { return Point{X: x*16 + 8, Z: 10*16 + 8} }
	s.Routes[a].PublishAtRevision([]Point{point(3), point(17)}, 0)
	s.Routes[b].PublishAtRevision([]Point{point(8), point(17)}, 0)
	s.BeginTick(1)
	return s, a, b
}

func checkpointClaimSearch(s *System, p ClaimsPilot, h pool.Handle) (*checkpointAccessorCapture, func(path.Cell, uint8) int32) {
	s.checkpointSearch = &checkpointAccessorRoots{}
	s.searchCfg = path.SearchConfig{FootPrintX: 0, FootPrintZ: -3}
	p.Search(s, path.Request{Unit: h}, &s.searchCfg)
	root, cost := s.checkpointSearch.cost, s.searchCfg.CostDir
	s.checkpointSearch, s.searchCfg = nil, path.SearchConfig{}
	return root, cost
}

// Count and own rows are independently aliased, live arrays. Resize must leave
// the earlier search attached to both old rows; serial wrap clears its existing
// own row in place (DESIGN_MULTIPLAYER §16.3.6; Modern route claims).
func TestCheckpointMetadataClaimRowsAndSerialWrap(t *testing.T) {
	p := ClaimsPilot{Oncoming: 24, Rank: true}
	s, a, _ := checkpointClaimFixture(t, p)
	st := s.PilotState.(*claimState)
	st.serial = 4
	first, cost := checkpointClaimSearch(s, p, a)
	g := st.grids[0]
	if first == nil || first.counts != g.checkpointAll || first.own != st.checkpointOwn || first.value.Row != 2 || first.value.Serial != 5 ||
		first.value.Per != 8 || first.value.Against != 24 || first.value.FootprintX != 1 || first.value.FootprintZ != 1 ||
		first.value.Width != st.w || first.value.Height != st.h {
		t.Fatalf("claim capture = %+v", first)
	}
	if first.counts.kind != 2 || first.own.kind != 1 || first.counts.own != nil || first.own.counts != nil ||
		&first.counts.counts[0] != &g.all[0] || &first.own.own[0] != &st.own[0] {
		t.Fatal("holders do not name the original backing rows")
	}
	at := path.Cell{X: 12, Z: 10}
	index := int32(5)*st.w + 6
	g.all[index] = [8]uint8{}
	g.all[index][path.DirE] = 4
	st.own[index] = 5<<3 | uint32(path.DirE)
	if got := cost(at, path.DirE); got != 24 {
		t.Fatalf("live row cost = %d, want 24", got)
	}

	st.own[0], st.serial = 99, (1<<29)-1
	wrapped, _ := checkpointClaimSearch(s, p, a)
	if wrapped.own != first.own || wrapped.counts != first.counts || wrapped.value.Serial != 1 || first.value.Serial != 5 || first.own.own[0] != 0 {
		t.Fatal("serial wrap replaced a holder, missed its clear, or rewrote a captured serial")
	}
	if got := cost(at, path.DirE); got != 32 {
		t.Fatalf("old closure did not see shared own-row wrap: %d", got)
	}

	// A slow requester chooses a different count row while sharing own.
	def := *s.world.Unit(a).Def
	def.TurnRate = 400
	s.world.Unit(a).Def = &def
	slow, _ := checkpointClaimSearch(s, p, a)
	if slow.value.Row != 3 || slow.counts != g.checkpointSlow || slow.counts == first.counts || slow.own != first.own {
		t.Fatal("slow counts and shared own row lost their independent identities")
	}

	oldCounts := slices.Clone(first.counts.counts)
	oldOwn := slices.Clone(first.own.own)
	s.Terrain = flatTerrain(t, 24, 24, 0)
	p.BeginTick(s, 4)
	if st.checkpointOwn == first.own || st.grids[0].checkpointAll == first.counts ||
		!slices.Equal(first.counts.counts, oldCounts) || !slices.Equal(first.own.own, oldOwn) {
		t.Fatal("resize replaced or cleared a suspended search's old rows")
	}
	first.counts.counts[index][path.DirE] = 2
	first.own.own[index] = 0
	if got := cost(at, path.DirE); got != 16 {
		t.Fatalf("old closure detached from its preserved rows: %d", got)
	}
	if st.grids[0].all[index][path.DirE] == 2 {
		t.Fatal("resized count row aliases the retired row")
	}
}

func TestCheckpointMetadataCompositePilotAndRefusals(t *testing.T) {
	p := Pilots{ClaimsPilot{Per: 3}, nil, ClaimsPilot{Per: 11}, ArrivePilot{}}
	s, a, _ := checkpointClaimFixture(t, p)
	state := s.PilotState.(*pilotsState)
	first, last := state[0].(*claimState), state[2].(*claimState)
	k := &checkpointHoldingKernel{}
	s.Kernel = k
	s.searchFunc(path.Request{Unit: a, Goal: path.PointGoal(path.Cell{X: 17, Z: 10}, 0)}, 65536, 0)
	roots := s.sessions[a].checkpoint
	if s.PilotState != state || s.checkpointSearch != nil || s.searchCfg.CostDir != nil || roots.cost == nil || roots.cost.value.Per != 11 ||
		roots.cost.counts != last.grids[0].checkpointAll || roots.cost.own != last.checkpointOwn || roots.cost.own == first.checkpointOwn ||
		!strings.Contains(roots.unsupported, "kernel") {
		t.Fatal("composite search lost the final cost writer, restored state, or cleared scratch")
	}
	for _, pilot := range []Pilot{nil, NoPilot{}, ClaimsPilot{}, ArrivePilot{}, p} {
		if !checkpointPilotSupported(pilot) {
			t.Fatalf("known pilot %T refused", pilot)
		}
	}
	for _, pilot := range []Pilot{&NoPilot{}, &ClaimsPilot{}, &ArrivePilot{}, &p, Pilots{checkpointCustomPilot{}}} {
		if checkpointPilotSupported(pilot) {
			t.Fatalf("unreviewed pilot %T accepted", pilot)
		}
	}

	for _, pilot := range []Pilot{checkpointCustomPilot{}, &NoPilot{}, Pilots{nil, checkpointCustomPilot{}}} {
		s, h := kernelFixture(t)
		s.trafficNow.Pilot = pilot
		req := path.Request{Unit: h, Goal: path.PointGoal(path.Cell{X: 18, Z: 18}, 0)}
		if work := s.searchFunc(req, 65536, 0); work.Done {
			t.Fatalf("fixture did not suspend: %+v", work)
		}
		if got := s.sessions[h].checkpoint.unsupported; !strings.Contains(got, "pilot") {
			t.Fatalf("pilot %T refusal = %q", pilot, got)
		}
		if s.checkpointSearch != nil {
			t.Fatal("unsupported pilot leaked scratch roots")
		}
	}

	s = NewSystem(nil, Template(), NewOccupancyGrid())
	s.Kernel = &checkpointHoldingKernel{}
	s.searchFunc(path.Request{Unit: 1, Goal: path.PointGoal(path.Cell{X: 5, Z: 5}, 0)}, 65536, 0)
	if r := s.sessions[1].checkpoint; r.passable != nil || r.revise != nil || !strings.Contains(r.unsupported, "no terrain") {
		t.Fatalf("no-terrain callback was given an invented descriptor: %+v", r)
	}
}

type checkpointCustomPilot struct{ NoPilot }

func (checkpointCustomPilot) Search(_ *System, _ path.Request, cfg *path.SearchConfig) {
	cfg.CostDir = func(path.Cell, uint8) int32 { return 1 }
}
