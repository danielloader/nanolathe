package session

import (
	"fmt"
	"testing"

	"github.com/nanolathe-gg/nanolathe/internal/gameplay"
	"github.com/nanolathe-gg/nanolathe/internal/orders"
	"github.com/nanolathe-gg/nanolathe/internal/survival"
)

// onlineTestSetup is a lobby's setup with the given teams; every seat takes
// side i&1 of a two-side catalog.
func onlineTestSetup(survivalBattle bool, teams ...uint8) OnlineMatchSetup {
	setup := OnlineMatchSetup{Survival: survivalBattle, MapName: "Great Divide", SimSeed: 7, CRTSeed: 11, SideCount: 2}
	for i, team := range teams {
		setup.Seats = append(setup.Seats, OnlineSeat{Team: team, Side: uint8(i & 1)})
	}
	if survivalBattle {
		setup.SurvivalOptions = SurvivalOptions{Pace: survival.PaceRelentless, NoNaval: true}
	}
	return setup
}

func onlineTestRequest(t *testing.T, setup OnlineMatchSetup) MatchConfigRequest {
	t.Helper()
	builder := orders.DefaultBuilderOptions()
	r, err := NewOnlineMatchRequest(setup, SkirmishEntryOptions{BuilderOptions: &builder}, matchTestRoom())
	if err != nil {
		t.Fatal(err)
	}
	return r
}

// A lobby's request resolves, survives its encoding, and holds one human row
// per seat in slot order with the seat's side, the slot's colour and
// participant, and the team's ally group and shared-victory bit
// (DESIGN_MULTIPLAYER §16.6).
func TestOnlineMatchRequestSkirmishRoundTrip(t *testing.T) {
	for _, teams := range [][]uint8{
		{0, 0},
		{1, 1, 2, 2},
		{1, 1, 1, 2, 2, 2, 3, 0, 5, 5},
	} {
		name := fmt.Sprint(teams)
		r := onlineTestRequest(t, onlineTestSetup(false, teams...))
		c := resolveMatch(t, r)
		payload, err := EncodeMatchConfig(c)
		if err != nil {
			t.Fatal(err)
		}
		decoded, err := DecodeMatchConfig(payload)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if decoded.Digest() != c.Digest() {
			t.Fatalf("%s: the decoded configuration has another digest", name)
		}
		got := decoded.Request()
		if got.SessionKind != MatchOnlineSkirmish || got.RuleName != string(gameplay.Modern) || got.SimulationSeed != 7 || got.CRTSeed != 11 || len(got.Seats) != len(teams) {
			t.Fatalf("%s: kind %d rule %q seeds %d/%d with %d rows", name, got.SessionKind, got.RuleName, got.SimulationSeed, got.CRTSeed, len(got.Seats))
		}
		for i, seat := range got.Seats {
			group := uint8(SkirmishDefaultAllyGroup)
			if teams[i] != OnlineTeamNone {
				group = teams[i] - 1
			}
			mates := 0
			for _, other := range teams {
				if other == teams[i] {
					mates++
				}
			}
			if seat.Role != MatchRoleHuman || seat.Side != uint8(i&1) || seat.Color != uint8(i) || seat.AllyGroup != group ||
				seat.Participant != (MatchParticipantID{byte(i + 1)}) || seat.Nickname != fmt.Sprintf("Player %d", i+1) ||
				seat.SharedVictory != (teams[i] != OnlineTeamNone && mates > 1) || seat.HostSeat != MatchHostNone {
				t.Fatalf("%s: row %d is %+v", name, i, seat)
			}
		}
	}
}

// Online Survival seats its survivors on the Survival team in the
// single-player survivor colours, with the attacker row last as the Survival
// setup writes it, and carries the setup's options (DESIGN_SURVIVAL §4.1).
func TestOnlineMatchRequestSurvivalRoundTrip(t *testing.T) {
	setup := onlineTestSetup(true, 3, 0, 1)
	setup.Seats[2].Side = 0
	r := onlineTestRequest(t, setup)
	c := resolveMatch(t, r)
	payload, err := EncodeMatchConfig(c)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := DecodeMatchConfig(payload)
	if err != nil {
		t.Fatal(err)
	}
	got := decoded.Request()
	if got.SessionKind != MatchOnlineSurvival || got.SurvivalPace != survival.PaceRelentless || !got.SurvivalNoNaval || got.SurvivalNoAir || len(got.Seats) != 4 {
		t.Fatalf("survival fields: kind %d pace %v noAir %v noNaval %v rows %d", got.SessionKind, got.SurvivalPace, got.SurvivalNoAir, got.SurvivalNoNaval, len(got.Seats))
	}
	layout := SurvivalSkirmishConfig("Great Divide", 2, SurvivalOptions{})
	for i, wantSide := range []uint8{0, 1, 0} {
		seat := got.Seats[i]
		if seat.Role != MatchRoleHuman || seat.Side != wantSide || int(seat.Color) != layout.Players[i].Color ||
			int(seat.AllyGroup) != layout.Players[i].AllyGroup || !seat.SharedVictory || seat.Participant != (MatchParticipantID{byte(i + 1)}) {
			t.Fatalf("survivor %d is %+v", i, seat)
		}
	}
	attacker := got.Seats[3]
	want := layout.Players[3]
	if attacker.Role != MatchRoleSurvivalAttacker || int(attacker.Side) != want.Side || int(attacker.Color) != want.Color ||
		attacker.AllyGroup != SkirmishDefaultAllyGroup || attacker.SharedVictory || attacker.Metal != 0 || attacker.Participant != (MatchParticipantID{}) {
		t.Fatalf("attacker row %+v, want the Survival layout's %+v", attacker, want)
	}
}

// The lobby's limits: a seat count outside the game type's range, a team
// above 5, a side the catalog lacks and one team holding every seat are
// refused; the single-player adapter still refuses a second human.
func TestOnlineMatchRequestRefusals(t *testing.T) {
	for _, tc := range []struct {
		name  string
		setup OnlineMatchSetup
	}{
		{"one seat", onlineTestSetup(false, 0)},
		{"eleven seats", onlineTestSetup(false, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0)},
		{"four survivors", onlineTestSetup(true, 0, 0, 0, 0)},
		{"team six", onlineTestSetup(false, 0, 6)},
		{"one team holds every seat", onlineTestSetup(false, 2, 2, 2)},
		{"a side the catalog lacks", func() OnlineMatchSetup {
			s := onlineTestSetup(false, 0, 0)
			s.Seats[1].Side = 2
			return s
		}()},
		{"no side count", func() OnlineMatchSetup {
			s := onlineTestSetup(false, 0, 0)
			s.SideCount = 0
			return s
		}()},
		{"no map", func() OnlineMatchSetup {
			s := onlineTestSetup(false, 0, 0)
			s.MapName = " "
			return s
		}()},
	} {
		if _, err := NewOnlineMatchRequest(tc.setup, SkirmishEntryOptions{}, matchTestRoom()); err == nil {
			t.Fatalf("%s: the setup was accepted", tc.name)
		}
	}
	cfg := DirectSkirmishConfig("Great Divide")
	cfg.Players[1].Controller = SkirmishControllerHuman
	if _, err := NewMatchConfigRequest(cfg, SkirmishEntryOptions{}, matchTestRoom()); err == nil {
		t.Fatal("the single-player adapter accepted two humans")
	}
}

func TestOnlineSetupRows(t *testing.T) {
	if got := onlineTestSetup(false, 0, 0, 0).Rows(); got != 3 {
		t.Fatalf("skirmish rows %d, want 3", got)
	}
	if got := onlineTestSetup(true, 0, 0, 0).Rows(); got != 4 {
		t.Fatalf("survival rows %d, want 4", got)
	}
}
