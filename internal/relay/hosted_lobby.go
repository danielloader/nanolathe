package relay

import (
	"context"
	"errors"
	"net"
	"strings"
	"sync"

	"github.com/nanolathe-gg/nanolathe/internal/netproto"
)

// HostedLobbyState is one seat's latest view of its room before Start
// (DESIGN_MULTIPLAYER §16.6.1). Seat 0 is the creator.
type HostedLobbyState struct {
	Present, Ready [2]bool
	// Mismatch reports both seats ready with different rehearsal digests:
	// their simulations disagree, so the match cannot start (§16.7).
	Mismatch bool
	Started  bool
}

// HostedLobby is one seat's connection to a hosted room from Create or Join
// until Start, when Battle hands the same connection to the grant stream. Its
// reader goroutine stops exactly after Started, so the first grant reaches
// the battle client. State never blocks, so the window host may poll it every
// frame.
type HostedLobby struct {
	code   string
	seat   uint8
	conn   net.Conn
	client *LocalClient
	done   chan struct{} // closed when the reader stops

	mu     sync.Mutex
	state  HostedLobbyState
	err    error
	handed bool
}

// DescribeHostedRoom returns a room's encoded configuration, before the joiner
// composes anything. address is host:port for TLS or a wss://host/relay URL.
func DescribeHostedRoom(ctx context.Context, address, room string, options HostedDialOptions) ([]byte, error) {
	if !validHostedCode(room) {
		return nil, hostedError("room code", "a six-character invitation")
	}
	ctx, cancel := context.WithTimeout(ctx, hostedDefaultTimeouts.handshake)
	defer cancel()
	conn, err := dialHostedTransport(ctx, address, options)
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	stop := context.AfterFunc(ctx, func() { _ = conn.Close() })
	defer stop()
	var w netproto.Writer
	w.U8(hostedDescribeMessage)
	w.U16(hostedVersion)
	w.Text(room)
	if err := writeLocalFrame(conn, w.Bytes()); err != nil {
		return nil, err
	}
	response, err := readLocalFrame(conn, hostedMaxConfigBytes+64)
	if err != nil {
		if expired := handshakeExpired(ctx); expired != nil {
			return nil, localIOError("room description", expired)
		}
		return nil, err
	}
	r := netproto.NewReader(response, hostedError)
	switch r.U8() {
	case hostedDescriptionMessage:
		config := r.Raw(r.Count(hostedMaxConfigBytes, 1))
		return config, r.End()
	case localRefusedMessage, localFailedMessage:
		message := r.Text(localMaxErrorBytes)
		if err := r.End(); err != nil {
			return nil, err
		}
		return nil, errors.New(message)
	default:
		return nil, hostedError("room description", "a description or refusal")
	}
}

// OpenHostedLobby creates a room when room is empty, carrying the host's
// encoded configuration (at most 64 KiB), or joins one with config nil.
func OpenHostedLobby(ctx context.Context, address, room string, hello LocalHello, config []byte, options HostedDialOptions) (*HostedLobby, error) {
	if room == "" && len(config) == 0 {
		return nil, hostedError("room configuration", "the creator's encoded configuration")
	}
	body, err := encodeHostedHello(room, 0, hello, config)
	if err != nil {
		return nil, err
	}
	conn, code, err := hostedHandshake(ctx, address, options, body, room, hello.Seat)
	if err != nil {
		return nil, err
	}
	l := &HostedLobby{code: code, seat: hello.Seat, conn: conn, client: newHostedClient(conn), done: make(chan struct{})}
	go l.read()
	return l, nil
}

func (l *HostedLobby) read() {
	defer close(l.done)
	for {
		body, err := readLocalFrame(l.conn, localMaxErrorBytes+64)
		if err != nil {
			l.fail(err)
			return
		}
		r := netproto.NewReader(body, hostedError)
		switch r.U8() {
		case hostedLobbyMessage:
			bits := r.U8()
			if err := r.End(); err != nil {
				l.fail(err)
				return
			}
			l.mu.Lock()
			for i := range 2 {
				l.state.Present[i] = bits&(1<<i) != 0
				l.state.Ready[i] = bits&(4<<i) != 0
			}
			l.state.Mismatch = bits&hostedLobbyMismatch != 0
			l.mu.Unlock()
		case hostedStartedMessage:
			if err := r.End(); err != nil {
				l.fail(err)
				return
			}
			l.mu.Lock()
			l.state.Started = true
			l.mu.Unlock()
			return // grants follow; the battle client reads them
		case localRefusedMessage, localFailedMessage:
			message := r.Text(localMaxErrorBytes)
			if err := r.End(); err != nil {
				l.fail(err)
				return
			}
			l.fail(errors.New(message))
			return
		case localDoneMessage:
			l.fail(hostedError("room", "an open room"))
			return
		default:
			l.fail(hostedError("lobby message", "lobby state, Started or a refusal"))
			return
		}
	}
}

func (l *HostedLobby) fail(err error) {
	l.mu.Lock()
	if l.err == nil {
		l.err = err
	}
	l.mu.Unlock()
	_ = l.conn.Close()
}

func (l *HostedLobby) Code() string { return l.code }
func (l *HostedLobby) Seat() uint8  { return l.seat }

// State returns the latest lobby snapshot, or the error that ended the lobby.
func (l *HostedLobby) State() (HostedLobbyState, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.state, l.err
}

// SetReady reports this seat ready with the digest of its rehearsal
// (session.RehearsalDigest), or not ready. The relay starts the match only
// when both seats are ready with equal digests (§16.7).
func (l *HostedLobby) SetReady(ready bool, rehearsal [32]byte) error {
	if _, err := l.State(); err != nil {
		return err
	}
	if !ready {
		return l.client.writeMessage([]byte{hostedReadyMessage, 0})
	}
	return l.client.writeMessage(append([]byte{hostedReadyMessage, 1}, rehearsal[:]...))
}

// Start asks the relay to begin the match; only seat 0 may, once both seats
// are present and ready. A refused start leaves the lobby open.
func (l *HostedLobby) Start() error {
	if l.seat != 0 {
		return hostedError("start", "the room's host")
	}
	if _, err := l.State(); err != nil {
		return err
	}
	return l.client.writeMessage([]byte{hostedStartMessage})
}

// Battle returns the grant-stream client once Started, or nil before then.
// The lobby no longer reads from the connection after it is returned, and
// Close leaves that connection to the battle.
func (l *HostedLobby) Battle() *LocalClient {
	l.mu.Lock()
	started := l.state.Started && l.err == nil
	l.mu.Unlock()
	if !started {
		return nil
	}
	<-l.done
	l.mu.Lock()
	l.handed = true
	l.mu.Unlock()
	return l.client
}

// Close leaves the lobby. After Battle has handed over the connection, the
// battle client owns it and Close does nothing.
func (l *HostedLobby) Close() error {
	l.mu.Lock()
	handed := l.handed
	if !handed && l.err == nil {
		l.err = hostedError("lobby", "an open lobby")
	}
	l.mu.Unlock()
	if handed {
		return nil
	}
	err := l.conn.Close()
	<-l.done
	return err
}

// NormalizeRoomCode accepts a typed invitation in any case, with spaces or
// dashes, and returns the canonical code.
func NormalizeRoomCode(typed string) (string, bool) {
	var b strings.Builder
	for _, r := range strings.ToUpper(typed) {
		if r == ' ' || r == '-' {
			continue
		}
		b.WriteRune(r)
	}
	code := b.String()
	return code, validHostedCode(code)
}
