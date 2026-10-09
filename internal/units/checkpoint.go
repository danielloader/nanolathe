package units

import (
	"fmt"

	"github.com/nanolathe-gg/nanolathe/internal/cob"
	"github.com/nanolathe-gg/nanolathe/internal/content"
	"github.com/nanolathe-gg/nanolathe/internal/pool"
	"github.com/nanolathe-gg/nanolathe/internal/sim/checkpoint"
)

// CheckpointContext holds the capture-local reference tables shared by units,
// orders and scripts. It is discarded after capture and never becomes world
// state (DESIGN_MULTIPLAYER §16.3.6).
type CheckpointContext struct {
	Keys               *content.CheckpointKeys
	Allocations        checkpoint.References[*Unit]
	VMs                checkpoint.References[*cob.VM]
	lifecycleWorld     *World
	lifecycleAuthority *checkpoint.BindingAuthority
	worldBindings      checkpointWorldContext
	scriptBindings     map[*Unit]*checkpointScriptBinding
}

// NewCheckpointContext binds admitted immutable keys without reading the
// world or registering any graph roots.
func NewCheckpointContext(keys *content.CheckpointKeys) *CheckpointContext {
	return &CheckpointContext{Keys: keys}
}

// CollectCheckpointReferences registers physical allocations before scanning
// every discovered allocation, including retired objects held by another
// owner. Raw empty-slot residuals are values, never allocation graph roots.
// Only collectors add references (DESIGN_MULTIPLAYER §16.3.6).
func (w *World) CollectCheckpointReferences(c *CheckpointContext) (added int, err error) {
	if err := w.validateCheckpoint(c); err != nil {
		return 0, err
	}
	for _, u := range w.units[1:] {
		if u == nil {
			continue
		}
		_, known := c.Allocations.Find(u)
		if _, err := c.Allocations.Add(u); err != nil {
			return added, err
		}
		if !known {
			added++
		}
	}
	serials := make(map[uint64]*Unit)
	for i, u := range c.Allocations.Values() {
		path := fmt.Sprintf("units.allocations[%d]", i+1)
		if err := w.validateCheckpointAllocation(c, u, path); err != nil {
			return added, err
		}
		if previous := serials[u.AllocationSerial]; previous != nil && previous != u {
			return added, unitCheckpointError(path+".AllocationSerial", "a distinct successful allocation serial")
		}
		serials[u.AllocationSerial] = u
		vm, err := checkpointUnitVM(u, path)
		if err != nil {
			return added, err
		}
		_, known := c.VMs.Find(vm)
		if _, err := c.VMs.Add(vm); err != nil {
			return added, err
		}
		if !known {
			added++
		}
	}
	return added, nil
}

// WriteCheckpoint writes section 2 only. World fields, in lexical order:
// OnCapture, OnCreate, OnDeath, OnDeathExtra, attachmentObserver, cobBinder,
// cobFS, cobLoader, createdCounters, extraction, lastAllocationSerial,
// liveCounters, pool, pose, simulationRNG; then physical slot roots and
// allocation table 1. Lifecycle and per-unit callbacks encode absent 0 or
// attested present 1; world bindings use the same validated presence tags.
// Slot tags are never allocated 0, live 1, freed residual 2. A residual
// has only Handle, Kills, Owner, Remaining. Script/order edges and VM bodies
// belong to sections 3–4. Physical RenderPieceFlags are unit-owned here; only
// VM-local fallback flags belong to scripts. The complete Unit,
// Slot, Move and Attachment field lists appear in their writers below
// (DESIGN_MULTIPLAYER §16.3.5–§16.3.6).
func (w *World) WriteCheckpoint(e *checkpoint.Encoder, c *CheckpointContext) error {
	e.Field("units.World")
	if err := w.validateCheckpoint(c); err != nil {
		e.Fail(err)
		return e.Err()
	}
	e.Field("units.World.OnCapture")
	e.Bool(w.CaptureHook() != nil)
	e.Field("units.World.OnCreate")
	e.Bool(w.CreateHook() != nil)
	e.Field("units.World.OnDeath")
	e.Bool(w.DeathHook() != nil)
	e.Field("units.World.OnDeathExtra")
	e.Bool(w.DeathExtraHook() != nil)
	e.Field("units.World.attachmentObserver")
	e.Bool(w.attachmentObserver != nil)
	e.Field("units.World.cobBinder")
	e.Bool(w.cobBinder != nil)
	e.Field("units.World.cobFS")
	e.Bool(w.cobFS != nil)
	e.Field("units.World.cobLoader")
	e.Bool(w.cobLoader != nil)
	e.Field("units.World.createdCounters")
	for _, count := range w.createdCounters {
		e.U32(count)
	}
	e.Field("units.World.extraction")
	e.Bool(w.extraction != nil)
	e.Field("units.World.lastAllocationSerial")
	e.U64(w.lastAllocationSerial)
	e.Field("units.World.liveCounters")
	for _, count := range w.liveCounters {
		e.I64(int64(count))
	}
	if err := w.pool.WriteCheckpoint(e); err != nil {
		return err
	}
	e.Field("units.World.pose")
	e.Bool(w.pose != nil)
	e.Field("units.World.simulationRNG")
	e.Bool(w.simulationRNG != nil)
	e.Field("units.World.slots")
	e.Count(len(w.units))
	for i, u := range w.units {
		e.Field(fmt.Sprintf("units.World.slots[%d]", i))
		if u != nil {
			e.U8(1)
			id, known := c.Allocations.Find(u)
			if !known {
				e.Fail(unitCheckpointError("units.World.slots", "a discovered allocation reference"))
				return e.Err()
			}
			e.U16(1)
			e.U32(uint32(id))
		} else if residual := w.rawUnits[i]; residual != nil {
			e.U8(2)
			e.U32(uint32(residual.Handle))
			e.I32(residual.Kills)
			e.U8(residual.Owner)
			e.Field(fmt.Sprintf("units.World.slots[%d].Remaining", i))
			e.F32(residual.Remaining)
		} else {
			e.U8(0)
		}
	}
	allocations := c.Allocations.Values()
	e.Field("units.allocations")
	e.U16(1)
	e.Count(len(allocations))
	serials := make(map[uint64]*Unit)
	for i, u := range allocations {
		path := fmt.Sprintf("units.allocations[%d]", i+1)
		if err := w.validateCheckpointAllocation(c, u, path); err != nil {
			e.Fail(err)
			return e.Err()
		}
		if previous := serials[u.AllocationSerial]; previous != nil && previous != u {
			e.Fail(unitCheckpointError(path+".AllocationSerial", "a distinct successful allocation serial"))
			return e.Err()
		}
		serials[u.AllocationSerial] = u
		writeCheckpointUnit(e, c, u, path)
		if e.Err() != nil {
			return e.Err()
		}
	}
	return e.Err()
}

func (w *World) validateCheckpoint(c *CheckpointContext) error {
	if c == nil || c.Keys == nil {
		return unitCheckpointError("units.context", "admitted content keys")
	}
	if w == nil || w.pool == nil {
		return unitCheckpointError("units.World", "a unit world and its allocator")
	}
	if w.pendingAllocationSerials != 0 {
		return unitCheckpointError("units.World.pendingAllocationSerials", "no creation in progress")
	}
	if w.catalog == nil || !w.catalog.Finalized() {
		return unitCheckpointError("units.World.catalog", "a finalized admitted catalog; fixture identity maps are unsupported")
	}
	if err := w.validateCheckpointLifecycle(c); err != nil {
		return err
	}
	if err := w.validateCheckpointBindings(c); err != nil {
		return err
	}
	// Resolve in catalog record order. Looking up every admitted record and
	// comparing the matched count detects foreign map keys without ranging a
	// pointer-keyed map; first-use cache insertion order is not world state.
	matched := 0
	for _, def := range w.catalog.UnitRecords() {
		if def == nil {
			continue
		}
		ref, err := c.Keys.Unit(def)
		if err != nil {
			return err
		}
		index, member := w.catalog.UnitIndexOf(def)
		if !member || index == 0 || index > 65535 || index != ref.Ordinal {
			return unitCheckpointError("units.World.catalog", "admitted catalog positions in the allocator identity domain")
		}
		if id, ok := w.defMap[def]; ok {
			if id != uint16(index) {
				return unitCheckpointError("units.World.defMap", "catalog-derived definition identities")
			}
			matched++
		}
	}
	if matched != len(w.defMap) {
		return unitCheckpointError("units.World.defMap", "only admitted catalog definitions")
	}
	if len(w.units) != w.pool.TotalRecords() || len(w.rawUnits) != len(w.units) || len(w.units) == 0 {
		return unitCheckpointError("units.World.slots", "matching physical arena lengths")
	}
	for i, u := range w.units {
		raw := w.rawUnits[i]
		h := pool.Handle(i)
		if i == 0 {
			if u != nil || raw != nil {
				return unitCheckpointError("units.World.slots[0]", "an empty null sentinel")
			}
			continue
		}
		if u == nil {
			if w.pool.Alive(h) {
				return unitCheckpointError("units.World.slots", "a unit record for each occupied slot")
			}
			if raw != nil && (int(raw.Handle) != i || raw.Owner >= pool.PlayerCount) {
				return unitCheckpointError("units.World.rawUnits", "a residual with its physical handle and valid owner")
			}
			continue
		}
		if int(u.Handle) != i || !u.Alive || raw != u || !w.pool.Alive(h) || w.pool.DefID(h) != w.defMap[u.Def] {
			return unitCheckpointError("units.World.slots", "matching live, raw and allocator records")
		}
	}
	return nil
}

func (w *World) validateCheckpointAllocation(c *CheckpointContext, u *Unit, path string) error {
	if u == nil || u.Handle == 0 || int(u.Handle) >= len(w.units) || u.Owner >= pool.PlayerCount {
		return unitCheckpointError(path, "a reachable allocation with a physical handle and valid owner")
	}
	if u.AllocationSerial == 0 || u.AllocationSerial > w.lastAllocationSerial {
		return unitCheckpointError(path+".AllocationSerial", "a successful allocation serial from this battle")
	}
	if u.Def == nil {
		return unitCheckpointError(path+".Def", "an admitted unit definition")
	}
	if _, err := c.Keys.Unit(u.Def); err != nil {
		return err
	}
	return w.validateCheckpointUnitCallbacks(c, u, path)
}

// checkpointUnitVM checks actual aliases, never GetScript's fallback. The
// scripts owner separately validates VM ports, program/model identities,
// bridge state and continuations, and emits them once in section 4.
func checkpointUnitVM(u *Unit, path string) (*cob.VM, error) {
	vm := u.Script
	if state := u.ScriptState; state != nil {
		if state.VM != vm {
			return nil, unitCheckpointError(path+".ScriptState.VM", "the same VM as Script")
		}
		if state.Bridge != nil && state.Bridge.VM != vm {
			return nil, unitCheckpointError(path+".ScriptState.Bridge", "the unit's VM alias")
		}
		if binding := state.Binding; binding != nil {
			if vm == nil || binding.VM != vm || binding.Callbacks != state.Bridge || binding.Program != vm.Program() {
				return nil, unitCheckpointError(path+".ScriptState.Binding", "the unit's VM, program and bridge aliases")
			}
		}
	}
	return vm, nil
}

func unitCheckpointError(path, expected string) error {
	return fmt.Errorf("nanolathe: unit checkpoint: logical path %s, providers searched [], expected %s", path, expected)
}

// Retained Unit fields, in lexical order:
// Activated, Alive, AllocationSerial, Armored, Attachment, BobPhase.
// BuggerOff, BuildingState, Busy, CachedOccupancyX, CachedOccupancyZ, CurrentSample.
// DeathCause, Def, Dying, EngagementTarget, Flags, FootprintSizeX.
// FootprintSizeZ, Group, Handle, HasMover, Health, Hidden.
// InBuildStance, IsCloaked, Kills, LastDamageCause, LastDamageSide, MaxHealth.
// Move, MoveTier, Owner, ParalyzeExpire, Pending, PlacementIdent.
// PlacementIdx, PlacementUnitName, PriorSample, Remaining, RenderPieceFlags.
// RestoredMoveMode, RevealDeadline.
// SightCellX, SightCellZ, Slots, SpotMetal, StructureFacing, Stunned.
// X, Y, YardOpen, Z, deathExtraHookFired, deathHookFired.
// statusCue, yardTransaction (validated presence tags).
// Orders, Script and ScriptState are owned by sections 3–4.
// numeric.Fixed has an int64 underlying type; the schema preserves all of
// that stored value, independently of the fixed-point fractional scale.
func writeCheckpointUnit(e *checkpoint.Encoder, c *CheckpointContext, u *Unit, path string) {
	e.Field(path + ".Activated")
	e.Bool(u.Activated)
	e.Field(path + ".Alive")
	e.Bool(u.Alive)
	e.Field(path + ".AllocationSerial")
	e.U64(u.AllocationSerial)
	e.Field(path + ".Armored")
	e.Bool(u.Armored)
	e.Field(path + ".Attachment")
	writeCheckpointAttachment(e, &u.Attachment)
	e.Field(path + ".BobPhase")
	e.I16(u.BobPhase)
	e.Field(path + ".BuggerOff")
	e.Bool(u.BuggerOff)
	e.Field(path + ".BuildingState")
	e.Bool(u.BuildingState)
	e.Field(path + ".Busy")
	e.Bool(u.Busy)
	e.Field(path + ".CachedOccupancyX")
	e.I16(u.CachedOccupancyX)
	e.Field(path + ".CachedOccupancyZ")
	e.I16(u.CachedOccupancyZ)
	e.Field(path + ".CurrentSample")
	e.U8(u.CurrentSample)
	e.Field(path + ".DeathCause")
	e.U8(uint8(u.DeathCause))
	e.Field(path + ".Def")
	ref, err := c.Keys.Unit(u.Def)
	if err != nil {
		e.Fail(err)
		return
	}
	e.Bool(true)
	e.Definition(ref)
	e.Field(path + ".Dying")
	e.Bool(u.Dying)
	e.Field(path + ".EngagementTarget")
	e.U32(uint32(u.EngagementTarget))
	e.Field(path + ".Flags")
	e.U32(u.Flags)
	e.Field(path + ".FootprintSizeX")
	e.I16(u.FootprintSizeX)
	e.Field(path + ".FootprintSizeZ")
	e.I16(u.FootprintSizeZ)
	e.Field(path + ".Group")
	e.U8(u.Group)
	e.Field(path + ".Handle")
	e.U32(uint32(u.Handle))
	e.Field(path + ".HasMover")
	e.Bool(u.HasMover)
	e.Field(path + ".Health")
	e.I32(u.Health)
	e.Field(path + ".Hidden")
	e.Bool(u.Hidden)
	e.Field(path + ".InBuildStance")
	e.Bool(u.InBuildStance)
	e.Field(path + ".IsCloaked")
	e.Bool(u.IsCloaked)
	e.Field(path + ".Kills")
	e.I32(u.Kills)
	e.Field(path + ".LastDamageCause")
	e.U8(u.LastDamageCause)
	e.Field(path + ".LastDamageSide")
	e.U8(u.LastDamageSide)
	e.Field(path + ".MaxHealth")
	e.I32(u.MaxHealth)
	e.Field(path + ".Move")
	writeCheckpointMove(e, &u.Move)
	e.Field(path + ".MoveTier")
	e.U8(u.MoveTier)
	e.Field(path + ".Owner")
	e.U8(u.Owner)
	e.Field(path + ".ParalyzeExpire")
	e.U32(u.ParalyzeExpire)
	e.Field(path + ".Pending")
	e.U32(u.Pending)
	e.Field(path + ".PlacementIdent")
	e.String(u.PlacementIdent)
	e.Field(path + ".PlacementIdx")
	e.I64(int64(u.PlacementIdx))
	e.Field(path + ".PlacementUnitName")
	e.String(u.PlacementUnitName)
	e.Field(path + ".PriorSample")
	e.U8(u.PriorSample)
	e.Field(path + ".Remaining")
	e.F32(u.Remaining)
	e.Field(path + ".RenderPieceFlags")
	e.Bytes(u.RenderPieceFlags)
	e.Field(path + ".RestoredMoveMode")
	e.Bool(u.RestoredMoveMode)
	e.Field(path + ".RevealDeadline")
	e.U32(u.RevealDeadline)
	e.Field(path + ".SightCellX")
	e.I16(u.SightCellX)
	e.Field(path + ".SightCellZ")
	e.I16(u.SightCellZ)
	e.Field(path + ".Slots")
	for i := range u.Slots {
		writeCheckpointSlot(e, c, &u.Slots[i], fmt.Sprintf("%s.Slots[%d]", path, i))
	}
	e.Field(path + ".SpotMetal")
	e.F32(u.SpotMetal)
	e.Field(path + ".StructureFacing")
	e.U8(uint8(u.StructureFacing))
	e.Field(path + ".Stunned")
	e.Bool(u.Stunned)
	e.Field(path + ".X")
	e.I64(int64(u.X))
	e.Field(path + ".Y")
	e.I64(int64(u.Y))
	e.Field(path + ".YardOpen")
	e.Bool(u.YardOpen)
	e.Field(path + ".Z")
	e.I64(int64(u.Z))
	e.Field(path + ".deathExtraHookFired")
	e.Bool(u.deathExtraHookFired)
	e.Field(path + ".deathHookFired")
	e.Bool(u.deathHookFired)
	e.Field(path + ".statusCue")
	e.Bool(u.statusCue != nil)
	e.Field(path + ".yardTransaction")
	e.Bool(u.yardTransaction != nil)
}

// Attachment fields: AttachPiece, Cargo (stored order), Carrier. Every cargo
// and carrier value remains a weak raw handle, including stale references.
func writeCheckpointAttachment(e *checkpoint.Encoder, a *AttachmentState) {
	e.I64(int64(a.AttachPiece))
	e.Count(len(a.Cargo))
	for _, h := range a.Cargo {
		e.U32(uint32(h))
	}
	e.U32(uint32(a.Carrier))
}

// Move fields: Bank, Heading, Mode, ModeMirror, Pitch, Speed, VelX, VelY, VelZ.
// PendingHeading/PendingSpeed are excluded parity/presentation bookkeeping.
// Speed and velocities retain numeric.Fixed's int64 storage width.
func writeCheckpointMove(e *checkpoint.Encoder, m *MoveState) {
	e.U16(m.Bank)
	e.U16(m.Heading)
	e.U8(m.Mode)
	e.U8(m.ModeMirror)
	e.U16(m.Pitch)
	e.I64(int64(m.Speed))
	e.I64(int64(m.VelX))
	e.I64(int64(m.VelY))
	e.I64(int64(m.VelZ))
}

// Slot fields: Aim, AimOriginPiece, Ammo, DesiredPitch, DesiredYaw,
// DistanceWord, Flags, MuzzlePiece, Reload, Target, Weapon. Target fields:
// Kind, Unit, X, Z. SavedTargetLow/High are excluded load staging.
func writeCheckpointSlot(e *checkpoint.Encoder, c *CheckpointContext, s *Slot, path string) {
	e.Field(path + ".Aim")
	if err := s.Aim.WriteCheckpoint(e); err != nil {
		return
	}
	e.Field(path + ".AimOriginPiece")
	e.I32(s.AimOriginPiece)
	e.Field(path + ".Ammo")
	e.I32(s.Ammo)
	e.Field(path + ".DesiredPitch")
	e.U16(s.DesiredPitch)
	e.Field(path + ".DesiredYaw")
	e.U16(s.DesiredYaw)
	e.Field(path + ".DistanceWord")
	e.I32(s.DistanceWord)
	e.Field(path + ".Flags")
	e.U8(s.Flags)
	e.Field(path + ".MuzzlePiece")
	e.I32(s.MuzzlePiece)
	e.Field(path + ".Reload")
	e.I32(s.Reload)
	e.Field(path + ".Target")
	e.U8(uint8(s.Target.Kind))
	e.U32(uint32(s.Target.Unit))
	e.I64(int64(s.Target.X))
	e.I64(int64(s.Target.Z))
	e.Field(path + ".Weapon")
	e.Bool(s.Weapon != nil)
	if s.Weapon != nil {
		ref, err := c.Keys.Weapon(s.Weapon)
		if err != nil {
			e.Fail(err)
			return
		}
		e.Definition(ref)
	}
}
