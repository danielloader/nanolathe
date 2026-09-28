//go:build retail

package session

import (
	"fmt"
	"strings"
	"testing"

	"github.com/nanolathe-gg/nanolathe/formats"
	"github.com/nanolathe-gg/nanolathe/internal/clock"
	"github.com/nanolathe-gg/nanolathe/internal/frame"
	"github.com/nanolathe-gg/nanolathe/internal/orders"
	"github.com/nanolathe-gg/nanolathe/internal/pool"
	"github.com/nanolathe-gg/nanolathe/internal/testsupport/retailcat"
	"github.com/nanolathe-gg/nanolathe/internal/units"
	"github.com/nanolathe-gg/nanolathe/internal/world"
)

// Nanolathe Modern policy: docs/DESIGN_MOVEMENT_PATH.md "Modern jam release",
// its work-site rule (issue #31). The fixture is authored here — flat
// terrain, nine retail construction kbots in a block west of a solar
// collector site, one building it and eight assisting — and says nothing
// about retail beyond the Strict run it compares.

const (
	workSiteX, workSiteZ = 48, 48 // the collector's site, in cells
	workSiteBuilders     = 9
	workSiteFollowTicks  = 900
)

// workSiteOrderTicks are the ticks the scene's first build order is issued on.
// Where the approach walks start depends on it: an order issued in the first
// sixty ticks waits out the path-request throttle and walks from tick sixty,
// a later one walks at once. The spread runs before, through and past that
// window; 67, 72, 79 and 85 are among the ticks at which the assisters' walks
// met so that one, jammed behind a builder at the site, was released and
// walked into it.
var workSiteOrderTicks = []int{10, 67, 72, 79, 85, 100, 150, 200}

type workSiteRun struct {
	complete int // tick the collector completed, 0 if it never did
	// intoWorker counts ticks before completion on which a builder overlapped
	// one already working at the site: parked, with its build or assist past
	// the approach. Two arriving builders passing through each other
	// head-on is allied pass-through (DESIGN_MOVEMENT_PATH "Modern allied
	// pass-through") and is counted only in overlapTicks.
	intoWorker   int
	overlapTicks int // ticks before completion on which any two builders overlapped
	leftInside   int // builder pairs overlapping on the tick the collector completed
	refused      int // follow-up builds refused as unreachable
	idle         int // builders that never left their post after the follow-up
}

// workSiteScene runs the fixture under the named rule set, issuing the first
// build on orderTick. On the tick after the collector completes every builder
// is ordered to build its own collector east of the site, and the scene runs
// workSiteFollowTicks more ticks.
func workSiteScene(t *testing.T, rules string, orderTick int) workSiteRun {
	t.Helper()
	cat, fs := retailcat.Shared(t)
	const w, h = 96, 96
	attrs := make([]formats.TNTAttribute, w*h)
	for i := range attrs {
		attrs[i] = formats.TNTAttribute{Height: 20, Feature: world.PlotFeatureNone}
	}
	ter := &world.Terrain{CellW: w, CellH: h, Plot: world.ExpandPlot(attrs, w, h), Version: world.VersionCanonical, WindMin: 100, WindMax: 200}
	if err := ter.ApplySchema(nil, 0); err != nil {
		t.Fatal(err)
	}
	base, ok := LookupRuleSet(rules)
	if !ok {
		t.Fatalf("unknown rule set %q", rules)
	}
	s := &Session{Gameplay: base.Base, Catalog: cat, World: ter, Mission: syntheticMission(), Clock: &clock.State{Requested: 10, Active: 10}, Snapshot: &frame.Buffer{}}
	if err := s.SetRules(rules); err != nil {
		t.Fatal(err)
	}
	s.EntryCommunity = s.Community
	var err error
	if s.Units, err = newBattleSlicedWorldWithCOBSized(cat, fs, 0, [pool.PlayerCount]uint32{}, 64); err != nil {
		t.Fatal(err)
	}
	s.Econ = economyForTest()
	for i := range s.Econ.Players {
		p := &s.Econ.Players[i]
		p.Exists, p.ControllerState, p.EndGameCountdown = true, 2, -1
		if i == 0 {
			p.ControllerState = 1
		}
	}
	s.Econ.SeedDeadlines(0)
	if err := createAndBindServicesForTest(t, s); err != nil {
		t.Fatal(err)
	}
	s.RegisterAll()
	s.State = StateBattle
	if err := s.SetRules(rules); err != nil {
		t.Fatal(err)
	}
	s.SeedSessionRNG(7, 11)
	// Resources are no part of the contract: the builders never wait on them.
	s.Econ.Players[0].Stock = [2]float32{1e6, 1e6}
	s.Econ.Players[0].Capacity = [2]float32{1e6, 1e6}
	solar, ok := cat.Unit("armsolar")
	if !ok {
		t.Skip("retail fixture unit armsolar is absent")
	}
	var bs []*units.Unit
	for i := int32(0); i < workSiteBuilders; i++ {
		u := placeCompleteRetailUnit(t, s, "armck", 0, unreachCell(workSiteX-18+i%2*3), unreachCell(workSiteZ-6+i/2*3))
		s.Movement.EnsureUnit(u)
		bs = append(bs, u)
	}
	overlap := func(a, b *units.Unit) bool {
		ac, afx, afz, _ := s.Movement.CommittedFootprint(a.Handle)
		bc, bfx, bfz, _ := s.Movement.CommittedFootprint(b.Handle)
		return ac.X < bc.X+int32(bfx) && bc.X < ac.X+int32(afx) && ac.Z < bc.Z+int32(bfz) && bc.Z < ac.Z+int32(afz)
	}
	// working: parked (no active route) with its build or assist past the
	// approach phase.
	working := func(u *units.Unit) bool {
		if s.Movement.DeveloperFollowerState(u.Handle).HasWaypoint {
			return false
		}
		q := orders.QueueOfUnit(u)
		if q == nil || q.Head() == nil || q.Head().Phase < 2 {
			return false
		}
		name := orders.DescriptorFor(q.Head().ID).Name
		return name == "MobileBuild" || name == "HelpBuild"
	}
	build := func(u *units.Unit, cx, cz int32) {
		x, z := unreachCell(cx), unreachCell(cz)
		if err := s.EnqueueHumanCommand(HumanCommand{Kind: HumanMobileBuild, MobileBuild: HumanMobileBuildCommand{
			Builder: u.Handle, Product: "armsolar", WX: x, WZ: z, WY: ter.HeightAt(x, z),
		}}); err != nil {
			t.Fatal(err)
		}
	}
	var run workSiteRun
	var site *units.Unit
	assisted := false
	followed := 0
	posts := make([][2]int32, len(bs))
	seen := map[string]bool{}
	for tick := 1; followed == 0 || tick <= followed+workSiteFollowTicks; tick++ {
		switch {
		case tick == orderTick:
			build(bs[0], workSiteX, workSiteZ)
		case site != nil && !assisted:
			var helpers []pool.Handle
			for _, u := range bs[1:] {
				helpers = append(helpers, u.Handle)
			}
			if err := s.EnqueueHumanCommand(HumanCommand{Kind: HumanOrder, Order: HumanOrderCommand{
				Handles: helpers, Code: 1, Target: site.Handle,
				Position: orders.ResolvePos{X: site.X, Y: site.Y, Z: site.Z, InterfaceType: orders.InterfaceTypeRightClick},
			}}); err != nil {
				t.Fatal(err)
			}
			assisted = true
		case run.complete != 0 && followed == 0:
			followed = tick
			for i, u := range bs {
				a, _, _, _ := s.Movement.CommittedFootprint(u.Handle)
				posts[i] = [2]int32{a.X, a.Z}
				build(u, workSiteX+20+int32(i%3)*6, workSiteZ-12+int32(i/3)*6)
			}
		}
		s.stepAuthoritativePhases(s.Clock.BeginSubTick())
		for _, e := range s.publication.events.SnapshotEvents() {
			key := fmt.Sprintf("%d/%d/%s", e.Tick, e.Source, e.StatusText)
			if followed != 0 && !seen[key] && (strings.Contains(e.StatusText, "can't reach") || strings.Contains(e.StatusText, "can't get there")) {
				run.refused++
			}
			seen[key] = true
		}
		if site == nil {
			for _, u := range s.Units.IterSliced() {
				if u != nil && u.Alive && u.Def == solar {
					site = u
				}
			}
		}
		if run.complete != 0 {
			continue
		}
		overlapped, intoWorker := false, false
		for i, a := range bs {
			for _, b := range bs[i+1:] {
				if overlap(a, b) {
					overlapped = true
					intoWorker = intoWorker || working(a) || working(b)
				}
			}
		}
		if overlapped {
			run.overlapTicks++
		}
		if intoWorker {
			run.intoWorker++
		}
		if site != nil && assisted && site.Remaining == 0 {
			run.complete = tick
			for i, a := range bs {
				for _, b := range bs[i+1:] {
					if overlap(a, b) {
						run.leftInside++
					}
				}
			}
		}
		if tick > orderTick+6000 {
			return run
		}
	}
	for i, u := range bs {
		a, _, _, _ := s.Movement.CommittedFootprint(u.Handle)
		if dx, dz := a.X-posts[i][0], a.Z-posts[i][1]; dx*dx+dz*dz < 9 {
			run.idle++
		}
	}
	return run
}

// A builder already working at the shared site is never walked into by an
// arriving one, none is left inside another when the collector completes,
// and every one takes its next build order on the tick after — whenever the
// first order is given. Before the work-site rule, jam release carried jammed
// assisters into the parked ones and a builder left inside a friend had its
// next build refused as unreachable (issue #31). The rule's first form still
// let an assister jammed behind a working builder be released the moment an
// arriving friend passed through it head-on, and walk into the worker.
func TestModernBuildersAtASharedSiteNeverStandInsideEachOther(t *testing.T) {
	for _, tick := range workSiteOrderTicks {
		t.Run(fmt.Sprintf("order at tick %d", tick), func(t *testing.T) {
			got := workSiteScene(t, ModernRuleSetName, tick)
			if got.complete == 0 {
				t.Fatal("the collector never completed")
			}
			if got.intoWorker != 0 || got.leftInside != 0 || got.refused != 0 || got.idle != 0 {
				t.Fatalf("a builder stood inside one working at the site on %d ticks and %d pairs overlapped at completion; %d follow-up builds were refused as unreachable and %d builders never left", got.intoWorker, got.leftInside, got.refused, got.idle)
			}
		})
	}
}

// Strict 3.1 releases nothing and passes no one through, so the same scenes
// never overlap at all.
func TestStrictBuildersAtASharedSiteNeverOverlap(t *testing.T) {
	for _, tick := range []int{10, 79, 150} {
		t.Run(fmt.Sprintf("order at tick %d", tick), func(t *testing.T) {
			got := workSiteScene(t, StrictRuleSetName, tick)
			if got.complete == 0 {
				t.Fatal("the collector never completed")
			}
			if got.overlapTicks != 0 {
				t.Fatalf("builders overlapped on %d ticks under Strict", got.overlapTicks)
			}
		})
	}
}
