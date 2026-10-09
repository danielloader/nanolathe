package session

import (
	"math"
	"testing"
)

// Input metadata counts drained records, not calls made while staging a scene.
// Both accepted no-ops and refused commands still have a consumed position.
func TestCheckpointConsumedInputAtDrainOnly(t *testing.T) {
	s := &Session{checkpoints: &sessionCheckpoints{enabled: true}}
	s.applyHumanCommand(HumanCommand{Kind: HumanStop}, 1)
	if s.checkpoints.consumed != 0 {
		t.Fatal("direct staging counted as queued input")
	}
	s.pendingHuman = []HumanCommand{
		{Kind: HumanStop, DueTick: 1},
		{Kind: HumanStop, DueTick: 2},
		{Kind: HumanStop, DueTick: 2},
	}
	s.applyHumanCommands(1)
	if s.checkpoints.consumed != 1 || len(s.pendingHuman) != 2 {
		t.Fatal("phase drain counted future input")
	}
	if n := s.applyPausedHumanCommands(2, nil); n != 2 || s.checkpoints.consumed != 3 {
		t.Fatal("paused drain lost input positions")
	}
	s.applyHumanCommands(2)
	if s.checkpoints.consumed != 3 {
		t.Fatal("drained inputs counted twice")
	}
	s.checkpoints.consumed = math.MaxUint64
	s.pendingHuman = []HumanCommand{{Kind: HumanNoShake, DueTick: 3}}
	s.applyHumanCommands(3)
	if s.checkpoints.consumed != math.MaxUint64 || s.CheckpointCaptureResult().Err == nil || !s.NoShake() {
		t.Fatal("ordinal overflow wrapped, failed silently or suppressed gameplay")
	}
}
