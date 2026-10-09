package units

import (
	"github.com/nanolathe-gg/nanolathe/internal/cob"
	"github.com/nanolathe-gg/nanolathe/internal/sim/checkpoint"
)

// Receipts belong to one exact Unit, including before a successful serial
// exists. A copied Unit retains the original owner and cannot seal its proof.
// Failure is diagnostic only; it never changes allocation or callback flow
// (DESIGN_MULTIPLAYER §16.3.51, §16.3.55, §16.3.60).
type checkpointUnitScriptInstallation struct {
	owner     *Unit
	authority *checkpoint.BindingAuthority
	vm        *cob.VM
	ports     []*cob.CheckpointPortInstallation
	runtime   []*cob.CheckpointVMInstallation
	err       error
}

func (u *Unit) beginCheckpointScriptInstallation(authority *checkpoint.BindingAuthority) {
	s := &u.checkpointScript
	if s.owner == nil && s.err == nil {
		s.owner, s.authority = u, authority
	} else if s.owner != u || !s.authority.Matches(authority) {
		u.failCheckpointScriptInstallation(unitCheckpointError("units.Script.installation", "the original unit and authority"))
	}
}

func (u *Unit) failCheckpointScriptInstallation(err error) {
	if u.checkpointScript.err == nil {
		u.checkpointScript.err = err
	}
}

// The admission belongs to the VM installed by PreCreate. An ordinary later
// SetScript must not inherit that authority when transport queries are bound.
func (u *Unit) bindCheckpointScriptVM(vm *cob.VM, authority *checkpoint.BindingAuthority) {
	s := &u.checkpointScript
	if !u.checkpointScriptReceiptOwner() || !s.authority.Matches(authority) || vm == nil || (s.vm != nil && s.vm != vm) {
		u.failCheckpointScriptInstallation(unitCheckpointError("units.Script.installation", "the original admitted VM and authority"))
		return
	}
	s.vm = vm
}

func (u *Unit) checkpointScriptAuthority(vm *cob.VM) *checkpoint.BindingAuthority {
	if u != nil && vm != nil && u.checkpointScript.owner == u && u.checkpointScript.vm == vm && u.checkpointScript.err == nil {
		return u.checkpointScript.authority
	}
	return nil
}

func (u *Unit) checkpointScriptReceiptOwner() bool {
	if u.checkpointScript.owner != u || u.checkpointScript.authority == nil {
		u.failCheckpointScriptInstallation(unitCheckpointError("units.Script.installation", "a prior admitted installation on this exact unit"))
		return false
	}
	return true
}

// RetainCheckpointPortInstallation ignores absent receipts. Present receipts
// seal immediately after allocation, or stay on this unit until its existing
// successful serial assignment. Refusal never affects gameplay (§16.3.60).
func (u *Unit) RetainCheckpointPortInstallation(receipt *cob.CheckpointPortInstallation) {
	if u == nil || receipt == nil || !u.checkpointScriptReceiptOwner() {
		return
	}
	if u.AllocationSerial != 0 {
		u.failCheckpointScriptInstallation(receipt.Seal(checkpoint.Allocation{Handle: uint32(u.Handle), Serial: u.AllocationSerial}))
	} else {
		u.checkpointScript.ports = append(u.checkpointScript.ports, receipt)
	}
}

// RetainCheckpointVMInstallation has the same lifetime as port receipts;
// runtime slots remain independent, and only the first diagnostic is kept.
func (u *Unit) RetainCheckpointVMInstallation(receipt *cob.CheckpointVMInstallation) {
	if u == nil || receipt == nil || !u.checkpointScriptReceiptOwner() {
		return
	}
	if u.AllocationSerial != 0 {
		u.failCheckpointScriptInstallation(receipt.Seal(checkpoint.Allocation{Handle: uint32(u.Handle), Serial: u.AllocationSerial}))
	} else {
		u.checkpointScript.runtime = append(u.checkpointScript.runtime, receipt)
	}
}

func (u *Unit) sealCheckpointScriptInstallations() {
	s := &u.checkpointScript
	if s.owner == nil && len(s.ports) == 0 && len(s.runtime) == 0 {
		return
	}
	if !u.checkpointScriptReceiptOwner() {
		return
	}
	owner := checkpoint.Allocation{Handle: uint32(u.Handle), Serial: u.AllocationSerial}
	for _, receipt := range s.ports {
		u.failCheckpointScriptInstallation(receipt.Seal(owner))
	}
	for _, receipt := range s.runtime {
		u.failCheckpointScriptInstallation(receipt.Seal(owner))
	}
	s.ports, s.runtime = nil, nil
}

type checkpointScriptBinding struct {
	vm         *cob.VM
	program    checkpoint.Definition
	allocation checkpoint.Allocation
	binding    *cob.Binding
	flags      []uint8
	lower      *cob.CheckpointContext
}

// ScriptBindings prepares one lower context after discovery. Parent admission
// registers its actual runtime sources and closed sinks immediately afterward.
// This never invokes a script, render getter or gameplay callback (§16.3.60).
func (c *CheckpointContext) ScriptBindings(u *Unit) (*cob.CheckpointContext, error) {
	key, err := c.checkpointScriptIdentity(u)
	if err != nil {
		return nil, err
	}
	if registered := c.scriptBindings[u]; registered != nil {
		if err := c.validateCheckpointScriptBinding(u, key, registered); err != nil {
			return nil, err
		}
		return registered.lower, nil
	}
	lower := &cob.CheckpointContext{Program: key}
	owner := checkpoint.Allocation{Handle: uint32(u.Handle), Serial: u.AllocationSerial}
	if err := lower.SetOwnerBindings(u.Script, owner, c.lifecycleAuthority); err != nil {
		return nil, err
	}
	var binding *cob.Binding
	if u.ScriptState != nil {
		binding = u.ScriptState.Binding
	}
	if c.scriptBindings == nil {
		c.scriptBindings = make(map[*Unit]*checkpointScriptBinding)
	}
	c.scriptBindings[u] = &checkpointScriptBinding{u.Script, key, owner, binding, u.RenderPieceFlags, lower}
	return lower, nil
}

func (c *CheckpointContext) checkpointScriptIdentity(u *Unit) (checkpoint.Definition, error) {
	if c == nil || c.Keys == nil || u == nil || u.Script == nil || c.lifecycleWorld == nil || c.lifecycleAuthority == nil {
		return checkpoint.Definition{}, unitCheckpointError("units.Script.context", "admitted keys, a discovered unit VM and exact lifecycle ownership")
	}
	if id, found := c.Allocations.Find(u); !found || id == 0 {
		return checkpoint.Definition{}, unitCheckpointError("units.Script.context", "a discovered allocation")
	}
	if id, found := c.VMs.Find(u.Script); !found || id == 0 {
		return checkpoint.Definition{}, unitCheckpointError("units.Script.context", "a discovered VM")
	}
	if err := c.lifecycleWorld.validateCheckpointAllocation(c, u, "units.Script.context"); err != nil {
		return checkpoint.Definition{}, err
	}
	if err := u.validateCheckpointScriptInstallation(c.lifecycleAuthority); err != nil {
		return checkpoint.Definition{}, err
	}
	return c.Keys.ProgramForUnit(u.Def, u.Script.Program())
}

func (u *Unit) validateCheckpointScriptInstallation(authority *checkpoint.BindingAuthority) error {
	s := u.checkpointScript
	if s.err != nil {
		return s.err
	}
	if s.owner != nil && (s.owner != u || !s.authority.Matches(authority) || s.vm == nil || s.vm != u.Script) {
		return unitCheckpointError("units.Script.installation", "the original unit, admitted VM and registered authority")
	}
	return nil
}

func (c *CheckpointContext) validateCheckpointScriptBinding(u *Unit, key checkpoint.Definition, b *checkpointScriptBinding) error {
	var binding *cob.Binding
	if u.ScriptState != nil {
		binding = u.ScriptState.Binding
	}
	flags := u.RenderPieceFlags
	if b.vm != u.Script || b.program != key || b.lower.Program != key ||
		b.allocation != (checkpoint.Allocation{Handle: uint32(u.Handle), Serial: u.AllocationSerial}) || b.binding != binding ||
		len(b.flags) != len(flags) || (len(flags) != 0 && &b.flags[0] != &flags[0]) {
		return unitCheckpointError("units.Script.context", "the unchanged unit VM, program, allocation, binding and render store")
	}
	return nil
}
