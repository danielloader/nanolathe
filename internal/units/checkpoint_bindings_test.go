package units

import (
	"bytes"
	"errors"
	"reflect"
	"testing"

	"github.com/nanolathe-gg/nanolathe/internal/cob"
	"github.com/nanolathe-gg/nanolathe/internal/content"
	"github.com/nanolathe-gg/nanolathe/internal/sim/checkpoint"
	"github.com/nanolathe-gg/nanolathe/internal/sim/rng"
	"github.com/nanolathe-gg/nanolathe/internal/world"
	"github.com/nanolathe-gg/nanolathe/vfs"
)

type checkpointWorldMover struct{ calls int }

func (m *checkpointWorldMover) AttachmentChanged(*Unit) {
	m.calls++
	panic("capture called attachment")
}
func (m *checkpointWorldMover) CorrectCreatedPose(*Unit) { m.calls++; panic("capture called pose") }

// The slice makes arbitrary interface equality unsafe. No method may run.
type checkpointWorldHostile []int

func (checkpointWorldHostile) SampleMetalWithFootprintSum(int32, int32, int, int, float32) (float32, uint16, error) {
	panic("capture called extraction")
}
func (checkpointWorldHostile) AttachmentChanged(*Unit)  { panic("capture called attachment") }
func (checkpointWorldHostile) CorrectCreatedPose(*Unit) { panic("capture called pose") }

type checkpointWorldFixture struct {
	w         *World
	inputs    *content.SimulationInputs
	keys      *content.CheckpointKeys
	authority *checkpoint.BindingAuthority
	terrain   *world.Terrain
	sim       *rng.Simulation
	loader    *cob.CachedLoader
	mover     *checkpointWorldMover
	binder    COBBinder
}

func newCheckpointWorldFixture(t *testing.T) checkpointWorldFixture {
	t.Helper()
	w, inputs := checkpointFixture(t)
	keys, err := inputs.CheckpointKeys()
	if err != nil {
		t.Fatal(err)
	}
	sim := rng.NewSimulation(73)
	return checkpointWorldFixture{
		w: w, inputs: inputs, keys: keys, authority: checkpoint.NewBindingAuthority(),
		terrain: &world.Terrain{}, sim: &sim, loader: cob.NewCachedLoader(), mover: &checkpointWorldMover{},
		binder: func(*Unit) error { panic("capture called binder") },
	}
}

func (f checkpointWorldFixture) context(t *testing.T) *CheckpointContext {
	t.Helper()
	c := NewCheckpointContext(f.keys)
	if err := c.SetWorldBindings(f.w, f.inputs, f.terrain, f.w.simulationRNG, f.w.cobLoader, f.authority); err != nil {
		t.Fatal(err)
	}
	return c
}

func (f checkpointWorldFixture) install() {
	f.w.SetAttachmentObserverWithCheckpointBinding(f.mover, f.authority)
	f.w.SetCOBBinderWithCheckpointBinding(f.binder, f.inputs.Filesystem(), f.authority)
	f.w.SetCOBSourceWithCheckpointBinding(f.inputs.Filesystem(), f.loader, f.authority)
	f.w.SetExtractionSamplerWithCheckpointBinding(f.terrain, f.authority)
	f.w.SetCreationPoseWithCheckpointBinding(f.mover, f.authority)
	f.w.SetSimulationRNGWithCheckpointBinding(f.sim, f.authority)
}

func checkpointWorldRefused(t *testing.T, w *World, c *CheckpointContext) {
	t.Helper()
	if _, err := w.CollectCheckpointReferences(c); err == nil {
		t.Fatal("collector accepted unproved bindings")
	}
	var out bytes.Buffer
	e := checkpoint.NewEncoder(&out)
	if err := w.WriteCheckpoint(e, c); err == nil || out.Len() != 0 {
		t.Fatal("writer emitted bytes for unproved bindings")
	}
	e.U8(1)
	if out.Len() != 0 {
		t.Fatal("refusal was not sticky")
	}
}

func TestCheckpointWorldBindingsPresenceAndIndependentReplacement(t *testing.T) {
	// Literal offsets in the empty limit-2 fixture: four lifecycle bytes,
	// four world bindings, ten u32 creation counts, extraction, u64 serial,
	// ten i64 live counts, and the 21-slot allocator before pose and RNG.
	for index, tc := range []struct {
		name      string
		offset    int
		install   func(checkpointWorldFixture, *checkpoint.BindingAuthority)
		reinstall func(checkpointWorldFixture)
		clear     func(checkpointWorldFixture)
	}{
		{"attachment", 4, func(f checkpointWorldFixture, a *checkpoint.BindingAuthority) {
			f.w.SetAttachmentObserverWithCheckpointBinding(f.mover, a)
		}, func(f checkpointWorldFixture) { f.w.SetAttachmentObserver(f.w.AttachmentObserver()) }, func(f checkpointWorldFixture) { f.w.SetAttachmentObserver(nil) }},
		{"binder", 5, func(f checkpointWorldFixture, a *checkpoint.BindingAuthority) {
			f.w.SetCOBBinderWithCheckpointBinding(f.binder, f.inputs.Filesystem(), a)
		}, func(f checkpointWorldFixture) { f.w.SetCOBBinder(f.binder) }, func(f checkpointWorldFixture) { f.w.SetCOBBinder(nil) }},
		{"source", 6, func(f checkpointWorldFixture, a *checkpoint.BindingAuthority) {
			f.w.SetCOBSourceWithCheckpointBinding(f.inputs.Filesystem(), nil, a)
		}, func(f checkpointWorldFixture) { f.w.SetCOBSource(f.inputs.Filesystem(), nil) }, func(f checkpointWorldFixture) { f.w.SetCOBSource(nil, nil) }},
		{"loader", 7, func(f checkpointWorldFixture, a *checkpoint.BindingAuthority) {
			f.w.SetCOBSourceWithCheckpointBinding(nil, f.loader, a)
		}, func(f checkpointWorldFixture) { f.w.SetCOBSource(nil, f.loader) }, func(f checkpointWorldFixture) { f.w.SetCOBSource(nil, nil) }},
		{"extraction", 48, func(f checkpointWorldFixture, a *checkpoint.BindingAuthority) {
			f.w.SetExtractionSamplerWithCheckpointBinding(f.terrain, a)
		}, func(f checkpointWorldFixture) { f.w.SetExtractionSampler(f.terrain) }, func(f checkpointWorldFixture) { f.w.SetExtractionSampler(nil) }},
		{"pose", 376, func(f checkpointWorldFixture, a *checkpoint.BindingAuthority) {
			f.w.SetCreationPoseWithCheckpointBinding(f.mover, a)
		}, func(f checkpointWorldFixture) { f.w.SetCreationPose(f.w.CreationPose()) }, func(f checkpointWorldFixture) { f.w.SetCreationPose(nil) }},
		{"RNG", 377, func(f checkpointWorldFixture, a *checkpoint.BindingAuthority) {
			f.w.SetSimulationRNGWithCheckpointBinding(f.sim, a)
		}, func(f checkpointWorldFixture) { f.w.SetSimulationRNG(f.sim) }, func(f checkpointWorldFixture) { f.w.SetSimulationRNG(nil) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newCheckpointWorldFixture(t)
			baseline := unitCallbackBytes(t, f.w, NewCheckpointContext(f.keys))
			if got := unitCallbackBytes(t, f.w, f.context(t)); !bytes.Equal(got, baseline) {
				t.Fatal("registration changed absent fixture bytes")
			}
			tc.install(f, f.authority)
			c := f.context(t)
			want := bytes.Clone(baseline)
			want[tc.offset] = 1
			if got := unitCallbackBytes(t, f.w, c); !bytes.Equal(got, want) {
				t.Fatal("binding changed more than its own presence byte")
			}
			checkpointWorldRefused(t, f.w, NewCheckpointContext(f.keys))
			copied := *f.w
			copyContext := NewCheckpointContext(f.keys)
			if err := copyContext.SetWorldBindings(&copied, f.inputs, f.terrain, f.w.simulationRNG, f.w.cobLoader, f.authority); err != nil {
				t.Fatal(err)
			}
			checkpointWorldRefused(t, &copied, copyContext)
			proofs := f.w.checkpointBindings
			tc.reinstall(f)
			checkpointWorldRefused(t, f.w, c)
			// Reinstall does not transfer proof, while an attested replacement
			// restores precisely the same canonical presence encoding.
			tc.install(f, f.authority)
			if f.w.checkpointBindings != proofs || !bytes.Equal(unitCallbackBytes(t, f.w, c), want) {
				t.Fatal("attested reinstall changed identity or bytes")
			}
			for _, a := range []*checkpoint.BindingAuthority{nil, checkpoint.NewBindingAuthority()} {
				tc.install(f, a)
				checkpointWorldRefused(t, f.w, c)
			}
			tc.clear(f)
			if got := unitCallbackBytes(t, f.w, f.context(t)); !bytes.Equal(got, baseline) {
				t.Fatal("cleared binding changed nil fixture framing")
			}
			f.install()
			proofs = f.w.checkpointBindings
			tc.reinstall(f)
			proofs[index] = checkpointWorldProof{}
			if tc.name == "source" || tc.name == "loader" {
				proofs[checkpointWorldSource], proofs[checkpointWorldLoader] = checkpointWorldProof{}, checkpointWorldProof{}
			}
			if f.w.checkpointBindings != proofs {
				t.Fatal("ordinary reinstall changed an independent slot's proof")
			}
		})
	}
}

func TestCheckpointWorldBindingsOwnerAndOperandIdentity(t *testing.T) {
	for _, tc := range []struct {
		name string
		edit func(*checkpointWorldFixture)
	}{
		{"catalog copy", func(f *checkpointWorldFixture) { copied := *f.w.catalog; f.w.catalog = &copied }},
		{"loader", func(f *checkpointWorldFixture) {
			f.w.SetCOBSourceWithCheckpointBinding(f.inputs.Filesystem(), cob.NewCachedLoader(), f.authority)
		}},
		{"missing loader", func(f *checkpointWorldFixture) {
			f.w.SetCOBSourceWithCheckpointBinding(f.inputs.Filesystem(), nil, f.authority)
		}},
		{"RNG", func(f *checkpointWorldFixture) {
			copied := *f.sim
			f.w.SetSimulationRNGWithCheckpointBinding(&copied, f.authority)
		}},
		{"missing RNG", func(f *checkpointWorldFixture) { f.w.SetSimulationRNGWithCheckpointBinding(nil, f.authority) }},
		{"terrain copy", func(f *checkpointWorldFixture) {
			copied := *f.terrain
			f.w.SetExtractionSamplerWithCheckpointBinding(&copied, f.authority)
		}},
		{"typed nil terrain", func(f *checkpointWorldFixture) {
			f.w.SetExtractionSamplerWithCheckpointBinding((*world.Terrain)(nil), f.authority)
		}},
		{"unknown terrain", func(f *checkpointWorldFixture) {
			f.w.SetExtractionSamplerWithCheckpointBinding(checkpointWorldHostile{1}, f.authority)
		}},
		{"wrapped source", func(f *checkpointWorldFixture) {
			f.w.SetCOBSourceWithCheckpointBinding(struct{ vfs.FSOps }{f.inputs.Filesystem()}, f.loader, f.authority)
		}},
		{"hostile source", func(f *checkpointWorldFixture) {
			f.w.SetCOBSourceWithCheckpointBinding(struct{ vfs.FSOps }{}, f.loader, f.authority)
		}},
		{"typed nil source", func(f *checkpointWorldFixture) {
			f.w.SetCOBSourceWithCheckpointBinding((*vfs.FS)(nil), f.loader, f.authority)
		}},
		{"binder source", func(f *checkpointWorldFixture) {
			f.w.SetCOBBinderWithCheckpointBinding(f.binder, struct{ vfs.FSOps }{}, f.authority)
		}},
		{"missing binder source", func(f *checkpointWorldFixture) { f.w.SetCOBBinderWithCheckpointBinding(f.binder, nil, f.authority) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newCheckpointWorldFixture(t)
			f.install()
			c := f.context(t)
			unitCallbackBytes(t, f.w, c)
			tc.edit(&f)
			checkpointWorldRefused(t, f.w, c)
		})
	}
	f := newCheckpointWorldFixture(t)
	f.install()
	copied := *f.w
	c := NewCheckpointContext(f.keys)
	if err := c.SetWorldBindings(&copied, f.inputs, f.terrain, f.sim, f.loader, f.authority); err != nil {
		t.Fatal(err)
	}
	checkpointWorldRefused(t, &copied, c)
	// Even an absent world cannot be used with a foreign registration.
	foreign := NewSliced(2, f.inputs.Catalog())
	checkpointWorldRefused(t, foreign, f.context(t))
	_, foreignInputs := checkpointFixture(t)
	for _, binder := range []bool{false, true} {
		f.install()
		if binder {
			f.w.SetCOBBinderWithCheckpointBinding(f.binder, foreignInputs.Filesystem(), f.authority)
		} else {
			f.w.SetCOBSourceWithCheckpointBinding(foreignInputs.Filesystem(), f.loader, f.authority)
		}
		checkpointWorldRefused(t, f.w, f.context(t))
	}
}

func TestCheckpointWorldBindingsRegistration(t *testing.T) {
	f := newCheckpointWorldFixture(t)
	c := NewCheckpointContext(f.keys)
	register := func(c *CheckpointContext, w *World, in *content.SimulationInputs, a *checkpoint.BindingAuthority) error {
		return c.SetWorldBindings(w, in, f.terrain, f.sim, f.loader, a)
	}
	for _, err := range []error{register(nil, f.w, f.inputs, f.authority), register(c, nil, f.inputs, f.authority), register(c, f.w, nil, f.authority), register(c, f.w, f.inputs, nil)} {
		if err == nil || c.worldBindings != (checkpointWorldContext{}) {
			t.Fatal("invalid registration succeeded or changed context")
		}
	}
	if err := register(c, f.w, f.inputs, f.authority); err != nil {
		t.Fatal(err)
	}
	if err := register(c, f.w, f.inputs, f.authority); err != nil {
		t.Fatal("same registration refused", err)
	}
	before := c.worldBindings
	for _, tc := range []struct {
		name string
		edit func(*checkpointWorldContext)
	}{
		{"world", func(v *checkpointWorldContext) { v.world = &World{} }},
		{"inputs", func(v *checkpointWorldContext) { v.inputs = &content.SimulationInputs{} }},
		{"terrain", func(v *checkpointWorldContext) { v.terrain = &world.Terrain{} }},
		{"RNG", func(v *checkpointWorldContext) { v.simulation = &rng.Simulation{} }},
		{"loader", func(v *checkpointWorldContext) { v.loader = cob.NewCachedLoader() }},
		{"authority", func(v *checkpointWorldContext) { v.authority = checkpoint.NewBindingAuthority() }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			next := before
			tc.edit(&next)
			if err := c.SetWorldBindings(next.world, next.inputs, next.terrain, next.simulation, next.loader, next.authority); err == nil || c.worldBindings != before {
				t.Fatal("conflicting registration succeeded or changed context")
			}
		})
	}
	for _, worldFirst := range []bool{false, true} {
		for _, wrongOwner := range []bool{false, true} {
			c := NewCheckpointContext(f.keys)
			w, a := f.w, checkpoint.NewBindingAuthority()
			if wrongOwner {
				w, a = &World{}, f.authority
			}
			if worldFirst {
				if err := register(c, f.w, f.inputs, f.authority); err != nil {
					t.Fatal(err)
				}
				if err := c.SetLifecycleBindings(w, a); err == nil || c.lifecycleWorld != nil {
					t.Fatal("conflicting lifecycle registration accepted")
				}
			} else {
				if err := c.SetLifecycleBindings(f.w, f.authority); err != nil {
					t.Fatal(err)
				}
				if err := register(c, w, f.inputs, a); err == nil || c.worldBindings.world != nil {
					t.Fatal("conflicting world registration accepted")
				}
			}
			if err := register(c, f.w, f.inputs, f.authority); err != nil {
				t.Fatal(err)
			}
			if err := c.SetLifecycleBindings(f.w, f.authority); err != nil {
				t.Fatal(err)
			}
		}
	}
}

func TestCheckpointWorldBindingsFuturePrograms(t *testing.T) {
	for _, tc := range []struct {
		name string
		edit func(*content.UnitDef)
	}{
		{"missing", func(u *content.UnitDef) { u.Script = nil }},
		{"empty", func(u *content.UnitDef) { u.Script.Code = nil }},
		{"code", func(u *content.UnitDef) { u.Script.Code[0]++ }},
		{"name", func(u *content.UnitDef) { u.UnitName += "changed" }},
		{"allocator ID", func(u *content.UnitDef) { u.UnitDefID++ }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newCheckpointWorldFixture(t)
			f.install()
			c := f.context(t)
			// No record has been allocated. Both collector and writer must
			// still reject a program that a later creation could consume.
			tc.edit(f.inputs.Catalog().UnitRecords()[1])
			checkpointWorldRefused(t, f.w, c)
		})
	}
	// The compiled-only subset applies to a registered production owner, not
	// to the pre-existing unregistered all-absent authored fixture boundary.
	f := newCheckpointWorldFixture(t)
	f.inputs.Catalog().UnitRecords()[0].Script = nil
	unitCallbackBytes(t, f.w, NewCheckpointContext(f.keys))
	checkpointWorldRefused(t, f.w, f.context(t))
}

func TestCheckpointWorldBindingsClearAndSlotIsolation(t *testing.T) {
	f := newCheckpointWorldFixture(t)
	f.install()
	before := f.w.checkpointBindings
	f.w.ClearAttachmentObserver(&checkpointWorldMover{})
	if f.w.checkpointBindings != before || f.w.AttachmentObserver() != f.mover {
		t.Fatal("unrelated clear changed current owner or proof")
	}
	f.w.ClearAttachmentObserver(f.mover)
	before[checkpointWorldAttachment] = checkpointWorldProof{}
	if f.w.checkpointBindings != before || f.w.AttachmentObserver() != nil {
		t.Fatal("successful clear did not clear exactly its own proof")
	}
	f.w.SetCOBSource(f.inputs.Filesystem(), f.loader)
	before[checkpointWorldSource], before[checkpointWorldLoader] = checkpointWorldProof{}, checkpointWorldProof{}
	if f.w.checkpointBindings != before || !f.inputs.CheckpointFilesystemMatches(f.w.checkpointBinderSource) {
		t.Fatal("source reinstall changed binder proof or its captured source")
	}
	f.w.SetCOBBinder(f.binder)
	before[checkpointWorldBinder] = checkpointWorldProof{}
	if f.w.checkpointBindings != before || f.w.checkpointBinderSource != nil {
		t.Fatal("binder reinstall retained source proof or changed another slot")
	}
	// Ordinary unattested hostile values refuse before any method or equality.
	f.install()
	f.w.SetAttachmentObserver(checkpointWorldHostile{1})
	unitCallbackRefused(t, f.w, f.context(t), "units.World.attachmentObserver")
	f.install()
	f.w.SetCreationPose(checkpointWorldHostile{1})
	unitCallbackRefused(t, f.w, f.context(t), "units.World.pose")
	var absent *World
	absent.SetAttachmentObserverWithCheckpointBinding(f.mover, f.authority)
	absent.SetCOBBinderWithCheckpointBinding(f.binder, f.inputs.Filesystem(), f.authority)
	absent.SetCOBSourceWithCheckpointBinding(f.inputs.Filesystem(), f.loader, f.authority)
	absent.SetExtractionSamplerWithCheckpointBinding(f.terrain, f.authority)
	absent.SetCreationPoseWithCheckpointBinding(f.mover, f.authority)
	absent.SetSimulationRNGWithCheckpointBinding(f.sim, f.authority)
	absent.ClearAttachmentObserver(f.mover)
	if absent.AttachmentObserver() != nil || absent.CreationPose() != nil {
		t.Fatal("nil world getters returned an owner")
	}
}

// Only cache setup uses this provider. Any access after setup panics; cached
// hits and misses must survive capture without reads, writes or invalidation.
type checkpointCacheSource struct {
	vfs.FSOps
	data []byte
	deny bool
}

func (s *checkpointCacheSource) ReadFileLimit(_ string, _ int64) ([]byte, error) {
	if s.deny {
		panic("capture touched the loader source")
	}
	if s.data == nil {
		return nil, errors.New("authored missing script")
	}
	return s.data, nil
}

func TestCheckpointWorldBindingsPureAndCacheIndependent(t *testing.T) {
	f := newCheckpointWorldFixture(t)
	poison := &checkpointCacheSource{data: unitCreateCOB()}
	hit, found, err := f.loader.Load(poison, "armdef")
	if err != nil || !found {
		t.Fatalf("cache setup: %v", err)
	}
	hit.Code[0]++ // Deliberately differs from the admitted compiled program.
	poison.data = nil
	if _, found, err := f.loader.Load(poison, "cordef"); err != nil || found {
		t.Fatalf("missing cache setup: %v", err)
	}
	poison.deny = true
	f.install()
	c := f.context(t)
	beforeProof, beforeRNG := f.w.checkpointBindings, *f.sim
	beforeRecords, beforeLoader := f.inputs.Catalog().UnitRecords(), f.w.cobLoader
	beforeManifest := f.inputs.Manifest()
	first := unitCallbackBytes(t, f.w, c)
	for range 3 {
		if got := unitCallbackBytes(t, f.w, c); !bytes.Equal(got, first) {
			t.Fatal("repeat capture changed bytes")
		}
	}
	if f.mover.calls != 0 || f.w.checkpointBindings != beforeProof || *f.sim != beforeRNG || f.w.cobLoader != beforeLoader ||
		!reflect.DeepEqual(beforeManifest, f.inputs.Manifest()) || !reflect.DeepEqual(beforeRecords, f.inputs.Catalog().UnitRecords()) {
		t.Fatal("capture changed owner, program, manifest, callbacks or RNG")
	}
	if got, found, err := f.loader.Load(poison, " ARMDEF "); err != nil || !found || got != hit {
		t.Fatalf("cached hit changed: %v", err)
	}
	if _, found, err := f.loader.Load(poison, "cordef"); err != nil || found {
		t.Fatalf("cached miss changed: %v", err)
	}
	// Later unrelated cache population does not change checkpoint bytes.
	poison.deny, poison.data = false, unitCreateCOB()
	if _, found, err := f.loader.Load(poison, "unrelated"); err != nil || !found {
		t.Fatalf("later cache setup: %v", err)
	}
	poison.deny = true
	if got := unitCallbackBytes(t, f.w, c); !bytes.Equal(got, first) {
		t.Fatal("cache population entered checkpoint bytes")
	}
}
