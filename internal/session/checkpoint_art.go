package session

import (
	"fmt"

	"github.com/nanolathe-gg/nanolathe/internal/content"
)

// Resolver provenance belongs to the installed callback, not merely the current
// art pointer (DESIGN_MULTIPLAYER §16.3.53). The private methods below are the
// only canonical installations; public replacement clears the matching slot.
type checkpointArtBinding struct {
	owner *Session
	art   *content.SimArt
}

func (s *Session) bindCheckpointEffectFrames(art *content.SimArt) {
	if s == nil {
		return
	}
	if art == nil {
		s.SetEffectEntryFrameCount(nil)
		return
	}
	s.SetEffectEntryFrameCount(art.EffectEntryFrameCount)
	s.checkpointArt[0] = checkpointArtBinding{s, art}
}

func (s *Session) bindCheckpointFeatureSequence(art *content.SimArt) {
	if s == nil {
		return
	}
	if art == nil {
		s.SetFeatureSequenceResolver(nil)
		return
	}
	s.SetFeatureSequenceResolver(art.FeatureSequence)
	s.checkpointArt[1] = checkpointArtBinding{s, art}
}

func (s *Session) validateCheckpointArtBindings(art *content.SimArt) error {
	fail := func(path string) error {
		return fmt.Errorf("nanolathe: checkpoint admission failed: logical path %s, providers searched [frozen simulation inputs], expected exact session resolver bound to admitted art", path)
	}
	if s == nil || art == nil || s.simArt != art {
		return fail("session.simArt")
	}
	names := [2]string{"effectFrameCount", "featureSequence"}
	present := [2]bool{s.effectFrameCount != nil, s.featureSequence != nil}
	for i, p := range s.checkpointArt {
		if !present[i] || p.owner != s || p.art != art {
			return fail("session." + names[i])
		}
	}
	return nil
}
