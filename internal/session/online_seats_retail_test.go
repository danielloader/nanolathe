//go:build retail

package session

import (
	"bytes"
	"testing"
	"time"

	"github.com/nanolathe-gg/nanolathe/internal/content"
	"github.com/nanolathe-gg/nanolathe/internal/survival"
	"github.com/nanolathe-gg/nanolathe/internal/testsupport/retailcat"
	"github.com/nanolathe-gg/nanolathe/internal/units"
)

// onlineRetailMatch resolves and freezes a lobby setup on the reference
// install, the way a seat composes at Ready (DESIGN_MULTIPLAYER §16.6).
func onlineRetailMatch(t *testing.T, setup OnlineMatchSetup) (*content.SimulationInputs, EffectiveMatchConfig) {
	t.Helper()
	cat, fs := retailcat.Shared(t)
	setup.SideCount = len(OnlineSides(cat))
	room := matchTestRoom()
	schema, err := OnlineMapSchema(fs, cat, setup.MapName, setup.Rows())
	if err != nil {
		t.Fatal(err)
	}
	room.MapSchema = schema
	r, err := NewOnlineMatchRequest(setup, SkirmishEntryOptions{}, room)
	if err != nil {
		t.Fatal(err)
	}
	config := resolveMatch(t, r)
	inputs, err := FreezeMatchInputs(fs, cat, config, nil)
	if err != nil {
		t.Fatal(err)
	}
	return inputs, config
}

func onlineRetailSetup(survivalBattle bool, pace survival.Pace, teams ...uint8) OnlineMatchSetup {
	setup := OnlineMatchSetup{Survival: survivalBattle, MapName: admittedSkirmishMap, SimSeed: 7, CRTSeed: 11}
	for i, team := range teams {
		setup.Seats = append(setup.Seats, OnlineSeat{Team: team, Side: uint8(i & 1), Color: uint8(i)})
	}
	if survivalBattle {
		setup.SurvivalOptions = SurvivalOptions{Pace: pace}
	}
	return setup
}

func onlineRetailSession(t *testing.T, inputs *content.SimulationInputs, config EffectiveMatchConfig, seat uint8) *Session {
	t.Helper()
	s, err := NewPlaytestSkirmish(inputs, config, seat, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.closeAIControllers)
	if err := s.PrepareGrantedBattle(); err != nil {
		t.Fatal(err)
	}
	return s
}

// The capacity is the largest start-position count of a map's network
// schemas, capped at ten, and the schema helper selects the schema
// admission requires for a row count.
func TestOnlineMapCapacityRetail(t *testing.T) {
	cat, fs := retailcat.Shared(t)
	for name, want := range map[string]int{admittedSkirmishMap: 10, "lava alley": 10, "luschinfloggen": 6} {
		got, err := OnlineMapCapacity(cat, name)
		if err != nil || got != want {
			t.Fatalf("%s: capacity %d, %v; want %d", name, got, err, want)
		}
	}
	if _, err := OnlineMapCapacity(cat, "no such map"); err == nil {
		t.Fatal("an unknown map has a capacity")
	}
	for _, rows := range []int{2, 4, 10} {
		got, err := OnlineMapSchema(fs, cat, admittedSkirmishMap, rows)
		if err != nil || got != admittedRoomSchema(t, fs, cat, admittedSkirmishMap, rows) {
			t.Fatalf("%d rows: schema %d, %v", rows, got, err)
		}
	}
	if sides := OnlineSides(cat); len(sides) != 2 || sides[0] != "ARM" || sides[1] != "CORE" {
		t.Fatalf("sides %q", sides)
	}
}

// Teammates share current sight: every coverage stamp reaches both members'
// grids, so a teammate sees its ally's commander, while the other team keeps
// its own picture (DESIGN_MULTIPLAYER §6.7 "Allied sight").
func TestOnlineTeamsShareSightRetail(t *testing.T) {
	inputs, config := onlineRetailMatch(t, onlineRetailSetup(false, 0, 1, 1, 2, 2))
	s := onlineRetailSession(t, inputs, config, 3)
	for tick := uint32(1); tick <= 31; tick++ {
		if err := s.StepGranted(tick); err != nil {
			t.Fatal(err)
		}
	}
	if !bytes.Equal(s.Vis.ByteGrid(0), s.Vis.ByteGrid(1)) || !bytes.Equal(s.Vis.ByteGrid(2), s.Vis.ByteGrid(3)) {
		t.Fatal("teammates hold different current sight")
	}
	if bytes.Equal(s.Vis.ByteGrid(0), s.Vis.ByteGrid(2)) {
		t.Fatal("opposing teams hold the same current sight")
	}
	var commanders [4]*units.Unit
	for _, u := range s.Units.IterSliced() {
		if u.Alive && u.Owner < 4 && commanders[u.Owner] == nil {
			commanders[u.Owner] = u
		}
	}
	for owner, u := range commanders {
		if u == nil {
			t.Fatalf("seat %d has no unit", owner)
		}
		mate := owner ^ 1
		if !s.IsUnitVisible(mate, u) {
			t.Fatalf("seat %d does not see its teammate %d's commander", mate, owner)
		}
	}
	// Each team's grid is exactly its own members' coverage; ashap plateau
	// starts the seats far enough apart that no commander sees an opponent.
	for owner, u := range commanders {
		for _, enemy := range []int{2, 3} {
			if owner < 2 && s.IsUnitVisible(enemy, u) {
				t.Fatalf("enemy seat %d sees seat %d's commander", enemy, owner)
			}
		}
	}
}

// Four seats on two teams rehearse alike whichever seat a composition
// presents, and the script moves every seat's commander (§16.7).
func TestOnlineRehearsalAgreesFourSeatsRetail(t *testing.T) {
	inputs, config := onlineRetailMatch(t, onlineRetailSetup(false, 0, 1, 1, 2, 2))
	var start [4][2]int64
	var moved [4]bool
	ticks := 0
	want, err := rehearse(inputs, config, 0, func(s *Session) {
		ticks++
		for seat := 0; seat < 4; seat++ {
			first := true
			s.Units.ForEachPlayerSliceLive(seat, func(u *units.Unit) {
				if !first {
					return
				}
				first = false
				at := [2]int64{int64(u.X), int64(u.Z)}
				if ticks == 1 {
					start[seat] = at
				}
				moved[seat] = moved[seat] || at != start[seat]
			})
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	if ticks != rehearsalTicks || moved != [4]bool{true, true, true, true} {
		t.Fatalf("ran %d ticks; moved %v", ticks, moved)
	}
	begin := time.Now()
	if got := rehearsalDigestOf(t, inputs, config, 3); got != want {
		t.Fatalf("seat 3's rehearsal digest %x, want %x", got, want)
	}
	t.Logf("four-seat rehearsal on %q: %v", admittedSkirmishMap, time.Since(begin))
}

// Three human survivors online: compositions presenting different seats
// agree tick for tick through the first wave, the waves hunt the survivor
// team, and no seat arms a result (DESIGN_MULTIPLAYER §16.6, DESIGN_SURVIVAL
// §6.7, §8).
func TestOnlineSurvivalThreeSurvivorsRetail(t *testing.T) {
	setup := onlineRetailSetup(true, survival.PaceRelentless, 0, 0, 0)
	inputsA, config := onlineRetailMatch(t, setup)
	inputsB, _ := onlineRetailMatch(t, setup)
	a := onlineRetailSession(t, inputsA, config, 0)
	b := onlineRetailSession(t, inputsB, config, 2)
	if a.Survival == nil || a.Survival.attacker != 3 || len(a.Survival.team) != 3 {
		t.Fatalf("survival state %+v", a.Survival)
	}
	if a.onlineResults.seats[3].present {
		t.Fatal("the attacker row is a present seat")
	}
	end := a.Survival.tuning.FirstWaveDelay + a.Survival.tuning.WarningTime + 2*a.Survival.tuning.RetargetEvery
	spawned := uint32(0)
	for tick := uint32(1); tick <= end; tick++ {
		for _, s := range []*Session{a, b} {
			if err := s.StepGranted(tick); err != nil {
				t.Fatal(err)
			}
		}
		if a.UnitStateChecksum() != b.UnitStateChecksum() || a.SimRNG().Draws() != b.SimRNG().Draws() || a.CrtRNG().Draws() != b.CrtRNG().Draws() {
			t.Fatalf("the compositions diverged at tick %d", tick)
		}
		if spawned == 0 && a.Survival.firstWaveSpawned() {
			spawned = tick
		}
		for seat := uint8(0); seat < 3; seat++ {
			if r := a.ResultForSeat(seat); r.Kind != "" {
				t.Fatalf("seat %d armed %q at tick %d", seat, r.Kind, tick)
			}
		}
	}
	if spawned == 0 || a.Units.CreatedCountForPlayer(3) == 0 {
		t.Fatal("the first wave did not spawn")
	}
	targeted := 0
	for i, u := range a.Survival.units {
		if u.target != b.Survival.units[i].target {
			t.Fatalf("wave unit %d targets differ across compositions", i)
		}
		if target := a.Units.Unit(u.target); u.target != 0 && target != nil {
			if !a.Survival.onTeam(target.Owner) {
				t.Fatalf("wave unit %d hunts owner %d", i, target.Owner)
			}
			targeted++
		}
	}
	if targeted == 0 {
		t.Fatal("no wave unit was sent at a survivor")
	}
	t.Logf("first wave spawned by tick %d; %d of %d wave units hunting survivors", spawned, targeted, len(a.Survival.units))
}

// The Survival rehearsal runs on through the first wave and agrees across
// presented seats (§16.7).
func TestOnlineRehearsalSurvivalAgreesRetail(t *testing.T) {
	inputs, config := onlineRetailMatch(t, onlineRetailSetup(true, survival.PaceRelentless, 0, 0, 0))
	var ticks uint32
	var waved bool
	want, err := rehearse(inputs, config, 0, func(s *Session) {
		ticks++
		waved = s.Units.CreatedCountForPlayer(int(s.Survival.attacker)) > 0
	})
	if err != nil {
		t.Fatal(err)
	}
	tuning := survival.DefaultTuning(survival.PaceRelentless)
	if !waved || ticks <= tuning.FirstWaveDelay+tuning.WarningTime || ticks > tuning.FirstWaveDelay+tuning.WarningTime+rehearsalWaveTicks {
		t.Fatalf("ran %d ticks; first wave created %v", ticks, waved)
	}
	begin := time.Now()
	if got := rehearsalDigestOf(t, inputs, config, 2); got != want {
		t.Fatalf("seat 2's rehearsal digest %x, want %x", got, want)
	}
	t.Logf("three-survivor rehearsal (relentless) on %q: %v for %d ticks", admittedSkirmishMap, time.Since(begin), ticks)
}
