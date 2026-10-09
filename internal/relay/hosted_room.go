package relay

import (
	"fmt"
	"time"

	"github.com/nanolathe-gg/nanolathe/internal/netproto"
)

type hostedEvent struct {
	peer *hostedPeer
	body []byte
	err  error
	join bool
}

type hostedRoom struct {
	server    *HostedServer
	code      string
	creator   *hostedPeer
	config    []byte // the creator's opaque configuration, for Describe
	autoStart bool
	joined    bool // Protected by server.mu; admission reserves the second seat.
	events    chan hostedEvent
	done      chan struct{}
}

func (r *hostedRoom) send(e hostedEvent) bool {
	select {
	case r.events <- e:
		return true
	case <-r.done:
		return false
	case <-r.server.done:
		return false
	}
}

type hostedAck struct {
	tick  uint32
	ended bool
	check [32]byte
}

// A ring keeps reports until their same-tick partner arrives. The grant window
// bounds the difference between the two executed ticks to 30 (§16.5.2).
type hostedProgress struct {
	acked    [2]uint32
	last     [2]time.Time
	history  [2][hostedMaxAhead + 1]hostedAck
	terminal uint32
}

func (p *hostedProgress) acknowledge(seat uint8, grant uint32, ack hostedAck, now time.Time) (bool, error) {
	if p.acked[seat] == ^uint32(0) || ack.tick != p.acked[seat]+1 || ack.tick > grant || (p.terminal != 0 && ack.tick > p.terminal) {
		return false, hostedError("acknowledgment tick", "exactly the next executed tick within the granted prefix")
	}
	if ack.ended {
		if p.terminal != 0 && p.terminal != ack.tick {
			return false, hostedError("terminal tick", "the same terminal tick from both seats")
		}
		p.terminal = ack.tick
	}
	p.acked[seat], p.last[seat] = ack.tick, now
	p.history[seat][ack.tick%(hostedMaxAhead+1)] = ack
	if p.acked[1-seat] < ack.tick {
		return false, nil
	}
	other := p.history[1-seat][ack.tick%(hostedMaxAhead+1)]
	if other.tick != ack.tick || other.ended != ack.ended {
		return false, hostedError("terminal agreement", "the same terminal tick and outcome bit from both seats")
	}
	if ack.tick%30 == 0 && other.check != ack.check {
		return false, hostedError(fmt.Sprintf("tick %d checksum", ack.tick), "identical unit checksums")
	}
	return ack.ended, nil
}

func (r *hostedRoom) run() {
	defer r.server.wg.Done()
	defer func() {
		r.server.mu.Lock()
		delete(r.server.rooms, r.code)
		r.server.mu.Unlock()
	}()
	peers := [2]*hostedPeer{r.creator, nil}
	var sequences [2]uint64
	var pending localCommandQueue
	var position uint64
	var tick uint32
	var progress hostedProgress
	var lastSeal time.Time
	seal := time.NewTimer(time.Hour)
	seal.Stop()
	defer seal.Stop()
	var ready <-chan time.Time
	deadline := time.NewTimer(r.server.timeouts.waiting)
	defer deadline.Stop()

	finish := func(err error) {
		// Unblock producers first; then let each writer flush its last frame.
		// Closing the room must never truncate the explicit done message.
		close(r.done)
		body := []byte{localDoneMessage}
		if err != nil {
			body = localFailureBody(localFailedMessage, err)
		}
		for _, peer := range peers {
			if peer != nil {
				if err := peer.enqueue(body, true); err != nil {
					_ = peer.conn.Close()
					peer.stop()
				}
			}
		}
		for _, peer := range peers {
			if peer != nil {
				<-peer.written
			}
		}
	}
	// Before Start the room is a lobby: which seats are present and ready
	// (DESIGN_MULTIPLAYER §16.6.1). Grants begin only after Started.
	var started bool
	var lobbyReady [2]bool
	lobby := func() error {
		var bits uint8
		for i, peer := range peers {
			if peer != nil {
				bits |= 1 << i
			}
			if lobbyReady[i] {
				bits |= 4 << i
			}
		}
		for _, peer := range peers {
			if peer != nil {
				if err := peer.enqueue([]byte{hostedLobbyMessage, bits}, false); err != nil {
					return err
				}
			}
		}
		return nil
	}
	arm := func() {
		if started && peers[1] != nil && ready == nil && progress.terminal == 0 && tick-min(progress.acked[0], progress.acked[1]) < hostedMaxAhead {
			seal.Reset(max(0, time.Until(lastSeal.Add(localTickInterval))))
			ready = seal.C
		}
	}
	begin := func() error {
		started = true
		for _, peer := range peers {
			if err := peer.enqueue([]byte{hostedStartedMessage}, false); err != nil {
				return err
			}
		}
		progress.last = [2]time.Time{time.Now(), time.Now()}
		deadline.Reset(r.server.timeouts.progress)
		arm()
		return nil
	}
	if err := lobby(); err != nil {
		finish(err)
		return
	}
	for {
		select {
		case <-r.server.done:
			finish(hostedError("server", "an open server"))
			return
		case <-deadline.C:
			path, want := "room wait", "a started match within 30 minutes"
			if started {
				path, want = "execution progress", "execution progress from both seats within ten seconds"
			}
			finish(hostedError(path, want))
			return
		case <-ready:
			ready = nil
			if tick == ^uint32(0) {
				finish(hostedError("grant tick", "a tick before uint32 exhaustion"))
				return
			}
			tick++
			// Keep the 30 Hz phase when the timer wakes late, so wake-up delay
			// does not accumulate into sealing below 30 Hz. A longer gap, such
			// as a wait at the lead bound, restarts the phase without a burst.
			now := time.Now()
			if next := lastSeal.Add(localTickInterval); !lastSeal.IsZero() && now.Sub(next) < localTickInterval {
				lastSeal = next
			} else {
				lastSeal = now
			}
			commands := pending.release(now)
			body := encodeLocalGrant(LocalGrant{Tick: tick, Position: pending.sealed, Commands: commands})
			for _, peer := range peers {
				if err := peer.enqueue(body, false); err != nil {
					finish(err)
					return
				}
			}
			arm()
		case e := <-r.events:
			if e.peer != peers[0] && e.peer != peers[1] && !e.join {
				continue // a released joiner's late writer error
			}
			if e.err != nil {
				if !started && e.peer == peers[1] {
					// A joiner leaving the lobby frees seat 2 for another.
					_ = e.peer.conn.Close()
					e.peer.stop()
					peers[1], lobbyReady[1] = nil, false
					r.server.mu.Lock()
					r.joined = false
					r.server.mu.Unlock()
					if err := lobby(); err != nil {
						finish(err)
						return
					}
					continue
				}
				if !started {
					finish(hostedError("room host", "the host to stay until the match starts"))
					return
				}
				finish(e.err)
				return
			}
			if e.join {
				peers[1] = e.peer
				if r.autoStart {
					lobbyReady = [2]bool{true, true}
				}
				if err := lobby(); err != nil {
					finish(err)
					return
				}
				if r.autoStart {
					if err := begin(); err != nil {
						finish(err)
						return
					}
				}
				continue
			}
			seat := e.peer.hello.Seat
			d := netproto.NewReader(e.body, hostedError)
			// Commands submitted before Start wait for the first grant, as
			// they did before the lobby; an early acknowledgment fails below.
			switch d.U8() {
			case hostedReadyMessage:
				flag := d.Bool()
				if err := d.End(); err != nil {
					finish(err)
					return
				}
				// After Start a late ready changes nothing (§16.6.1).
				if !started {
					lobbyReady[seat] = flag
					if err := lobby(); err != nil {
						finish(err)
						return
					}
				}
			case hostedStartMessage:
				if err := d.End(); err != nil {
					finish(err)
					return
				}
				if started {
					continue
				}
				if seat != 0 {
					finish(hostedError("start", "the room's host"))
					return
				}
				// A start that races an unready joiner just repeats the state.
				var err error
				if peers[1] == nil || !lobbyReady[0] || !lobbyReady[1] {
					err = lobby()
				} else {
					err = begin()
				}
				if err != nil {
					finish(err)
					return
				}
			case localSubmitMessage:
				sequence := d.U64()
				n := d.Count(netproto.MaxCommandBytes, 1)
				payload := d.Raw(n)
				if err := d.End(); err != nil {
					finish(err)
					return
				}
				if progress.terminal != 0 {
					// Input already in flight cannot add work after shared termination.
					continue
				}
				if sequence <= sequences[seat] {
					continue
				}
				if sequence != sequences[seat]+1 {
					err := hostedError("command sequence", fmt.Sprintf("sequence %d", sequences[seat]+1))
					if err := e.peer.enqueue(localFailureBody(localRefusedMessage, err), false); err != nil {
						finish(err)
						return
					}
					continue
				}
				if len(pending.entries) == localMaxCommands || n > hostedMaxPendingBytes-pending.bytes || position == ^uint64(0) {
					finish(hostedError("pending commands", "at most 64 pending commands and 256 KiB"))
					return
				}
				sequences[seat] = sequence
				position++
				pending.entries = append(pending.entries, localPendingCommand{command: LocalCommand{Seat: seat, Sequence: sequence, Position: position, Payload: payload}})
				pending.bytes += n
			case localAckMessage:
				ack := hostedAck{tick: d.U32(), ended: d.Bool()}
				if ack.tick%30 == 0 {
					ack.check = d.Digest()
				}
				if err := d.End(); err != nil {
					finish(err)
					return
				}
				complete, err := progress.acknowledge(seat, tick, ack, time.Now())
				if err != nil || complete {
					finish(err)
					return
				}
				last := progress.last[0]
				if progress.last[1].Before(last) {
					last = progress.last[1]
				}
				deadline.Reset(max(0, time.Until(last.Add(r.server.timeouts.progress))))
				if progress.terminal != 0 {
					seal.Stop()
					ready = nil
				}
				arm()
			default:
				finish(hostedError("client message", "ready, start, submit or acknowledgment after hello"))
				return
			}
		}
	}
}
