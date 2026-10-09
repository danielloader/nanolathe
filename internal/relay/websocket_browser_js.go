package relay

import (
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"sync"
	"syscall/js"
	"time"
)

// The browser build reaches a relay only through the browser's own WebSocket
// (DESIGN_MULTIPLAYER §16.5.1). The browser performs the TLS handshake,
// verifies the certificate against its own trust store and answers the
// relay's pings; this file adapts the message API to the byte stream the
// existing envelope reader and writer expect.

const (
	// browserMaxSend splits a write into messages the relay accepts from a
	// client; the relay's stream joins them again.
	browserMaxSend = hostedMaxClientFrame + 5
	// browserMaxMessage is the largest relay message the client accepts, as
	// the native client's stream bounds it.
	browserMaxMessage = localMaxGrantFrame + 5
	// browserMaxBuffered bounds received bytes not yet read. The browser
	// applies no backpressure, so a relay that sends faster than the battle
	// reads fails the connection rather than growing it without bound.
	browserMaxBuffered = 2 * browserMaxMessage
	// browserMaxUnsent bounds the browser's send buffer, which a write waits
	// to drain under its deadline as a native socket write would.
	browserMaxUnsent = hostedMaxQueuedBytes
	browserSendPoll  = 5 * time.Millisecond
)

var (
	jsArrayBuffer = js.Global().Get("ArrayBuffer")
	jsUint8Array  = js.Global().Get("Uint8Array")
)

// dialHostedSocket has no browser path: a page cannot open a TCP socket.
func dialHostedSocket(context.Context, string, HostedDialOptions) (net.Conn, error) {
	return nil, hostedError("relay address", "a wss://host/relay URL, because the browser connects only to wss:// relays")
}

// dialWebSocketTransport opens the browser's WebSocket to a URL that passes
// the native client's rules and waits for it to open within the context.
func dialWebSocketTransport(ctx context.Context, address string, options HostedDialOptions) (net.Conn, error) {
	u, _, err := hostedWebSocketURL(address, options)
	if err != nil {
		return nil, err
	}
	if options.TLSConfig != nil {
		return nil, hostedError("WebSocket TLS", "the browser's own certificate verification, without a custom TLS configuration")
	}
	c, err := openBrowserWebSocket(u.String())
	if err != nil {
		return nil, err
	}
	select {
	case <-c.opened:
	case <-c.closed:
		err := c.failure()
		_ = c.Close()
		return nil, localIOError("WebSocket dial", err)
	case <-ctx.Done():
		_ = c.Close()
		return nil, localIOError("WebSocket handshake", ctx.Err())
	}
	if protocol := c.ws.Get("protocol").String(); protocol != websocketProtocol {
		_ = c.Close()
		return nil, hostedError("WebSocket handshake", "the "+websocketProtocol+" subprotocol")
	}
	deadline, _ := ctx.Deadline()
	_ = c.SetDeadline(deadline)
	return c, nil
}

// browserConn is a net.Conn over one browser WebSocket. Reads cross message
// boundaries; each write becomes one or more binary messages.
//
// Go's wasm port runs JavaScript callbacks on the single thread that also
// runs goroutines, so the callbacks never block: they copy a message in,
// record the end, or signal, and return. No goroutine calls into JavaScript
// while it holds mu, so a callback never waits for it.
type browserConn struct {
	ws     js.Value
	url    string
	funcs  []js.Func
	detach sync.Once

	opened chan struct{} // closed when the socket opens
	closed chan struct{} // closed when the socket ends, locally or remotely
	ready  chan struct{} // capacity 1: a message or the end arrived
	once   struct{ open, end sync.Once }

	mu       sync.Mutex
	queue    [][]byte
	buffered int
	err      error // why the socket ended; reads return it once the queue drains

	writeMu     sync.Mutex
	read, write browserDeadline
}

func openBrowserWebSocket(address string) (*browserConn, error) {
	ctor := js.Global().Get("WebSocket")
	if ctor.Type() != js.TypeFunction {
		return nil, hostedError("WebSocket", "a browser WebSocket")
	}
	c := &browserConn{url: address, opened: make(chan struct{}), closed: make(chan struct{}), ready: make(chan struct{}, 1)}
	if err := jsTry(func() { c.ws = ctor.New(address, websocketProtocol) }); err != nil {
		return nil, localIOError("WebSocket dial", err)
	}
	c.ws.Set("binaryType", "arraybuffer")
	c.handle("onopen", func(js.Value) { c.once.open.Do(func() { close(c.opened) }) })
	c.handle("onmessage", c.message)
	c.handle("onclose", func(event js.Value) {
		c.end(fmt.Errorf("WebSocket closed with code %d: %w", event.Get("code").Int(), io.ErrUnexpectedEOF))
		c.release()
	})
	// A failure is always followed by a close event, which reports it.
	c.handle("onerror", func(js.Value) {})
	return c, nil
}

func (c *browserConn) handle(name string, fn func(js.Value)) {
	f := js.FuncOf(func(_ js.Value, args []js.Value) any {
		if len(args) > 0 {
			fn(args[0])
		}
		return nil
	})
	c.funcs = append(c.funcs, f)
	c.ws.Set(name, f)
}

// release detaches the callbacks, so the browser never calls a released
// function, then frees them. Release may run inside a callback.
func (c *browserConn) release() {
	c.detach.Do(func() {
		for _, name := range []string{"onopen", "onmessage", "onclose", "onerror"} {
			c.ws.Set(name, js.Null())
		}
		for _, f := range c.funcs {
			f.Release()
		}
	})
}

// message copies one received message into the read queue.
func (c *browserConn) message(event js.Value) {
	data := event.Get("data")
	if !data.InstanceOf(jsArrayBuffer) {
		c.abort(hostedError("WebSocket message", "binary data"))
		return
	}
	n := data.Get("byteLength").Int()
	if n > browserMaxMessage {
		c.abort(hostedError("WebSocket message", "a message within the relay envelope limit"))
		return
	}
	if n == 0 {
		return
	}
	b := make([]byte, n)
	js.CopyBytesToGo(b, jsUint8Array.New(data))
	c.mu.Lock()
	if c.err == nil && n > browserMaxBuffered-c.buffered {
		c.err = hostedError("WebSocket receive buffer", "relay messages read as fast as they arrive")
	}
	failed := c.err != nil
	if !failed {
		c.queue = append(c.queue, b)
		c.buffered += n
	}
	c.mu.Unlock()
	if failed {
		c.abort(nil)
		return
	}
	c.signal()
}

func (c *browserConn) signal() {
	select {
	case c.ready <- struct{}{}:
	default:
	}
}

// end records why the socket ended; the first reason wins.
func (c *browserConn) end(err error) {
	c.mu.Lock()
	if c.err == nil {
		c.err = err
	}
	c.mu.Unlock()
	c.once.end.Do(func() { close(c.closed) })
	c.signal()
}

// abort ends the socket for a protocol violation and closes it.
func (c *browserConn) abort(err error) {
	if err != nil {
		c.end(err)
	}
	_ = c.Close()
}

func (c *browserConn) failure() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.err
}

func (c *browserConn) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	for {
		expired := c.read.wait()
		select {
		case <-expired:
			return 0, os.ErrDeadlineExceeded
		default:
		}
		c.mu.Lock()
		if len(c.queue) > 0 {
			n := copy(p, c.queue[0])
			if c.queue[0] = c.queue[0][n:]; len(c.queue[0]) == 0 {
				c.queue[0] = nil
				c.queue = c.queue[1:]
			}
			c.buffered -= n
			c.mu.Unlock()
			return n, nil
		}
		err := c.err
		c.mu.Unlock()
		if err != nil {
			return 0, err
		}
		select {
		case <-c.ready:
		case <-c.closed:
		case <-expired:
			return 0, os.ErrDeadlineExceeded
		}
	}
}

func (c *browserConn) Write(p []byte) (int, error) {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	written := 0
	for len(p) != 0 {
		if err := c.awaitSendRoom(); err != nil {
			return written, err
		}
		n := min(len(p), browserMaxSend)
		message := jsUint8Array.New(n)
		js.CopyBytesToJS(message, p[:n])
		if err := jsTry(func() { c.ws.Call("send", message) }); err != nil {
			c.end(localIOError("WebSocket send", err))
			return written, c.failure()
		}
		written += n
		p = p[n:]
	}
	return written, nil
}

// awaitSendRoom waits, under the write deadline, until the socket is open
// and its send buffer has drained below the bound.
func (c *browserConn) awaitSendRoom() error {
	for {
		if err := c.failure(); err != nil {
			return err
		}
		expired := c.write.wait()
		select {
		case <-expired:
			return os.ErrDeadlineExceeded
		default:
		}
		// readyState 1 is OPEN; a closing socket discards what it is sent.
		if c.ws.Get("readyState").Int() != 1 {
			return net.ErrClosed
		}
		if c.ws.Get("bufferedAmount").Int() <= browserMaxUnsent {
			return nil
		}
		poll := time.NewTimer(browserSendPoll)
		select {
		case <-poll.C:
		case <-c.closed:
		case <-expired:
		}
		poll.Stop()
	}
}

// Close sends a normal closure, which the relay treats as the seat leaving.
func (c *browserConn) Close() error {
	c.end(net.ErrClosed)
	c.release()
	_ = jsTry(func() { c.ws.Call("close", 1000) })
	return nil
}

func (c *browserConn) LocalAddr() net.Addr  { return browserAddr("browser") }
func (c *browserConn) RemoteAddr() net.Addr { return browserAddr(c.url) }

func (c *browserConn) SetDeadline(t time.Time) error {
	c.read.set(t)
	c.write.set(t)
	return nil
}

func (c *browserConn) SetReadDeadline(t time.Time) error {
	c.read.set(t)
	return nil
}

func (c *browserConn) SetWriteDeadline(t time.Time) error {
	c.write.set(t)
	return nil
}

type browserAddr string

func (a browserAddr) Network() string { return "websocket" }
func (a browserAddr) String() string  { return string(a) }

// browserDeadline is one direction's deadline: wait returns a channel that
// closes when it passes, or nil (never ready) when none is set. Each set
// makes a fresh channel, so a superseded timer closes only its own.
type browserDeadline struct {
	mu    sync.Mutex
	timer *time.Timer
	fired chan struct{}
}

func (d *browserDeadline) set(t time.Time) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.timer != nil {
		d.timer.Stop()
		d.timer = nil
	}
	d.fired = nil
	if t.IsZero() {
		return
	}
	fired := make(chan struct{})
	d.fired = fired
	if wait := time.Until(t); wait > 0 {
		d.timer = time.AfterFunc(wait, func() { close(fired) })
	} else {
		close(fired)
	}
}

func (d *browserDeadline) wait() <-chan struct{} {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.fired
}

// jsTry runs fn and returns a JavaScript exception it throws as an error.
func jsTry(fn func()) (err error) {
	defer func() {
		if r := recover(); r != nil {
			e, ok := r.(js.Error)
			if !ok {
				panic(r)
			}
			err = e
		}
	}()
	fn()
	return nil
}
