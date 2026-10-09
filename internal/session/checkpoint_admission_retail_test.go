//go:build retail

package session

import (
	"bytes"
	"crypto/sha256"
	"testing"

	"github.com/nanolathe-gg/nanolathe/internal/ai"
	"github.com/nanolathe-gg/nanolathe/internal/combat"
	"github.com/nanolathe-gg/nanolathe/internal/construction"
	"github.com/nanolathe-gg/nanolathe/internal/economy"
	"github.com/nanolathe-gg/nanolathe/internal/effects"
	"github.com/nanolathe-gg/nanolathe/internal/features"
	"github.com/nanolathe-gg/nanolathe/internal/gameplay"
	"github.com/nanolathe-gg/nanolathe/internal/movement"
	"github.com/nanolathe-gg/nanolathe/internal/orders"
	"github.com/nanolathe-gg/nanolathe/internal/path"
	"github.com/nanolathe-gg/nanolathe/internal/sim/checkpoint"
	"github.com/nanolathe-gg/nanolathe/internal/testsupport/retailcat"
	"github.com/nanolathe-gg/nanolathe/internal/visibility"
	"github.com/nanolathe-gg/nanolathe/internal/world"
)

// Frozen input validation and installation receipts must survive real battle
// entry and later allocations; checking only authored empty fixtures cannot
// establish that composition preserves the captured immutable inputs (§16.3.63).
func TestCheckpointAdmittedOwnersRetail(t *testing.T) {
	cat, fs := retailcat.Shared(t)
	for _, mode := range []gameplay.Mode{gameplay.Strict31, gameplay.Modern, gameplay.Community39} {
		for _, survival := range []bool{false, true} {
			name := string(mode) + "/skirmish"
			cfg := DirectSkirmishConfig(admittedSkirmishMap)
			if survival {
				name = string(mode) + "/survival"
				cfg = SurvivalSkirmishConfig(admittedSkirmishMap, 1, SurvivalOptions{})
			}
			t.Run(name, func(t *testing.T) {
				cfg.Gameplay, cfg.RNGSimSeed, cfg.RNGCrtSeed = mode, 7, 7
				room := matchTestRoom()
				room.MapSchema = admittedRoomSchema(t, fs, cat, admittedSkirmishMap, cfg.NumPlayers)
				r, err := NewMatchConfigRequest(cfg, SkirmishEntryOptions{}, room)
				if err != nil {
					t.Fatal(err)
				}
				config := resolveMatch(t, r)
				inputs, err := FreezeMatchInputs(fs, cat, config, nil)
				if err != nil {
					t.Fatal(err)
				}
				s, err := NewAdmittedSkirmish(inputs, config, nil)
				if err != nil {
					t.Fatal(err)
				}
				if !s.PublishOpeningFrame() {
					t.Fatal("opening publication failed")
				}
				fullKeys, err := inputs.CheckpointKeys()
				if err != nil {
					t.Fatal(err)
				}
				for _, m := range s.AI {
					if m != nil {
						if err := m.EnableCheckpointApplications(s.checkpointAdmission.identity, fullKeys); err != nil {
							t.Fatal(err)
						}
					}
				}
				for phase := 0; phase < 2; phase++ {
					if phase != 0 {
						for range 300 {
							s.Step(s.Clock.ScaledAnchor + 1)
						}
					}
					if err := s.validateCheckpointInputs(); err != nil {
						t.Fatal(err)
					}
					if err := inputs.ValidateCheckpointInputs(); err != nil {
						t.Fatal(err)
					}
					uc := checkpointSessionScriptContext(t, s, inputs)
					oc := orders.NewCheckpointContext(uc)
					if err := s.prepareCheckpointOrderBinding(oc); err != nil {
						t.Fatal(err)
					}
					if err := s.prepareCheckpointOrderHandlers(oc); err != nil {
						t.Fatal(err)
					}
					pump := &orders.Pump{}
					wc := world.NewCheckpointContext(uc.Keys)
					wc.Terrain = s.World
					cc := combat.NewCheckpointContext(uc, wc)
					if err := s.prepareCheckpointCombat(cc); err != nil {
						t.Fatal(err)
					}
					var ac [10]*ai.CheckpointContext
					for player, m := range s.AI {
						if m == nil {
							continue
						}
						ac[player] = ai.NewCheckpointContext(uc, wc)
						if err := s.prepareCheckpointAI(ac[player], uint8(player)); err != nil {
							t.Fatal(err)
						}
					}
					pc := path.NewCheckpointContext()
					mc := movement.NewCheckpointContext(oc, pc)
					if err := s.prepareCheckpointMovement(mc, wc); err != nil {
						t.Fatal(err)
					}
					fc := features.NewCheckpointContext(wc)
					if err := fc.SetBindings(s.Features, &s.rngSim, &s.rngCrt, s.Wind, s.checkpointBindingAuthority()); err != nil {
						t.Fatal(err)
					}
					vc := visibility.NewCheckpointContext(uc.Keys)
					if err := vc.SetBindings(s.Vis, s.World, s.checkpointBindingAuthority()); err != nil {
						t.Fatal(err)
					}
					ec2 := economy.NewCheckpointContext(wc)
					if err := ec2.SetBindings(s.Econ, s.Wind, s.checkpointBindingAuthority()); err != nil {
						t.Fatal(err)
					}
					bc := construction.NewCheckpointContext(oc, wc)
					if err := s.prepareCheckpointConstruction(bc); err != nil {
						t.Fatal(err)
					}
					if _, err := s.Build.CollectCheckpointReferences(bc); err != nil {
						t.Fatal(err)
					}
					effectPool := s.publication.effects.CheckpointFixedPool()
					ec := effects.NewCheckpointContext(uc.Keys, effectPool)
					if err := s.prepareCheckpointEffects(ec); err != nil {
						t.Fatal(err)
					}
					if _, err := s.publication.effects.CollectCheckpointReferences(ec); err != nil {
						t.Fatal(err)
					}
					if _, err := effectPool.CollectCheckpointReferences(ec); err != nil {
						t.Fatal(err)
					}
					for {
						added := 0
						for _, collect := range []func() (int, error){
							func() (int, error) { return s.Units.CollectCheckpointReferences(uc) },
							func() (int, error) { return pump.CollectCheckpointReferences(oc) },
							func() (int, error) { return s.Build.CollectCheckpointReferences(bc) },
							func() (int, error) { return s.Movement.CollectCheckpointReferences(mc) },
							func() (int, error) { return s.Path.CollectCheckpointReferences(pc) },
							func() (int, error) { return s.Combat.CollectCheckpointReferences(cc) },
							func() (int, error) { return s.World.CollectCheckpointReferences(wc) },
							func() (int, error) { return s.Features.CollectCheckpointReferences(fc) },
							func() (int, error) { return s.Vis.CollectCheckpointReferences(vc) },
							func() (int, error) { return s.Econ.CollectCheckpointReferences(ec2) },
						} {
							n, err := collect()
							if err != nil {
								t.Fatal(err)
							}
							added += n
						}
						for player, m := range s.AI {
							if m == nil {
								continue
							}
							n, err := m.CollectCheckpointReferences(ac[player])
							if err != nil {
								t.Fatal(err)
							}
							added += n
						}
						if added == 0 {
							break
						}
					}
					beforeSim, beforeCRT, beforeTick := *s.SimRNG(), *s.CrtRNG(), s.Clock.GlobalTick
					boundary := CheckpointEntry
					if phase != 0 {
						boundary = CheckpointFinalPumpTick
					}
					position := CheckpointPosition{Tick: s.Clock.GlobalTick, Boundary: boundary}
					var full bytes.Buffer
					record, err := s.captureCheckpoint(fullKeys, position, &full)
					if err != nil {
						t.Fatal(err)
					}
					position.Pump, position.ConsumedInput = 17, 99
					repeated, err := s.captureCheckpoint(fullKeys, position, nil)
					if err != nil {
						t.Fatal(err)
					}
					if record.Digests != repeated.Digests || record.Digests.Full != sha256.Sum256(full.Bytes()) {
						t.Fatal("full bytes, digest-only capture or metadata exclusion disagree")
					}
					if err := s.prepareCheckpointUnitScripts(uc); err != nil {
						t.Fatal(err)
					}
					var script, order, build, effect, move, paths, weapons, computers bytes.Buffer
					var land, feature, vision, resources bytes.Buffer
					for _, write := range []func() error{
						func() error { return s.World.WriteCheckpoint(checkpoint.NewEncoder(&land), wc) },
						func() error { return s.Features.WriteCheckpoint(checkpoint.NewEncoder(&feature), fc) },
						func() error { return s.Vis.WriteCheckpoint(checkpoint.NewEncoder(&vision), vc) },
						func() error { return s.Econ.WriteCheckpoint(checkpoint.NewEncoder(&resources), ec2) },
					} {
						if err := write(); err != nil {
							t.Fatal(err)
						}
					}

					for player, m := range s.AI {
						if m == nil {
							continue
						}
						if err := m.WriteCheckpoint(checkpoint.NewEncoder(&computers), ac[player]); err != nil {
							t.Fatal(err)
						}
					}
					if err := s.Combat.WriteCheckpoint(checkpoint.NewEncoder(&weapons), cc); err != nil {
						t.Fatal(err)
					}
					if err := s.Movement.WriteCheckpoint(checkpoint.NewEncoder(&move), mc); err != nil {
						t.Fatal(err)
					}
					if err := s.Path.WriteCheckpoint(checkpoint.NewEncoder(&paths), pc); err != nil {
						t.Fatal(err)
					}
					if err := s.publication.effects.WriteCheckpoint(checkpoint.NewEncoder(&effect), ec); err != nil {
						t.Fatal(err)
					}
					if err := effectPool.WriteCheckpoint(checkpoint.NewEncoder(&effect), ec); err != nil {
						t.Fatal(err)
					}
					if err := s.Build.WriteCheckpoint(checkpoint.NewEncoder(&build), bc); err != nil {
						t.Fatal(err)
					}
					if err := s.Units.WriteScriptCheckpoint(checkpoint.NewEncoder(&script), uc); err != nil {
						t.Fatal(err)
					}
					if err := pump.WriteCheckpoint(checkpoint.NewEncoder(&order), oc); err != nil {
						t.Fatal(err)
					}
					if script.Len() == 0 || order.Len() == 0 || *s.SimRNG() != beforeSim || *s.CrtRNG() != beforeCRT || s.Clock.GlobalTick != beforeTick {
						t.Fatal("empty capture or live state changed")
					}
				}
			})
		}
	}
}
