package session

import (
	"github.com/nanolathe-gg/nanolathe/internal/combat"
	"github.com/nanolathe-gg/nanolathe/internal/movement"
	"github.com/nanolathe-gg/nanolathe/internal/orders"
	"github.com/nanolathe-gg/nanolathe/internal/sim/checkpoint"
)

// VisitOffMapFiled retains the grid's method receiver; the remaining canonical
// closures retain Session. Preserve both original owners (§16.3.70).
type sessionCheckpointCombatBinding struct {
	owner     *Session
	service   *combat.Service
	grid      *movement.OccupancyGrid
	reaction  *combat.ReactionSeams
	authority *checkpoint.BindingAuthority
}

func (s *Session) prepareCheckpointCombat(c *combat.CheckpointContext) error {
	if s == nil || c == nil || s.checkpointBindingAuthority() == nil || s.checkpointAdmission.inputs == nil || s.Combat == nil || s.Movement == nil {
		return runtimeCheckpointError("combat.bindings", "the admitted session and combat owner")
	}
	p := s.checkpointCombat
	if p.owner != s || p.service != s.Combat || p.grid == nil || p.grid != s.Movement.Grid || p.reaction == nil || p.reaction != s.Combat.Reaction || !p.authority.Matches(s.checkpointBindingAuthority()) {
		return runtimeCheckpointError("combat.bindings", "the original combat, reaction, grid and session owners")
	}
	if err := orders.ValidateCheckpointParalyzeTaskBinding(); err != nil {
		return err
	}
	return c.SetBindings(s.Combat, s.checkpointAdmission.inputs, s.Features, s.Wind, p.reaction, p.authority)
}
