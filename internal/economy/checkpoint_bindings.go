package economy

import (
	"github.com/nanolathe-gg/nanolathe/internal/sim/checkpoint"
	"github.com/nanolathe-gg/nanolathe/internal/units"
	"github.com/nanolathe-gg/nanolathe/internal/world"
)

const (
	checkpointCloakCost = iota
	checkpointCloakDue
	checkpointEndCondition
)

// A proof belongs to one private callback installation on one exact Service.
// Extraction and copying cannot transfer it (DESIGN_MULTIPLAYER §16.3.48).
type checkpointCallbackProof struct {
	owner     *Service
	authority *checkpoint.BindingAuthority
}

// SetCloakCostWithCheckpointBinding installs a reviewed canonical callback.
// The session keeps its authority private; capture still checks exact ownership.
func (s *Service) SetCloakCostWithCheckpointBinding(fn func(*units.Unit) float32, authority *checkpoint.BindingAuthority) {
	s.SetCloakCost(fn)
	if fn != nil && authority != nil {
		s.checkpointCallbacks[checkpointCloakCost] = checkpointCallbackProof{owner: s, authority: authority}
	}
}

// SetCloakDueWithCheckpointBinding installs a reviewed canonical callback.
func (s *Service) SetCloakDueWithCheckpointBinding(fn func(*units.Unit) bool, authority *checkpoint.BindingAuthority) {
	s.SetCloakDue(fn)
	if fn != nil && authority != nil {
		s.checkpointCallbacks[checkpointCloakDue] = checkpointCallbackProof{owner: s, authority: authority}
	}
}

// SetEndConditionWithCheckpointBinding installs a reviewed canonical callback.
func (s *Service) SetEndConditionWithCheckpointBinding(fn func(int, uint32), authority *checkpoint.BindingAuthority) {
	s.SetEndCondition(fn)
	if fn != nil && authority != nil {
		s.checkpointCallbacks[checkpointEndCondition] = checkpointCallbackProof{owner: s, authority: authority}
	}
}

// SetBindings records the exact service, wind and authority for this capture.
// Conflicts leave the original registration intact (DESIGN_MULTIPLAYER §16.3.48).
func (c *CheckpointContext) SetBindings(s *Service, wind *world.Wind, authority *checkpoint.BindingAuthority) error {
	if c == nil || s == nil || authority == nil {
		return economyCheckpointError("economy.bindings", "a context, exact service and binding authority")
	}
	if c.bindingService != nil && (c.bindingService != s || c.bindingWind != wind || !c.bindingAuthority.Matches(authority)) {
		return economyCheckpointError("economy.bindings", "one consistent service, wind and authority")
	}
	c.bindingService, c.bindingWind, c.bindingAuthority = s, wind, authority
	return nil
}

func (s *Service) validateCheckpointBindings(c *CheckpointContext) error {
	if c.bindingService != nil && c.bindingService != s {
		return economyCheckpointError("economy.bindings", "the registered service")
	}
	present := [3]bool{s.cloakCost != nil, s.cloakDue != nil, s.endCondition != nil}
	names := [3]string{"CloakCost", "CloakDue", "EndCondition"}
	for slot, set := range present {
		proof := s.checkpointCallbacks[slot]
		if set && (c.bindingService != s || proof.owner != s || !proof.authority.Matches(c.bindingAuthority)) {
			return economyCheckpointError("economy.Service."+names[slot], "the exact service's attested callback binding")
		}
	}
	if s.Wind != c.bindingWind {
		return economyCheckpointError("economy.Service.Wind", "the registered wind identity")
	}
	return nil
}
