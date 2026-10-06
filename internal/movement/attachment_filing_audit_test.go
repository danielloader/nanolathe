package movement

import (
	"slices"
	"testing"

	"github.com/nanolathe-gg/nanolathe/internal/content"
	"github.com/nanolathe-gg/nanolathe/internal/orders"
	"github.com/nanolathe-gg/nanolathe/internal/pool"
	"github.com/nanolathe-gg/nanolathe/internal/sim/numeric"
	"github.com/nanolathe-gg/nanolathe/internal/units"
)

func attachmentIndexFixture(t *testing.T, footprint int16) (*System, *units.World, func(int32, int32) *units.Unit) {
	t.Helper()
	profile := Profile{FootPrintX: footprint, FootPrintZ: footprint, MinWaterDepth: -10000, MaxWaterDepth: 100, MaxSlope: 255}
	s := NewSystem(syntheticFlat(64, 64), profile, NewOccupancyGrid())
	w := newMovementFixtureWorld(16)
	s.BindWorld(w)
	s.AttachOverlapBinding(func(uint8) uint8 { return 1 })
	d := setScratchMovement(&content.UnitDef{UnitName: "filing", BMCode: 1, CanMove: true, MaxDamage: 100, FootprintX: int32(footprint), FootprintZ: int32(footprint)}, profile)
	create := func(x, z int32) *units.Unit {
		t.Helper()
		h, err := w.Create(d, 0, numeric.FixedFromInt(int64(x)), 0, numeric.FixedFromInt(int64(z)))
		if err != nil {
			t.Fatal(err)
		}
		u := w.Unit(h)
		s.EnsureUnit(u)
		return u
	}
	return s, w, create
}

func linked(s *System, u *units.Unit) bool {
	return int(u.Handle) < len(s.Grid.links) && s.Grid.links[u.Handle].linked
}

func TestAttachmentFilingDetachReheadsSynchronously(t *testing.T) {
	s, w, create := attachmentIndexFixture(t, 1)
	carrier, child, other := create(40, 40), create(64, 40), create(80, 40)
	c := handleRow(s.Collisions, child.Handle)
	old := c.Filing
	if !AttachFactoryProduct(w, carrier.Handle, child.Handle, -1) {
		t.Fatal("attach refused")
	}
	if linked(s, child) || c.Filing != old {
		t.Fatal("attach changed retained filing or left a top-level member")
	}
	// Same-carrier attachment and transfer never insert into a world bucket.
	if !AttachCargoMode(w, carrier.Handle, child.Handle, -1, 1) || !AttachCargoMode(w, other.Handle, child.Handle, -1, 1) {
		t.Fatal("reattach/transfer refused")
	}
	if linked(s, child) || c.Filing != old || len(carrier.Attachment.Cargo) != 0 {
		t.Fatal("transfer changed filing")
	}
	seq := s.Grid.linkSeq
	if _, ok := DetachFactoryProduct(w, child.Handle); !ok {
		t.Fatal("detach refused")
	}
	if !linked(s, child) || s.Grid.bucketHead(old.SX, old.SZ) != int32(child.Handle)+1 || c.Filing.Seq != seq+1 {
		t.Fatal("stationary detach did not immediately head-insert")
	}
	if c.CachedMode != 1 || child.Move.Mode != 1 {
		t.Fatal("mode1 release changed mode")
	}
	if _, ok := DetachFactoryProduct(w, child.Handle); ok || s.Grid.linkSeq != seq+1 {
		t.Fatal("no-op detach relinked")
	}
	if AttachCargo(w, child.Handle, child.Handle, -1) || s.Grid.linkSeq != seq+1 {
		t.Fatal("rejected attach relinked")
	}
	s.BindWorld(w)
	if s.Grid.linkSeq != seq+1 {
		t.Fatal("same-world bind replayed attachment")
	}
}

func TestCarriedFilingNoPlaneOffMapAndHangQuantization(t *testing.T) {
	s, w, create := attachmentIndexFixture(t, 1)
	carrier, child := create(127, 80), create(40, 80)
	if !AttachCargo(w, carrier.Handle, child.Handle, -1) {
		t.Fatal("attach")
	}
	handleRow(s.Collisions, carrier.Handle).VX = 2 << 16
	before := s.Grid.linkSeq
	s.SyncCarriedMotion(w)
	c := handleRow(s.Collisions, child.Handle)
	if c.CachedAnchor.X != 7 || c.X != 127<<16 || c.VX != 2<<16 {
		t.Fatalf("hang was advanced by copied velocity: %+v", c)
	}
	if linked(s, child) || c.HasStamp || c.Filing.SX != 0 || s.Grid.linkSeq != before {
		t.Fatal("mode0 hang must update only its retained reference")
	}
	carrier.X = 300 << 16
	s.SyncCarriedMotion(w)
	if c.Filing.SX != 2 || linked(s, child) || s.Grid.linkSeq != before {
		t.Fatal("carried sector crossing inserted child")
	}
	carrier.X = -32 << 16
	s.SyncCarriedMotion(w)
	if !c.Filing.OffMap || linked(s, child) {
		t.Fatal("off-map cargo was independently linked")
	}
	var off []pool.Handle
	s.Grid.VisitOffMapFiled(func(h pool.Handle, _ uint64) bool { off = append(off, h); return true })
	if slices.Contains(off, child.Handle) {
		t.Fatal("off-map consumer saw cargo reference")
	}
	if _, ok := DetachCargo(w, child.Handle); !ok {
		t.Fatal("detach")
	}
	if !s.Grid.links[child.Handle].offMap || s.Grid.offMapHead != int32(child.Handle)+1 {
		t.Fatal("detach lost retained off-map record")
	}
	if !AttachCargo(w, carrier.Handle, child.Handle, -1) {
		t.Fatal("reattach")
	}
	carrier.X = 300 << 16
	s.SyncCarriedMotion(w)
	if c.Filing.OffMap || c.Filing.SX != 2 || linked(s, child) {
		t.Fatal("return on map changed membership")
	}
}

func TestSameAnchorRetainsSectorThroughRestampAndDetach(t *testing.T) {
	s, w, create := attachmentIndexFixture(t, 2)
	carrier, child := create(127, 80), create(127, 80)
	if !AttachFactoryProduct(w, carrier.Handle, child.Handle, -1) {
		t.Fatal("attach")
	}
	s.SyncCarriedMotion(w)
	c := handleRow(s.Collisions, child.Handle)
	old := c.Filing
	carrier.X = 129 << 16
	s.SyncCarriedMotion(w)
	if c.CachedAnchor.X != 7 || c.X != 129<<16 || c.Filing != old {
		t.Fatal("same-anchor update refreshed sector")
	}
	// Consumer boundary: an established intruder restamp must not become an
	// ordinary stamp or file from the newly copied XYZ.
	child.Flags |= units.OverlapIntruderStatus
	s.Grid.Restamp(int(child.Handle))
	if c.Filing != old || linked(s, child) {
		t.Fatal("restamp changed retained filing")
	}
	if _, ok := DetachFactoryProduct(w, child.Handle); !ok {
		t.Fatal("detach")
	}
	if s.Grid.links[child.Handle].sx != 0 || c.Filing.SX != 0 {
		t.Fatal("detach used current XYZ instead of retained reference")
	}
}

func TestAttachmentObserverReplacementAndMissingFiling(t *testing.T) {
	s, w, create := attachmentIndexFixture(t, 1)
	parent, child := create(40, 40), create(64, 40)
	replacement := NewSystem(s.Terrain, s.Fallback, NewOccupancyGrid())
	replacement.BindWorld(w)
	replacement.AttachOverlapBinding(func(uint8) uint8 { return 1 })
	replacement.EnsureUnit(parent)
	replacement.EnsureUnit(child)
	s.BindWorld(newMovementFixtureWorld(4))
	if !AttachCargo(w, parent.Handle, child.Handle, -1) || linked(replacement, child) {
		t.Fatal("old owner cleared replacement observer")
	}
	// Explicit incomplete host context: no retained reference is fabricated.
	replacement.Grid.ForgetFiling(int(child.Handle))
	seq := replacement.Grid.linkSeq
	if _, ok := DetachCargo(w, child.Handle); !ok {
		t.Fatal("detach")
	}
	if linked(replacement, child) || replacement.Grid.linkSeq != seq {
		t.Fatal("missing filing guessed a rehead target")
	}
}

func TestAttachmentRestoreAndForgetReuse(t *testing.T) {
	s, w, create := attachmentIndexFixture(t, 1)
	parent, child := create(40, 40), create(64, 40)
	if !AttachCargo(w, parent.Handle, child.Handle, -1) {
		t.Fatal("attach")
	}
	c := handleRow(s.Collisions, child.Handle)
	c.Mode = 0
	c.X = 300 << 16
	c.Z = 80 << 16
	if err := s.RestoreOccupancy(child.Handle, 18, 5); err != nil {
		t.Fatal(err)
	}
	if !c.Filing.Filed || c.Filing.SX != 2 || linked(s, child) {
		t.Fatal("restored mode0 omitted filing or relinked child")
	}
	// Cleanup does not detach/rehead; free then reallocate the same pool slot.
	h := child.Handle
	s.ForgetUnit(h)
	w.FreeNeverCreated(h)
	if linked(s, child) || handleRow(s.Collisions, h) != nil {
		t.Fatal("forgotten cargo retained index state")
	}
	replacement := create(480, 80)
	if replacement.Handle != h {
		t.Fatalf("fixture did not reuse slot: %d != %d", replacement.Handle, h)
	}
	rc := handleRow(s.Collisions, h)
	if rc.Filing.SX != 3 || !linked(s, replacement) {
		t.Fatal("reused slot retained old reference")
	}
	// An actual same-cell direct setter also leaves the reference untouched.
	if !s.PlaceUnit(orders.PlaceRequest{Unit: h, X: replacement.X, Y: replacement.Y, Z: replacement.Z}) {
		t.Fatal("place")
	}
}

// This fixture isolates the query contract: cargo has a retained reference
// distinct from its parent's selected bucket, and list order differs from ID.
type attachmentOverlapFixture struct {
	*filingFixture
	carrier []int
	cargo   [][]pool.Handle
}

func (f *attachmentOverlapFixture) OverlapAttachment(id int) (int, []pool.Handle) {
	return f.carrier[id], f.cargo[id]
}
func TestOverlapSelectsParentThenCargoIndependently(t *testing.T) {
	for _, outside := range []bool{false, true} {
		f := &attachmentOverlapFixture{newFilingFixture(6), make([]int, 6), make([][]pool.Handle, 6)}
		g := NewOccupancyGrid()
		g.AttachOverlap(f, func(uint8) uint8 { return 1 })
		f.addAt(1, 0, Cell{15, 10}, 1, 1) // parent misses rectangle but is in its sector span
		f.addAt(2, 0, Cell{10, 10}, 1, 1)
		f.addAt(3, 0, Cell{10, 10}, 1, 1)
		f.addAt(4, 0, Cell{10, 10}, 1, 1)
		f.carrier[2], f.carrier[3] = 1, 1
		f.cargo[1] = []pool.Handle{3, 2}
		// These retained references are intentionally outside the query span.
		f.posX[2], f.posX[3] = 800<<16, 900<<16
		if outside {
			f.posX[1] = 800 << 16
		}
		for _, id := range []int{1, 2, 3} {
			r := f.rect[id]
			g.fileUnit(id, r.anchor, r.fx, r.fz)
		}
		f.intruder[2], f.intruder[3] = true, true
		f.restamp = func(id int) { r := f.rect[id]; g.stampPlaneCells(PlaneGround, r.anchor, r.fx, r.fz, id) }
		g.overlapScan(4, Cell{10, 10}, 1, 1)
		if outside {
			if len(f.restamped) != 0 {
				t.Fatalf("outside parent exposed cargo: %v", f.restamped)
			}
			continue
		}
		if !slices.Equal(f.restamped, []int{3, 2}) {
			t.Fatalf("cargo walk %v", f.restamped)
		}
		if got, _ := g.OccupantAt(Cell{10, 10}); got != 3 {
			t.Fatalf("first cargo did not win: %d", got)
		}
		// The clearing parent is still a cargo-list owner. The self flag has
		// already been consumed by clear, so only the children restamp.
		f.rect[1] = overlapRect{Cell{10, 10}, 1, 1, true}
		f.intruder[2], f.intruder[3] = true, true
		f.restamped = nil
		g.overlapScan(1, Cell{10, 10}, 1, 1)
		if !slices.Equal(f.restamped, []int{3, 2}) {
			t.Fatalf("clearing parent skipped cargo: %v", f.restamped)
		}
	}
}

func TestScriptDropPreservesRetainedFilingUntilMoverCommit(t *testing.T) {
	s, w, create := attachmentIndexFixture(t, 2)
	parent, child := create(40, 80), create(127, 80)
	if !AttachCargo(w, parent.Handle, child.Handle, -1) {
		t.Fatal("attach")
	}
	// Commit the ordinary carried mode before the same-anchor XYZ change;
	// the placement gate then sees the empty ground plane it requires.
	if !s.PlaceUnit(orders.PlaceRequest{Unit: child.Handle, X: child.X, Y: child.Y, Z: child.Z}) {
		t.Fatal("initial place")
	}
	if !s.PlaceUnit(orders.PlaceRequest{Unit: child.Handle, X: 129 << 16, Y: child.Y, Z: child.Z}) {
		t.Fatal("place")
	}
	c := handleRow(s.Collisions, child.Handle)
	if c.Filing.SX != 0 || c.CachedAnchor.X != 7 {
		t.Fatal("fixture lost stale reference")
	}
	if !s.ScriptDropCargo(w, parent.Handle, child.Handle) {
		t.Fatal("script drop refused")
	}
	if c.Filing.SX != 0 || s.Grid.links[child.Handle].sx != 0 {
		t.Fatal("script drop refreshed retained reference")
	}
}

func TestOrphanCleanupDoesNotInventFiling(t *testing.T) {
	s, w, create := attachmentIndexFixture(t, 1)
	parent, child := create(40, 40), create(64, 40)
	if !AttachCargo(w, parent.Handle, child.Handle, -1) {
		t.Fatal("attach")
	}
	// Consumer-only incomplete host history; no retail producer is asserted.
	child.Attachment.Carrier = pool.Handle(w.Capacity() + 1)
	old := handleRow(s.Collisions, child.Handle).Filing
	s.syncCarriedUnit(w, child)
	if child.Attachment.Carrier != 0 || linked(s, child) || handleRow(s.Collisions, child.Handle).Filing != old {
		t.Fatal("orphan cleanup fabricated a detach filing")
	}
}

type attachmentCommitProbe struct {
	system   *System
	carriers []pool.Handle
	modes    []uint8
}

func (p *attachmentCommitProbe) AttachmentChanged(u *units.Unit) {
	p.carriers = append(p.carriers, u.Attachment.Carrier)
	p.modes = append(p.modes, u.Move.Mode)
	p.system.AttachmentChanged(u)
}
func TestAttachmentProjectionPrecedesRequestedMode(t *testing.T) {
	s, w, create := attachmentIndexFixture(t, 1)
	parent, child := create(40, 40), create(64, 40)
	probe := &attachmentCommitProbe{system: s}
	w.SetAttachmentObserver(probe)
	if !AttachCargo(w, parent.Handle, child.Handle, -1) {
		t.Fatal("attach")
	}
	if _, ok := DetachTakeoff(w, child.Handle); !ok {
		t.Fatal("detach")
	}
	if !slices.Equal(probe.carriers, []pool.Handle{parent.Handle, 0}) || !slices.Equal(probe.modes, []uint8{1, 0}) || child.Move.Mode != 2 {
		t.Fatalf("projection ordering: carriers=%v modes=%v final=%d", probe.carriers, probe.modes, child.Move.Mode)
	}
}
