package session

import "github.com/nanolathe-gg/nanolathe/internal/ai"

// RegisterModernAIWithCheckpointBinding retains the reviewed planner and closed
// controller source in the existing registration (DESIGN_MULTIPLAYER §16.3.75).
func RegisterModernAIWithCheckpointBinding[T ai.ModernAIStep](step T, source ai.CheckpointControllerSource) {
	if !source.Valid() {
		panic("nanolathe: Modern AI checkpoint registration failed: logical path <mods>, providers searched [session Modern AI step], expected a generated controller source")
	}
	registerModernAI(step, ai.NewCheckpointModernPlanner(step), source)
}

// checkpointModernPlannerMatches uses only the frozen generated witness.
func (s *Session) checkpointModernPlannerMatches(planner ai.Planner) bool {
	return s != nil && s.modernAICheckpointWitness.Matches(s.modernAI) && s.modernAICheckpointWitness.Matches(planner)
}
