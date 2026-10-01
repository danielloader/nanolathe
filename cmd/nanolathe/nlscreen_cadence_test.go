package main

import (
	"testing"
	"time"
)

// These lock the background cadence's arithmetic (DESIGN_INTERFACE_HUD_INPUT
// §3.17 "Live preview") with synthetic frame times; no device is involved.

func cadenceAt(hz int, frames int) (*nlCadence, time.Time) {
	c := &nlCadence{}
	at := time.Unix(1000, 0)
	r := time.Second / time.Duration(hz)
	for range frames {
		c.observe(at, false)
		at = at.Add(r)
	}
	return c, at
}

func TestNLCadenceStrideFollowsRateAndRefresh(t *testing.T) {
	for _, tc := range []struct {
		hz, fps, want int
	}{
		{120, 0, 1}, {120, 30, 4}, {120, 60, 2}, {120, 144, 1},
		{144, 30, 5}, {144, 60, 2}, {144, 120, 1}, {60, 60, 1}, {60, 30, 2}, {75, 60, 1},
	} {
		c, _ := cadenceAt(tc.hz, 20)
		r := c.refresh()
		if want := time.Second / time.Duration(tc.hz); r != want {
			t.Fatalf("%d Hz measured a %v refresh, want %v", tc.hz, r, want)
		}
		if got := c.stride(tc.fps, r); got != tc.want {
			t.Fatalf("%d FPS at %d Hz: stride %d, want %d", tc.fps, tc.hz, got, tc.want)
		}
	}
}

func TestNLCadenceSlowsForCostButNotBelowItsFloor(t *testing.T) {
	c, _ := cadenceAt(120, 20)
	r := c.refresh()
	c.spent(2 * time.Millisecond) // under 35% of 8.3 ms
	if got := c.stride(0, r); got != 1 {
		t.Fatalf("an affordable repaint had stride %d", got)
	}
	c.cost = 4 * time.Millisecond
	if got := c.stride(0, r); got != 2 {
		t.Fatalf("a 4 ms repaint at 120 Hz had stride %d, want 2", got)
	}
	c.cost = time.Second
	if got := c.stride(0, r); got != 4 {
		t.Fatalf("an unaffordable repaint had stride %d, want the 30 FPS floor of 4", got)
	}
}

func TestNLCadenceBacksOffWhileFramesAreLate(t *testing.T) {
	c, at := cadenceAt(120, 20)
	r := time.Second / 120
	frame := func(late, quiet bool) {
		if late {
			at = at.Add(r)
		}
		at = at.Add(r)
		c.observe(at, quiet)
	}
	// Quiet frames never vote, however late.
	for range 3 * nlCadenceWindow {
		frame(true, true)
	}
	if c.backoff != 0 {
		t.Fatal("quiet frames moved the back-off")
	}
	for i := range nlCadenceWindow {
		frame(i%5 == 0, false)
	}
	if c.backoff != 1 || c.stride(0, c.refresh()) != 2 {
		t.Fatalf("a fifth of the frames late left back-off %d", c.backoff)
	}
	for range 2 * nlCadenceWindow {
		frame(false, false)
	}
	if c.backoff != 1 {
		t.Fatal("two calm verdicts already removed the back-off")
	}
	for range nlCadenceWindow {
		frame(false, false)
	}
	if c.backoff != 0 {
		t.Fatal("three calm verdicts kept the back-off")
	}
}

func TestNLCadenceDueKeepsWholeRefreshes(t *testing.T) {
	c, now := cadenceAt(120, 20)
	r := time.Second / 120
	last := now
	// 60 FPS at 120 Hz repaints every second refresh, through half a refresh
	// of jitter either way.
	if c.due(last.Add(r+r/3), last, 60) {
		t.Fatal("a late first refresh repainted early")
	}
	if !c.due(last.Add(2*r-r/3), last, 60) {
		t.Fatal("an early second refresh missed its repaint")
	}
	// Before the refresh is known, the preferred rate holds in wall time.
	fresh := &nlCadence{}
	if fresh.due(last.Add(time.Second/60), last, 30) || !fresh.due(last.Add(time.Second/30), last, 30) {
		t.Fatal("the wall-clock fallback did not keep 30 FPS")
	}
}
