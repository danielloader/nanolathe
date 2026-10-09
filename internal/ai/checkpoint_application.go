package ai

import (
	"github.com/nanolathe-gg/nanolathe/internal/content"
	"github.com/nanolathe-gg/nanolathe/internal/sim/checkpoint"
	"github.com/nanolathe-gg/nanolathe/internal/units"
)

// EnableCheckpointApplications installs the diagnostic chain before a manager
// creates its controller. Session owns entry-boundary admission; this method
// never resets a running history (DESIGN_MULTIPLAYER §16.3.24).
func (m *Manager) EnableCheckpointApplications(identity checkpoint.Identity, keys *content.CheckpointKeys) error {
	if m == nil || keys == nil {
		return aiCheckpointError("ai.applications", "a manager and admitted content keys")
	}
	if m.checkpointHistory != nil || m.Ext != nil {
		return aiCheckpointError("ai.applications", "an unstarted controller without an existing history")
	}
	var kind uint8
	switch m.Controller {
	case ControllerClassic:
		kind = 1
	case ControllerModern:
		kind = 2
	default:
		return aiCheckpointError("ai.applications.Controller", "a supported computer controller")
	}
	h, err := NewApplicationHistory(identity, m.Player, kind)
	if err != nil {
		return err
	}
	m.checkpointHistory, m.checkpointKeys = h, keys
	return nil
}

// EnableControllerCheckpointApplications attaches a fresh Modern history to
// the manager's exact controller pointer after completed entry (§16.3.43).
// Pointer ownership is not planner or binding admission; the concrete caller
// and session establish those separately. A type assertion avoids comparing
// arbitrary, potentially noncomparable Ext values or invoking their methods.
func EnableControllerCheckpointApplications[T any](m *Manager, expected *T, identity checkpoint.Identity, keys *content.CheckpointKeys) error {
	if m == nil || expected == nil || keys == nil {
		return aiCheckpointError("ai.applications", "a manager, controller pointer and admitted content keys")
	}
	if m.Controller != ControllerModern || m.checkpointHistory != nil {
		return aiCheckpointError("ai.applications", "a Modern controller without an existing history")
	}
	actual, ok := m.Ext.(*T)
	if !ok || actual != expected {
		return aiCheckpointError("ai.applications.Ext", "the exact owning controller pointer")
	}
	h, err := NewApplicationHistory(identity, m.Player, 2)
	if err != nil {
		return err
	}
	m.checkpointHistory, m.checkpointKeys = h, keys
	return nil
}

// CheckpointApplicationHistory is nil for ordinary, uninstrumented managers.
// The Modern host borrows it at construction; it is never worker-owned.
func (m *Manager) CheckpointApplicationHistory() *ApplicationHistory {
	if m == nil {
		return nil
	}
	return m.checkpointHistory
}

// CheckpointApplicationKey resolves the observed object, including retired
// equal-name definitions, without consulting or preparing a live catalog.
func (m *Manager) CheckpointApplicationKey(def *content.UnitDef) (checkpoint.Definition, error) {
	if m == nil || m.checkpointHistory == nil || m.checkpointKeys == nil {
		return checkpoint.Definition{}, aiCheckpointError("ai.applications.product", "enabled history with admitted keys")
	}
	return m.checkpointKeys.Unit(def)
}

// CheckpointAllocation names an already-observed allocation. A retired unit
// remains valid; looking up its current slot would erase the stale identity.
func CheckpointAllocation(u *units.Unit) (checkpoint.Allocation, error) {
	if u == nil || u.Handle == 0 || u.AllocationSerial == 0 {
		return checkpoint.Allocation{}, aiCheckpointError("ai.applications.actor", "a nonzero observed allocation identity")
	}
	return checkpoint.Allocation{Handle: uint32(u.Handle), Serial: u.AllocationSerial}, nil
}
