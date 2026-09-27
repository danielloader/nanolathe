package main

import "testing"

// On the wall clock each host step's budget reads one scaled unit past the
// last, however the raw timebase dithers across a unit boundary, so every
// 1x step releases exactly one tick; a stall resynchronises to the raw
// timebase, a raw timebase behind the lock holds it, and an injected source is
// read raw (battle_controller.go stepScaled).
func TestStepScaledLocksTheBudgetSampleToTheHostStep(t *testing.T) {
	c := &BattleController{stepLocked: true}
	// Host steps whose raw sample dithers across the boundary: sampled raw
	// they would release 0, 2, 0, 2... ticks.
	raws := []int32{100, 100, 102, 102, 104, 105, 105, 107}
	want := []int32{100, 101, 102, 103, 104, 105, 106, 107}
	for i, raw := range raws {
		if got := c.stepScaled(raw); got != want[i] {
			t.Fatalf("step %d: raw %d read %d, want %d", i, raw, got, want[i])
		}
	}
	// A stall: the raw timebase is two or more units ahead.
	if got := c.stepScaled(115); got != 115 {
		t.Fatalf("after a stall the sample is %d, want the raw 115", got)
	}
	// Steps with no wall time passing release at most once, then hold.
	if a, b := c.stepScaled(115), c.stepScaled(115); a != 116 || b != 116 {
		t.Fatalf("steps without time read %d then %d, want 116 twice", a, b)
	}
	raw := &BattleController{}
	if got := raw.stepScaled(100); got != 100 {
		t.Fatalf("an injected source read %d, want the raw 100", got)
	}
	if got := raw.stepScaled(100); got != 100 {
		t.Fatalf("an injected source read %d on the second step, want the raw 100", got)
	}
}
