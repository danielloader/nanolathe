package aikit

import (
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/nanolathe-gg/nanolathe/internal/ai"
	"github.com/nanolathe-gg/nanolathe/internal/content"
	"github.com/nanolathe-gg/nanolathe/internal/orders"
	"github.com/nanolathe-gg/nanolathe/internal/pool"
	"github.com/nanolathe-gg/nanolathe/internal/sim/checkpoint"
)

func checkpointAttachIdentity() checkpoint.Identity {
	var id checkpoint.Identity
	id.Content[0], id.Config[0] = 0x11, 0x22
	return id
}
func checkpointAttachHost() *Host {
	m := &ai.Manager{Controller: ai.ControllerModern, BattleSeed: 37}
	h := NewHost(m, &countBrain{}, PersonaHard)
	m.Ext = h
	return h
}

func TestCheckpointAttachLifecycle(t *testing.T) {
	for _, tc := range []struct {
		name                 string
		initialized, pending bool
		serial, next         uint64
	}{
		{"unstarted", false, false, 0, 1}, {"initialized", true, false, 0, 1}, {"pending at tick zero", true, true, 1, 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := checkpointAttachHost()
			h.inited, h.checkpointDeadlinePresent = tc.initialized, tc.pending
			if tc.initialized {
				h.ex.m = h.m
			}
			h.nextThink = 19
			h.ex.tokens = -23
			h.ex.lastFill = 29
			h.persona.Skill = -7 // Attachment must not normalize even authored raw values.
			before := checkpointAttachObserve(h)
			if err := h.EnableCheckpointApplications(checkpointAttachIdentity(), &content.CheckpointKeys{}); err != nil {
				t.Fatal(err)
			}
			if h.checkpointHistory == nil || h.checkpointHistory != h.m.CheckpointApplicationHistory() {
				t.Fatal("host did not borrow the manager's single chain")
			}
			state, err := h.checkpointHistory.Snapshot()
			if err != nil || state.Kind != 2 || state.Player != 0 || state.Count != 0 || state.NextSerial != tc.next || state.Hash != checkpointHostInitialHash() || h.checkpointBatchSerial != tc.serial {
				t.Fatalf("attachment history=%+v serial=%d err=%v", state, h.checkpointBatchSerial, err)
			}
			brain := h.brain.(*countBrain)
			if brain.inits != 0 || brain.thinks != 0 {
				t.Fatal("attachment invoked the brain")
			}
			after := checkpointAttachObserve(h)
			before.history, before.managerHistory, before.serial = after.history, after.managerHistory, after.serial
			if !reflect.DeepEqual(after, before) {
				t.Fatal("attachment changed scheduling/executor/persona state")
			}
			var summary checkpoint.Summary
			if err := h.AppendControllerCheckpointSummary(&summary); err != nil {
				t.Fatal(err)
			}
			if err := h.EnableCheckpointApplications(checkpoint.Identity{}, &content.CheckpointKeys{}); err == nil {
				t.Fatal("repeated attachment accepted")
			}
			if after, err := h.checkpointHistory.Snapshot(); err != nil || after != state {
				t.Fatal("repeated attachment changed chain", err)
			}
		})
	}
	// The original before-construction route remains valid and cannot be reset.
	m := &ai.Manager{Controller: ai.ControllerModern}
	if err := m.EnableCheckpointApplications(checkpointAttachIdentity(), &content.CheckpointKeys{}); err != nil {
		t.Fatal(err)
	}
	h := NewHost(m, &countBrain{}, PersonaHard)
	m.Ext = h
	before := h.ControllerCheckpoint()
	if err := h.EnableCheckpointApplications(checkpoint.Identity{}, &content.CheckpointKeys{}); err == nil || h.ControllerCheckpoint() != before {
		t.Fatal("pre-start attachment was reset")
	}
}

// Only simulation-thread fields participate; no worker storage is observed.
type checkpointAttachObserved struct {
	manager, executorManager       *ai.Manager
	ext                            any
	history, managerHistory        *ai.ApplicationHistory
	initialized, applying, pending bool
	scope                          *checkpointCommand
	serial, executorSerial         uint64
	nextThink, deadline, lastFill  uint32
	tokens                         int64
	persona                        Persona
}

func checkpointAttachObserve(h *Host) checkpointAttachObserved {
	v := checkpointAttachObserved{manager: h.m, executorManager: h.ex.m, history: h.checkpointHistory, initialized: h.inited, applying: h.checkpointApplying, pending: h.checkpointDeadlinePresent, scope: h.ex.checkpointApplication, serial: h.checkpointBatchSerial, executorSerial: h.ex.checkpointBatchSerial, nextThink: h.nextThink, deadline: h.checkpointDeadlineTick, lastFill: h.ex.lastFill, tokens: h.ex.tokens, persona: h.persona}
	if h.m != nil {
		v.ext = h.m.Ext
		v.managerHistory = h.m.CheckpointApplicationHistory()
	}
	return v
}

func TestCheckpointAttachRefusalsAreAtomic(t *testing.T) {
	for _, tc := range []struct {
		name    string
		edit    func(*Host)
		nilKeys bool
	}{
		{"nil manager", func(h *Host) { h.m = nil }, false},
		{"nil Ext", func(h *Host) { h.m.Ext = nil }, false},
		{"typed nil Ext", func(h *Host) { h.m.Ext = (*Host)(nil) }, false},
		{"foreign Ext", func(h *Host) { h.m.Ext = &Host{} }, false},
		{"noncomparable Ext", func(h *Host) { h.m.Ext = []int{1} }, false},
		{"host history", func(h *Host) { h.checkpointHistory, _ = ai.NewApplicationHistory(checkpoint.Identity{}, 0, 2) }, false},
		{"manager history", func(h *Host) {
			if err := ai.EnableControllerCheckpointApplications(h.m, h, checkpoint.Identity{}, &content.CheckpointKeys{}); err != nil {
				t.Fatal(err)
			}
		}, false},
		{"host active", func(h *Host) { h.checkpointApplying = true }, false},
		{"executor scope", func(h *Host) { h.ex.checkpointApplication = &checkpointCommand{} }, false},
		{"executor serial", func(h *Host) { h.ex.checkpointBatchSerial = 7 }, false},
		{"host serial residue", func(h *Host) { h.checkpointBatchSerial = 9 }, false},
		{"missing executor manager", func(h *Host) { h.inited = true }, false},
		{"foreign executor manager", func(h *Host) { h.inited = true; h.ex.m = &ai.Manager{} }, false},
		{"premature executor manager", func(h *Host) { h.ex.m = h.m }, false},
		{"uninitialized pending deadline", func(h *Host) { h.checkpointDeadlinePresent = true }, false},
		{"classic manager", func(h *Host) { h.m.Controller = ai.ControllerClassic }, false},
		{"unknown manager", func(h *Host) { h.m.Controller = 255 }, false},
		{"invalid player", func(h *Host) { h.m.Player = 10 }, false},
		{"nil keys", func(*Host) {}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := checkpointAttachHost()
			tc.edit(h)
			before := checkpointAttachObserve(h)
			keys := &content.CheckpointKeys{}
			if tc.nilKeys {
				keys = nil
			}
			if err := h.EnableCheckpointApplications(checkpointAttachIdentity(), keys); err == nil {
				t.Fatal("invalid attachment accepted")
			}
			if !reflect.DeepEqual(checkpointAttachObserve(h), before) {
				t.Fatal("refusal changed host/manager state")
			}
		})
	}
	if err := (*Host)(nil).EnableCheckpointApplications(checkpointAttachIdentity(), &content.CheckpointKeys{}); err == nil {
		t.Fatal("nil host accepted")
	}
}

type checkpointAttachBrain struct {
	entered, release chan struct{}
	draws            []uint32
}

func (*checkpointAttachBrain) Name() string { return "checkpoint-attach" }
func (b *checkpointAttachBrain) Init(k *Kit) {
	if b.entered != nil {
		close(b.entered)
		<-b.release
	}
	b.draws = append(b.draws, k.Rand.Uint32())
}
func (b *checkpointAttachBrain) Think(k *Kit, o *Obs) {
	b.draws = append(b.draws, k.Rand.Uint32())
	if len(o.Own) > 0 {
		k.Move([]pool.Handle{o.Own[0].H}, 41, 43, false)
	}
}

func TestCheckpointAttachOmitsCompletedPrimeAttempts(t *testing.T) {
	e, w, u, _ := modernApplicationFixture(t, 0)
	h := NewHost(e.m, &checkpointAttachBrain{}, Persona{ThinkEvery: 10, Reaction: 0, APM: 60, Burst: 3})
	h.m.Ext = h
	defer h.Close()
	econ := computerEconomy()
	h.Step(0, w, econ)
	if h.ex.stats.Applied != 1 || h.checkpointDeadlinePresent || h.checkpointBatchSerial != 0 || orders.QueueForUnit(u).LenPrimary() != 1 {
		t.Fatal("fixture did not complete an ordinary prime command")
	}
	if err := h.EnableCheckpointApplications(checkpointAttachIdentity(), &content.CheckpointKeys{}); err != nil {
		t.Fatal(err)
	}
	state, err := h.checkpointHistory.Snapshot()
	if err != nil || state.Count != 0 || state.NextSerial != 1 || state.Hash != checkpointHostInitialHash() {
		t.Fatal("attachment fabricated prime history", state, err)
	}
	h.Step(10, w, econ)
	state, err = h.checkpointHistory.Snapshot()
	if err != nil || state.Count != 1 || state.NextSerial != 2 || state.Hash == checkpointHostInitialHash() || h.ex.stats.Applied != 2 {
		t.Fatal("next ordinary application did not start the fresh chain", state, err)
	}
}

type checkpointAttachOutcome struct {
	queue                  []int64
	unit                   [6]int64
	simState               uint32
	simDraws, privateState uint64
	privateDraws           []uint32
	stats                  ApplyStats
	tokens                 int64
	lastFill               uint32
}

func TestCheckpointAttachBlockedWorkerPreservesApplication(t *testing.T) {
	run := func(attach bool) checkpointAttachOutcome {
		t.Helper()
		e, w, u, _ := modernApplicationFixture(t, 0)
		brain := &checkpointAttachBrain{entered: make(chan struct{}), release: make(chan struct{})}
		h := NewHost(e.m, brain, Persona{ThinkEvery: 10, Reaction: 3, APM: 60, Burst: 3, Async: true})
		h.m.Ext = h
		var release sync.Once
		defer func() { release.Do(func() { close(brain.release) }); h.Close() }()
		econ := computerEconomy()
		h.Step(1, w, econ)
		select {
		case <-brain.entered:
		case <-time.After(5 * time.Second):
			t.Fatal("fixture worker did not enter preparation")
		}
		h.Step(10, w, econ) // A pending think is queued behind blocked preparation.
		if !h.inited || !h.checkpointDeadlinePresent || h.checkpointDeadlineTick != 13 || h.checkpointBatchSerial != 0 {
			t.Fatal("fixture did not schedule uninstrumented pending work")
		}
		if attach {
			done := make(chan error, 1)
			go func() {
				if err := h.EnableCheckpointApplications(checkpointAttachIdentity(), &content.CheckpointKeys{}); err != nil {
					done <- err
					return
				}
				var summary checkpoint.Summary
				done <- h.AppendControllerCheckpointSummary(&summary)
			}()
			select {
			case err := <-done:
				if err != nil {
					t.Fatal(err)
				}
			case <-time.After(5 * time.Second):
				t.Fatal("attachment or summary waited for the worker")
			}
			state, err := h.checkpointHistory.Snapshot()
			if err != nil || state.Count != 0 || state.NextSerial != 2 || state.Hash != checkpointHostInitialHash() || h.checkpointBatchSerial != 1 {
				t.Fatal("pending attachment did not reserve exactly serial 1", state, err)
			}
		}
		release.Do(func() { close(brain.release) })
		h.Step(13, w, econ) // Existing deadline join applies the now-instrumented batch.
		if h.ex.stats.Applied != 1 || h.checkpointDeadlinePresent {
			t.Fatal("pending application was not completed")
		}
		if attach {
			state, err := h.checkpointHistory.Snapshot()
			if err != nil || state.Count != 1 || state.NextSerial != 2 || state.Hash == checkpointHostInitialHash() {
				t.Fatal("pending application was not recorded normally", state, err)
			}
		}
		h.Join() // Outcome inspection, after the actual application, may read RNG.
		q := orders.QueueForUnit(u)
		if q.LenPrimary() != 1 {
			t.Fatal("fixture failed to change the live order queue")
		}
		result := checkpointAttachOutcome{unit: [6]int64{int64(u.Handle), int64(u.AllocationSerial), int64(u.Health), int64(u.X), int64(u.Y), int64(u.Z)}, simState: h.m.RNG.State, simDraws: h.m.RNG.Draws(), privateState: h.rand.Position(), privateDraws: append([]uint32(nil), brain.draws...), stats: h.ex.stats, tokens: h.ex.tokens, lastFill: h.ex.lastFill}
		for _, n := range q.Primary() {
			result.queue = append(result.queue, int64(n.ID), int64(n.Phase), int64(n.GoalX), int64(n.GoalY), int64(n.GoalZ), int64(n.Flags), int64(n.CreationTick))
		}
		return result
	}
	baseline := run(false)
	if got := run(true); !reflect.DeepEqual(got, baseline) {
		t.Fatalf("attachment changed world/RNG/application\ngot %+v\nwant %+v", got, baseline)
	}
	if len(baseline.privateDraws) != 2 {
		t.Fatal("comparison did not exercise private Init and Think draws")
	}
}
