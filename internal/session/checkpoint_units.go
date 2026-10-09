package session

import (
	"slices"

	"github.com/nanolathe-gg/nanolathe/internal/cob"

	"github.com/nanolathe-gg/nanolathe/internal/movement"
	"github.com/nanolathe-gg/nanolathe/internal/units"
)

// prepareCheckpointUnitWorld supplies the closed upper-owner aliases that the
// unit package cannot inspect without a dependency cycle (§16.3.56). It observes
// existing wiring; installation proof is still checked by the lower owner.
func (s *Session) prepareCheckpointUnitWorld(c *units.CheckpointContext) error {
	if s == nil || c == nil || s.checkpointBindingAuthority() == nil || s.checkpointAdmission.inputs == nil ||
		s.Units == nil || s.World == nil || s.Movement == nil {
		return runtimeCheckpointError("units.bindings", "an admitted session and complete unit/movement owners")
	}
	observer, ok := s.Units.AttachmentObserver().(*movement.System)
	if !ok || observer == nil || observer != s.Movement {
		return runtimeCheckpointError("units.attachmentObserver", "the session's exact movement owner")
	}
	pose, ok := s.Units.CreationPose().(*movement.System)
	if !ok || pose == nil || pose != s.Movement || s.Movement.Terrain != s.World {
		return runtimeCheckpointError("units.pose", "the session's exact movement and terrain owners")
	}
	fs, loader := s.Units.COBSource()
	if loader == nil || !s.checkpointAdmission.inputs.CheckpointFilesystemMatches(fs) || !s.Units.HasCOBBinder() {
		return runtimeCheckpointError("units.COBSource", "the frozen source, per-battle loader and strict binder")
	}
	a := s.checkpointBindingAuthority()
	if err := c.SetLifecycleBindings(s.Units, a); err != nil {
		return err
	}
	return c.SetWorldBindings(s.Units, s.checkpointAdmission.inputs, s.World, s.SimRNG(), loader, a)
}

// validateCheckpointCOBSinks uses only closed concrete pointers and fields.
// These sinks append authoritative effect records; capture must neither emit an
// event nor ask a sink to describe itself (DESIGN_MULTIPLAYER §16.3.55).
func (s *Session) validateCheckpointCOBSinks(u *units.Unit) (*cobPresentationSink, *cobExplosionSink, error) {
	bad := func() (*cobPresentationSink, *cobExplosionSink, error) {
		return nil, nil, runtimeCheckpointError("scripts.sinks", "the exact session, unit, publication, clock and piece-map owners")
	}
	if s == nil || u == nil || s.publication == nil || s.Clock == nil || u.Script == nil {
		return bad()
	}
	binding := u.COBBinding()
	if binding == nil || binding.VM != u.Script {
		return bad()
	}
	presentation, ok := binding.PresentationSink.(*cobPresentationSink)
	if !ok || presentation == nil || presentation.session != s || presentation.publication != s.publication ||
		presentation.clock != s.Clock || presentation.source != u.Handle || !slices.Equal(presentation.pieceMap, binding.PieceMap) {
		return bad()
	}
	if s.checkpointBindingAuthority() != nil && (presentation.checkpointUnit != u ||
		presentation.checkpointTerrain == nil || presentation.checkpointTerrain != s.World) {
		return bad()
	}
	explosion, ok := u.Script.ExplosionSink().(*cobExplosionSink)
	if !ok || explosion == nil || explosion.presentation != presentation {
		return bad()
	}
	return presentation, explosion, nil
}

// prepareCheckpointUnitScripts supplies the actual lower runtime operands only
// after reference discovery. All bindings were installed before Create; capture
// checks them and never calls a port, callback, or script (§16.3.60, §16.3.65).
func (s *Session) prepareCheckpointUnitScripts(c *units.CheckpointContext) error {
	if s == nil || c == nil || s.checkpointBindingAuthority() == nil || s.Units == nil {
		return runtimeCheckpointError("scripts.bindings", "an admitted session and discovered unit context")
	}
	for _, u := range c.Allocations.Values() {
		if u == nil {
			return runtimeCheckpointError("scripts.bindings", "a nonnil discovered allocation")
		}
		if u.Script == nil {
			continue
		}
		presentation, explosion, err := s.validateCheckpointCOBSinks(u)
		if err != nil {
			return err
		}
		lower, err := c.ScriptBindings(u)
		if err != nil {
			return err
		}
		binding := u.COBBinding()
		if err := lower.SetRuntimeSources(binding, s.SimRNG(), u.RenderPieceFlags, binding.PieceMap); err != nil {
			return err
		}
		if err := cob.SetCheckpointPresentationSink(lower, presentation); err != nil {
			return err
		}
		if err := cob.SetCheckpointExplosionSink(lower, explosion); err != nil {
			return err
		}
		if err := u.Script.ValidateCheckpointBindings(lower); err != nil {
			return err
		}
	}
	return nil
}
