package session

import (
	"testing"

	"github.com/nanolathe-gg/nanolathe/internal/construction"
	"github.com/nanolathe-gg/nanolathe/internal/frame"
	"github.com/nanolathe-gg/nanolathe/internal/pool"
	"github.com/nanolathe-gg/nanolathe/internal/sim/numeric"
	"github.com/nanolathe-gg/nanolathe/internal/visibility"
)

func TestOnlineBuildKnowledgePrecedesQueueMutation(t *testing.T) {
	f := newSeatFixture(t, true, false)
	s := f.s
	s.onlineResults = newOnlineResultState([10]bool{true, true})
	f.def.FootprintX, f.def.FootprintZ = 2, 2
	s.Build = construction.NewService(s.World, s.Catalog, s.Units, s.Econ)
	s.Vis = visibility.New(s.World, visibility.ModeHistoryEnabled)
	s.Vis.EnableOwnerPerspectives()
	expectOutcome(t, f.issue(t, 1, moveTo([]pool.UnitRef{f.ref(f.own1a)}, 300)), CommandApplied)
	head := f.head(t, f.own1a)
	c := SeatCommand{Kind: SeatMobileBuild, MobileBuild: MobileBuildPayload{Builder: f.ref(f.own1a), Product: "scout", Position: CommandPoint{X: 96 << 16, Z: 96 << 16}}}
	cell := 3*int(s.Vis.W) + 3
	s.Vis.WordMask()[cell] = 1 // Local seat 0 knows it; issuing seat 1 does not.
	expectOutcome(t, f.issue(t, 1, c), CommandRejected)
	if f.head(t, f.own1a) != head {
		t.Fatal("unknown build replaced existing order")
	}
	s.Vis.WordMask()[cell] = 2
	c.MobileBuild.Position.Y = 1 << 16
	expectOutcome(t, f.issue(t, 1, c), CommandRejected)
	if f.head(t, f.own1a) != head {
		t.Fatal("forged height replaced existing order")
	}
	c.MobileBuild.Position.Y = 0
	expectOutcome(t, f.issue(t, 1, c), CommandApplied)
	if f.head(t, f.own1a) == head {
		t.Fatal("known build did not replace order")
	}
	s.onlineResults.seats[1].latch.Bits = 4
	expectOutcome(t, f.issue(t, 1, SeatCommand{Kind: SeatStop, Stop: StopPayload{Actors: []pool.UnitRef{f.ref(f.own1a)}}}), CommandRejected)
}

func TestOnlineDeathSightKeepsEachOwnersCapacityAndExpiry(t *testing.T) {
	s := newEyeballSession(t)
	s.onlineResults = newOnlineResultState([10]bool{true, true})
	s.Vis.EnableOwnerPerspectives()
	s.Clock.GlobalTick = 100
	for owner := uint8(0); owner < 2; owner++ {
		for i := 0; i < eyeballCapacity+1; i++ {
			kill(s, spawnEyeballVictim(t, s, owner, 10+int32(owner)*10))
		}
	}
	if len(s.postLoop.eyeballs.records) != 0 {
		t.Fatal("online death used the local-only list")
	}
	for owner := range 2 {
		if len(s.postLoop.onlineEyeballs[owner].records) != 20 {
			t.Fatalf("seat %d lost independent death-sight capacity", owner)
		}
	}
	s.runRetailPostLoopTail(160)
	for owner := range 2 {
		if len(s.postLoop.onlineEyeballs[owner].records) != 20 {
			t.Fatal("expiry tick was excluded")
		}
	}
	s.runRetailPostLoopTail(161)
	for owner := range 2 {
		if len(s.postLoop.onlineEyeballs[owner].records) != 0 {
			t.Fatal("expired sight retained")
		}
	}
}

// Kind 3 draws jitter before each allocation even when authored starts replace
// it [08 R-ENTRY-01 §5]. Compare the interleaved control stream, not a census.
func TestOnlinePlacementDrawsBeforeEachCommander(t *testing.T) {
	control := strictNewSessionWithUnits(t, 0, 7, 11)
	s := strictNewSessionWithUnits(t, 0, 7, 11)
	s.onlineResults = newOnlineResultState([10]bool{true, true})
	m := wu19178Mission()
	for _, name := range []string{"armcom", "corcom"} {
		x := numeric.Fixed(control.SimRNG().Uint32n(uint32(control.World.CellW*16-160))+80) << 16
		z := numeric.Fixed(control.SimRNG().Uint32n(uint32(control.World.CellH*16-160))+80) << 16
		owner := uint8(0)
		if name == "corcom" {
			owner = 1
		}
		if _, err := control.Units.Create(control.Catalog.Units[name], owner, x, 0, z); err != nil {
			t.Fatal(err)
		}
	}
	if err := skirmishReconstructUnits(s, wu19178Config(), m); err != nil {
		t.Fatal(err)
	}
	if s.SimRNG().State != control.SimRNG().State || s.SimRNG().Draws() != control.SimRNG().Draws() {
		t.Fatal("online placement did not interleave jitter and allocation draws")
	}
	for i, u := range s.Units.Iter() {
		want := control.Units.Iter()[i]
		if u.X != want.X || u.Z != want.Z {
			t.Fatal("missing online StartPos did not retain jitter")
		}
	}
}

func TestOnlineEffectsUseViewingSightWithoutAliasing(t *testing.T) {
	s := visibilityFixture(t, true)
	s.onlineResults = newOnlineResultState([10]bool{true, true})
	def := s.Catalog.Units["armcom"]
	for owner, x := range []numeric.Fixed{128 << 16, 896 << 16} {
		if _, err := s.Units.Create(def, uint8(owner), x, 0, 128<<16); err != nil {
			t.Fatal(err)
		}
	}
	publishVisibilityForAll(s)
	source := []frame.EffectView{
		{ID: 1, X: 128 << 16, Z: 128 << 16, DurationsA: []int32{1}},
		{ID: 2, X: 896 << 16, Z: 128 << 16, DurationsA: []int32{2}},
		{ID: 3, X: 896 << 16, Z: 128 << 16, DurationsA: []int32{3}},
	}
	f := frame.Frame{Effects: source, Debris: []frame.DebrisView{{Slot: 1, X: 128 << 16, Z: 128 << 16}, {Slot: 2, X: 896 << 16, Z: 128 << 16}}}
	s.ViewingOwner = 1
	s.filterOnlineVisuals(&f)
	if len(f.Effects) != 2 || f.Effects[0].ID != 2 || f.Effects[1].ID != 3 || len(f.Debris) != 1 || f.Debris[0].Slot != 2 {
		t.Fatalf("wrong local effects/debris: %+v / %+v", f.Effects, f.Debris)
	}
	all := f.Effects[:cap(f.Effects)]
	for i := range all {
		all[i].DurationsA[0] = int32(i + 10)
	}
	for i := range all {
		if all[i].DurationsA[0] != int32(i+10) {
			t.Fatal("duration buffers aliased during compaction")
		}
	}
	s.onlineResults = nil
	f.Effects = all
	s.filterOnlineVisuals(&f)
	if len(f.Effects) != 3 {
		t.Fatal("single-player effect publication changed")
	}
}
