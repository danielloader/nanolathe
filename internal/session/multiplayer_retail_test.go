//go:build retail

package session

import (
	"reflect"
	"testing"

	"github.com/nanolathe-gg/nanolathe/internal/content"
	"github.com/nanolathe-gg/nanolathe/internal/pool"
	"github.com/nanolathe-gg/nanolathe/internal/sim/numeric"
	"github.com/nanolathe-gg/nanolathe/internal/testsupport/retailcat"
	"github.com/nanolathe-gg/nanolathe/internal/units"
)

// Different local seats must produce the same battle through actual entry,
// orders, construction and a terminal commander loss (§16.4). This uses only
// the approved small checksum; RNG counts add an inexpensive fixture check.
func TestTwoHumanPlaytestAgreesAcrossLocalSeatsRetail(t *testing.T) {
	cat, fs := retailcat.Shared(t)
	setup := DirectSkirmishConfig(admittedSkirmishMap)
	setup.RNGSimSeed, setup.RNGCrtSeed = 7, 11
	room := matchTestRoom()
	room.MapSchema = admittedRoomSchema(t, fs, cat, setup.MapName, 2)
	r, err := NewMatchConfigRequest(setup, SkirmishEntryOptions{}, room)
	if err != nil {
		t.Fatal(err)
	}
	r.Seats[1].Role = MatchRoleHuman
	r.Seats[1].HostSeat = MatchHostNone
	r.Seats[1].ComputerKind, r.Seats[1].Difficulty = 0, 0
	r.Seats[1].AIParams = nil
	r.Seats[1].Participant = matchTestID(2)
	config := resolveMatch(t, r)
	inputs, err := FreezeMatchInputs(fs, cat, config, nil)
	if err != nil {
		t.Fatal(err)
	}
	var copies [2]*Session
	for seat := range copies {
		copies[seat], err = NewPlaytestSkirmish(inputs, config, uint8(seat), nil)
		if err != nil {
			t.Fatal(err)
		}
		defer copies[seat].closeAIControllers()
		if err := copies[seat].PrepareGrantedBattle(); err != nil {
			t.Fatal(err)
		}
	}
	compare := func(tick uint32) {
		t.Helper()
		if copies[0].UnitStateChecksum() != copies[1].UnitStateChecksum() {
			t.Fatalf("unit checksum differs at tick %d", tick)
		}
		if copies[0].SimRNG().Draws() != copies[1].SimRNG().Draws() || copies[0].CrtRNG().Draws() != copies[1].CrtRNG().Draws() {
			t.Fatalf("random draw counts differ at tick %d", tick)
		}
		for owner := 0; owner < 2; owner++ {
			a, b := copies[0].Econ.Players[owner], copies[1].Econ.Players[owner]
			if a.Losses != b.Losses || a.CommanderLosses != b.CommanderLosses {
				t.Fatalf("seat %d loss statistics differ at tick %d", owner, tick)
			}
		}
		if copies[0].LocalOwner != 0 || copies[1].LocalOwner != 1 {
			t.Fatal("local seat was overwritten")
		}
		if tick == 0 {
			return // Battle entry has not published its first granted tick.
		}
		// The host can only present the committed frame, including the final
		// one: the relay grants no extra tick after both seats have ended.
		for seat, s := range copies {
			want := s.GetResult()
			countdown := want.Countdown
			if want.Kind == "" {
				countdown = -1 // No end predicate has armed this seat yet.
			}
			got := s.Snapshot.Current().Result
			if got.Ended != want.Ended || got.Kind != want.Kind || got.Tick != want.Tick ||
				got.ArmedTick != want.ArmedTick || got.Countdown != countdown ||
				!reflect.DeepEqual(got.Scores, want.Scores) {
				t.Fatalf("seat %d published result at tick %d: got %+v, want %+v with countdown %d", seat, tick, got, want, countdown)
			}
		}
	}
	compare(0)
	var commanders [2]*units.Unit
	for _, u := range copies[0].Units.IterSliced() {
		if u.Alive && u.Owner < 2 && copies[0].isCommanderForOwner(u) {
			commanders[u.Owner] = u
		}
	}
	for i, u := range commanders {
		if u == nil {
			t.Fatalf("no commander for seat %d", i)
		}
	}
	var position uint64
	issue := func(seat uint8, tick uint32, command SeatCommand) {
		t.Helper()
		position++
		for _, s := range copies {
			if err := s.EnqueueSeatCommand(CommandStamp{Seat: seat, Tick: tick, Position: position}, command); err != nil {
				t.Fatal(err)
			}
		}
	}
	ref := func(u *units.Unit) pool.UnitRef { return pool.UnitRef{Handle: u.Handle, Serial: u.AllocationSerial} }
	for seat, u := range commanders {
		issue(uint8(seat), 1, SeatCommand{Kind: SeatOrder, Order: OrderPayload{Actors: []pool.UnitRef{ref(u)}, Code: 2, Position: CommandPosition{X: u.X + 64<<16, Y: u.Y, Z: u.Z}}})
	}
	built := false
	for tick := uint32(1); tick <= 1200; tick++ {
		if tick == 90 {
			for seat, u := range commanders {
				key := "armsolar"
				if u.Def.CanonicalKey == "corcom" {
					key = "corsolar"
				}
				def, _ := copies[0].Catalog.Unit(key)
				if def == nil {
					t.Fatalf("no %s", key)
				}
				point, ok := playtestBuildSite(copies[seat], u, def)
				if !ok {
					t.Fatalf("no legal build site for seat %d", seat)
				}
				issue(uint8(seat), tick, SeatCommand{Kind: SeatMobileBuild, MobileBuild: MobileBuildPayload{Builder: ref(u), Product: key, Position: point}})
			}
		}
		if tick == 600 {
			issue(1, tick, SeatCommand{Kind: SeatSelfDestruct, SelfDestruct: SelfDestructPayload{Actors: []pool.UnitRef{ref(commanders[1])}}})
		}
		for _, s := range copies {
			if err := s.StepGranted(tick); err != nil {
				t.Fatal(err)
			}
			for _, receipt := range s.DrainCommandReceipts() {
				if receipt.Outcome == CommandRejected {
					t.Fatal(receipt.Diagnostic)
				}
			}
		}
		compare(tick)
		if copies[0].Units.LiveCountForPlayer(0) > 1 && copies[0].Units.LiveCountForPlayer(1) > 1 {
			built = true
		}
		if copies[0].OnlineBattleEnded() || copies[1].OnlineBattleEnded() {
			if !built {
				t.Fatal("no structure was created for both seats")
			}
			if !copies[0].OnlineBattleEnded() || !copies[1].OnlineBattleEnded() || copies[0].GetResult().Kind != "victory" || copies[1].GetResult().Kind != "defeat" {
				t.Fatalf("different terminal outcome: %+v / %+v", copies[0].GetResult(), copies[1].GetResult())
			}
			if copies[0].Econ.Players[1].CommanderLosses != 1 {
				t.Fatal("commander self-destruct did not file its owner's loss")
			}
			for seat := uint8(0); seat < 2; seat++ {
				if !reflect.DeepEqual(copies[0].ResultForSeat(seat), copies[1].ResultForSeat(seat)) {
					t.Fatalf("seat %d results differ across clients", seat)
				}
			}
			t.Logf("two local perspectives agreed through tick %d with construction and commander loss", tick)
			return
		}
	}
	t.Fatal("battle did not end after commander self-destruct")
}

func playtestBuildSite(s *Session, builder *units.Unit, def *content.UnitDef) (CommandPoint, bool) {
	cx, cz := int32(builder.X>>20), int32(builder.Z>>20)
	for z := cz - 8; z <= cz+8; z++ {
		for x := cx - 8; x <= cx+8; x++ {
			result, err := s.PreviewPlacementForCursor(x, z, def, def.FootprintX, def.FootprintZ, builder.Handle)
			if err == nil {
				return CommandPoint{X: numeric.Fixed(int64(def.FootprintX+2*x) << 19), Y: numeric.Fixed(int64(result.SiteHeight) << 16), Z: numeric.Fixed(int64(def.FootprintZ+2*z) << 19)}, true
			}
		}
	}
	return CommandPoint{}, false
}
