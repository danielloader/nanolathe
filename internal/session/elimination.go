package session

import (
	"fmt"

	"github.com/nanolathe-gg/nanolathe/internal/frame"
	"github.com/nanolathe-gg/nanolathe/internal/mission"
)

// The elimination announcement [01 §7.5 "The elimination announcement"]
// [08 R-CAMP-01 §9 "Elimination"].
//
// The central death handler decrements the victim's owner's live-unit count as
// the last thing it does; when that count reaches **0**, a skirmish session
// (kind 2) draws one value from the **CRT** stream and takes it modulo three to
// pick one of three tails, formats `"%s %s"` with the owner's name and posts
// the result as a message line of class 4 attributed to the owner's slot.
// Campaign sessions post nothing. Multiplayer selects the eight-entry table
// with one CRT draw and posts locally; it sends no message [08 R-CAMP-01 §9].
//
// The draw is on the CRT stream and it happens INSIDE the tick, on the death
// path, so a skirmish elimination advances the CRT stream by exactly one draw
// and leaves the Park-Miller simulation stream untouched. Its position in the
// stream is the contract: the CRT stream is also authoritative for the wind
// interval, the meteor scheduler and the victory-timer arm [01 §7.5], so
// skipping the draw — which this engine did until now — shifts every later CRT
// consumer in any battle where a player is eliminated. The draw is therefore
// taken at the death, never deferred to the publication boundary; only the
// posting of the finished line crosses that boundary [01 §7.7][I4][I6].
//
// It fires once per arrival at zero, not once per battle. Retail has no
// elimination flag and no once-only latch here — the test is the value of the
// counter the handler has just decremented [05 R-SHARE-01 §3] — so a slot that
// is given a unit after its last one died and then loses that one too is
// announced again. That is what the retail handler does, and the position of
// the second draw in the stream is as load-bearing as the first.

// eliminationTails are the three possessive tails the `mod 3` draw selects
// between, in the table order [08 R-CAMP-01 §9] gives them in. They are
// retail's translated string data, reproduced verbatim; the format is
// `"%s %s"` with the owner's name first.
//
// TODO(question): whether the stored strings carry a leading possessive marker
// (so that the formatted line reads "<name>'s forces have been obliterated"
// rather than "<name> forces have been obliterated") is not settled. [08
// R-CAMP-01 §9] calls them "possessive tails" but quotes them without one, and
// the format string it quotes is a bare `"%s %s"`. Reading the three strings
// out of the retail string table would settle it. Nothing but the drawn text
// depends on the answer: the draw, its stream and its position are unaffected.
var eliminationTails = [3]string{
	"forces have been obliterated",
	"forces have gone to a better place",
	"vermin have been exterminated",
}

// onlineEliminationTails is the kind-3 table in its authored order
// [08 R-CAMP-01 §9]. The prototype has no removal/watch controls, so both
// clients execute the draw at the same death boundary (DESIGN_MULTIPLAYER §16.4.1).
var onlineEliminationTails = [8]string{
	"has been obliterated",
	"has been liquidated",
	"has been eradicated",
	"has terminated",
	"has bowed out",
	"has gone to a better place",
	"has been shown the door",
	"has left the scene",
}

// messageClassElimination is the ring class the elimination line is posted
// under [08 R-CAMP-01 §9]. Class 4 survives the `screenchat` filter's
// zero mode, which retains classes 1, 4 and 8 [07 R-HUD-03 §14.4].
const messageClassElimination uint8 = 4

// postsEliminationAnnouncement is the session-kind gate of [08 R-CAMP-01 §9]:
// only a skirmish session posts the line.
//
// Both ordinary and online skirmishes retain the skirmish mission format;
// onlineResults selects the runtime kind-3 table below. The two discriminants
// remain distinct (DESIGN_MULTIPLAYER §6.4, §16.4.1).
func (s *Session) postsEliminationAnnouncement() bool {
	return s != nil && s.Mission != nil && s.Mission.Type == mission.TypeSkirmish
}

// announceElimination is the elimination branch of the central death handler
// [08 R-CAMP-01 §9]. owner is the slot whose live-unit count has just been
// decremented; it runs only when that count is now zero.
//
// Order inside the branch: the draw first, then the format, then the post. The
// draw is unconditional once the branch is entered — an owner with no name or
// a session with no publication boundary still spends it, because the stream
// position is the simulation-visible half and the text is not.
func (s *Session) announceElimination(owner int, tick uint32) {
	if s == nil || owner < 0 || owner >= 10 {
		return
	}
	if !s.postsEliminationAnnouncement() || s.isSurvivalAttacker(owner) {
		return
	}
	// One CRT draw at either site: mask to eight entries online, modulo
	// three in single-player [08 R-CAMP-01 §9][01 §7.2][01 §7.5].
	var tail string
	if s.onlineResults != nil {
		tail = onlineEliminationTails[s.CrtRNG().Rand()&7]
	} else {
		tail = eliminationTails[s.CrtRNG().Uint32n(uint32(len(eliminationTails)))]
	}
	name := ""
	if p := s.playerRecord(owner); p != nil {
		name = p.Name
	}
	line := fmt.Sprintf("%s %s", name, tail)
	if s.publication == nil || s.publication.events == nil {
		return
	}
	// The line crosses the publication boundary as a committed event; the
	// presentation edge owns the ring and the drawing [03 §2.4][I6]. The
	// speaker byte is the owner's slot, which is what selects the colour the
	// column draws the line in [01 §7.5][07 R-HUD-03 §14.3].
	s.publication.events.EmitAnnounce(frame.Event{
		Tick:         tick,
		StatusText:   line,
		StatusClass:  messageClassElimination,
		AnnounceSlot: uint8(owner),
	})
}
