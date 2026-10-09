package session

import (
	"io"

	"github.com/nanolathe-gg/nanolathe/internal/ai"
	"github.com/nanolathe-gg/nanolathe/internal/combat"
	"github.com/nanolathe-gg/nanolathe/internal/construction"
	"github.com/nanolathe-gg/nanolathe/internal/content"
	"github.com/nanolathe-gg/nanolathe/internal/economy"
	"github.com/nanolathe-gg/nanolathe/internal/effects"
	"github.com/nanolathe-gg/nanolathe/internal/features"
	"github.com/nanolathe-gg/nanolathe/internal/movement"
	"github.com/nanolathe-gg/nanolathe/internal/orders"
	"github.com/nanolathe-gg/nanolathe/internal/path"
	"github.com/nanolathe-gg/nanolathe/internal/sim/checkpoint"
	"github.com/nanolathe-gg/nanolathe/internal/units"
	"github.com/nanolathe-gg/nanolathe/internal/visibility"
	"github.com/nanolathe-gg/nanolathe/internal/world"
)

// captureCheckpoint composes the existing owner fragments in schema order.
// The lifecycle caller owns the quiescent boundary; reference tables exist for
// this capture only (DESIGN_MULTIPLAYER §16.3.6, §16.3.78).
func (s *Session) captureCheckpoint(keys *content.CheckpointKeys, position CheckpointPosition, out io.Writer) (CheckpointRecord, error) {
	fail := func(err error) (CheckpointRecord, error) { return CheckpointRecord{}, err }
	if s == nil || s.checkpointBindingAuthority() == nil || !s.checkpointAdmission.ready || keys == nil || !s.rngInitialized || s.Clock == nil ||
		s.Units == nil || s.World == nil || s.Features == nil || s.Vis == nil || s.Movement == nil || s.Path == nil || s.Econ == nil || s.Build == nil || s.Combat == nil ||
		s.publication == nil || s.publication.events == nil || s.publication.effects == nil || s.publication.effects.CheckpointFixedPool() == nil || s.Snapshot == nil {
		return fail(runtimeCheckpointError("capture", "a complete admitted battle at a published boundary"))
	}
	if tick, ok := s.Snapshot.PublishedTick(); !ok || tick != s.Clock.GlobalTick || position.Tick != tick {
		return fail(runtimeCheckpointError("capture.publication", "successful publication of the current completed tick"))
	}
	a := s.checkpointAdmission
	if a.inputs == nil || s.Catalog != a.inputs.Catalog() {
		return fail(runtimeCheckpointError("capture.catalog", "the admitted catalog"))
	}
	if err := s.validateCheckpointInputs(); err != nil {
		return fail(err)
	}
	if err := a.inputs.ValidateCheckpointInputs(); err != nil {
		return fail(err)
	}
	if err := s.validateCheckpointRuleBindings(a.rules); err != nil {
		return fail(err)
	}
	if err := s.validateCheckpointArtBindings(a.inputs.SimArt()); err != nil {
		return fail(err)
	}
	uc := units.NewCheckpointContext(keys)
	oc := orders.NewCheckpointContext(uc)
	pc := path.NewCheckpointContext()
	mc := movement.NewCheckpointContext(oc, pc)
	wc := world.NewCheckpointContext(keys)
	wc.Terrain = s.World
	fc := features.NewCheckpointContext(wc)
	vc := visibility.NewCheckpointContext(keys)
	ec := economy.NewCheckpointContext(wc)
	bc := construction.NewCheckpointContext(oc, wc)
	cc := combat.NewCheckpointContext(uc, wc)
	fixed := s.publication.effects.CheckpointFixedPool()
	fx := effects.NewCheckpointContext(keys, fixed)
	authority := s.checkpointBindingAuthority()
	for _, prepare := range []func() error{
		func() error { return s.prepareCheckpointUnitWorld(uc) },
		func() error { return s.prepareCheckpointOrderBinding(oc) },
		func() error { return s.prepareCheckpointOrderHandlers(oc) },
		func() error { return s.prepareCheckpointMovement(mc, wc) },
		func() error { return fc.SetBindings(s.Features, &s.rngSim, &s.rngCrt, s.Wind, authority) },
		func() error { return vc.SetBindings(s.Vis, s.World, authority) },
		func() error { return ec.SetBindings(s.Econ, s.Wind, authority) },
		func() error { return s.prepareCheckpointConstruction(bc) },
		func() error { return s.prepareCheckpointCombat(cc) },
		func() error { return s.prepareCheckpointEffects(fx) },
	} {
		if err := prepare(); err != nil {
			return fail(err)
		}
	}
	var ac [10]*ai.CheckpointContext
	for player, m := range s.AI {
		if m == nil {
			continue
		}
		ac[player] = ai.NewCheckpointContext(uc, wc)
		if err := s.prepareCheckpointAI(ac[player], uint8(player)); err != nil {
			return fail(err)
		}
		state, err := m.CheckpointApplicationHistory().Snapshot()
		if err != nil {
			return fail(err)
		}
		if !state.Enabled || state.Player != uint8(player) || state.Kind != uint8(m.Controller)+1 {
			return fail(runtimeCheckpointError("capture.computers.history", "enabled matching player and controller history"))
		}
	}
	// The first unit visit establishes physical roots before any upper owner.
	if _, err := s.Units.CollectCheckpointReferences(uc); err != nil {
		return fail(err)
	}
	pump := &orders.Pump{}
	for {
		added := 0
		for _, collect := range []func() (int, error){
			func() (int, error) { return s.Units.CollectCheckpointReferences(uc) },
			func() (int, error) { return pump.CollectCheckpointReferences(oc) },
			func() (int, error) { return s.World.CollectCheckpointReferences(wc) },
			func() (int, error) { return s.Features.CollectCheckpointReferences(fc) },
			func() (int, error) { return s.Vis.CollectCheckpointReferences(vc) },
			func() (int, error) { return s.Movement.CollectCheckpointReferences(mc) },
			func() (int, error) { return s.Path.CollectCheckpointReferences(pc) },
			func() (int, error) { return s.Econ.CollectCheckpointReferences(ec) },
			func() (int, error) { return s.Build.CollectCheckpointReferences(bc) },
			func() (int, error) { return s.Combat.CollectCheckpointReferences(cc) },
			func() (int, error) { return s.publication.effects.CollectCheckpointReferences(fx) },
			func() (int, error) { return fixed.CollectCheckpointReferences(fx) },
		} {
			n, err := collect()
			if err != nil {
				return fail(err)
			}
			added += n
		}
		for player, m := range s.AI {
			if m != nil {
				n, err := m.CollectCheckpointReferences(ac[player])
				if err != nil {
					return fail(err)
				}
				added += n
			}
		}
		if added == 0 {
			break
		}
	}
	if err := s.prepareCheckpointUnitScripts(uc); err != nil {
		return fail(err)
	}
	capture, err := checkpoint.NewCapture(a.identity, out)
	if err != nil {
		return fail(err)
	}
	writers := [...]func(*checkpoint.Encoder) error{
		func(e *checkpoint.Encoder) error { return s.writeCheckpointRuntime(e, keys, position.Boundary) },
		func(e *checkpoint.Encoder) error { return s.Units.WriteCheckpoint(e, uc) },
		func(e *checkpoint.Encoder) error { return pump.WriteCheckpoint(e, oc) },
		func(e *checkpoint.Encoder) error { return s.Units.WriteScriptCheckpoint(e, uc) },
		func(e *checkpoint.Encoder) error {
			e.Bool(true)
			if err := s.World.WriteCheckpoint(e, wc); err != nil {
				return err
			}
			e.Bool(true)
			return s.Features.WriteCheckpoint(e, fc)
		},
		func(e *checkpoint.Encoder) error {
			e.Bool(true)
			if err := s.Vis.WriteCheckpoint(e, vc); err != nil {
				return err
			}
			return s.writeCheckpointVisibilityTail(e)
		},
		func(e *checkpoint.Encoder) error { return s.Movement.WriteCheckpoint(e, mc) },
		func(e *checkpoint.Encoder) error {
			if err := s.Movement.WritePathProviderCheckpoint(e, mc); err != nil {
				return err
			}
			return s.Path.WriteCheckpoint(e, pc)
		},
		func(e *checkpoint.Encoder) error { return s.Econ.WriteCheckpoint(e, ec) },
		func(e *checkpoint.Encoder) error { return s.Build.WriteCheckpoint(e, bc) },
		func(e *checkpoint.Encoder) error { return s.Combat.WriteCheckpoint(e, cc) },
		func(e *checkpoint.Encoder) error {
			e.Bool(true)
			if err := s.publication.events.WriteCheckpoint(e); err != nil {
				return err
			}
			e.Bool(true)
			if err := s.publication.effects.WriteCheckpoint(e, fx); err != nil {
				return err
			}
			e.Bool(true)
			if err := fixed.WriteCheckpoint(e, fx); err != nil {
				return err
			}
			e.Bool(s.debris != nil)
			if s.debris != nil {
				if err := s.debris.WriteCheckpoint(e); err != nil {
					return err
				}
			}
			e.Bool(s.strips != nil)
			if s.strips != nil {
				return s.strips.writeCheckpoint(e)
			}
			return e.Err()
		},
		func(e *checkpoint.Encoder) error {
			for player, m := range s.AI {
				e.Bool(m != nil)
				if m == nil {
					continue
				}
				if err := m.WriteCheckpoint(e, ac[player]); err != nil {
					return err
				}
				if err := m.CheckpointApplicationHistory().WriteCheckpoint(e); err != nil {
					return err
				}
				e.Bool(m.Ext != nil)
				if m.Ext != nil {
					if err := s.modernAICheckpointSource.WriteCheckpoint(m, e, ac[player]); err != nil {
						return err
					}
				}
			}
			return s.writeScenarioCheckpoint(e, &checkpointScenarioContext{keys: keys, mission: a.mission})
		},
	}
	for i, write := range writers {
		enc, err := capture.Section(checkpoint.Owner(i+1), true)
		if err != nil {
			return fail(err)
		}
		if err := write(enc); err != nil {
			return fail(err)
		}
	}
	digests, err := capture.Finish()
	if err != nil {
		return fail(err)
	}
	return CheckpointRecord{Position: position, Digests: digests}, nil
}
