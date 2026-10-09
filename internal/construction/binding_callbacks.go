package construction

import (
	"github.com/nanolathe-gg/nanolathe/internal/model"
	"github.com/nanolathe-gg/nanolathe/internal/units"
)

// These ordinary callback boundaries preserve the existing invocation
// contracts and expose no checkpoint provenance (DESIGN_MULTIPLAYER §16.3.67).
func (s *Service) SetCRTRandom(fn func(uint32) uint32) {
	s.checkpointCallbacks[checkpointCRTRandom] = checkpointCallbackProof{}
	s.crtRandom = fn
}
func (s *Service) CRTRandomHook() func(uint32) uint32 { return s.crtRandom }
func (s *Service) SetIsSpecialSecondState(fn func(uint8) bool) {
	s.checkpointCallbacks[checkpointSpecialSecondState] = checkpointCallbackProof{}
	s.isSpecialSecondState = fn
}
func (s *Service) IsSpecialSecondStateHook() func(uint8) bool { return s.isSpecialSecondState }
func (s *Service) SetModelForFactory(fn func(*units.Unit) *model.Model) {
	s.checkpointCallbacks[checkpointModelForFactory] = checkpointCallbackProof{}
	s.modelForFactory = fn
}
func (s *Service) ModelForFactoryHook() func(*units.Unit) *model.Model { return s.modelForFactory }
func (s *Service) SetModelForUnit(fn func(*units.Unit) *model.Model) {
	s.checkpointCallbacks[checkpointModelForUnit] = checkpointCallbackProof{}
	s.modelForUnit = fn
}
func (s *Service) ModelForUnitHook() func(*units.Unit) *model.Model { return s.modelForUnit }
