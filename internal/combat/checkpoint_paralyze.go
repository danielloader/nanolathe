package combat

import "github.com/nanolathe-gg/nanolathe/internal/units"

// CheckpointParalyzeTaskInstallation identifies one static installation. Its
// private nonzero-size value prevents different live receipts sharing identity.
// Orders privately retains its initializer's receipt (DESIGN_MULTIPLAYER §16.3.72).
type CheckpointParalyzeTaskInstallation struct{ installed bool }

var checkpointParalyzeTaskInstallation *CheckpointParalyzeTaskInstallation

// SetParalyzeTaskPush replaces the static entry point. Even reinstalling an
// extracted function cannot recover an earlier receipt; nil clears it.
func SetParalyzeTaskPush(fn func(*units.Unit, uint32, uint32)) *CheckpointParalyzeTaskInstallation {
	paralyzeTaskPush = fn
	checkpointParalyzeTaskInstallation = nil
	if fn != nil {
		checkpointParalyzeTaskInstallation = &CheckpointParalyzeTaskInstallation{installed: true}
	}
	return checkpointParalyzeTaskInstallation
}

// ParalyzeTaskPushHook returns only the function, without installation proof.
func ParalyzeTaskPushHook() func(*units.Unit, uint32, uint32) { return paralyzeTaskPush }

// ValidateCheckpointParalyzeTaskInstallation checks identity without invoking
// the task or changing global wiring.
func ValidateCheckpointParalyzeTaskInstallation(expected *CheckpointParalyzeTaskInstallation) error {
	if expected == nil || !expected.installed || expected != checkpointParalyzeTaskInstallation || paralyzeTaskPush == nil {
		return combatCheckpointError("combat.ParalyzeTaskPush", "the original orders initializer's installation")
	}
	return nil
}
