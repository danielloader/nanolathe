package movement

import "testing"

type countingPilot struct {
	NoPilot
	add int
}

func (c countingPilot) BeginTick(s *System, _ uint32) {
	n, _ := s.PilotState.(*int)
	if n == nil {
		n = new(int)
		s.PilotState = n
	}
	*n += c.add
}

// Combined pilots each keep their own state.
func TestPilotsKeepTheirOwnState(t *testing.T) {
	s := &System{}
	p := Pilots{countingPilot{add: 1}, nil, countingPilot{add: 10}}
	for tick := uint32(1); tick <= 3; tick++ {
		p.BeginTick(s, tick)
	}
	st, ok := s.PilotState.(*pilotsState)
	if !ok {
		t.Fatalf("the combination's state was replaced by a pilot's: %T", s.PilotState)
	}
	if a, b := *st[0].(*int), *st[2].(*int); a != 3 || b != 30 {
		t.Fatalf("states are %d and %d, want 3 and 30", a, b)
	}
	if st[1] != nil {
		t.Fatal("an empty place was given state")
	}
}

// A traffic policy is compared with the zero value once a tick, so a pilot
// must be comparable.
func TestPilotsAreComparable(t *testing.T) {
	a := Traffic{Pilot: Pilots{countingPilot{add: 1}}}
	if a == (Traffic{}) {
		t.Fatal("a policy with a pilot equals the zero value")
	}
}
