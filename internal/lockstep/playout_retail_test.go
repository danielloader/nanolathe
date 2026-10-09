//go:build retail

package lockstep

import (
	"errors"
	"io"
	"testing"
	"time"

	"github.com/nanolathe-gg/nanolathe/internal/pool"
	"github.com/nanolathe-gg/nanolathe/internal/relay"
	"github.com/nanolathe-gg/nanolathe/internal/session"
)

func newPacedRetailPair(t *testing.T) ([2]*session.Session, *LocalDriver, *testClient, *time.Time) {
	t.Helper()
	inputs, cfg := playtestInputs(t)
	var pair [2]*session.Session
	for seat := range pair {
		s, err := session.NewPlaytestSkirmish(inputs, cfg, uint8(seat), nil)
		if err != nil {
			t.Fatal(err)
		}
		if err := s.PrepareGrantedBattle(); err != nil {
			t.Fatal(err)
		}
		pair[seat] = s
	}
	c := newTestClient()
	d, err := NewPacedDriver(pair[0], c)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = d.Close() })
	now := time.Unix(0, 0)
	d.playout.now = func() time.Time { return now }
	return pair, d, c, &now
}

func TestPacedDriverAppliesOnlySealedTickRetail(t *testing.T) {
	pair, d, c, now := newPacedRetailPair(t)
	s, reference := pair[0], pair[1]
	var actor pool.Handle
	for _, u := range s.Units.IterSliced() {
		if u.Alive && u.Owner == 0 {
			actor = u.Handle
			break
		}
	}
	before := s.UnitStateChecksum()
	seq, err := d.Submit(session.HumanCommand{Kind: session.HumanStop, Stop: session.HumanStopCommand{Handles: []pool.Handle{actor}}})
	if err != nil || seq != 1 || len(c.commands) != 1 {
		t.Fatalf("submission was delayed: %d / %v", seq, err)
	}
	if before != s.UnitStateChecksum() || s.Clock.GlobalTick != 0 {
		t.Fatal("submission mutated the session optimistically")
	}
	command, err := session.DecodeSeatCommand(session.OnlineCommand, c.commands[0])
	if err != nil {
		t.Fatal(err)
	}
	if err := reference.EnqueueSeatCommand(session.CommandStamp{Seat: 0, Tick: 1, Position: 1}, command); err != nil {
		t.Fatal(err)
	}
	d.reads <- localRead{grant: relay.LocalGrant{Tick: 1, Position: 1, Commands: []relay.LocalCommand{{Seat: 0, Sequence: seq, Position: 1, Payload: c.commands[0]}}}}
	if advanced, err := d.Pump(); advanced || err != nil {
		t.Fatalf("single startup grant bypassed reserve: %v / %v", advanced, err)
	}
	for tick := uint32(2); tick <= 32; tick++ {
		d.reads <- localRead{grant: relay.LocalGrant{Tick: tick, Position: 1}}
	}
	// One host step per normal interval with a deep queue owes 33 Hz catch-up,
	// so some pumps run two ticks; each still matches one StepGranted per tick.
	for executed := uint32(0); executed < 32; {
		*now = now.Add(playoutInterval)
		if advanced, err := d.Pump(); !advanced || err != nil || s.Clock.GlobalTick <= executed || s.Clock.GlobalTick > executed+uint32(playoutBurstLimit) {
			t.Fatalf("granted ticks after %d: %v / %v, executed %d", executed, advanced, err, s.Clock.GlobalTick)
		}
		if advanced, err := d.Pump(); advanced || err != nil {
			t.Fatalf("second call at unchanged time advanced: %v / %v", advanced, err)
		}
		for executed < s.Clock.GlobalTick {
			executed++
			if err := reference.StepGranted(executed); err != nil {
				t.Fatal(err)
			}
			ack := c.acks[executed-1]
			if ack.tick != executed || ack.ended || (executed%30 == 0 && ack.hash != reference.UnitStateChecksum()) || (executed%30 != 0 && ack.hash != [32]byte{}) {
				t.Fatalf("incorrect tick/checksum acknowledgment: %+v", ack)
			}
		}
		if s.UnitStateChecksum() != reference.UnitStateChecksum() || s.SimRNG().Draws() != reference.SimRNG().Draws() || s.CrtRNG().Draws() != reference.CrtRNG().Draws() {
			t.Fatalf("paced replica differs from one-pump-per-tick reference at %d", executed)
		}
	}
	if len(c.acks) != 32 {
		t.Fatal("did not acknowledge each executed tick once")
	}
	// Both an explicit close and a failed ACK must freeze subsequent grants.
	d.reads <- localRead{grant: relay.LocalGrant{Tick: 33, Position: 1}}
	d.reads <- localRead{grant: relay.LocalGrant{Tick: 34, Position: 1}}
	c.ackError = io.ErrClosedPipe
	*now = now.Add(playoutInterval)
	if advanced, err := d.Pump(); !advanced || !errors.Is(err, io.ErrClosedPipe) {
		t.Fatalf("failed ACK lost the executed tick or its error: %v / %v", advanced, err)
	}
	before = s.UnitStateChecksum()
	if advanced, err := d.Pump(); advanced || !errors.Is(err, io.ErrClosedPipe) || s.Clock.GlobalTick != 33 || before != s.UnitStateChecksum() || len(d.reads) != 1 {
		t.Fatalf("failed ACK driver consumed surplus: %v / %v", advanced, err)
	}
}

func TestPacedDriverFinalGrantAfterUnderrunRetail(t *testing.T) {
	for _, completion := range []error{io.EOF, io.ErrUnexpectedEOF} {
		t.Run(completion.Error(), func(t *testing.T) { testPacedDriverFinalGrantAfterUnderrunRetail(t, completion) })
	}
}

func testPacedDriverFinalGrantAfterUnderrunRetail(t *testing.T, completion error) {
	pair, d, c, now := newPacedRetailPair(t)
	s, reference := pair[0], pair[1]
	var actor pool.UnitRef
	for _, u := range s.Units.IterSliced() {
		if u.Alive && u.Owner == 1 {
			actor = pool.UnitRef{Handle: u.Handle, Serial: u.AllocationSerial}
			break
		}
	}
	command := session.SeatCommand{Kind: session.SeatSelfDestruct, SelfDestruct: session.SelfDestructPayload{Actors: []pool.UnitRef{actor}}}
	payload, err := session.EncodeSeatCommand(session.OnlineCommand, command)
	if err != nil {
		t.Fatal(err)
	}
	if err := reference.EnqueueSeatCommand(session.CommandStamp{Seat: 1, Tick: 1, Position: 1}, command); err != nil {
		t.Fatal(err)
	}
	d.reads <- localRead{grant: relay.LocalGrant{Tick: 1, Position: 1, Commands: []relay.LocalCommand{{Seat: 1, Sequence: 1, Position: 1, Payload: payload}}}}
	d.reads <- localRead{grant: relay.LocalGrant{Tick: 2, Position: 1}}
	var finalTick uint32
	for tick := uint32(1); tick <= 600; tick++ {
		if err := reference.StepGranted(tick); err != nil {
			t.Fatal(err)
		}
		*now = now.Add(playoutInterval)
		if reference.OnlineBattleEnded() {
			// The other replica's final ACK stops new grants. This client has
			// underrun and must finish with just the lone terminal grant.
			if advanced, err := d.Pump(); advanced || err != nil || d.playout.running {
				t.Fatalf("expected underrun before final grant: %v / %v", advanced, err)
			}
			d.reads <- localRead{grant: relay.LocalGrant{Tick: tick, Position: 1}}
			if advanced, err := d.Pump(); advanced || err != nil {
				t.Fatalf("expected refill before final grant: %v / %v", advanced, err)
			}
			*now = now.Add(2 * playoutInterval)
			finalTick = tick
		} else if tick > 2 {
			d.reads <- localRead{grant: relay.LocalGrant{Tick: tick, Position: 1}}
		}
		if advanced, err := d.Pump(); !advanced || err != nil || s.Clock.GlobalTick != tick {
			t.Fatalf("grant %d failed: %v / %v", tick, advanced, err)
		}
		if s.UnitStateChecksum() != reference.UnitStateChecksum() {
			t.Fatalf("replicas differ at tick %d", tick)
		}
		if finalTick != 0 {
			break
		}
	}
	if finalTick == 0 || !s.OnlineBattleEnded() || !s.Snapshot.Current().Result.Ended || d.Completed() {
		t.Fatal("final grant did not publish the shared result while awaiting relay confirmation")
	}
	if ack := c.acks[len(c.acks)-1]; !ack.ended || ack.tick != finalTick || len(c.acks) != int(finalTick) {
		t.Fatalf("final tick was not acknowledged exactly once: %+v / %d", ack, len(c.acks))
	}
	before := s.UnitStateChecksum()
	// Surplus commands must not even be decoded after the shared end boundary.
	for tick := finalTick + 1; tick <= finalTick+3; tick++ {
		d.reads <- localRead{grant: relay.LocalGrant{Tick: tick, Commands: []relay.LocalCommand{{Payload: []byte{0xff}}}}}
	}
	for range 3 {
		*now = now.Add(time.Second)
		if advanced, err := d.Pump(); advanced || err != nil || d.Completed() {
			t.Fatalf("surplus grant advanced or completed without relay confirmation: %v / %v", advanced, err)
		}
	}
	if len(d.reads) != 0 || s.Clock.GlobalTick != finalTick || len(c.acks) != int(finalTick) || s.UnitStateChecksum() != before {
		t.Fatal("surplus grants changed terminal session or were not drained")
	}
	d.readError <- completion
	if !errors.Is(completion, io.EOF) {
		if advanced, err := d.Pump(); advanced || !errors.Is(err, completion) || d.Completed() {
			t.Fatalf("transport loss after final tick accepted as completion: %v / %v", advanced, err)
		}
		return
	}
	if advanced, err := d.Pump(); advanced || err != nil || !d.Completed() {
		t.Fatalf("explicit normal completion was not accepted: %v / %v", advanced, err)
	}
	if advanced, err := d.Pump(); advanced || err != nil || len(c.acks) != int(finalTick) {
		t.Fatalf("completed driver advanced or repeated final ACK: %v / %v", advanced, err)
	}
}

func TestPacedDriverRejectsPrematureCompletionRetail(t *testing.T) {
	pair, d, _, _ := newPacedRetailPair(t)
	d.reads <- localRead{grant: relay.LocalGrant{Tick: 1}}
	d.readError <- io.EOF
	if advanced, err := d.Pump(); advanced || !errors.Is(err, io.EOF) || d.Completed() || pair[0].Clock.GlobalTick != 0 || len(d.reads) != 1 {
		t.Fatalf("premature completion consumed a grant or succeeded: %v / %v", advanced, err)
	}
}
