package relay

import (
	"context"
	"strings"
	"testing"
	"time"
)

func awaitLobby(t *testing.T, l *HostedLobby, what string, ok func(HostedLobbyState, error) bool) (HostedLobbyState, error) {
	t.Helper()
	end := time.Now().Add(3 * time.Second)
	for {
		state, err := l.State()
		if ok(state, err) {
			return state, err
		}
		if time.Now().After(end) {
			t.Fatalf("lobby never reached %s: %+v %v", what, state, err)
		}
		time.Sleep(time.Millisecond)
	}
}

func openLobbyTest(t *testing.T, address, room string, seat uint8, config []byte, options HostedDialOptions) *HostedLobby {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	l, err := OpenHostedLobby(ctx, address, room, localTestHello(seat), config, options)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = l.Close() })
	return l
}

func describeTest(address, room string, options HostedDialOptions) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	return DescribeHostedRoom(ctx, address, room, options)
}

func present(both bool) func(HostedLobbyState, error) bool {
	return func(s HostedLobbyState, err error) bool { return err == nil && s.Present[0] && s.Present[1] == both }
}

// The lobby of DESIGN_MULTIPLAYER §16.6.1 over both hosted transports: the
// joiner learns the configuration first, readiness gates the host's start,
// and the first grant reaches the battle client after Started.
func TestHostedLobbyDescribeReadyAndStart(t *testing.T) {
	for _, websocket := range []bool{false, true} {
		var s *HostedServer
		address := ""
		if websocket {
			s = listenWebSocketTest(t, HostedConfig{InsecureLoopback: true}, false, hostedDefaultTimeouts)
			address = "ws://" + s.Addr() + "/relay"
		} else {
			s = listenHostedTest(t, HostedConfig{InsecureLoopback: true}, hostedDefaultTimeouts)
			address = s.Addr()
		}
		options := HostedDialOptions{InsecureLoopback: true}
		host := openLobbyTest(t, address, "", 0, []byte("configuration"), options)
		awaitLobby(t, host, "the host's seat", present(false))
		if len(host.Code()) != hostedCodeLength || host.Seat() != 0 {
			t.Fatalf("room %q seat %d", host.Code(), host.Seat())
		}
		if config, err := describeTest(address, host.Code(), options); err != nil || string(config) != "configuration" {
			t.Fatalf("describe: %q %v", config, err)
		}
		joiner := openLobbyTest(t, address, host.Code(), 1, nil, options)
		awaitLobby(t, host, "both seats", present(true))
		awaitLobby(t, joiner, "both seats", present(true))
		if _, err := describeTest(address, host.Code(), options); err == nil || !strings.Contains(err.Error(), "unoccupied second seat") {
			t.Fatalf("describe of a full room: %v", err)
		}
		if err := joiner.Start(); err == nil {
			t.Fatal("the joining seat started the match")
		}
		// A start before both seats are ready only repeats the state.
		if err := host.SetReady(true); err != nil {
			t.Fatal(err)
		}
		if err := host.Start(); err != nil {
			t.Fatal(err)
		}
		awaitLobby(t, joiner, "the host ready", func(s HostedLobbyState, err error) bool { return err == nil && s.Ready[0] })
		if err := joiner.SetReady(true); err != nil {
			t.Fatal(err)
		}
		state, _ := awaitLobby(t, host, "both ready", func(s HostedLobbyState, err error) bool { return err == nil && s.Ready[0] && s.Ready[1] })
		if state.Started || host.Battle() != nil {
			t.Fatal("an early start began the match")
		}
		if err := host.Start(); err != nil {
			t.Fatal(err)
		}
		for _, l := range []*HostedLobby{host, joiner} {
			awaitLobby(t, l, "Started", func(s HostedLobbyState, err error) bool { return err == nil && s.Started })
		}
		battles := [2]*LocalClient{host.Battle(), joiner.Battle()}
		if battles[0] == nil || battles[1] == nil {
			t.Fatal("started lobby did not hand over its connection")
		}
		_ = host.Close() // the battle owns the connection now
		for _, c := range battles {
			_ = c.conn.SetReadDeadline(time.Now().Add(3 * time.Second))
		}
		if g := readLocalPair(t, battles); g.Tick != 1 {
			t.Fatalf("first grant after Started: %+v", g)
		}
		for _, c := range battles {
			_ = c.Close()
		}
		awaitHostedCapacity(t, s, 0, 0)
	}
}

func TestHostedLobbySeatsLeaveAndExpire(t *testing.T) {
	s := listenHostedTest(t, HostedConfig{InsecureLoopback: true}, hostedDefaultTimeouts)
	options := HostedDialOptions{InsecureLoopback: true}
	host := openLobbyTest(t, s.Addr(), "", 0, []byte{1}, options)
	first := openLobbyTest(t, s.Addr(), host.Code(), 1, nil, options)
	if err := first.SetReady(true); err != nil {
		t.Fatal(err)
	}
	awaitLobby(t, host, "the first joiner ready", func(s HostedLobbyState, err error) bool { return err == nil && s.Ready[1] })
	// A joiner leaving frees seat 2 and its readiness for another player.
	_ = first.Close()
	awaitLobby(t, host, "seat 2 free", func(s HostedLobbyState, err error) bool { return err == nil && !s.Present[1] && !s.Ready[1] })
	second := openLobbyTest(t, s.Addr(), host.Code(), 1, nil, options)
	awaitLobby(t, host, "the second joiner", present(true))
	// The host leaving closes the room for the joiner.
	_ = host.Close()
	if _, err := awaitLobby(t, second, "the room closed", func(_ HostedLobbyState, err error) bool { return err != nil }); !strings.Contains(err.Error(), "room host") {
		t.Fatalf("joiner after the host left: %v", err)
	}
	awaitHostedCapacity(t, s, 0, 0)

	timeouts := hostedDefaultTimeouts
	timeouts.waiting = 200 * time.Millisecond
	s = listenHostedTest(t, HostedConfig{InsecureLoopback: true}, timeouts)
	waiting := openLobbyTest(t, s.Addr(), "", 0, []byte{1}, options)
	if _, err := awaitLobby(t, waiting, "expiry", func(_ HostedLobbyState, err error) bool { return err != nil }); !strings.Contains(err.Error(), "room wait") {
		t.Fatalf("lobby expiry: %v", err)
	}
	if _, err := describeTest(s.Addr(), "ABCDEF", options); err == nil || !strings.Contains(err.Error(), "existing invitation") {
		t.Fatalf("describe of an unknown room: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if _, err := OpenHostedLobby(ctx, s.Addr(), "", localTestHello(0), nil, options); err == nil {
		t.Fatal("created a lobby without a configuration")
	}
	if _, err := OpenHostedLobby(ctx, s.Addr(), "ABCDEF", localTestHello(1), []byte{1}, options); err == nil {
		t.Fatal("a joiner sent a configuration")
	}
	awaitHostedCapacity(t, s, 0, 0)
}

// A command-line client joins a lobby room, reports ready at once and plays
// when the lobby's host starts; its grant reader skips the lobby messages.
func TestHostedCommandLineClientJoinsLobby(t *testing.T) {
	s := listenHostedTest(t, HostedConfig{InsecureLoopback: true}, hostedDefaultTimeouts)
	options := HostedDialOptions{InsecureLoopback: true}
	host := openLobbyTest(t, s.Addr(), "", 0, []byte{1}, options)
	joiner, _ := dialHostedTest(t, s, host.Code(), 1)
	awaitLobby(t, host, "the command-line joiner ready", func(s HostedLobbyState, err error) bool { return err == nil && s.Present[1] && s.Ready[1] })
	if err := host.SetReady(true); err != nil {
		t.Fatal(err)
	}
	awaitLobby(t, host, "both ready", func(s HostedLobbyState, err error) bool { return err == nil && s.Ready[0] })
	if err := host.Start(); err != nil {
		t.Fatal(err)
	}
	awaitLobby(t, host, "Started", func(s HostedLobbyState, err error) bool { return err == nil && s.Started })
	battle := host.Battle()
	_ = battle.conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	if g := readLocalPair(t, [2]*LocalClient{battle, joiner}); g.Tick != 1 {
		t.Fatalf("first grant: %+v", g)
	}
	_ = battle.Close()
}
