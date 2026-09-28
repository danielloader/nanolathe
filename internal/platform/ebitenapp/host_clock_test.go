package ebitenapp

import (
	"testing"
	"time"
)

func TestHostCadenceIndependentOfRefresh(t *testing.T) {
	for _, hz := range []int{20, 30, 60, 120, 144, 240} {
		var clock hostClock
		start := time.Unix(1, 0)
		bodies := 0
		for frame := 0; frame <= hz*10; frame++ {
			bodies += clock.advance(start.Add(time.Duration(frame) * time.Second / time.Duration(hz)))
		}
		if bodies != 301 { // One initialization, then ten seconds at 30 Hz.
			t.Fatalf("%d Hz produced %d host steps, want 301", hz, bodies)
		}
	}
}

func TestHostCadenceStallAndRecovery(t *testing.T) {
	var clock hostClock
	start := time.Unix(1, 0)
	clock.advance(start)
	if got := clock.advance(start.Add(time.Hour)); got != 5 {
		t.Fatalf("resume ran %d steps, want bounded catch-up of 5", got)
	}
	if got := clock.advance(start.Add(time.Hour)); got != 0 {
		t.Fatalf("discarded stall debt leaked into next frame: %d steps", got)
	}
	if got := clock.advance(start.Add(time.Hour + time.Second/30)); got != 1 {
		t.Fatalf("normal cadence after resume: got %d steps", got)
	}
}

func TestHostCadenceAndDeferredBodiesStayBalanced(t *testing.T) {
	var clock hostClock
	var ledger updateLedger
	start := time.Unix(1, 0)
	owed, bodies, issued := 0, 0, 0
	for frame := range 1201 {
		for range clock.advance(start.Add(time.Duration(frame) * time.Second / 120)) {
			issued++
			owed++
			for range ledger.call(true) {
				owed--
				bodies++
			}
		}
		// Exercise skipped presents: tails run at 30 Hz, with a one-second gap.
		if frame%4 == 0 && (frame < 400 || frame > 520) && ledger.tail() {
			owed--
			bodies++
		}
		if owed < 0 || owed > 1 {
			t.Fatalf("frame %d: %d inputs outstanding", frame, owed)
		}
	}
	if bodies+owed != issued || issued != 301 {
		t.Fatalf("issued %d, ran %d, pending %d", issued, bodies, owed)
	}
}

// A step whose ideal instant falls near the midpoint between two refreshes
// keeps landing on the same Update through refresh jitter, where rounding to
// the nearest Update moved it back and forth: two steps one refresh apart, then
// a gap of three (hostClock).
func TestHostCadenceHoldsItsPhaseThroughJitter(t *testing.T) {
	for _, hz := range []int{60, 120} {
		for _, phase := range []time.Duration{0, 4 * time.Millisecond, 8 * time.Millisecond, 12 * time.Millisecond} {
			var clock hostClock
			start := time.Unix(1, 0)
			refresh := time.Second / time.Duration(hz)
			// A refresh a few ppm fast, so the step phase drifts across every
			// alignment, plus ±1.5 ms of arrival jitter.
			clock.advance(start.Add(-phase))
			var stepsAt []int
			frames := hz * 60
			for frame := 1; frame <= frames; frame++ {
				jitter := time.Duration((frame*7919)%31-15) * 100 * time.Microsecond
				at := start.Add(time.Duration(frame)*refresh*99995/100000 + jitter)
				for range clock.advance(at) {
					stepsAt = append(stepsAt, frame)
				}
			}
			cadence := hz / 30
			irregular := 0
			for i := 1; i < len(stepsAt); i++ {
				if stepsAt[i]-stepsAt[i-1] != cadence {
					irregular++
				}
			}
			// 60 s at a 50 ppm drift moves the phase 3 ms: at most one slip.
			if irregular > 2 {
				t.Errorf("%d Hz, phase %v: %d irregular step intervals in a minute", hz, phase, irregular)
			}
			if got, want := len(stepsAt), 1800; got < want-2 || got > want+1 {
				t.Errorf("%d Hz, phase %v: %d steps in a minute, want %d", hz, phase, got, want)
			}
		}
	}
}

// Each step's ideal instant is one period after the last, whichever Update
// takes it and however many one Update takes, so the battle's presentation
// clock can be rebased on a grid that jitter, a dropped refresh or a catch-up
// never moves (§13.13). A step is never taken more than a refresh either side
// of its instant while the Updates keep coming.
func TestHostStepDueIsTheIdealGrid(t *testing.T) {
	const period = time.Second / presentationTPS
	var clock hostClock
	start := time.Unix(1, 0)
	clock.advance(start)
	refresh := time.Second / 120
	var dues []time.Time
	for frame := 1; frame <= 1200; frame++ {
		// Every seventh refresh dropped, every 97th a 70 ms stall, and ±1.5 ms
		// of arrival jitter.
		if frame%7 == 0 {
			continue
		}
		at := start.Add(time.Duration(frame)*refresh + time.Duration((frame*7919)%31-15)*100*time.Microsecond)
		if frame%97 == 0 {
			at = at.Add(70 * time.Millisecond)
			start = start.Add(70 * time.Millisecond)
		}
		n := clock.advance(at)
		for i := range n {
			due := clock.stepDue(at, n, i)
			if early, late := due.Sub(at), at.Sub(due); n == 1 && (early > refresh || late > 2*refresh) {
				t.Fatalf("frame %d: a step taken at %v for %v", frame, at.Sub(start), due.Sub(start))
			}
			dues = append(dues, due)
		}
	}
	for i := 1; i < len(dues); i++ {
		if d := dues[i].Sub(dues[i-1]); d != period {
			t.Fatalf("step %d is %v after the last, want %v", i, d, period)
		}
	}
}
