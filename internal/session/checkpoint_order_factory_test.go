package session

import (
	"testing"

	"github.com/nanolathe-gg/nanolathe/internal/movement"
	"github.com/nanolathe-gg/nanolathe/internal/orders"
)

func checkpointOrderFactoryFixture(t *testing.T) *Session {
	t.Helper()
	s := newLoopTestSession(t, 0)
	a := checkpointAdmissionForTest(t, nil, EffectiveMatchConfig{}, s.Mission)
	a.owner, s.checkpointAdmission = s, a
	s.Build.OrderBinding = s.newOrderBinding()
	return s
}

func TestCheckpointOrderFactoryRetainsCapturedOwners(t *testing.T) {
	for _, tc := range []struct {
		name string
		edit func(*Session)
	}{
		{"binding", func(s *Session) {
			copy := *s.Build.OrderBinding
			s.Build.OrderBinding = &copy
		}},
		{"world", func(s *Session) { s.Build.OrderBinding.World = &orders.WorldQueryAdapter{} }},
		{"work", func(s *Session) { s.Build.OrderBinding.Work = &orders.WorkAdapter{} }},
		{"presentation", func(s *Session) { s.Build.OrderBinding.Presentation = &orders.PresentationAdapter{} }},
		{"weapons", func(s *Session) { s.Build.OrderBinding.Weapons = &orders.WeaponAdapter{} }},
		{"movement adapter", func(s *Session) { s.Build.OrderBinding.Movement = &orders.MovementGoalAdapter{} }},
		{"movement system", func(s *Session) { s.Movement = &movement.System{} }},
		{"copied movement", func(s *Session) {
			copy := *s.Movement
			s.Movement = &copy
		}},
		{"ordinary callback", func(s *Session) { b := s.Build.OrderBinding; b.SetLookup(b.LookupHook()) }},
		{"ordinary captured reader", func(s *Session) { b := s.Build.OrderBinding.World; b.SetLookupUnit(b.LookupUnitHook()) }},
	} {
		for _, beforeRegistration := range []bool{false, true} {
			t.Run(tc.name, func(t *testing.T) {
				s := checkpointOrderFactoryFixture(t)
				c := orders.NewCheckpointContext(nil)
				if !beforeRegistration {
					if err := s.prepareCheckpointOrderBinding(c); err != nil {
						t.Fatalf("canonical factory: %v", err)
					}
				}
				tc.edit(s)
				if err := s.prepareCheckpointOrderBinding(c); err == nil {
					t.Fatal("replacement concealed a captured owner")
				}
			})
		}
	}
}

func TestCheckpointOrderFactoryDoesNotInvokeReadiness(t *testing.T) {
	s := checkpointOrderFactoryFixture(t)
	b, a := s.Build.OrderBinding, s.checkpointBindingAuthority()
	panicReady := func() bool { panic("capture called gameplay readiness") }
	b.Movement.SetReadyWithCheckpointBinding(panicReady, a)
	b.Work.SetReadyWithCheckpointBinding(panicReady, a)
	b.Weapons.SetReadyWithCheckpointBinding(panicReady, a)
	b.Presentation.SetReadyWithCheckpointBinding(panicReady, a)
	beforeSim, beforeCRT, beforeTick := *s.SimRNG(), *s.CrtRNG(), s.Clock.GlobalTick
	c := orders.NewCheckpointContext(nil)
	for range 3 {
		if err := s.prepareCheckpointOrderBinding(c); err != nil {
			t.Fatal(err)
		}
	}
	if *s.SimRNG() != beforeSim || *s.CrtRNG() != beforeCRT || s.Clock.GlobalTick != beforeTick {
		t.Fatal("capture changed the session")
	}
	copy := Session{checkpointAdmission: s.checkpointAdmission, checkpointOrders: s.checkpointOrders, Build: s.Build, Econ: s.Econ, Movement: s.Movement}
	if err := copy.prepareCheckpointOrderBinding(orders.NewCheckpointContext(nil)); err == nil {
		t.Fatal("copied session acquired captured owners")
	}
}

func TestCheckpointOrderFactoryOrdinaryCompositionHasNoProof(t *testing.T) {
	s := newLoopTestSession(t, 0)
	if err := s.prepareCheckpointOrderBinding(orders.NewCheckpointContext(nil)); err == nil {
		t.Fatal("ordinary factory admitted")
	}
}
