package construction

import (
	"testing"

	"github.com/nanolathe-gg/nanolathe/internal/movement"
	"github.com/nanolathe-gg/nanolathe/internal/world"
)

// The attached rollback is a defensive host lifecycle fixture, not a claim
// that the retail allocation-failure arm reaches an accepted attachment.
func TestNeverCreatedRollbackForgetsAttachedFilingBeforeReuse(t *testing.T) {
	facDef, prodDef := newFactoryDef("filingfac", 2, 2, 1000), exitMobileDef("filingprod", 1, 1)
	cat := exitCatalog(facDef, prodDef)
	terrain := exitTerrain(32, 32)
	svc, w := exitService(t, terrain, cat)
	s := movement.NewSystem(terrain, movement.Template(), movement.NewOccupancyGrid())
	s.SetClasses(cat.Movement)
	s.BindWorld(w)
	s.AttachOverlapBinding(func(uint8) uint8 { return 1 })
	svc.Movement = s
	fh, err := w.Create(facDef, 0, world.CellToWorld(4), 0, world.CellToWorld(4))
	if err != nil {
		t.Fatal(err)
	}
	ph, err := w.Create(prodDef, 0, world.CellToWorld(10), 0, world.CellToWorld(10))
	if err != nil {
		t.Fatal(err)
	}
	factory, product := w.Unit(fh), w.Unit(ph)
	s.EnsureUnit(factory)
	s.EnsureUnit(product)
	if !movement.AttachFactoryProduct(w, fh, ph, -1) {
		t.Fatal("attach")
	}
	svc.freeNeverExistedProduct(product)
	if w.Unit(ph) != nil || s.Collisions[ph] != nil || len(factory.Attachment.Cargo) != 0 {
		t.Fatal("rollback retained attachment/movement state")
	}
	rh, err := w.Create(prodDef, 0, world.CellToWorld(20), 0, world.CellToWorld(20))
	if err != nil {
		t.Fatal(err)
	}
	if rh != ph {
		t.Fatalf("expected immediate reuse: %d != %d", rh, ph)
	}
	s.EnsureUnit(w.Unit(rh))
	if f := s.Collisions[rh].Filing; !f.Filed || f.SX != 2 || f.SZ != 2 {
		t.Fatalf("reused old filing: %+v", f)
	}
}
