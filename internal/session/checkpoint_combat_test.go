package session

import (
	"testing"

	"github.com/nanolathe-gg/nanolathe/internal/combat"
	"github.com/nanolathe-gg/nanolathe/internal/movement"
	"github.com/nanolathe-gg/nanolathe/internal/world"
)

func TestCheckpointCombatOriginalOwners(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(*Session)
	}{
		{"service copy", func(s *Session) { copy := *s.Combat; s.Combat = &copy }},
		{"reaction copy", func(s *Session) { copy := *s.Combat.Reaction; s.Combat.Reaction = &copy }},
		{"grid replacement", func(s *Session) { s.Movement.Grid = movement.NewOccupancyGrid() }},
		{"ordinary callback", func(s *Session) { s.Combat.SetEvents(s.Combat.EventsHook()) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, inputs := checkpointSessionScriptsFixture(t, true)
			uc := checkpointSessionScriptContext(t, s, inputs)
			wc := world.NewCheckpointContext(uc.Keys)
			c := combat.NewCheckpointContext(uc, wc)
			if err := s.prepareCheckpointCombat(c); err != nil {
				t.Fatal(err)
			}
			tc.change(s)
			if err := s.prepareCheckpointCombat(c); err == nil {
				t.Fatal("replacement inherited combat admission")
			}
		})
	}
}
