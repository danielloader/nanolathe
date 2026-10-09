package orders

import (
	"errors"

	"github.com/nanolathe-gg/nanolathe/internal/combat"
	"github.com/nanolathe-gg/nanolathe/internal/content"
	"github.com/nanolathe-gg/nanolathe/internal/economy"
	"github.com/nanolathe-gg/nanolathe/internal/pool"
	"github.com/nanolathe-gg/nanolathe/internal/sim/checkpoint"
	"github.com/nanolathe-gg/nanolathe/internal/sim/numeric"
	"github.com/nanolathe-gg/nanolathe/internal/sim/rng"
	"github.com/nanolathe-gg/nanolathe/internal/units"
)

// Proof belongs to the exact callback storage owner, independently for every
// slot. The authority proves reviewed installation, not function identity;
// copied callbacks need their own destination installation (§16.3.59).
type checkpointCallbackProof[T any] struct {
	owner     *T
	authority *checkpoint.BindingAuthority
}

func newCheckpointCallbackProof[T any](owner *T, present bool, authority *checkpoint.BindingAuthority) checkpointCallbackProof[T] {
	if !present || authority == nil {
		return checkpointCallbackProof[T]{}
	}
	return checkpointCallbackProof[T]{owner: owner, authority: authority}
}

func (p checkpointCallbackProof[T]) matches(owner *T, authority *checkpoint.BindingAuthority) bool {
	return p.owner == owner && p.authority.Matches(authority)
}

type checkpointQueueBindingProofs struct {
	buildList           checkpointCallbackProof[QueueBinding]
	builderOptions      checkpointCallbackProof[QueueBinding]
	currentTick         checkpointCallbackProof[QueueBinding]
	damage              checkpointCallbackProof[QueueBinding]
	dangerCanRespond    checkpointCallbackProof[QueueBinding]
	dangerRouteFeasible checkpointCallbackProof[QueueBinding]
	dangerStepFeasible  checkpointCallbackProof[QueueBinding]
	dangerVisible       checkpointCallbackProof[QueueBinding]
	hostility           checkpointCallbackProof[QueueBinding]
	lookup              checkpointCallbackProof[QueueBinding]
	modernAIPlayer      checkpointCallbackProof[QueueBinding]
	reclaimFeature      checkpointCallbackProof[QueueBinding]
	resources           checkpointCallbackProof[QueueBinding]
	transportAdmission  checkpointCallbackProof[QueueBinding]
}

// NewQueueBindingWithCheckpointBinding shares ordinary construction and proves
// each present callback independently without invoking it (§16.3.59).
func NewQueueBindingWithCheckpointBinding(c QueueBindingConfig, authority *checkpoint.BindingAuthority) *QueueBinding {
	return newQueueBinding(c, authority)
}

// SetBuildListWithCheckpointBinding performs the ordinary install once,
// then records only this slot's provenance (§16.3.59).
func (b *QueueBinding) SetBuildListWithCheckpointBinding(fn func(*content.UnitDef) bool, authority *checkpoint.BindingAuthority) {
	b.SetBuildList(fn)
	b.checkpointProofs.buildList = newCheckpointCallbackProof(b, fn != nil, authority)
}

// SetBuilderOptionsWithCheckpointBinding performs the ordinary install once,
// then records only this slot's provenance (§16.3.59).
func (b *QueueBinding) SetBuilderOptionsWithCheckpointBinding(fn func(owner uint8) BuilderOptions, authority *checkpoint.BindingAuthority) {
	b.SetBuilderOptions(fn)
	b.checkpointProofs.builderOptions = newCheckpointCallbackProof(b, fn != nil, authority)
}

// SetCurrentTickWithCheckpointBinding performs the ordinary install once,
// then records only this slot's provenance (§16.3.59).
func (b *QueueBinding) SetCurrentTickWithCheckpointBinding(fn func() uint32, authority *checkpoint.BindingAuthority) {
	b.SetCurrentTick(fn)
	b.checkpointProofs.currentTick = newCheckpointCallbackProof(b, fn != nil, authority)
}

// SetDamageWithCheckpointBinding performs the ordinary install once,
// then records only this slot's provenance (§16.3.59).
func (b *QueueBinding) SetDamageWithCheckpointBinding(fn func(uint32, combat.DamageInput) combat.DamageResult, authority *checkpoint.BindingAuthority) {
	b.SetDamage(fn)
	b.checkpointProofs.damage = newCheckpointCallbackProof(b, fn != nil, authority)
}

// SetDangerCanRespondWithCheckpointBinding performs the ordinary install once,
// then records only this slot's provenance (§16.3.59).
func (b *QueueBinding) SetDangerCanRespondWithCheckpointBinding(fn func(observer, target *units.Unit, slot int) bool, authority *checkpoint.BindingAuthority) {
	b.SetDangerCanRespond(fn)
	b.checkpointProofs.dangerCanRespond = newCheckpointCallbackProof(b, fn != nil, authority)
}

// SetDangerRouteFeasibleWithCheckpointBinding performs the ordinary install once,
// then records only this slot's provenance (§16.3.59).
func (b *QueueBinding) SetDangerRouteFeasibleWithCheckpointBinding(fn func(u *units.Unit, x, z numeric.Fixed) bool, authority *checkpoint.BindingAuthority) {
	b.SetDangerRouteFeasible(fn)
	b.checkpointProofs.dangerRouteFeasible = newCheckpointCallbackProof(b, fn != nil, authority)
}

// SetDangerStepFeasibleWithCheckpointBinding performs the ordinary install once,
// then records only this slot's provenance (§16.3.59).
func (b *QueueBinding) SetDangerStepFeasibleWithCheckpointBinding(fn func(u *units.Unit, x, z numeric.Fixed) bool, authority *checkpoint.BindingAuthority) {
	b.SetDangerStepFeasible(fn)
	b.checkpointProofs.dangerStepFeasible = newCheckpointCallbackProof(b, fn != nil, authority)
}

// SetDangerVisibleWithCheckpointBinding performs the ordinary install once,
// then records only this slot's provenance (§16.3.59).
func (b *QueueBinding) SetDangerVisibleWithCheckpointBinding(fn func(observer, target *units.Unit) bool, authority *checkpoint.BindingAuthority) {
	b.SetDangerVisible(fn)
	b.checkpointProofs.dangerVisible = newCheckpointCallbackProof(b, fn != nil, authority)
}

// SetHostilityWithCheckpointBinding performs the ordinary install once,
// then records only this slot's provenance (§16.3.59).
func (b *QueueBinding) SetHostilityWithCheckpointBinding(fn func(actor *units.Unit, target *units.Unit) bool, authority *checkpoint.BindingAuthority) {
	b.SetHostility(fn)
	b.checkpointProofs.hostility = newCheckpointCallbackProof(b, fn != nil, authority)
}

// SetLookupWithCheckpointBinding performs the ordinary install once,
// then records only this slot's provenance (§16.3.59).
func (b *QueueBinding) SetLookupWithCheckpointBinding(fn func(pool.Handle) *units.Unit, authority *checkpoint.BindingAuthority) {
	b.SetLookup(fn)
	b.checkpointProofs.lookup = newCheckpointCallbackProof(b, fn != nil, authority)
}

// SetModernAIPlayerWithCheckpointBinding performs the ordinary install once,
// then records only this slot's provenance (§16.3.59).
func (b *QueueBinding) SetModernAIPlayerWithCheckpointBinding(fn func(owner uint8) bool, authority *checkpoint.BindingAuthority) {
	b.SetModernAIPlayer(fn)
	b.checkpointProofs.modernAIPlayer = newCheckpointCallbackProof(b, fn != nil, authority)
}

// SetReclaimFeatureWithCheckpointBinding performs the ordinary install once,
// then records only this slot's provenance (§16.3.59).
func (b *QueueBinding) SetReclaimFeatureWithCheckpointBinding(fn func(cx, cz int) (metal, energy float32, ok bool), authority *checkpoint.BindingAuthority) {
	b.SetReclaimFeature(fn)
	b.checkpointProofs.reclaimFeature = newCheckpointCallbackProof(b, fn != nil, authority)
}

// SetResourcesWithCheckpointBinding performs the ordinary install once,
// then records only this slot's provenance (§16.3.59).
func (b *QueueBinding) SetResourcesWithCheckpointBinding(fn func(uint8) (ResourceView, bool), authority *checkpoint.BindingAuthority) {
	b.SetResources(fn)
	b.checkpointProofs.resources = newCheckpointCallbackProof(b, fn != nil, authority)
}

// SetTransportAdmissionWithCheckpointBinding performs the ordinary install once,
// then records only this slot's provenance (§16.3.59).
func (b *QueueBinding) SetTransportAdmissionWithCheckpointBinding(fn func(carrier, candidate *units.Unit) bool, authority *checkpoint.BindingAuthority) {
	b.SetTransportAdmission(fn)
	b.checkpointProofs.transportAdmission = newCheckpointCallbackProof(b, fn != nil, authority)
}

func (b *QueueBinding) validateCheckpointCallbacks(authority *checkpoint.BindingAuthority) error {
	if b == nil {
		return nil
	}
	if b.buildList != nil && !b.checkpointProofs.buildList.matches(b, authority) {
		return bindingCheckpointError("QueueBinding.BuildList", "the exact callback owner and authority; TODO(M3-U6) for unattested installations")
	}
	if b.builderOptions != nil && !b.checkpointProofs.builderOptions.matches(b, authority) {
		return bindingCheckpointError("QueueBinding.BuilderOptions", "the exact callback owner and authority; TODO(M3-U6) for unattested installations")
	}
	if b.currentTick != nil && !b.checkpointProofs.currentTick.matches(b, authority) {
		return bindingCheckpointError("QueueBinding.CurrentTick", "the exact callback owner and authority; TODO(M3-U6) for unattested installations")
	}
	if b.damage != nil && !b.checkpointProofs.damage.matches(b, authority) {
		return bindingCheckpointError("QueueBinding.Damage", "the exact callback owner and authority; TODO(M3-U6) for unattested installations")
	}
	if b.dangerCanRespond != nil && !b.checkpointProofs.dangerCanRespond.matches(b, authority) {
		return bindingCheckpointError("QueueBinding.DangerCanRespond", "the exact callback owner and authority; TODO(M3-U6) for unattested installations")
	}
	if b.dangerRouteFeasible != nil && !b.checkpointProofs.dangerRouteFeasible.matches(b, authority) {
		return bindingCheckpointError("QueueBinding.DangerRouteFeasible", "the exact callback owner and authority; TODO(M3-U6) for unattested installations")
	}
	if b.dangerStepFeasible != nil && !b.checkpointProofs.dangerStepFeasible.matches(b, authority) {
		return bindingCheckpointError("QueueBinding.DangerStepFeasible", "the exact callback owner and authority; TODO(M3-U6) for unattested installations")
	}
	if b.dangerVisible != nil && !b.checkpointProofs.dangerVisible.matches(b, authority) {
		return bindingCheckpointError("QueueBinding.DangerVisible", "the exact callback owner and authority; TODO(M3-U6) for unattested installations")
	}
	if b.hostility != nil && !b.checkpointProofs.hostility.matches(b, authority) {
		return bindingCheckpointError("QueueBinding.Hostility", "the exact callback owner and authority; TODO(M3-U6) for unattested installations")
	}
	if b.lookup != nil && !b.checkpointProofs.lookup.matches(b, authority) {
		return bindingCheckpointError("QueueBinding.Lookup", "the exact callback owner and authority; TODO(M3-U6) for unattested installations")
	}
	if b.modernAIPlayer != nil && !b.checkpointProofs.modernAIPlayer.matches(b, authority) {
		return bindingCheckpointError("QueueBinding.ModernAIPlayer", "the exact callback owner and authority; TODO(M3-U6) for unattested installations")
	}
	if b.reclaimFeature != nil && !b.checkpointProofs.reclaimFeature.matches(b, authority) {
		return bindingCheckpointError("QueueBinding.ReclaimFeature", "the exact callback owner and authority; TODO(M3-U6) for unattested installations")
	}
	if b.resources != nil && !b.checkpointProofs.resources.matches(b, authority) {
		return bindingCheckpointError("QueueBinding.Resources", "the exact callback owner and authority; TODO(M3-U6) for unattested installations")
	}
	if b.transportAdmission != nil && !b.checkpointProofs.transportAdmission.matches(b, authority) {
		return bindingCheckpointError("QueueBinding.TransportAdmission", "the exact callback owner and authority; TODO(M3-U6) for unattested installations")
	}
	return nil
}

type checkpointMovementGoalAdapterProofs struct {
	airBases           checkpointCallbackProof[MovementGoalAdapter]
	crowdedMoveBlocked checkpointCallbackProof[MovementGoalAdapter]
	destroy            checkpointCallbackProof[MovementGoalAdapter]
	detachTakeoff      checkpointCallbackProof[MovementGoalAdapter]
	installAir         checkpointCallbackProof[MovementGoalAdapter]
	installAnnulus     checkpointCallbackProof[MovementGoalAdapter]
	installPoint       checkpointCallbackProof[MovementGoalAdapter]
	installRectangle   checkpointCallbackProof[MovementGoalAdapter]
	placeUnit          checkpointCallbackProof[MovementGoalAdapter]
	ready              checkpointCallbackProof[MovementGoalAdapter]
	release            checkpointCallbackProof[MovementGoalAdapter]
	runAir             checkpointCallbackProof[MovementGoalAdapter]
}

// NewMovementGoalAdapterWithCheckpointBinding shares ordinary construction and proves
// each present callback independently without invoking it (§16.3.59).
func NewMovementGoalAdapterWithCheckpointBinding(c MovementGoalAdapterConfig, authority *checkpoint.BindingAuthority) *MovementGoalAdapter {
	return newMovementGoalAdapter(c, authority)
}

// SetAirBasesWithCheckpointBinding performs the ordinary install once,
// then records only this slot's provenance (§16.3.59).
func (b *MovementGoalAdapter) SetAirBasesWithCheckpointBinding(fn func(allyGroup uint8) []pool.Handle, authority *checkpoint.BindingAuthority) {
	b.SetAirBases(fn)
	b.checkpointProofs.airBases = newCheckpointCallbackProof(b, fn != nil, authority)
}

// SetCrowdedMoveBlockedWithCheckpointBinding performs the ordinary install once,
// then records only this slot's provenance (§16.3.59).
func (b *MovementGoalAdapter) SetCrowdedMoveBlockedWithCheckpointBinding(fn func(*units.Unit, *Node) (anchorX, anchorZ int32, blocked bool), authority *checkpoint.BindingAuthority) {
	b.SetCrowdedMoveBlocked(fn)
	b.checkpointProofs.crowdedMoveBlocked = newCheckpointCallbackProof(b, fn != nil, authority)
}

// SetDestroyWithCheckpointBinding performs the ordinary install once,
// then records only this slot's provenance (§16.3.59).
func (b *MovementGoalAdapter) SetDestroyWithCheckpointBinding(fn func(*Node) bool, authority *checkpoint.BindingAuthority) {
	b.SetDestroy(fn)
	b.checkpointProofs.destroy = newCheckpointCallbackProof(b, fn != nil, authority)
}

// SetDetachTakeoffWithCheckpointBinding performs the ordinary install once,
// then records only this slot's provenance (§16.3.59).
func (b *MovementGoalAdapter) SetDetachTakeoffWithCheckpointBinding(fn func(*units.Unit) bool, authority *checkpoint.BindingAuthority) {
	b.SetDetachTakeoff(fn)
	b.checkpointProofs.detachTakeoff = newCheckpointCallbackProof(b, fn != nil, authority)
}

// SetInstallAirWithCheckpointBinding performs the ordinary install once,
// then records only this slot's provenance (§16.3.59).
func (b *MovementGoalAdapter) SetInstallAirWithCheckpointBinding(fn func(AirGoalRequest) bool, authority *checkpoint.BindingAuthority) {
	b.SetInstallAir(fn)
	b.checkpointProofs.installAir = newCheckpointCallbackProof(b, fn != nil, authority)
}

// SetInstallAnnulusWithCheckpointBinding performs the ordinary install once,
// then records only this slot's provenance (§16.3.59).
func (b *MovementGoalAdapter) SetInstallAnnulusWithCheckpointBinding(fn func(AnnulusGoalRequest) bool, authority *checkpoint.BindingAuthority) {
	b.SetInstallAnnulus(fn)
	b.checkpointProofs.installAnnulus = newCheckpointCallbackProof(b, fn != nil, authority)
}

// SetInstallPointWithCheckpointBinding performs the ordinary install once,
// then records only this slot's provenance (§16.3.59).
func (b *MovementGoalAdapter) SetInstallPointWithCheckpointBinding(fn func(PointGoalRequest) bool, authority *checkpoint.BindingAuthority) {
	b.SetInstallPoint(fn)
	b.checkpointProofs.installPoint = newCheckpointCallbackProof(b, fn != nil, authority)
}

// SetInstallRectangleWithCheckpointBinding performs the ordinary install once,
// then records only this slot's provenance (§16.3.59).
func (b *MovementGoalAdapter) SetInstallRectangleWithCheckpointBinding(fn func(RectangleGoalRequest) bool, authority *checkpoint.BindingAuthority) {
	b.SetInstallRectangle(fn)
	b.checkpointProofs.installRectangle = newCheckpointCallbackProof(b, fn != nil, authority)
}

// SetPlaceUnitWithCheckpointBinding performs the ordinary install once,
// then records only this slot's provenance (§16.3.59).
func (b *MovementGoalAdapter) SetPlaceUnitWithCheckpointBinding(fn func(PlaceRequest) bool, authority *checkpoint.BindingAuthority) {
	b.SetPlaceUnit(fn)
	b.checkpointProofs.placeUnit = newCheckpointCallbackProof(b, fn != nil, authority)
}

// SetReadyWithCheckpointBinding performs the ordinary install once,
// then records only this slot's provenance (§16.3.59).
func (b *MovementGoalAdapter) SetReadyWithCheckpointBinding(fn func() bool, authority *checkpoint.BindingAuthority) {
	b.SetReady(fn)
	b.checkpointProofs.ready = newCheckpointCallbackProof(b, fn != nil, authority)
}

// SetReleaseWithCheckpointBinding performs the ordinary install once,
// then records only this slot's provenance (§16.3.59).
func (b *MovementGoalAdapter) SetReleaseWithCheckpointBinding(fn func(*Node) bool, authority *checkpoint.BindingAuthority) {
	b.SetRelease(fn)
	b.checkpointProofs.release = newCheckpointCallbackProof(b, fn != nil, authority)
}

// SetRunAirWithCheckpointBinding performs the ordinary install once,
// then records only this slot's provenance (§16.3.59).
func (b *MovementGoalAdapter) SetRunAirWithCheckpointBinding(fn AirLegRunner, authority *checkpoint.BindingAuthority) {
	b.SetRunAir(fn)
	b.checkpointProofs.runAir = newCheckpointCallbackProof(b, fn != nil, authority)
}

func (b *MovementGoalAdapter) validateCheckpointCallbacks(authority *checkpoint.BindingAuthority) error {
	if b == nil {
		return nil
	}
	if b.airBases != nil && !b.checkpointProofs.airBases.matches(b, authority) {
		return bindingCheckpointError("MovementGoalAdapter.AirBases", "the exact callback owner and authority; TODO(M3-U6) for unattested installations")
	}
	if b.crowdedMoveBlocked != nil && !b.checkpointProofs.crowdedMoveBlocked.matches(b, authority) {
		return bindingCheckpointError("MovementGoalAdapter.CrowdedMoveBlocked", "the exact callback owner and authority; TODO(M3-U6) for unattested installations")
	}
	if b.destroy != nil && !b.checkpointProofs.destroy.matches(b, authority) {
		return bindingCheckpointError("MovementGoalAdapter.Destroy", "the exact callback owner and authority; TODO(M3-U6) for unattested installations")
	}
	if b.detachTakeoff != nil && !b.checkpointProofs.detachTakeoff.matches(b, authority) {
		return bindingCheckpointError("MovementGoalAdapter.DetachTakeoff", "the exact callback owner and authority; TODO(M3-U6) for unattested installations")
	}
	if b.installAir != nil && !b.checkpointProofs.installAir.matches(b, authority) {
		return bindingCheckpointError("MovementGoalAdapter.InstallAir", "the exact callback owner and authority; TODO(M3-U6) for unattested installations")
	}
	if b.installAnnulus != nil && !b.checkpointProofs.installAnnulus.matches(b, authority) {
		return bindingCheckpointError("MovementGoalAdapter.InstallAnnulus", "the exact callback owner and authority; TODO(M3-U6) for unattested installations")
	}
	if b.installPoint != nil && !b.checkpointProofs.installPoint.matches(b, authority) {
		return bindingCheckpointError("MovementGoalAdapter.InstallPoint", "the exact callback owner and authority; TODO(M3-U6) for unattested installations")
	}
	if b.installRectangle != nil && !b.checkpointProofs.installRectangle.matches(b, authority) {
		return bindingCheckpointError("MovementGoalAdapter.InstallRectangle", "the exact callback owner and authority; TODO(M3-U6) for unattested installations")
	}
	if b.placeUnit != nil && !b.checkpointProofs.placeUnit.matches(b, authority) {
		return bindingCheckpointError("MovementGoalAdapter.PlaceUnit", "the exact callback owner and authority; TODO(M3-U6) for unattested installations")
	}
	if b.ready != nil && !b.checkpointProofs.ready.matches(b, authority) {
		return bindingCheckpointError("MovementGoalAdapter.Ready", "the exact callback owner and authority; TODO(M3-U6) for unattested installations")
	}
	if b.release != nil && !b.checkpointProofs.release.matches(b, authority) {
		return bindingCheckpointError("MovementGoalAdapter.Release", "the exact callback owner and authority; TODO(M3-U6) for unattested installations")
	}
	if b.runAir != nil && !b.checkpointProofs.runAir.matches(b, authority) {
		return bindingCheckpointError("MovementGoalAdapter.RunAir", "the exact callback owner and authority; TODO(M3-U6) for unattested installations")
	}
	return nil
}

// Callback payload order: AirBases, CrowdedMoveBlocked, Destroy, DetachTakeoff, InstallAir, InstallAnnulus, InstallPoint, InstallRectangle, PlaceUnit, Ready, Release, RunAir.
func (b *MovementGoalAdapter) writeCheckpoint(e *checkpoint.Encoder) {
	e.Field("MovementGoalAdapter")
	e.Bool(b != nil)
	if b == nil {
		return
	}
	e.Field("MovementGoalAdapter.AirBases")
	e.Bool(b.airBases != nil)
	e.Field("MovementGoalAdapter.CrowdedMoveBlocked")
	e.Bool(b.crowdedMoveBlocked != nil)
	e.Field("MovementGoalAdapter.Destroy")
	e.Bool(b.destroy != nil)
	e.Field("MovementGoalAdapter.DetachTakeoff")
	e.Bool(b.detachTakeoff != nil)
	e.Field("MovementGoalAdapter.InstallAir")
	e.Bool(b.installAir != nil)
	e.Field("MovementGoalAdapter.InstallAnnulus")
	e.Bool(b.installAnnulus != nil)
	e.Field("MovementGoalAdapter.InstallPoint")
	e.Bool(b.installPoint != nil)
	e.Field("MovementGoalAdapter.InstallRectangle")
	e.Bool(b.installRectangle != nil)
	e.Field("MovementGoalAdapter.PlaceUnit")
	e.Bool(b.placeUnit != nil)
	e.Field("MovementGoalAdapter.Ready")
	e.Bool(b.ready != nil)
	e.Field("MovementGoalAdapter.Release")
	e.Bool(b.release != nil)
	e.Field("MovementGoalAdapter.RunAir")
	e.Bool(b.runAir != nil)
}

type checkpointWorldQueryAdapterProofs struct {
	declaresAlliance    checkpointCallbackProof[WorldQueryAdapter]
	forEachFeature      checkpointCallbackProof[WorldQueryAdapter]
	forEachUnit         checkpointCallbackProof[WorldQueryAdapter]
	forEachUnitInRadius checkpointCallbackProof[WorldQueryAdapter]
	hostile             checkpointCallbackProof[WorldQueryAdapter]
	lookupFeature       checkpointCallbackProof[WorldQueryAdapter]
	lookupUnit          checkpointCallbackProof[WorldQueryAdapter]
	mappingWord         checkpointCallbackProof[WorldQueryAdapter]
	seaLevel            checkpointCallbackProof[WorldQueryAdapter]
	terrainHeight       checkpointCallbackProof[WorldQueryAdapter]
}

// NewWorldQueryAdapterWithCheckpointBinding shares ordinary construction and proves
// each present callback independently without invoking it (§16.3.59).
func NewWorldQueryAdapterWithCheckpointBinding(c WorldQueryAdapterConfig, authority *checkpoint.BindingAuthority) *WorldQueryAdapter {
	return newWorldQueryAdapter(c, authority)
}

// SetDeclaresAllianceWithCheckpointBinding performs the ordinary install once,
// then records only this slot's provenance (§16.3.59).
func (b *WorldQueryAdapter) SetDeclaresAllianceWithCheckpointBinding(fn func(from, toward uint8) bool, authority *checkpoint.BindingAuthority) {
	b.SetDeclaresAlliance(fn)
	b.checkpointProofs.declaresAlliance = newCheckpointCallbackProof(b, fn != nil, authority)
}

// SetForEachFeatureWithCheckpointBinding performs the ordinary install once,
// then records only this slot's provenance (§16.3.59).
func (b *WorldQueryAdapter) SetForEachFeatureWithCheckpointBinding(fn func(func(FeatureView) bool), authority *checkpoint.BindingAuthority) {
	b.SetForEachFeature(fn)
	b.checkpointProofs.forEachFeature = newCheckpointCallbackProof(b, fn != nil, authority)
}

// SetForEachUnitWithCheckpointBinding performs the ordinary install once,
// then records only this slot's provenance (§16.3.59).
func (b *WorldQueryAdapter) SetForEachUnitWithCheckpointBinding(fn func(func(pool.Handle, *units.Unit) bool), authority *checkpoint.BindingAuthority) {
	b.SetForEachUnit(fn)
	b.checkpointProofs.forEachUnit = newCheckpointCallbackProof(b, fn != nil, authority)
}

// SetForEachUnitInRadiusWithCheckpointBinding performs the ordinary install once,
// then records only this slot's provenance (§16.3.59).
func (b *WorldQueryAdapter) SetForEachUnitInRadiusWithCheckpointBinding(fn func(numeric.Fixed, numeric.Fixed, numeric.Fixed, func(pool.Handle, *units.Unit) bool), authority *checkpoint.BindingAuthority) {
	b.SetForEachUnitInRadius(fn)
	b.checkpointProofs.forEachUnitInRadius = newCheckpointCallbackProof(b, fn != nil, authority)
}

// SetHostileWithCheckpointBinding performs the ordinary install once,
// then records only this slot's provenance (§16.3.59).
func (b *WorldQueryAdapter) SetHostileWithCheckpointBinding(fn func(*units.Unit, *units.Unit) bool, authority *checkpoint.BindingAuthority) {
	b.SetHostile(fn)
	b.checkpointProofs.hostile = newCheckpointCallbackProof(b, fn != nil, authority)
}

// SetLookupFeatureWithCheckpointBinding performs the ordinary install once,
// then records only this slot's provenance (§16.3.59).
func (b *WorldQueryAdapter) SetLookupFeatureWithCheckpointBinding(fn func(int32, int32) (FeatureView, bool), authority *checkpoint.BindingAuthority) {
	b.SetLookupFeature(fn)
	b.checkpointProofs.lookupFeature = newCheckpointCallbackProof(b, fn != nil, authority)
}

// SetLookupUnitWithCheckpointBinding performs the ordinary install once,
// then records only this slot's provenance (§16.3.59).
func (b *WorldQueryAdapter) SetLookupUnitWithCheckpointBinding(fn func(pool.Handle) *units.Unit, authority *checkpoint.BindingAuthority) {
	b.SetLookupUnit(fn)
	b.checkpointProofs.lookupUnit = newCheckpointCallbackProof(b, fn != nil, authority)
}

// SetMappingWordWithCheckpointBinding performs the ordinary install once,
// then records only this slot's provenance (§16.3.59).
func (b *WorldQueryAdapter) SetMappingWordWithCheckpointBinding(fn func(tileX, tileZ int32) (uint16, bool), authority *checkpoint.BindingAuthority) {
	b.SetMappingWord(fn)
	b.checkpointProofs.mappingWord = newCheckpointCallbackProof(b, fn != nil, authority)
}

// SetSeaLevelWithCheckpointBinding performs the ordinary install once,
// then records only this slot's provenance (§16.3.59).
func (b *WorldQueryAdapter) SetSeaLevelWithCheckpointBinding(fn func() uint8, authority *checkpoint.BindingAuthority) {
	b.SetSeaLevel(fn)
	b.checkpointProofs.seaLevel = newCheckpointCallbackProof(b, fn != nil, authority)
}

// SetTerrainHeightWithCheckpointBinding performs the ordinary install once,
// then records only this slot's provenance (§16.3.59).
func (b *WorldQueryAdapter) SetTerrainHeightWithCheckpointBinding(fn func(numeric.Fixed, numeric.Fixed) (numeric.Fixed, bool), authority *checkpoint.BindingAuthority) {
	b.SetTerrainHeight(fn)
	b.checkpointProofs.terrainHeight = newCheckpointCallbackProof(b, fn != nil, authority)
}

func (b *WorldQueryAdapter) validateCheckpointCallbacks(authority *checkpoint.BindingAuthority) error {
	if b == nil {
		return nil
	}
	if b.declaresAlliance != nil && !b.checkpointProofs.declaresAlliance.matches(b, authority) {
		return bindingCheckpointError("WorldQueryAdapter.DeclaresAlliance", "the exact callback owner and authority; TODO(M3-U6) for unattested installations")
	}
	if b.forEachFeature != nil && !b.checkpointProofs.forEachFeature.matches(b, authority) {
		return bindingCheckpointError("WorldQueryAdapter.ForEachFeature", "the exact callback owner and authority; TODO(M3-U6) for unattested installations")
	}
	if b.forEachUnit != nil && !b.checkpointProofs.forEachUnit.matches(b, authority) {
		return bindingCheckpointError("WorldQueryAdapter.ForEachUnit", "the exact callback owner and authority; TODO(M3-U6) for unattested installations")
	}
	if b.forEachUnitInRadius != nil && !b.checkpointProofs.forEachUnitInRadius.matches(b, authority) {
		return bindingCheckpointError("WorldQueryAdapter.ForEachUnitInRadius", "the exact callback owner and authority; TODO(M3-U6) for unattested installations")
	}
	if b.hostile != nil && !b.checkpointProofs.hostile.matches(b, authority) {
		return bindingCheckpointError("WorldQueryAdapter.Hostile", "the exact callback owner and authority; TODO(M3-U6) for unattested installations")
	}
	if b.lookupFeature != nil && !b.checkpointProofs.lookupFeature.matches(b, authority) {
		return bindingCheckpointError("WorldQueryAdapter.LookupFeature", "the exact callback owner and authority; TODO(M3-U6) for unattested installations")
	}
	if b.lookupUnit != nil && !b.checkpointProofs.lookupUnit.matches(b, authority) {
		return bindingCheckpointError("WorldQueryAdapter.LookupUnit", "the exact callback owner and authority; TODO(M3-U6) for unattested installations")
	}
	if b.mappingWord != nil && !b.checkpointProofs.mappingWord.matches(b, authority) {
		return bindingCheckpointError("WorldQueryAdapter.MappingWord", "the exact callback owner and authority; TODO(M3-U6) for unattested installations")
	}
	if b.seaLevel != nil && !b.checkpointProofs.seaLevel.matches(b, authority) {
		return bindingCheckpointError("WorldQueryAdapter.SeaLevel", "the exact callback owner and authority; TODO(M3-U6) for unattested installations")
	}
	if b.terrainHeight != nil && !b.checkpointProofs.terrainHeight.matches(b, authority) {
		return bindingCheckpointError("WorldQueryAdapter.TerrainHeight", "the exact callback owner and authority; TODO(M3-U6) for unattested installations")
	}
	return nil
}

// Callback payload order: DeclaresAlliance, ForEachFeature, ForEachUnit, ForEachUnitInRadius, Hostile, LookupFeature, LookupUnit, MappingWord, SeaLevel, TerrainHeight.
func (b *WorldQueryAdapter) writeCheckpoint(e *checkpoint.Encoder) {
	e.Field("WorldQueryAdapter")
	e.Bool(b != nil)
	if b == nil {
		return
	}
	e.Field("WorldQueryAdapter.DeclaresAlliance")
	e.Bool(b.declaresAlliance != nil)
	e.Field("WorldQueryAdapter.ForEachFeature")
	e.Bool(b.forEachFeature != nil)
	e.Field("WorldQueryAdapter.ForEachUnit")
	e.Bool(b.forEachUnit != nil)
	e.Field("WorldQueryAdapter.ForEachUnitInRadius")
	e.Bool(b.forEachUnitInRadius != nil)
	e.Field("WorldQueryAdapter.Hostile")
	e.Bool(b.hostile != nil)
	e.Field("WorldQueryAdapter.LookupFeature")
	e.Bool(b.lookupFeature != nil)
	e.Field("WorldQueryAdapter.LookupUnit")
	e.Bool(b.lookupUnit != nil)
	e.Field("WorldQueryAdapter.MappingWord")
	e.Bool(b.mappingWord != nil)
	e.Field("WorldQueryAdapter.SeaLevel")
	e.Bool(b.seaLevel != nil)
	e.Field("WorldQueryAdapter.TerrainHeight")
	e.Bool(b.terrainHeight != nil)
}

type checkpointWorkAdapterProofs struct {
	assist              checkpointCallbackProof[WorkAdapter]
	canResurrectFeature checkpointCallbackProof[WorkAdapter]
	cancelNotice        checkpointCallbackProof[WorkAdapter]
	capture             checkpointCallbackProof[WorkAdapter]
	ready               checkpointCallbackProof[WorkAdapter]
	repair              checkpointCallbackProof[WorkAdapter]
	resurrect           checkpointCallbackProof[WorkAdapter]
}

// NewWorkAdapterWithCheckpointBinding shares ordinary construction and proves
// each present callback independently without invoking it (§16.3.59).
func NewWorkAdapterWithCheckpointBinding(c WorkAdapterConfig, authority *checkpoint.BindingAuthority) *WorkAdapter {
	return newWorkAdapter(c, authority)
}

// SetAssistWithCheckpointBinding performs the ordinary install once,
// then records only this slot's provenance (§16.3.59).
func (b *WorkAdapter) SetAssistWithCheckpointBinding(fn func(*units.Unit, *Node, uint32) bool, authority *checkpoint.BindingAuthority) {
	b.SetAssist(fn)
	b.checkpointProofs.assist = newCheckpointCallbackProof(b, fn != nil, authority)
}

// SetCanResurrectFeatureWithCheckpointBinding performs the ordinary install once,
// then records only this slot's provenance (§16.3.59).
func (b *WorkAdapter) SetCanResurrectFeatureWithCheckpointBinding(fn func(FeatureView) bool, authority *checkpoint.BindingAuthority) {
	b.SetCanResurrectFeature(fn)
	b.checkpointProofs.canResurrectFeature = newCheckpointCallbackProof(b, fn != nil, authority)
}

// SetCancelNoticeWithCheckpointBinding performs the ordinary install once,
// then records only this slot's provenance (§16.3.59).
func (b *WorkAdapter) SetCancelNoticeWithCheckpointBinding(fn func(owner *units.Unit, n *Node, tick uint32) bool, authority *checkpoint.BindingAuthority) {
	b.SetCancelNotice(fn)
	b.checkpointProofs.cancelNotice = newCheckpointCallbackProof(b, fn != nil, authority)
}

// SetCaptureWithCheckpointBinding performs the ordinary install once,
// then records only this slot's provenance (§16.3.59).
func (b *WorkAdapter) SetCaptureWithCheckpointBinding(fn func(*units.Unit, *Node, uint32) bool, authority *checkpoint.BindingAuthority) {
	b.SetCapture(fn)
	b.checkpointProofs.capture = newCheckpointCallbackProof(b, fn != nil, authority)
}

// SetReadyWithCheckpointBinding performs the ordinary install once,
// then records only this slot's provenance (§16.3.59).
func (b *WorkAdapter) SetReadyWithCheckpointBinding(fn func() bool, authority *checkpoint.BindingAuthority) {
	b.SetReady(fn)
	b.checkpointProofs.ready = newCheckpointCallbackProof(b, fn != nil, authority)
}

// SetRepairWithCheckpointBinding performs the ordinary install once,
// then records only this slot's provenance (§16.3.59).
func (b *WorkAdapter) SetRepairWithCheckpointBinding(fn func(builder, patient *units.Unit, node *Node, tick uint32) bool, authority *checkpoint.BindingAuthority) {
	b.SetRepair(fn)
	b.checkpointProofs.repair = newCheckpointCallbackProof(b, fn != nil, authority)
}

// SetResurrectWithCheckpointBinding performs the ordinary install once,
// then records only this slot's provenance (§16.3.59).
func (b *WorkAdapter) SetResurrectWithCheckpointBinding(fn func(*units.Unit, *Node, uint32) bool, authority *checkpoint.BindingAuthority) {
	b.SetResurrect(fn)
	b.checkpointProofs.resurrect = newCheckpointCallbackProof(b, fn != nil, authority)
}

func (b *WorkAdapter) validateCheckpointCallbacks(authority *checkpoint.BindingAuthority) error {
	if b == nil {
		return nil
	}
	if b.assist != nil && !b.checkpointProofs.assist.matches(b, authority) {
		return bindingCheckpointError("WorkAdapter.Assist", "the exact callback owner and authority; TODO(M3-U6) for unattested installations")
	}
	if b.canResurrectFeature != nil && !b.checkpointProofs.canResurrectFeature.matches(b, authority) {
		return bindingCheckpointError("WorkAdapter.CanResurrectFeature", "the exact callback owner and authority; TODO(M3-U6) for unattested installations")
	}
	if b.cancelNotice != nil && !b.checkpointProofs.cancelNotice.matches(b, authority) {
		return bindingCheckpointError("WorkAdapter.CancelNotice", "the exact callback owner and authority; TODO(M3-U6) for unattested installations")
	}
	if b.capture != nil && !b.checkpointProofs.capture.matches(b, authority) {
		return bindingCheckpointError("WorkAdapter.Capture", "the exact callback owner and authority; TODO(M3-U6) for unattested installations")
	}
	if b.ready != nil && !b.checkpointProofs.ready.matches(b, authority) {
		return bindingCheckpointError("WorkAdapter.Ready", "the exact callback owner and authority; TODO(M3-U6) for unattested installations")
	}
	if b.repair != nil && !b.checkpointProofs.repair.matches(b, authority) {
		return bindingCheckpointError("WorkAdapter.Repair", "the exact callback owner and authority; TODO(M3-U6) for unattested installations")
	}
	if b.resurrect != nil && !b.checkpointProofs.resurrect.matches(b, authority) {
		return bindingCheckpointError("WorkAdapter.Resurrect", "the exact callback owner and authority; TODO(M3-U6) for unattested installations")
	}
	return nil
}

// Callback payload order: Assist, CanResurrectFeature, CancelNotice, Capture, Ready, Repair, Resurrect.
func (b *WorkAdapter) writeCheckpoint(e *checkpoint.Encoder) {
	e.Field("WorkAdapter")
	e.Bool(b != nil)
	if b == nil {
		return
	}
	e.Field("WorkAdapter.Assist")
	e.Bool(b.assist != nil)
	e.Field("WorkAdapter.CanResurrectFeature")
	e.Bool(b.canResurrectFeature != nil)
	e.Field("WorkAdapter.CancelNotice")
	e.Bool(b.cancelNotice != nil)
	e.Field("WorkAdapter.Capture")
	e.Bool(b.capture != nil)
	e.Field("WorkAdapter.Ready")
	e.Bool(b.ready != nil)
	e.Field("WorkAdapter.Repair")
	e.Bool(b.repair != nil)
	e.Field("WorkAdapter.Resurrect")
	e.Bool(b.resurrect != nil)
}

type checkpointWeaponAdapterProofs struct {
	acquire               checkpointCallbackProof[WeaponAdapter]
	canEngage             checkpointCallbackProof[WeaponAdapter]
	engaged               checkpointCallbackProof[WeaponAdapter]
	firePoint             checkpointCallbackProof[WeaponAdapter]
	fireTarget            checkpointCallbackProof[WeaponAdapter]
	firingPositionBlocked checkpointCallbackProof[WeaponAdapter]
	firingPositionClear   checkpointCallbackProof[WeaponAdapter]
	inhibitSlot           checkpointCallbackProof[WeaponAdapter]
	ready                 checkpointCallbackProof[WeaponAdapter]
	releaseSlot           checkpointCallbackProof[WeaponAdapter]
	setManualTarget       checkpointCallbackProof[WeaponAdapter]
	stopFiring            checkpointCallbackProof[WeaponAdapter]
	targetsInRadius       checkpointCallbackProof[WeaponAdapter]
}

// NewWeaponAdapterWithCheckpointBinding shares ordinary construction and proves
// each present callback independently without invoking it (§16.3.59).
func NewWeaponAdapterWithCheckpointBinding(c WeaponAdapterConfig, authority *checkpoint.BindingAuthority) *WeaponAdapter {
	return newWeaponAdapter(c, authority)
}

// SetAcquireWithCheckpointBinding performs the ordinary install once,
// then records only this slot's provenance (§16.3.59).
func (b *WeaponAdapter) SetAcquireWithCheckpointBinding(fn func(*units.Unit, int, uint32) (pool.Handle, bool), authority *checkpoint.BindingAuthority) {
	b.SetAcquire(fn)
	b.checkpointProofs.acquire = newCheckpointCallbackProof(b, fn != nil, authority)
}

// SetCanEngageWithCheckpointBinding performs the ordinary install once,
// then records only this slot's provenance (§16.3.59).
func (b *WeaponAdapter) SetCanEngageWithCheckpointBinding(fn func(*units.Unit, pool.Handle, int) bool, authority *checkpoint.BindingAuthority) {
	b.SetCanEngage(fn)
	b.checkpointProofs.canEngage = newCheckpointCallbackProof(b, fn != nil, authority)
}

// SetEngagedWithCheckpointBinding performs the ordinary install once,
// then records only this slot's provenance (§16.3.59).
func (b *WeaponAdapter) SetEngagedWithCheckpointBinding(fn func(*units.Unit, int) bool, authority *checkpoint.BindingAuthority) {
	b.SetEngaged(fn)
	b.checkpointProofs.engaged = newCheckpointCallbackProof(b, fn != nil, authority)
}

// SetFirePointWithCheckpointBinding performs the ordinary install once,
// then records only this slot's provenance (§16.3.59).
func (b *WeaponAdapter) SetFirePointWithCheckpointBinding(fn func(*units.Unit, int, numeric.Fixed, numeric.Fixed, uint32) bool, authority *checkpoint.BindingAuthority) {
	b.SetFirePoint(fn)
	b.checkpointProofs.firePoint = newCheckpointCallbackProof(b, fn != nil, authority)
}

// SetFireTargetWithCheckpointBinding performs the ordinary install once,
// then records only this slot's provenance (§16.3.59).
func (b *WeaponAdapter) SetFireTargetWithCheckpointBinding(fn func(*units.Unit, int, pool.Handle, uint32) bool, authority *checkpoint.BindingAuthority) {
	b.SetFireTarget(fn)
	b.checkpointProofs.fireTarget = newCheckpointCallbackProof(b, fn != nil, authority)
}

// SetFiringPositionBlockedWithCheckpointBinding performs the ordinary install once,
// then records only this slot's provenance (§16.3.59).
func (b *WeaponAdapter) SetFiringPositionBlockedWithCheckpointBinding(fn func(shooter, target *units.Unit, tick uint32) bool, authority *checkpoint.BindingAuthority) {
	b.SetFiringPositionBlocked(fn)
	b.checkpointProofs.firingPositionBlocked = newCheckpointCallbackProof(b, fn != nil, authority)
}

// SetFiringPositionClearWithCheckpointBinding performs the ordinary install once,
// then records only this slot's provenance (§16.3.59).
func (b *WeaponAdapter) SetFiringPositionClearWithCheckpointBinding(fn func(shooter, target *units.Unit, tick uint32, x, y, z numeric.Fixed) bool, authority *checkpoint.BindingAuthority) {
	b.SetFiringPositionClear(fn)
	b.checkpointProofs.firingPositionClear = newCheckpointCallbackProof(b, fn != nil, authority)
}

// SetInhibitSlotWithCheckpointBinding performs the ordinary install once,
// then records only this slot's provenance (§16.3.59).
func (b *WeaponAdapter) SetInhibitSlotWithCheckpointBinding(fn func(*units.Unit, int) bool, authority *checkpoint.BindingAuthority) {
	b.SetInhibitSlot(fn)
	b.checkpointProofs.inhibitSlot = newCheckpointCallbackProof(b, fn != nil, authority)
}

// SetReadyWithCheckpointBinding performs the ordinary install once,
// then records only this slot's provenance (§16.3.59).
func (b *WeaponAdapter) SetReadyWithCheckpointBinding(fn func() bool, authority *checkpoint.BindingAuthority) {
	b.SetReady(fn)
	b.checkpointProofs.ready = newCheckpointCallbackProof(b, fn != nil, authority)
}

// SetReleaseSlotWithCheckpointBinding performs the ordinary install once,
// then records only this slot's provenance (§16.3.59).
func (b *WeaponAdapter) SetReleaseSlotWithCheckpointBinding(fn func(*units.Unit, int) bool, authority *checkpoint.BindingAuthority) {
	b.SetReleaseSlot(fn)
	b.checkpointProofs.releaseSlot = newCheckpointCallbackProof(b, fn != nil, authority)
}

// SetSetManualTargetWithCheckpointBinding performs the ordinary install once,
// then records only this slot's provenance (§16.3.59).
func (b *WeaponAdapter) SetSetManualTargetWithCheckpointBinding(fn func(*units.Unit, int, pool.Handle) bool, authority *checkpoint.BindingAuthority) {
	b.SetSetManualTarget(fn)
	b.checkpointProofs.setManualTarget = newCheckpointCallbackProof(b, fn != nil, authority)
}

// SetStopFiringWithCheckpointBinding performs the ordinary install once,
// then records only this slot's provenance (§16.3.59).
func (b *WeaponAdapter) SetStopFiringWithCheckpointBinding(fn func(*units.Unit, int) bool, authority *checkpoint.BindingAuthority) {
	b.SetStopFiring(fn)
	b.checkpointProofs.stopFiring = newCheckpointCallbackProof(b, fn != nil, authority)
}

// SetTargetsInRadiusWithCheckpointBinding performs the ordinary install once,
// then records only this slot's provenance (§16.3.59).
func (b *WeaponAdapter) SetTargetsInRadiusWithCheckpointBinding(fn func(*units.Unit, numeric.Fixed, numeric.Fixed, int32) []pool.Handle, authority *checkpoint.BindingAuthority) {
	b.SetTargetsInRadius(fn)
	b.checkpointProofs.targetsInRadius = newCheckpointCallbackProof(b, fn != nil, authority)
}

func (b *WeaponAdapter) validateCheckpointCallbacks(authority *checkpoint.BindingAuthority) error {
	if b == nil {
		return nil
	}
	if b.acquire != nil && !b.checkpointProofs.acquire.matches(b, authority) {
		return bindingCheckpointError("WeaponAdapter.Acquire", "the exact callback owner and authority; TODO(M3-U6) for unattested installations")
	}
	if b.canEngage != nil && !b.checkpointProofs.canEngage.matches(b, authority) {
		return bindingCheckpointError("WeaponAdapter.CanEngage", "the exact callback owner and authority; TODO(M3-U6) for unattested installations")
	}
	if b.engaged != nil && !b.checkpointProofs.engaged.matches(b, authority) {
		return bindingCheckpointError("WeaponAdapter.Engaged", "the exact callback owner and authority; TODO(M3-U6) for unattested installations")
	}
	if b.firePoint != nil && !b.checkpointProofs.firePoint.matches(b, authority) {
		return bindingCheckpointError("WeaponAdapter.FirePoint", "the exact callback owner and authority; TODO(M3-U6) for unattested installations")
	}
	if b.fireTarget != nil && !b.checkpointProofs.fireTarget.matches(b, authority) {
		return bindingCheckpointError("WeaponAdapter.FireTarget", "the exact callback owner and authority; TODO(M3-U6) for unattested installations")
	}
	if b.firingPositionBlocked != nil && !b.checkpointProofs.firingPositionBlocked.matches(b, authority) {
		return bindingCheckpointError("WeaponAdapter.FiringPositionBlocked", "the exact callback owner and authority; TODO(M3-U6) for unattested installations")
	}
	if b.firingPositionClear != nil && !b.checkpointProofs.firingPositionClear.matches(b, authority) {
		return bindingCheckpointError("WeaponAdapter.FiringPositionClear", "the exact callback owner and authority; TODO(M3-U6) for unattested installations")
	}
	if b.inhibitSlot != nil && !b.checkpointProofs.inhibitSlot.matches(b, authority) {
		return bindingCheckpointError("WeaponAdapter.InhibitSlot", "the exact callback owner and authority; TODO(M3-U6) for unattested installations")
	}
	if b.ready != nil && !b.checkpointProofs.ready.matches(b, authority) {
		return bindingCheckpointError("WeaponAdapter.Ready", "the exact callback owner and authority; TODO(M3-U6) for unattested installations")
	}
	if b.releaseSlot != nil && !b.checkpointProofs.releaseSlot.matches(b, authority) {
		return bindingCheckpointError("WeaponAdapter.ReleaseSlot", "the exact callback owner and authority; TODO(M3-U6) for unattested installations")
	}
	if b.setManualTarget != nil && !b.checkpointProofs.setManualTarget.matches(b, authority) {
		return bindingCheckpointError("WeaponAdapter.SetManualTarget", "the exact callback owner and authority; TODO(M3-U6) for unattested installations")
	}
	if b.stopFiring != nil && !b.checkpointProofs.stopFiring.matches(b, authority) {
		return bindingCheckpointError("WeaponAdapter.StopFiring", "the exact callback owner and authority; TODO(M3-U6) for unattested installations")
	}
	if b.targetsInRadius != nil && !b.checkpointProofs.targetsInRadius.matches(b, authority) {
		return bindingCheckpointError("WeaponAdapter.TargetsInRadius", "the exact callback owner and authority; TODO(M3-U6) for unattested installations")
	}
	return nil
}

// Callback payload order: Acquire, CanEngage, Engaged, FirePoint, FireTarget, FiringPositionBlocked, FiringPositionClear, InhibitSlot, Ready, ReleaseSlot, SetManualTarget, StopFiring, TargetsInRadius.
func (b *WeaponAdapter) writeCheckpoint(e *checkpoint.Encoder) {
	e.Field("WeaponAdapter")
	e.Bool(b != nil)
	if b == nil {
		return
	}
	e.Field("WeaponAdapter.Acquire")
	e.Bool(b.acquire != nil)
	e.Field("WeaponAdapter.CanEngage")
	e.Bool(b.canEngage != nil)
	e.Field("WeaponAdapter.Engaged")
	e.Bool(b.engaged != nil)
	e.Field("WeaponAdapter.FirePoint")
	e.Bool(b.firePoint != nil)
	e.Field("WeaponAdapter.FireTarget")
	e.Bool(b.fireTarget != nil)
	e.Field("WeaponAdapter.FiringPositionBlocked")
	e.Bool(b.firingPositionBlocked != nil)
	e.Field("WeaponAdapter.FiringPositionClear")
	e.Bool(b.firingPositionClear != nil)
	e.Field("WeaponAdapter.InhibitSlot")
	e.Bool(b.inhibitSlot != nil)
	e.Field("WeaponAdapter.Ready")
	e.Bool(b.ready != nil)
	e.Field("WeaponAdapter.ReleaseSlot")
	e.Bool(b.releaseSlot != nil)
	e.Field("WeaponAdapter.SetManualTarget")
	e.Bool(b.setManualTarget != nil)
	e.Field("WeaponAdapter.StopFiring")
	e.Bool(b.stopFiring != nil)
	e.Field("WeaponAdapter.TargetsInRadius")
	e.Bool(b.targetsInRadius != nil)
}

type checkpointPresentationAdapterProofs struct {
	nanolathe        checkpointCallbackProof[PresentationAdapter]
	nanolatheFeature checkpointCallbackProof[PresentationAdapter]
	ready            checkpointCallbackProof[PresentationAdapter]
	status           checkpointCallbackProof[PresentationAdapter]
	teleport         checkpointCallbackProof[PresentationAdapter]
}

// NewPresentationAdapterWithCheckpointBinding shares ordinary construction and proves
// each present callback independently without invoking it (§16.3.59).
func NewPresentationAdapterWithCheckpointBinding(c PresentationAdapterConfig, authority *checkpoint.BindingAuthority) *PresentationAdapter {
	return newPresentationAdapter(c, authority)
}

// SetNanolatheWithCheckpointBinding performs the ordinary install once,
// then records only this slot's provenance (§16.3.59).
func (b *PresentationAdapter) SetNanolatheWithCheckpointBinding(fn func(*units.Unit, *Node, uint32) bool, authority *checkpoint.BindingAuthority) {
	b.SetNanolathe(fn)
	b.checkpointProofs.nanolathe = newCheckpointCallbackProof(b, fn != nil, authority)
}

// SetNanolatheFeatureWithCheckpointBinding performs the ordinary install once,
// then records only this slot's provenance (§16.3.59).
func (b *PresentationAdapter) SetNanolatheFeatureWithCheckpointBinding(fn func(*units.Unit, *Node, FeatureView, uint32) bool, authority *checkpoint.BindingAuthority) {
	b.SetNanolatheFeature(fn)
	b.checkpointProofs.nanolatheFeature = newCheckpointCallbackProof(b, fn != nil, authority)
}

// SetReadyWithCheckpointBinding performs the ordinary install once,
// then records only this slot's provenance (§16.3.59).
func (b *PresentationAdapter) SetReadyWithCheckpointBinding(fn func() bool, authority *checkpoint.BindingAuthority) {
	b.SetReady(fn)
	b.checkpointProofs.ready = newCheckpointCallbackProof(b, fn != nil, authority)
}

// SetStatusWithCheckpointBinding performs the ordinary install once,
// then records only this slot's provenance (§16.3.59).
func (b *PresentationAdapter) SetStatusWithCheckpointBinding(fn func(*units.Unit, uint8, string) bool, authority *checkpoint.BindingAuthority) {
	b.SetStatus(fn)
	b.checkpointProofs.status = newCheckpointCallbackProof(b, fn != nil, authority)
}

// SetTeleportWithCheckpointBinding performs the ordinary install once,
// then records only this slot's provenance (§16.3.59).
func (b *PresentationAdapter) SetTeleportWithCheckpointBinding(fn func(moved *units.Unit, fromX, fromY, fromZ, toX, toY, toZ numeric.Fixed) bool, authority *checkpoint.BindingAuthority) {
	b.SetTeleport(fn)
	b.checkpointProofs.teleport = newCheckpointCallbackProof(b, fn != nil, authority)
}

func (b *PresentationAdapter) validateCheckpointCallbacks(authority *checkpoint.BindingAuthority) error {
	if b == nil {
		return nil
	}
	if b.nanolathe != nil && !b.checkpointProofs.nanolathe.matches(b, authority) {
		return bindingCheckpointError("PresentationAdapter.Nanolathe", "the exact callback owner and authority; TODO(M3-U6) for unattested installations")
	}
	if b.nanolatheFeature != nil && !b.checkpointProofs.nanolatheFeature.matches(b, authority) {
		return bindingCheckpointError("PresentationAdapter.NanolatheFeature", "the exact callback owner and authority; TODO(M3-U6) for unattested installations")
	}
	if b.ready != nil && !b.checkpointProofs.ready.matches(b, authority) {
		return bindingCheckpointError("PresentationAdapter.Ready", "the exact callback owner and authority; TODO(M3-U6) for unattested installations")
	}
	if b.status != nil && !b.checkpointProofs.status.matches(b, authority) {
		return bindingCheckpointError("PresentationAdapter.Status", "the exact callback owner and authority; TODO(M3-U6) for unattested installations")
	}
	if b.teleport != nil && !b.checkpointProofs.teleport.matches(b, authority) {
		return bindingCheckpointError("PresentationAdapter.Teleport", "the exact callback owner and authority; TODO(M3-U6) for unattested installations")
	}
	return nil
}

// Callback payload order: Nanolathe, NanolatheFeature, Ready, Status, Teleport.
func (b *PresentationAdapter) writeCheckpoint(e *checkpoint.Encoder) {
	e.Field("PresentationAdapter")
	e.Bool(b != nil)
	if b == nil {
		return
	}
	e.Field("PresentationAdapter.Nanolathe")
	e.Bool(b.nanolathe != nil)
	e.Field("PresentationAdapter.NanolatheFeature")
	e.Bool(b.nanolatheFeature != nil)
	e.Field("PresentationAdapter.Ready")
	e.Bool(b.ready != nil)
	e.Field("PresentationAdapter.Status")
	e.Bool(b.status != nil)
	e.Field("PresentationAdapter.Teleport")
	e.Bool(b.teleport != nil)
}

// The capture-local tuple never installs live proof. Adapter pointers are
// recorded explicitly because their callbacks belong to those exact owners;
// the session separately proves captured cross-adapter edges (§16.3.59).
type checkpointBindingSources struct {
	binding      *QueueBinding
	economy      *economy.Service
	sim          *rng.Simulation
	authority    *checkpoint.BindingAuthority
	movement     *MovementGoalAdapter
	presentation *PresentationAdapter
	weapons      *WeaponAdapter
	work         *WorkAdapter
	world        *WorldQueryAdapter
}

// SetBindings checks the complete actual tuple before registering it. An exact
// repeat revalidates live fields; any failure leaves the context unchanged.
func (c *CheckpointContext) SetBindings(b *QueueBinding, economyOwner *economy.Service, sim *rng.Simulation, authority *checkpoint.BindingAuthority) error {
	if c == nil || b == nil || authority == nil {
		return bindingCheckpointError("QueueBinding", "a context, binding and authority")
	}
	if c.handlerAuthority != nil && !c.handlerAuthority.Matches(authority) {
		return bindingCheckpointError("QueueBinding", "one authority for binding and handler sources")
	}
	sources := checkpointBindingSources{binding: b, economy: economyOwner, sim: sim, authority: authority,
		movement: b.Movement, presentation: b.Presentation, weapons: b.Weapons, work: b.Work, world: b.World}
	if c.bindings != nil && *c.bindings != sources {
		return bindingCheckpointError("QueueBinding", "the same registered binding, adapters, economy, RNG and authority")
	}
	if err := sources.validate(b); err != nil {
		return err
	}
	if c.bindings == nil {
		c.bindings = &sources
	}
	return nil
}

// ValidateBinding inspects installed state only. A nil binding is the explicit
// absent fixture; a nonnil binding needs this context's exact registration.
// Ready and QueueBinding.Validate are gameplay and are never called here.
func (c *CheckpointContext) ValidateBinding(b *QueueBinding) error {
	if c == nil {
		return bindingCheckpointError("QueueBinding", "a checkpoint context")
	}
	if b == nil {
		return nil
	}
	if c.bindings == nil {
		return bindingCheckpointError("QueueBinding", "a registered binding; TODO(M3-U6) for unattested bindings")
	}
	return c.bindings.validate(b)
}

func (s *checkpointBindingSources) validate(b *QueueBinding) error {
	if b != s.binding {
		return bindingCheckpointError("QueueBinding", "the registered binding")
	}
	if b.Movement != s.movement || b.Presentation != s.presentation || b.Weapons != s.weapons || b.Work != s.work || b.World != s.world {
		return bindingCheckpointError("QueueBinding.adapters", "the registered adapter owners")
	}
	if s.economy == nil {
		if b.Economy != nil {
			return bindingCheckpointError("QueueBinding.Economy", "an absent economy")
		}
	} else if actual, ok := b.Economy.(*economy.Service); !ok || actual == nil || actual != s.economy {
		return bindingCheckpointError("QueueBinding.Economy", "the registered concrete economy service")
	}
	if b.SimRNG != s.sim {
		return bindingCheckpointError("QueueBinding.SimRNG", "the registered simulation RNG")
	}
	if _, err := CheckpointRulesKind(b.Rules); err != nil {
		return err
	}
	if err := b.validateCheckpointCallbacks(s.authority); err != nil {
		return err
	}
	if err := b.Movement.validateCheckpointCallbacks(s.authority); err != nil {
		return err
	}
	if err := b.Presentation.validateCheckpointCallbacks(s.authority); err != nil {
		return err
	}
	if err := b.Weapons.validateCheckpointCallbacks(s.authority); err != nil {
		return err
	}
	if err := b.Work.validateCheckpointCallbacks(s.authority); err != nil {
		return err
	}
	return b.World.validateCheckpointCallbacks(s.authority)
}

func bindingCheckpointError(path, expected string) error {
	return orderCheckpointError(path, errors.New(expected))
}

// WriteCheckpoint retains the original public fields in lexical order:
// BuildList, BuilderOptions, Community, CurrentTick, Damage, DangerCanRespond,
// DangerRouteFeasible, DangerStepFeasible, DangerVisible, Economy, Hostility,
// Lookup, ModernAIPlayer, Movement, Presentation, ReclaimFeature, Resources,
// Rules, SimRNG, TransportAdmission, Weapons, Work, World. Callbacks/services
// retain validated presence, adapters presence plus their lexical callback
// payload, Rules its closed u8 kind, Community its complete existing body.
// Private proof/context identities never become bytes (§16.3.59).
func (b *QueueBinding) WriteCheckpoint(e *checkpoint.Encoder, c *CheckpointContext) error {
	e.Field("QueueBinding")
	if err := c.ValidateBinding(b); err != nil {
		e.Fail(err)
		return e.Err()
	}
	e.Bool(b != nil)
	if b == nil {
		return e.Err()
	}
	e.Field("QueueBinding.BuildList")
	e.Bool(b.buildList != nil)
	e.Field("QueueBinding.BuilderOptions")
	e.Bool(b.builderOptions != nil)
	if err := b.Community.WriteCheckpoint(e); err != nil {
		return err
	}
	e.Field("QueueBinding.CurrentTick")
	e.Bool(b.currentTick != nil)
	e.Field("QueueBinding.Damage")
	e.Bool(b.damage != nil)
	e.Field("QueueBinding.DangerCanRespond")
	e.Bool(b.dangerCanRespond != nil)
	e.Field("QueueBinding.DangerRouteFeasible")
	e.Bool(b.dangerRouteFeasible != nil)
	e.Field("QueueBinding.DangerStepFeasible")
	e.Bool(b.dangerStepFeasible != nil)
	e.Field("QueueBinding.DangerVisible")
	e.Bool(b.dangerVisible != nil)
	e.Field("QueueBinding.Economy")
	e.Bool(b.Economy != nil)
	e.Field("QueueBinding.Hostility")
	e.Bool(b.hostility != nil)
	e.Field("QueueBinding.Lookup")
	e.Bool(b.lookup != nil)
	e.Field("QueueBinding.ModernAIPlayer")
	e.Bool(b.modernAIPlayer != nil)
	b.Movement.writeCheckpoint(e)
	b.Presentation.writeCheckpoint(e)
	e.Field("QueueBinding.ReclaimFeature")
	e.Bool(b.reclaimFeature != nil)
	e.Field("QueueBinding.Resources")
	e.Bool(b.resources != nil)
	e.Field("QueueBinding.Rules")
	kind, err := CheckpointRulesKind(b.Rules)
	if err != nil {
		e.Fail(err)
		return e.Err()
	}
	e.U8(kind)
	e.Field("QueueBinding.SimRNG")
	e.Bool(b.SimRNG != nil)
	e.Field("QueueBinding.TransportAdmission")
	e.Bool(b.transportAdmission != nil)
	b.Weapons.writeCheckpoint(e)
	b.Work.writeCheckpoint(e)
	b.World.writeCheckpoint(e)
	return e.Err()
}
