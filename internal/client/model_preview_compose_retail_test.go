package client

import (
	"slices"
	"strings"
	"testing"

	"github.com/nanolathe-gg/nanolathe/internal/construction"
	"github.com/nanolathe-gg/nanolathe/internal/frame"
	"github.com/nanolathe-gg/nanolathe/internal/headless"
	compiledmodel "github.com/nanolathe-gg/nanolathe/internal/model"
	"github.com/nanolathe-gg/nanolathe/internal/sim/numeric"
	"github.com/nanolathe-gg/nanolathe/internal/sim/rng"
	"github.com/nanolathe-gg/nanolathe/internal/testsupport"
	"github.com/nanolathe-gg/nanolathe/vfs"
)

// A stock factory's pad placement reproduces where and how the running
// session hangs its product: the published offset from the factory and the
// published heading [04 R-FAC-02 §2][04 R-REV-02]. The pad piece is the one
// the factory's own script answers QueryBuildInfo with.
func TestModelPreviewPiecePlacementMatchesRetailFactoryProduct(t *testing.T) {
	fs := vfs.New()
	if err := fs.MountGameDirectory(testsupport.RetailRoot(t)); err != nil {
		t.Skipf("retail assets not mountable: %v", err)
	}
	defer fs.Close()
	preview, err := NewModelPreviewRenderer(fs)
	if err != nil {
		t.Skipf("retail presentation assets not loadable: %v", err)
	}
	rng.SeedGlobal(1, 1)
	battle, err := headless.ComposeFreshBattle(headless.FreshBattleRequest{
		Kind: headless.ScenarioDirectOTA, Map: "Great Divide", LocalOwner: -1,
		SimulationSeed: 1, CRTSeed: 1, FS: fs,
	})
	if err != nil {
		t.Fatal(err)
	}
	sess := battle.Session
	for tick := int32(1); tick <= 30; tick++ {
		sess.Step(tick)
	}
	def, ok := sess.Catalog.Unit("armlab")
	if !ok || def == nil {
		t.Fatal("stock ARMLAB is missing")
	}
	var x, z numeric.Fixed
	for _, u := range sess.Units.Iter() {
		if u != nil && u.Alive && u.Owner == sess.LocalOwner && u.Def != nil && strings.HasSuffix(strings.ToLower(u.Def.UnitName), "com") {
			x, z = u.X+numeric.Fixed(10<<20), u.Z
			break
		}
	}
	handle, err := sess.Units.Create(def, sess.LocalOwner, x, sess.World.HeightAt(x, z), z)
	if err != nil {
		t.Fatal(err)
	}
	factory := sess.Units.Unit(handle)
	if err := sess.Build.RegisterBuildingPlacement(factory); err != nil {
		t.Fatal(err)
	}
	if err := construction.QueueFactoryBuild(factory, "armpw", 1, sess.Catalog); err != nil {
		t.Fatal(err)
	}
	checked := 0
	for tick := int32(31); tick <= 3000 && checked < 2; tick++ {
		for i := range sess.Econ.Players {
			sess.Econ.Players[i].Stock = [2]float32{1e8, 1e8}
			sess.Econ.Players[i].Capacity = [2]float32{1e8, 1e8}
		}
		sess.Step(tick)
		cur := sess.Snapshot.Current()
		if cur == nil {
			continue
		}
		var carrier, child *frame.UnitView
		for i := range cur.Units {
			if u := &cur.Units[i]; u.Slot == handle {
				carrier = u
			} else if u.Carrier == handle && u.BuildRemaining > 0 {
				child = u
			}
		}
		if carrier == nil || child == nil || child.BuildRemaining > 1-.4*float32(checked+1) {
			continue
		}
		binding := factory.COBBinding()
		pad := binding.Model.Pieces[binding.PieceMap[binding.Callbacks.QueryBuildInfo().QueryValue()]].Name
		placement, err := preview.PiecePlacement(carrier.Model, carrier.Pieces, pad)
		if err != nil {
			t.Fatal(err)
		}
		if got := carrier.Heading + placement.Heading; got != child.Heading || placement.Pitch != 0 || placement.Bank != 0 {
			t.Fatalf("tick %d: composed product heading %d (pitch %d, bank %d), session %d", tick, got, placement.Pitch, placement.Bank, child.Heading)
		}
		m := preview.client.modelForUnit(frame.UnitView{Model: carrier.Model})
		states := slices.Clone(preview.client.modelStates(m, carrier.Pieces))
		compiledmodel.FoldRootAngles(states, m.compiled.Root, carrier.Heading, carrier.Pitch, carrier.Bank)
		origin := compiledmodel.Compose(m.compiled, states, m.compiled.Root).Apply(placement.Position)
		// Model space mirrors world Z [03 R-RAST-01 §2].
		if want := [3]numeric.Fixed{child.X - carrier.X, child.Y - carrier.Y, carrier.Z - child.Z}; origin != want {
			t.Fatalf("tick %d: composed product origin %v, session offset %v", tick, origin, want)
		}
		checked++
	}
	if checked < 2 {
		t.Fatalf("session reached %d of 2 construction samples", checked)
	}
}
