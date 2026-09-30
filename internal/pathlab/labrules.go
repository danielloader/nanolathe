package pathlab

import (
	"github.com/nanolathe-gg/nanolathe/internal/pathlab/labrules"
	"github.com/nanolathe-gg/nanolathe/internal/session"
)

// The laboratory's rule sets: Modern with the movement policy and the search
// kernel of each set in labrules, registered so that a replay can name one.
// None is a reserved set and the game links none.

// BaselineName names the baseline: what "modern" named in every measurement
// made before Modern adopted its traffic policy (docs/PATHFINDING_LAB.md).
const BaselineName = labrules.Baseline

func init() {
	for _, set := range labrules.All() {
		session.RegisterRuleSet(set.Name, func() session.RuleSet {
			return session.RuleSet{Movement: set.Movement, Path: set.Kernel}
		})
	}
}

// RuleSetNames lists the laboratory's rule sets. A harness that plays them
// under its own rule set asks for them here (cmd/ai-arena).
func RuleSetNames() []string {
	var names []string
	for _, set := range labrules.All() {
		names = append(names, set.Name)
	}
	return names
}
