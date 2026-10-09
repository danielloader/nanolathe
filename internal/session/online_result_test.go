package session

import (
	"reflect"
	"testing"

	"github.com/nanolathe-gg/nanolathe/internal/content"
	"github.com/nanolathe-gg/nanolathe/internal/economy"
	"github.com/nanolathe-gg/nanolathe/internal/frame"
	"github.com/nanolathe-gg/nanolathe/internal/mission"
	"github.com/nanolathe-gg/nanolathe/internal/pool"
	"github.com/nanolathe-gg/nanolathe/internal/sim/rng"
	"github.com/nanolathe-gg/nanolathe/internal/units"
)

func onlineResultFixture(t *testing.T, local uint8) (*Session, *content.UnitDef) {
	t.Helper()
	w, def := eliminationFixtureWorld(t)
	s := &Session{
		Units: w, Econ: &economy.Service{}, LocalOwner: local, ViewingOwner: local,
		Mission: &mission.Mission{Type: mission.TypeSkirmish}, State: StateBattle,
		Latch: NewEndLatch(), onlineResults: newOnlineResultState([10]bool{true, true}),
	}
	s.SeedSessionRNG(17, 31)
	for player := 0; player < 2; player++ {
		s.Econ.Players[player] = economy.Player{
			Exists: true, ControllerState: 1, EndGameCountdown: -1,
		}
	}
	return s, def
}

func onlineResultCreate(t *testing.T, s *Session, def *content.UnitDef, owner uint8) pool.Handle {
	t.Helper()
	h, err := s.Units.Create(def, owner, 0, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	return h
}

func onlineResultKill(t *testing.T, s *Session, h pool.Handle) {
	t.Helper()
	s.Units.Destroy(h, units.DeathKilled)
	if !s.Units.FinalizeDeath(h, 0).Freed {
		t.Fatal("fixture death was not finalized")
	}
}

// A created-nothing opponent blocks kind-3 victory, unlike kind 2. Defeat
// still reads only the own live count [08 R-SKIR-01 §3].
func TestOnlineResultNeverCreatedOpponent(t *testing.T) {
	s, def := onlineResultFixture(t, 0)
	onlineResultCreate(t, s, def, 0)
	s.evaluateOnlineSeatResult(0, 0)
	if row := s.onlineResults.seats[0]; row.armed || row.latch.Countdown != -1 {
		t.Fatalf("never-created opponent allowed victory: %+v", row)
	}
	s.evaluateOnlineSeatResult(1, 0)
	if row := s.onlineResults.seats[1]; !row.armed || row.latch.Countdown != 4 || row.result.Kind != "defeat" {
		t.Fatalf("own zero live count did not arm defeat: %+v", row)
	}
	h := onlineResultCreate(t, s, def, 1)
	onlineResultKill(t, s, h)
	s.evaluateOnlineSeatResult(0, 30)
	if row := s.onlineResults.seats[0]; !row.armed || row.latch.Countdown != 4 || row.result.Kind != "victory" {
		t.Fatalf("eliminated opponent did not arm victory: %+v", row)
	}
}

// Tick zero must remain the first arming tick even after a false due and a
// change of terminal path [08 R-TRIG-01 §6]. Every player owns its mirror.
func TestOnlineResultDueMirrorsAndFinalPath(t *testing.T) {
	s, def := onlineResultFixture(t, 0)
	h0 := onlineResultCreate(t, s, def, 0)
	h1 := onlineResultCreate(t, s, def, 1)
	onlineResultKill(t, s, h0)
	s.Econ.Players[0].Mirror[economy.Metal].Production = 7
	s.Econ.Players[1].Mirror[economy.Metal].Production = 11
	s.Econ.Players[1].UpdateTime = 7
	s.Econ.SetEndCondition(s.evaluateOnlineSeatResult)
	if !s.Econ.TickPlayer(0, 0, s.Units, nil) || s.Econ.TickPlayer(1, 0, s.Units, nil) {
		t.Fatal("fixture deadlines did not remain independently phased")
	}
	if p := s.Econ.Players[0]; p.UpdateTime != 30 || p.EndGameCountdown != 4 || p.GameEnded || p.Mirror[economy.Metal].Production != 7 {
		t.Fatalf("due row did not freeze before settlement: %+v", p)
	}
	if p := s.Econ.Players[1]; p.UpdateTime != 7 || p.EndGameCountdown != -1 || p.GameEnded {
		t.Fatalf("due row changed its peer: %+v", p)
	}

	// Restore the defeated player's live count: both predicates are false.
	onlineResultCreate(t, s, def, 0)
	if !s.Econ.TickPlayer(1, 7, s.Units, nil) {
		t.Fatal("peer's independent due did not run")
	}
	if p := s.Econ.Players[1]; p.UpdateTime != 37 || p.PassProduced[economy.Metal] != 11 || p.Mirror[economy.Metal].Production != 0 || p.EndGameCountdown != -1 {
		t.Fatalf("one seat's countdown prevented the peer settling: %+v", p)
	}
	before := s.ResultForSeat(0)
	s.Econ.TickPlayer(0, 30, s.Units, nil)
	if got := s.ResultForSeat(0); !reflect.DeepEqual(got, before) {
		t.Fatalf("false due changed pending result: before=%+v after=%+v", before, got)
	}

	onlineResultKill(t, s, h1)
	for i, want := range []int16{3, 2, 1, 0, -1} {
		tick := uint32(60 + 30*i)
		s.evaluateOnlineSeatResult(0, tick)
		row := s.onlineResults.seats[0]
		if row.latch.Countdown != want || row.result.Ended != (want < 0) || row.result.ArmedTick != 0 || row.result.Kind != "victory" {
			t.Fatalf("true due %d: %+v", tick, row)
		}
	}
	if !s.onlineSeatEnded(0) || s.onlineSeatEnded(1) || s.onlineBattleEnded() || s.State != StateBattle {
		t.Fatal("local latch stopped the shared battle or ended its peer")
	}
	if s.Latch != NewEndLatch() || s.result.Ended || s.Econ.Players[1].GameEnded || s.Econ.Players[1].EndGameCountdown != -1 {
		t.Fatal("online ending wrote the single-player latch or peer's gates")
	}
	if got := s.GetResult(); !got.Ended || got.Tick != 180 || got.Kind != "victory" {
		t.Fatalf("local projection: %+v", got)
	}
	for i := 0; i < 6; i++ {
		s.evaluateOnlineSeatResult(1, uint32(200+30*i))
	}
	if !s.onlineBattleEnded() || !s.Econ.Players[0].GameEnded || !s.Econ.Players[1].GameEnded || s.State != StateBattle {
		t.Fatal("both latches must report shared completion without taking the caller's transition")
	}
	final := s.ResultForSeat(0)
	s.evaluateOnlineSeatResult(0, 999)
	s.stepOnlineNoHumanEnd(999)
	if !reflect.DeepEqual(final, s.ResultForSeat(0)) {
		t.Fatal("terminal row was rewritten")
	}
}

func TestOnlineResultNoHumanTailSharesDueCountdown(t *testing.T) {
	for _, dueFirst := range []bool{false, true} {
		s, def := onlineResultFixture(t, 1)
		for owner := uint8(0); owner < 2; owner++ {
			h := onlineResultCreate(t, s, def, owner)
			onlineResultKill(t, s, h)
			if dueFirst {
				s.evaluateOnlineSeatResult(int(owner), 0)
			}
		}
		last := uint32(5)
		if dueFirst {
			last = 4 // One true due precedes the tail on tick zero.
		}
		for tick := uint32(0); tick <= last; tick++ {
			s.stepOnlineNoHumanEnd(tick)
			if s.onlineBattleEnded() != (tick == last) {
				t.Fatalf("dueFirst=%v tick=%d shared terminal=%v", dueFirst, tick, s.onlineBattleEnded())
			}
		}
		for player := uint8(0); player < 2; player++ {
			r := s.ResultForSeat(player)
			if !r.Ended || r.Kind != "defeat" || r.Draw || r.WinnerTeam != -1 || len(r.Winners) != 0 || r.Tick != last || r.ArmedTick != 0 || r.Countdown != -1 {
				t.Fatalf("mutual wipe result for %d: %+v", player, r)
			}
			if !reflect.DeepEqual(r.Losers, []int{100, 101}) {
				t.Fatalf("mutual wipe losers: %v", r.Losers)
			}
		}
	}
}

func TestOnlineResultNoHumanTailRetainsFalseCountdown(t *testing.T) {
	s, def := onlineResultFixture(t, 0)
	// A human that has never created anything counts as live [08 R-SESS-01 §1].
	for tick := uint32(0); tick < 8; tick++ {
		s.stepOnlineNoHumanEnd(tick)
	}
	if s.onlineResults.seats[0].armed || s.onlineResults.seats[1].armed {
		t.Fatal("never-created humans triggered the no-human tail")
	}
	for owner := uint8(0); owner < 2; owner++ {
		onlineResultKill(t, s, onlineResultCreate(t, s, def, owner))
	}
	s.stepOnlineNoHumanEnd(10)
	h := onlineResultCreate(t, s, def, 1)
	before := *s.onlineResults
	s.stepOnlineNoHumanEnd(11)
	if !reflect.DeepEqual(before, *s.onlineResults) {
		t.Fatal("false no-human tail changed a countdown")
	}
	onlineResultKill(t, s, h)
	s.stepOnlineNoHumanEnd(12)
	if s.onlineResults.seats[0].latch.Countdown != 3 || s.onlineResults.seats[1].latch.Countdown != 3 {
		t.Fatal("no-human tail restarted instead of continuing")
	}
}

// Another seat's result does not mutate the row predicates counted by the
// no-human site [08 R-SESS-01 §1]. It must not accelerate this seat's due.
func TestOnlineResultLiveWinnerDoesNotAcceleratePeer(t *testing.T) {
	s, def := onlineResultFixture(t, 0)
	onlineResultCreate(t, s, def, 0)
	onlineResultKill(t, s, onlineResultCreate(t, s, def, 1))
	for i := 0; i < 6; i++ {
		s.evaluateOnlineSeatResult(0, uint32(i*30))
	}
	s.evaluateOnlineSeatResult(1, 151)
	if !s.onlineSeatEnded(0) || s.onlineSeatEnded(1) || s.onlineBattleEnded() {
		t.Fatal("fixture must have an earlier winner and a pending loser")
	}
	before := s.ResultForSeat(1)
	for tick := uint32(151); tick < 181; tick++ {
		s.stepOnlineNoHumanEnd(tick)
	}
	if !reflect.DeepEqual(before, s.ResultForSeat(1)) {
		t.Fatal("winner's ending latch accelerated the peer's countdown")
	}
	for i := 0; i < 5; i++ {
		s.evaluateOnlineSeatResult(1, uint32(181+i*30))
	}
	if !s.onlineBattleEnded() || s.ResultForSeat(1).Tick != 301 {
		t.Fatal("peer did not finish on its sixth true due")
	}
}

func TestOnlineResultIndependentOfLocalProjection(t *testing.T) {
	a, da := onlineResultFixture(t, 0)
	b, db := onlineResultFixture(t, 1)
	for i, s := range []*Session{a, b} {
		def := []*content.UnitDef{da, db}[i]
		onlineResultCreate(t, s, def, 0)
		onlineResultKill(t, s, onlineResultCreate(t, s, def, 1))
		for tick := uint32(0); tick <= 150; tick += 30 {
			for player := 0; player < 2; player++ {
				s.evaluateOnlineSeatResult(player, tick)
			}
			s.stepOnlineNoHumanEnd(tick)
		}
	}
	if !reflect.DeepEqual(a.onlineResults, b.onlineResults) || a.Econ.Players != b.Econ.Players {
		t.Fatal("local/viewing owner changed authoritative seat results")
	}
	if a.GetResult().Kind != "victory" || b.GetResult().Kind != "defeat" {
		t.Fatal("GetResult did not select the local row")
	}
	if a.SimRNG().Draws() != 0 || a.CrtRNG().Draws() != 0 || b.SimRNG().Draws() != 0 || b.CrtRNG().Draws() != 0 {
		t.Fatal("result evaluation drew randomness")
	}
}

func TestOnlineResultCopiesAndAbsentRows(t *testing.T) {
	s, _ := onlineResultFixture(t, 0)
	s.onlineResults.seats[0].result = Result{
		Winners: []int{100}, Losers: []int{101}, Scores: []frame.ResultScore{{Name: "original"}},
		Survival: &frame.SurvivalResult{Score: 7},
	}
	before := s.ResultForSeat(0)
	for _, r := range []Result{s.ResultForSeat(0), s.GetResult()} {
		r.Winners[0], r.Losers[0], r.Scores[0].Name, r.Survival.Score = 9, 8, "changed", 0
	}
	if !reflect.DeepEqual(before, s.ResultForSeat(0)) {
		t.Fatal("presentation result retained mutable aliases")
	}
	for _, player := range []uint8{2, 9, 10, 255} {
		if !reflect.DeepEqual(s.ResultForSeat(player), Result{}) || s.onlineSeatEnded(int(player)) {
			t.Fatalf("absent player %d exposed a result", player)
		}
		s.evaluateOnlineSeatResult(int(player), 7)
	}
	if s.onlineSeatEnded(-1) {
		t.Fatal("negative player ended")
	}
	s.evaluateOnlineSeatResult(-1, 7)
	for _, absent := range []*Session{nil, {}, {onlineResults: newOnlineResultState([10]bool{})}} {
		absent.evaluateOnlineSeatResult(0, 0)
		absent.stepOnlineNoHumanEnd(0)
		if absent.onlineBattleEnded() || absent.onlineSeatEnded(0) || !reflect.DeepEqual(absent.ResultForSeat(0), Result{}) {
			t.Fatal("absent online state invented a result")
		}
	}
}

func TestOnlineResultCommanderSweepAfterLocalEnd(t *testing.T) {
	s, extra := newLobbyEndRuleSession(t, int(CommanderDeathEnds), true)
	s.LocalOwner = 0
	s.onlineResults = newOnlineResultState([10]bool{true, true})
	commander := pool.Handle(commanderHandles(s)[0][0])
	onlineResultKill(t, s, commander)
	for i := 0; i < 6; i++ {
		s.evaluateOnlineSeatResult(0, uint32(i*30))
	}
	if !s.GetResult().Ended || s.onlineBattleEnded() {
		t.Fatal("fixture must have one locally terminal seat")
	}
	commander = pool.Handle(commanderHandles(s)[1][0])
	onlineResultKill(t, s, commander)
	s.processPendingCommanderDeaths(160)
	u := s.Units.Unit(extra)
	if u == nil || !u.Alive || !u.Dying || u.Health > 0 || u.LastDamageSide != 1 {
		t.Fatalf("other owner's ordinary damage sweep stopped after local result: %+v", u)
	}
	if s.pendingCommanderDeaths[1] {
		t.Fatal("commander sweep left its pending marker set")
	}
	health := u.Health
	s.processPendingCommanderDeaths(161)
	if u.Health != health {
		t.Fatal("owner sweep repeated")
	}
}

func TestOnlineEliminationAnnouncementUsesEightEntryTable(t *testing.T) {
	// Authored seeds below select each index on the first CRT draw.
	wantTails := [...]string{
		"has been obliterated", "has been liquidated", "has been eradicated", "has terminated",
		"has bowed out", "has gone to a better place", "has been shown the door", "has left the scene",
	}
	for index, seed := range [...]uint32{3, 1, 6, 4, 9, 2, 0, 10} {
		s, w, _ := eliminationAnnouncementSession(t, mission.TypeSkirmish)
		s.onlineResults = newOnlineResultState([10]bool{true, true})
		s.SeedSessionRNG(17, seed)
		if draws := killOneOwnerUnit(t, s, w, 1, 10); draws != 0 {
			t.Fatal("non-eliminating death drew randomness")
		}
		predict := rng.CRTFromState(seed)
		if int(predict.Rand()&7) != index {
			t.Fatal("fixture seed no longer selects its authored index")
		}
		if draws := killOneOwnerUnit(t, s, w, 1, 11); draws != 1 || s.CrtRNG().State != predict.State || s.SimRNG().Draws() != 0 {
			t.Fatal("online elimination did not spend exactly one CRT draw")
		}
		events := announceEvents(s)
		if len(events) != 1 || events[0].StatusText != "Vermin "+wantTails[index] || events[0].StatusClass != 4 || events[0].AnnounceSlot != 1 || events[0].Tick != 11 {
			t.Fatalf("online elimination event: %+v", events)
		}
	}
}

func TestOnlineEliminationDrawWithoutPublication(t *testing.T) {
	s, w, def := eliminationAnnouncementSession(t, mission.TypeSkirmish)
	s.onlineResults = newOnlineResultState([10]bool{true, true})
	s.publication = nil
	killOneOwnerUnit(t, s, w, 1, 10)
	if killOneOwnerUnit(t, s, w, 1, 11) != 1 {
		t.Fatal("missing publication suppressed the draw")
	}
	onlineResultCreate(t, s, def, 1)
	if killOneOwnerUnit(t, s, w, 1, 12) != 1 {
		t.Fatal("a second emptying suppressed its draw")
	}
}
