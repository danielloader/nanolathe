package orders

import "github.com/nanolathe-gg/nanolathe/internal/combat"

var checkpointParalyzeInstallation *combat.CheckpointParalyzeTaskInstallation

// ValidateCheckpointParalyzeTaskBinding verifies the initializer's original
// build-wide installation, without reinstallation or invocation (§16.3.72).
func ValidateCheckpointParalyzeTaskBinding() error {
	return combat.ValidateCheckpointParalyzeTaskInstallation(checkpointParalyzeInstallation)
}
