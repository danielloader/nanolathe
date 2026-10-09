package lockstep

import "time"

// These are provisional host pacing limits, never inputs to a tick
// (DESIGN_MULTIPLAYER §16.5.2). The normal interval is the window host's own
// step period, so a grant due on a host step is never missed by rounding.
const (
	pacedReceiveLimit      = 32
	playoutInterval        = time.Second / 30
	playoutCatchupInterval = (time.Second + 32) / 33
	// Host steps land on display refreshes: on a 75 Hz display they alternate
	// 26.7 and 40 ms apart, and a window updating at 20 Hz takes two steps at
	// one instant. Lateness up to two normal intervals is that quantization,
	// not a stall, so it is retained; at most three catch-up deadlines then
	// fit in it.
	playoutLateness   = 2 * playoutInterval
	playoutBurstLimit = int(playoutLateness/playoutCatchupInterval) + 1
)

// playoutClock is used only by the host caller. Injecting now lets tests move
// monotonic host time without waiting or putting a clock in the simulation.
type playoutClock struct {
	now       func() time.Time
	next      time.Time
	running   bool
	waiting   bool
	waitSince time.Time
}

// due reports how many of depth received grants to run now. The window host
// calls Pump once per 30 Hz host step, and those steps land on display
// refreshes rather than on exact deadlines, so one call may owe several ticks.
func (p *playoutClock) due(depth int) int {
	now := p.now()
	if !p.running {
		// Keep one granted tick behind the first execution, at entry and
		// after an underrun. While running, that reserve may absorb jitter.
		if depth == 0 {
			p.waiting = false
			return 0
		}
		if depth == 1 {
			if !p.waiting {
				p.waiting, p.waitSince = true, now
			}
			// A peer's terminal ACK stops further grants. A lone final grant
			// must still run, so reserve refill may wait only two intervals.
			if now.Sub(p.waitSince) < 2*playoutInterval {
				return 0
			}
		}
		p.running, p.waiting, p.next = true, false, now
	}
	// Preserve phase across host-step quantization, but never bank a long
	// stall as a burst of owed simulation work: lateness beyond that
	// quantization is discarded.
	if now.Sub(p.next) > playoutLateness {
		p.next = now.Add(-playoutLateness)
	}
	n := 0
	for n < playoutBurstLimit && !now.Before(p.next) {
		if n == depth {
			p.running = false
			break
		}
		// Excess backlog drains at no more than 33 Hz.
		interval := playoutInterval
		if depth-n > 2 {
			interval = playoutCatchupInterval
		}
		p.next = p.next.Add(interval)
		n++
	}
	return n
}
