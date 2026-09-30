package movement

import (
	"github.com/nanolathe-gg/nanolathe/internal/path"
	"github.com/nanolathe-gg/nanolathe/internal/pool"
	"github.com/nanolathe-gg/nanolathe/internal/sim/numeric"
)

// Pilots runs up to four pilots in order, each seeing what the ones before
// it wanted: Modern's route claims and arrival places, which were built
// apart, run as one (docs/DESIGN_MOVEMENT_PATH.md "Modern traffic"). A nil
// place is skipped.
//
// Each pilot keeps its state in System.PilotState as if it were alone: the
// combination holds one state per place and lends the System's slot to each
// pilot for the length of its call.
type Pilots [4]Pilot

type pilotsState [4]any

func (p Pilots) each(s *System, call func(Pilot)) {
	st, _ := s.PilotState.(*pilotsState)
	if st == nil {
		st = new(pilotsState)
	}
	for i, one := range p {
		if one == nil {
			continue
		}
		s.PilotState = st[i]
		call(one)
		st[i] = s.PilotState
	}
	s.PilotState = st
}

func (p Pilots) BeginTick(s *System, tick uint32) {
	p.each(s, func(one Pilot) { one.BeginTick(s, tick) })
}

func (p Pilots) Visit(s *System, v *Visit) {
	p.each(s, func(one Pilot) { one.Visit(s, v) })
}

func (p Pilots) GroupOrdered(s *System, owner uint8, members []pool.Handle, x, z numeric.Fixed, tick uint32) {
	p.each(s, func(one Pilot) { one.GroupOrdered(s, owner, members, x, z, tick) })
}

func (p Pilots) Search(s *System, r path.Request, cfg *path.SearchConfig) {
	p.each(s, func(one Pilot) { one.Search(s, r, cfg) })
}

func (p Pilots) Forget(s *System, h pool.Handle) {
	p.each(s, func(one Pilot) { one.Forget(s, h) })
}
