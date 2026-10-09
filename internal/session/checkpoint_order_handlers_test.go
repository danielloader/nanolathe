package session

import (
	"bytes"
	"testing"

	"github.com/nanolathe-gg/nanolathe/internal/orders"
	"github.com/nanolathe-gg/nanolathe/internal/sim/checkpoint"
	"github.com/nanolathe-gg/nanolathe/internal/units"
	"github.com/nanolathe-gg/nanolathe/internal/world"
)

func TestCheckpointSessionOrderHandlersFollowLateQueues(t *testing.T) {
	s, inputs := checkpointSessionScriptsFixture(t, true)
	u := checkpointSessionCreateScriptUnit(t, s)
	q := orders.QueueForUnit(u)
	s.bindExistingOrderQueue(u)
	for phase := 0; phase < 2; phase++ {
		if phase == 1 {
			q = &orders.Queue{}
			orders.BindQueue(u, q)
		}
		uc := checkpointSessionScriptContext(t, s, inputs)
		c := orders.NewCheckpointContext(uc)
		if err := s.prepareCheckpointOrderBinding(c); err != nil {
			t.Fatal(err)
		}
		beforeSim, beforeCRT, beforeTick := *s.SimRNG(), *s.CrtRNG(), s.Clock.GlobalTick
		for range 2 {
			if err := s.prepareCheckpointOrderHandlers(c); err != nil {
				t.Fatal(err)
			}
		}
		p := &orders.Pump{}
		for {
			added, err := p.CollectCheckpointReferences(c)
			if err != nil {
				t.Fatal(err)
			}
			if added == 0 {
				break
			}
		}
		var first bytes.Buffer
		if err := p.WriteCheckpoint(checkpoint.NewEncoder(&first), c); err != nil {
			t.Fatal(err)
		}
		var second bytes.Buffer
		if err := p.WriteCheckpoint(checkpoint.NewEncoder(&second), c); err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(first.Bytes(), second.Bytes()) || *s.SimRNG() != beforeSim || *s.CrtRNG() != beforeCRT || s.Clock.GlobalTick != beforeTick {
			t.Fatal("handler capture changed world")
		}
	}
	// Returning a function through the ordinary getter cannot confer the
	// constructor's source proof on a replacement installation.
	id := orders.Lookup("MobileBuild")
	q.SetOwnedHandler(id, q.OwnedHandlerFor(id))
	c := orders.NewCheckpointContext(checkpointSessionScriptContext(t, s, inputs))
	if err := s.prepareCheckpointOrderBinding(c); err != nil {
		t.Fatal(err)
	}
	if err := s.prepareCheckpointOrderHandlers(c); err != nil {
		t.Fatal(err)
	}
	if _, err := (&orders.Pump{}).CollectCheckpointReferences(c); err == nil {
		t.Fatal("ordinary handler reinstall acquired proof")
	}
}

func TestCheckpointSessionOrderHandlerOwnerRefusal(t *testing.T) {
	s, _ := checkpointSessionScriptsFixture(t, true)
	c := orders.NewCheckpointContext(units.NewCheckpointContext(nil))
	original := s.Movement
	copy := *original
	s.Movement = &copy
	if err := s.prepareCheckpointOrderHandlers(c); err == nil {
		t.Fatal("copied movement admitted")
	}
	s.Movement = original
	if err := s.prepareCheckpointOrderHandlers(c); err != nil {
		t.Fatal("failed registration changed context", err)
	}
	other := Session{checkpointAdmission: s.checkpointAdmission, Build: s.Build, Movement: s.Movement}
	if err := other.prepareCheckpointOrderHandlers(c); err == nil {
		t.Fatal("foreign session reused receipt")
	}
	// Captured terrain ownership remains separately verified; handler admission
	// alone never repairs or grants the unit-world graph.
	s.World = &world.Terrain{}
	if err := s.prepareCheckpointUnitWorld(units.NewCheckpointContext(nil)); err == nil {
		t.Fatal("handler admission bypassed world ownership")
	}
}
