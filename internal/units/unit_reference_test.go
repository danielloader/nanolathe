package units

import (
	"errors"
	"math"
	"testing"

	"github.com/nanolathe-gg/nanolathe/internal/content"
	"github.com/nanolathe-gg/nanolathe/internal/pool"
	"github.com/nanolathe-gg/nanolathe/internal/sim/rng"
)

func TestUnitReferencesAcrossCreationPathsAndReuse(t *testing.T) {
	w := newFixtureWorld(3, nil)
	def := p28AngleDef("reference", 4096)
	var notified []pool.UnitRef
	w.SetCreateHook(func(h pool.Handle, u *Unit) {
		ref := w.Reference(h)
		if ref.Serial == 0 || ref.Serial != w.LastAllocationSerial() || w.LookupReference(ref) != u {
			t.Fatal("creation notification cannot resolve its successful allocation")
		}
		notified = append(notified, ref)
	})
	creators := []func() (pool.Handle, error){
		func() (pool.Handle, error) { return w.Create(def, 0, 0, 0, 0) },
		func() (pool.Handle, error) { return w.CreateNanoframe(def, 1, 0, 0, 0) },
		func() (pool.Handle, error) { return w.CreateWithForcedSlot(def, 0, 0, 0, 0, 3) },
	}
	for i, create := range creators {
		h, err := create()
		if err != nil {
			t.Fatal(err)
		}
		if got := w.Reference(h).Serial; got != uint64(i+1) {
			t.Fatalf("creation %d serial = %d", i, got)
		}
	}
	old := notified[0]
	w.FreeImmediate(old.Handle)
	if w.Reference(old.Handle) != (pool.UnitRef{}) || w.LookupReference(old) != nil {
		t.Fatal("freed allocation still resolves")
	}
	h, err := w.Create(def, 0, 0, 0, 0)
	if err != nil || h != old.Handle {
		t.Fatalf("immediate reuse = %d, %v", h, err)
	}
	if w.LookupReference(old) != nil || w.Reference(h).Serial != 4 {
		t.Fatal("reuse redirected an old reference or reused its serial")
	}
	// Raw simulation handles retain retail aliasing even though commands reject
	// the old serial [I5][06 §5.1].
	if !w.ApplyDamage(old.Handle, 1) || w.Unit(h).Health != int32(def.MaxDamage)-1 {
		t.Fatal("command references changed raw-slot damage aliasing")
	}
	for _, ref := range []pool.UnitRef{{}, {Handle: h}, {Serial: 4}, {Handle: math.MaxUint16, Serial: 4}} {
		if w.LookupReference(ref) != nil {
			t.Fatalf("invalid reference resolved: %+v", ref)
		}
	}
	var absent *World
	if absent.Reference(h) != (pool.UnitRef{}) || absent.LookupReference(old) != nil || absent.LastAllocationSerial() != 0 {
		t.Fatal("nil world has a reference namespace")
	}
}

var referenceCreators = []struct {
	name   string
	create func(*World, *content.UnitDef) (pool.Handle, error)
}{
	{"ordinary", func(w *World, d *content.UnitDef) (pool.Handle, error) { return w.Create(d, 0, 0, 0, 0) }},
	{"forced", func(w *World, d *content.UnitDef) (pool.Handle, error) {
		return w.CreateWithForcedSlot(d, 0, 0, 0, 0, 1)
	}},
}

// The diagnostic binder's failure retains the initializer's existing heading
// and bob draws [04 R-COB-01 §3], but consumes no command serial (M2-C1).
func TestUnitReferenceFailedCreation(t *testing.T) {
	for _, path := range referenceCreators {
		t.Run(path.name, func(t *testing.T) {
			w := newFixtureWorld(1, nil)
			def := p28AngleDef("failure", 4096)
			sim, expected := rng.NewSimulation(53), rng.NewSimulation(53)
			w.SetSimulationRNG(&sim)
			notifications := 0
			w.SetCreateHook(func(pool.Handle, *Unit) { notifications++ })
			w.SetCOBBinder(func(u *Unit) error {
				if w.Reference(u.Handle) != (pool.UnitRef{}) || u.AllocationSerial != 0 {
					t.Fatal("unfinished binding has a successful allocation reference")
				}
				return errors.New("fixture binding failure")
			})
			if h, err := path.create(w, def); h != 0 || err == nil {
				t.Fatalf("binding failure = %d, %v", h, err)
			}
			expected.Uint32n(uint32(def.BuildAngle))
			expected.Uint32n(0x10000)
			if sim.State != expected.State || sim.Draws() != expected.Draws() {
				t.Fatal("failed binding changed already-consumed initializer RNG")
			}
			if w.LastAllocationSerial() != 0 || w.pendingAllocationSerials != 0 || w.pool.Used() != 0 || notifications != 0 {
				t.Fatal("failed binding retained allocation state or notified creation")
			}
			w.SetCOBBinder(nil)
			h, err := path.create(w, def)
			if err != nil || h != 1 || w.Reference(h).Serial != 1 {
				t.Fatalf("retry = %d, %v, serial %d", h, err, w.Reference(h).Serial)
			}
			before, draws := sim.State, sim.Draws()
			if _, err := path.create(w, def); err == nil {
				t.Fatal("occupied slice accepted another creation")
			}
			if w.LastAllocationSerial() != 1 || w.pendingAllocationSerials != 0 || sim.State != before || sim.Draws() != draws {
				t.Fatal("allocation refusal consumed a serial or RNG")
			}
		})
	}
}

func TestUnitReferenceSerialExhaustion(t *testing.T) {
	for _, path := range referenceCreators {
		t.Run(path.name, func(t *testing.T) {
			w := newFixtureWorld(2, nil)
			w.lastAllocationSerial = math.MaxUint64 - 1
			def := p28AngleDef("last", 4096)
			sim := rng.NewSimulation(53)
			w.SetSimulationRNG(&sim)
			h, err := path.create(w, def)
			if err != nil || w.Reference(h).Serial != math.MaxUint64 {
				t.Fatalf("last serial = %d, %v", w.Reference(h).Serial, err)
			}
			w.FreeImmediate(h)
			before, draws := sim.State, sim.Draws()
			w.SetCOBBinder(func(*Unit) error { t.Fatal("exhaustion reached binder"); return nil })
			if h, err := path.create(w, def); h != 0 || err == nil {
				t.Fatalf("exhaustion = %d, %v", h, err)
			}
			if w.LastAllocationSerial() != math.MaxUint64 || w.pool.Used() != 0 || sim.State != before || sim.Draws() != draws {
				t.Fatal("exhaustion wrapped, allocated, or drew RNG")
			}
		})
	}
}

func TestUnitReferenceNestedBinderAtSerialLimit(t *testing.T) {
	for _, path := range referenceCreators {
		for _, remaining := range []uint64{1, 2} {
			t.Run(path.name+"/"+string(rune('0'+remaining)), func(t *testing.T) {
				w := newFixtureWorld(2, nil)
				w.lastAllocationSerial = math.MaxUint64 - remaining
				def := p28AngleDef("nested", 4096)
				sim := rng.NewSimulation(53)
				w.SetSimulationRNG(&sim)
				var inner pool.Handle
				w.SetCOBBinder(func(u *Unit) error {
					if u.Owner != 0 {
						return nil
					}
					before, draws := sim.State, sim.Draws()
					var err error
					inner, err = w.Create(def, 1, 0, 0, 0)
					if remaining == 1 {
						if err == nil || inner != 0 || sim.State != before || sim.Draws() != draws {
							t.Fatal("nested creation consumed reserved final serial or RNG")
						}
					} else if err != nil || w.Reference(inner).Serial != math.MaxUint64-1 {
						t.Fatalf("nested success = %d, %v", w.Reference(inner).Serial, err)
					}
					return nil
				})
				h, err := path.create(w, def)
				if err != nil || w.Reference(h).Serial != math.MaxUint64 || w.pendingAllocationSerials != 0 {
					t.Fatalf("outer success = %d, %v, pending %d", w.Reference(h).Serial, err, w.pendingAllocationSerials)
				}
			})
		}
	}
}

func TestUnitReferenceReservationReleasedBeforeNotification(t *testing.T) {
	w := newFixtureWorld(2, nil)
	w.lastAllocationSerial = math.MaxUint64 - 2
	def := p28AngleDef("notification", 4096)
	w.SetCreateHook(func(_ pool.Handle, u *Unit) {
		if w.pendingAllocationSerials != 0 {
			t.Fatal("successful reservation still held in notification")
		}
		if u.Owner == 0 {
			h, err := w.Create(def, 1, 0, 0, 0)
			if err != nil || w.Reference(h).Serial != math.MaxUint64 {
				t.Fatalf("notification creation = %d, %v", w.Reference(h).Serial, err)
			}
		}
	})
	if _, err := w.Create(def, 0, 0, 0, 0); err != nil {
		t.Fatal(err)
	}
}

func TestUnitReferenceNestedSuccessSurvivesOuterBindingFailure(t *testing.T) {
	for _, path := range referenceCreators {
		t.Run(path.name, func(t *testing.T) {
			w := newFixtureWorld(2, nil)
			w.lastAllocationSerial = math.MaxUint64 - 2
			def := p28AngleDef("nested failure", 4096)
			var inner pool.UnitRef
			w.SetCOBBinder(func(u *Unit) error {
				if u.Owner != 0 {
					return nil
				}
				h, err := w.Create(def, 1, 0, 0, 0)
				if err != nil {
					t.Fatal(err)
				}
				inner = w.Reference(h)
				return errors.New("outer binding failure")
			})
			if h, err := path.create(w, def); h != 0 || err == nil {
				t.Fatalf("outer failure = %d, %v", h, err)
			}
			if inner.Serial != math.MaxUint64-1 || w.LookupReference(inner) == nil || w.LastAllocationSerial() != inner.Serial || w.pendingAllocationSerials != 0 {
				t.Fatal("failed outer creation consumed or undid the nested success's serial")
			}
			w.SetCOBBinder(nil)
			h, err := path.create(w, def)
			if err != nil || w.Reference(h).Serial != math.MaxUint64 {
				t.Fatalf("released reservation retry = %d, %v", w.Reference(h).Serial, err)
			}
		})
	}
}
