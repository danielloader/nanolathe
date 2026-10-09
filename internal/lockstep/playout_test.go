package lockstep

import (
	"errors"
	"io"
	"sync"
	"testing"
	"time"

	"github.com/nanolathe-gg/nanolathe/internal/relay"
	"github.com/nanolathe-gg/nanolathe/internal/session"
)

func TestPlayoutReserveAndUnderrun(t *testing.T) {
	now := time.Unix(0, 0)
	p := playoutClock{now: func() time.Time { return now }}
	if p.due(0) != 0 || p.due(1) != 0 {
		t.Fatal("started without the one-tick reserve")
	}
	if p.due(2) != 1 || p.due(2) != 0 {
		t.Fatal("two grants must start exactly one tick")
	}
	now = now.Add(playoutInterval - 1)
	if p.due(1) != 0 {
		t.Fatal("released before the normal deadline")
	}
	now = now.Add(1)
	if p.due(1) != 1 {
		t.Fatal("reserve did not absorb late delivery at the deadline")
	}
	now = now.Add(playoutInterval)
	if p.due(0) != 0 || p.running {
		t.Fatal("underrun did not enter refill")
	}
	now = now.Add(time.Second)
	if p.due(1) != 0 {
		t.Fatal("underrun debt bypassed the refill")
	}
	if p.due(2) != 1 || p.due(2) != 0 {
		t.Fatal("refill did not restart with one tick")
	}
}

func TestPlayoutLoneGrantRefillIsBounded(t *testing.T) {
	now := time.Unix(0, 0)
	p := playoutClock{now: func() time.Time { return now }}
	for _, phase := range []string{"startup", "underrun"} {
		if p.due(1) != 0 {
			t.Fatalf("%s bypassed refill", phase)
		}
		now = now.Add(2*playoutInterval - 1)
		if p.due(1) != 0 {
			t.Fatalf("%s released before the refill bound", phase)
		}
		now = now.Add(1)
		if p.due(1) != 1 || p.due(1) != 0 {
			t.Fatalf("%s did not release exactly one lone grant at the bound", phase)
		}
		now = now.Add(playoutInterval)
		if p.due(0) != 0 {
			t.Fatal("released absent grant")
		}
	}
}

func TestPlayoutHostCadenceAndBoundedCatchup(t *testing.T) {
	for _, tc := range []struct {
		name        string
		depth, want int
	}{
		{"steady", 2, 300},
		{"backlog", pacedReceiveLimit, 330},
	} {
		t.Run(tc.name, func(t *testing.T) {
			now := time.Unix(0, 0)
			start := now
			p := playoutClock{now: func() time.Time { return now }}
			count := 0
			for update := 0; update < 600; update++ {
				now = start.Add(time.Duration(update) * time.Second / 60)
				count += p.due(tc.depth)
				if p.due(tc.depth) != 0 {
					t.Fatal("repeated host call released another tick at the same time")
				}
			}
			if count < tc.want-1 || count > tc.want+1 {
				t.Fatalf("10-second release count = %d, want %d +/- 1", count, tc.want)
			}
		})
	}
	// A long host stall retains only host-step quantization's lateness, never
	// a burst of owed simulation work.
	now := time.Unix(0, 0)
	p := playoutClock{now: func() time.Time { return now }}
	if p.due(pacedReceiveLimit) != 1 {
		t.Fatal("backlog did not start")
	}
	now = now.Add(5 * time.Second)
	if p.due(pacedReceiveLimit) != playoutBurstLimit || p.due(pacedReceiveLimit) != 0 {
		t.Fatal("stall must release exactly the retained lateness")
	}
	resume := time.Duration(playoutBurstLimit)*playoutCatchupInterval - playoutLateness
	now = now.Add(resume - 1)
	if p.due(pacedReceiveLimit) != 0 {
		t.Fatal("stall debt accelerated catch-up beyond its bound")
	}
	now = now.Add(1)
	if p.due(pacedReceiveLimit) != 1 {
		t.Fatal("catch-up missed its exact deadline after a stall")
	}
}

// The window host pumps only on its 30 Hz steps, each landing on the display
// refresh nearest its ideal instant; a slow window takes several steps at one
// instant. Grants arrive from a relay sealing at exactly 30 Hz. This locks the
// executed rate and backlog drain at the displays players use, not at a
// convenient test cadence (DESIGN_MULTIPLAYER §16.5.2).
func TestPlayoutWindowHostDisplayRates(t *testing.T) {
	const period = time.Second / 30
	const seconds = 20
	for _, hz := range []int{20, 24, 50, 60, 75, 100, 120, 144, 165, 240} {
		for _, backlog := range []int{0, 28} {
			now := time.Unix(0, 0)
			start := now
			p := playoutClock{now: func() time.Time { return now }}
			refresh := time.Second / time.Duration(hz)
			queued, delivered, executed, worst := backlog, 0, 0, 0
			hostSteps := func(k int) int64 { return (int64(k)*int64(refresh) + int64(period)/2) / int64(period) }
			for k := 1; time.Duration(k)*refresh <= seconds*time.Second; k++ {
				at := time.Duration(k) * refresh
				now = start.Add(at)
				// Grant i is sealed at i periods and arrives 5 ms later.
				for time.Duration(delivered+1)*period+5*time.Millisecond <= at {
					delivered++
					queued++
				}
				for range hostSteps(k) - hostSteps(k-1) {
					n := p.due(queued)
					if n > queued {
						t.Fatalf("%d Hz released %d of %d received grants", hz, n, queued)
					}
					queued -= n
					executed += n
				}
				if at > 15*time.Second {
					worst = max(worst, queued)
				}
			}
			if backlog == 0 && executed < seconds*30-3 {
				t.Errorf("%d Hz display executed %d ticks in %d s, want about %d", hz, executed, seconds, seconds*30)
			}
			// Steady play keeps the one-tick reserve plus host-step jitter;
			// a stall's backlog has drained at the catch-up rate.
			if worst > 3 {
				t.Errorf("%d Hz display with backlog %d still queued %d grants after 15 s", hz, backlog, worst)
			}
		}
	}
}

type clientAck struct {
	tick  uint32
	hash  [32]byte
	ended bool
}

// The host owns submissions/acks. Only ReadGrant and Close run concurrently.
// Tests control time and grant delivery independently of the reader goroutine.
type testClient struct {
	input    chan localRead
	closed   chan struct{}
	once     sync.Once
	commands [][]byte
	acks     []clientAck
	ackError error
}

func newTestClient() *testClient {
	return &testClient{input: make(chan localRead), closed: make(chan struct{})}
}
func (c *testClient) Submit(payload []byte) (uint64, error) {
	c.commands = append(c.commands, append([]byte(nil), payload...))
	return uint64(len(c.commands)), nil
}
func (c *testClient) ReadGrant() (relay.LocalGrant, error) {
	select {
	case r := <-c.input:
		return r.grant, r.err
	case <-c.closed:
		return relay.LocalGrant{}, io.ErrClosedPipe
	}
}
func (c *testClient) Acknowledge(tick uint32, hash [32]byte, ended, _ bool) error {
	c.acks = append(c.acks, clientAck{tick, hash, ended})
	return c.ackError
}
func (c *testClient) Close() error {
	c.once.Do(func() { close(c.closed) })
	return nil
}

func TestPacedDriverAdmissionAndFailurePriority(t *testing.T) {
	if (*LocalDriver)(nil).Completed() {
		t.Fatal("absent driver reported completion")
	}
	c := newTestClient()
	for _, s := range []*session.Session{nil, {}} {
		if _, err := NewPacedDriver(s, c); err == nil {
			t.Fatal("accepted an unprepared session")
		}
	}
	for _, explicitClose := range []bool{false, true} {
		c := newTestClient()
		d := &LocalDriver{client: c, reads: make(chan localRead, pacedReceiveLimit), readError: make(chan error, 1), done: make(chan struct{}), playout: &playoutClock{}}
		d.reads <- localRead{grant: relay.LocalGrant{Tick: 1}}
		want := io.ErrClosedPipe
		if explicitClose {
			_ = d.Close()
		} else {
			want = io.ErrUnexpectedEOF
			d.readError <- want
		}
		for range 2 {
			if advanced, err := d.Pump(); advanced || !errors.Is(err, want) {
				t.Fatalf("closed/failed driver advanced or lost failure: %v / %v", advanced, err)
			}
			if len(d.reads) != 1 || d.Completed() {
				t.Fatal("closed/failed driver consumed a buffered grant or reported normal completion")
			}
		}
		if _, err := d.Submit(session.HumanCommand{}); !errors.Is(err, want) || len(c.commands) != 0 {
			t.Fatalf("closed/failed driver submitted: %v", err)
		}
	}
}

func TestPacedReaderBoundsDeliveryAndCloseUnblocks(t *testing.T) {
	c := newTestClient()
	d := &LocalDriver{client: c, reads: make(chan localRead, pacedReceiveLimit), readError: make(chan error, 1), done: make(chan struct{})}
	stopped := make(chan struct{})
	go func() { d.read(); close(stopped) }()
	defer d.Close()
	// The reader can fill 32 entries and hold one in-flight grant; it cannot
	// ask the transport for another while delivery is blocked.
	deadline := time.NewTimer(5 * time.Second)
	defer deadline.Stop()
	for tick := uint32(1); tick <= pacedReceiveLimit+1; tick++ {
		select {
		case c.input <- localRead{grant: relay.LocalGrant{Tick: tick}}:
		case <-deadline.C:
			t.Fatal("reader did not fill its bounded queue")
		}
	}
	if len(d.reads) != pacedReceiveLimit || cap(d.reads) != pacedReceiveLimit {
		t.Fatalf("receive depth/capacity = %d/%d", len(d.reads), cap(d.reads))
	}
	select {
	case c.input <- localRead{grant: relay.LocalGrant{Tick: pacedReceiveLimit + 2}}:
		t.Fatal("reader accepted beyond its bounded queue and in-flight grant")
	default:
	}
	_ = d.Close()
	select {
	case <-stopped:
	case <-deadline.C:
		t.Fatal("close did not unblock the reader")
	}
}

func TestPacedReaderFailureOvertakesBufferedGrants(t *testing.T) {
	c := newTestClient()
	d := &LocalDriver{client: c, reads: make(chan localRead, pacedReceiveLimit), readError: make(chan error, 1), done: make(chan struct{}), playout: &playoutClock{}}
	stopped := make(chan struct{})
	go func() { d.read(); close(stopped) }()
	defer d.Close()
	deadline := time.NewTimer(5 * time.Second)
	defer deadline.Stop()
	for _, r := range []localRead{{grant: relay.LocalGrant{Tick: 1}}, {grant: relay.LocalGrant{Tick: 2}}, {err: io.ErrUnexpectedEOF}} {
		select {
		case c.input <- r:
		case <-deadline.C:
			t.Fatal("reader stalled before reporting the terminal failure")
		}
	}
	select {
	case <-stopped:
	case <-deadline.C:
		t.Fatal("reader did not finish after failure")
	}
	if advanced, err := d.Pump(); advanced || !errors.Is(err, io.ErrUnexpectedEOF) || len(d.reads) != 2 || d.Completed() {
		t.Fatalf("reader failure did not overtake buffered grants: %v / %v", advanced, err)
	}
}
