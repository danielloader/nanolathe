package features

import (
	"github.com/nanolathe-gg/nanolathe/internal/content"
	"github.com/nanolathe-gg/nanolathe/internal/sim/numeric"
)

// Callback accessors retain the existing feature seams with private ownership.
// Getters transfer no checkpoint proof; ordinary setters clear only their
// slot's proof (DESIGN_MULTIPLAYER §16.3.50).
func (s *Service) SequenceFramesHook() func(*content.FeatureDef, uint8) []int32 {
	return s.sequenceFrames
}
func (s *Service) SetSequenceFrames(fn func(*content.FeatureDef, uint8) []int32) {
	s.sequenceFrames = fn
	s.checkpointCallbacks[checkpointSequenceFrames] = checkpointCallbackProof{}
}

func (s *Service) BurnWeaponHook() func(string, [3]numeric.Fixed) { return s.burnWeapon }
func (s *Service) SetBurnWeapon(fn func(string, [3]numeric.Fixed)) {
	s.burnWeapon = fn
	s.checkpointCallbacks[checkpointBurnWeapon] = checkpointCallbackProof{}
}

func (s *Service) BurnSoundHook() func([3]numeric.Fixed) { return s.burnSound }
func (s *Service) SetBurnSound(fn func([3]numeric.Fixed)) {
	s.burnSound = fn
	s.checkpointCallbacks[checkpointBurnSound] = checkpointCallbackProof{}
}

func (s *Service) GeothermalSteamHook() func(numeric.Fixed, numeric.Fixed, numeric.Fixed) {
	return s.geothermalSteam
}
func (s *Service) SetGeothermalSteam(fn func(numeric.Fixed, numeric.Fixed, numeric.Fixed)) {
	s.geothermalSteam = fn
	s.checkpointCallbacks[checkpointGeothermalSteam] = checkpointCallbackProof{}
}

func (s *Service) BurnFrameGeometryHook() func(*content.FeatureDef, int32) (int32, int32, int32, int32) {
	return s.burnFrameGeometry
}
func (s *Service) SetBurnFrameGeometry(fn func(*content.FeatureDef, int32) (int32, int32, int32, int32)) {
	s.burnFrameGeometry = fn
	s.checkpointCallbacks[checkpointBurnFrameGeometry] = checkpointCallbackProof{}
}

func (s *Service) BurnSmokeHook() func([3]numeric.Fixed) { return s.burnSmoke }
func (s *Service) SetBurnSmoke(fn func([3]numeric.Fixed)) {
	s.burnSmoke = fn
	s.checkpointCallbacks[checkpointBurnSmoke] = checkpointCallbackProof{}
}
