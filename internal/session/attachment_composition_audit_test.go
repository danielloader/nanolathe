package session

import (
	"encoding/binary"
	"slices"
	"testing"

	"github.com/nanolathe-gg/nanolathe/internal/movement"
	"github.com/nanolathe-gg/nanolathe/internal/orders"
	"github.com/nanolathe-gg/nanolathe/internal/pool"
	"github.com/nanolathe-gg/nanolathe/internal/save"
	"github.com/nanolathe-gg/nanolathe/internal/units"
	"github.com/nanolathe-gg/nanolathe/internal/world"
)

func TestProductionOrderDetachUsesAttachmentIndexOwner(t *testing.T) {
	s := assistSeamSession(t)
	create := func(x int32) *units.Unit {
		h, err := s.Units.Create(s.Catalog.Units["assistseamprod"], 0, world.CellToWorld(x), 0, world.CellToWorld(8))
		if err != nil {
			t.Fatal(err)
		}
		u := s.Units.Unit(h)
		s.Movement.EnsureUnit(u)
		s.bindOrderQueue(u)
		return u
	}
	parent, child, other := create(8), create(10), create(12)
	if !movement.AttachFactoryProduct(s.Units, parent.Handle, child.Handle, -1) {
		t.Fatal("attach")
	}
	before := s.Movement.Collisions[other.Handle].Filing.Seq
	port := orders.QueueForUnit(child).Binding().Movement
	if port.DetachTakeoff == nil || !port.DetachTakeoff(child) {
		t.Fatal("production detach adapter absent/refused")
	}
	f := s.Movement.Collisions[child.Handle].Filing
	if f.Seq <= before || child.Attachment.Carrier != 0 || child.Move.Mode != 2 || len(parent.Attachment.Cargo) != 0 {
		t.Fatalf("detach missed synchronous projection: %+v", f)
	}
	seq := f.Seq
	s.Movement.BindWorld(s.Units)
	if s.Movement.Collisions[child.Handle].Filing.Seq != seq {
		t.Fatal("composition refresh replayed detach")
	}
}

func TestRecursiveRetailRestoreBindsAttachmentFiling(t *testing.T) {
	s, fixtures := newRestoreCoreFixture(t, 3)
	childA, childB, carrier := fixtures[0], fixtures[1], fixtures[2]
	terrain := &world.Terrain{CellW: 32, CellH: 32, Plot: make([]world.PlotCell, 32*32)}
	s.Movement = movement.NewSystem(terrain, movement.Template(), movement.NewOccupancyGrid())
	s.Movement.AttachOverlapBinding(func(uint8) uint8 { return 1 })
	// Do not BindWorld here: production RestoreRetailBattleCore must bind the
	// observer before following these recursive saved references [08 R-SAVE-02 §6].
	childRecord := func(piece byte, engagement uint16) []byte {
		data := unitRecordData(false)
		// Authored saved-unit format fields, as in the existing core restore fixture.
		binary.LittleEndian.PutUint16(data[0x89:], carrier.stableID)
		binary.LittleEndian.PutUint16(data[0x8b:], engagement)
		data[0x8d] = piece
		return data
	}
	stage := &RetailBattleStage{Session: s, StableUnit: stableUnitMap(fixtures), Image: &save.BattleImage{Units: save.UnitImage{Records: []save.UnitRecord{
		{StableID: childA.stableID, Data: childRecord(2, childB.stableID)},
		{StableID: childB.stableID, Data: childRecord(5, 0)},
		{StableID: carrier.stableID, Data: unitRecordData(false)},
	}}}}
	if err := RestoreRetailBattleCore(stage); err != nil {
		t.Fatal(err)
	}
	if got := s.Units.Unit(carrier.handle).Attachment.Cargo; !slices.Equal(got, []pool.Handle{childB.handle, childA.handle}) {
		t.Fatalf("recursive attachment order: %v", got)
	}
	cf := s.Movement.Collisions[carrier.handle].Filing
	for _, child := range []restoreCoreFixtureUnit{childA, childB} {
		f := s.Movement.Collisions[child.handle].Filing
		if !f.Filed || f.Seq != 0 {
			t.Fatalf("restored child gained a top-level insertion: %+v", f)
		}
	}
	if !cf.Filed || cf.Seq == 0 {
		t.Fatal("restored carrier missing top-level insertion")
	}
	// The post-restore release must reach the observer installed by restoration,
	// and a refresh must not replay insertion or disturb the recursive cargo list.
	if _, ok := movement.DetachTakeoff(s.Units, childA.handle); !ok {
		t.Fatal("detach")
	}
	released := s.Movement.Collisions[childA.handle].Filing
	if released.Seq != cf.Seq+1 {
		t.Fatalf("release did not insert once: carrier=%+v child=%+v", cf, released)
	}
	s.Movement.BindWorld(s.Units)
	if s.Movement.Collisions[childA.handle].Filing != released || !slices.Equal(s.Units.Unit(carrier.handle).Attachment.Cargo, []pool.Handle{childB.handle}) {
		t.Fatal("rebind replayed restored relationships")
	}
	var visited []pool.Handle
	s.Movement.VisitUnitsInRadius(0, 0, 16<<16, func(h pool.Handle, _ *units.Unit) bool { visited = append(visited, h); return false })
	if !slices.Equal(visited, []pool.Handle{childA.handle, carrier.handle}) {
		t.Fatalf("restored/released bucket sequence: %v", visited)
	}
}
