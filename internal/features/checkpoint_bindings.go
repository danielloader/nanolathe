package features

import (
	"github.com/nanolathe-gg/nanolathe/internal/content"
	"github.com/nanolathe-gg/nanolathe/internal/sim/checkpoint"
	"github.com/nanolathe-gg/nanolathe/internal/sim/numeric"
	"github.com/nanolathe-gg/nanolathe/internal/sim/rng"
	"github.com/nanolathe-gg/nanolathe/internal/world"
)

const (
	checkpointBurnFrameGeometry = iota
	checkpointBurnSmoke
	checkpointBurnSound
	checkpointBurnWeapon
	checkpointGeothermalSteam
	checkpointSequenceFrames
)

// Each proof belongs to one private callback installation on one exact Service.
// Extraction and copying cannot transfer it (DESIGN_MULTIPLAYER §16.3.50).
type checkpointCallbackProof struct {
	owner     *Service
	authority *checkpoint.BindingAuthority
}

// SetBurnFrameGeometryWithCheckpointBinding installs a reviewed canonical callback.
// The session keeps its authority private; capture still checks exact ownership.
func (s *Service) SetBurnFrameGeometryWithCheckpointBinding(fn func(*content.FeatureDef, int32) (int32, int32, int32, int32), authority *checkpoint.BindingAuthority) {
	s.SetBurnFrameGeometry(fn)
	if fn != nil && authority != nil {
		s.checkpointCallbacks[checkpointBurnFrameGeometry] = checkpointCallbackProof{owner: s, authority: authority}
	}
}

// SetBurnSmokeWithCheckpointBinding installs a reviewed canonical callback.
func (s *Service) SetBurnSmokeWithCheckpointBinding(fn func([3]numeric.Fixed), authority *checkpoint.BindingAuthority) {
	s.SetBurnSmoke(fn)
	if fn != nil && authority != nil {
		s.checkpointCallbacks[checkpointBurnSmoke] = checkpointCallbackProof{owner: s, authority: authority}
	}
}

// SetBurnSoundWithCheckpointBinding installs a reviewed canonical callback.
func (s *Service) SetBurnSoundWithCheckpointBinding(fn func([3]numeric.Fixed), authority *checkpoint.BindingAuthority) {
	s.SetBurnSound(fn)
	if fn != nil && authority != nil {
		s.checkpointCallbacks[checkpointBurnSound] = checkpointCallbackProof{owner: s, authority: authority}
	}
}

// SetBurnWeaponWithCheckpointBinding installs a reviewed canonical callback.
func (s *Service) SetBurnWeaponWithCheckpointBinding(fn func(string, [3]numeric.Fixed), authority *checkpoint.BindingAuthority) {
	s.SetBurnWeapon(fn)
	if fn != nil && authority != nil {
		s.checkpointCallbacks[checkpointBurnWeapon] = checkpointCallbackProof{owner: s, authority: authority}
	}
}

// SetGeothermalSteamWithCheckpointBinding installs a reviewed canonical callback.
func (s *Service) SetGeothermalSteamWithCheckpointBinding(fn func(numeric.Fixed, numeric.Fixed, numeric.Fixed), authority *checkpoint.BindingAuthority) {
	s.SetGeothermalSteam(fn)
	if fn != nil && authority != nil {
		s.checkpointCallbacks[checkpointGeothermalSteam] = checkpointCallbackProof{owner: s, authority: authority}
	}
}

// SetSequenceFramesWithCheckpointBinding installs a reviewed canonical callback.
func (s *Service) SetSequenceFramesWithCheckpointBinding(fn func(*content.FeatureDef, uint8) []int32, authority *checkpoint.BindingAuthority) {
	s.SetSequenceFrames(fn)
	if fn != nil && authority != nil {
		s.checkpointCallbacks[checkpointSequenceFrames] = checkpointCallbackProof{owner: s, authority: authority}
	}
}

// SetBindings records the exact service and RNG/wind owners for this capture.
// Conflicts leave the original registration intact (DESIGN_MULTIPLAYER §16.3.50).
func (c *CheckpointContext) SetBindings(s *Service, sim *rng.Simulation, crt *rng.CRT, wind *world.Wind, authority *checkpoint.BindingAuthority) error {
	if c == nil || s == nil || authority == nil {
		return featureCheckpointError("features.bindings", "a context, exact service and binding authority")
	}
	if c.bindingService != nil && (c.bindingService != s || c.bindingSim != sim || c.bindingCrt != crt || c.bindingWind != wind || !c.bindingAuthority.Matches(authority)) {
		return featureCheckpointError("features.bindings", "one consistent service, simulation RNG, CRT RNG, wind and authority")
	}
	c.bindingService, c.bindingSim, c.bindingCrt, c.bindingWind, c.bindingAuthority = s, sim, crt, wind, authority
	return nil
}

func (s *Service) validateCheckpointBindings(c *CheckpointContext) error {
	if c.bindingService != nil && c.bindingService != s {
		return featureCheckpointError("features.bindings", "the registered service")
	}
	present := [6]bool{s.burnFrameGeometry != nil, s.burnSmoke != nil, s.burnSound != nil, s.burnWeapon != nil, s.geothermalSteam != nil, s.sequenceFrames != nil}
	names := [6]string{"BurnFrameGeometry", "BurnSmoke", "BurnSound", "BurnWeapon", "GeothermalSteam", "SequenceFrames"}
	for slot, set := range present {
		proof := s.checkpointCallbacks[slot]
		if set && (c.bindingService != s || proof.owner != s || !proof.authority.Matches(c.bindingAuthority)) {
			return featureCheckpointError("features.Service."+names[slot], "the exact service's attested callback binding")
		}
	}
	if s.Crt != c.bindingCrt {
		return featureCheckpointError("features.Service.Crt", "the registered CRT RNG identity")
	}
	if s.Sim != c.bindingSim {
		return featureCheckpointError("features.Service.Sim", "the registered simulation RNG identity")
	}
	if s.Wind != c.bindingWind {
		return featureCheckpointError("features.Service.Wind", "the registered wind identity")
	}
	return nil
}
