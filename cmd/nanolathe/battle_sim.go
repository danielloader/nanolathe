package main

import (
	"time"

	"github.com/nanolathe-gg/nanolathe/internal/client"
	"github.com/nanolathe-gg/nanolathe/internal/content"
	committedframe "github.com/nanolathe-gg/nanolathe/internal/frame"
	"github.com/nanolathe-gg/nanolathe/internal/platform/ebitenapp"
	"github.com/nanolathe-gg/nanolathe/internal/session"
)

// The asynchronous simulation (docs/DESIGN_GPU_RENDERER.md §13.13).
//
// The modern window runs the session's sub-ticks on their own goroutine so a
// tick no longer shares a presented frame's budget with recording, Execute and
// Ebitengine's present. The host keeps every decision on the game goroutine,
// in the order the synchronous step makes them:
//
//  1. At the start of a host step it joins the batch the previous step
//     launched and applies what the batch left for the client — its captions,
//     its message-ring retire, its feature admissions — then feeds each
//     publication the batch made, oldest first, to the observers that ran
//     inside the batch before (camera follow and shake, the Enhanced history
//     layers), runs the follow hotkeys that wait for the batch, and drains
//     audio — all with the session quiescent.
//  2. Input is handled and commands are queued exactly as before, and the
//     session's PrepareStep releases this pump's sub-ticks.
//  3. When the host step returns, ExecuteStep runs those sub-ticks on the
//     simulation goroutine until the next host step joins it: about 33 ms at
//     1x.
//
// Commands therefore reach the same sub-ticks they would synchronously, and
// the authoritative sequence is unchanged; only where the sub-ticks run moves.
// Presentation shows only ticks the host has joined — at 1x the tick the
// simulation finished while it computes the next, one tick behind the
// synchronous view — so everything the host applies at the join is in place
// before a publication is drawn, and a pre-recorded pass and the Draw that
// consumes it name the same pair.

// simRun is one launched batch.
type simRun struct {
	sess   *session.Session
	plan   session.StepPlan
	timing *simPhaseTimer
}

// battleSim is the simulation goroutine and the host's bookkeeping for it. All
// fields are owned by the game goroutine; the goroutine itself only receives
// runs and reports their duration.
type battleSim struct {
	runs chan simRun
	done chan time.Duration
	// running says a batch was launched and not yet joined; pending is the
	// plan this host step prepared, launched when the step returns.
	running bool
	pending *simRun
	// observed is the publication number the host last fed to its observers,
	// and observedTick that publication's tick; republished says it repeated
	// the tick before it, which only the paused-input boundary does.
	observed                   uint64
	observedTick               uint32
	observedValid, republished bool
	// terminal is copied only from a joined publication. No later sub-tick
	// can release the normal presentation delay once that result is latched.
	terminal bool
	// followAfterBatch holds follow hotkeys that retail handles after the
	// sub-tick batch; they run at the join that completes it.
	followAfterBatch func()
	// lastRun is the most recent batch's wall time on its goroutine.
	lastRun time.Duration
	// admissions are feature definitions a batch admitted, applied to the
	// model texture registry at the join, when no recording pass reads it.
	admissions []*content.FeatureDef
	// retireTick is the last tick of a batch's executor tail. The tail's
	// message-ring retire acts on the client's ring, which recording passes
	// read, so the batch only notes it and the join applies it.
	retireTick    uint32
	retirePending bool
	// timing times each batch by phase while a live trace runs, and
	// observerBefore is the session's phase observer it chained; launchedTicks
	// is the running batch's tick count (battle_sim_timing.go).
	timing         *simPhaseTimer
	observerBefore func(string, uint32)
	launchedTicks  int
}

func newBattleSim() *battleSim {
	r := &battleSim{runs: make(chan simRun), done: make(chan time.Duration)}
	go r.serve()
	return r
}

// noteRetire stands in for the message-ring retire in the executor tail while
// the simulation goroutine runs the pumps [01 R-PLAT-02 §8].
func (r *battleSim) noteRetire(lastTick uint32) bool {
	r.retireTick, r.retirePending = lastTick, true
	return false
}

func (r *battleSim) serve() {
	ebitenapp.RaiseCurrentThread()
	for run := range r.runs {
		started := time.Now()
		if run.timing != nil {
			run.timing.begin(started)
		}
		run.sess.ExecuteStep(run.plan)
		elapsed := time.Since(started)
		if run.timing != nil {
			run.timing.end(started.Add(elapsed))
		}
		r.done <- elapsed
	}
}

// syncSimulationMode starts or stops the simulation goroutine to match the
// client's presentation: only the modern window path sets AsyncSimulation.
// It runs at the start of a host step, after any running batch was joined.
func (b *battleSession) syncSimulationMode(cl *client.Client) {
	if b == nil || b.sess == nil {
		return
	}
	want := cl != nil && cl.AsyncSimulation() && b.sess.Snapshot != nil
	switch {
	case want && b.sim == nil:
		r := newBattleSim()
		r.observed = b.sess.Snapshot.PublicationSeq()
		if cur := b.sess.Snapshot.Current(); cur != nil {
			r.observedTick, r.observedValid = cur.Tick, true
			r.terminal = cur.Result.Ended
		}
		if b.shell != nil && b.shell.opts.LiveTrace != "" {
			r.traceSimulation(b.sess)
		}
		b.sim = r
		// The presentation clock starts again at the first prepared step.
		b.present = presentClock{}
		b.sess.BindMessageRetirement(r.noteRetire)
		// The pre-existing release stamp was taken after a synchronous step;
		// the next prepared pump restamps it at release.
	case !want && b.sim != nil:
		b.stopSimulation(cl)
	}
}

// joinSimulation waits for the batch the previous host step launched and
// applies, on the game goroutine, everything the synchronous step applied
// inside or right after that batch (see the file comment).
func (b *battleSession) joinSimulation(cl *client.Client) {
	if b == nil || b.sim == nil {
		return
	}
	r := b.sim
	if r.running {
		waitStarted := time.Now()
		r.lastRun = <-r.done
		r.running = false
		if cl != nil {
			cl.NoteSimulationTime(r.lastRun)
			cl.NoteSimulationJoin(time.Since(waitStarted), r.lastRun)
		}
		b.keepSlowBatch(r)
	}
	// In the synchronous order: captions a sub-tick's audio queue resolved,
	// then the tail's retire [01 R-PLAT-02 §8].
	if cl != nil {
		cl.DeferCaptions(false)
	}
	if r.retirePending {
		r.retirePending = false
		if cl != nil {
			cl.MessageRing().RetireOne(r.retireTick)
		}
	}
	for i, def := range r.admissions {
		b.modelTextures.AdmitFeatureDefinition(def)
		r.admissions[i] = nil
	}
	r.admissions = r.admissions[:0]
	if b.sess != nil && b.sess.Snapshot != nil {
		r.observed = b.sess.Snapshot.PublicationsSince(r.observed, func(f *committedframe.Frame) {
			r.republished = r.observedValid && f.Tick == r.observedTick
			r.observedTick, r.observedValid = f.Tick, true
			r.terminal = f.Result.Ended
			b.applyPublishedCamera(f)
			if cl != nil {
				cl.ObserveCommittedFrame(f)
			}
		})
	}
	if follow := r.followAfterBatch; follow != nil {
		r.followAfterBatch = nil
		follow()
	}
	if cl != nil {
		cl.TickPresentationAudio()
	}
}

// launchSimulation starts the batch this host step prepared. Until the join,
// captions the batch's audio queue resolves wait in the client.
func (b *battleSession) launchSimulation(cl *client.Client) {
	if b == nil || b.sim == nil || b.sim.pending == nil || b.sim.running {
		return
	}
	run := *b.sim.pending
	b.sim.pending = nil
	if cl != nil {
		cl.DeferCaptions(true)
	}
	b.sim.running = true
	b.sim.launchedTicks = run.plan.Ticks()
	b.sim.runs <- run
}

// stopSimulation joins any running batch as joinSimulation does and ends the
// goroutine. The session is then quiescent and back on the synchronous path.
func (b *battleSession) stopSimulation(cl *client.Client) {
	if b == nil || b.sim == nil {
		return
	}
	b.joinSimulation(cl)
	if b.sim.timing != nil && b.sess != nil {
		b.sess.PhaseObserver = b.sim.observerBefore
	}
	close(b.sim.runs)
	b.sim = nil
	if cl != nil {
		bindBattleMessageRetirement(b.sess, cl)
	} else if b.sess != nil {
		b.sess.BindMessageRetirement(nil)
	}
}

// prepareSimulationStep is the asynchronous half of the controller's step: it
// releases this pump's sub-ticks with PrepareStep, stamps the release for the
// presentation clock, and leaves ExecuteStep to the simulation goroutine. It
// returns how many sub-ticks the pump released. due is the step's ideal
// instant from the window's host clock, zero when there is none.
func (b *battleSession) prepareSimulationStep(scaled int32, due time.Time) int {
	plan := b.sess.PrepareStep(scaled)
	if plan.Ticks() > 0 {
		if b.millisSource == nil {
			b.millisSource = newMonotonicMillisSource()
		}
		// Stamped at release rather than when the batch finishes: presentation
		// paces the blend by when ticks became due, not by how long the batch
		// took on its goroutine.
		b.tickFiredAt = b.millisSource.Millis32()
		b.tickFiredCarry = b.sess.Clock.Carry
		b.tickFiredTick = b.sess.Clock.GlobalTick + uint32(plan.Ticks())
		b.tickFiredValid = true
	}
	b.simPaused = b.sess.Clock.Paused
	b.simActive = b.sess.Clock.Active
	ideal, late := b.presentStepMillis(due)
	b.notePresentStep(ideal, late, float64(b.sess.Clock.GlobalTick)+float64(plan.Ticks())+float64(b.sess.Clock.Carry), b.simActive)
	if !plan.Runs() {
		return plan.Ticks()
	}
	if b.sess.HasPendingHumanCommand(session.HumanGameplay) {
		// A gameplay switch reassigns rule state the HUD reads while it draws
		// (construction facings, the community switches), so the pump that
		// applies it runs here, before any recording pass; the join still
		// observes its publications.
		b.sess.ExecuteStep(plan)
		return plan.Ticks()
	}
	b.sim.pending = &simRun{sess: b.sess, plan: plan, timing: b.sim.timing}
	return plan.Ticks()
}
