package ebitenapp

import "time"

// hostClock keeps the existing 30 Hz host work independent of Ebitengine's
// refresh-paced input snapshots. This is window scheduling policy, not the
// simulation clock (DESIGN_GPU_RENDERER §13.5).
//
// Host steps can only land on Updates, which arrive once per display refresh,
// so each step is quantized to one of them. carry is the signed time since the
// ideal instant of the last step, so the long-run rate is exactly 30 Hz
// whatever the refresh; the choice is only which Update takes each step.
//
// Rounding each step to the nearest Update left a step whose ideal instant fell
// near the midpoint of two Updates to jitter: one step landed a refresh early,
// the remainder moved by a refresh to the opposite edge of its window, and the
// next jitter moved it back — two steps one refresh apart, then a gap one
// refresh too long, again and again while the phases stayed close. Everything
// paced by the step (the blended world and camera, §13.5) jumped ahead and then
// held at each such pair. The choice therefore has hysteresis: a step lands on
// the Update that keeps the cadence (every second at 60 Hz, every fourth at
// 120 Hz) while that leaves it within three quarters of a refresh of its ideal
// instant, and moves by one Update only past that, which leaves the remainder a
// quarter of a refresh from ideal on the other side, well inside the window.
type hostClock struct {
	last  time.Time
	carry time.Duration
	// frame is the smoothed interval between Updates, zero while unknown or
	// while Updates come slower than the steps; slow counts consecutive
	// Updates at least a step apart, and since how many Updates have passed
	// without a step.
	frame time.Duration
	slow  int
	since int
}

func (c *hostClock) advance(now time.Time) int {
	if c.last.IsZero() {
		c.last = now
		return 1 // Initialize the client before its first Draw.
	}
	elapsed := max(now.Sub(c.last), 0)
	c.last = now
	const period = time.Second / presentationTPS
	// Match the window scheduler's previous five-step catch-up bound. A long
	// stall must not trigger an unbounded burst of camera/menu steps on resume.
	c.carry = min(c.carry+elapsed, 5*period)
	switch {
	case elapsed >= period:
		// A stall is one long interval; Updates that stay this slow (a
		// throttled or hidden window) leave nothing to keep a cadence with.
		if c.slow++; c.slow >= 4 {
			c.frame = 0
		}
	case elapsed > 0:
		c.slow = 0
		if c.frame == 0 {
			c.frame = elapsed
		} else {
			c.frame += (elapsed - c.frame) / 8
		}
	}
	steps := c.due(period)
	c.carry -= time.Duration(steps) * period
	if steps == 0 {
		c.since++
	} else {
		c.since = 0
	}
	return steps
}

// stepDue is the ideal instant of step i of the n that the advance at now
// just took: carry is the signed time since the last one's, and the steps of
// one Update are a period apart. A step lands on an Update up to a refresh
// either side of it, and its body may run a refresh or two later still, at a
// modern Draw's tail (§13.10); the battle's presentation clock and budget
// sample read this instant instead, so that where a step happened to run does
// not move the presented world (§13.13).
func (c *hostClock) stepDue(now time.Time, n, i int) time.Time {
	const period = time.Second / presentationTPS
	return now.Add(-c.carry - time.Duration(n-1-i)*period)
}

// due is how many steps this Update takes. With no measured refresh faster
// than the steps it rounds to the closest step, as it always did; the signed
// remainder keeps display jitter around a boundary from alternating zero and
// two steps.
func (c *hostClock) due(period time.Duration) int {
	d := c.frame
	if d <= 0 || d >= period {
		return int((c.carry + period/2) / period)
	}
	// e is how late a step taken now would be against its ideal instant.
	e := c.carry - period
	cadence := int((period + d/2) / d)
	limit := -d * 3 / 4
	if c.since+1 < cadence {
		// Stepping ahead of the cadence must be clearly nearer the ideal.
		limit = -d / 4
	}
	if e < limit {
		return 0
	}
	return 1 + int(max(e+d/2, 0)/period)
}
