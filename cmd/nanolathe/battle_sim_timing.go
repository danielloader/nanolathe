package main

import (
	"time"

	"github.com/nanolathe-gg/nanolathe/internal/session"
)

// Live-trace timing of the simulation goroutine's batches
// (docs/BATTLE_BENCHMARK.md "Live window trace"). While a trace runs, the
// session's host phase observer [01 §4.4] stamps each phase boundary on the
// simulation goroutine, and the join keeps any batch long enough to matter,
// by phase, until the trace's census takes it. The observer is host-side by
// contract: it reads no authoritative state and decides nothing [I6].

// slowSimBatch is the batch time from which the trace keeps a batch's phases:
// half of the 33 ms a batch has at 1x before the next host step waits for it.
const slowSimBatch = 16 * time.Millisecond

// slowSimKept bounds the batches held between two census samples.
const slowSimKept = 32

// simTailPhase names the time after the last phase boundary of a batch: the
// executor tail's sharing, result evaluation and publication [01 §4.4].
const simTailPhase = "tail-sharing-result-publication"

// simPhaseTimer accumulates one batch's wall time by phase. Only the
// simulation goroutine writes it while a batch runs; the game goroutine reads
// it after joining the batch, so the join's channel receive orders the two.
// Phases are summed by name across the ticks of a batch, and in a batch of
// several ticks the time between one tick's last boundary and the next tick's
// first lands in that first phase.
type simPhaseTimer struct {
	names  [16]string
	totals [16]time.Duration
	used   int
	last   time.Time
	active bool
}

func (t *simPhaseTimer) begin(now time.Time) {
	*t = simPhaseTimer{last: now, active: true}
}

// observe is the session's PhaseObserver while a trace runs. A pump the host
// executes on the game goroutine (a gameplay switch) runs with the timer
// inactive and is not timed.
func (t *simPhaseTimer) observe(phase string) {
	if !t.active {
		return
	}
	now := time.Now()
	i := 0
	for i < t.used && t.names[i] != phase {
		i++
	}
	if i == t.used {
		if t.used < len(t.names)-1 {
			t.names[i] = phase
			t.used++
		} else {
			i = t.used - 1
		}
	}
	t.totals[i] += now.Sub(t.last)
	t.last = now
}

func (t *simPhaseTimer) end(now time.Time) {
	if !t.active {
		return
	}
	t.active = false
	tail := len(t.names) - 1
	t.names[tail] = simTailPhase
	t.totals[tail] = now.Sub(t.last)
}

// slowSimRecord is one slow batch as the census reports it.
type slowSimRecord struct {
	Tick   uint32         `json:"tick"`
	Ticks  int            `json:"ticks"`
	Millis float64        `json:"ms"`
	Phases []slowSimPhase `json:"phases"`
}

type slowSimPhase struct {
	Phase  string  `json:"phase"`
	Millis float64 `json:"ms"`
}

func (t *simPhaseTimer) record(tick uint32, ticks int, elapsed time.Duration) slowSimRecord {
	out := slowSimRecord{Tick: tick, Ticks: ticks, Millis: millis(elapsed)}
	for i, name := range t.names {
		if name != "" {
			out.Phases = append(out.Phases, slowSimPhase{Phase: name, Millis: millis(t.totals[i])})
		}
	}
	return out
}

func millis(d time.Duration) float64 { return float64(d.Microseconds()) / 1000 }

// traceSimulation installs the phase timer on a new simulation goroutine's
// session when a live trace is running, chaining any observer already there.
func (r *battleSim) traceSimulation(sess *session.Session) {
	timing := &simPhaseTimer{}
	previous := sess.PhaseObserver
	r.timing, r.observerBefore = timing, previous
	sess.PhaseObserver = func(phase string, tick uint32) {
		timing.observe(phase)
		if previous != nil {
			previous(phase, tick)
		}
	}
}

// keepSlowBatch runs at the join: a batch over slowSimBatch is kept for the
// census, the oldest dropped past slowSimKept.
func (b *battleSession) keepSlowBatch(r *battleSim) {
	if r.timing == nil || r.lastRun < slowSimBatch {
		return
	}
	if len(b.slowSim) >= slowSimKept {
		b.slowSim = b.slowSim[1:]
	}
	b.slowSim = append(b.slowSim, r.timing.record(b.sess.Clock.GlobalTick, r.launchedTicks, r.lastRun))
}

// takeSlowBatches hands the kept batches to the census and forgets them.
func (b *battleSession) takeSlowBatches() []slowSimRecord {
	out := b.slowSim
	b.slowSim = nil
	return out
}
