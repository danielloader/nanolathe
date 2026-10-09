package session

import (
	"io"
	"math"

	"github.com/nanolathe-gg/nanolathe/internal/ai"
	"github.com/nanolathe-gg/nanolathe/internal/content"
	"github.com/nanolathe-gg/nanolathe/internal/sim/checkpoint"
)

// This state belongs to the owning simulation thread and never enters a digest.
// Histories retain values only; pending is borrowed until one synchronous write
// (DESIGN_MULTIPLAYER §16.3.6, §16.3.78).
type sessionCheckpoints struct {
	summaries                  [CheckpointOwnerCount]checkpoint.Summary
	enabled, inPump, inCapture bool
	keys                       *content.CheckpointKeys
	applications               [10]*ai.ApplicationHistory
	history                    checkpointHistoryRing
	result                     CheckpointCaptureResult
	pending                    io.Writer
	pump, consumed             uint64
	scopeErr                   error
}

func (s *Session) EnableCheckpoints() error {
	if s == nil {
		return runtimeCheckpointError("capture.enable", "an admitted session")
	}
	if c := s.checkpoints; c != nil {
		if c.inPump || c.inCapture {
			return s.checkpointReentry("enable")
		}
		return runtimeCheckpointError("capture.enable", "a fresh admitted entry with no prior enable attempt")
	}
	if s.checkpointBindingAuthority() == nil || !s.checkpointAdmission.ready || s.checkpointAdmission.ticked || s.Clock == nil || s.Clock.GlobalTick != 0 || !s.rngInitialized || s.Snapshot == nil {
		return runtimeCheckpointError("capture.enable", "completed unticked admitted entry")
	}
	if tick, ok := s.Snapshot.PublishedTick(); !ok || tick != 0 {
		return runtimeCheckpointError("capture.enable", "the host's completed opening publication")
	}
	keys, err := s.checkpointAdmission.inputs.CheckpointKeys()
	if err != nil {
		return err
	}
	c := &sessionCheckpoints{keys: keys, inCapture: true}
	s.checkpoints = c
	stop := func(err error) error {
		for _, h := range c.applications {
			h.Fail(err)
		}
		c.inCapture = false
		c.result = CheckpointCaptureResult{Err: err}
		return err
	}
	for player, m := range s.AI {
		if m == nil {
			continue
		}
		if m.Controller == ai.ControllerModern {
			if !s.checkpointModernPlannerMatches(m.Planner) {
				return stop(runtimeCheckpointError("capture.enable.modern", "the registered Modern planner"))
			}
			err = s.modernAICheckpointSource.EnableApplications(m, s.checkpointAdmission.identity, keys)
		} else {
			err = m.EnableCheckpointApplications(s.checkpointAdmission.identity, keys)
		}
		if err != nil {
			return stop(err)
		}
		c.applications[player] = m.CheckpointApplicationHistory()
	}
	record, err := s.captureCheckpoint(keys, CheckpointPosition{Boundary: CheckpointEntry}, nil)
	if err != nil {
		return stop(err)
	}
	c.inCapture, c.enabled = false, true
	c.history.appendRecord(record)
	c.result = CheckpointCaptureResult{Record: record}
	return nil
}

func (s *Session) DisableCheckpoints() {
	if s == nil || s.checkpoints == nil {
		return
	}
	c := s.checkpoints
	if c.inPump || c.inCapture {
		_ = s.checkpointReentry("disable")
		return
	}
	err := runtimeCheckpointError("capture.disabled", "a fresh admitted entry to start a new history")
	for _, h := range c.applications {
		h.Fail(err)
	}
	result := CheckpointCaptureResult{}
	if c.pending != nil {
		result.Err = runtimeCheckpointError("capture.request", "an enabled session; pending capture canceled")
	}
	s.checkpoints = &sessionCheckpoints{result: result}
}

func (s *Session) RequestCheckpointCapture(out io.Writer) error {
	if s == nil || s.checkpoints == nil || !s.checkpoints.enabled {
		return runtimeCheckpointError("capture.request", "enabled checkpoints")
	}
	c := s.checkpoints
	if c.inPump || c.inCapture {
		return s.checkpointReentry("request")
	}
	if out == nil || c.pending != nil {
		return runtimeCheckpointError("capture.request", "one nonnil output sink with no request already pending")
	}
	c.pending = out
	c.result = CheckpointCaptureResult{Pending: true}
	return nil
}

func (s *Session) CheckpointCaptureResult() CheckpointCaptureResult {
	if s == nil || s.checkpoints == nil {
		return CheckpointCaptureResult{}
	}
	return s.checkpoints.result
}

func (s *Session) CheckpointHistory() CheckpointHistory {
	if s == nil || s.checkpoints == nil {
		return CheckpointHistory{}
	}
	return s.checkpoints.history.snapshot()
}

func (s *Session) checkpointReentry(operation string) error {
	err := runtimeCheckpointError("capture."+operation, "a non-reentrant owning-thread operation between pumps")
	if s != nil && s.checkpoints != nil {
		c := s.checkpoints
		if c.scopeErr == nil {
			c.scopeErr = err
		}
		c.result = CheckpointCaptureResult{Err: err}
	}
	return err
}

// Call at the queue's actual drain boundary, including refused/no-op inputs.
func (s *Session) checkpointConsumedInput() {
	c := s.checkpoints
	if c == nil || !c.enabled {
		return
	}
	if c.consumed == math.MaxUint64 {
		c.scopeErr = runtimeCheckpointError("capture.input", "a representable consumed-input ordinal")
		c.result = CheckpointCaptureResult{Err: c.scopeErr}
		return
	}
	c.consumed++
}

func (s *Session) checkpointCompletedTick(boundary CheckpointBoundary) {
	c := s.checkpoints
	if c == nil || !c.enabled {
		return
	}
	position := CheckpointPosition{Tick: s.Clock.GlobalTick, Boundary: boundary, Pump: c.pump, ConsumedInput: c.consumed}
	finishError := func(err error) {
		c.pending = nil
		c.result = CheckpointCaptureResult{Err: err}
	}
	if c.scopeErr != nil {
		finishError(c.scopeErr)
		return
	}
	row, err := s.checkpointRingRow(position)
	if err != nil {
		finishError(err)
		return
	}
	cadence := position.Tick%30 == 0
	requested := c.pending != nil
	if cadence || requested {
		record, err := func() (record CheckpointRecord, err error) {
			c.inCapture = true
			completed := false
			defer func() {
				c.inCapture = false
				c.pending = nil
				if !completed {
					finishError(runtimeCheckpointError("capture.sink", "a synchronous writer that returns normally"))
				}
			}()
			record, err = s.captureCheckpoint(c.keys, position, c.pending)
			completed = true
			return
		}()
		if c.scopeErr != nil {
			err = c.scopeErr
		}
		if err != nil {
			finishError(err)
			return
		}
		// Keep the result paired with delivered bytes even when a later tick
		// in this pump also falls on cadence (DESIGN_MULTIPLAYER §16.3.6).
		if requested {
			c.result = CheckpointCaptureResult{Record: record}
		}
		if cadence {
			c.history.appendRecord(record)
		}
	}
	c.history.appendTick(row)
}
