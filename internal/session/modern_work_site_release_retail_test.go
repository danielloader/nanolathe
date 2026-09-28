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

type workSiteRun struct {
	complete     int // tick the collector completed, 0 if it never did
	overlapTicks int // ticks before completion on which two builders overlapped
	refused      int // follow-up builds refused as unreachable
	idle         int // builders that never left their post after the follow-up
}

// workSiteScene runs the fixture under the named rule set. On the tick after
// the collector completes every builder is ordered to build its own collector
// east of the site, and the scene runs workSiteFollowTicks more ticks.
func workSiteScene(t *testing.T, rules string) workSiteRun {
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
	overlapping := func() bool {
		for i, a := range bs {
			ac, afx, afz, _ := s.Movement.CommittedFootprint(a.Handle)
			for _, b := range bs[i+1:] {
				bc, bfx, bfz, _ := s.Movement.CommittedFootprint(b.Handle)
				if ac.X < bc.X+int32(bfx) && bc.X < ac.X+int32(afx) && ac.Z < bc.Z+int32(bfz) && bc.Z < ac.Z+int32(afz) {
					return true
				}
			}
		}
		return false
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
		case tick == 10:
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
		if run.complete == 0 {
			if site != nil && assisted && site.Remaining == 0 {
				run.complete = tick
			}
			if overlapping() {
				run.overlapTicks++
			}
		}
		if run.complete == 0 && tick > 6000 {
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

// A builder already working at the shared site is never passed through by a
// jammed one, so no two builders stand inside each other while they build,
// and every one of them takes its next build order the tick the collector
// completes. Before the work-site rule, jam release carried jammed assisters
// into the parked ones — two hundred ticks of overlap in this scene — and a
// builder left inside a friend had its next build refused as unreachable.
func TestModernBuildersAtASharedSiteNeverStandInsideEachOther(t *testing.T) {
	got := workSiteScene(t, ModernRuleSetName)
	if got.complete == 0 {
		t.Fatal("the collector never completed")
	}
	if got.overlapTicks != 0 || got.refused != 0 || got.idle != 0 {
		t.Fatalf("builders overlapped on %d ticks while building; %d follow-up builds were refused as unreachable and %d builders never left", got.overlapTicks, got.refused, got.idle)
	}
}

// Strict 3.1 releases nothing, so the same scene never overlaps either.
func TestStrictBuildersAtASharedSiteNeverOverlap(t *testing.T) {
	got := workSiteScene(t, StrictRuleSetName)
	if got.complete == 0 {
		t.Fatal("the collector never completed")
	}
	if got.overlapTicks != 0 {
		t.Fatalf("builders overlapped on %d ticks under Strict", got.overlapTicks)
	}
}
