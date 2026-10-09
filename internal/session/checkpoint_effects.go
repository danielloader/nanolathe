package session

import "github.com/nanolathe-gg/nanolathe/internal/effects"

// Effect art and the per-tick fragment context are simulation inputs. Capture
// checks their installed owners, never advancing or resolving them (§16.3.68).
func (s *Session) prepareCheckpointEffects(c *effects.CheckpointContext) error {
	if s == nil || c == nil || s.checkpointBindingAuthority() == nil || s.checkpointAdmission.inputs == nil ||
		s.publication == nil || s.publication.effects == nil || s.Clock == nil || s.World == nil ||
		s.simArt != s.checkpointAdmission.inputs.SimArt() {
		return runtimeCheckpointError("effects.bindings", "the admitted session's effect, art, clock and terrain owners")
	}
	return c.SetBindings(s.publication.effects, s.simArt, s.World, s.Clock.GlobalTick, s.checkpointBindingAuthority())
}
