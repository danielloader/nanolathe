package aikit

import (
	"github.com/nanolathe-gg/nanolathe/internal/ai"
	"github.com/nanolathe-gg/nanolathe/internal/content"
	"github.com/nanolathe-gg/nanolathe/internal/world"
)

// One sealed source is shared by registration and capture. Reconstructing a
// source at each call would discard its identity (DESIGN_MULTIPLAYER §16.3.75).
var checkpointControllerSource = ai.NewCheckpointControllerSource[Host, *Host]()

func CheckpointControllerSource() ai.CheckpointControllerSource {
	return checkpointControllerSource
}

// Construction provenance exists even when application diagnostics are disabled:
// battle entry may prime a Host before attaching its history. Neither this proof
// nor the executor aliases is payload. Controller/persona/executor values retain
// their existing disposition; all worker storage remains excluded (§16.3.75).
type checkpointHostOwner struct {
	self    *Host
	manager *ai.Manager
}

type checkpointHostExecutor struct {
	executor *executor
	manager  *ai.Manager
	table    *Table
	catalog  *content.Catalog
	terrain  *world.Terrain
}

func (h *Host) validateCheckpointOwner(m *ai.Manager) error {
	if h == nil || m == nil || h.checkpointOwner.self != h || h.checkpointOwner.manager != m || h.m != m {
		return hostCheckpointError("aikit.Host.m", "the original constructed host and manager")
	}
	actual, ok := m.Ext.(*Host)
	if !ok || actual != h {
		return hostCheckpointError("aikit.Host.m.Ext", "this exact host as the manager's controller")
	}
	return nil
}

// ValidateCheckpointBindings checks only simulation-thread aliases, history and
// the immutable table snapshot. ValidateModernManager checks direct aliases, so
// calling it here cannot recurse into this Host (DESIGN_MULTIPLAYER §16.3.75).
func (h *Host) ValidateCheckpointBindings(m *ai.Manager, c *ai.CheckpointContext) error {
	_, err := h.checkpointBindingKind(m, c)
	return err
}

func (h *Host) checkpointBindingKind(m *ai.Manager, c *ai.CheckpointContext) (uint8, error) {
	if err := h.validateCheckpointOwner(m); err != nil {
		return 0, err
	}
	if _, err := h.checkpointControllerBoundary(); err != nil {
		return 0, err
	}
	if err := c.ValidateModernManager(m); err != nil {
		return 0, err
	}
	if h.ex.checkpointApplication != nil || h.ex.checkpointBatchSerial != 0 {
		return 0, executorCheckpointError("aikit.executor.application", "no command application in progress")
	}
	b := h.checkpointExecutor
	if !h.inited {
		if b != (checkpointHostExecutor{}) || h.ex.m != nil || h.ex.table != nil {
			return 0, hostCheckpointError("aikit.Host.ex", "absent executor bindings before initialization")
		}
		return 0, nil
	}
	if b.executor != &h.ex || b.manager != m || h.ex.m != m || b.table == nil || h.ex.table != b.table {
		return 0, hostCheckpointError("aikit.Host.ex", "the original executor, manager and table")
	}
	if m.Catalog != b.catalog || m.Terrain != b.terrain || c.World.Terrain != b.terrain {
		return 0, hostCheckpointError("aikit.Host.ex", "the original catalog and bound world terrain")
	}
	return b.table.ValidateCheckpointBindings(b.catalog, c.Units.Keys)
}

// Empty standalone leaves retain their old fixture bytes. A present binding
// must belong to the exact embedded executor, not an equal copy. No observer or
// mapInfo pointer is inspected, including for presence (§16.3.20, §16.3.75).
func (e *executor) checkpointBindingKind(c *ai.CheckpointContext) (uint8, error) {
	if e == nil {
		return 0, executorCheckpointError("aikit.executor", "a present executor")
	}
	if c == nil || c.Units == nil || c.World == nil {
		return 0, executorCheckpointError("aikit.context", "unit and world checkpoint contexts")
	}
	if e.checkpointApplication != nil || e.checkpointBatchSerial != 0 {
		return 0, executorCheckpointError("aikit.executor.application", "no command application in progress")
	}
	if e.m == nil {
		if e.table != nil {
			return 0, executorCheckpointError("aikit.executor.table", "an absent table or an exact owning host")
		}
		return 0, nil
	}
	h, ok := e.m.Ext.(*Host)
	if !ok || h == nil || &h.ex != e {
		return 0, executorCheckpointError("aikit.executor.m", "the exact owning host's embedded executor")
	}
	return h.checkpointBindingKind(e.m, c)
}
