//go:build retail

package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net"
	"os"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/nanolathe-gg/nanolathe/internal/content"
	"github.com/nanolathe-gg/nanolathe/internal/lockstep"
	"github.com/nanolathe-gg/nanolathe/internal/mission"
	"github.com/nanolathe-gg/nanolathe/internal/netproto"
	"github.com/nanolathe-gg/nanolathe/internal/orders"
	"github.com/nanolathe-gg/nanolathe/internal/pool"
	"github.com/nanolathe-gg/nanolathe/internal/relay"
	"github.com/nanolathe-gg/nanolathe/internal/session"
	"github.com/nanolathe-gg/nanolathe/internal/testsupport/retailcat"
)

// Opt-in wall-clock acceptance, kept outside ordinary fixture tests. The proxy
// delays independently scheduled byte chunks, never sleeping once per write
// on the sender or multiplying latency by the number of messages (§16.5.3).
func TestHostedRelayLatencyRetail(t *testing.T) {
	endpoint := os.Getenv("NANOLATHE_RELAY_ENDPOINT")
	if os.Getenv("NANOLATHE_RELAY_LATENCY") != "1" && endpoint == "" {
		t.Skip("set NANOLATHE_RELAY_LATENCY=1 for local delays or NANOLATHE_RELAY_ENDPOINT for a cloud test")
	}
	if endpoint != "" {
		if !strings.HasPrefix(endpoint, "wss://") {
			t.Fatal("the cloud test requires a certificate-verified wss:// endpoint")
		}
		if err := validateHostedAddress(Options{RelayAddress: endpoint}); err != nil {
			t.Fatal(err)
		}
	}
	websocket := os.Getenv("NANOLATHE_RELAY_WEBSOCKET") == "1"
	// 75 Hz is the default display because its host steps alternate 26.7 and
	// 40 ms apart, unlike 60 or 120 Hz.
	display := 75
	if v := os.Getenv("NANOLATHE_RELAY_DISPLAY_HZ"); v != "" {
		hz, err := strconv.Atoi(v)
		if err != nil || hz < 20 || hz > 480 {
			t.Fatal("NANOLATHE_RELAY_DISPLAY_HZ must be 20..480")
		}
		display = hz
	}
	cat, fs := retailcat.Shared(t)
	m, err := mission.LoadWithType(fs, mission.TypeSkirmish, "ashap plateau", 0, 2, nil)
	if err != nil {
		t.Fatal(err)
	}
	schema := -1
	for i, candidate := range cat.Maps[content.CanonicalKey(m.TerrainKey)].Schemas {
		if candidate.Name == m.Schema.Name {
			schema = i
			break
		}
	}
	if schema < 0 {
		t.Fatal("missing network schema")
	}
	config, err := localMultiplayerConfig(Options{Map: "ashap plateau", Seed: -1}, &contentSet{profile: "retail", limits: content.RetailLimits()}, uint32(schema))
	if err != nil {
		t.Fatal(err)
	}
	inputs, err := session.FreezeMatchInputs(fs, cat, config, nil)
	if err != nil {
		t.Fatal(err)
	}
	type latencyCase struct {
		name   string
		rtt    [2]time.Duration
		jitter time.Duration
		stall  bool
	}
	cases := []latencyCase{
		{"zero", [2]time.Duration{0, 0}, 0, false},
		{"50ms", [2]time.Duration{50 * time.Millisecond, 50 * time.Millisecond}, 0, false},
		{"100ms-jitter", [2]time.Duration{100 * time.Millisecond, 100 * time.Millisecond}, 10 * time.Millisecond, false},
		{"150ms", [2]time.Duration{150 * time.Millisecond, 150 * time.Millisecond}, 0, false},
		{"asymmetric-stall", [2]time.Duration{50 * time.Millisecond, 150 * time.Millisecond}, 10 * time.Millisecond, true},
	}
	if endpoint != "" {
		cases = []latencyCase{{name: "cloud"}}
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var server *relay.HostedServer
			if endpoint == "" {
				listen := relay.ListenHosted
				if websocket {
					listen = func(address string, config relay.HostedConfig) (*relay.HostedServer, error) {
						return relay.ListenHostedWebSocket(address, config, false)
					}
				}
				server, err = listen("127.0.0.1:0", relay.HostedConfig{InsecureLoopback: true})
				if err != nil {
					t.Fatal(err)
				}
				defer server.Close()
			}
			var worlds [2]*session.Session
			var drivers [2]*lockstep.LocalDriver
			var connections [2]*observedHostedClient
			var proxies [2]*hostedDelayProxy
			var actors [2]pool.Handle
			var room string
			for seat := range worlds {
				s, err := session.NewPlaytestSkirmish(inputs, config, uint8(seat), nil)
				if err != nil {
					t.Fatal(err)
				}
				if err := s.PrepareGrantedBattle(); err != nil {
					t.Fatal(err)
				}
				worlds[seat] = s
				for _, u := range s.Units.IterSliced() {
					if u.Alive && u.Owner == uint8(seat) {
						actors[seat] = u.Handle
						break
					}
				}
				address, dial := endpoint, relay.DialHostedWebSocket
				options := relay.HostedDialOptions{}
				if endpoint == "" {
					proxies[seat] = newHostedDelayProxy(t, server.Addr(), tc.rtt[seat]/2, tc.jitter)
					address, dial = proxies[seat].listener.Addr().String(), relay.DialHosted
					options.InsecureLoopback = true
					if websocket {
						address, dial = "ws://"+address+"/relay", relay.DialHostedWebSocket
					}
				}
				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				c, code, err := dial(ctx, address, room, relay.LocalHello{Seat: uint8(seat), Identity: netproto.Identity{Protocol: netproto.CommandSchemaVersion, Rules: netproto.RuleIdentity{Name: "modern"}}, InitialChecksum: s.UnitStateChecksum()}, options)
				cancel()
				if err != nil {
					t.Fatal(err)
				}
				room = code
				connections[seat] = &observedHostedClient{LocalClient: c, seat: uint8(seat), submitted: make(map[uint64]time.Time), assigned: make(map[uint32][]time.Time), checks: make(map[uint32][32]byte)}
				drivers[seat], err = lockstep.NewPacedDriver(s, connections[seat])
				if err != nil {
					t.Fatal(err)
				}
				defer drivers[seat].Close()
			}
			const warm, target = uint32(30), uint32(180)
			var began, finished [2]time.Time
			var beganTick, finishedTick, submittedAt [2]uint32
			stalled := false
			deadline := time.Now().Add(12 * time.Second)
			host := newProbeHost(display)
			defer host.ticker.Stop()
			for worlds[0].Clock.GlobalTick < target || worlds[1].Clock.GlobalTick < target {
				for range host.steps() {
					for seat, d := range drivers {
						s := worlds[seat]
						if s.Clock.GlobalTick >= target {
							continue
						}
						advanced, err := d.Pump()
						if err != nil {
							t.Fatal(err)
						}
						if !advanced {
							continue
						}
						tick := s.Clock.GlobalTick
						for _, receipt := range s.DrainCommandReceipts() {
							if receipt.Outcome == session.CommandRejected {
								t.Fatal(receipt.Diagnostic)
							}
						}
						// One host step may run several ticks.
						if tick >= warm && began[seat].IsZero() {
							began[seat], beganTick[seat] = time.Now(), tick
						}
						if tick >= target && finished[seat].IsZero() {
							finished[seat], finishedTick[seat] = time.Now(), tick
						}
						if tick >= warm && tick < target-10 && tick/5 != submittedAt[seat]/5 {
							submittedAt[seat] = tick
							u := s.Units.Unit(actors[seat])
							_, err = d.Submit(session.HumanCommand{Kind: session.HumanOrder, Order: session.HumanOrderCommand{Handles: []pool.Handle{u.Handle}, Code: 2, Position: orders.ResolvePos{X: u.X + 32<<16, Y: u.Y, Z: u.Z}}})
							if err != nil {
								t.Fatal(err)
							}
						}
					}
				}
				if tc.stall && !stalled && worlds[0].Clock.GlobalTick >= 75 {
					for _, p := range proxies {
						p.stall(250 * time.Millisecond)
					}
					stalled = true
				}
				if time.Now().After(deadline) {
					t.Fatal("hosted match stalled beyond the measurement deadline")
				}
			}
			// A host step may pass the target, so compare the acknowledged
			// checksum of that same tick rather than the replicas' last state.
			first, ok0 := connections[0].check(target)
			second, ok1 := connections[1].check(target)
			if !ok0 || !ok1 || first != second {
				t.Fatal("hosted replicas differ at the same checked tick")
			}
			for seat, c := range connections {
				rate := float64(finishedTick[seat]-beganTick[seat]) / finished[seat].Sub(began[seat]).Seconds()
				c.mu.Lock()
				samples := slices.Clone(c.samples)
				c.mu.Unlock()
				if len(samples) < 20 {
					t.Fatalf("seat %d only measured %d applied commands", seat, len(samples))
				}
				slices.Sort(samples)
				p95 := samples[(len(samples)*95+99)/100-1]
				t.Logf("seat=%d endpoint=%q display=%dHz injected-RTT=%s jitter=%s ticks/s=%.2f command-p95=%s samples=%d checksum=agreed stall=%v", seat, endpoint, display, tc.rtt[seat], tc.jitter, rate, p95.Round(time.Millisecond), len(samples), tc.stall)
				if !tc.stall && (rate < 28.5 || rate > 31.5) {
					t.Errorf("seat %d steady tick rate outside 30 Hz +/-5%%: %.2f", seat, rate)
				}
				if tc.name == "100ms-jitter" && p95 > 225*time.Millisecond {
					t.Errorf("seat %d command p95 %s exceeds 225 ms target", seat, p95)
				}
			}
			if tc.name == "zero" || tc.name == "cloud" {
				// Exercise actual terminal grants and delayed confirmation through
				// the host's result gate, not only isolated driver/wire fixtures.
				if _, err := drivers[1].Submit(session.HumanCommand{Kind: session.HumanSelfDestruct, SelfDestruct: session.HumanSelfDestructCommand{Handles: []pool.Handle{actors[1]}}}); err != nil {
					t.Fatal(err)
				}
				deadline = time.Now().Add(20 * time.Second)
				var terminal [2]uint32
				for !drivers[0].Completed() || !drivers[1].Completed() {
					if host.steps() == 0 {
						continue
					}
					for seat, d := range drivers {
						if _, err := d.Pump(); err != nil {
							t.Fatal(err)
						}
						s := worlds[seat]
						b := &battleSession{sess: s, multiplayer: &battleMultiplayer{driver: d, completed: d.Completed}}
						if s.OnlineBattleEnded() && terminal[seat] == 0 {
							terminal[seat] = s.Clock.GlobalTick
						}
						if terminal[seat] != 0 && s.Clock.GlobalTick != terminal[seat] {
							t.Fatal("surplus grant advanced the completed match")
						}
						if b.isResultVisible() != d.Completed() {
							t.Fatal("result visibility did not wait for relay confirmation")
						}
						if d.Completed() {
							b.multiplayer.failure = errors.New("late failure")
							if b.isResultVisible() {
								t.Fatal("transport failure awarded a result")
							}
						}
					}
					if time.Now().After(deadline) {
						t.Fatal("terminal agreement exceeded deadline")
					}
				}
				if terminal[0] != terminal[1] || worlds[0].UnitStateChecksum() != worlds[1].UnitStateChecksum() {
					t.Fatal("terminal replicas differ")
				}
				t.Logf("both results confirmed at terminal tick %d; surplus grants did not advance simulation", terminal[0])
			}
		})
	}
}

// probeHost reproduces the window host's cadence: 30 Hz steps, each on the
// display refresh nearest its ideal instant, never a convenient test rate.
type probeHost struct {
	ticker  *time.Ticker
	refresh time.Duration
	frames  int64
}

func newProbeHost(hz int) *probeHost {
	refresh := time.Second / time.Duration(hz)
	return &probeHost{ticker: time.NewTicker(refresh), refresh: refresh}
}

// steps waits for the next refresh and reports how many host steps it takes.
func (h *probeHost) steps() int {
	<-h.ticker.C
	h.frames++
	const period = int64(time.Second / 30)
	at := func(k int64) int64 { return (k*int64(h.refresh) + period/2) / period }
	return int(at(h.frames) - at(h.frames-1))
}

type observedHostedClient struct {
	*relay.LocalClient
	seat      uint8
	mu        sync.Mutex
	submitted map[uint64]time.Time
	assigned  map[uint32][]time.Time
	samples   []time.Duration
	checks    map[uint32][32]byte
}

func (c *observedHostedClient) check(tick uint32) ([32]byte, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	hash, ok := c.checks[tick]
	return hash, ok
}

func (c *observedHostedClient) Submit(payload []byte) (uint64, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	start := time.Now()
	seq, err := c.LocalClient.Submit(payload)
	if err == nil {
		c.submitted[seq] = start
	}
	return seq, err
}
func (c *observedHostedClient) ReadGrant() (relay.LocalGrant, error) {
	g, err := c.LocalClient.ReadGrant()
	if err != nil {
		return g, err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, command := range g.Commands {
		if command.Seat == c.seat {
			if start, ok := c.submitted[command.Sequence]; ok {
				c.assigned[g.Tick] = append(c.assigned[g.Tick], start)
				delete(c.submitted, command.Sequence)
			}
		}
	}
	return g, nil
}
func (c *observedHostedClient) Acknowledge(tick uint32, hash [32]byte, ended bool) error {
	c.mu.Lock()
	for _, start := range c.assigned[tick] {
		c.samples = append(c.samples, time.Since(start))
	}
	delete(c.assigned, tick)
	if tick%30 == 0 {
		c.checks[tick] = hash
	}
	c.mu.Unlock()
	return c.LocalClient.Acknowledge(tick, hash, ended)
}

type hostedDelayProxy struct {
	listener    net.Listener
	mu          sync.Mutex
	connections []net.Conn
	until       time.Time
	done        chan struct{}
}

func newHostedDelayProxy(t *testing.T, target string, delay, jitter time.Duration) *hostedDelayProxy {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	p := &hostedDelayProxy{listener: listener, done: make(chan struct{})}
	t.Cleanup(func() {
		close(p.done)
		listener.Close()
		p.mu.Lock()
		defer p.mu.Unlock()
		for _, c := range p.connections {
			c.Close()
		}
	})
	go func() {
		client, err := listener.Accept()
		if err != nil {
			return
		}
		upstream, err := net.Dial("tcp", target)
		if err != nil {
			client.Close()
			return
		}
		p.mu.Lock()
		select {
		case <-p.done:
			client.Close()
			upstream.Close()
			p.mu.Unlock()
			return
		default:
		}
		p.connections = []net.Conn{client, upstream}
		p.mu.Unlock()
		go p.forward(upstream, client, delay, jitter)
		p.forward(client, upstream, delay, jitter)
	}()
	return p
}
func (p *hostedDelayProxy) stall(d time.Duration) {
	p.mu.Lock()
	p.until = time.Now().Add(d)
	p.mu.Unlock()
}
func (p *hostedDelayProxy) forward(dst, src net.Conn, delay, jitter time.Duration) {
	type chunk struct {
		data []byte
		at   time.Time
	}
	queue := make(chan chunk, 128)
	go func() {
		defer close(queue)
		var buffer [64 << 10]byte
		count := 0
		for {
			n, err := src.Read(buffer[:])
			if n > 0 {
				j := time.Duration(0)
				switch count % 4 {
				case 0:
					j = -jitter
				case 2:
					j = jitter
				}
				count++
				wait := max(delay+j, 0)
				part := chunk{slices.Clone(buffer[:n]), time.Now().Add(wait)}
				select {
				case queue <- part:
				case <-p.done:
					return
				}
			}
			if err != nil {
				return
			}
		}
	}()
	defer dst.Close()
	for part := range queue {
		p.mu.Lock()
		until := p.until
		p.mu.Unlock()
		if until.After(part.at) {
			part.at = until
		}
		timer := time.NewTimer(max(time.Until(part.at), 0))
		select {
		case <-timer.C:
		case <-p.done:
			timer.Stop()
			return
		}
		if _, err := io.Copy(dst, bytes.NewReader(part.data)); err != nil {
			return
		}
	}
}
