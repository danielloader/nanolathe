package session

import (
	"strings"
	"testing"

	"github.com/nanolathe-gg/nanolathe/internal/content"
)

func checkpointArtFixture() *Session {
	s := &Session{simArt: &content.SimArt{}}
	s.bindCheckpointEffectFrames(s.simArt)
	s.bindCheckpointFeatureSequence(s.simArt)
	return s
}

func TestCheckpointArtInstallationOwnership(t *testing.T) {
	for _, slot := range []struct {
		name      string
		reinstall func(*Session)
		clear     func(*Session)
		bind      func(*Session, *content.SimArt)
		index     int
	}{
		{"effectFrameCount", func(s *Session) { s.SetEffectEntryFrameCount(s.effectFrameCount) }, func(s *Session) { s.SetEffectEntryFrameCount(nil) }, (*Session).bindCheckpointEffectFrames, 0},
		{"featureSequence", func(s *Session) { s.SetFeatureSequenceResolver(s.featureSequence) }, func(s *Session) { s.SetFeatureSequenceResolver(nil) }, (*Session).bindCheckpointFeatureSequence, 1},
	} {
		t.Run(slot.name, func(t *testing.T) {
			s := checkpointArtFixture()
			if err := s.validateCheckpointArtBindings(s.simArt); err != nil {
				t.Fatal(err)
			}
			other := s.checkpointArt[1-slot.index]
			refuse := func() {
				t.Helper()
				err := s.validateCheckpointArtBindings(s.simArt)
				if err == nil || !strings.Contains(err.Error(), slot.name) {
					t.Fatalf("missing %s refusal: %v", slot.name, err)
				}
			}
			slot.reinstall(s)
			refuse()
			if s.checkpointArt[slot.index] != (checkpointArtBinding{}) || s.checkpointArt[1-slot.index] != other {
				t.Fatal("ordinary reinstall retained proof or invalidated another slot")
			}
			slot.bind(s, s.simArt)
			if err := s.validateCheckpointArtBindings(s.simArt); err != nil {
				t.Fatal(err)
			}
			slot.bind(s, &content.SimArt{})
			refuse()
			slot.bind(s, s.simArt)
			slot.clear(s)
			refuse()
			slot.bind(s, s.simArt)
			slot.bind(s, nil)
			refuse()
			if s.checkpointArt[slot.index] != (checkpointArtBinding{}) {
				t.Fatal("nil art retained proof")
			}
		})
	}
	s := checkpointArtFixture()
	// Explicitly alias the relevant fields, without copying Session's mutex.
	copySession := &Session{simArt: s.simArt, effectFrameCount: s.effectFrameCount, featureSequence: s.featureSequence, checkpointArt: s.checkpointArt}
	if err := copySession.validateCheckpointArtBindings(s.simArt); err == nil {
		t.Fatal("foreign session reused resolver proof")
	}
	for _, art := range []*content.SimArt{nil, &content.SimArt{}} {
		if err := s.validateCheckpointArtBindings(art); err == nil {
			t.Fatal("foreign/missing art accepted")
		}
	}
	if err := (*Session)(nil).validateCheckpointArtBindings(s.simArt); err == nil {
		t.Fatal("nil session accepted")
	}
	(*Session)(nil).bindCheckpointEffectFrames(s.simArt)
	(*Session)(nil).bindCheckpointFeatureSequence(s.simArt)
}

func TestCheckpointArtValidationPurity(t *testing.T) {
	s := checkpointArtFixture()
	before := s.checkpointArt
	if n := testing.AllocsPerRun(50, func() {
		if err := s.validateCheckpointArtBindings(s.simArt); err != nil {
			t.Fatal(err)
		}
	}); n != 0 {
		t.Fatalf("validation allocated %v", n)
	}
	if s.checkpointArt != before {
		t.Fatal("validation changed resolver proof")
	}
	// Unknown replacements are rejected without invocation. The effect setter's
	// existing resolve pass has no containers in this fixture.
	s.SetEffectEntryFrameCount(func(string, string) (int, bool) { panic("capture invoked effect resolver") })
	s.SetFeatureSequenceResolver(func(string, string, int32) (int32, int32, int32, int32, int32, bool) {
		panic("capture invoked feature resolver")
	})
	if err := s.validateCheckpointArtBindings(s.simArt); err == nil {
		t.Fatal("ordinary replacements accepted")
	}
}
