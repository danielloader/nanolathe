//go:build retail

package session

import (
	"errors"
	"testing"

	"github.com/nanolathe-gg/nanolathe/internal/ai"
	"github.com/nanolathe-gg/nanolathe/internal/content"
	"github.com/nanolathe-gg/nanolathe/internal/gameplay"
	"github.com/nanolathe-gg/nanolathe/internal/mission"
	"github.com/nanolathe-gg/nanolathe/internal/orders"
	"github.com/nanolathe-gg/nanolathe/internal/survival"
	"github.com/nanolathe-gg/nanolathe/internal/testsupport/retailcat"
	"github.com/nanolathe-gg/nanolathe/vfs"
)

const admittedSkirmishMap = "ashap plateau"

// admittedRoomSchema is the schema index a room resolves once with the
// existing map-entry code: the network schema selected for the seat count,
// as its position in the map header.
func admittedRoomSchema(t *testing.T, fs vfs.FSOps, cat *content.Catalog, mapName string, players int) uint32 {
	t.Helper()
	m, err := mission.LoadWithType(fs, mission.TypeSkirmish, mapName, 0, players, nil)
	if err != nil {
		t.Fatal(err)
	}
	header := cat.Maps[content.CanonicalKey(m.TerrainKey)]
	if header == nil {
		t.Fatalf("no map header for %q", m.TerrainKey)
	}
	return mapSchemaIndex(header, m.Schema.Name)
}

func admittedFingerprint(t *testing.T, name, side string, s *Session) string {
	t.Helper()
	hash, err := s.PartialStateFingerprint()
	if err != nil {
		t.Fatalf("%s: %s fingerprint: %v", name, side, err)
	}
	return hash
}

// M2-C9/M2-C10 on the reference install. A single-seat configuration composed
// through NewAdmittedSkirmish, from the content frozen for that
// configuration, is the battle the local adapter's setup and options compose
// through NewSkirmishWithEntryOptions: the same frozen identity, the same
// state at entry and the same state after play. The admitted battle runs on
// the catalog and the sealed view the inputs hold — a mutated catalog is not
// mutated a second time — and nothing is frozen again.
func TestAdmittedSkirmishComposesTheLocalBattleRetail(t *testing.T) {
	cat, fs := retailcat.Shared(t)
	builder := orders.DefaultBuilderOptions()
	builder.Guard[0] = orders.GuardStay
	strict := DirectSkirmishConfig(admittedSkirmishMap)
	strict.Gameplay = gameplay.Strict31
	// Not the setup's default word, so the Classic computer's own
	// difficulty is what reaches the plan gate and the income discount.
	strict.Difficulty = 0
	modern := DirectSkirmishConfig(admittedSkirmishMap)
	modern.Players[1].AI = ai.ControllerModern
	modern.Location, modern.Difficulty = 0, 2
	var overrides AIOverrides
	overrides.All = "jitter=0"
	buddy := SurvivalSkirmishConfig(admittedSkirmishMap, 1, SurvivalOptions{Pace: survival.PaceRelaxed})
	alone := SurvivalSkirmishConfig(admittedSkirmishMap, 0, SurvivalOptions{})
	// No reader consults the difficulty word without an added computer
	// (§6.6): the local setup's easy word and the admitted default compose
	// one battle.
	alone.Difficulty = 0
	for _, c := range []struct {
		name    string
		cfg     SkirmishConfig
		options SkirmishEntryOptions
	}{
		{"Strict 3.1", strict, SkirmishEntryOptions{}},
		{"Modern with options and mutators", modern, SkirmishEntryOptions{BuilderOptions: &builder, AIOverrides: overrides,
			Mutators: content.Mutators{Income: content.Factor{Num: 3, Den: 2}, BuildSpeed: content.Factor{Num: 2, Den: 1}}}},
		{"Survival with a buddy", buddy, SkirmishEntryOptions{}},
		{"Survival alone", alone, SkirmishEntryOptions{}},
	} {
		// The setup stream's first draw from CRT seed 5006 is 16386, which
		// shuffles two eligible starts (shuffleEligibleStarts' gate), so
		// randomized and identity placement compose different battles.
		c.cfg.RNGSimSeed, c.cfg.RNGCrtSeed = 7, 5006
		local, err := NewSkirmishWithEntryOptions(fs, cat, c.cfg, c.options)
		if err != nil {
			t.Fatalf("%s: single-player entry: %v", c.name, err)
		}
		room := matchTestRoom()
		room.MapSchema = admittedRoomSchema(t, fs, cat, admittedSkirmishMap, local.Skirmish.NumPlayers)
		r, err := NewMatchConfigRequest(c.cfg, c.options, room)
		if err != nil {
			t.Fatalf("%s: adapter: %v", c.name, err)
		}
		config := resolveMatch(t, r)
		inputs, err := FreezeMatchInputs(fs, cat, config, nil)
		if err != nil {
			t.Fatalf("%s: freeze the configuration's content: %v", c.name, err)
		}
		if inputs.Digest() != local.frozenSimulationInputs().Digest() {
			t.Fatalf("%s: the configuration froze other content than single-player entry", c.name)
		}
		admitted, err := NewAdmittedSkirmish(inputs, config, nil)
		if err != nil {
			t.Fatalf("%s: admitted entry: %v", c.name, err)
		}
		if local.checkpointAdmission != nil {
			t.Fatalf("%s: ordinary local entry acquired checkpoint admission", c.name)
		}
		receipt := admitted.checkpointAdmission
		if receipt == nil || !receipt.ready || receipt.owner != admitted || receipt.inputs != inputs || receipt.mission != admitted.Mission ||
			receipt.config.Digest() != config.Digest() || receipt.authority == nil {
			t.Fatalf("%s: admitted entry lost its private completed provenance", c.name)
		}
		for _, mgr := range admitted.AI {
			if mgr != nil && mgr.CheckpointApplicationHistory() != nil {
				t.Fatalf("%s: constructor provenance unexpectedly enabled recording", c.name)
			}
		}
		if admitted.frozenSimulationInputs() != inputs || admitted.Catalog != inputs.Catalog() || admitted.simArt != inputs.SimArt() {
			t.Fatalf("%s: the admitted battle does not run on the inputs it was given", c.name)
		}
		if admitted.Catalog.Hash != local.Catalog.Hash || admitted.Rules.Name != local.Rules.Name || admitted.Community != local.Community {
			t.Fatalf("%s: catalog %s rules %q, want %s %q", c.name, admitted.Catalog.Hash, admitted.Rules.Name, local.Catalog.Hash, local.Rules.Name)
		}
		if !c.options.Mutators.IsZero() && admitted.Catalog.Hash == cat.Hash {
			t.Fatalf("%s: the mutators were not applied", c.name)
		}
		if a, l := admittedFingerprint(t, c.name, "admitted", admitted), admittedFingerprint(t, c.name, "local", local); a != l {
			t.Fatalf("%s: composes to %s, single-player entry to %s", c.name, a, l)
		}
		// A minute of play, long enough for the computers to start building.
		for tick := 1; tick <= 1800; tick++ {
			local.Step(local.Clock.ScaledAnchor + 1)
			admitted.Step(admitted.Clock.ScaledAnchor + 1)
			if tick%600 != 0 {
				continue
			}
			if a, l := admittedFingerprint(t, c.name, "admitted", admitted), admittedFingerprint(t, c.name, "local", local); a != l {
				t.Fatalf("%s: after %d steps %s, single-player entry %s", c.name, tick, a, l)
			}
		}
		if admitted.Clock.GlobalTick < 1700 {
			t.Fatalf("%s: the battle did not run: tick %d", c.name, admitted.Clock.GlobalTick)
		}
		if got := inputs.UncapturedLookups(); len(got) != 0 {
			t.Fatalf("%s: the admitted battle read content its capture had not frozen: %v", c.name, got)
		}
	}
}

// Content frozen under mutators admits only the configuration that names
// them: a configuration without them would compose on a catalog prepared for
// another battle.
func TestAdmissionRefusesContentPreparedForOtherMutatorsRetail(t *testing.T) {
	cat, fs := retailcat.Shared(t)
	cfg := DirectSkirmishConfig(admittedSkirmishMap)
	cfg.RNGSimSeed, cfg.RNGCrtSeed = 7, 11
	room := matchTestRoom()
	room.MapSchema = admittedRoomSchema(t, fs, cat, admittedSkirmishMap, cfg.NumPlayers)
	mutated := SkirmishEntryOptions{Mutators: content.Mutators{Damage: content.Factor{Num: 2, Den: 1}}}
	r, err := NewMatchConfigRequest(cfg, mutated, room)
	if err != nil {
		t.Fatal(err)
	}
	inputs, err := FreezeMatchInputs(fs, cat, resolveMatch(t, r), nil)
	if err != nil {
		t.Fatal(err)
	}
	r.Mutators = content.Mutators{}
	plain := resolveMatch(t, r)
	if err := ValidateMatchInputs(plain, inputs); !errors.Is(err, ErrMatchContentMismatch) {
		t.Fatalf("content prepared with mutators admitted a configuration without them: %v", err)
	}
	if s, err := NewAdmittedSkirmish(inputs, plain, nil); s != nil || !errors.Is(err, ErrMatchContentMismatch) {
		t.Fatalf("composed: %v", err)
	}
}
