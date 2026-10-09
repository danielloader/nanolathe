package ai

import (
	"github.com/nanolathe-gg/nanolathe/internal/sim/checkpoint"
	"github.com/nanolathe-gg/nanolathe/internal/units"
)

// beginClassicApplication observes one selected actor submission, never the
// search or upkeep that selected it (DESIGN_MULTIPLAYER §16.3.27). Admission
// failures stop reporting only; callers retain their existing gameplay path.
func (m *Manager) beginClassicApplication(tick uint32, actor, target *units.Unit, intent ClassicApplicationIntent) *ApplicationAttempt {
	h := m.CheckpointApplicationHistory()
	if h == nil {
		return nil
	}
	if h.kind != 1 {
		h.Fail(aiCheckpointError("ai.classic.history", "a Classic application history"))
		return nil
	}
	serial := h.NextSerial()
	if serial == 0 {
		return nil
	}
	ref, err := CheckpointAllocation(actor)
	if err != nil {
		h.Fail(err)
		return nil
	}
	intent.Operands.Actors = []checkpoint.Allocation{ref}
	if target != nil {
		ref, err := CheckpointAllocation(target)
		if err != nil {
			h.Fail(err)
			return nil
		}
		intent.Operands.Target = &ref
	}
	if intent.Kind == 2 {
		cat := m.Catalog
		if cat == nil {
			cat = m.Strategic.Catalog
		}
		if def, ok := cat.Unit(intent.UnitKey); ok && def != nil {
			ref, err := m.CheckpointApplicationKey(def)
			if err != nil {
				h.Fail(err)
				return nil
			}
			intent.Operands.Product = &ref
		}
	}
	return h.BeginAttempt(tick, serial, 0, intent.WriteCheckpoint)
}

func classicBuildIntent(req BuildRequest) ClassicApplicationIntent {
	return ClassicApplicationIntent{Kind: 2, BuildKind: int64(req.Kind), UnitKey: req.UnitKey,
		X: req.X, Z: req.Z, Count: int64(req.Count), RequestTick: req.Tick}
}

// Capture the allocation before invoking the producer: a later slot lookup
// would lose the observed object if cleanup retired or replaced it.
func classicApplicationActor(a *ApplicationAttempt, actor *units.Unit) checkpoint.Allocation {
	if a == nil {
		return checkpoint.Allocation{}
	}
	ref, err := CheckpointAllocation(actor)
	a.history.Fail(err)
	return ref
}

func finishClassicApplication(a *ApplicationAttempt, succeeded bool) {
	terminal := uint8(3)
	if succeeded {
		terminal = 2
	} else if a.CommittedOperations() != 0 {
		terminal = 4
	}
	a.Finish(1, terminal)
}

// The outer producer's completion supersedes nested preparation or cleanup
// insertions. A nil callback return alone does not prove requested work ran.
func classicBuildSucceeded(a *ApplicationAttempt, mark uint64, actor checkpoint.Allocation, err error) bool {
	result, completed := a.InsertionAfter(mark, actor, 2)
	return err == nil && completed && (result.Inserted || result.Coalesced)
}

func (m *Manager) beginClassicActivation(tick uint32, actor *units.Unit, active bool) *ApplicationAttempt {
	a := m.beginClassicApplication(tick, actor, nil, ClassicApplicationIntent{Kind: 3, Active: active})
	a.RecordActivation(classicApplicationActor(a, actor), active)
	return a
}
