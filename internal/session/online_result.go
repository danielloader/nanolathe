package session

// The prototype is explicitly two hostile humans, without watching, removal,
// shared victory or respawn (DESIGN_MULTIPLAYER §16.4.1). Each row below is the
// countdown and result that its owner's machine would retain [08 R-TRIG-01 §6].
type onlineSeatResult struct {
	present bool
	latch   EndLatch
	armed   bool // Tick zero is a valid first true due, not an unarmed sentinel.
	result  Result
}

type onlineResultState struct {
	seats [10]onlineSeatResult
}

func newOnlineResultState(seats [10]bool) *onlineResultState {
	state := &onlineResultState{}
	for player, present := range seats {
		if present {
			state.seats[player] = onlineSeatResult{present: true, latch: NewEndLatch()}
		}
	}
	return state
}

// evaluateOnlineSeatResult is called inside this player's settlement due,
// after its deadline advances and before settlement gates [08 R-TRIG-01 §6].
// The caller owns the due; this method adds no deadline or catch-up loop.
func (s *Session) evaluateOnlineSeatResult(player int, tick uint32) {
	if s == nil || s.onlineResults == nil || s.Units == nil || s.Econ == nil ||
		player < 0 || player >= len(s.onlineResults.seats) {
		return
	}
	row := &s.onlineResults.seats[player]
	if !row.present || row.latch.IsEnding() {
		return
	}
	s.processPendingCommanderDeaths(tick)
	// Defeat wins a simultaneous wipe. Otherwise the closed, hostile prototype
	// needs every opponent to have created something and now own no live unit
	// [08 R-SKIR-01 §3]. In particular, kind 2's zero-live skip is insufficient.
	if s.Units.LiveCountForPlayer(player) == 0 {
		s.advanceOnlineSeatResult(player, tick, false)
		return
	}
	for other := range s.onlineResults.seats {
		if other == player || !s.onlineResults.seats[other].present {
			continue
		}
		if s.Units.CreatedCountForPlayer(other) == 0 || s.Units.LiveCountForPlayer(other) != 0 {
			return // A false due retains both the countdown and its pending view.
		}
	}
	s.advanceOnlineSeatResult(player, tick, true)
}

// stepOnlineNoHumanEnd is the after-player-loop site, once per tick. It shares
// each seat's countdown with the won/lost due paths [08 R-SESS-01 §1]
// [08 R-TRIG-01 §6]; it is not another timer and can follow a true due this tick.
func (s *Session) stepOnlineNoHumanEnd(tick uint32) {
	if s == nil || s.onlineResults == nil || s.Units == nil || s.Econ == nil ||
		CommanderDeathMode(s.Skirmish.CommanderDeath) == CommanderDeathDeathmatch {
		return
	}
	for player := range s.onlineResults.seats {
		row := &s.onlineResults.seats[player]
		p := &s.Econ.Players[player]
		// An ending latch is not a player-record mutation. A live winner still
		// counts here; all-seat completion is checked separately [08 R-SESS-01 §1].
		if row.present && p.Exists && p.Side != 10 &&
			p.ControllerState == 1 && !p.Watcher &&
			(s.Units.LiveCountForPlayer(player) != 0 || s.Units.CreatedCountForPlayer(player) == 0) {
			return
		}
	}
	for player := range s.onlineResults.seats {
		row := &s.onlineResults.seats[player]
		if row.present && !row.latch.IsEnding() {
			s.advanceOnlineSeatResult(player, tick, false)
		}
	}
}

// advanceOnlineSeatResult refreshes presentation metadata on each true path;
// only the path crossing below zero writes terminal bits [08 R-TRIG-01 §6].
func (s *Session) advanceOnlineSeatResult(player int, tick uint32, victory bool) {
	row := &s.onlineResults.seats[player]
	if !row.armed {
		row.armed = true
		row.result.ArmedTick = tick
	}
	var ended bool
	if victory {
		ended = row.latch.AdvanceWin(true)
	} else {
		ended = row.latch.AdvanceLose(true)
	}
	p := &s.Econ.Players[player]
	p.GameEnded = row.latch.IsEnding()
	p.EndGameCountdown = int32(row.latch.Countdown)

	winner, losers := s.resultTeamsForOwner(player, victory)
	scores := s.collectScores(winner, false)
	reason := ReasonAllUnits
	if CommanderDeathMode(s.Skirmish.CommanderDeath) != CommanderDeathContinues {
		reason = ReasonCommanderDeath
	}
	kind := "defeat"
	if victory {
		kind = "victory"
	}
	row.result = Result{
		Ended: ended, Kind: kind, WinnerTeam: winner,
		Winners: resultWinnersFor(winner, false), Losers: losers, Reason: reason,
		ArmedTick: row.result.ArmedTick, Countdown: row.latch.Countdown,
		Scores: scores, ColumnMaxima: resultColumnMaxima(scores),
	}
	if ended {
		row.result.Tick = tick
	}
}

func (s *Session) onlineSeatEnded(player int) bool {
	return s != nil && s.onlineResults != nil && player >= 0 && player < len(s.onlineResults.seats) &&
		s.onlineResults.seats[player].present && s.onlineResults.seats[player].latch.IsEnding()
}

// onlineBattleEnded controls shared simulation termination. GetResult's local
// projection must not stop the other human's world (DESIGN_MULTIPLAYER §16.4.1).
func (s *Session) onlineBattleEnded() bool {
	if s == nil || s.onlineResults == nil {
		return false
	}
	present := false
	for player := range s.onlineResults.seats {
		row := &s.onlineResults.seats[player]
		if row.present {
			present = true
			if !row.latch.IsEnding() {
				return false
			}
		}
	}
	return present
}

// ResultForSeat returns a detached pending or final online result, or zero for
// an absent seat or an ordinary session (DESIGN_MULTIPLAYER §16.4.1).
func (s *Session) ResultForSeat(player uint8) Result {
	if s == nil || s.onlineResults == nil || int(player) >= len(s.onlineResults.seats) ||
		!s.onlineResults.seats[player].present {
		return Result{}
	}
	r := copyResult(s.onlineResults.seats[player].result)
	r.Survival = r.Survival.Copy()
	return r
}
