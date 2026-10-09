package cob

import (
	"io"

	"github.com/nanolathe-gg/nanolathe/internal/sim/checkpoint"
	"github.com/nanolathe-gg/nanolathe/internal/sim/rng"
	"github.com/nanolathe-gg/nanolathe/vfs"
)

const (
	checkpointVMCargoContains = iota
	checkpointVMCarrierIdentity
	checkpointVMRenderFlags
	checkpointVMScriptTouched
	checkpointVMSFXVisible
	checkpointVMTransportAttach
	checkpointVMTransportDrop
	checkpointVMSlotCount
)

// CheckpointVMInstallation identifies one original installation, independently
// of other VM slots. It is pending until the allocator supplies its successful
// identity. A Binding reader shares its VM receipt. These fields and captured
// render slice headers are proof only, never wire data (§16.3.55).
//
// Runtime dispositions: callback slots retain validated presence; renderFlags
// also retains direct/mapped kind and selects external pieceFlags source 0.
// RNG and sinks require exact structural aliases. Port maps retain §16.3.47
// framing. The parent's model/program admission validates PieceMap values;
// getter scratch is overwritten before use and is excluded. No reader, sink,
// adapter method or RNG operation runs during validation.
type CheckpointVMInstallation struct {
	vm         *VM
	slot       int
	binding    *Binding
	authority  *checkpoint.BindingAuthority
	allocation checkpoint.Allocation
	renderKind uint8
	flags      []uint8
	pieceMap   []int
}

// Seal changes only the original still-installed proof. A stale or copied
// receipt, zero allocation or conflicting repeat refuses atomically (§16.3.51).
func (r *CheckpointVMInstallation) Seal(owner checkpoint.Allocation) error {
	if r == nil || r.vm == nil || owner.Handle == 0 || owner.Serial == 0 {
		return checkpointPortError("VM.bindings.installation", "a receipt and nonzero successful allocation")
	}
	if r.slot < 0 || r.slot >= checkpointVMSlotCount || r.vm.checkpointBindingsProof[r.slot] != r || r.authority == nil ||
		(r.binding != nil && (r.binding.VM != r.vm || r.binding.checkpointSFXVisible != r)) {
		return checkpointPortError("VM.bindings.installation", "the original still-installed runtime proof")
	}
	if r.allocation == owner {
		return nil
	}
	if r.allocation != (checkpoint.Allocation{}) {
		return checkpointPortError("VM.bindings.installation", "the same successfully sealed allocation")
	}
	r.allocation = owner
	return nil
}

func (v *VM) checkpointInstallation(slot int, present bool, authority *checkpoint.BindingAuthority) *CheckpointVMInstallation {
	if v == nil || !present || authority == nil {
		return nil
	}
	r := &CheckpointVMInstallation{vm: v, slot: slot, authority: authority}
	v.checkpointBindingsProof[slot] = r
	return r
}

func (r *CheckpointVMInstallation) matches(v *VM, c *CheckpointContext) bool {
	return r != nil && r.vm == v && r.slot >= 0 && r.slot < checkpointVMSlotCount &&
		v.checkpointBindingsProof[r.slot] == r && c.ownerVM == v &&
		r.allocation.Handle != 0 && r.allocation.Serial != 0 && r.allocation == c.ownerAllocation &&
		r.authority.Matches(c.bindingAuthority)
}

// BindScriptTouchedWithPendingCheckpointBinding preserves the ordinary install
// and records only its own slot; it never raises the marker (§16.3.55).
func (v *VM) BindScriptTouchedWithPendingCheckpointBinding(fn func(), authority *checkpoint.BindingAuthority) *CheckpointVMInstallation {
	v.BindScriptTouched(fn)
	return v.checkpointInstallation(checkpointVMScriptTouched, fn != nil, authority)
}

// BindTransportQueriesWithPendingCheckpointBinding returns independent cargo
// and carrier receipts, in that order. Neither query is polled (§16.3.55).
func (v *VM) BindTransportQueriesWithPendingCheckpointBinding(cargo func(int32) bool, carrier func() int32, authority *checkpoint.BindingAuthority) [2]*CheckpointVMInstallation {
	v.BindTransportQueries(cargo, carrier)
	return [2]*CheckpointVMInstallation{
		v.checkpointInstallation(checkpointVMCargoContains, cargo != nil, authority),
		v.checkpointInstallation(checkpointVMCarrierIdentity, carrier != nil, authority),
	}
}

// BindTransportMutationsWithPendingCheckpointBinding returns independent attach
// and drop receipts, in that order, without performing either (§16.3.55).
func (v *VM) BindTransportMutationsWithPendingCheckpointBinding(attach func(int32, int32, int32), drop func(int32), authority *checkpoint.BindingAuthority) [2]*CheckpointVMInstallation {
	v.BindTransportMutations(attach, drop)
	return [2]*CheckpointVMInstallation{
		v.checkpointInstallation(checkpointVMTransportAttach, attach != nil, authority),
		v.checkpointInstallation(checkpointVMTransportDrop, drop != nil, authority),
	}
}

// SetSFXVisibleWithPendingCheckpointBinding preserves SetSFXVisible's nil-VM
// panic and leaves nil callbacks absent (§16.3.55).
func (v *VM) SetSFXVisibleWithPendingCheckpointBinding(fn func(int, int32) bool, authority *checkpoint.BindingAuthority) *CheckpointVMInstallation {
	v.SetSFXVisible(fn)
	return v.checkpointInstallation(checkpointVMSFXVisible, fn != nil, authority)
}

// BindRenderFlagsWithPendingCheckpointBinding retains the captured direct
// slice, including an explicitly bound empty store (§16.3.55).
func (v *VM) BindRenderFlagsWithPendingCheckpointBinding(flags []uint8, authority *checkpoint.BindingAuthority) *CheckpointVMInstallation {
	v.BindRenderFlags(flags)
	r := v.checkpointInstallation(checkpointVMRenderFlags, true, authority)
	if r != nil {
		r.renderKind, r.flags = 1, flags
	}
	return r
}

// BindRenderFlagHandlersWithPendingCheckpointBinding records the exact slice
// operands captured by the canonical mapped handlers. Mixed forms remain
// unsupported; the ordinary setter still installs them unchanged (§16.3.55).
func (v *VM) BindRenderFlagHandlersWithPendingCheckpointBinding(get func() []uint8, set func(int, uint8, bool) bool, flags []uint8, pieceMap []int, authority *checkpoint.BindingAuthority) *CheckpointVMInstallation {
	v.BindRenderFlagHandlers(get, set)
	r := v.checkpointInstallation(checkpointVMRenderFlags, get != nil && set != nil, authority)
	if r != nil {
		r.renderKind, r.flags, r.pieceMap = 2, flags, pieceMap
	}
	return r
}

// BindStrictWithCheckpointBinding stamps the copied reader before PreCreate
// can replace it. All ordinary binding work and callback order remain in the
// shared implementation. Failed creation supplies no sealable result (§16.3.55).
func BindStrictWithCheckpointBinding(fs vfs.FSOps, req BindingRequest, authority *checkpoint.BindingAuthority) (*Binding, *CheckpointVMInstallation, error) {
	var receipt *CheckpointVMInstallation
	b, err := bindStrict(fs, req, authority, &receipt)
	if err != nil {
		return nil, nil, err
	}
	return b, receipt, nil
}

// SetSFXSinkWithPendingCheckpointBinding performs the ordinary installation
// once and pairs only the copied Binding/VM visibility reader (§16.3.55).
func (b *Binding) SetSFXSinkWithPendingCheckpointBinding(sink SFXSink, visible func(int, int32) bool, authority *checkpoint.BindingAuthority) *CheckpointVMInstallation {
	b.SetSFXSink(sink, visible)
	return b.checkpointVisibleInstallation(authority)
}

func (b *Binding) checkpointVisibleInstallation(authority *checkpoint.BindingAuthority) *CheckpointVMInstallation {
	if b == nil || b.VM == nil {
		return nil
	}
	r := b.VM.checkpointInstallation(checkpointVMSFXVisible, b.sfxVisible != nil, authority)
	if r != nil {
		r.binding = b
		b.checkpointSFXVisible = r
	}
	return r
}

type checkpointRuntimeSources struct {
	registered bool
	binding    *Binding
	sim        *rng.Simulation
	flags      []uint8
	pieceMap   []int
}

// SetRuntimeSources records expected aliases after owner registration; it
// cannot attest ordinary callbacks or bless current mutable sinks (§16.3.55).
func (c *CheckpointContext) SetRuntimeSources(b *Binding, sim *rng.Simulation, flags []uint8, pieceMap []int) error {
	if c == nil || c.ownerVM == nil || c.bindingAuthority == nil || c.ownerAllocation.Handle == 0 || c.ownerAllocation.Serial == 0 ||
		(b != nil && b.VM != c.ownerVM) {
		return checkpointPortError("VM.bindings.sources", "registered ownership and the same Binding VM")
	}
	r := c.checkpointSources()
	if r.registered && (r.binding != b || r.sim != sim || !checkpointSliceAliases(r.flags, flags) || !checkpointSliceAliases(r.pieceMap, pieceMap)) {
		return checkpointPortError("VM.bindings.sources", "the same registered runtime sources")
	}
	if r.registered {
		return nil
	}
	c.runtimeSources = &checkpointRuntimeSources{registered: true, binding: b, sim: sim, flags: flags, pieceMap: pieceMap}
	return nil
}

func checkpointSliceAliases[T any](a, b []T) bool {
	return len(a) == len(b) && (len(a) == 0 || &a[0] == &b[0])
}

// These matchers are constructed only by the closed generic functions below.
// They perform a concrete pointer assertion and comparison, never a sink call.
type checkpointPresentationMatch struct{ matches func(PresentationSink) bool }
type checkpointExplosionMatch struct{ matches func(ExplosionSink) bool }

// SetCheckpointPresentationSink records an expected nonnil concrete pointer.
// Root admission verifies its session-owned fields before registration (§16.3.55).
func SetCheckpointPresentationSink[T any, P interface {
	*T
	PresentationSink
}](c *CheckpointContext, expected P) error {
	if c == nil || expected == nil {
		return checkpointPortError("VM.bindings.sfxSink", "a context and nonnil concrete presentation sink")
	}
	if c.presentationSink != nil {
		if !c.presentationSink.matches(expected) {
			return checkpointPortError("VM.bindings.sfxSink", "the same registered presentation sink")
		}
		return nil
	}
	c.presentationSink = &checkpointPresentationMatch{matches: func(actual PresentationSink) bool {
		p, ok := actual.(P)
		return ok && p == expected
	}}
	return nil
}

// SetCheckpointExplosionSink records a nonnil concrete pointer without calling
// any synchronous admission method (§16.3.55).
func SetCheckpointExplosionSink[T any, P interface {
	*T
	ExplosionSink
}](c *CheckpointContext, expected P) error {
	if c == nil || expected == nil {
		return checkpointPortError("VM.bindings.explosionSink", "a context and nonnil concrete explosion sink")
	}
	if c.explosionSink != nil {
		if !c.explosionSink.matches(expected) {
			return checkpointPortError("VM.bindings.explosionSink", "the same registered explosion sink")
		}
		return nil
	}
	c.explosionSink = &checkpointExplosionMatch{matches: func(actual ExplosionSink) bool {
		p, ok := actual.(P)
		return ok && p == expected
	}}
	return nil
}

// ExplosionSink returns the actual installed sink without proof or invocation.
func (v *VM) ExplosionSink() ExplosionSink {
	if v == nil {
		return nil
	}
	return v.explosionSink
}

// ValidateCheckpointBindings performs the same binding checks as the writer,
// without emitting bytes. Units uses it before collecting/writing the retained
// Binding fields. Program/model/bridge validation remains with units (§16.3.55).
func (v *VM) ValidateCheckpointBindings(c *CheckpointContext) error {
	if v == nil || c == nil {
		return checkpointPortError("VM.bindings", "a VM and checkpoint context")
	}
	e := checkpoint.NewEncoder(io.Discard)
	_, _, err := v.checkpointBindings(e, c)
	return err
}

func (v *VM) checkpointBindingPresence() [12]bool {
	return [12]bool{
		v.cargoContains != nil, v.carrierIdentity != nil, v.explosionSink != nil,
		false, false, // Port shape is validated and framed separately.
		v.renderFlagsBound || v.renderFlags != nil || v.renderFlagGet != nil || v.renderFlagSet != nil,
		v.scriptTouched != nil, v.sfxSink != nil, v.sfxVisible != nil,
		v.simRng != nil, v.transportAttach != nil, v.transportDrop != nil,
	}
}

func (v *VM) validateCheckpointRuntime(c *CheckpointContext) error {
	if c.ownerVM != nil && c.ownerVM != v {
		return checkpointPortError("VM.bindings.owner", "the registered VM")
	}
	present := v.checkpointBindingPresence()
	// Only callback slots use installation proof. RNG and interface sinks are
	// compared structurally below; render additionally checks captured stores.
	for slot, bindingIndex := range [...]int{0, 1, 5, 6, 8, 10, 11} {
		if present[bindingIndex] && !v.checkpointBindingsProof[slot].matches(v, c) {
			return checkpointPortError("VM.bindings."+checkpointBindingNames[bindingIndex], "the sealed exact VM, allocation and authority; TODO(M3-U6) for unattested bindings")
		}
	}
	s := c.checkpointSources()
	if s.registered && s.binding != nil && s.binding.VM != v {
		return checkpointPortError("VM.bindings.sources", "the registered Binding VM")
	}
	if present[5] {
		if err := v.validateCheckpointRender(c); err != nil {
			return err
		}
	}
	if v.simRng != s.sim || (s.binding != nil && s.binding.SimulationRNG != s.sim) {
		return checkpointPortError("VM.bindings.simRng", "the registered RNG alias")
	}
	if b := s.binding; b != nil {
		if (b.sfxVisible != nil) != (v.sfxVisible != nil) || (b.sfxVisible != nil &&
			(b.checkpointSFXVisible != v.checkpointBindingsProof[checkpointVMSFXVisible] || b.checkpointSFXVisible.binding != b)) {
			return checkpointPortError("VM.bindings.sfxVisible", "the paired Binding and VM reader installation")
		}
	}
	if c.presentationSink == nil {
		if v.sfxSink != nil || (s.binding != nil && (s.binding.PresentationSink != nil || s.binding.SFXSink != nil)) {
			return checkpointPortError("VM.bindings.sfxSink", "an admitted presentation sink; TODO(M3-U6) for other sinks")
		}
	} else {
		adapter, ok := v.sfxSink.(PresentationSinkAdapter)
		if c.ownerVM != v || !ok || !c.presentationSink.matches(adapter.Sink) {
			return checkpointPortError("VM.bindings.sfxSink", "the value adapter around the registered presentation sink")
		}
		if b := s.binding; b != nil {
			if !c.presentationSink.matches(b.PresentationSink) {
				return checkpointPortError("Binding.PresentationSink", "the registered presentation sink")
			}
			if b.SFXSink != nil {
				adapter, ok := b.SFXSink.(PresentationSinkAdapter)
				if !ok || !c.presentationSink.matches(adapter.Sink) {
					return checkpointPortError("Binding.SFXSink", "nil or the same value presentation adapter")
				}
			}
		}
	}
	if c.explosionSink == nil {
		if v.explosionSink != nil {
			return checkpointPortError("VM.bindings.explosionSink", "an admitted explosion sink; TODO(M3-U6) for other sinks")
		}
	} else if c.ownerVM != v || !c.explosionSink.matches(v.explosionSink) {
		return checkpointPortError("VM.bindings.explosionSink", "the registered explosion sink")
	}
	return nil
}

func (v *VM) validateCheckpointRender(c *CheckpointContext) error {
	r, s := v.checkpointBindingsProof[checkpointVMRenderFlags], c.checkpointSources()
	valid := s.registered && v.renderFlagsBound && checkpointSliceAliases(r.flags, s.flags)
	switch r.renderKind {
	case 1:
		valid = valid && v.renderFlagGet == nil && v.renderFlagSet == nil && checkpointSliceAliases(v.renderFlags, s.flags)
	case 2:
		valid = valid && v.renderFlags == nil && v.renderFlagGet != nil && v.renderFlagSet != nil && checkpointSliceAliases(r.pieceMap, s.pieceMap)
		if s.binding != nil {
			valid = valid && checkpointSliceAliases(r.pieceMap, s.binding.PieceMap)
		}
	default:
		valid = false
	}
	if !valid {
		return checkpointPortError("VM.bindings.renderFlags", "the captured direct or mapped external store")
	}
	return nil
}

func (c *CheckpointContext) checkpointSources() checkpointRuntimeSources {
	if c.runtimeSources != nil {
		return *c.runtimeSources
	}
	return checkpointRuntimeSources{}
}
