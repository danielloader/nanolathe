package aikit

import (
	"bytes"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/nanolathe-gg/nanolathe/internal/platform/benchlock"
	"github.com/nanolathe-gg/nanolathe/internal/sim/checkpoint"
	"github.com/nanolathe-gg/nanolathe/internal/units"
)

type checkpointCostBrain struct {
	prepareEntered, prepareRelease chan struct{}
	thinkEntered, thinkRelease     chan struct{}
}

func (*checkpointCostBrain) Name() string { return "checkpoint-cost-barrier" }
func (b *checkpointCostBrain) Init(*Kit) {
	close(b.prepareEntered)
	<-b.prepareRelease
}
func (b *checkpointCostBrain) Think(k *Kit, _ *Obs) {
	close(b.thinkEntered)
	<-b.thinkRelease
	k.Rand.Uint32()
}

// This opt-in probe isolates the worker-sensitive controller leaf; the full
// Session writer is measured separately. Barriers hold preparation/think
// outstanding, not CPU-busy, and no application deadline is crossed. Entry,
// release and the final public Join are outside every measured operation
// (DESIGN_MULTIPLAYER §16.3.4 M3-C9 and §16.3.79).
func TestCheckpointHostCost(t *testing.T) {
	if os.Getenv("NANOLATHE_CHECKPOINT_COST") != "1" {
		t.Skip("set NANOLATHE_CHECKPOINT_COST=1 for checkpoint cost measurements")
	}
	lockPath, err := benchlock.Path()
	if err != nil {
		t.Fatal(err)
	}
	held, err := benchlock.Acquire(lockPath, func() { t.Log("waiting for benchmark lock") })
	if err != nil {
		t.Fatal(err)
	}
	defer held.Close()

	brain := &checkpointCostBrain{make(chan struct{}), make(chan struct{}), make(chan struct{}), make(chan struct{})}
	h := newCheckpointHost(t, brain, Persona{ThinkEvery: 10, Reaction: 3, APM: 60, Burst: 4, Async: true})
	var releasePrepare, releaseThink sync.Once
	defer func() {
		releasePrepare.Do(func() { close(brain.prepareRelease) })
		releaseThink.Do(func() { close(brain.thinkRelease) })
		h.Close()
	}()
	w, econ := units.NewSliced(4, nil), computerEconomy()
	h.Step(1, w, econ)
	<-brain.prepareEntered
	h.Step(10, w, econ)
	before := h.ControllerCheckpoint()
	if !before.DeadlinePresent || before.DeadlineTick != 13 || before.NextThinkTick != 20 || before.NextBatchSerial != 2 || before.ApplicationCount != 0 {
		t.Fatalf("expected an unapplied first batch at tick 10: %+v", before)
	}
	context := checkpointHostContext(t, h)
	type capture struct {
		data    []byte
		summary checkpoint.Summary
		err     error
	}
	read := func() capture {
		var out bytes.Buffer
		result := capture{}
		if result.err = h.WriteControllerCheckpoint(checkpoint.NewEncoder(&out), context); result.err == nil {
			result.err = h.AppendControllerCheckpointSummary(&result.summary)
		}
		result.data = out.Bytes()
		return result
	}
	// The timeout is only a deadlock guard: capture must finish while the
	// worker cannot pass its barrier. No measured timing has a threshold.
	readWithoutJoin := func() capture {
		t.Helper()
		done := make(chan capture, 1)
		go func() { done <- read() }()
		select {
		case result := <-done:
			if result.err != nil {
				t.Fatal(result.err)
			}
			return result
		case <-time.After(5 * time.Second):
			t.Fatal("checkpoint capture waited for the barrier-held worker")
			return capture{}
		}
	}
	want := readWithoutJoin()
	check := func() {
		t.Helper()
		got := readWithoutJoin()
		if !bytes.Equal(got.data, want.data) || got.summary != want.summary || h.ControllerCheckpoint() != before {
			t.Fatal("worker phase or cost measurement changed canonical controller state")
		}
	}
	var encoded bytes.Buffer
	encoded.Grow(len(want.data))
	operations := []struct {
		name string
		run  func() error
	}{
		{"full-writer-reused-buffer", func() error {
			encoded.Reset()
			return h.WriteControllerCheckpoint(checkpoint.NewEncoder(&encoded), context)
		}},
		{"selected-summary", func() error {
			var summary checkpoint.Summary
			return h.AppendControllerCheckpointSummary(&summary)
		}},
	}
	var baseline [2]testing.BenchmarkResult
	for phase, name := range []string{"preparation-outstanding-think-queued", "think-outstanding", "worker-completed-before-deadline"} {
		switch phase {
		case 1:
			releasePrepare.Do(func() { close(brain.prepareRelease) })
			<-brain.thinkEntered
		case 2:
			releaseThink.Do(func() { close(brain.thinkRelease) })
			h.Join() // Complete the released worker before timing; do not apply its batch.
		}
		check()
		for index, operation := range operations {
			var runErr error
			result := testing.Benchmark(func(b *testing.B) {
				b.ReportAllocs()
				for b.Loop() {
					if runErr = operation.run(); runErr != nil {
						b.Fatal(runErr)
					}
				}
			})
			if runErr != nil {
				t.Fatal(runErr)
			}
			t.Logf("controller-leaf phase=%s tick=10 deadline=13 canonical-bytes=%d %s: %s; %s", name, len(want.data), operation.name, result.String(), result.MemString())
			if phase == 0 {
				baseline[index] = result
			} else if result.AllocsPerOp() != baseline[index].AllocsPerOp() || result.AllocedBytesPerOp() != baseline[index].AllocedBytesPerOp() {
				t.Fatalf("%s allocation cost changed with worker phase: %s versus %s", operation.name, result.MemString(), baseline[index].MemString())
			}
			check()
		}
	}
}
