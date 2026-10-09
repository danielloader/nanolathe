package relay

import (
	"net"
	"sync"
	"time"

	"github.com/nanolathe-gg/nanolathe/internal/netproto"
)

type hostedOutput struct {
	body []byte
	last bool
}

type hostedPeer struct {
	conn          net.Conn
	hello         LocalHello
	room          *hostedRoom
	timeout       time.Duration
	helloDeadline time.Time
	out           chan hostedOutput
	written       chan struct{}
	stopped       chan struct{}
	once          sync.Once
	mu            sync.Mutex
	bytes         int
}

func newHostedPeer(conn net.Conn, hello LocalHello, timeout time.Duration) *hostedPeer {
	return &hostedPeer{conn: conn, hello: hello, timeout: timeout, out: make(chan hostedOutput, hostedMaxQueuedFrames), written: make(chan struct{}), stopped: make(chan struct{})}
}

func (p *hostedPeer) stop() { p.once.Do(func() { close(p.stopped) }) }

// Charge queued and in-flight frames together. Neither a blocked writer nor
// tiny refusal floods may consume unbounded storage (§16.5.1).
func (p *hostedPeer) enqueue(body []byte, last bool) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	n := len(body) + netproto.UvarintLen(uint64(len(body)))
	if n > hostedMaxQueuedBytes-p.bytes {
		return hostedError("peer write queue", "at most 1 MiB plus the frame header allowance")
	}
	select {
	case <-p.written:
		return hostedError("peer write queue", "an open writer")
	default:
	}
	select {
	case p.out <- hostedOutput{body: body, last: last}:
		p.bytes += n
		return nil
	default:
		return hostedError("peer write queue", "at most 64 queued frames")
	}
}

func minDeadline(a, b time.Time) time.Time {
	if a.IsZero() || b.Before(a) {
		return b
	}
	return a
}

func (p *hostedPeer) write() {
	defer close(p.written)
	defer p.conn.Close()
	firstDeadline := p.helloDeadline
	for {
		select {
		case <-p.stopped:
			return
		case message := <-p.out:
			err := p.conn.SetWriteDeadline(minDeadline(firstDeadline, time.Now().Add(p.timeout)))
			firstDeadline = time.Time{}
			if err == nil {
				err = writeLocalFrame(p.conn, message.body)
			}
			p.mu.Lock()
			p.bytes -= len(message.body) + netproto.UvarintLen(uint64(len(message.body)))
			p.mu.Unlock()
			if err != nil {
				p.room.send(hostedEvent{peer: p, err: err})
				return
			}
			if message.last {
				return
			}
		}
	}
}
