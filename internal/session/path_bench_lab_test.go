//go:build pathbench && retail

package session

import "github.com/nanolathe-gg/nanolathe/internal/pathlab/labrules"

// The pathfinding laboratory's rule sets, for the path benchmark
// (docs/PATHFINDING_LAB.md): the list the laboratory's replay registers,
// registered here because this package cannot import the replay.
func init() {
	for _, set := range labrules.All() {
		RegisterRuleSet(set.Name, func() RuleSet {
			return RuleSet{Movement: set.Movement, Path: set.Kernel}
		})
	}
}
