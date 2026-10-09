package session

import (
	"slices"

	"github.com/nanolathe-gg/nanolathe/internal/ai"
	"github.com/nanolathe-gg/nanolathe/internal/sim/checkpoint"
)

// QueueBuildTyped captures both Session and Manager; the other canonical
// callbacks capture Session. Retain the original slot owner (§16.3.71).
type sessionCheckpointAIManager struct {
	owner     *Session
	manager   *ai.Manager
	authority *checkpoint.BindingAuthority
}

func (s *Session) prepareCheckpointAI(c *ai.CheckpointContext, player uint8) error {
	if s == nil || c == nil || s.checkpointBindingAuthority() == nil || s.checkpointAdmission.inputs == nil || !s.rngInitialized || s.Build == nil || int(player) >= len(s.AI) {
		return runtimeCheckpointError("ai.bindings", "an admitted session, initialized RNG and computer slot")
	}
	m, p := s.AI[player], s.checkpointAI[player]
	if m == nil || m.Player != player || p.owner != s || p.manager != m || !p.authority.Matches(s.checkpointBindingAuthority()) {
		return runtimeCheckpointError("ai.bindings", "the original manager in its session slot")
	}
	var info *ai.SurvivalInfo
	if st := s.Survival; st != nil {
		if st.info == nil {
			return runtimeCheckpointError("ai.Survival", "the scenario's initialized team information")
		}
		if player != st.attacker && slices.Contains(st.info.Team, player) {
			info = st.info
		}
	}
	if m.Controller == ai.ControllerModern {
		if err := c.SetModernBindings(m, s.modernAI, s.modernAICheckpointWitness, s.modernAICheckpointSource, p.authority); err != nil {
			return err
		}
	}
	return c.SetBindings(m, s.checkpointAdmission.inputs, &s.rngSim, s.Build.OrderBinding, info, p.authority)
}
