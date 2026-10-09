package cob

import (
	"fmt"
	"slices"

	"github.com/nanolathe-gg/nanolathe/internal/sim/checkpoint"
)

// A copied port row belongs to this exact VM/allocation/authority tuple. A
// shallow VM copy cannot borrow it; callback or pointer identity is never
// encoded. The parent attests the actual unit/program graph (§16.3.47).
type checkpointPortProof struct {
	vm         *VM
	allocation checkpoint.Allocation
	authority  *checkpoint.BindingAuthority

	// Only pending installs need receipt identity. It prevents an old receipt
	// from sealing a same-key replacement; it is never encoded (§16.3.51).
	installation *CheckpointPortInstallation
}

// BindPortWithCheckpointBinding performs the ordinary install once and stamps
// only its legacy slot. Invalid proof inputs leave the callback installed but
// unsupported, and a nil callback is absent (DESIGN_MULTIPLAYER §16.3.47).
func (v *VM) BindPortWithCheckpointBinding(port Port, fn func([]int32) int32, owner checkpoint.Allocation, authority *checkpoint.BindingAuthority) {
	v.BindPort(port, fn)
	if fn == nil || owner.Handle == 0 || owner.Serial == 0 || authority == nil {
		return
	}
	if v.checkpointPortFuncs == nil {
		v.checkpointPortFuncs = make(map[Port]checkpointPortProof)
	}
	v.checkpointPortFuncs[port] = checkpointPortProof{vm: v, allocation: owner, authority: authority}
}

// BindPortBindingWithCheckpointBinding stamps the copied explicit row after
// ordinary installation. Read and Write retain their independent fallback
// arms; installation invokes neither (DESIGN_MULTIPLAYER §16.3.47).
func (v *VM) BindPortBindingWithCheckpointBinding(port Port, binding PortBinding, owner checkpoint.Allocation, authority *checkpoint.BindingAuthority) {
	v.BindPortBinding(port, binding)
	if v == nil || (binding.Read == nil && binding.Write == nil) || owner.Handle == 0 || owner.Serial == 0 || authority == nil {
		return
	}
	if v.checkpointPortBindings == nil {
		v.checkpointPortBindings = make(map[Port]checkpointPortProof)
	}
	v.checkpointPortBindings[port] = checkpointPortProof{vm: v, allocation: owner, authority: authority}
}

// CheckpointPortInstallation names one exact provisional copied installation.
// All fields are private. Its identity, map family and key are diagnostic
// metadata only; neither receipt nor proof is a new wire field (§16.3.51).
// A copied receipt cannot seal the original installation.
type CheckpointPortInstallation struct {
	vm       *VM
	port     Port
	explicit bool
}

// BindPortWithPendingCheckpointBinding installs the legacy callback once,
// without predicting an allocation serial. Pending proof refuses capture until
// the units owner seals it after successful creation (§16.3.51).
func (v *VM) BindPortWithPendingCheckpointBinding(port Port, fn func([]int32) int32, authority *checkpoint.BindingAuthority) *CheckpointPortInstallation {
	v.BindPort(port, fn)
	if fn == nil || authority == nil {
		return nil
	}
	r := &CheckpointPortInstallation{vm: v, port: port}
	if v.checkpointPortFuncs == nil {
		v.checkpointPortFuncs = make(map[Port]checkpointPortProof)
	}
	v.checkpointPortFuncs[port] = checkpointPortProof{vm: v, authority: authority, installation: r}
	return r
}

// BindPortBindingWithPendingCheckpointBinding preserves explicit arm fallback
// and the ordinary nil-receiver no-op while recording pending proof (§16.3.51).
func (v *VM) BindPortBindingWithPendingCheckpointBinding(port Port, binding PortBinding, authority *checkpoint.BindingAuthority) *CheckpointPortInstallation {
	v.BindPortBinding(port, binding)
	if v == nil || (binding.Read == nil && binding.Write == nil) || authority == nil {
		return nil
	}
	r := &CheckpointPortInstallation{vm: v, port: port, explicit: true}
	if v.checkpointPortBindings == nil {
		v.checkpointPortBindings = make(map[Port]checkpointPortProof)
	}
	v.checkpointPortBindings[port] = checkpointPortProof{vm: v, authority: authority, installation: r}
	return r
}

// Seal records the successful allocator's actual identity on only the original
// still-installed proof. Replacements retire receipts even for the same
// callback. Refusals mutate nothing and never affect creation (§16.3.51).
func (r *CheckpointPortInstallation) Seal(owner checkpoint.Allocation) error {
	const path = "VM.bindings.portInstallation"
	if r == nil || r.vm == nil || owner.Handle == 0 || owner.Serial == 0 {
		return checkpointPortError(path, "a receipt and nonzero successful allocation")
	}
	proofs := r.vm.checkpointPortFuncs
	if r.explicit {
		proofs = r.vm.checkpointPortBindings
	}
	proof := proofs[r.port]
	if proof.installation != r || proof.vm != r.vm || proof.authority == nil {
		return checkpointPortError(path, "the original still-installed port proof")
	}
	if proof.allocation == owner {
		return nil
	}
	if proof.allocation != (checkpoint.Allocation{}) {
		return checkpointPortError(path, "the same successfully sealed allocation")
	}
	proof.allocation = owner
	proofs[r.port] = proof
	return nil
}

// SetOwnerBindings registers capture-local expectations, not live proof. It
// cannot attest a callback installed through an ordinary API (§16.3.47).
func (c *CheckpointContext) SetOwnerBindings(v *VM, owner checkpoint.Allocation, authority *checkpoint.BindingAuthority) error {
	if c == nil || v == nil || owner.Handle == 0 || owner.Serial == 0 || authority == nil {
		return checkpointPortError("VM.bindings.owner", "a context, VM, nonzero allocation and authority")
	}
	if c.ownerVM != nil && (c.ownerVM != v || c.ownerAllocation != owner || !c.bindingAuthority.Matches(authority)) {
		return checkpointPortError("VM.bindings.owner", "the same registered VM, allocation and authority")
	}
	c.ownerVM, c.ownerAllocation, c.bindingAuthority = v, owner, authority
	return nil
}

func (p checkpointPortProof) matches(v *VM, c *CheckpointContext) bool {
	return p.allocation.Handle != 0 && p.allocation.Serial != 0 &&
		c.ownerVM == v && p.vm == v && p.allocation == c.ownerAllocation && p.authority.Matches(c.bindingAuthority)
}

// checkpointPorts validates before the VM emits any payload. Keys alone are
// gathered and sorted; nil-only rows are omitted after sorting, as both read
// and write paths treat them as absent. Actual copied rows supply the shape;
// caller-owned request maps and PortBinding values are not retained.
func (v *VM) checkpointPorts(e *checkpoint.Encoder, c *CheckpointContext) ([]Port, []Port, error) {
	e.Field("VM.bindings.owner")
	if c.ownerVM != nil && c.ownerVM != v {
		e.Fail(checkpointPortError("VM.bindings.owner", "the registered VM"))
		return nil, nil, e.Err()
	}
	keys := checkpointPortKeys(v.portBindings)
	bindings := keys[:0]
	for _, port := range keys {
		b := v.portBindings[port]
		if b.Read == nil && b.Write == nil {
			continue
		}
		e.Field(fmt.Sprintf("VM.bindings.portBindings[%d]", port))
		if !v.checkpointPortBindings[port].matches(v, c) {
			e.Fail(checkpointPortError("VM.bindings.portBindings", "the copied row's exact VM, allocation and authority"))
			return nil, nil, e.Err()
		}
		bindings = append(bindings, port)
	}
	keys = checkpointPortKeys(v.portFuncs)
	funcs := keys[:0]
	for _, port := range keys {
		if v.portFuncs[port] == nil {
			continue
		}
		e.Field(fmt.Sprintf("VM.bindings.portFuncs[%d]", port))
		if !v.checkpointPortFuncs[port].matches(v, c) {
			e.Fail(checkpointPortError("VM.bindings.portFuncs", "the copied row's exact VM, allocation and authority"))
			return nil, nil, e.Err()
		}
		funcs = append(funcs, port)
	}
	return bindings, funcs, e.Err()
}

func checkpointPortKeys[V any](ports map[Port]V) []Port {
	keys := make([]Port, 0, len(ports))
	for port := range ports {
		keys = append(keys, port)
	}
	slices.Sort(keys)
	return keys
}

func checkpointPortError(path, expected string) error {
	return fmt.Errorf("nanolathe: checkpoint owner scripts: logical path %s, providers searched [], expected %s", path, expected)
}
