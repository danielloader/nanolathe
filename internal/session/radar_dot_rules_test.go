package session

import (
	"testing"

	"github.com/nanolathe-gg/nanolathe/internal/frame"
	"github.com/nanolathe-gg/nanolathe/internal/gameplay"
)

// Rule publication is a stateless Modern policy; switches/rebinds and frame
// reuse must never leave the Modern permission on a Strict frame.
func TestRadarDotRulePublicationAndRebinding(t *testing.T) {
	s := &Session{Snapshot: &frame.Buffer{}}
	s.SeedSessionRNG(73, 23)
	sim, crt := *s.SimRNG(), *s.CrtRNG()
	for i, mode := range []gameplay.Mode{gameplay.Modern, gameplay.Strict31, gameplay.Community39, gameplay.Modern, gameplay.Strict31} {
		s.SetGameplay(mode)
		s.RebindRules()
		s.publishFrame(uint32(i+1), false)
		if got := s.Snapshot.Current().MainViewRadarDots; got != (mode == gameplay.Modern) {
			t.Fatalf("%s: dots allowed %v", mode, got)
		}
	}
	if *s.SimRNG() != sim || *s.CrtRNG() != crt {
		t.Fatal("rule selection/publication consumed RNG")
	}
}
