package economy

import "github.com/nanolathe-gg/nanolathe/internal/units"

// These accessors retain the existing economy seams while keeping each installed
// callback privately owned (DESIGN_MULTIPLAYER §16.3.48).
func (s *Service) CloakCostHook() func(*units.Unit) float32 { return s.cloakCost }
func (s *Service) CloakDueHook() func(*units.Unit) bool     { return s.cloakDue }
func (s *Service) EndConditionHook() func(int, uint32)      { return s.endCondition }

// Ordinary installation clears only the replaced slot's checkpoint proof.
func (s *Service) SetCloakCost(fn func(*units.Unit) float32) {
	s.cloakCost = fn
	s.checkpointCallbacks[checkpointCloakCost] = checkpointCallbackProof{}
}

func (s *Service) SetCloakDue(fn func(*units.Unit) bool) {
	s.cloakDue = fn
	s.checkpointCallbacks[checkpointCloakDue] = checkpointCallbackProof{}
}

func (s *Service) SetEndCondition(fn func(int, uint32)) {
	s.endCondition = fn
	s.checkpointCallbacks[checkpointEndCondition] = checkpointCallbackProof{}
}
