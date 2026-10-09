package relay

import (
	"testing"
	"time"
)

func TestLocalCommandDelayReleasesOnlyReadyPrefix(t *testing.T) {
	start := time.Unix(100, 0)
	entries := []localPendingCommand{
		{LocalCommand{Position: 1, Payload: []byte{1, 2}}, start.Add(100 * time.Millisecond)},
		{LocalCommand{Position: 2, Payload: []byte{3}}, start.Add(150 * time.Millisecond)},
	}
	q := localCommandQueue{entries: entries, bytes: 3}
	if got := q.release(start.Add(99 * time.Millisecond)); len(got) != 0 || q.sealed != 0 || q.bytes != 3 {
		t.Fatal("unready command entered the sealed prefix")
	}
	got := q.release(start.Add(100 * time.Millisecond))
	if len(got) != 1 || got[0].Position != 1 || q.sealed != 1 || q.bytes != 1 || len(q.entries) != 1 || entries[0].command.Payload != nil {
		t.Fatal("first release changed ordering, bounds or retained a payload")
	}
	if got := q.release(start.Add(149 * time.Millisecond)); len(got) != 0 || q.sealed != 1 {
		t.Fatal("empty grant exposed a future stream position")
	}
	got = q.release(start.Add(150 * time.Millisecond))
	if len(got) != 1 || got[0].Position != 2 || q.sealed != 2 || q.bytes != 0 || len(q.entries) != 0 || entries[1].command.Payload != nil {
		t.Fatal("final release did not drain the bounded queue")
	}
}

func TestLocalCommandDelayKeepsGrantsMoving(t *testing.T) {
	const delay = time.Second
	r, err := ListenLocalWithCommandDelay("127.0.0.1:0", delay)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	clients := [2]*LocalClient{dialLocalTest(t, r.Addr(), localTestHello(0)), dialLocalTest(t, r.Addr(), localTestHello(1))}
	g := readLocalPair(t, clients)
	start := time.Now()
	for _, c := range clients {
		if _, err := c.Submit([]byte{1}); err != nil {
			t.Fatal(err)
		}
	}
	var received uint64
	var emptyGrants int
	for received < 2 {
		ackLocalPair(t, clients, g.Tick, [32]byte{}, false)
		g = readLocalPair(t, clients)
		for _, command := range g.Commands {
			if time.Since(start) < delay {
				t.Fatal("order arrived before its configured additional delay")
			}
			received++
			if command.Position != received || command.Sequence != 1 {
				t.Fatal("delayed orders changed stream or client ordering")
			}
		}
		if g.Position != received {
			t.Fatal("grant sealed commands it did not contain")
		}
		if len(g.Commands) == 0 {
			emptyGrants++
		}
	}
	if emptyGrants == 0 {
		t.Fatal("waiting orders stopped ordinary grants")
	}
	t.Logf("%d empty grants advanced while orders waited; delivery after %s", emptyGrants, time.Since(start).Round(time.Millisecond))
}

func TestLocalCommandDelayRejectsInvalidValues(t *testing.T) {
	for _, delay := range []time.Duration{-1, time.Second + 1} {
		if r, err := ListenLocalWithCommandDelay("127.0.0.1:0", delay); err == nil {
			r.Close()
			t.Fatal("invalid delay accepted")
		}
	}
}
