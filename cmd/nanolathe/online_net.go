package main

// Online battle measurements: the network overlay and the end-of-match summary
// (DESIGN_INTERFACE_HUD_INPUT "Online games"). The host wraps the grant stream
// to see grants arrive and its own commands assigned to ticks, and the pump
// reports the ticks it executed. These are host timings only; nothing here
// reaches the simulation [I6].

import (
	"fmt"
	"os"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/nanolathe-gg/nanolathe/internal/client"
	"github.com/nanolathe-gg/nanolathe/internal/lockstep"
	"github.com/nanolathe-gg/nanolathe/internal/relay"
	"github.com/nanolathe-gg/nanolathe/internal/session"
)

// onlineStallGap is the gap between executed ticks the host counts as a
// stall: three tick intervals at 30 ticks a second. A host measurement
// threshold, not a simulation value.
const onlineStallGap = 100 * time.Millisecond

// onlineLatencyWindow is how many recent orders the overlay's percentiles
// cover; onlineLatencyKeep bounds what the summary keeps.
const (
	onlineLatencyWindow = 64
	onlineLatencyKeep   = 1 << 14
)

// onlineNetStats wraps a battle's grant stream.
type onlineNetStats struct {
	lockstep.Client
	slot   uint8
	humans int

	mu        sync.Mutex
	submitted map[uint64]time.Time
	assigned  []onlineAssigned // own orders granted, awaiting execution
	received  uint32           // the newest grant's tick

	// Game-goroutine state from observe.
	started, firstTick, lastTick time.Time
	lastTickNo                   uint32
	recent                       []time.Time // execution times in the last second
	latency                      []float64   // milliseconds, newest last
	stalls                       int
	stalled                      time.Duration
	window                       time.Time
	windowTicks                  int
	worstRate                    float64
	haveWorst                    bool
	summarized                   bool
}

type onlineAssigned struct {
	tick uint32
	at   time.Time
}

func newOnlineNetStats(c lockstep.Client, slot uint8, humans int) *onlineNetStats {
	return &onlineNetStats{Client: c, slot: slot, humans: humans, submitted: map[uint64]time.Time{}}
}

// Submit records when this seat sent each order.
func (n *onlineNetStats) Submit(payload []byte) (uint64, error) {
	sequence, err := n.Client.Submit(payload)
	if err == nil {
		n.mu.Lock()
		n.submitted[sequence] = time.Now()
		n.mu.Unlock()
	}
	return sequence, err
}

// ReadGrant notes each grant and the tick this seat's orders were given.
func (n *onlineNetStats) ReadGrant() (relay.LocalGrant, error) {
	grant, err := n.Client.ReadGrant()
	if err == nil {
		n.mu.Lock()
		n.received = grant.Tick
		for _, c := range grant.Commands {
			if c.Seat != n.slot {
				continue
			}
			if at, ok := n.submitted[c.Sequence]; ok {
				delete(n.submitted, c.Sequence)
				n.assigned = append(n.assigned, onlineAssigned{tick: grant.Tick, at: at})
			}
		}
		n.mu.Unlock()
	}
	return grant, err
}

// observe runs after every pump on the game goroutine: executed ticks, their
// rate and stalls, and the latency of orders whose tick has now run.
func (n *onlineNetStats) observe(sess *session.Session, now time.Time) {
	if n == nil || sess == nil || sess.Clock == nil {
		return
	}
	if n.started.IsZero() {
		n.started = now
	}
	tick := sess.Clock.GlobalTick
	if tick != n.lastTickNo {
		if n.firstTick.IsZero() {
			n.firstTick, n.window = now, now
		} else if gap := now.Sub(n.lastTick); gap > onlineStallGap && !sess.OnlineBattleEnded() {
			n.stalls++
			n.stalled += gap
		}
		ran := int(tick - n.lastTickNo)
		for range ran {
			n.recent = append(n.recent, now)
		}
		n.windowTicks += ran
		n.lastTick, n.lastTickNo = now, tick
		n.mu.Lock()
		keep := n.assigned[:0]
		for _, a := range n.assigned {
			if a.tick <= tick {
				n.latency = append(n.latency, float64(now.Sub(a.at))/float64(time.Millisecond))
				continue
			}
			keep = append(keep, a)
		}
		n.assigned = keep
		n.mu.Unlock()
		if len(n.latency) > onlineLatencyKeep {
			n.latency = n.latency[len(n.latency)-onlineLatencyKeep:]
		}
	}
	cut := 0
	for cut < len(n.recent) && now.Sub(n.recent[cut]) > time.Second {
		cut++
	}
	n.recent = n.recent[cut:]
	if !n.firstTick.IsZero() && now.Sub(n.window) >= time.Second && !sess.OnlineBattleEnded() {
		rate := float64(n.windowTicks) / now.Sub(n.window).Seconds()
		if !n.haveWorst || rate < n.worstRate {
			n.worstRate, n.haveWorst = rate, true
		}
		n.window, n.windowTicks = now, 0
	}
}

// percentiles are the 50th and 95th of the newest count samples.
func onlinePercentiles(samples []float64, count int) (p50, p95 float64, n int) {
	if count > 0 && len(samples) > count {
		samples = samples[len(samples)-count:]
	}
	if len(samples) == 0 {
		return 0, 0, 0
	}
	sorted := slices.Clone(samples)
	slices.Sort(sorted)
	at := func(q float64) float64 { return sorted[min(len(sorted)-1, int(q*float64(len(sorted))))] }
	return at(0.5), at(0.95), len(sorted)
}

// overlayLines are the network overlay's text.
func (n *onlineNetStats) overlayLines(sess *session.Session) []string {
	n.mu.Lock()
	received := n.received
	n.mu.Unlock()
	tick := sess.Clock.GlobalTick
	buffered := 0
	if received > tick {
		buffered = int(received - tick)
	}
	p50, p95, orders := onlinePercentiles(n.latency, onlineLatencyWindow)
	lines := []string{
		fmt.Sprintf("Online  tick %d  %d ticks/s", tick, len(n.recent)),
		fmt.Sprintf("Buffered grants %d  stalls %d", buffered, n.stalls),
	}
	if orders == 0 {
		lines = append(lines, "Orders  no samples yet")
	} else {
		lines = append(lines, fmt.Sprintf("Orders  p50 %.0f ms  p95 %.0f ms", p50, p95))
	}
	lines = append(lines, fmt.Sprintf("Checksum sent at tick %d", tick/30*30))
	for slot := 0; slot < n.humans; slot++ {
		who := fmt.Sprintf("Player %d", slot+1)
		if uint8(slot) == n.slot {
			who += " (You)"
		}
		// TODO(question): a defeated seat that disconnected is "left", which
		// only the relay knows; the grant stream reports no departure. Settle
		// by a relay notice of finished seats (DESIGN_MULTIPLAYER §16.6.1).
		state := "playing"
		if r := sess.ResultForSeat(uint8(slot)); r.Ended {
			state = "defeated"
			if r.Kind == "victory" {
				state = "won"
			}
		}
		lines = append(lines, who+"  "+state)
	}
	return lines
}

// summary is the end-of-match line on standard error.
func (n *onlineNetStats) summary(sess *session.Session, ending string) string {
	duration := time.Duration(0)
	if !n.firstTick.IsZero() {
		duration = n.lastTick.Sub(n.firstTick)
	}
	ticks := uint32(0)
	if sess != nil && sess.Clock != nil {
		ticks = sess.Clock.GlobalTick
	}
	average := 0.0
	if duration > 0 {
		average = float64(ticks) / duration.Seconds()
	}
	p50, p95, orders := onlinePercentiles(n.latency, 0)
	var b strings.Builder
	fmt.Fprintf(&b, "nanolathe: online match %s after %s and %d ticks; ticks/s average %.1f", ending, duration.Round(100*time.Millisecond), ticks, average)
	if n.haveWorst {
		fmt.Fprintf(&b, ", worst %.1f", n.worstRate)
	}
	if orders > 0 {
		fmt.Fprintf(&b, "; order latency p50 %.0f ms, p95 %.0f ms over %d orders", p50, p95, orders)
	}
	fmt.Fprintf(&b, "; stalls %d (%.1f s)", n.stalls, n.stalled.Seconds())
	return b.String()
}

// netOverlayShown reports the network overlay: an online battle with +net, or
// with the FPS display.
func (b *battleSession) netOverlayShown() bool {
	if b == nil || b.multiplayer == nil || b.multiplayer.net == nil || b.sess == nil || b.sess.Clock == nil {
		return false
	}
	shown := b.netVisible
	if b.shell != nil {
		shown = b.shell.netVisible
	}
	return shown || b.fpsShown()
}

// drawOnlineNetwork draws the network overlay at the top left of the world
// view, in the console face the clock and +bps use.
func (h *retailBattleHUD) drawOnlineNetwork(c *client.Client, b *battleSession) {
	if h == nil || c == nil || h.console == nil || !b.netOverlayShown() {
		return
	}
	lines := b.multiplayer.net.overlayLines(b.sess)
	width := 0
	for _, line := range lines {
		width = max(width, client.MeasureText(h.console, line))
	}
	step := int(h.console.Height) + 1
	const x, y = 134, 38
	c.UIFillRect(x-3, y-2, width+6, len(lines)*step+3, h.guiColor(0))
	for i, line := range lines {
		c.UIText(h.console, line, x, y+i*step, h.guiColor(15))
	}
}

// summarizeOnline writes the end-of-match summary once, at the battle's exit.
func (m *battleMultiplayer) summarizeOnline(sess *session.Session) {
	if m == nil || m.net == nil || m.net.summarized {
		return
	}
	m.net.summarized = true
	ending := "left"
	switch {
	case m.failure != nil:
		ending = "stopped (" + onlineRefusalText(m.failure) + ")"
	case sess != nil && sess.OnlineBattleEnded():
		ending = "ended"
	case sess != nil && sess.ResultForSeat(sess.LocalOwner).Ended:
		ending = "left after this player's defeat"
	}
	fmt.Fprintln(os.Stderr, m.net.summary(sess, ending))
}
