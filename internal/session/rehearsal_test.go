package session

import (
	"testing"

	"github.com/nanolathe-gg/nanolathe/internal/content"
)

// Rehearsal tests of DESIGN_MULTIPLAYER §16.7 on the authored restriction
// install: two hostile humans whose weaponless commanders have no build
// list, so only the move runs. The retail test covers construction and
// weapons. These are Nanolathe protocol values, not retail data.

// rehearsalFixtureConfig is the authored scene's two-human configuration,
// with edit applied to its setup and options first.
func rehearsalFixtureConfig(t *testing.T, edit func(*SkirmishConfig, *SkirmishEntryOptions)) EffectiveMatchConfig {
	t.Helper()
	cfg, options := restrictionMatchSetup()
	if edit != nil {
		edit(&cfg, &options)
	}
	r, err := NewMatchConfigRequest(cfg, options, matchTestRoom())
	if err != nil {
		t.Fatal(err)
	}
	second := &r.Seats[1]
	second.Role, second.HostSeat = MatchRoleHuman, MatchHostNone
	second.ComputerKind, second.Difficulty, second.AIParams = 0, 0, nil
	second.Participant = matchTestID(2)
	return resolveMatch(t, r)
}

// rehearsalFixtureInputs freezes config's content from a fresh mount of the
// authored install.
func rehearsalFixtureInputs(t *testing.T, config EffectiveMatchConfig) *content.SimulationInputs {
	t.Helper()
	fs, cat := restrictionMatchInstall(t)
	inputs, err := FreezeMatchInputs(fs, cat, config, nil)
	if err != nil {
		t.Fatal(err)
	}
	return inputs
}

func rehearsalDigestOf(t *testing.T, inputs *content.SimulationInputs, config EffectiveMatchConfig, localSeat uint8) [32]byte {
	t.Helper()
	d, err := rehearse(inputs, config, localSeat, nil)
	if err != nil {
		t.Fatal(err)
	}
	if d == ([32]byte{}) {
		t.Fatal("the rehearsal reported a zero digest")
	}
	return d
}

// Each seat freezes its own content from the configuration's bytes, so the
// two machines' rehearsals are independent compositions. Their digests are
// equal whichever seat a composition presents, the same inputs give the same
// digest again, and a rehearsal leaves the match session composed from the
// same inputs exactly as it would have been without one.
func TestRehearsalDigestAgreesAcrossCompositions(t *testing.T) {
	config := rehearsalFixtureConfig(t, nil)
	host := rehearsalFixtureInputs(t, config)
	payload, err := EncodeMatchConfig(config)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := DecodeMatchConfig(payload)
	if err != nil {
		t.Fatal(err)
	}
	joiner := rehearsalFixtureInputs(t, decoded)

	match, err := NewPlaytestSkirmish(host, config, 0, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := match.PrepareGrantedBattle(); err != nil {
		t.Fatal(err)
	}
	before, sim, crt := match.UnitStateChecksum(), *match.SimRNG(), *match.CrtRNG()

	want, err := RehearsalDigest(host, config)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name   string
		inputs *content.SimulationInputs
		config EffectiveMatchConfig
		seat   uint8
	}{
		{"the host's inputs again", host, config, 0},
		{"the host's inputs presenting seat 1", host, config, 1},
		{"the joiner's own inputs", joiner, decoded, 0},
		{"the joiner's own inputs presenting seat 1", joiner, decoded, 1},
	} {
		if got := rehearsalDigestOf(t, tc.inputs, tc.config, tc.seat); got != want {
			t.Fatalf("%s: digest %x, want %x", tc.name, got, want)
		}
	}

	if match.UnitStateChecksum() != before || *match.SimRNG() != sim || *match.CrtRNG() != crt || match.Clock.GlobalTick != 0 {
		t.Fatal("a rehearsal changed the match session composed from its inputs")
	}
	control, err := NewPlaytestSkirmish(joiner, decoded, 1, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := control.PrepareGrantedBattle(); err != nil {
		t.Fatal(err)
	}
	for tick := uint32(1); tick <= 60; tick++ {
		for _, s := range []*Session{match, control} {
			if err := s.StepGranted(tick); err != nil {
				t.Fatal(err)
			}
		}
		if match.UnitStateChecksum() != control.UnitStateChecksum() || match.SimRNG().Draws() != control.SimRNG().Draws() {
			t.Fatalf("the match session left the control at tick %d after a rehearsal on its inputs", tick)
		}
	}
}

// A configuration that differs only in its seeds or in one mutator is
// another rehearsal.
func TestRehearsalDigestFollowsTheConfiguration(t *testing.T) {
	config := rehearsalFixtureConfig(t, nil)
	base := rehearsalDigestOf(t, rehearsalFixtureInputs(t, config), config, 0)
	speed, err := content.ParseMutators(map[string]string{"unitSpeed": "2"})
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name string
		edit func(*SkirmishConfig, *SkirmishEntryOptions)
	}{
		{"another simulation seed", func(c *SkirmishConfig, _ *SkirmishEntryOptions) { c.RNGSimSeed++ }},
		{"another CRT seed", func(c *SkirmishConfig, _ *SkirmishEntryOptions) { c.RNGCrtSeed++ }},
		{"a unit speed mutator", func(_ *SkirmishConfig, o *SkirmishEntryOptions) { o.Mutators = speed }},
	} {
		other := rehearsalFixtureConfig(t, tc.edit)
		if other.Digest() == config.Digest() {
			t.Fatalf("%s: the edit did not change the configuration", tc.name)
		}
		if rehearsalDigestOf(t, rehearsalFixtureInputs(t, other), other, 0) == base {
			t.Fatalf("%s: the rehearsal digest did not change", tc.name)
		}
	}
}
