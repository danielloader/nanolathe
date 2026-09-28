package main

import (
	"math"
	"testing"
	"time"
)

// presentTestMillis is a host millisecond source the test sets directly.
type presentTestMillis struct{ ms uint32 }

func (s *presentTestMillis) Millis32() uint32 { return s.ms }

// The lag is the smallest that never presents an unjoined tick: two ticks at
// the nominal speed, as before, and 2k plus the largest carry a speed keeps
// (battle_present.go).
func TestPresentationLagPerSpeed(t *testing.T) {
	for active, want := range map[int32]float64{10: 2, 20: 4, 15: 3.5, 5: 1.5, 11: 3.1, 1: 1.1} {
		if got := presentationLag(active); math.Abs(got-want) > 1e-9 {
			t.Errorf("presentationLag(%d) = %v, want %v", active, got, want)
		}
	}
}

// presentStep plays one prepared host step: the join made joined the newest
// presentable tick, and the release left the global tick at released.
func presentStep(b *battleSession, ms uint32, joined, released uint32, active int32) {
	b.millisSource.(*presentTestMillis).ms = ms
	b.sim.observedTick, b.sim.observedValid = joined, true
	b.simActive = active
	b.notePresentStep(b.presentMillis(), 0, float64(released), active)
}

// position is the world time a sample presents: the named tick less one, plus
// the fraction toward it.
func position(tick uint32, fraction float32) float64 { return float64(tick) - 1 + float64(fraction) }

// presented is the position a Draw presents now.
func presented(b *battleSession) float64 {
	tick, fraction, _ := b.presentationAt(0)
	return position(tick, fraction)
}

// Each presented frame moves the world by the time since the last one, at 1x
// and at 2x, where a host step releases two ticks: the pair then moves on part
// way through the step instead of the older tick easing in and then leaping
// (DESIGN_GPU_RENDERER §13.5).
func TestPresentationClockMovesEvenly(t *testing.T) {
	for _, c := range []struct {
		name   string
		active int32
		per    uint32 // ticks each step releases
	}{{"1x", 10, 1}, {"2x", 20, 2}} {
		t.Run(c.name, func(t *testing.T) {
			b := &battleSession{sim: &battleSim{}, millisSource: &presentTestMillis{}}
			if _, _, named := b.presentationAt(0); named {
				t.Fatal("named a tick before the first join")
			}
			var last float64
			frames := 0
			for step := uint32(0); step < 6; step++ {
				start := 1000 + step*100/3 // 30 Hz host steps
				released := 100 + (step+1)*c.per
				presentStep(b, start, released-c.per, released, c.active)
				for draw := uint32(0); draw < 4; draw++ { // 120 Hz Draws
					b.millisSource.(*presentTestMillis).ms = start + draw*25/3
					tick, fraction, named := b.presentationAt(0)
					if !named || tick > released-c.per {
						t.Fatalf("step %d draw %d named %d, %v; newest joined is %d", step, draw, tick, named, released-c.per)
					}
					at := position(tick, fraction)
					if frames > 0 {
						// Milliseconds are whole in this source, so a quarter step
						// is 8 or 9 of them.
						want := float64(c.per) / 4
						if d := at - last; math.Abs(d-want) > float64(c.per)*0.04 {
							t.Fatalf("step %d draw %d moved %.3f ticks, want %.3f", step, draw, d, want)
						}
					}
					last = at
					frames++
				}
			}
		})
	}
}

// A step's body runs when the window reaches it, a few milliseconds to a
// refresh or two after the step's ideal instant, and how late changes from one
// step to the next. The clock is rebased at the ideal instant and held back by
// the lateness recently seen, so the world moves by the wall time between
// Draws whatever the bodies do, and never names a tick not yet joined.
// Rebased at the body, as it was, the same schedule moved the world a third of
// a present interval out of step at every change in lateness.
func TestPresentationClockIgnoresStepLateness(t *testing.T) {
	src := &presentTestMillis{}
	b := &battleSession{sim: &battleSim{}, millisSource: src}
	lateness := []float64{2, 10, 2, 2, 10, 10, 2, 10, 2, 2, 2, 10, 2, 10, 10, 2}
	const period = 100.0 / 3
	step := 0
	var last, lastAt float64
	for draw := 0; draw < 2*len(lateness)-4; draw++ {
		at := 1005 + float64(draw)*50/3 // 60 Hz Draws
		for step < len(lateness) && 1000+float64(step)*period+lateness[step] <= at {
			// The join makes the previous release presentable; this release
			// leaves the global tick one further on.
			b.sim.observedTick, b.sim.observedValid = uint32(100+step), true
			b.simActive = 10
			b.notePresentStep(1000+float64(step)*period, lateness[step], float64(101+step), 10)
			step++
		}
		src.ms = uint32(at)
		tick, fraction, named := b.presentationAt(0)
		if !named || tick > b.sim.observedTick {
			t.Fatalf("draw %d named %d, %v; newest joined is %d", draw, tick, named, b.sim.observedTick)
		}
		pos := position(tick, fraction)
		// The first 10 ms body is later than the margin yet seen and holds the
		// world once; from the Draw after it every frame moves the world by the
		// time since the last, less what the margin gives back when a late
		// body raises it again.
		if at > 1000+period+10+50/3 {
			moved := (pos - last) / 0.03
			if want := float64(src.ms) - lastAt; math.Abs(moved-want) > 1.5 {
				t.Fatalf("draw %d moved the world %.2f ms in %.0f ms", draw, moved, want)
			}
		}
		last, lastAt = pos, float64(src.ms)
	}
	// A stall is not jitter: it holds the world, and the margin it leaves is
	// no larger than one 60 Hz refresh.
	b.notePresentStep(1000+float64(step)*period, 180, float64(101+step), 10)
	if m := b.presentMarginAt(1000 + float64(step)*period + 180); m > presentMarginCeiling+1e-9 {
		t.Fatalf("a 180 ms stall left a %.1f ms margin, want at most %.1f", m, presentMarginCeiling)
	}
}

// At 1x the pair is the tick before the last release throughout the step, as
// the window has always presented it, and the fraction is the elapsed share of
// the step.
func TestPresentationClockNominalPair(t *testing.T) {
	b := &battleSession{sim: &battleSim{}, millisSource: &presentTestMillis{}}
	presentStep(b, 1000, 10, 11, 10)
	for _, c := range []struct {
		ms       uint32
		fraction float32
	}{{1000, 0}, {1010, 0.3}, {1020, 0.6}, {1033, 0.99}} {
		b.millisSource.(*presentTestMillis).ms = c.ms
		tick, fraction, _ := b.presentationAt(0)
		if tick != 10 || math.Abs(float64(fraction-c.fraction)) > 1e-4 {
			t.Errorf("at %d ms presented %d + %.4f, want 10 + %.4f", c.ms, tick, fraction, c.fraction)
		}
	}
	// A late host step holds the newest joined tick rather than naming the one
	// its batch is still computing.
	b.millisSource.(*presentTestMillis).ms = 1040
	if tick, fraction, _ := b.presentationAt(0); tick != 10 || fraction != 1 {
		t.Errorf("late step presented %d + %v, want 10 held", tick, fraction)
	}
}

// A prediction samples the same clock at the instant it is for, and does not
// move what a Draw may present next.
func TestPresentationClockPredictsAhead(t *testing.T) {
	b := &battleSession{sim: &battleSim{}, millisSource: &presentTestMillis{}}
	presentStep(b, 1000, 10, 12, 20)
	tick, fraction, _ := b.presentationAt(25 * time.Millisecond / 3)
	if tick != 9 || math.Abs(float64(fraction)-0.5) > 1e-4 {
		t.Fatalf("prediction 8.3 ms ahead = %d + %v, want 9 + 0.5", tick, fraction)
	}
	if tick, fraction, _ := b.presentationAt(0); tick != 9 || fraction != 0 {
		t.Fatalf("Draw after the prediction presented %d + %v, want 9 + 0", tick, fraction)
	}
}

// A pause freezes the presented sample; a speed-up that needs a longer lag
// holds the world still until the clock reaches it again rather than
// replaying ticks; a slow-down eases the lag back instead of skipping ticks.
func TestPresentationClockPauseAndSpeedChanges(t *testing.T) {
	b := &battleSession{sim: &battleSim{}, millisSource: &presentTestMillis{}}
	presentStep(b, 1000, 10, 11, 10)
	b.millisSource.(*presentTestMillis).ms = 1016
	tick, fraction, _ := b.presentationAt(0)
	b.simPaused = true
	b.millisSource.(*presentTestMillis).ms = 1500
	if t2, f2, _ := b.presentationAt(0); t2 != tick || f2 != fraction {
		t.Fatalf("paused presentation moved from %d + %v to %d + %v", tick, fraction, t2, f2)
	}
	b.simPaused = false

	// 1x → 2x: the lag grows from two ticks to four.
	b = &battleSession{sim: &battleSim{}, millisSource: &presentTestMillis{}}
	presentStep(b, 1000, 10, 11, 10)
	b.millisSource.(*presentTestMillis).ms = 1033
	before := presented(b)
	presentStep(b, 1033, 11, 13, 20)
	for ms := uint32(1033); ms < 1100; ms += 4 {
		b.millisSource.(*presentTestMillis).ms = ms
		at := presented(b)
		if at < before-1e-9 {
			t.Fatalf("speed-up presented %.3f at %d ms after %.3f", at, ms, before)
		}
		before = at
		if ms == 1065 {
			presentStep(b, 1066, 13, 15, 20)
		}
	}

	// 2x → 1x: the lag eases from four ticks to two, so the world runs a little
	// fast and never jumps.
	presentStep(b, 1100, 15, 16, 10)
	b.millisSource.(*presentTestMillis).ms = 1100
	before = presented(b)
	next := uint32(1)
	for ms := uint32(1104); ms < 1900; ms += 4 {
		if at := 1100 + next*100/3; ms >= at {
			presentStep(b, at, 15+next, 16+next, 10)
			next++
		}
		b.millisSource.(*presentTestMillis).ms = ms
		at := presented(b)
		if d := at - before; d < 0 || d > 4*0.03*1.2+1e-9 {
			t.Fatalf("slow-down moved %.3f ticks in 4 ms at %d ms", d, ms)
		}
		before = at
	}
}
