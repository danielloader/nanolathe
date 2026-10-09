package ai

import (
	"github.com/nanolathe-gg/nanolathe/internal/community"
	"github.com/nanolathe-gg/nanolathe/internal/construction"
	"github.com/nanolathe-gg/nanolathe/internal/content"
	"github.com/nanolathe-gg/nanolathe/internal/orders"
	"github.com/nanolathe-gg/nanolathe/internal/pool"
	"github.com/nanolathe-gg/nanolathe/internal/sim/checkpoint"
	"github.com/nanolathe-gg/nanolathe/internal/sim/numeric"
	"github.com/nanolathe-gg/nanolathe/internal/sim/rng"
	"github.com/nanolathe-gg/nanolathe/internal/units"
	"github.com/nanolathe-gg/nanolathe/internal/world"
)

// ManagerConfig supplies copied installation inputs; see Manager for each field's contract.
type ManagerConfig struct {
	WeaponMaintenance   func(player uint8)
	CanPursueAir        func(member, target *units.Unit) bool
	Player              uint8
	Passive             bool
	Strategic           Strategic
	Deadlines           [TaskKindCount]uint32
	Profile             *Profile
	OriginX             numeric.Fixed
	OriginZ             numeric.Fixed
	SurfaceMetal        int32
	Catalog             *content.Catalog
	Factory             *units.Unit
	Terrain             *world.Terrain
	MissionGateFlag     int32
	QueueBuildTyped     func(BuildRequest) error
	OrderBinding        *orders.QueueBinding
	IsAlliance          func(a, b uint8) bool
	GroupResource       []pool.Handle
	GroupWaveA          []pool.Handle
	GroupRegroupA       []pool.Handle
	GroupConstruction   []pool.Handle
	GroupNull           []pool.Handle
	GroupWaveB          []pool.Handle
	GroupRegroupB       []pool.Handle
	GroupExplore        []pool.Handle
	GroupRally          []pool.Handle
	RallyVisible        func(viewer uint8, target *units.Unit) bool
	RallyProbeKnown     func(owner uint8, x, y, z numeric.Fixed) bool
	RallyShotTimeAdmits func(unit *units.Unit, x, y, z numeric.Fixed) bool
	RNG                 *rng.Simulation
	Planner             Planner
	Controller          Controller
	ConstructionRules   construction.Rules
	Ext                 any
	Shared              *BattleShared
	StartPositions      [][2]int32
	StartOwners         []int8
	BattleSeed          uint32
	ResumeGenerator     *uint64
	ControllerParams    string
	UnitVisible         func(viewer uint8, target *units.Unit) bool
	JammerSuppresses    func(viewer, jammerOwner uint8) bool
	Community           community.Features
	Survival            *SurvivalInfo
}

// NewManager copies configuration without invoking any callback.
func NewManager(c ManagerConfig) *Manager {
	return &Manager{
		weaponMaintenance:   c.WeaponMaintenance,
		canPursueAir:        c.CanPursueAir,
		Player:              c.Player,
		Passive:             c.Passive,
		Strategic:           c.Strategic,
		Deadlines:           c.Deadlines,
		Profile:             c.Profile,
		OriginX:             c.OriginX,
		OriginZ:             c.OriginZ,
		SurfaceMetal:        c.SurfaceMetal,
		Catalog:             c.Catalog,
		Factory:             c.Factory,
		Terrain:             c.Terrain,
		MissionGateFlag:     c.MissionGateFlag,
		queueBuildTyped:     c.QueueBuildTyped,
		OrderBinding:        c.OrderBinding,
		isAlliance:          c.IsAlliance,
		GroupResource:       c.GroupResource,
		GroupWaveA:          c.GroupWaveA,
		GroupRegroupA:       c.GroupRegroupA,
		GroupConstruction:   c.GroupConstruction,
		GroupNull:           c.GroupNull,
		GroupWaveB:          c.GroupWaveB,
		GroupRegroupB:       c.GroupRegroupB,
		GroupExplore:        c.GroupExplore,
		GroupRally:          c.GroupRally,
		rallyVisible:        c.RallyVisible,
		rallyProbeKnown:     c.RallyProbeKnown,
		rallyShotTimeAdmits: c.RallyShotTimeAdmits,
		RNG:                 c.RNG,
		Planner:             c.Planner,
		Controller:          c.Controller,
		ConstructionRules:   c.ConstructionRules,
		Ext:                 c.Ext,
		Shared:              c.Shared,
		StartPositions:      c.StartPositions,
		StartOwners:         c.StartOwners,
		BattleSeed:          c.BattleSeed,
		ResumeGenerator:     c.ResumeGenerator,
		ControllerParams:    c.ControllerParams,
		unitVisible:         c.UnitVisible,
		jammerSuppresses:    c.JammerSuppresses,
		Community:           c.Community,
		Survival:            c.Survival,
	}
}

// WeaponMaintenanceHook returns the installed callback without checkpoint proof.
func (b *Manager) WeaponMaintenanceHook() func(player uint8) { return b.weaponMaintenance }

// SetWeaponMaintenance replaces this callback with an ordinary installation.
func (b *Manager) SetWeaponMaintenance(fn func(player uint8)) {
	b.checkpointCallbacks[checkpointWeaponMaintenance] = checkpointManagerCallbackProof{}
	b.weaponMaintenance = fn
}

// CanPursueAirHook returns the installed callback without checkpoint proof.
func (b *Manager) CanPursueAirHook() func(member, target *units.Unit) bool { return b.canPursueAir }

// SetCanPursueAir replaces this callback with an ordinary installation.
func (b *Manager) SetCanPursueAir(fn func(member, target *units.Unit) bool) {
	b.checkpointCallbacks[checkpointCanPursueAir] = checkpointManagerCallbackProof{}
	b.canPursueAir = fn
}

// QueueBuildTypedHook returns the installed callback without checkpoint proof.
func (b *Manager) QueueBuildTypedHook() func(BuildRequest) error { return b.queueBuildTyped }

// SetQueueBuildTyped replaces this callback with an ordinary installation.
func (b *Manager) SetQueueBuildTyped(fn func(BuildRequest) error) {
	b.checkpointCallbacks[checkpointQueueBuildTyped] = checkpointManagerCallbackProof{}
	b.queueBuildTyped = fn
}

// IsAllianceHook returns the installed callback without checkpoint proof.
func (b *Manager) IsAllianceHook() func(a, b uint8) bool { return b.isAlliance }

// SetIsAlliance replaces this callback with an ordinary installation.
func (b *Manager) SetIsAlliance(fn func(a, b uint8) bool) {
	b.checkpointCallbacks[checkpointIsAlliance] = checkpointManagerCallbackProof{}
	b.isAlliance = fn
}

// RallyVisibleHook returns the installed callback without checkpoint proof.
func (b *Manager) RallyVisibleHook() func(viewer uint8, target *units.Unit) bool {
	return b.rallyVisible
}

// SetRallyVisible replaces this callback with an ordinary installation.
func (b *Manager) SetRallyVisible(fn func(viewer uint8, target *units.Unit) bool) {
	b.checkpointCallbacks[checkpointRallyVisible] = checkpointManagerCallbackProof{}
	b.rallyVisible = fn
}

// RallyProbeKnownHook returns the installed callback without checkpoint proof.
func (b *Manager) RallyProbeKnownHook() func(owner uint8, x, y, z numeric.Fixed) bool {
	return b.rallyProbeKnown
}

// SetRallyProbeKnown replaces this callback with an ordinary installation.
func (b *Manager) SetRallyProbeKnown(fn func(owner uint8, x, y, z numeric.Fixed) bool) {
	b.checkpointCallbacks[checkpointRallyProbeKnown] = checkpointManagerCallbackProof{}
	b.rallyProbeKnown = fn
}

// RallyShotTimeAdmitsHook returns the installed callback without checkpoint proof.
func (b *Manager) RallyShotTimeAdmitsHook() func(unit *units.Unit, x, y, z numeric.Fixed) bool {
	return b.rallyShotTimeAdmits
}

// SetRallyShotTimeAdmits replaces this callback with an ordinary installation.
func (b *Manager) SetRallyShotTimeAdmits(fn func(unit *units.Unit, x, y, z numeric.Fixed) bool) {
	b.checkpointCallbacks[checkpointRallyShotTimeAdmits] = checkpointManagerCallbackProof{}
	b.rallyShotTimeAdmits = fn
}

// UnitVisibleHook returns the installed callback without checkpoint proof.
func (b *Manager) UnitVisibleHook() func(viewer uint8, target *units.Unit) bool { return b.unitVisible }

// SetUnitVisible replaces this callback with an ordinary installation.
func (b *Manager) SetUnitVisible(fn func(viewer uint8, target *units.Unit) bool) {
	b.checkpointCallbacks[checkpointUnitVisible] = checkpointManagerCallbackProof{}
	b.unitVisible = fn
}

// JammerSuppressesHook returns the installed callback without checkpoint proof.
func (b *Manager) JammerSuppressesHook() func(viewer, jammerOwner uint8) bool {
	return b.jammerSuppresses
}

// SetJammerSuppresses replaces this callback with an ordinary installation.
func (b *Manager) SetJammerSuppresses(fn func(viewer, jammerOwner uint8) bool) {
	b.checkpointCallbacks[checkpointJammerSuppresses] = checkpointManagerCallbackProof{}
	b.jammerSuppresses = fn
}

// SetWeaponMaintenanceWithCheckpointBinding retains only this installation
// owner and authority; no callback is invoked (DESIGN_MULTIPLAYER §16.3.71).
func (m *Manager) SetWeaponMaintenanceWithCheckpointBinding(fn func(player uint8), authority *checkpoint.BindingAuthority) {
	m.SetWeaponMaintenance(fn)
	m.stampCheckpointCallback(checkpointWeaponMaintenance, fn != nil, authority)
}

// SetCanPursueAirWithCheckpointBinding retains only this installation
// owner and authority; no callback is invoked (DESIGN_MULTIPLAYER §16.3.71).
func (m *Manager) SetCanPursueAirWithCheckpointBinding(fn func(member, target *units.Unit) bool, authority *checkpoint.BindingAuthority) {
	m.SetCanPursueAir(fn)
	m.stampCheckpointCallback(checkpointCanPursueAir, fn != nil, authority)
}

// SetQueueBuildTypedWithCheckpointBinding retains only this installation
// owner and authority; no callback is invoked (DESIGN_MULTIPLAYER §16.3.71).
func (m *Manager) SetQueueBuildTypedWithCheckpointBinding(fn func(BuildRequest) error, authority *checkpoint.BindingAuthority) {
	m.SetQueueBuildTyped(fn)
	m.stampCheckpointCallback(checkpointQueueBuildTyped, fn != nil, authority)
}

// SetIsAllianceWithCheckpointBinding retains only this installation
// owner and authority; no callback is invoked (DESIGN_MULTIPLAYER §16.3.71).
func (m *Manager) SetIsAllianceWithCheckpointBinding(fn func(a, b uint8) bool, authority *checkpoint.BindingAuthority) {
	m.SetIsAlliance(fn)
	m.stampCheckpointCallback(checkpointIsAlliance, fn != nil, authority)
}

// SetRallyVisibleWithCheckpointBinding retains only this installation
// owner and authority; no callback is invoked (DESIGN_MULTIPLAYER §16.3.71).
func (m *Manager) SetRallyVisibleWithCheckpointBinding(fn func(viewer uint8, target *units.Unit) bool, authority *checkpoint.BindingAuthority) {
	m.SetRallyVisible(fn)
	m.stampCheckpointCallback(checkpointRallyVisible, fn != nil, authority)
}

// SetRallyProbeKnownWithCheckpointBinding retains only this installation
// owner and authority; no callback is invoked (DESIGN_MULTIPLAYER §16.3.71).
func (m *Manager) SetRallyProbeKnownWithCheckpointBinding(fn func(owner uint8, x, y, z numeric.Fixed) bool, authority *checkpoint.BindingAuthority) {
	m.SetRallyProbeKnown(fn)
	m.stampCheckpointCallback(checkpointRallyProbeKnown, fn != nil, authority)
}

// SetRallyShotTimeAdmitsWithCheckpointBinding retains only this installation
// owner and authority; no callback is invoked (DESIGN_MULTIPLAYER §16.3.71).
func (m *Manager) SetRallyShotTimeAdmitsWithCheckpointBinding(fn func(unit *units.Unit, x, y, z numeric.Fixed) bool, authority *checkpoint.BindingAuthority) {
	m.SetRallyShotTimeAdmits(fn)
	m.stampCheckpointCallback(checkpointRallyShotTimeAdmits, fn != nil, authority)
}

// SetUnitVisibleWithCheckpointBinding retains only this installation
// owner and authority; no callback is invoked (DESIGN_MULTIPLAYER §16.3.71).
func (m *Manager) SetUnitVisibleWithCheckpointBinding(fn func(viewer uint8, target *units.Unit) bool, authority *checkpoint.BindingAuthority) {
	m.SetUnitVisible(fn)
	m.stampCheckpointCallback(checkpointUnitVisible, fn != nil, authority)
}

// SetJammerSuppressesWithCheckpointBinding retains only this installation
// owner and authority; no callback is invoked (DESIGN_MULTIPLAYER §16.3.71).
func (m *Manager) SetJammerSuppressesWithCheckpointBinding(fn func(viewer, jammerOwner uint8) bool, authority *checkpoint.BindingAuthority) {
	m.SetJammerSuppresses(fn)
	m.stampCheckpointCallback(checkpointJammerSuppresses, fn != nil, authority)
}
