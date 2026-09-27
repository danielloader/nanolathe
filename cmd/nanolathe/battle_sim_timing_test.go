package main

import (
	"testing"
	"time"
)

// A slow batch is reported by phase, summed by name across its ticks, with
// the time after the last boundary as the tail; the phases account for the
// whole batch (battle_sim_timing.go).
func TestSimPhaseTimerAccountsForTheWholeBatch(t *testing.T) {
	var timer simPhaseTimer
	start := time.Now()
	timer.begin(start)
	for range 2 {
		timer.observe("movement")
		timer.observe("combat")
	}
	end := time.Now().Add(time.Millisecond)
	timer.end(end)
	rec := timer.record(90, 2, end.Sub(start))
	var sum float64
	names := []string{}
	for _, p := range rec.Phases {
		names = append(names, p.Phase)
		sum += p.Millis
	}
	if len(names) != 3 || names[0] != "movement" || names[1] != "combat" || names[2] != simTailPhase {
		t.Fatalf("phases %v", names)
	}
	if d := sum - rec.Millis; d < -0.01 || d > 0.01 {
		t.Fatalf("phases sum to %.3f ms of a %.3f ms batch", sum, rec.Millis)
	}
	var idle simPhaseTimer
	idle.observe("movement")
	if idle.used != 0 {
		t.Fatal("an inactive timer recorded a phase")
	}
}
