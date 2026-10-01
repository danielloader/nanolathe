package main

import (
	"slices"
	"time"
)

// nlCadence paces the settings screen's background repaints
// (DESIGN_INTERFACE_HUD_INPUT §3.17 "Live preview"). A repaint lands on a
// whole number of display refreshes — its stride — so the pictures stay evenly
// spaced: the preferred rate rounded to the refresh, slowed while the measured
// repaint cost would take more than nlRepaintBudget of a refresh, and slowed
// further while display frames keep arriving late for any reason (a GPU or
// host that cannot keep up), never below nlCadenceFloor pictures a second. It
// is host presentation policy and touches no simulation state.
type nlCadence struct {
	intervals [nlCadenceSamples]time.Duration // recent display intervals, a ring
	n, at     int
	last      time.Time     // the previous display frame
	cost      time.Duration // smoothed CPU cost of one repaint (both pictures)

	// backoff is the extra stride late frames have earned; window and late
	// count the display frames of the current verdict, and calm the calm
	// verdicts in a row since the last change.
	backoff, window, late, calm int
}

const (
	nlCadenceSamples = 31
	// nlCadenceWindow display frames make one back-off verdict: more than a
	// tenth late adds a refresh to the stride, and three verdicts in a row
	// with under one late frame in fifty take one away.
	nlCadenceWindow = 90
	nlRepaintBudget = 0.35
	nlCadenceFloor  = 30
)

// observe records a display frame at now. A quiet frame — a scene loading,
// arriving or fading in — still measures the refresh but casts no vote on
// lateness, which those moments cause on their own.
func (c *nlCadence) observe(now time.Time, quiet bool) {
	if !c.last.IsZero() {
		if d := now.Sub(c.last); d > 0 && d < 250*time.Millisecond {
			c.intervals[c.at] = d
			c.at = (c.at + 1) % nlCadenceSamples
			c.n = min(c.n+1, nlCadenceSamples)
			if r := c.refresh(); !quiet && r > 0 {
				c.window++
				if d > r*3/2 {
					c.late++
				}
			}
		}
	}
	c.last = now
	if quiet {
		c.window, c.late = 0, 0
	}
	if c.window < nlCadenceWindow {
		return
	}
	switch {
	case c.late*10 > c.window:
		c.backoff++
		c.calm = 0
	case c.late*50 < c.window:
		if c.calm++; c.calm >= 3 && c.backoff > 0 {
			c.backoff--
			c.calm = 0
		}
	default:
		c.calm = 0
	}
	c.window, c.late = 0, 0
}

// refresh is the median recent display interval, zero until enough frames
// have been seen.
func (c *nlCadence) refresh() time.Duration {
	if c.n < 8 {
		return 0
	}
	s := make([]time.Duration, c.n)
	copy(s, c.intervals[:c.n])
	slices.Sort(s)
	return s[c.n/2]
}

// spent records one ordinary repaint's CPU time.
func (c *nlCadence) spent(d time.Duration) {
	if c.cost == 0 {
		c.cost = d
		return
	}
	c.cost += (d - c.cost) / 5
}

// stride is the display refreshes between repaints for a preferred rate fps
// (zero: every refresh), at refresh interval r.
func (c *nlCadence) stride(fps int, r time.Duration) int {
	k := 1
	if fps > 0 {
		k = max(1, int((time.Second/time.Duration(fps)+r/2)/r))
	}
	if budget := time.Duration(nlRepaintBudget * float64(r)); budget > 0 {
		k = max(k, int((c.cost+budget-1)/budget))
	}
	floor := max(1, int((time.Second/nlCadenceFloor+r/2)/r))
	return min(k+c.backoff, floor)
}

// due reports whether a repaint at now, lastDrawn having been the previous
// one, keeps the stride. Before the refresh is known it falls back to the
// preferred rate in wall-clock time.
func (c *nlCadence) due(now, lastDrawn time.Time, fps int) bool {
	r := c.refresh()
	if r <= 0 {
		return fps <= 0 || now.Sub(lastDrawn) >= time.Second/time.Duration(fps)-2*time.Millisecond
	}
	// Half a refresh of slack keeps a late or early display frame from
	// skipping a stride.
	return now.Sub(lastDrawn) >= time.Duration(c.stride(fps, r))*r-r/2
}
