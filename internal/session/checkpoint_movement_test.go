package session

import (
	"testing"

	"github.com/nanolathe-gg/nanolathe/internal/movement"
	"github.com/nanolathe-gg/nanolathe/internal/orders"
	"github.com/nanolathe-gg/nanolathe/internal/path"
	"github.com/nanolathe-gg/nanolathe/internal/world"
)

func TestCheckpointMovementCanonicalSessionWiring(t *testing.T) {
	s, inputs := checkpointSessionScriptsFixture(t, true)
	uc := checkpointSessionScriptContext(t, s, inputs)
	c, w := movement.NewCheckpointContext(orders.NewCheckpointContext(uc), path.NewCheckpointContext()), world.NewCheckpointContext(uc.Keys)
	if err := s.prepareCheckpointMovement(c, w); err != nil {
		t.Fatal(err)
	}
	if err := s.prepareCheckpointMovement(c, w); err != nil {
		t.Fatal("repeat registration", err)
	}
	copy := Session{checkpointAdmission: s.checkpointAdmission, World: s.World, Units: s.Units, Movement: s.Movement, Path: s.Path}
	if err := copy.prepareCheckpointMovement(movement.NewCheckpointContext(nil, path.NewCheckpointContext()), world.NewCheckpointContext(nil)); err == nil {
		t.Fatal("copied session acquired authority")
	}
	s.Path = path.NewScheduler(nil, nil)
	if err := s.prepareCheckpointMovement(c, w); err == nil {
		t.Fatal("foreign scheduler accepted")
	}
}
