package aikit

import (
	"fmt"

	"github.com/nanolathe-gg/nanolathe/internal/ai"
	"github.com/nanolathe-gg/nanolathe/internal/content"
	"github.com/nanolathe-gg/nanolathe/internal/sim/checkpoint"
)

var _ ai.CheckpointControllerOwner = (*Host)(nil)

// EnableCheckpointApplications attaches at a completed entry boundary without
// reading or joining a worker (DESIGN_MULTIPLAYER §16.3.43). A batch scheduled
// before attachment borrows serial 1; completed prime attempts are not invented.
// The parent still owns actual planner/binding and entry-boundary admission.
func (h *Host) EnableCheckpointApplications(identity checkpoint.Identity, keys *content.CheckpointKeys) error {
	if h == nil || h.m == nil {
		return hostCheckpointError("aikit.Host", "a host and owning manager")
	}
	if err := h.validateCheckpointOwner(h.m); err != nil {
		return err
	}
	if h.checkpointHistory != nil || h.m.CheckpointApplicationHistory() != nil {
		return hostCheckpointError("aikit.Host.checkpointHistory", "no existing host or manager history")
	}
	if h.checkpointApplying || h.ex.checkpointApplication != nil || h.ex.checkpointBatchSerial != 0 || h.checkpointBatchSerial != 0 {
		return hostCheckpointError("aikit.Host.application", "no active application, command scope or diagnostic batch serial")
	}
	if (h.inited && h.ex.m != h.m) || (!h.inited && h.ex.m != nil) {
		return hostCheckpointError("aikit.Host.ex.m", "the owning manager after initialization and absence before it")
	}
	if h.checkpointDeadlinePresent && !h.inited {
		return hostCheckpointError("aikit.Host.checkpointDeadlinePresent", "an initialized host for a pending deadline")
	}
	if err := ai.EnableControllerCheckpointApplications(h.m, h, identity, keys); err != nil {
		return err
	}
	h.checkpointHistory = h.m.CheckpointApplicationHistory()
	if h.checkpointDeadlinePresent {
		// The freshly installed history cannot be active, failed or exhausted.
		h.checkpointBatchSerial = h.checkpointHistory.NextSerial()
	}
	return nil
}

// ControllerCheckpoint returns a detached simulation-thread value, not an
// admission verdict. Unsuccessful snapshots supply no chain fields; full writers
// and summaries validate separately (DESIGN_MULTIPLAYER §16.3.24). No batch or
// worker field, including its pointer or readiness, is inspected.
func (h *Host) ControllerCheckpoint() ai.ControllerCheckpoint {
	if h == nil {
		return ai.ControllerCheckpoint{}
	}
	state, err := h.checkpointHistory.Snapshot()
	if err != nil {
		state = ai.ApplicationHistoryState{}
	}
	return h.checkpointControllerValue(state)
}

func (h *Host) checkpointControllerValue(state ai.ApplicationHistoryState) ai.ControllerCheckpoint {
	return ai.ControllerCheckpoint{
		Present: true, Initialized: h.inited,
		NextThinkPresent: h.inited, NextThinkTick: h.nextThink,
		DeadlinePresent: h.checkpointDeadlinePresent, DeadlineTick: h.checkpointDeadlineTick,
		NextBatchSerial: state.NextSerial, ApplicationCount: state.Count, ApplicationHash: state.Hash,
		Tokens: h.ex.tokens, LastFill: h.ex.lastFill,
	}
}

// checkpointControllerBoundary observes only the borrowed simulation-thread
// history and its manager identity. Diagnostic failure never gates Step.
func (h *Host) checkpointControllerBoundary() (ai.ApplicationHistoryState, error) {
	if h == nil {
		return ai.ApplicationHistoryState{}, hostCheckpointError("aikit.Host", "a present host")
	}
	if h.checkpointApplying {
		return ai.ApplicationHistoryState{}, hostCheckpointError("aikit.Host.checkpointApplying", "a completed application boundary")
	}
	if h.checkpointHistory == nil {
		return ai.ApplicationHistoryState{}, hostCheckpointError("aikit.Host.checkpointHistory", "an enabled application history borrowed at construction")
	}
	if h.m == nil || h.checkpointHistory != h.m.CheckpointApplicationHistory() {
		return ai.ApplicationHistoryState{}, hostCheckpointError("aikit.Host.checkpointHistory", "the owning manager's application history")
	}
	state, err := h.checkpointHistory.Snapshot()
	if err != nil {
		return ai.ApplicationHistoryState{}, fmt.Errorf("%w: %w", hostCheckpointError("aikit.Host.checkpointHistory", "a completed, successful history snapshot"), err)
	}
	if !state.Enabled || state.Player != h.m.Player || state.Kind != 2 || h.m.Controller != ai.ControllerModern {
		return ai.ApplicationHistoryState{}, hostCheckpointError("aikit.Host.checkpointHistory", "the owning player's Modern application history")
	}
	return state, nil
}

// WriteControllerCheckpoint emits the detached controller record, stored
// Persona, then private executor, with no extra framing (§16.3.24). All Host
// and executor bindings preflight before any bytes (§16.3.75).
func (h *Host) WriteControllerCheckpoint(enc *checkpoint.Encoder, c *ai.CheckpointContext) error {
	if enc == nil {
		return hostCheckpointError("aikit.Host.encoder", "a checkpoint encoder")
	}
	enc.Field("aikit.Host")
	state, err := h.checkpointControllerBoundary()
	if err != nil {
		enc.Fail(err)
		return enc.Err()
	}
	kind, err := h.checkpointBindingKind(h.m, c)
	if err != nil {
		enc.Fail(err)
		return enc.Err()
	}
	if err := h.checkpointControllerValue(state).WriteCheckpoint(enc); err != nil {
		return err
	}
	if err := h.persona.writeCheckpoint(enc); err != nil {
		return err
	}
	return h.ex.writeCheckpointValues(enc, kind)
}

// AppendControllerCheckpointSummary validates first, then appends only the
// detached value's eleven selected words. It neither encodes executor caches
// nor validates their staged production bindings (§16.3.24).
func (h *Host) AppendControllerCheckpointSummary(summary *checkpoint.Summary) error {
	state, err := h.checkpointControllerBoundary()
	if err != nil {
		return err
	}
	return h.checkpointControllerValue(state).AppendCheckpointSummary(summary)
}

func hostCheckpointError(path, expected string) error {
	return fmt.Errorf("nanolathe: controller checkpoint failed: logical path %s, providers searched [], expected %s", path, expected)
}
