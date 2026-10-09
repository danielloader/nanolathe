package session

import (
	"github.com/nanolathe-gg/nanolathe/internal/construction"
	"github.com/nanolathe-gg/nanolathe/internal/sim/checkpoint"
)

// The sink can create authoritative strips. Retain the original installation,
// not a replacement inferred from its current fields (§16.3.67).
type sessionCheckpointBuildBinding struct {
	owner     *Session
	service   *construction.Service
	sink      *buildPresentationSink
	authority *checkpoint.BindingAuthority
}

func (s *Session) prepareCheckpointConstruction(c *construction.CheckpointContext) error {
	if s == nil || c == nil || s.checkpointBindingAuthority() == nil || s.checkpointAdmission.inputs == nil || s.Build == nil {
		return runtimeCheckpointError("construction.bindings", "the admitted session and construction owner")
	}
	p := s.checkpointBuild
	if p.owner != s || p.service != s.Build || p.sink == nil || p.sink.session != s || !p.authority.Matches(s.checkpointBindingAuthority()) {
		return runtimeCheckpointError("construction.Presentation", "the original construction sink and its session owner")
	}
	next := *c
	if err := construction.SetCheckpointPresentationSink(&next, p.sink); err != nil {
		return err
	}
	if err := next.SetBindings(s.Build, s.checkpointAdmission.inputs, s.Units, s.Econ, s.Combat, s.Movement, s.Build.OrderBinding, p.authority); err != nil {
		return err
	}
	*c = next
	return nil
}
