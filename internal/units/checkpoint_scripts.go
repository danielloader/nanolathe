package units

import (
	"fmt"
	"slices"

	"github.com/nanolathe-gg/nanolathe/internal/cob"
	"github.com/nanolathe-gg/nanolathe/internal/sim/checkpoint"
)

// WriteScriptCheckpoint writes section 4 after reference discovery has reached
// a fixed point (DESIGN_MULTIPLAYER §16.3.6). Roots retain Script and the
// ScriptState presence/binding/bridge fields; VM bodies appear only in table 4.
// Unit.RenderPieceFlags is already owned by section 2. Capture invokes no
// script, callback, render getter, file read or mutable pose-cache operation.
func (w *World) WriteScriptCheckpoint(e *checkpoint.Encoder, c *CheckpointContext) error {
	if w == nil || c == nil || c.Keys == nil {
		return scriptCheckpointFailure(e, "scripts", "a world and admitted reference context")
	}
	if c.lifecycleWorld != nil && c.lifecycleWorld != w {
		return scriptCheckpointFailure(e, "scripts.context", "the registered lifecycle world")
	}
	owners := make(map[*cob.VM]*Unit)
	allocations := c.Allocations.Values()
	for _, u := range allocations {
		if err := validateScriptCheckpoint(u, c); err != nil {
			e.Field("scripts.aliases")
			e.Fail(err)
			return e.Err()
		}
		if u.Script != nil {
			if id, known := c.VMs.Find(u.Script); !known || id == 0 {
				return scriptCheckpointFailure(e, "scripts.aliases", "a discovered unit VM")
			}
			if prior := owners[u.Script]; prior != nil && prior != u {
				return scriptCheckpointFailure(e, "scripts.aliases", "one allocation owner per unit VM")
			}
			owners[u.Script] = u
		}
	}
	vms := c.VMs.Values()
	for _, vm := range vms {
		if owners[vm] == nil {
			return scriptCheckpointFailure(e, "scripts.VMs", "a discovered VM associated with its allocation")
		}
	}
	e.Field("scripts.roots")
	e.Count(len(allocations))
	for i, u := range allocations {
		e.Field(fmt.Sprintf("scripts.roots[%d]", i))
		id, ok := c.Allocations.Find(u)
		if !ok || id == 0 {
			return scriptCheckpointFailure(e, "scripts.roots", "a discovered allocation")
		}
		writeScriptReference(e, 1, id)
		vmID, ok := c.VMs.Find(u.Script)
		if !ok {
			return scriptCheckpointFailure(e, "scripts.roots.Script", "a discovered VM")
		}
		writeScriptReference(e, 4, vmID)
		state := u.ScriptState
		e.Bool(state != nil)
		if state == nil {
			continue
		}
		// Binding's Program/VM/Callbacks and piece mapping are validated
		// aliases/derivations. Model is its immutable input edge; preflight
		// validated the four retained binding presence fields (§16.3.60).
		e.Bool(state.Binding != nil)
		if state.Binding != nil {
			key, err := c.Keys.Model(state.Binding.Model)
			if err != nil {
				e.Fail(err)
				return e.Err()
			}
			e.Definition(key)
			// PresentationSink, SFXSink, SFXVisible, SimulationRNG.
			e.Bool(state.Binding.PresentationSink != nil)
			e.Bool(state.Binding.SFXSink != nil)
			e.Bool(state.Binding.SFXVisibleReader() != nil)
			e.Bool(state.Binding.SimulationRNG != nil)
		}
		e.Bool(state.Bridge != nil)
		if state.Bridge != nil {
			if err := state.Bridge.WriteCheckpoint(e); err != nil {
				return err
			}
		}
	}
	e.Field("scripts.VMs")
	e.U16(4)
	e.Count(len(vms))
	for _, vm := range vms {
		u := owners[vm]
		if u == nil {
			return scriptCheckpointFailure(e, "scripts.VMs", "a discovered VM associated with its allocation")
		}
		key, err := c.Keys.ProgramForUnit(u.Def, vm.Program())
		if err != nil {
			e.Fail(err)
			return e.Err()
		}
		lower := &cob.CheckpointContext{Program: key}
		if registered := c.scriptBindings[u]; registered != nil {
			lower = registered.lower
		}
		if err := vm.WriteCheckpoint(e, lower); err != nil {
			return err
		}
	}
	return e.Err()
}

func writeScriptReference(e *checkpoint.Encoder, table uint16, id checkpoint.ObjectID) {
	e.U16(table)
	e.U32(uint32(id))
}

func scriptCheckpointFailure(e *checkpoint.Encoder, path, expected string) error {
	e.Field(path)
	e.Fail(fmt.Errorf("nanolathe: script checkpoint failed: logical path %s, providers searched [], expected %s", path, expected))
	return e.Err()
}

// ScriptState's aliases are not independent mutable objects. A bridge shared
// across differing VMs, or a piece mapping no longer derived from the admitted
// model/program, cannot be represented as the ordinary unit binding.
func validateScriptCheckpoint(u *Unit, c *CheckpointContext) error {
	bad := func(expected string) error {
		return fmt.Errorf("nanolathe: script checkpoint failed: logical path scripts.aliases, providers searched [], expected %s", expected)
	}
	if u == nil {
		return bad("a nonnil discovered allocation")
	}
	if err := u.validateCheckpointScriptInstallation(c.lifecycleAuthority); err != nil {
		return err
	}
	vm := u.Script
	state := u.ScriptState
	if state != nil {
		if state.VM != vm || (state.Bridge != nil && (vm == nil || state.Bridge.VM != vm)) {
			return bad("consistent unit, script-state and bridge VM aliases")
		}
		if binding := state.Binding; binding != nil {
			if vm == nil || binding.VM != vm || binding.Program != vm.Program() ||
				binding.Callbacks != state.Bridge || binding.Model == nil || state.Bridge == nil {
				return bad("consistent program, model, VM and callback binding aliases")
			}
			if _, err := c.Keys.Model(binding.Model); err != nil {
				return err
			}
			names := make([]string, len(binding.Model.Pieces))
			for i, piece := range binding.Model.Pieces {
				names[i] = piece.Name
			}
			if binding.Program == nil || !slices.Equal(binding.PieceMap, cob.LinkPieces(binding.Program.Pieces, names)) ||
				binding.CreateInvoked != state.Bridge.CreateInvoked() {
				return bad("piece links and creation marker derived from the admitted binding")
			}
			if c.scriptBindings[u] == nil && (binding.SimulationRNG != nil || binding.SFXSink != nil || binding.SFXVisibleReader() != nil || binding.PresentationSink != nil) {
				return bad("registered runtime sources for present binding callbacks")
			}
		}
	}
	if vm == nil {
		if c.scriptBindings[u] != nil {
			return bad("the registered unit VM")
		}
		return nil
	}
	key, err := c.Keys.ProgramForUnit(u.Def, vm.Program())
	if err != nil {
		return err
	}
	lower := &cob.CheckpointContext{Program: key}
	if registered := c.scriptBindings[u]; registered != nil {
		if _, err := c.checkpointScriptIdentity(u); err != nil {
			return err
		}
		if err := c.validateCheckpointScriptBinding(u, key, registered); err != nil {
			return err
		}
		lower = registered.lower
	}
	if err := vm.ValidateCheckpointBindings(lower); err != nil {
		return err
	}
	continuations, err := vm.CheckpointContinuations()
	if err != nil {
		return err
	}
	for _, continuation := range continuations {
		if continuation.Kind == cob.CheckpointContinuationNone {
			continue
		}
		if continuation.Kind != cob.CheckpointContinuationSlotAim ||
			continuation.Target != (checkpoint.Allocation{Handle: uint32(u.Handle), Serial: u.AllocationSerial}) ||
			int(continuation.WeaponSlot) >= len(u.Slots) {
			return bad("slot-aim completion targeting the associated unit allocation and weapon slot")
		}
	}
	return nil
}
