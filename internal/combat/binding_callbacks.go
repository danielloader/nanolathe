package combat

import (
	"github.com/nanolathe-gg/nanolathe/internal/community"
	"github.com/nanolathe-gg/nanolathe/internal/features"
	"github.com/nanolathe-gg/nanolathe/internal/pool"
	"github.com/nanolathe-gg/nanolathe/internal/sim/numeric"
	"github.com/nanolathe-gg/nanolathe/internal/units"
	"github.com/nanolathe-gg/nanolathe/internal/visibility"
	"github.com/nanolathe-gg/nanolathe/internal/world"
)

// ServiceConfig supplies copied installation inputs; see Service for each field's contract.
type ServiceConfig struct {
	Community        community.Features
	Rules            Rules
	InfectionThreat  func(*units.Unit) bool
	VisitOffMapFiled func(yield func(pool.Handle, uint64) bool)
	IsOffMapFiled    func(pool.Handle) bool
	TransportDeaths  TransportDeathState
	DangerNotice     func(victim, attacker *units.Unit, tick uint32)
	ImpactNotice     func(victim, attacker *units.Unit, bearing numeric.Angle, tick uint32)
	ProjectileWind   *world.Wind
	Slots            pool.Projectiles
	Records          []Projectile
	Events           func(Event)
	Visibility       func(viewer visibility.PlayerID, target visibility.Target) bool
	ControlByte      func(owner uint8) uint8
	Reaction         *ReactionSeams
	DamageActivity   func(victim, attacker *units.Unit, tick uint32)
	HealthLost       func(victim, attacker *units.Unit, lost int32)
	Features         *features.Service
	OpaqueLiquidMode bool
}

// NewService copies configuration without invoking any callback.
func NewService(c ServiceConfig) *Service {
	return &Service{
		Community:        c.Community,
		Rules:            c.Rules,
		infectionThreat:  c.InfectionThreat,
		visitOffMapFiled: c.VisitOffMapFiled,
		isOffMapFiled:    c.IsOffMapFiled,
		TransportDeaths:  c.TransportDeaths,
		dangerNotice:     c.DangerNotice,
		impactNotice:     c.ImpactNotice,
		ProjectileWind:   c.ProjectileWind,
		Slots:            c.Slots,
		Records:          c.Records,
		events:           c.Events,
		visibility:       c.Visibility,
		controlByte:      c.ControlByte,
		Reaction:         c.Reaction,
		damageActivity:   c.DamageActivity,
		healthLost:       c.HealthLost,
		Features:         c.Features,
		OpaqueLiquidMode: c.OpaqueLiquidMode,
	}
}

// InfectionThreatHook returns the installed callback without checkpoint proof.
func (b *Service) InfectionThreatHook() func(*units.Unit) bool { return b.infectionThreat }

// SetInfectionThreat replaces this callback with an ordinary installation.
func (b *Service) SetInfectionThreat(fn func(*units.Unit) bool) {
	b.checkpointCallbacks[checkpointServiceInfectionThreat] = checkpointServiceCallbackProof{}
	b.infectionThreat = fn
}

// VisitOffMapFiledHook returns the installed callback without checkpoint proof.
func (b *Service) VisitOffMapFiledHook() func(yield func(pool.Handle, uint64) bool) {
	return b.visitOffMapFiled
}

// SetVisitOffMapFiled replaces this callback with an ordinary installation.
func (b *Service) SetVisitOffMapFiled(fn func(yield func(pool.Handle, uint64) bool)) {
	b.checkpointCallbacks[checkpointServiceVisitOffMapFiled] = checkpointServiceCallbackProof{}
	b.visitOffMapFiled = fn
}

// IsOffMapFiledHook returns the installed callback without checkpoint proof.
func (b *Service) IsOffMapFiledHook() func(pool.Handle) bool { return b.isOffMapFiled }

// SetIsOffMapFiled replaces this callback with an ordinary installation.
func (b *Service) SetIsOffMapFiled(fn func(pool.Handle) bool) {
	b.checkpointCallbacks[checkpointServiceIsOffMapFiled] = checkpointServiceCallbackProof{}
	b.isOffMapFiled = fn
}

// DangerNoticeHook returns the installed callback without checkpoint proof.
func (b *Service) DangerNoticeHook() func(victim, attacker *units.Unit, tick uint32) {
	return b.dangerNotice
}

// SetDangerNotice replaces this callback with an ordinary installation.
func (b *Service) SetDangerNotice(fn func(victim, attacker *units.Unit, tick uint32)) {
	b.checkpointCallbacks[checkpointServiceDangerNotice] = checkpointServiceCallbackProof{}
	b.dangerNotice = fn
}

// ImpactNoticeHook returns the installed callback without checkpoint proof.
func (b *Service) ImpactNoticeHook() func(victim, attacker *units.Unit, bearing numeric.Angle, tick uint32) {
	return b.impactNotice
}

// SetImpactNotice replaces this callback with an ordinary installation.
func (b *Service) SetImpactNotice(fn func(victim, attacker *units.Unit, bearing numeric.Angle, tick uint32)) {
	b.checkpointCallbacks[checkpointServiceImpactNotice] = checkpointServiceCallbackProof{}
	b.impactNotice = fn
}

// EventsHook returns the installed callback without checkpoint proof.
func (b *Service) EventsHook() func(Event) { return b.events }

// SetEvents replaces this callback with an ordinary installation.
func (b *Service) SetEvents(fn func(Event)) {
	b.checkpointCallbacks[checkpointServiceEvents] = checkpointServiceCallbackProof{}
	b.events = fn
}

// VisibilityHook returns the installed callback without checkpoint proof.
func (b *Service) VisibilityHook() func(viewer visibility.PlayerID, target visibility.Target) bool {
	return b.visibility
}

// SetVisibility replaces this callback with an ordinary installation.
func (b *Service) SetVisibility(fn func(viewer visibility.PlayerID, target visibility.Target) bool) {
	b.checkpointCallbacks[checkpointServiceVisibility] = checkpointServiceCallbackProof{}
	b.visibility = fn
}

// ControlByteHook returns the installed callback without checkpoint proof.
func (b *Service) ControlByteHook() func(owner uint8) uint8 { return b.controlByte }

// SetControlByte replaces this callback with an ordinary installation.
func (b *Service) SetControlByte(fn func(owner uint8) uint8) {
	b.checkpointCallbacks[checkpointServiceControlByte] = checkpointServiceCallbackProof{}
	b.controlByte = fn
}

// DamageActivityHook returns the installed callback without checkpoint proof.
func (b *Service) DamageActivityHook() func(victim, attacker *units.Unit, tick uint32) {
	return b.damageActivity
}

// SetDamageActivity replaces this callback with an ordinary installation.
func (b *Service) SetDamageActivity(fn func(victim, attacker *units.Unit, tick uint32)) {
	b.checkpointCallbacks[checkpointServiceDamageActivity] = checkpointServiceCallbackProof{}
	b.damageActivity = fn
}

// HealthLostHook returns the installed callback without checkpoint proof.
func (b *Service) HealthLostHook() func(victim, attacker *units.Unit, lost int32) {
	return b.healthLost
}

// SetHealthLost replaces this callback with an ordinary installation.
func (b *Service) SetHealthLost(fn func(victim, attacker *units.Unit, lost int32)) {
	b.checkpointCallbacks[checkpointServiceHealthLost] = checkpointServiceCallbackProof{}
	b.healthLost = fn
}

// ReactionSeamsConfig supplies copied installation inputs; see ReactionSeams for each field's contract.
type ReactionSeamsConfig struct {
	ObserverNotice          func(victim *units.Unit)
	Allied                  func(a, b uint8) bool
	ArmConstructionThrottle func(owner uint8, tick uint32)
	PurgeOrdersOnDamage     func(victim *units.Unit)
	RetaliationOrder        func(victim, attacker *units.Unit) bool
	SlotAcquisitionAdmits   func(victim *units.Unit, slotIdx int, candidate *units.Unit) bool
	UnderAttackSilenced     func(victim *units.Unit) bool
	UnderAttackNotice       func(victim *units.Unit)
}

// NewReactionSeams copies configuration without invoking any callback.
func NewReactionSeams(c ReactionSeamsConfig) *ReactionSeams {
	return &ReactionSeams{
		observerNotice:          c.ObserverNotice,
		allied:                  c.Allied,
		armConstructionThrottle: c.ArmConstructionThrottle,
		purgeOrdersOnDamage:     c.PurgeOrdersOnDamage,
		retaliationOrder:        c.RetaliationOrder,
		slotAcquisitionAdmits:   c.SlotAcquisitionAdmits,
		underAttackSilenced:     c.UnderAttackSilenced,
		underAttackNotice:       c.UnderAttackNotice,
	}
}

// ObserverNoticeHook returns the installed callback without checkpoint proof.
func (b *ReactionSeams) ObserverNoticeHook() func(victim *units.Unit) { return b.observerNotice }

// SetObserverNotice replaces this callback with an ordinary installation.
func (b *ReactionSeams) SetObserverNotice(fn func(victim *units.Unit)) {
	b.checkpointCallbacks[checkpointReactionObserverNotice] = checkpointReactionCallbackProof{}
	b.observerNotice = fn
}

// AlliedHook returns the installed callback without checkpoint proof.
func (b *ReactionSeams) AlliedHook() func(a, b uint8) bool { return b.allied }

// SetAllied replaces this callback with an ordinary installation.
func (b *ReactionSeams) SetAllied(fn func(a, b uint8) bool) {
	b.checkpointCallbacks[checkpointReactionAllied] = checkpointReactionCallbackProof{}
	b.allied = fn
}

// ArmConstructionThrottleHook returns the installed callback without checkpoint proof.
func (b *ReactionSeams) ArmConstructionThrottleHook() func(owner uint8, tick uint32) {
	return b.armConstructionThrottle
}

// SetArmConstructionThrottle replaces this callback with an ordinary installation.
func (b *ReactionSeams) SetArmConstructionThrottle(fn func(owner uint8, tick uint32)) {
	b.checkpointCallbacks[checkpointReactionArmConstructionThrottle] = checkpointReactionCallbackProof{}
	b.armConstructionThrottle = fn
}

// PurgeOrdersOnDamageHook returns the installed callback without checkpoint proof.
func (b *ReactionSeams) PurgeOrdersOnDamageHook() func(victim *units.Unit) {
	return b.purgeOrdersOnDamage
}

// SetPurgeOrdersOnDamage replaces this callback with an ordinary installation.
func (b *ReactionSeams) SetPurgeOrdersOnDamage(fn func(victim *units.Unit)) {
	b.checkpointCallbacks[checkpointReactionPurgeOrdersOnDamage] = checkpointReactionCallbackProof{}
	b.purgeOrdersOnDamage = fn
}

// RetaliationOrderHook returns the installed callback without checkpoint proof.
func (b *ReactionSeams) RetaliationOrderHook() func(victim, attacker *units.Unit) bool {
	return b.retaliationOrder
}

// SetRetaliationOrder replaces this callback with an ordinary installation.
func (b *ReactionSeams) SetRetaliationOrder(fn func(victim, attacker *units.Unit) bool) {
	b.checkpointCallbacks[checkpointReactionRetaliationOrder] = checkpointReactionCallbackProof{}
	b.retaliationOrder = fn
}

// SlotAcquisitionAdmitsHook returns the installed callback without checkpoint proof.
func (b *ReactionSeams) SlotAcquisitionAdmitsHook() func(victim *units.Unit, slotIdx int, candidate *units.Unit) bool {
	return b.slotAcquisitionAdmits
}

// SetSlotAcquisitionAdmits replaces this callback with an ordinary installation.
func (b *ReactionSeams) SetSlotAcquisitionAdmits(fn func(victim *units.Unit, slotIdx int, candidate *units.Unit) bool) {
	b.checkpointCallbacks[checkpointReactionSlotAcquisitionAdmits] = checkpointReactionCallbackProof{}
	b.slotAcquisitionAdmits = fn
}

// UnderAttackSilencedHook returns the installed callback without checkpoint proof.
func (b *ReactionSeams) UnderAttackSilencedHook() func(victim *units.Unit) bool {
	return b.underAttackSilenced
}

// SetUnderAttackSilenced replaces this callback with an ordinary installation.
func (b *ReactionSeams) SetUnderAttackSilenced(fn func(victim *units.Unit) bool) {
	b.checkpointCallbacks[checkpointReactionUnderAttackSilenced] = checkpointReactionCallbackProof{}
	b.underAttackSilenced = fn
}

// UnderAttackNoticeHook returns the installed callback without checkpoint proof.
func (b *ReactionSeams) UnderAttackNoticeHook() func(victim *units.Unit) { return b.underAttackNotice }

// SetUnderAttackNotice replaces this callback with an ordinary installation.
func (b *ReactionSeams) SetUnderAttackNotice(fn func(victim *units.Unit)) {
	b.checkpointCallbacks[checkpointReactionUnderAttackNotice] = checkpointReactionCallbackProof{}
	b.underAttackNotice = fn
}
