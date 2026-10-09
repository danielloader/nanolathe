package units

import (
	"github.com/nanolathe-gg/nanolathe/internal/cob"
	"github.com/nanolathe-gg/nanolathe/internal/content"
	"github.com/nanolathe-gg/nanolathe/internal/sim/checkpoint"
	"github.com/nanolathe-gg/nanolathe/internal/sim/rng"
	"github.com/nanolathe-gg/nanolathe/internal/world"
	"github.com/nanolathe-gg/nanolathe/vfs"
)

const (
	checkpointWorldAttachment = iota
	checkpointWorldBinder
	checkpointWorldSource
	checkpointWorldLoader
	checkpointWorldExtraction
	checkpointWorldPose
	checkpointWorldRNG
)

// Each slot retains its exact installation owner. Copying a World or reading
// its bindings cannot transfer this proof (DESIGN_MULTIPLAYER §16.3.56).
type checkpointWorldProof struct {
	owner     *World
	authority *checkpoint.BindingAuthority
}

type checkpointWorldContext struct {
	world      *World
	inputs     *content.SimulationInputs
	terrain    *world.Terrain
	simulation *rng.Simulation
	loader     *cob.CachedLoader
	authority  *checkpoint.BindingAuthority
}

// AttachmentObserver returns the current owner without transferring proof.
func (w *World) AttachmentObserver() AttachmentObserver {
	if w == nil {
		return nil
	}
	return w.attachmentObserver
}

// CreationPose returns the current correction owner without transferring proof.
func (w *World) CreationPose() CreationPose {
	if w == nil {
		return nil
	}
	return w.pose
}

// SetAttachmentObserverWithCheckpointBinding installs a reviewed session owner.
// Capture does not call it; session admission checks the concrete movement alias.
func (w *World) SetAttachmentObserverWithCheckpointBinding(observer AttachmentObserver, authority *checkpoint.BindingAuthority) {
	w.SetAttachmentObserver(observer)
	if w != nil && observer != nil && authority != nil {
		w.checkpointBindings[checkpointWorldAttachment] = checkpointWorldProof{w, authority}
	}
}

// SetCOBSourceWithCheckpointBinding preserves the ordinary installation and
// independently stamps the filesystem and loader slots. Capture never reads
// the cache; the compiled-program proof makes its fallback unreachable.
func (w *World) SetCOBSourceWithCheckpointBinding(fs vfs.FSOps, loader *cob.CachedLoader, authority *checkpoint.BindingAuthority) {
	w.SetCOBSource(fs, loader)
	if w != nil && authority != nil {
		if fs != nil {
			w.checkpointBindings[checkpointWorldSource] = checkpointWorldProof{w, authority}
		}
		if loader != nil {
			w.checkpointBindings[checkpointWorldLoader] = checkpointWorldProof{w, authority}
		}
	}
}

// SetCOBBinderWithCheckpointBinding retains the actual filesystem captured by
// the reviewed binder, independently of SetCOBSource's current source.
func (w *World) SetCOBBinderWithCheckpointBinding(binder COBBinder, fs vfs.FSOps, authority *checkpoint.BindingAuthority) {
	w.SetCOBBinder(binder)
	if w != nil && binder != nil && authority != nil {
		w.checkpointBindings[checkpointWorldBinder] = checkpointWorldProof{w, authority}
		w.checkpointBinderSource = fs
	}
}

// SetExtractionSamplerWithCheckpointBinding stamps the existing installation;
// capture separately requires the exact concrete terrain from its context.
func (w *World) SetExtractionSamplerWithCheckpointBinding(s ExtractionSampler, authority *checkpoint.BindingAuthority) {
	w.SetExtractionSampler(s)
	if w != nil && s != nil && authority != nil {
		w.checkpointBindings[checkpointWorldExtraction] = checkpointWorldProof{w, authority}
	}
}

// SetCreationPoseWithCheckpointBinding installs a reviewed movement owner.
// Session admission checks its concrete alias without running its correction.
func (w *World) SetCreationPoseWithCheckpointBinding(p CreationPose, authority *checkpoint.BindingAuthority) {
	w.SetCreationPose(p)
	if w != nil && p != nil && authority != nil {
		w.checkpointBindings[checkpointWorldPose] = checkpointWorldProof{w, authority}
	}
}

// SetSimulationRNGWithCheckpointBinding stamps the session's current stream;
// capture checks its exact pointer without drawing from it.
func (w *World) SetSimulationRNGWithCheckpointBinding(sim *rng.Simulation, authority *checkpoint.BindingAuthority) {
	w.SetSimulationRNG(sim)
	if w != nil && sim != nil && authority != nil {
		w.checkpointBindings[checkpointWorldRNG] = checkpointWorldProof{w, authority}
	}
}

// SetWorldBindings records capture-local expectations without blessing live
// bindings. Nil expected terrain, RNG and loader are permitted fixture values;
// World, frozen inputs and authority are required (DESIGN_MULTIPLAYER §16.3.56).
func (c *CheckpointContext) SetWorldBindings(w *World, inputs *content.SimulationInputs, terrain *world.Terrain, simulation *rng.Simulation, loader *cob.CachedLoader, authority *checkpoint.BindingAuthority) error {
	if c == nil || w == nil || inputs == nil || authority == nil {
		return unitCheckpointError("units.bindings", "a context, exact world, frozen inputs and binding authority")
	}
	next := checkpointWorldContext{w, inputs, terrain, simulation, loader, authority}
	if c.worldBindings.world != nil && c.worldBindings != next {
		return unitCheckpointError("units.bindings", "one consistent world binding context")
	}
	if c.lifecycleWorld != nil && (c.lifecycleWorld != w || !c.lifecycleAuthority.Matches(authority)) {
		return unitCheckpointError("units.bindings", "the same world and authority as the lifecycle bindings")
	}
	c.worldBindings = next
	return nil
}

func (w *World) validateCheckpointBindings(c *CheckpointContext) error {
	bound := c.worldBindings
	if bound.world != nil && bound.world != w {
		return unitCheckpointError("units.bindings", "the registered world")
	}
	// Presence is a nil check only. The observation-only readinessObserver is
	// excluded; no open interface method proves ownership (§16.3.56).
	for i, binding := range [...]struct {
		name string
		set  bool
	}{
		{"attachmentObserver", w.attachmentObserver != nil},
		{"cobBinder", w.cobBinder != nil},
		{"cobFS", w.cobFS != nil},
		{"cobLoader", w.cobLoader != nil},
		{"extraction", w.extraction != nil},
		{"pose", w.pose != nil},
		{"simulationRNG", w.simulationRNG != nil},
	} {
		proof := w.checkpointBindings[i]
		if binding.set && (bound.world != w || proof.owner != w || !proof.authority.Matches(bound.authority)) {
			return unitCheckpointError("units.World."+binding.name, "the exact world's attested gameplay binding")
		}
	}
	if bound.world == nil {
		return nil
	}
	if w.catalog != bound.inputs.Catalog() {
		return unitCheckpointError("units.World.catalog", "the registered frozen catalog")
	}
	if err := c.Keys.ValidateCompiledAllocationPrograms(w.catalog); err != nil {
		return err
	}
	if w.cobLoader != bound.loader {
		return unitCheckpointError("units.World.cobLoader", "the registered loader")
	}
	if w.simulationRNG != bound.simulation {
		return unitCheckpointError("units.World.simulationRNG", "the registered simulation RNG")
	}
	if w.cobFS != nil && !bound.inputs.CheckpointFilesystemMatches(w.cobFS) {
		return unitCheckpointError("units.World.cobFS", "the exact frozen filesystem")
	}
	if w.cobBinder != nil && !bound.inputs.CheckpointFilesystemMatches(w.checkpointBinderSource) {
		return unitCheckpointError("units.World.cobBinder", "the exact frozen filesystem captured by the binder")
	}
	if w.extraction != nil {
		terrain, ok := w.extraction.(*world.Terrain)
		if !ok || terrain == nil || terrain != bound.terrain {
			return unitCheckpointError("units.World.extraction", "the exact registered terrain")
		}
	}
	return nil
}
