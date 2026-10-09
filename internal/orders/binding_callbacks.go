package orders

import (
	"github.com/nanolathe-gg/nanolathe/internal/combat"
	"github.com/nanolathe-gg/nanolathe/internal/community"
	"github.com/nanolathe-gg/nanolathe/internal/content"
	"github.com/nanolathe-gg/nanolathe/internal/economy"
	"github.com/nanolathe-gg/nanolathe/internal/pool"
	"github.com/nanolathe-gg/nanolathe/internal/sim/checkpoint"
	"github.com/nanolathe-gg/nanolathe/internal/sim/numeric"
	"github.com/nanolathe-gg/nanolathe/internal/sim/rng"
	"github.com/nanolathe-gg/nanolathe/internal/units"
)

// QueueBindingConfig supplies copied installation inputs; see QueueBinding for each field's contract.
type QueueBindingConfig struct {
	Community           community.Features
	BuilderOptions      func(owner uint8) BuilderOptions
	Rules               Rules
	DangerVisible       func(observer, target *units.Unit) bool
	DangerCanRespond    func(observer, target *units.Unit, slot int) bool
	DangerStepFeasible  func(u *units.Unit, x, z numeric.Fixed) bool
	DangerRouteFeasible func(u *units.Unit, x, z numeric.Fixed) bool
	ModernAIPlayer      func(owner uint8) bool
	Economy             interface {
		UnitBuckets(pool.Handle) *[2]economy.Bucket
	}
	Lookup             func(pool.Handle) *units.Unit
	Hostility          func(actor *units.Unit, target *units.Unit) bool
	SimRNG             *rng.Simulation
	CurrentTick        func() uint32
	Damage             func(uint32, combat.DamageInput) combat.DamageResult
	Movement           *MovementGoalAdapter
	World              *WorldQueryAdapter
	Work               *WorkAdapter
	Weapons            *WeaponAdapter
	Presentation       *PresentationAdapter
	Resources          func(uint8) (ResourceView, bool)
	ReclaimFeature     func(cx, cz int) (metal, energy float32, ok bool)
	BuildList          func(*content.UnitDef) bool
	TransportAdmission func(carrier, candidate *units.Unit) bool
}

// NewQueueBinding copies configuration without invoking any callback.
func NewQueueBinding(c QueueBindingConfig) *QueueBinding {
	return newQueueBinding(c, nil)
}

func newQueueBinding(c QueueBindingConfig, authority *checkpoint.BindingAuthority) *QueueBinding {
	b := &QueueBinding{}
	b.Community = c.Community
	b.SetBuilderOptionsWithCheckpointBinding(c.BuilderOptions, authority)
	b.Rules = c.Rules
	b.SetDangerVisibleWithCheckpointBinding(c.DangerVisible, authority)
	b.SetDangerCanRespondWithCheckpointBinding(c.DangerCanRespond, authority)
	b.SetDangerStepFeasibleWithCheckpointBinding(c.DangerStepFeasible, authority)
	b.SetDangerRouteFeasibleWithCheckpointBinding(c.DangerRouteFeasible, authority)
	b.SetModernAIPlayerWithCheckpointBinding(c.ModernAIPlayer, authority)
	b.Economy = c.Economy
	b.SetLookupWithCheckpointBinding(c.Lookup, authority)
	b.SetHostilityWithCheckpointBinding(c.Hostility, authority)
	b.SimRNG = c.SimRNG
	b.SetCurrentTickWithCheckpointBinding(c.CurrentTick, authority)
	b.SetDamageWithCheckpointBinding(c.Damage, authority)
	b.Movement = c.Movement
	b.World = c.World
	b.Work = c.Work
	b.Weapons = c.Weapons
	b.Presentation = c.Presentation
	b.SetResourcesWithCheckpointBinding(c.Resources, authority)
	b.SetReclaimFeatureWithCheckpointBinding(c.ReclaimFeature, authority)
	b.SetBuildListWithCheckpointBinding(c.BuildList, authority)
	b.SetTransportAdmissionWithCheckpointBinding(c.TransportAdmission, authority)
	return b
}

// BuilderOptionsHook returns the installed callback without checkpoint proof.
func (b *QueueBinding) BuilderOptionsHook() func(owner uint8) BuilderOptions { return b.builderOptions }

// SetBuilderOptions replaces this callback with an ordinary installation.
func (b *QueueBinding) SetBuilderOptions(fn func(owner uint8) BuilderOptions) {
	b.builderOptions = fn
	b.checkpointProofs.builderOptions = checkpointCallbackProof[QueueBinding]{}
}

// DangerVisibleHook returns the installed callback without checkpoint proof.
func (b *QueueBinding) DangerVisibleHook() func(observer, target *units.Unit) bool {
	return b.dangerVisible
}

// SetDangerVisible replaces this callback with an ordinary installation.
func (b *QueueBinding) SetDangerVisible(fn func(observer, target *units.Unit) bool) {
	b.dangerVisible = fn
	b.checkpointProofs.dangerVisible = checkpointCallbackProof[QueueBinding]{}
}

// DangerCanRespondHook returns the installed callback without checkpoint proof.
func (b *QueueBinding) DangerCanRespondHook() func(observer, target *units.Unit, slot int) bool {
	return b.dangerCanRespond
}

// SetDangerCanRespond replaces this callback with an ordinary installation.
func (b *QueueBinding) SetDangerCanRespond(fn func(observer, target *units.Unit, slot int) bool) {
	b.dangerCanRespond = fn
	b.checkpointProofs.dangerCanRespond = checkpointCallbackProof[QueueBinding]{}
}

// DangerStepFeasibleHook returns the installed callback without checkpoint proof.
func (b *QueueBinding) DangerStepFeasibleHook() func(u *units.Unit, x, z numeric.Fixed) bool {
	return b.dangerStepFeasible
}

// SetDangerStepFeasible replaces this callback with an ordinary installation.
func (b *QueueBinding) SetDangerStepFeasible(fn func(u *units.Unit, x, z numeric.Fixed) bool) {
	b.dangerStepFeasible = fn
	b.checkpointProofs.dangerStepFeasible = checkpointCallbackProof[QueueBinding]{}
}

// DangerRouteFeasibleHook returns the installed callback without checkpoint proof.
func (b *QueueBinding) DangerRouteFeasibleHook() func(u *units.Unit, x, z numeric.Fixed) bool {
	return b.dangerRouteFeasible
}

// SetDangerRouteFeasible replaces this callback with an ordinary installation.
func (b *QueueBinding) SetDangerRouteFeasible(fn func(u *units.Unit, x, z numeric.Fixed) bool) {
	b.dangerRouteFeasible = fn
	b.checkpointProofs.dangerRouteFeasible = checkpointCallbackProof[QueueBinding]{}
}

// ModernAIPlayerHook returns the installed callback without checkpoint proof.
func (b *QueueBinding) ModernAIPlayerHook() func(owner uint8) bool { return b.modernAIPlayer }

// SetModernAIPlayer replaces this callback with an ordinary installation.
func (b *QueueBinding) SetModernAIPlayer(fn func(owner uint8) bool) {
	b.modernAIPlayer = fn
	b.checkpointProofs.modernAIPlayer = checkpointCallbackProof[QueueBinding]{}
}

// LookupHook returns the installed callback without checkpoint proof.
func (b *QueueBinding) LookupHook() func(pool.Handle) *units.Unit { return b.lookup }

// SetLookup replaces this callback with an ordinary installation.
func (b *QueueBinding) SetLookup(fn func(pool.Handle) *units.Unit) {
	b.lookup = fn
	b.checkpointProofs.lookup = checkpointCallbackProof[QueueBinding]{}
}

// HostilityHook returns the installed callback without checkpoint proof.
func (b *QueueBinding) HostilityHook() func(actor *units.Unit, target *units.Unit) bool {
	return b.hostility
}

// SetHostility replaces this callback with an ordinary installation.
func (b *QueueBinding) SetHostility(fn func(actor *units.Unit, target *units.Unit) bool) {
	b.hostility = fn
	b.checkpointProofs.hostility = checkpointCallbackProof[QueueBinding]{}
}

// CurrentTickHook returns the installed callback without checkpoint proof.
func (b *QueueBinding) CurrentTickHook() func() uint32 { return b.currentTick }

// SetCurrentTick replaces this callback with an ordinary installation.
func (b *QueueBinding) SetCurrentTick(fn func() uint32) {
	b.currentTick = fn
	b.checkpointProofs.currentTick = checkpointCallbackProof[QueueBinding]{}
}

// DamageHook returns the installed callback without checkpoint proof.
func (b *QueueBinding) DamageHook() func(uint32, combat.DamageInput) combat.DamageResult {
	return b.damage
}

// SetDamage replaces this callback with an ordinary installation.
func (b *QueueBinding) SetDamage(fn func(uint32, combat.DamageInput) combat.DamageResult) {
	b.damage = fn
	b.checkpointProofs.damage = checkpointCallbackProof[QueueBinding]{}
}

// ResourcesHook returns the installed callback without checkpoint proof.
func (b *QueueBinding) ResourcesHook() func(uint8) (ResourceView, bool) { return b.resources }

// SetResources replaces this callback with an ordinary installation.
func (b *QueueBinding) SetResources(fn func(uint8) (ResourceView, bool)) {
	b.resources = fn
	b.checkpointProofs.resources = checkpointCallbackProof[QueueBinding]{}
}

// ReclaimFeatureHook returns the installed callback without checkpoint proof.
func (b *QueueBinding) ReclaimFeatureHook() func(cx, cz int) (metal, energy float32, ok bool) {
	return b.reclaimFeature
}

// SetReclaimFeature replaces this callback with an ordinary installation.
func (b *QueueBinding) SetReclaimFeature(fn func(cx, cz int) (metal, energy float32, ok bool)) {
	b.reclaimFeature = fn
	b.checkpointProofs.reclaimFeature = checkpointCallbackProof[QueueBinding]{}
}

// BuildListHook returns the installed callback without checkpoint proof.
func (b *QueueBinding) BuildListHook() func(*content.UnitDef) bool { return b.buildList }

// SetBuildList replaces this callback with an ordinary installation.
func (b *QueueBinding) SetBuildList(fn func(*content.UnitDef) bool) {
	b.buildList = fn
	b.checkpointProofs.buildList = checkpointCallbackProof[QueueBinding]{}
}

// TransportAdmissionHook returns the installed callback without checkpoint proof.
func (b *QueueBinding) TransportAdmissionHook() func(carrier, candidate *units.Unit) bool {
	return b.transportAdmission
}

// SetTransportAdmission replaces this callback with an ordinary installation.
func (b *QueueBinding) SetTransportAdmission(fn func(carrier, candidate *units.Unit) bool) {
	b.transportAdmission = fn
	b.checkpointProofs.transportAdmission = checkpointCallbackProof[QueueBinding]{}
}

// MovementGoalAdapterConfig supplies copied installation inputs; see MovementGoalAdapter for each field's contract.
type MovementGoalAdapterConfig struct {
	DetachTakeoff      func(*units.Unit) bool
	CrowdedMoveBlocked func(*units.Unit, *Node) (anchorX, anchorZ int32, blocked bool)
	Ready              func() bool
	InstallPoint       func(PointGoalRequest) bool
	InstallAnnulus     func(AnnulusGoalRequest) bool
	InstallRectangle   func(RectangleGoalRequest) bool
	InstallAir         func(AirGoalRequest) bool
	Release            func(*Node) bool
	Destroy            func(*Node) bool
	RunAir             AirLegRunner
	AirBases           func(allyGroup uint8) []pool.Handle
	PlaceUnit          func(PlaceRequest) bool
}

// NewMovementGoalAdapter copies configuration without invoking any callback.
func NewMovementGoalAdapter(c MovementGoalAdapterConfig) *MovementGoalAdapter {
	return newMovementGoalAdapter(c, nil)
}

func newMovementGoalAdapter(c MovementGoalAdapterConfig, authority *checkpoint.BindingAuthority) *MovementGoalAdapter {
	b := &MovementGoalAdapter{}
	b.SetDetachTakeoffWithCheckpointBinding(c.DetachTakeoff, authority)
	b.SetCrowdedMoveBlockedWithCheckpointBinding(c.CrowdedMoveBlocked, authority)
	b.SetReadyWithCheckpointBinding(c.Ready, authority)
	b.SetInstallPointWithCheckpointBinding(c.InstallPoint, authority)
	b.SetInstallAnnulusWithCheckpointBinding(c.InstallAnnulus, authority)
	b.SetInstallRectangleWithCheckpointBinding(c.InstallRectangle, authority)
	b.SetInstallAirWithCheckpointBinding(c.InstallAir, authority)
	b.SetReleaseWithCheckpointBinding(c.Release, authority)
	b.SetDestroyWithCheckpointBinding(c.Destroy, authority)
	b.SetRunAirWithCheckpointBinding(c.RunAir, authority)
	b.SetAirBasesWithCheckpointBinding(c.AirBases, authority)
	b.SetPlaceUnitWithCheckpointBinding(c.PlaceUnit, authority)
	return b
}

// DetachTakeoffHook returns the installed callback without checkpoint proof.
func (b *MovementGoalAdapter) DetachTakeoffHook() func(*units.Unit) bool { return b.detachTakeoff }

// SetDetachTakeoff replaces this callback with an ordinary installation.
func (b *MovementGoalAdapter) SetDetachTakeoff(fn func(*units.Unit) bool) {
	b.detachTakeoff = fn
	b.checkpointProofs.detachTakeoff = checkpointCallbackProof[MovementGoalAdapter]{}
}

// CrowdedMoveBlockedHook returns the installed callback without checkpoint proof.
func (b *MovementGoalAdapter) CrowdedMoveBlockedHook() func(*units.Unit, *Node) (anchorX, anchorZ int32, blocked bool) {
	return b.crowdedMoveBlocked
}

// SetCrowdedMoveBlocked replaces this callback with an ordinary installation.
func (b *MovementGoalAdapter) SetCrowdedMoveBlocked(fn func(*units.Unit, *Node) (anchorX, anchorZ int32, blocked bool)) {
	b.crowdedMoveBlocked = fn
	b.checkpointProofs.crowdedMoveBlocked = checkpointCallbackProof[MovementGoalAdapter]{}
}

// ReadyHook returns the installed callback without checkpoint proof.
func (b *MovementGoalAdapter) ReadyHook() func() bool { return b.ready }

// SetReady replaces this callback with an ordinary installation.
func (b *MovementGoalAdapter) SetReady(fn func() bool) {
	b.ready = fn
	b.checkpointProofs.ready = checkpointCallbackProof[MovementGoalAdapter]{}
}

// InstallPointHook returns the installed callback without checkpoint proof.
func (b *MovementGoalAdapter) InstallPointHook() func(PointGoalRequest) bool { return b.installPoint }

// SetInstallPoint replaces this callback with an ordinary installation.
func (b *MovementGoalAdapter) SetInstallPoint(fn func(PointGoalRequest) bool) {
	b.installPoint = fn
	b.checkpointProofs.installPoint = checkpointCallbackProof[MovementGoalAdapter]{}
}

// InstallAnnulusHook returns the installed callback without checkpoint proof.
func (b *MovementGoalAdapter) InstallAnnulusHook() func(AnnulusGoalRequest) bool {
	return b.installAnnulus
}

// SetInstallAnnulus replaces this callback with an ordinary installation.
func (b *MovementGoalAdapter) SetInstallAnnulus(fn func(AnnulusGoalRequest) bool) {
	b.installAnnulus = fn
	b.checkpointProofs.installAnnulus = checkpointCallbackProof[MovementGoalAdapter]{}
}

// InstallRectangleHook returns the installed callback without checkpoint proof.
func (b *MovementGoalAdapter) InstallRectangleHook() func(RectangleGoalRequest) bool {
	return b.installRectangle
}

// SetInstallRectangle replaces this callback with an ordinary installation.
func (b *MovementGoalAdapter) SetInstallRectangle(fn func(RectangleGoalRequest) bool) {
	b.installRectangle = fn
	b.checkpointProofs.installRectangle = checkpointCallbackProof[MovementGoalAdapter]{}
}

// InstallAirHook returns the installed callback without checkpoint proof.
func (b *MovementGoalAdapter) InstallAirHook() func(AirGoalRequest) bool { return b.installAir }

// SetInstallAir replaces this callback with an ordinary installation.
func (b *MovementGoalAdapter) SetInstallAir(fn func(AirGoalRequest) bool) {
	b.installAir = fn
	b.checkpointProofs.installAir = checkpointCallbackProof[MovementGoalAdapter]{}
}

// ReleaseHook returns the installed callback without checkpoint proof.
func (b *MovementGoalAdapter) ReleaseHook() func(*Node) bool { return b.release }

// SetRelease replaces this callback with an ordinary installation.
func (b *MovementGoalAdapter) SetRelease(fn func(*Node) bool) {
	b.release = fn
	b.checkpointProofs.release = checkpointCallbackProof[MovementGoalAdapter]{}
}

// DestroyHook returns the installed callback without checkpoint proof.
func (b *MovementGoalAdapter) DestroyHook() func(*Node) bool { return b.destroy }

// SetDestroy replaces this callback with an ordinary installation.
func (b *MovementGoalAdapter) SetDestroy(fn func(*Node) bool) {
	b.destroy = fn
	b.checkpointProofs.destroy = checkpointCallbackProof[MovementGoalAdapter]{}
}

// RunAirHook returns the installed callback without checkpoint proof.
func (b *MovementGoalAdapter) RunAirHook() AirLegRunner { return b.runAir }

// SetRunAir replaces this callback with an ordinary installation.
func (b *MovementGoalAdapter) SetRunAir(fn AirLegRunner) {
	b.runAir = fn
	b.checkpointProofs.runAir = checkpointCallbackProof[MovementGoalAdapter]{}
}

// AirBasesHook returns the installed callback without checkpoint proof.
func (b *MovementGoalAdapter) AirBasesHook() func(allyGroup uint8) []pool.Handle { return b.airBases }

// SetAirBases replaces this callback with an ordinary installation.
func (b *MovementGoalAdapter) SetAirBases(fn func(allyGroup uint8) []pool.Handle) {
	b.airBases = fn
	b.checkpointProofs.airBases = checkpointCallbackProof[MovementGoalAdapter]{}
}

// PlaceUnitHook returns the installed callback without checkpoint proof.
func (b *MovementGoalAdapter) PlaceUnitHook() func(PlaceRequest) bool { return b.placeUnit }

// SetPlaceUnit replaces this callback with an ordinary installation.
func (b *MovementGoalAdapter) SetPlaceUnit(fn func(PlaceRequest) bool) {
	b.placeUnit = fn
	b.checkpointProofs.placeUnit = checkpointCallbackProof[MovementGoalAdapter]{}
}

// WorldQueryAdapterConfig supplies copied installation inputs; see WorldQueryAdapter for each field's contract.
type WorldQueryAdapterConfig struct {
	LookupUnit          func(pool.Handle) *units.Unit
	Hostile             func(*units.Unit, *units.Unit) bool
	ForEachUnit         func(func(pool.Handle, *units.Unit) bool)
	ForEachUnitInRadius func(numeric.Fixed, numeric.Fixed, numeric.Fixed, func(pool.Handle, *units.Unit) bool)
	LookupFeature       func(int32, int32) (FeatureView, bool)
	ForEachFeature      func(func(FeatureView) bool)
	TerrainHeight       func(numeric.Fixed, numeric.Fixed) (numeric.Fixed, bool)
	SeaLevel            func() uint8
	DeclaresAlliance    func(from, toward uint8) bool
	MappingWord         func(tileX, tileZ int32) (uint16, bool)
}

// NewWorldQueryAdapter copies configuration without invoking any callback.
func NewWorldQueryAdapter(c WorldQueryAdapterConfig) *WorldQueryAdapter {
	return newWorldQueryAdapter(c, nil)
}

func newWorldQueryAdapter(c WorldQueryAdapterConfig, authority *checkpoint.BindingAuthority) *WorldQueryAdapter {
	b := &WorldQueryAdapter{}
	b.SetLookupUnitWithCheckpointBinding(c.LookupUnit, authority)
	b.SetHostileWithCheckpointBinding(c.Hostile, authority)
	b.SetForEachUnitWithCheckpointBinding(c.ForEachUnit, authority)
	b.SetForEachUnitInRadiusWithCheckpointBinding(c.ForEachUnitInRadius, authority)
	b.SetLookupFeatureWithCheckpointBinding(c.LookupFeature, authority)
	b.SetForEachFeatureWithCheckpointBinding(c.ForEachFeature, authority)
	b.SetTerrainHeightWithCheckpointBinding(c.TerrainHeight, authority)
	b.SetSeaLevelWithCheckpointBinding(c.SeaLevel, authority)
	b.SetDeclaresAllianceWithCheckpointBinding(c.DeclaresAlliance, authority)
	b.SetMappingWordWithCheckpointBinding(c.MappingWord, authority)
	return b
}

// LookupUnitHook returns the installed callback without checkpoint proof.
func (b *WorldQueryAdapter) LookupUnitHook() func(pool.Handle) *units.Unit { return b.lookupUnit }

// SetLookupUnit replaces this callback with an ordinary installation.
func (b *WorldQueryAdapter) SetLookupUnit(fn func(pool.Handle) *units.Unit) {
	b.lookupUnit = fn
	b.checkpointProofs.lookupUnit = checkpointCallbackProof[WorldQueryAdapter]{}
}

// HostileHook returns the installed callback without checkpoint proof.
func (b *WorldQueryAdapter) HostileHook() func(*units.Unit, *units.Unit) bool { return b.hostile }

// SetHostile replaces this callback with an ordinary installation.
func (b *WorldQueryAdapter) SetHostile(fn func(*units.Unit, *units.Unit) bool) {
	b.hostile = fn
	b.checkpointProofs.hostile = checkpointCallbackProof[WorldQueryAdapter]{}
}

// ForEachUnitHook returns the installed callback without checkpoint proof.
func (b *WorldQueryAdapter) ForEachUnitHook() func(func(pool.Handle, *units.Unit) bool) {
	return b.forEachUnit
}

// SetForEachUnit replaces this callback with an ordinary installation.
func (b *WorldQueryAdapter) SetForEachUnit(fn func(func(pool.Handle, *units.Unit) bool)) {
	b.forEachUnit = fn
	b.checkpointProofs.forEachUnit = checkpointCallbackProof[WorldQueryAdapter]{}
}

// ForEachUnitInRadiusHook returns the installed callback without checkpoint proof.
func (b *WorldQueryAdapter) ForEachUnitInRadiusHook() func(numeric.Fixed, numeric.Fixed, numeric.Fixed, func(pool.Handle, *units.Unit) bool) {
	return b.forEachUnitInRadius
}

// SetForEachUnitInRadius replaces this callback with an ordinary installation.
func (b *WorldQueryAdapter) SetForEachUnitInRadius(fn func(numeric.Fixed, numeric.Fixed, numeric.Fixed, func(pool.Handle, *units.Unit) bool)) {
	b.forEachUnitInRadius = fn
	b.checkpointProofs.forEachUnitInRadius = checkpointCallbackProof[WorldQueryAdapter]{}
}

// LookupFeatureHook returns the installed callback without checkpoint proof.
func (b *WorldQueryAdapter) LookupFeatureHook() func(int32, int32) (FeatureView, bool) {
	return b.lookupFeature
}

// SetLookupFeature replaces this callback with an ordinary installation.
func (b *WorldQueryAdapter) SetLookupFeature(fn func(int32, int32) (FeatureView, bool)) {
	b.lookupFeature = fn
	b.checkpointProofs.lookupFeature = checkpointCallbackProof[WorldQueryAdapter]{}
}

// ForEachFeatureHook returns the installed callback without checkpoint proof.
func (b *WorldQueryAdapter) ForEachFeatureHook() func(func(FeatureView) bool) {
	return b.forEachFeature
}

// SetForEachFeature replaces this callback with an ordinary installation.
func (b *WorldQueryAdapter) SetForEachFeature(fn func(func(FeatureView) bool)) {
	b.forEachFeature = fn
	b.checkpointProofs.forEachFeature = checkpointCallbackProof[WorldQueryAdapter]{}
}

// TerrainHeightHook returns the installed callback without checkpoint proof.
func (b *WorldQueryAdapter) TerrainHeightHook() func(numeric.Fixed, numeric.Fixed) (numeric.Fixed, bool) {
	return b.terrainHeight
}

// SetTerrainHeight replaces this callback with an ordinary installation.
func (b *WorldQueryAdapter) SetTerrainHeight(fn func(numeric.Fixed, numeric.Fixed) (numeric.Fixed, bool)) {
	b.terrainHeight = fn
	b.checkpointProofs.terrainHeight = checkpointCallbackProof[WorldQueryAdapter]{}
}

// SeaLevelHook returns the installed callback without checkpoint proof.
func (b *WorldQueryAdapter) SeaLevelHook() func() uint8 { return b.seaLevel }

// SetSeaLevel replaces this callback with an ordinary installation.
func (b *WorldQueryAdapter) SetSeaLevel(fn func() uint8) {
	b.seaLevel = fn
	b.checkpointProofs.seaLevel = checkpointCallbackProof[WorldQueryAdapter]{}
}

// DeclaresAllianceHook returns the installed callback without checkpoint proof.
func (b *WorldQueryAdapter) DeclaresAllianceHook() func(from, toward uint8) bool {
	return b.declaresAlliance
}

// SetDeclaresAlliance replaces this callback with an ordinary installation.
func (b *WorldQueryAdapter) SetDeclaresAlliance(fn func(from, toward uint8) bool) {
	b.declaresAlliance = fn
	b.checkpointProofs.declaresAlliance = checkpointCallbackProof[WorldQueryAdapter]{}
}

// MappingWordHook returns the installed callback without checkpoint proof.
func (b *WorldQueryAdapter) MappingWordHook() func(tileX, tileZ int32) (uint16, bool) {
	return b.mappingWord
}

// SetMappingWord replaces this callback with an ordinary installation.
func (b *WorldQueryAdapter) SetMappingWord(fn func(tileX, tileZ int32) (uint16, bool)) {
	b.mappingWord = fn
	b.checkpointProofs.mappingWord = checkpointCallbackProof[WorldQueryAdapter]{}
}

// WorkAdapterConfig supplies copied installation inputs; see WorkAdapter for each field's contract.
type WorkAdapterConfig struct {
	Ready               func() bool
	Assist              func(*units.Unit, *Node, uint32) bool
	Repair              func(builder, patient *units.Unit, node *Node, tick uint32) bool
	Capture             func(*units.Unit, *Node, uint32) bool
	Resurrect           func(*units.Unit, *Node, uint32) bool
	CanResurrectFeature func(FeatureView) bool
	CancelNotice        func(owner *units.Unit, n *Node, tick uint32) bool
}

// NewWorkAdapter copies configuration without invoking any callback.
func NewWorkAdapter(c WorkAdapterConfig) *WorkAdapter {
	return newWorkAdapter(c, nil)
}

func newWorkAdapter(c WorkAdapterConfig, authority *checkpoint.BindingAuthority) *WorkAdapter {
	b := &WorkAdapter{}
	b.SetReadyWithCheckpointBinding(c.Ready, authority)
	b.SetAssistWithCheckpointBinding(c.Assist, authority)
	b.SetRepairWithCheckpointBinding(c.Repair, authority)
	b.SetCaptureWithCheckpointBinding(c.Capture, authority)
	b.SetResurrectWithCheckpointBinding(c.Resurrect, authority)
	b.SetCanResurrectFeatureWithCheckpointBinding(c.CanResurrectFeature, authority)
	b.SetCancelNoticeWithCheckpointBinding(c.CancelNotice, authority)
	return b
}

// ReadyHook returns the installed callback without checkpoint proof.
func (b *WorkAdapter) ReadyHook() func() bool { return b.ready }

// SetReady replaces this callback with an ordinary installation.
func (b *WorkAdapter) SetReady(fn func() bool) {
	b.ready = fn
	b.checkpointProofs.ready = checkpointCallbackProof[WorkAdapter]{}
}

// AssistHook returns the installed callback without checkpoint proof.
func (b *WorkAdapter) AssistHook() func(*units.Unit, *Node, uint32) bool { return b.assist }

// SetAssist replaces this callback with an ordinary installation.
func (b *WorkAdapter) SetAssist(fn func(*units.Unit, *Node, uint32) bool) {
	b.assist = fn
	b.checkpointProofs.assist = checkpointCallbackProof[WorkAdapter]{}
}

// RepairHook returns the installed callback without checkpoint proof.
func (b *WorkAdapter) RepairHook() func(builder, patient *units.Unit, node *Node, tick uint32) bool {
	return b.repair
}

// SetRepair replaces this callback with an ordinary installation.
func (b *WorkAdapter) SetRepair(fn func(builder, patient *units.Unit, node *Node, tick uint32) bool) {
	b.repair = fn
	b.checkpointProofs.repair = checkpointCallbackProof[WorkAdapter]{}
}

// CaptureHook returns the installed callback without checkpoint proof.
func (b *WorkAdapter) CaptureHook() func(*units.Unit, *Node, uint32) bool { return b.capture }

// SetCapture replaces this callback with an ordinary installation.
func (b *WorkAdapter) SetCapture(fn func(*units.Unit, *Node, uint32) bool) {
	b.capture = fn
	b.checkpointProofs.capture = checkpointCallbackProof[WorkAdapter]{}
}

// ResurrectHook returns the installed callback without checkpoint proof.
func (b *WorkAdapter) ResurrectHook() func(*units.Unit, *Node, uint32) bool { return b.resurrect }

// SetResurrect replaces this callback with an ordinary installation.
func (b *WorkAdapter) SetResurrect(fn func(*units.Unit, *Node, uint32) bool) {
	b.resurrect = fn
	b.checkpointProofs.resurrect = checkpointCallbackProof[WorkAdapter]{}
}

// CanResurrectFeatureHook returns the installed callback without checkpoint proof.
func (b *WorkAdapter) CanResurrectFeatureHook() func(FeatureView) bool { return b.canResurrectFeature }

// SetCanResurrectFeature replaces this callback with an ordinary installation.
func (b *WorkAdapter) SetCanResurrectFeature(fn func(FeatureView) bool) {
	b.canResurrectFeature = fn
	b.checkpointProofs.canResurrectFeature = checkpointCallbackProof[WorkAdapter]{}
}

// CancelNoticeHook returns the installed callback without checkpoint proof.
func (b *WorkAdapter) CancelNoticeHook() func(owner *units.Unit, n *Node, tick uint32) bool {
	return b.cancelNotice
}

// SetCancelNotice replaces this callback with an ordinary installation.
func (b *WorkAdapter) SetCancelNotice(fn func(owner *units.Unit, n *Node, tick uint32) bool) {
	b.cancelNotice = fn
	b.checkpointProofs.cancelNotice = checkpointCallbackProof[WorkAdapter]{}
}

// WeaponAdapterConfig supplies copied installation inputs; see WeaponAdapter for each field's contract.
type WeaponAdapterConfig struct {
	FiringPositionBlocked func(shooter, target *units.Unit, tick uint32) bool
	FiringPositionClear   func(shooter, target *units.Unit, tick uint32, x, y, z numeric.Fixed) bool
	Ready                 func() bool
	ReleaseSlot           func(*units.Unit, int) bool
	InhibitSlot           func(*units.Unit, int) bool
	SetManualTarget       func(*units.Unit, int, pool.Handle) bool
	FireTarget            func(*units.Unit, int, pool.Handle, uint32) bool
	FirePoint             func(*units.Unit, int, numeric.Fixed, numeric.Fixed, uint32) bool
	StopFiring            func(*units.Unit, int) bool
	Acquire               func(*units.Unit, int, uint32) (pool.Handle, bool)
	Engaged               func(*units.Unit, int) bool
	TargetsInRadius       func(*units.Unit, numeric.Fixed, numeric.Fixed, int32) []pool.Handle
	CanEngage             func(*units.Unit, pool.Handle, int) bool
}

// NewWeaponAdapter copies configuration without invoking any callback.
func NewWeaponAdapter(c WeaponAdapterConfig) *WeaponAdapter {
	return newWeaponAdapter(c, nil)
}

func newWeaponAdapter(c WeaponAdapterConfig, authority *checkpoint.BindingAuthority) *WeaponAdapter {
	b := &WeaponAdapter{}
	b.SetFiringPositionBlockedWithCheckpointBinding(c.FiringPositionBlocked, authority)
	b.SetFiringPositionClearWithCheckpointBinding(c.FiringPositionClear, authority)
	b.SetReadyWithCheckpointBinding(c.Ready, authority)
	b.SetReleaseSlotWithCheckpointBinding(c.ReleaseSlot, authority)
	b.SetInhibitSlotWithCheckpointBinding(c.InhibitSlot, authority)
	b.SetSetManualTargetWithCheckpointBinding(c.SetManualTarget, authority)
	b.SetFireTargetWithCheckpointBinding(c.FireTarget, authority)
	b.SetFirePointWithCheckpointBinding(c.FirePoint, authority)
	b.SetStopFiringWithCheckpointBinding(c.StopFiring, authority)
	b.SetAcquireWithCheckpointBinding(c.Acquire, authority)
	b.SetEngagedWithCheckpointBinding(c.Engaged, authority)
	b.SetTargetsInRadiusWithCheckpointBinding(c.TargetsInRadius, authority)
	b.SetCanEngageWithCheckpointBinding(c.CanEngage, authority)
	return b
}

// FiringPositionBlockedHook returns the installed callback without checkpoint proof.
func (b *WeaponAdapter) FiringPositionBlockedHook() func(shooter, target *units.Unit, tick uint32) bool {
	return b.firingPositionBlocked
}

// SetFiringPositionBlocked replaces this callback with an ordinary installation.
func (b *WeaponAdapter) SetFiringPositionBlocked(fn func(shooter, target *units.Unit, tick uint32) bool) {
	b.firingPositionBlocked = fn
	b.checkpointProofs.firingPositionBlocked = checkpointCallbackProof[WeaponAdapter]{}
}

// FiringPositionClearHook returns the installed callback without checkpoint proof.
func (b *WeaponAdapter) FiringPositionClearHook() func(shooter, target *units.Unit, tick uint32, x, y, z numeric.Fixed) bool {
	return b.firingPositionClear
}

// SetFiringPositionClear replaces this callback with an ordinary installation.
func (b *WeaponAdapter) SetFiringPositionClear(fn func(shooter, target *units.Unit, tick uint32, x, y, z numeric.Fixed) bool) {
	b.firingPositionClear = fn
	b.checkpointProofs.firingPositionClear = checkpointCallbackProof[WeaponAdapter]{}
}

// ReadyHook returns the installed callback without checkpoint proof.
func (b *WeaponAdapter) ReadyHook() func() bool { return b.ready }

// SetReady replaces this callback with an ordinary installation.
func (b *WeaponAdapter) SetReady(fn func() bool) {
	b.ready = fn
	b.checkpointProofs.ready = checkpointCallbackProof[WeaponAdapter]{}
}

// ReleaseSlotHook returns the installed callback without checkpoint proof.
func (b *WeaponAdapter) ReleaseSlotHook() func(*units.Unit, int) bool { return b.releaseSlot }

// SetReleaseSlot replaces this callback with an ordinary installation.
func (b *WeaponAdapter) SetReleaseSlot(fn func(*units.Unit, int) bool) {
	b.releaseSlot = fn
	b.checkpointProofs.releaseSlot = checkpointCallbackProof[WeaponAdapter]{}
}

// InhibitSlotHook returns the installed callback without checkpoint proof.
func (b *WeaponAdapter) InhibitSlotHook() func(*units.Unit, int) bool { return b.inhibitSlot }

// SetInhibitSlot replaces this callback with an ordinary installation.
func (b *WeaponAdapter) SetInhibitSlot(fn func(*units.Unit, int) bool) {
	b.inhibitSlot = fn
	b.checkpointProofs.inhibitSlot = checkpointCallbackProof[WeaponAdapter]{}
}

// SetManualTargetHook returns the installed callback without checkpoint proof.
func (b *WeaponAdapter) SetManualTargetHook() func(*units.Unit, int, pool.Handle) bool {
	return b.setManualTarget
}

// SetSetManualTarget replaces this callback with an ordinary installation.
func (b *WeaponAdapter) SetSetManualTarget(fn func(*units.Unit, int, pool.Handle) bool) {
	b.setManualTarget = fn
	b.checkpointProofs.setManualTarget = checkpointCallbackProof[WeaponAdapter]{}
}

// FireTargetHook returns the installed callback without checkpoint proof.
func (b *WeaponAdapter) FireTargetHook() func(*units.Unit, int, pool.Handle, uint32) bool {
	return b.fireTarget
}

// SetFireTarget replaces this callback with an ordinary installation.
func (b *WeaponAdapter) SetFireTarget(fn func(*units.Unit, int, pool.Handle, uint32) bool) {
	b.fireTarget = fn
	b.checkpointProofs.fireTarget = checkpointCallbackProof[WeaponAdapter]{}
}

// FirePointHook returns the installed callback without checkpoint proof.
func (b *WeaponAdapter) FirePointHook() func(*units.Unit, int, numeric.Fixed, numeric.Fixed, uint32) bool {
	return b.firePoint
}

// SetFirePoint replaces this callback with an ordinary installation.
func (b *WeaponAdapter) SetFirePoint(fn func(*units.Unit, int, numeric.Fixed, numeric.Fixed, uint32) bool) {
	b.firePoint = fn
	b.checkpointProofs.firePoint = checkpointCallbackProof[WeaponAdapter]{}
}

// StopFiringHook returns the installed callback without checkpoint proof.
func (b *WeaponAdapter) StopFiringHook() func(*units.Unit, int) bool { return b.stopFiring }

// SetStopFiring replaces this callback with an ordinary installation.
func (b *WeaponAdapter) SetStopFiring(fn func(*units.Unit, int) bool) {
	b.stopFiring = fn
	b.checkpointProofs.stopFiring = checkpointCallbackProof[WeaponAdapter]{}
}

// AcquireHook returns the installed callback without checkpoint proof.
func (b *WeaponAdapter) AcquireHook() func(*units.Unit, int, uint32) (pool.Handle, bool) {
	return b.acquire
}

// SetAcquire replaces this callback with an ordinary installation.
func (b *WeaponAdapter) SetAcquire(fn func(*units.Unit, int, uint32) (pool.Handle, bool)) {
	b.acquire = fn
	b.checkpointProofs.acquire = checkpointCallbackProof[WeaponAdapter]{}
}

// EngagedHook returns the installed callback without checkpoint proof.
func (b *WeaponAdapter) EngagedHook() func(*units.Unit, int) bool { return b.engaged }

// SetEngaged replaces this callback with an ordinary installation.
func (b *WeaponAdapter) SetEngaged(fn func(*units.Unit, int) bool) {
	b.engaged = fn
	b.checkpointProofs.engaged = checkpointCallbackProof[WeaponAdapter]{}
}

// TargetsInRadiusHook returns the installed callback without checkpoint proof.
func (b *WeaponAdapter) TargetsInRadiusHook() func(*units.Unit, numeric.Fixed, numeric.Fixed, int32) []pool.Handle {
	return b.targetsInRadius
}

// SetTargetsInRadius replaces this callback with an ordinary installation.
func (b *WeaponAdapter) SetTargetsInRadius(fn func(*units.Unit, numeric.Fixed, numeric.Fixed, int32) []pool.Handle) {
	b.targetsInRadius = fn
	b.checkpointProofs.targetsInRadius = checkpointCallbackProof[WeaponAdapter]{}
}

// CanEngageHook returns the installed callback without checkpoint proof.
func (b *WeaponAdapter) CanEngageHook() func(*units.Unit, pool.Handle, int) bool { return b.canEngage }

// SetCanEngage replaces this callback with an ordinary installation.
func (b *WeaponAdapter) SetCanEngage(fn func(*units.Unit, pool.Handle, int) bool) {
	b.canEngage = fn
	b.checkpointProofs.canEngage = checkpointCallbackProof[WeaponAdapter]{}
}

// PresentationAdapterConfig supplies copied installation inputs; see PresentationAdapter for each field's contract.
type PresentationAdapterConfig struct {
	Ready            func() bool
	Status           func(*units.Unit, uint8, string) bool
	Nanolathe        func(*units.Unit, *Node, uint32) bool
	NanolatheFeature func(*units.Unit, *Node, FeatureView, uint32) bool
	Teleport         func(moved *units.Unit, fromX, fromY, fromZ, toX, toY, toZ numeric.Fixed) bool
}

// NewPresentationAdapter copies configuration without invoking any callback.
func NewPresentationAdapter(c PresentationAdapterConfig) *PresentationAdapter {
	return newPresentationAdapter(c, nil)
}

func newPresentationAdapter(c PresentationAdapterConfig, authority *checkpoint.BindingAuthority) *PresentationAdapter {
	b := &PresentationAdapter{}
	b.SetReadyWithCheckpointBinding(c.Ready, authority)
	b.SetStatusWithCheckpointBinding(c.Status, authority)
	b.SetNanolatheWithCheckpointBinding(c.Nanolathe, authority)
	b.SetNanolatheFeatureWithCheckpointBinding(c.NanolatheFeature, authority)
	b.SetTeleportWithCheckpointBinding(c.Teleport, authority)
	return b
}

// ReadyHook returns the installed callback without checkpoint proof.
func (b *PresentationAdapter) ReadyHook() func() bool { return b.ready }

// SetReady replaces this callback with an ordinary installation.
func (b *PresentationAdapter) SetReady(fn func() bool) {
	b.ready = fn
	b.checkpointProofs.ready = checkpointCallbackProof[PresentationAdapter]{}
}

// StatusHook returns the installed callback without checkpoint proof.
func (b *PresentationAdapter) StatusHook() func(*units.Unit, uint8, string) bool { return b.status }

// SetStatus replaces this callback with an ordinary installation.
func (b *PresentationAdapter) SetStatus(fn func(*units.Unit, uint8, string) bool) {
	b.status = fn
	b.checkpointProofs.status = checkpointCallbackProof[PresentationAdapter]{}
}

// NanolatheHook returns the installed callback without checkpoint proof.
func (b *PresentationAdapter) NanolatheHook() func(*units.Unit, *Node, uint32) bool {
	return b.nanolathe
}

// SetNanolathe replaces this callback with an ordinary installation.
func (b *PresentationAdapter) SetNanolathe(fn func(*units.Unit, *Node, uint32) bool) {
	b.nanolathe = fn
	b.checkpointProofs.nanolathe = checkpointCallbackProof[PresentationAdapter]{}
}

// NanolatheFeatureHook returns the installed callback without checkpoint proof.
func (b *PresentationAdapter) NanolatheFeatureHook() func(*units.Unit, *Node, FeatureView, uint32) bool {
	return b.nanolatheFeature
}

// SetNanolatheFeature replaces this callback with an ordinary installation.
func (b *PresentationAdapter) SetNanolatheFeature(fn func(*units.Unit, *Node, FeatureView, uint32) bool) {
	b.nanolatheFeature = fn
	b.checkpointProofs.nanolatheFeature = checkpointCallbackProof[PresentationAdapter]{}
}

// TeleportHook returns the installed callback without checkpoint proof.
func (b *PresentationAdapter) TeleportHook() func(moved *units.Unit, fromX, fromY, fromZ, toX, toY, toZ numeric.Fixed) bool {
	return b.teleport
}

// SetTeleport replaces this callback with an ordinary installation.
func (b *PresentationAdapter) SetTeleport(fn func(moved *units.Unit, fromX, fromY, fromZ, toX, toY, toZ numeric.Fixed) bool) {
	b.teleport = fn
	b.checkpointProofs.teleport = checkpointCallbackProof[PresentationAdapter]{}
}
