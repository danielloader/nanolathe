package session

import (
	"bytes"
	"slices"
	"testing"

	"github.com/nanolathe-gg/nanolathe/internal/clock"
	"github.com/nanolathe-gg/nanolathe/internal/cob"
	"github.com/nanolathe-gg/nanolathe/internal/content"
	"github.com/nanolathe-gg/nanolathe/internal/frame"
	"github.com/nanolathe-gg/nanolathe/internal/gameplay"
	"github.com/nanolathe-gg/nanolathe/internal/pool"
	"github.com/nanolathe-gg/nanolathe/internal/sim/checkpoint"
	"github.com/nanolathe-gg/nanolathe/internal/units"
)

func checkpointSessionScriptsFixture(t *testing.T, admitted bool) (*Session, *content.SimulationInputs) {
	t.Helper()
	inputs := checkpointAuthoredUnitInputs(t)
	s := &Session{Gameplay: gameplay.Strict31, Catalog: inputs.Catalog(), World: minimalTerrain(), Mission: syntheticMission(), Clock: &clock.State{Requested: 10, Active: 10}, Snapshot: &frame.Buffer{}}
	if admitted {
		a := checkpointAdmissionForTest(t, inputs, EffectiveMatchConfig{}, s.Mission)
		a.owner, s.checkpointAdmission = s, a
	}
	w, err := newBattleSlicedWorldWithCheckpointBinding(s.Catalog, inputs.Filesystem(), sessionKindSkirmish, [pool.PlayerCount]uint32{}, 4, s.checkpointBindingAuthority())
	if err != nil {
		t.Fatal(err)
	}
	s.Units, s.Econ = w, economyForTest()
	if err := createAndBindServicesForTest(t, s); err != nil {
		t.Fatal(err)
	}
	return s, inputs
}

func checkpointSessionCreateScriptUnit(t *testing.T, s *Session) *units.Unit {
	t.Helper()
	h, err := s.Units.Create(s.Catalog.Units["tiny"], 0, 0, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	u := s.Units.Unit(h)
	if u == nil || u.Script == nil || u.AllocationSerial == 0 {
		t.Fatal("fixture missed real allocation/binding")
	}
	return u
}

func checkpointSessionScriptContext(t *testing.T, s *Session, inputs *content.SimulationInputs) *units.CheckpointContext {
	t.Helper()
	keys, err := inputs.CheckpointKeys()
	if err != nil {
		t.Fatal(err)
	}
	c := units.NewCheckpointContext(keys)
	if err := s.prepareCheckpointUnitWorld(c); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Units.CollectCheckpointReferences(c); err != nil {
		t.Fatal(err)
	}
	return c
}

func TestCheckpointSessionScriptsCanonicalAndLateCreation(t *testing.T) {
	s, inputs := checkpointSessionScriptsFixture(t, true)
	first := checkpointSessionCreateScriptUnit(t, s)
	for phase := 0; phase < 2; phase++ {
		if phase == 1 {
			checkpointSessionCreateScriptUnit(t, s)
		}
		c := checkpointSessionScriptContext(t, s, inputs)
		sim, crt, tick := *s.SimRNG(), *s.CrtRNG(), s.Clock.GlobalTick
		threads, drain, flags := first.Script.Threads, first.Script.DrainCalls, slices.Clone(first.RenderPieceFlags)
		var prior []byte
		for n := 0; n < 3; n++ {
			if err := s.prepareCheckpointUnitScripts(c); err != nil {
				t.Fatal(err)
			}
			var out bytes.Buffer
			if err := s.Units.WriteScriptCheckpoint(checkpoint.NewEncoder(&out), c); err != nil {
				t.Fatal(err)
			}
			if out.Len() == 0 {
				t.Fatal("empty script capture")
			}
			if n != 0 && !bytes.Equal(prior, out.Bytes()) {
				t.Fatal("repeat script capture changed bytes")
			}
			prior = slices.Clone(out.Bytes())
		}
		if *s.SimRNG() != sim || *s.CrtRNG() != crt || s.Clock.GlobalTick != tick || first.Script.Threads != threads || first.Script.DrainCalls != drain || !slices.Equal(flags, first.RenderPieceFlags) {
			t.Fatal("capture changed live RNG/script state")
		}
	}
}

func TestCheckpointSessionScriptsRejectChangedInstallation(t *testing.T) {
	for _, tc := range []struct {
		name string
		edit func(*Session, *units.Unit)
	}{
		{"query port", func(_ *Session, u *units.Unit) {
			u.Script.BindPort(7, func([]int32) int32 { panic("capture called port") })
		}},
		{"adopted port", func(_ *Session, u *units.Unit) {
			u.Script.BindPortBinding(32, cob.PortBinding{Read: func([4]int32) int32 { panic("capture called port") }})
		}},
		{"transport", func(_ *Session, u *units.Unit) {
			u.Script.BindTransportMutations(func(int32, int32, int32) { panic("capture attached cargo") }, nil)
		}},
		{"captured terrain", func(s *Session, _ *units.Unit) {
			copy := *s.World
			s.World = &copy
		}},
		{"sink unit", func(_ *Session, u *units.Unit) {
			copy := *u
			u.COBBinding().PresentationSink.(*cobPresentationSink).checkpointUnit = &copy
		}},
	} {
		for _, before := range []bool{false, true} {
			t.Run(tc.name, func(t *testing.T) {
				s, inputs := checkpointSessionScriptsFixture(t, true)
				u := checkpointSessionCreateScriptUnit(t, s)
				c := checkpointSessionScriptContext(t, s, inputs)
				if !before {
					if err := s.prepareCheckpointUnitScripts(c); err != nil {
						t.Fatal(err)
					}
				}
				tc.edit(s, u)
				if err := s.prepareCheckpointUnitScripts(c); err == nil {
					t.Fatal("changed script installation accepted")
				}
			})
		}
	}
	s, inputs := checkpointSessionScriptsFixture(t, true)
	checkpointSessionCreateScriptUnit(t, s)
	c := checkpointSessionScriptContext(t, s, inputs)
	copy := Session{checkpointAdmission: s.checkpointAdmission, Units: s.Units, World: s.World, Clock: s.Clock, publication: s.publication}
	if err := copy.prepareCheckpointUnitScripts(c); err == nil {
		t.Fatal("copied session admitted")
	}
	ordinary, _ := checkpointSessionScriptsFixture(t, false)
	checkpointSessionCreateScriptUnit(t, ordinary)
	if err := ordinary.prepareCheckpointUnitScripts(c); err == nil {
		t.Fatal("ordinary session acquired admission")
	}
}
