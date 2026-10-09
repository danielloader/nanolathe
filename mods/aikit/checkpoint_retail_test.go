//go:build retail

package aikit_test

import (
	"bytes"
	"crypto/sha256"
	"hash"
	"reflect"
	"testing"

	"github.com/nanolathe-gg/nanolathe/internal/ai"
	"github.com/nanolathe-gg/nanolathe/internal/aikit"
	"github.com/nanolathe-gg/nanolathe/internal/content"
	"github.com/nanolathe-gg/nanolathe/internal/gameplay"
	"github.com/nanolathe-gg/nanolathe/internal/mission"
	"github.com/nanolathe-gg/nanolathe/internal/session"
	"github.com/nanolathe-gg/nanolathe/internal/sim/checkpoint"
	"github.com/nanolathe-gg/nanolathe/internal/testsupport/retailcat"
	_ "github.com/nanolathe-gg/nanolathe/mods/aikit"
)

// This exercises the real registered util+tac/Survival Host, including entry
// priming before history attachment. Request frequency must not affect the
// simulation or the cadence records (DESIGN_MULTIPLAYER §16.3.75, §16.3.78).
func TestCheckpointModernLifecycleRetail(t *testing.T) {
	cat, fs := retailcat.Shared(t)
	const mapName = "ashap plateau"
	for _, mode := range []gameplay.Mode{gameplay.Strict31, gameplay.Modern, gameplay.Community39} {
		for _, survival := range []bool{false, true} {
			name := string(mode) + "/skirmish"
			cfg := session.DirectSkirmishConfig(mapName)
			if survival {
				name = string(mode) + "/survival"
				cfg = session.SurvivalSkirmishConfig(mapName, 1, session.SurvivalOptions{})
			}
			t.Run(name, func(t *testing.T) {
				cfg.Gameplay, cfg.Difficulty, cfg.RNGSimSeed, cfg.RNGCrtSeed = mode, 2, 7, 7
				if err := cfg.ApplyComputerAI([]session.ComputerAI{{Row: session.ComputerAIEveryRow, Controller: ai.ControllerModern}}); err != nil {
					t.Fatal(err)
				}
				selected, err := mission.LoadWithType(fs, mission.TypeSkirmish, mapName, 0, cfg.NumPlayers, nil)
				if err != nil {
					t.Fatal(err)
				}
				header := cat.Maps[content.CanonicalKey(selected.TerrainKey)]
				if header == nil {
					t.Fatal("selected map header absent")
				}
				room := session.MatchRoomInputs{
					MapSchema: uint32(len(header.Schemas)), ContentProfile: "retail",
					PlayerView:    session.MatchView{MinimumScale: 1024, MaximumScale: 2048},
					SpectatorView: session.MatchView{MinimumScale: 256, MaximumScale: 2048, FullMap: true},
					ReplayView:    session.MatchView{MinimumScale: 256, MaximumScale: 2048, FullMap: true},
					Policies:      session.MatchPolicies{Revision: 1, Scheduling: 1, Pacing: 1, Drop: 1, Audience: 1, RejoinGraceMilliseconds: 90000},
				}
				room.Participants[0][0] = 1
				for i, schema := range header.Schemas {
					if schema.Name == selected.Schema.Name {
						room.MapSchema = uint32(i)
						break
					}
				}
				r, err := session.NewMatchConfigRequest(cfg, session.SkirmishEntryOptions{}, room)
				if err != nil {
					t.Fatal(err)
				}
				config, err := session.ResolveMatchConfig(r)
				if err != nil {
					t.Fatal(err)
				}
				inputs, err := session.FreezeMatchInputs(fs, cat, config, nil)
				if err != nil {
					t.Fatal(err)
				}

				// Same frozen entry, ordinary play, cadence-only diagnostics, then
				// cadence plus repeated explicit stream requests at other ticks.
				ordinary := runModernCheckpointBattle(t, inputs, config, false, false)
				cadence := runModernCheckpointBattle(t, inputs, config, true, false)
				requested := runModernCheckpointBattle(t, inputs, config, true, true)
				if ordinary.game != cadence.game || ordinary.game != requested.game ||
					ordinary.random != cadence.random || ordinary.random != requested.random {
					t.Fatal("checkpointing changed gameplay, controller outcomes or simulation/CRT draws")
				}
				if !reflect.DeepEqual(cadence.history, requested.history) || cadence.applications != requested.applications {
					t.Fatal("explicit captures changed cadence digests, cheap rows or Modern application history")
				}
			})
		}
	}
}

const modernCheckpointTicks = 120

type modernCheckpointGame struct {
	partial string // This existing fingerprint is deliberately not a complete-state proof.
	stats   [10]aikit.ApplyStats
}
type modernCheckpointRun struct {
	game         modernCheckpointGame
	random       [modernCheckpointTicks][4]uint64
	history      session.CheckpointHistory
	applications [10]ai.ApplicationHistoryState
}

type modernCheckpointOutput struct {
	hash.Hash
	bytes int
}

func (w *modernCheckpointOutput) Write(p []byte) (int, error) {
	n, err := w.Hash.Write(p)
	w.bytes += n
	return n, err
}

func runModernCheckpointBattle(t *testing.T, inputs *content.SimulationInputs, config session.EffectiveMatchConfig, enabled, requests bool) modernCheckpointRun {
	t.Helper()
	s, err := session.NewAdmittedSkirmish(inputs, config, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		for _, m := range s.AI {
			if m != nil {
				if h, ok := m.Ext.(*aikit.Host); ok && h != nil {
					h.Close()
				}
			}
		}
	}()
	for player, m := range s.AI {
		if m != nil && m.Controller == ai.ControllerModern {
			h, ok := m.Ext.(*aikit.Host)
			if !ok || h == nil || !h.ControllerCheckpoint().Initialized {
				t.Fatalf("entry did not prime the real Modern Host for slot %d: %T", player, m.Ext)
			}
			if m.CheckpointApplicationHistory() != nil {
				t.Fatal("entry fabricated pre-attachment application history")
			}
		}
	}
	if !s.PublishOpeningFrame() {
		t.Fatal("opening publication failed")
	}
	beforeSim, beforeCRT := *s.SimRNG(), *s.CrtRNG()
	before, err := s.PartialStateFingerprint()
	if err != nil {
		t.Fatal(err)
	}
	if enabled {
		if err := s.EnableCheckpoints(); err != nil {
			t.Fatalf("enable Modern checkpoints: %v", err)
		}
		result := s.CheckpointCaptureResult()
		history := s.CheckpointHistory()
		if result.Err != nil || result.Pending || result.Record.Position != (session.CheckpointPosition{Boundary: session.CheckpointEntry}) ||
			len(history.Records) != 1 || history.Records[0] != result.Record || len(history.Ticks) != 0 {
			t.Fatalf("entry capture result/history: %+v, %+v", result, history)
		}
		for _, d := range result.Record.Digests.Owners {
			if d == (checkpoint.Digest{}) {
				t.Fatal("entry omitted owner digest")
			}
		}
		for _, m := range s.AI {
			if m == nil {
				continue
			}
			state, err := m.CheckpointApplicationHistory().Snapshot()
			if err != nil || !state.Enabled || state.Kind != uint8(m.Controller)+1 || state.Count != 0 {
				t.Fatalf("entry history: %+v, %v", state, err)
			}
		}
	}
	after, err := s.PartialStateFingerprint()
	if err != nil || after != before || *s.SimRNG() != beforeSim || *s.CrtRNG() != beforeCRT || s.Clock.GlobalTick != 0 {
		t.Fatal("entry capture changed the completed battle")
	}
	var out modernCheckpointRun
	for tick := uint32(1); tick <= modernCheckpointTicks; tick++ {
		var stream *modernCheckpointOutput
		if requests && (tick == 17 || tick == 31 || tick == 61 || tick == 119) {
			stream = &modernCheckpointOutput{Hash: sha256.New()}
			if err := s.RequestCheckpointCapture(stream); err != nil {
				t.Fatal(err)
			}
			if !s.CheckpointCaptureResult().Pending {
				t.Fatal("request did not become pending")
			}
		}
		// Entry may still need its first state dispatch; zero-tick pumps
		// do not satisfy a pending capture request.
		for pumps := 0; s.Clock.GlobalTick < tick && pumps < 16; pumps++ {
			s.Step(s.Clock.ScaledAnchor + 1)
		}
		if s.Clock.GlobalTick != tick {
			t.Fatalf("battle did not complete tick %d, got %d", tick, s.Clock.GlobalTick)
		}
		out.random[tick-1] = [4]uint64{uint64(s.SimRNG().State), s.SimRNG().Draws(), uint64(s.CrtRNG().State), s.CrtRNG().Draws()}
		if enabled {
			result := s.CheckpointCaptureResult()
			if result.Err != nil {
				t.Fatalf("capture at tick %d: %v", tick, result.Err)
			}
			if stream != nil && (result.Pending || result.Record.Position.Tick != tick || result.Record.Position.Boundary != session.CheckpointFinalPumpTick ||
				stream.bytes == 0 || !bytes.Equal(stream.Sum(nil), result.Record.Digests.Full[:])) {
				t.Fatalf("requested full stream/result disagree at tick %d: %+v, bytes %d", tick, result, stream.bytes)
			}
		}
	}
	out.game.partial, err = s.PartialStateFingerprint()
	if err != nil {
		t.Fatal(err)
	}
	computers := 0
	for player, m := range s.AI {
		if m == nil {
			continue
		}
		if m.Controller != ai.ControllerModern {
			continue // Session also keeps passive human-slot managers.
		}
		computers++
		h, ok := m.Ext.(*aikit.Host)
		if !ok || h == nil {
			t.Fatalf("slot %d did not use the real Modern Host: %T", player, m.Ext)
		}
		out.game.stats[player] = h.Stats()
		if h.Stats().Applied == 0 {
			t.Fatalf("Modern slot %d applied no work by tick %d: %+v", player, modernCheckpointTicks, h.Stats())
		}
		if enabled {
			state, err := m.CheckpointApplicationHistory().Snapshot()
			if err != nil || !state.Enabled || state.Count == 0 || state.Kind != 2 || state.Player != uint8(player) {
				t.Fatalf("applied Modern history: %+v, %v", state, err)
			}
			out.applications[player] = state
		} else if m.CheckpointApplicationHistory() != nil {
			t.Fatal("ordinary battle enabled diagnostic history")
		}
	}
	if computers != 1 {
		t.Fatalf("fixture expected one Modern computer, got %d", computers)
	}
	out.history = s.CheckpointHistory()
	if enabled {
		if len(out.history.Ticks) != modernCheckpointTicks || len(out.history.Records) != 1+modernCheckpointTicks/30 {
			t.Fatal("entry/cadence history framing changed")
		}
		last := out.history.Records[len(out.history.Records)-1]
		if last.Position.Tick != modernCheckpointTicks || last.Digests.Owners[checkpoint.OwnerComputersScenario-1] == out.history.Records[0].Digests.Owners[checkpoint.OwnerComputersScenario-1] {
			t.Fatal("completed work did not reach the computer-owner checkpoint")
		}
	}
	return out
}
