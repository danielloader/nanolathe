package visibility

import (
	"github.com/nanolathe-gg/nanolathe/internal/sim/checkpoint"
	"github.com/nanolathe-gg/nanolathe/internal/world"
)

// Each proof belongs to one copied reader installation on one exact Service.
// Community scalar projection and callback getters cannot manufacture proof;
// session admission separately verifies rule scalars and callback provenance
// (DESIGN_MULTIPLAYER §16.3.49). These pointers are never checkpoint bytes.
type checkpointReaderProof struct {
	owner     *Service
	authority *checkpoint.BindingAuthority
}

// SetAlliedWithCheckpointBinding performs the ordinary install once, then
// stamps only a present reader with a present authority. An invalid proof input
// never prevents the requested installation or calls the reader (§16.3.49).
func (s *Service) SetAlliedWithCheckpointBinding(fn func(PlayerID, PlayerID) bool, authority *checkpoint.BindingAuthority) {
	s.Community.SetAllied(fn)
	if fn != nil && authority != nil {
		s.Community.checkpointAllied = checkpointReaderProof{owner: s, authority: authority}
	}
}

// SetOffMapWithCheckpointBinding preserves the ordinary reader installation
// and independently stamps this slot (DESIGN_MULTIPLAYER §16.3.49).
func (s *Service) SetOffMapWithCheckpointBinding(fn func(uint16) bool, authority *checkpoint.BindingAuthority) {
	s.Community.SetOffMap(fn)
	if fn != nil && authority != nil {
		s.Community.checkpointOffMap = checkpointReaderProof{owner: s, authority: authority}
	}
}

// SetBindings records capture-local expectations without stamping live state.
// Nil terrain is a valid expected singleton; service and authority are required.
// Parent admission verifies the whole session graph (DESIGN_MULTIPLAYER §16.3.49).
func (c *CheckpointContext) SetBindings(s *Service, terrain *world.Terrain, authority *checkpoint.BindingAuthority) error {
	if c == nil || s == nil || authority == nil {
		return visibilityCheckpointError("visibility.bindings", "a context, exact service and binding authority")
	}
	if c.bindingService != nil && (c.bindingService != s || c.bindingTerrain != terrain || !c.bindingAuthority.Matches(authority)) {
		return visibilityCheckpointError("visibility.bindings", "one consistent service, terrain and authority")
	}
	c.bindingService, c.bindingTerrain, c.bindingAuthority = s, terrain, authority
	return nil
}

func (s *Service) validateCheckpointOwnerBindings(c *CheckpointContext) error {
	if c.bindingService != nil {
		if c.bindingService != s {
			return visibilityCheckpointError("visibility.Service", "the registered service")
		}
		if c.bindingTerrain != s.terrain {
			return visibilityCheckpointError("visibility.Service.terrain", "the registered terrain")
		}
	}
	// Disabled readers still need proof: later rule projections can consume
	// them. Presence tests inspect no callback behavior (§16.3.49).
	if s.Community.allied != nil && !s.Community.checkpointAllied.matches(s, c) {
		return visibilityCheckpointError("visibility.Service.Community.Allied", "the exact service's attested alliance reader")
	}
	if s.Community.offMap != nil && !s.Community.checkpointOffMap.matches(s, c) {
		return visibilityCheckpointError("visibility.Service.Community.OffMap", "the exact service's attested off-map reader")
	}
	return nil
}

func (p checkpointReaderProof) matches(s *Service, c *CheckpointContext) bool {
	return c.bindingService == s && p.owner == s && p.authority.Matches(c.bindingAuthority)
}
