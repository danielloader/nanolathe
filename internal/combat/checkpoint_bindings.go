package combat

import (
	"github.com/nanolathe-gg/nanolathe/internal/content"
	"github.com/nanolathe-gg/nanolathe/internal/features"
	"github.com/nanolathe-gg/nanolathe/internal/pool"
	"github.com/nanolathe-gg/nanolathe/internal/sim/checkpoint"
	"github.com/nanolathe-gg/nanolathe/internal/sim/numeric"
	"github.com/nanolathe-gg/nanolathe/internal/units"
	"github.com/nanolathe-gg/nanolathe/internal/visibility"
	"github.com/nanolathe-gg/nanolathe/internal/world"
)

const (
	checkpointServiceControlByte = iota
	checkpointServiceDamageActivity
	checkpointServiceDangerNotice
	checkpointServiceEvents
	checkpointServiceHealthLost
	checkpointServiceImpactNotice
	checkpointServiceInfectionThreat
	checkpointServiceIsOffMapFiled
	checkpointServiceVisibility
	checkpointServiceVisitOffMapFiled
	checkpointServiceCallbackCount
)

// Each proof belongs to one private slot on one exact owner; extracting a
// callback or copying its owner cannot transfer it (DESIGN_MULTIPLAYER §16.3.70).
type checkpointServiceCallbackProof struct {
	owner     *Service
	authority *checkpoint.BindingAuthority
}

// SetControlByteWithCheckpointBinding performs the ordinary installation, then
// retains the reviewed session's exact owner and authority for this slot.
func (b *Service) SetControlByteWithCheckpointBinding(fn func(owner uint8) uint8, authority *checkpoint.BindingAuthority) {
	b.SetControlByte(fn)
	if fn != nil && authority != nil {
		b.checkpointCallbacks[checkpointServiceControlByte] = checkpointServiceCallbackProof{b, authority}
	}
}

// SetDamageActivityWithCheckpointBinding performs the ordinary installation, then
// retains the reviewed session's exact owner and authority for this slot.
func (b *Service) SetDamageActivityWithCheckpointBinding(fn func(victim, attacker *units.Unit, tick uint32), authority *checkpoint.BindingAuthority) {
	b.SetDamageActivity(fn)
	if fn != nil && authority != nil {
		b.checkpointCallbacks[checkpointServiceDamageActivity] = checkpointServiceCallbackProof{b, authority}
	}
}

// SetDangerNoticeWithCheckpointBinding performs the ordinary installation, then
// retains the reviewed session's exact owner and authority for this slot.
func (b *Service) SetDangerNoticeWithCheckpointBinding(fn func(victim, attacker *units.Unit, tick uint32), authority *checkpoint.BindingAuthority) {
	b.SetDangerNotice(fn)
	if fn != nil && authority != nil {
		b.checkpointCallbacks[checkpointServiceDangerNotice] = checkpointServiceCallbackProof{b, authority}
	}
}

// SetEventsWithCheckpointBinding performs the ordinary installation, then
// retains the reviewed session's exact owner and authority for this slot.
func (b *Service) SetEventsWithCheckpointBinding(fn func(Event), authority *checkpoint.BindingAuthority) {
	b.SetEvents(fn)
	if fn != nil && authority != nil {
		b.checkpointCallbacks[checkpointServiceEvents] = checkpointServiceCallbackProof{b, authority}
	}
}

// SetHealthLostWithCheckpointBinding performs the ordinary installation, then
// retains the reviewed session's exact owner and authority for this slot.
func (b *Service) SetHealthLostWithCheckpointBinding(fn func(victim, attacker *units.Unit, lost int32), authority *checkpoint.BindingAuthority) {
	b.SetHealthLost(fn)
	if fn != nil && authority != nil {
		b.checkpointCallbacks[checkpointServiceHealthLost] = checkpointServiceCallbackProof{b, authority}
	}
}

// SetImpactNoticeWithCheckpointBinding performs the ordinary installation, then
// retains the reviewed session's exact owner and authority for this slot.
func (b *Service) SetImpactNoticeWithCheckpointBinding(fn func(victim, attacker *units.Unit, bearing numeric.Angle, tick uint32), authority *checkpoint.BindingAuthority) {
	b.SetImpactNotice(fn)
	if fn != nil && authority != nil {
		b.checkpointCallbacks[checkpointServiceImpactNotice] = checkpointServiceCallbackProof{b, authority}
	}
}

// SetInfectionThreatWithCheckpointBinding performs the ordinary installation, then
// retains the reviewed session's exact owner and authority for this slot.
func (b *Service) SetInfectionThreatWithCheckpointBinding(fn func(*units.Unit) bool, authority *checkpoint.BindingAuthority) {
	b.SetInfectionThreat(fn)
	if fn != nil && authority != nil {
		b.checkpointCallbacks[checkpointServiceInfectionThreat] = checkpointServiceCallbackProof{b, authority}
	}
}

// SetIsOffMapFiledWithCheckpointBinding performs the ordinary installation, then
// retains the reviewed session's exact owner and authority for this slot.
func (b *Service) SetIsOffMapFiledWithCheckpointBinding(fn func(pool.Handle) bool, authority *checkpoint.BindingAuthority) {
	b.SetIsOffMapFiled(fn)
	if fn != nil && authority != nil {
		b.checkpointCallbacks[checkpointServiceIsOffMapFiled] = checkpointServiceCallbackProof{b, authority}
	}
}

// SetVisibilityWithCheckpointBinding performs the ordinary installation, then
// retains the reviewed session's exact owner and authority for this slot.
func (b *Service) SetVisibilityWithCheckpointBinding(fn func(viewer visibility.PlayerID, target visibility.Target) bool, authority *checkpoint.BindingAuthority) {
	b.SetVisibility(fn)
	if fn != nil && authority != nil {
		b.checkpointCallbacks[checkpointServiceVisibility] = checkpointServiceCallbackProof{b, authority}
	}
}

// SetVisitOffMapFiledWithCheckpointBinding performs the ordinary installation, then
// retains the reviewed session's exact owner and authority for this slot.
func (b *Service) SetVisitOffMapFiledWithCheckpointBinding(fn func(yield func(pool.Handle, uint64) bool), authority *checkpoint.BindingAuthority) {
	b.SetVisitOffMapFiled(fn)
	if fn != nil && authority != nil {
		b.checkpointCallbacks[checkpointServiceVisitOffMapFiled] = checkpointServiceCallbackProof{b, authority}
	}
}

const (
	checkpointReactionAllied = iota
	checkpointReactionArmConstructionThrottle
	checkpointReactionObserverNotice
	checkpointReactionPurgeOrdersOnDamage
	checkpointReactionRetaliationOrder
	checkpointReactionSlotAcquisitionAdmits
	checkpointReactionUnderAttackNotice
	checkpointReactionUnderAttackSilenced
	checkpointReactionCallbackCount
)

// Each proof belongs to one private slot on one exact owner; extracting a
// callback or copying its owner cannot transfer it (DESIGN_MULTIPLAYER §16.3.70).
type checkpointReactionCallbackProof struct {
	owner     *ReactionSeams
	authority *checkpoint.BindingAuthority
}

// SetAlliedWithCheckpointBinding performs the ordinary installation, then
// retains the reviewed session's exact owner and authority for this slot.
func (b *ReactionSeams) SetAlliedWithCheckpointBinding(fn func(a, b uint8) bool, authority *checkpoint.BindingAuthority) {
	b.SetAllied(fn)
	if fn != nil && authority != nil {
		b.checkpointCallbacks[checkpointReactionAllied] = checkpointReactionCallbackProof{b, authority}
	}
}

// SetArmConstructionThrottleWithCheckpointBinding performs the ordinary installation, then
// retains the reviewed session's exact owner and authority for this slot.
func (b *ReactionSeams) SetArmConstructionThrottleWithCheckpointBinding(fn func(owner uint8, tick uint32), authority *checkpoint.BindingAuthority) {
	b.SetArmConstructionThrottle(fn)
	if fn != nil && authority != nil {
		b.checkpointCallbacks[checkpointReactionArmConstructionThrottle] = checkpointReactionCallbackProof{b, authority}
	}
}

// SetObserverNoticeWithCheckpointBinding performs the ordinary installation, then
// retains the reviewed session's exact owner and authority for this slot.
func (b *ReactionSeams) SetObserverNoticeWithCheckpointBinding(fn func(victim *units.Unit), authority *checkpoint.BindingAuthority) {
	b.SetObserverNotice(fn)
	if fn != nil && authority != nil {
		b.checkpointCallbacks[checkpointReactionObserverNotice] = checkpointReactionCallbackProof{b, authority}
	}
}

// SetPurgeOrdersOnDamageWithCheckpointBinding performs the ordinary installation, then
// retains the reviewed session's exact owner and authority for this slot.
func (b *ReactionSeams) SetPurgeOrdersOnDamageWithCheckpointBinding(fn func(victim *units.Unit), authority *checkpoint.BindingAuthority) {
	b.SetPurgeOrdersOnDamage(fn)
	if fn != nil && authority != nil {
		b.checkpointCallbacks[checkpointReactionPurgeOrdersOnDamage] = checkpointReactionCallbackProof{b, authority}
	}
}

// SetRetaliationOrderWithCheckpointBinding performs the ordinary installation, then
// retains the reviewed session's exact owner and authority for this slot.
func (b *ReactionSeams) SetRetaliationOrderWithCheckpointBinding(fn func(victim, attacker *units.Unit) bool, authority *checkpoint.BindingAuthority) {
	b.SetRetaliationOrder(fn)
	if fn != nil && authority != nil {
		b.checkpointCallbacks[checkpointReactionRetaliationOrder] = checkpointReactionCallbackProof{b, authority}
	}
}

// SetSlotAcquisitionAdmitsWithCheckpointBinding performs the ordinary installation, then
// retains the reviewed session's exact owner and authority for this slot.
func (b *ReactionSeams) SetSlotAcquisitionAdmitsWithCheckpointBinding(fn func(victim *units.Unit, slotIdx int, candidate *units.Unit) bool, authority *checkpoint.BindingAuthority) {
	b.SetSlotAcquisitionAdmits(fn)
	if fn != nil && authority != nil {
		b.checkpointCallbacks[checkpointReactionSlotAcquisitionAdmits] = checkpointReactionCallbackProof{b, authority}
	}
}

// SetUnderAttackNoticeWithCheckpointBinding performs the ordinary installation, then
// retains the reviewed session's exact owner and authority for this slot.
func (b *ReactionSeams) SetUnderAttackNoticeWithCheckpointBinding(fn func(victim *units.Unit), authority *checkpoint.BindingAuthority) {
	b.SetUnderAttackNotice(fn)
	if fn != nil && authority != nil {
		b.checkpointCallbacks[checkpointReactionUnderAttackNotice] = checkpointReactionCallbackProof{b, authority}
	}
}

// SetUnderAttackSilencedWithCheckpointBinding performs the ordinary installation, then
// retains the reviewed session's exact owner and authority for this slot.
func (b *ReactionSeams) SetUnderAttackSilencedWithCheckpointBinding(fn func(victim *units.Unit) bool, authority *checkpoint.BindingAuthority) {
	b.SetUnderAttackSilenced(fn)
	if fn != nil && authority != nil {
		b.checkpointCallbacks[checkpointReactionUnderAttackSilenced] = checkpointReactionCallbackProof{b, authority}
	}
}

// NewReactionSeamsWithCheckpointBinding copies the same inputs as the ordinary
// constructor without invoking a callback (DESIGN_MULTIPLAYER §16.3.70).
func NewReactionSeamsWithCheckpointBinding(config ReactionSeamsConfig, authority *checkpoint.BindingAuthority) *ReactionSeams {
	b := &ReactionSeams{}
	b.SetObserverNoticeWithCheckpointBinding(config.ObserverNotice, authority)
	b.SetAlliedWithCheckpointBinding(config.Allied, authority)
	b.SetArmConstructionThrottleWithCheckpointBinding(config.ArmConstructionThrottle, authority)
	b.SetPurgeOrdersOnDamageWithCheckpointBinding(config.PurgeOrdersOnDamage, authority)
	b.SetRetaliationOrderWithCheckpointBinding(config.RetaliationOrder, authority)
	b.SetSlotAcquisitionAdmitsWithCheckpointBinding(config.SlotAcquisitionAdmits, authority)
	b.SetUnderAttackSilencedWithCheckpointBinding(config.UnderAttackSilenced, authority)
	b.SetUnderAttackNoticeWithCheckpointBinding(config.UnderAttackNotice, authority)
	return b
}

type checkpointBindings struct {
	service   *Service
	inputs    *content.SimulationInputs
	features  *features.Service
	wind      *world.Wind
	reaction  *ReactionSeams
	authority *checkpoint.BindingAuthority
}

// SetBindings validates the current aliases and independent slot proofs before
// committing one capture-local tuple. Root validates the frozen input contents
// once for the complete capture; this owner never calls its lookup cache
// (DESIGN_MULTIPLAYER §16.3.70).
func (c *CheckpointContext) SetBindings(s *Service, inputs *content.SimulationInputs, features *features.Service, wind *world.Wind, reaction *ReactionSeams, authority *checkpoint.BindingAuthority) error {
	if c == nil || s == nil || inputs == nil || authority == nil {
		return combatCheckpointError("combat.bindings", "a context, exact service, frozen inputs and binding authority")
	}
	binding := checkpointBindings{s, inputs, features, wind, reaction, authority}
	if c.binding.service != nil && c.binding != binding {
		return combatCheckpointError("combat.bindings", "one consistent service, inputs, features, wind, reaction and authority")
	}
	candidate := *c
	candidate.binding = binding
	if err := s.validateCheckpointBindings(&candidate); err != nil {
		return err
	}
	c.binding = binding
	return nil
}

func (s *Service) validateCheckpointBindings(c *CheckpointContext) error {
	b := c.binding
	if b.service != nil && b.service != s {
		return combatCheckpointError("combat.bindings", "the registered service")
	}
	present := [...]bool{s.controlByte != nil, s.damageActivity != nil, s.dangerNotice != nil, s.events != nil, s.healthLost != nil, s.impactNotice != nil, s.infectionThreat != nil, s.isOffMapFiled != nil, s.visibility != nil, s.visitOffMapFiled != nil}
	names := [...]string{"ControlByte", "DamageActivity", "DangerNotice", "Events", "HealthLost", "ImpactNotice", "InfectionThreat", "IsOffMapFiled", "Visibility", "VisitOffMapFiled"}
	for slot, set := range present {
		proof := s.checkpointCallbacks[slot]
		if set && (b.service != s || proof.owner != s || !proof.authority.Matches(b.authority)) {
			return combatCheckpointError("combat.Service."+names[slot], "the exact owner's attested callback binding")
		}
	}
	if s.Features != b.features {
		return combatCheckpointError("combat.Service.Features", "the registered feature service identity")
	}
	if s.ProjectileWind != b.wind {
		return combatCheckpointError("combat.Service.ProjectileWind", "the registered wind identity")
	}
	if s.Reaction != b.reaction {
		return combatCheckpointError("combat.Service.Reaction", "the registered reaction identity")
	}
	if s.Reaction != nil {
		return s.Reaction.validateCheckpointBindings(b.authority)
	}
	return nil
}

func (r *ReactionSeams) validateCheckpointBindings(authority *checkpoint.BindingAuthority) error {
	present := [...]bool{r.allied != nil, r.armConstructionThrottle != nil, r.observerNotice != nil, r.purgeOrdersOnDamage != nil, r.retaliationOrder != nil, r.slotAcquisitionAdmits != nil, r.underAttackNotice != nil, r.underAttackSilenced != nil}
	names := [...]string{"Allied", "ArmConstructionThrottle", "ObserverNotice", "PurgeOrdersOnDamage", "RetaliationOrder", "SlotAcquisitionAdmits", "UnderAttackNotice", "UnderAttackSilenced"}
	for slot, set := range present {
		proof := r.checkpointCallbacks[slot]
		if set && (proof.owner != r || !proof.authority.Matches(authority)) {
			return combatCheckpointError("combat.Service.Reaction."+names[slot], "the exact owner's attested callback binding")
		}
	}
	return nil
}

// Reaction payload preserves every optional member in logical lexical order;
// nil retains the original single absent byte (DESIGN_MULTIPLAYER §16.3.70).
func (r *ReactionSeams) writeCheckpoint(e *checkpoint.Encoder) {
	e.Field("combat.Service.Reaction")
	e.Bool(r != nil)
	if r == nil {
		return
	}
	e.Field("combat.Service.Reaction.Allied")
	e.Bool(r.allied != nil)
	e.Field("combat.Service.Reaction.ArmConstructionThrottle")
	e.Bool(r.armConstructionThrottle != nil)
	e.Field("combat.Service.Reaction.ObserverNotice")
	e.Bool(r.observerNotice != nil)
	e.Field("combat.Service.Reaction.PurgeOrdersOnDamage")
	e.Bool(r.purgeOrdersOnDamage != nil)
	e.Field("combat.Service.Reaction.RetaliationOrder")
	e.Bool(r.retaliationOrder != nil)
	e.Field("combat.Service.Reaction.SlotAcquisitionAdmits")
	e.Bool(r.slotAcquisitionAdmits != nil)
	e.Field("combat.Service.Reaction.UnderAttackNotice")
	e.Bool(r.underAttackNotice != nil)
	e.Field("combat.Service.Reaction.UnderAttackSilenced")
	e.Bool(r.underAttackSilenced != nil)
}
