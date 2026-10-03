package session

import (
	"github.com/nanolathe-gg/nanolathe/internal/frame"
	"github.com/nanolathe-gg/nanolathe/internal/pool"
	"github.com/nanolathe-gg/nanolathe/internal/units"
)

// The simulation half of the local interface (DESIGN_MULTIPLAYER §7.3,
// §16.2 M2-C6).
//
// Selection, the two visited bits, the build page and BigBrother are
// client-side local state keyed by allocation reference (hud.LocalInterface).
// No phase reads any of them. Two of their rules, though, observe the
// simulation at points inside the tick that no publication shows:
//
//   - the unit sweep's selection maintenance drops a selected unit that is not
//     ready at its own visit [04 R-MOV-03 §1 step 7]; and
//   - BigBrother's cycle walks the local player's slice at the sweep tail,
//     before phase 3, testing the same readiness predicate there
//     [04 R-MOV-03 §1][07 R-CAM-01 §12].
//
// A unit can be unready at its visit and ready again by the tail or by the
// end of the tick, so neither observation can be recovered from a committed
// frame. The session therefore records both as facts of the tick
// (frame.InterfaceFacts), which the publication carries and the frame buffer
// retains in tick order until the host's local state consumes them.
//
// Both are pure observations. The visit-time verdict comes from the unit
// world's step-7 observation sink (units.World.SetReadinessObserver), which
// the sweep calls with each visited unit's reference and readiness verdict
// before its own clear; the sink only appends to the staged facts. The tail
// walk reads the local player's slice with the same predicate. Neither writes
// a unit record, draws from an RNG stream or changes a lifetime, so a battle
// whose host consumes facts reaches the same status words, partial
// fingerprint and save as one without (interface_facts_test.go; the M2-C7
// masked lock comparison runs both ways) [I6].

// bigBrotherState is the session's half of the local interface. The name is
// kept from the session-owned BigBrother it replaces because composition.go
// clears enabled at every world rebuild and step.go calls the two phase hooks
// below by their old names (resetBigBrotherEvents, stepBigBrother); both
// files belong to other units.
type bigBrotherState struct {
	// enabled reports that a host's local interface state consumes this
	// battle's facts (SetLocalInterfaceFacts). A world rebuild clears it, so a
	// recomposed or restored battle starts without a consumer until its host
	// attaches one.
	enabled bool
	// sink is the step-7 observer installed on the unit world while enabled,
	// built once so installing it every tick allocates nothing.
	sink units.ReadinessObserver
	// facts is the tick's staged facts, copied into the publication.
	facts frame.InterfaceFacts
	// developerSubject is the developer diagnostics' movement subject, the
	// first unit of the host's selection (SetDeveloperMovementSubject).
	developerSubject pool.Handle
}

// SetLocalInterfaceFacts attaches (or detaches) a host's local interface
// state: while on, every completed tick stages its local-interface facts for
// the local seat in the publication. The host calls it on its own goroutine
// while the simulation is quiescent. It changes no simulation state, no draw
// and no lifetime [I6].
func (s *Session) SetLocalInterfaceFacts(on bool) {
	if s == nil {
		return
	}
	s.bigBrother.enabled = on
	s.syncReadinessObserver()
}

// syncReadinessObserver installs the step-7 sink on the current unit world
// while a host consumes facts and removes it otherwise. It runs whenever the
// consumer changes and at every phase-1 reset, so a unit world replaced under
// a consumer is observed from its first sweep.
func (s *Session) syncReadinessObserver() {
	if s.Units == nil {
		return
	}
	b := &s.bigBrother
	if !b.enabled {
		s.Units.SetReadinessObserver(nil)
		return
	}
	if b.sink == nil {
		b.sink = s.observeReadiness
	}
	s.Units.SetReadinessObserver(b.sink)
}

// observeReadiness is the step-7 sink. A unit not ready at its own visit is a
// unit whose selected bit the step clears, whoever owns it, so every such
// reference is recorded, in visit order; ready verdicts need no record
// [04 R-MOV-03 §1 step 7]. It writes nothing but the staged facts.
func (s *Session) observeReadiness(ref pool.UnitRef, ready bool) {
	if !ready {
		s.bigBrother.facts.Unready = append(s.bigBrother.facts.Unready, ref)
	}
}

// resetBigBrotherEvents is phase 1's leading act and the paused-input
// boundary's: it discards the facts staged for the previous publication, so
// a republication carries none and every tick's facts are delivered once, and
// keeps the step-7 sink installed exactly while a host consumes facts.
func (s *Session) resetBigBrotherEvents() {
	f := &s.bigBrother.facts
	*f = frame.InterfaceFacts{Unready: f.Unready[:0], Ready: f.Ready[:0]}
	s.syncReadinessObserver()
}

// stepBigBrother runs at the unit sweep tail, before phase 3. When a host
// consumes facts it completes the tick's facts: the visit-time verdicts the
// sink recorded during the sweep, and the tail walk's ready units.
func (s *Session) stepBigBrother() {
	b := &s.bigBrother
	if !b.enabled || s.Units == nil {
		return
	}
	f := &b.facts
	f.Owner = s.LocalOwner
	if s.Clock != nil {
		f.Tick = s.Clock.GlobalTick
	}
	// The tail walk: the local player's slice, live units, the shared
	// readiness predicate with its carrier clause, slice order
	// [04 R-MOV-03 §1][07 R-CAM-01 §12].
	f.Ready = f.Ready[:0]
	s.Units.ForEachPlayerSliceLive(int(s.LocalOwner), func(u *units.Unit) {
		if s.Units.SelectionReady(u) {
			f.Ready = append(f.Ready, pool.UnitRef{Handle: u.Handle, Serial: u.AllocationSerial})
		}
	})
	f.Valid = true
}

// publishInterfaceFacts copies the staged facts into the frame's own storage.
func (s *Session) publishInterfaceFacts(dst *frame.Frame) {
	src := &s.bigBrother.facts
	dst.Interface = frame.InterfaceFacts{
		Valid: src.Valid, Tick: src.Tick, Owner: src.Owner,
		Unready: append(dst.Interface.Unready[:0], src.Unready...),
		Ready:   append(dst.Interface.Ready[:0], src.Ready...),
	}
}
