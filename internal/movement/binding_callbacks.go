package movement

import "github.com/nanolathe-gg/nanolathe/internal/combat"

// Callback installation stays explicit so replacement can invalidate diagnostic
// provenance without invoking the function (DESIGN_MULTIPLAYER §16.3.69).
func (s *System) SetDamage(fn func(uint32, combat.DamageInput) combat.DamageResult) {
	s.damage = fn
	s.checkpointDamage = checkpointCompositionProof{}
}
func (s *System) DamageHook() func(uint32, combat.DamageInput) combat.DamageResult {
	return s.damage
}
func (s *System) SetProductFootprint(fn func(uint32) (int32, int32, bool)) {
	s.productFootprint = fn
	s.checkpointProductFootprint = checkpointCompositionProof{}
}
func (s *System) ProductFootprintHook() func(uint32) (int32, int32, bool) {
	return s.productFootprint
}
