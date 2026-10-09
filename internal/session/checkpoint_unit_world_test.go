package session

import (
	"strings"
	"testing"

	"github.com/nanolathe-gg/nanolathe/internal/cob"
	"github.com/nanolathe-gg/nanolathe/internal/content"
	"github.com/nanolathe-gg/nanolathe/internal/movement"
	"github.com/nanolathe-gg/nanolathe/internal/pool"
	"github.com/nanolathe-gg/nanolathe/internal/sim/checkpoint"
	"github.com/nanolathe-gg/nanolathe/internal/units"
)

func checkpointAuthoredUnitInputs(t *testing.T) *content.SimulationInputs {
	t.Helper()
	data, err := (sessionFixtureCOBFS{}).ReadFileLimit("scripts/tiny.cob", 0)
	if err != nil {
		t.Fatal(err)
	}
	fs := fsFromMapSkirmish(t, map[string]string{
		"scripts/tiny.cob":   string(data),
		"objects3d/tiny.3do": string(fixture3DO(0)),
	})
	program, err := cob.Load(data)
	if err != nil {
		t.Fatal(err)
	}
	cat := &content.Catalog{Hash: "authored-checkpoint-fixture", Units: map[string]*content.UnitDef{"tiny": {
		DefinitionHeader: content.DefinitionHeader{CanonicalKey: "tiny"}, UnitName: "tiny", ObjectName: "tiny",
		Script: program, UnitDefID: 1, Limit: -1, BMCode: 1, MaxDamage: 100,
	}}}
	sources, err := content.CaptureSimulationSources(fs, "")
	if err != nil {
		t.Fatal(err)
	}
	inputs, err := content.FreezeSimulationInputs(sources, content.SimulationInputRequest{Catalog: cat})
	if err != nil {
		t.Fatal(err)
	}
	return inputs
}

func TestCheckpointUnitWorldConstructorSourceProof(t *testing.T) {
	inputs := checkpointAuthoredUnitInputs(t)
	keys, err := inputs.CheckpointKeys()
	if err != nil {
		t.Fatal(err)
	}
	a := checkpoint.NewBindingAuthority()
	for _, admitted := range []bool{false, true} {
		var authority *checkpoint.BindingAuthority
		if admitted {
			authority = a
		}
		w, err := newBattleSlicedWorldWithCheckpointBinding(inputs.Catalog(), inputs.Filesystem(), sessionKindSkirmish, [pool.PlayerCount]uint32{}, 2, authority)
		if err != nil {
			t.Fatal(err)
		}
		_, loader := w.COBSource()
		c := units.NewCheckpointContext(keys)
		if err := c.SetWorldBindings(w, inputs, nil, nil, loader, a); err != nil {
			t.Fatal(err)
		}
		_, err = w.CollectCheckpointReferences(c)
		if admitted && err != nil {
			t.Fatalf("canonical source: %v", err)
		}
		if !admitted && (err == nil || !strings.Contains(err.Error(), "cobFS")) {
			t.Fatalf("ordinary source acquired proof: %v", err)
		}
	}
}

type checkpointHostileUnitOwner []byte

func (checkpointHostileUnitOwner) AttachmentChanged(*units.Unit)  { panic("capture called observer") }
func (checkpointHostileUnitOwner) CorrectCreatedPose(*units.Unit) { panic("capture called pose") }

func TestCheckpointUnitWorldClosedMovementAliases(t *testing.T) {
	for _, tc := range []struct {
		name string
		edit func(*Session)
	}{
		{"foreign observer", func(s *Session) { s.Units.SetAttachmentObserver(&movement.System{}) }},
		{"nil observer", func(s *Session) { s.Units.SetAttachmentObserver((*movement.System)(nil)) }},
		{"hostile observer", func(s *Session) { s.Units.SetAttachmentObserver(checkpointHostileUnitOwner{1}) }},
		{"foreign pose", func(s *Session) { s.Units.SetCreationPose(&movement.System{}) }},
		{"nil pose", func(s *Session) { s.Units.SetCreationPose((*movement.System)(nil)) }},
		{"hostile pose", func(s *Session) { s.Units.SetCreationPose(checkpointHostileUnitOwner{1}) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := newLoopTestSession(t, 0)
			inputs := checkpointAuthoredUnitInputs(t)
			a := checkpointAdmissionForTest(t, inputs, EffectiveMatchConfig{}, s.Mission)
			a.owner, s.checkpointAdmission = s, a
			s.Units.SetCOBSourceWithCheckpointBinding(inputs.Filesystem(), globalCobLoader, a.authority)
			s.Units.SetCOBBinderWithCheckpointBinding(func(*units.Unit) error { panic("capture called binder") }, inputs.Filesystem(), a.authority)
			s.Units.SetAttachmentObserverWithCheckpointBinding(s.Movement, a.authority)
			s.Units.SetCreationPoseWithCheckpointBinding(s.Movement, a.authority)
			c := units.NewCheckpointContext(&content.CheckpointKeys{})
			if err := s.prepareCheckpointUnitWorld(c); err != nil {
				t.Fatalf("canonical aliases: %v", err)
			}
			tc.edit(s)
			if err := s.prepareCheckpointUnitWorld(c); err == nil {
				t.Fatal("foreign movement alias accepted")
			}
		})
	}
}
