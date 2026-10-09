package ai

import "github.com/nanolathe-gg/nanolathe/internal/sim/checkpoint"

// WriteCheckpoint writes the one scenario-owned input record, not any worker
// copy (DESIGN_MULTIPLAYER §16.3.38). Its lexical fields are Attacker, CentreX, CentreZ,
// Computer, Starts, Team, warnings. The caller supplies record presence and
// composition validates the managers' aliases separately.
func (s *SurvivalInfo) WriteCheckpoint(e *checkpoint.Encoder) error {
	e.Field("ai.SurvivalInfo")
	if s == nil {
		e.Fail(aiCheckpointError("ai.SurvivalInfo", "present scenario input"))
		return e.Err()
	}
	e.U8(s.Attacker)
	e.I32(s.CentreX)
	e.I32(s.CentreZ)
	e.Field("ai.SurvivalInfo.Computer")
	e.Count(len(s.Computer))
	for _, v := range s.Computer {
		e.Bool(v)
	}
	e.Field("ai.SurvivalInfo.Starts")
	e.Count(len(s.Starts))
	for _, v := range s.Starts {
		e.I32(v[0])
		e.I32(v[1])
	}
	e.Field("ai.SurvivalInfo.Team")
	e.Count(len(s.Team))
	for _, v := range s.Team {
		e.U8(v)
	}
	// PublishWarning replaces the list whole. One atomic load retains that
	// exact list, including warnings after any hypothetical observation tick.
	warnings := s.warnings.Load()
	e.Field("ai.SurvivalInfo.warnings")
	e.Bool(warnings != nil)
	if warnings != nil {
		e.Count(len(*warnings))
		for _, w := range *warnings {
			// Warning: Arrive, Groups, Tick, Wave. Approach: Angle, Domain, X, Z.
			e.U32(w.Arrive)
			e.Count(len(w.Groups))
			for _, g := range w.Groups {
				e.U16(g.Angle)
				e.String(g.Domain)
				e.I32(g.X)
				e.I32(g.Z)
			}
			e.U32(w.Tick)
			e.I32(w.Wave)
		}
	}
	return e.Err()
}
