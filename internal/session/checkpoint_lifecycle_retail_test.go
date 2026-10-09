//go:build retail

package session

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"io"
	"reflect"
	"testing"

	"github.com/nanolathe-gg/nanolathe/internal/frame"
	"github.com/nanolathe-gg/nanolathe/internal/sim/checkpoint"
	"github.com/nanolathe-gg/nanolathe/internal/testsupport/retailcat"
)

func checkpointLifecycleBattle(t *testing.T) *Session {
	t.Helper()
	cat, fs := retailcat.Shared(t)
	cfg := DirectSkirmishConfig(admittedSkirmishMap)
	cfg.RNGSimSeed, cfg.RNGCrtSeed = 7, 7
	room := matchTestRoom()
	room.MapSchema = admittedRoomSchema(t, fs, cat, admittedSkirmishMap, cfg.NumPlayers)
	r, err := NewMatchConfigRequest(cfg, SkirmishEntryOptions{}, room)
	if err != nil {
		t.Fatal(err)
	}
	config := resolveMatch(t, r)
	inputs, err := FreezeMatchInputs(fs, cat, config, nil)
	if err != nil {
		t.Fatal(err)
	}
	s, err := NewAdmittedSkirmish(inputs, config, nil)
	if err != nil {
		t.Fatal(err)
	}
	for range 8 {
		if plan := s.PrepareStep(s.Clock.ScaledAnchor); plan.Runs() {
			return s
		}
	}
	t.Fatal("entry did not reach its first pump")
	return nil
}

func enableLifecycleBattle(t *testing.T, s *Session) {
	t.Helper()
	if !s.PublishOpeningFrame() {
		t.Fatal("opening publication")
	}
	beforeSim, beforeCRT := *s.SimRNG(), *s.CrtRNG()
	if err := s.EnableCheckpoints(); err != nil {
		t.Fatal(err)
	}
	if *s.SimRNG() != beforeSim || *s.CrtRNG() != beforeCRT || s.Clock.GlobalTick != 0 {
		t.Fatal("enabling capture changed tick or RNG")
	}
}

// Capture uses actual pump boundaries, including temporary sight whose expiry
// falls inside a catch-up batch [03 R-COMP-02 §2], not the publication observer.
func TestCheckpointLifecycleBoundariesRetail(t *testing.T) {
	s := checkpointLifecycleBattle(t)
	if err := s.EnableCheckpoints(); err == nil {
		t.Fatal("enabled before opening publication")
	}
	postLoopStateFor(s).eyeballs.records = []eyeballRecord{{owner: 0, expiry: 1}}
	enableLifecycleBattle(t, s)
	entry := s.CheckpointHistory()
	if len(entry.Records) != 1 || len(entry.Ticks) != 0 || entry.Records[0].Position != (CheckpointPosition{Boundary: CheckpointEntry}) {
		t.Fatalf("entry history: %+v", entry)
	}
	var out bytes.Buffer
	if err := s.RequestCheckpointCapture(&out); err != nil {
		t.Fatal(err)
	}
	if err := s.RequestCheckpointCapture(io.Discard); err == nil {
		t.Fatal("replaced pending sink")
	}
	// A drained accepted no-op still occupies the consumed-input sequence.
	if err := s.EnqueueHumanCommand(HumanCommand{Kind: HumanStop}); err != nil {
		t.Fatal(err)
	}
	s.ExecuteStep(StepPlan{run: true})
	if !s.CheckpointCaptureResult().Pending || out.Len() != 0 || !reflect.DeepEqual(entry, s.CheckpointHistory()) {
		t.Fatal("zero-tick pump completed or replaced a checkpoint")
	}
	s.ExecuteStep(StepPlan{run: true, ticks: 3})
	got := s.CheckpointHistory()
	if len(got.Ticks) != 3 || len(got.Records) != 1 {
		t.Fatalf("history lengths: %d records, %d rows; capture %v; state %v tick %d", len(got.Records), len(got.Ticks), s.CheckpointCaptureResult().Err, s.State, s.Clock.GlobalTick)
	}
	for i, row := range got.Ticks {
		boundary := CheckpointInteriorTick
		if i == 2 {
			boundary = CheckpointFinalPumpTick
		}
		want := CheckpointPosition{Tick: uint32(i + 1), Boundary: boundary, Pump: 2, ConsumedInput: 1}
		if row.Position != want {
			t.Fatalf("row %d position = %+v, want %+v", i, row.Position, want)
		}
	}
	// One eye contributes six selected words. It remains after tick 2's
	// publication, then the single pump tail removes it after tick 3.
	if got.Ticks[0].Owners[5].Words != got.Ticks[2].Owners[5].Words+6 ||
		got.Ticks[1].Owners[5].Words != got.Ticks[0].Owners[5].Words || s.EyeballCount() != 0 {
		t.Fatal("temporary-sight expiry was not captured at the actual tail")
	}
	result := s.CheckpointCaptureResult()
	if result.Pending || result.Err != nil || result.Record.Position != got.Ticks[0].Position ||
		result.Record.Digests.Full != sha256.Sum256(out.Bytes()) {
		t.Fatalf("requested bytes/result: %+v", result)
	}
	for s.Clock.GlobalTick < 30 {
		s.ExecuteStep(StepPlan{run: true, ticks: 1})
	}
	got = s.CheckpointHistory()
	if len(got.Records) != 2 || got.Records[1].Position.Tick != 30 || len(got.Ticks) != 30 {
		t.Fatal("30-tick digest cadence or per-tick history changed")
	}
	if s.CheckpointCaptureResult() != result {
		t.Fatal("automatic cadence replaced the requested byte result")
	}
	got.Ticks[0].Owners[0].Sum++
	got.Records[0].Position.Tick++
	if reflect.DeepEqual(got, s.CheckpointHistory()) {
		t.Fatal("history reader borrowed retained storage")
	}
	if err := s.RequestCheckpointCapture(io.Discard); err != nil {
		t.Fatal(err)
	}
	s.ExecuteStep(StepPlan{run: true, ticks: 1})
	if s.CheckpointCaptureResult().Record.Position.Tick != 31 || len(s.CheckpointHistory().Records) != 2 {
		t.Fatal("off-cadence request entered digest ring or failed to update result")
	}
	position := s.CheckpointHistory().Ticks[30].Position
	beforeSim, beforeCRT := *s.SimRNG(), *s.CrtRNG()
	if allocs := testing.AllocsPerRun(10, func() {
		if _, err := s.checkpointRingRow(position); err != nil {
			panic(err)
		}
	}); allocs != 0 {
		t.Fatalf("cheap row allocations = %v", allocs)
	}
	if *s.SimRNG() != beforeSim || *s.CrtRNG() != beforeCRT {
		t.Fatal("cheap row advanced RNG")
	}
}

type checkpointSinkFunc func([]byte) (int, error)

func (f checkpointSinkFunc) Write(p []byte) (int, error) { return f(p) }

func TestCheckpointLifecycleFailureDisableAndGameplayRetail(t *testing.T) {
	s, control := checkpointLifecycleBattle(t), checkpointLifecycleBattle(t)
	enableLifecycleBattle(t, s)
	if !control.PublishOpeningFrame() {
		t.Fatal("control opening publication")
	}
	failed := errors.New("authored sink failure")
	if err := s.RequestCheckpointCapture(checkpointSinkFunc(func([]byte) (int, error) { return 0, failed })); err != nil {
		t.Fatal(err)
	}
	s.ExecuteStep(StepPlan{run: true, ticks: 1})
	control.ExecuteStep(StepPlan{run: true, ticks: 1})
	if !errors.Is(s.CheckpointCaptureResult().Err, failed) || len(s.CheckpointHistory().Ticks) != 0 {
		t.Fatal("failed capture returned a digest or committed a ring row")
	}
	if err := s.RequestCheckpointCapture(io.Discard); err != nil {
		t.Fatal(err)
	}
	s.ExecuteStep(StepPlan{run: true, ticks: 1})
	control.ExecuteStep(StepPlan{run: true, ticks: 1})
	if result := s.CheckpointCaptureResult(); result.Err != nil || result.Record.Position.Tick != 2 {
		t.Fatalf("recovery: %+v", result)
	}
	if err := s.RequestCheckpointCapture(io.Discard); err != nil {
		t.Fatal(err)
	}
	history := s.AI[1].CheckpointApplicationHistory()
	s.DisableCheckpoints()
	if result := s.CheckpointCaptureResult(); result.Err == nil || result.Pending {
		t.Fatal("disable lost cancellation result")
	}
	if got := s.CheckpointHistory(); len(got.Records) != 0 || len(got.Ticks) != 0 {
		t.Fatal("disable retained rings")
	}
	if _, err := history.Snapshot(); err == nil || history.NextSerial() != 0 {
		t.Fatal("disable did not stop the application chain")
	}
	if attempt := history.BeginAttempt(3, 1, 0, func(*checkpoint.Encoder) error { panic("disabled history encoded intent") }); attempt != nil {
		t.Fatal("disabled attempt accepted")
	}
	if err := s.EnableCheckpoints(); err == nil {
		t.Fatal("late enable accepted")
	}
	for range 30 {
		s.ExecuteStep(StepPlan{run: true, ticks: 1})
		control.ExecuteStep(StepPlan{run: true, ticks: 1})
	}
	if admittedFingerprint(t, "disabled", "capture", s) != admittedFingerprint(t, "disabled", "control", control) ||
		*s.SimRNG() != *control.SimRNG() || *s.CrtRNG() != *control.CrtRNG() {
		t.Fatal("capture, failure or disabling changed gameplay")
	}
}

func TestCheckpointLifecycleReentryAndShortPumpRetail(t *testing.T) {
	t.Run("sink reentry", func(t *testing.T) {
		s := checkpointLifecycleBattle(t)
		enableLifecycleBattle(t, s)
		called := false
		if err := s.RequestCheckpointCapture(checkpointSinkFunc(func(p []byte) (int, error) {
			if !called {
				called = true
				s.Step(s.Clock.ScaledAnchor + 1)
				s.ExecuteStep(StepPlan{run: true, ticks: 1})
			}
			return len(p), nil
		})); err != nil {
			t.Fatal(err)
		}
		s.ExecuteStep(StepPlan{run: true, ticks: 1})
		if !called || s.Clock.GlobalTick != 1 || s.CheckpointCaptureResult().Err == nil || len(s.CheckpointHistory().Ticks) != 0 {
			t.Fatal("sink reentered simulation or returned a usable checkpoint")
		}
	})
	t.Run("actual terminal tick", func(t *testing.T) {
		s := checkpointLifecycleBattle(t)
		enableLifecycleBattle(t, s)
		postLoopStateFor(s).eyeballs.records = []eyeballRecord{{expiry: 0}}
		// Emulate the already-completed result transition at publication.
		s.publicationObserver = func(*frame.Frame) { s.State = StatePostBattle }
		if err := s.RequestCheckpointCapture(io.Discard); err != nil {
			t.Fatal(err)
		}
		s.ExecuteStep(StepPlan{run: true, ticks: 5})
		history := s.CheckpointHistory()
		if s.Clock.GlobalTick != 1 || s.EyeballCount() != 0 || len(history.Ticks) != 1 ||
			history.Ticks[0].Position.Boundary != CheckpointFinalPumpTick || s.CheckpointCaptureResult().Err != nil {
			t.Fatalf("shortened pump did not capture the final tick after tail: %+v, %v", history, s.CheckpointCaptureResult().Err)
		}
	})
}

func TestCheckpointLifecycleNoRestartUntickedRetail(t *testing.T) {
	s := checkpointLifecycleBattle(t)
	enableLifecycleBattle(t, s)
	s.DisableCheckpoints()
	if err := s.EnableCheckpoints(); err == nil {
		t.Fatal("stopped unticked entry restarted")
	}
}

func TestCheckpointLifecyclePanickingSinkRetail(t *testing.T) {
	s := checkpointLifecycleBattle(t)
	enableLifecycleBattle(t, s)
	if err := s.RequestCheckpointCapture(checkpointSinkFunc(func([]byte) (int, error) { panic("authored writer panic") })); err != nil {
		t.Fatal(err)
	}
	func() {
		defer func() {
			if recover() != "authored writer panic" {
				t.Fatal("writer panic not propagated")
			}
		}()
		s.ExecuteStep(StepPlan{run: true, ticks: 1})
	}()
	if s.checkpoints.inCapture || s.checkpoints.inPump || s.CheckpointCaptureResult().Err == nil || len(s.CheckpointHistory().Ticks) != 0 {
		t.Fatal("panic retained capture scope or usable row")
	}
	s.DisableCheckpoints()
	s.ExecuteStep(StepPlan{run: true, ticks: 1})
	if s.Clock.GlobalTick != 2 {
		t.Fatal("diagnostic panic stopped future gameplay")
	}
}

func TestCheckpointLifecycleWrappedTickIsNotEntryRetail(t *testing.T) {
	s := checkpointLifecycleBattle(t)
	s.Clock.GlobalTick = ^uint32(0)
	s.ExecuteStep(StepPlan{run: true, ticks: 1})
	if s.Clock.GlobalTick != 0 {
		t.Fatal("fixture did not wrap")
	}
	if err := s.EnableCheckpoints(); err == nil {
		t.Fatal("runtime tick zero mistaken for unticked entry")
	}
}

// A byte request is fulfilled at the first completed tick. Later automatic
// cadence in the same host pump must not detach the result from those bytes.
func TestCheckpointLifecycleRequestAcrossCadenceRetail(t *testing.T) {
	s := checkpointLifecycleBattle(t)
	enableLifecycleBattle(t, s)
	for range 5 {
		s.ExecuteStep(StepPlan{run: true, ticks: 5})
	}
	var out bytes.Buffer
	if err := s.RequestCheckpointCapture(&out); err != nil {
		t.Fatal(err)
	}
	s.ExecuteStep(StepPlan{run: true, ticks: 5})
	result := s.CheckpointCaptureResult()
	if result.Err != nil || result.Pending || result.Record.Position.Tick != 26 || result.Record.Position.Boundary != CheckpointInteriorTick || result.Record.Digests.Full != sha256.Sum256(out.Bytes()) {
		t.Fatalf("request result lost to later cadence: %+v", result)
	}
	history := s.CheckpointHistory()
	if len(history.Records) != 2 || history.Records[1].Position.Tick != 30 || history.Records[1].Position.Boundary != CheckpointFinalPumpTick {
		t.Fatal("automatic final-pump cadence was not retained independently")
	}
}
