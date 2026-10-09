//go:build retail

package session

import (
	"testing"
	"time"

	"github.com/nanolathe-gg/nanolathe/internal/content"
	"github.com/nanolathe-gg/nanolathe/internal/testsupport/retailcat"
	"github.com/nanolathe-gg/nanolathe/internal/units"
)

// rehearsalRetailMatch is the two-human Modern match on the retail
// two-player map, with options applied to its entry options.
func rehearsalRetailMatch(t *testing.T, options SkirmishEntryOptions) (*content.SimulationInputs, EffectiveMatchConfig) {
	t.Helper()
	cat, fs := retailcat.Shared(t)
	setup := DirectSkirmishConfig(admittedSkirmishMap)
	setup.RNGSimSeed, setup.RNGCrtSeed = 7, 11
	room := matchTestRoom()
	room.MapSchema = admittedRoomSchema(t, fs, cat, setup.MapName, 2)
	r, err := NewMatchConfigRequest(setup, options, room)
	if err != nil {
		t.Fatal(err)
	}
	second := &r.Seats[1]
	second.Role, second.HostSeat = MatchRoleHuman, MatchHostNone
	second.ComputerKind, second.Difficulty, second.AIParams = 0, 0, nil
	second.Participant = matchTestID(2)
	config := resolveMatch(t, r)
	inputs, err := FreezeMatchInputs(fs, cat, config, nil)
	if err != nil {
		t.Fatal(err)
	}
	return inputs, config
}

// The script reaches movement, construction and weapons for both seats on
// retail content (DESIGN_MULTIPLAYER §16.7): each commander walks, completes
// a structure from its own build list and then hits it with its own weapon.
// The digest is the same whichever seat the rehearsal presents, and a build
// speed mutator changes it.
func TestRehearsalExercisesTheScriptRetail(t *testing.T) {
	inputs, config := rehearsalRetailMatch(t, SkirmishEntryOptions{})
	var (
		ticks     int
		start     [2][2]int64
		moved     [2]bool
		completed [2]bool
		damaged   [2]bool
		shots     int
	)
	health := map[uint64]int32{}
	digest, err := rehearse(inputs, config, 0, func(s *Session) {
		ticks++
		if s.Combat.Count() != 0 {
			shots++
		}
		for seat := 0; seat < 2; seat++ {
			first := true
			s.Units.ForEachPlayerSliceLive(seat, func(u *units.Unit) {
				if first {
					at := [2]int64{int64(u.X), int64(u.Z)}
					if ticks == 1 {
						start[seat] = at
					}
					moved[seat] = moved[seat] || at != start[seat]
				} else if u.Remaining == 0 {
					completed[seat] = true
				}
				first = false
				if was, ok := health[u.AllocationSerial]; ok && u.Health < was {
					damaged[seat] = true
				}
				health[u.AllocationSerial] = u.Health
			})
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	if ticks != rehearsalTicks {
		t.Fatalf("the rehearsal ran %d ticks, want %d", ticks, rehearsalTicks)
	}
	if moved != [2]bool{true, true} || completed != [2]bool{true, true} || damaged != [2]bool{true, true} || shots == 0 {
		t.Fatalf("the script did not reach both seats: moved %v, completed a structure %v, took damage %v, ticks with projectiles %d", moved, completed, damaged, shots)
	}

	begin := time.Now()
	got, err := RehearsalDigest(inputs, config)
	elapsed := time.Since(begin)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("RehearsalDigest on %q: %v for %d ticks, digest %x", admittedSkirmishMap, elapsed, rehearsalTicks, got)
	if got != digest || rehearsalDigestOf(t, inputs, config, 1) != digest {
		t.Fatal("the rehearsal digest depends on the presented seat or the run")
	}

	speed, err := content.ParseMutators(map[string]string{"buildSpeed": "2"})
	if err != nil {
		t.Fatal(err)
	}
	faster, fasterConfig := rehearsalRetailMatch(t, SkirmishEntryOptions{Mutators: speed})
	if rehearsalDigestOf(t, faster, fasterConfig, 0) == digest {
		t.Fatal("a build speed mutator did not change the rehearsal digest")
	}
}
