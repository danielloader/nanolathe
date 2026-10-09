package session

import (
	"github.com/nanolathe-gg/nanolathe/internal/construction"
	"github.com/nanolathe-gg/nanolathe/internal/orders"
	"github.com/nanolathe-gg/nanolathe/internal/world"
	"testing"
)

func TestCheckpointConstructionOriginalSink(t *testing.T) {
	for _, replace := range []bool{false, true} {
		s, inputs := checkpointSessionScriptsFixture(t, true)
		keys, err := inputs.CheckpointKeys()
		if err != nil {
			t.Fatal(err)
		}
		wc := world.NewCheckpointContext(keys)
		wc.Terrain = s.World
		uc := checkpointSessionScriptContext(t, s, inputs)
		c := construction.NewCheckpointContext(orders.NewCheckpointContext(uc), wc)
		if err := s.prepareCheckpointConstruction(c); err != nil {
			t.Fatal(err)
		}
		if replace {
			s.Build.Presentation = &buildPresentationSink{session: s}
		} else {
			s.checkpointBuild.sink.session = &Session{}
		}
		if err := s.prepareCheckpointConstruction(c); err == nil {
			t.Fatal("replacement sink or captured session accepted")
		}
	}
}
