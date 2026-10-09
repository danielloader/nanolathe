package ai

// CheckpointPlannerKind identifies only the reviewed stateless planner leaves.
// It never invokes Step. Custom controllers, including registered mod planners,
// need their own composition witness (DESIGN_MULTIPLAYER §16.3.42).
func CheckpointPlannerKind(p Planner) (uint8, error) {
	switch v := p.(type) {
	case nil:
		return 0, nil
	case RetailPlanner:
		return 1, nil
	case *RetailPlanner:
		if v != nil {
			return 1, nil
		}
	case ModernPlanner:
		return 2, nil
	case *ModernPlanner:
		if v != nil {
			return 2, nil
		}
	}
	return 0, aiCheckpointError("ai.Manager.Planner", "nil or a nonnil reviewed stateless planner")
}

// checkpointPlannerKind adds only the registered Modern witness to the closed
// stateless leaf tags. Full validation separately checks its owner tuple.
func (m *Manager) checkpointPlannerKind(c *CheckpointContext) (uint8, error) {
	if c.modern != nil {
		if err := c.ValidateModernManager(m); err != nil {
			return 0, err
		}
		return 3, nil
	}
	return CheckpointPlannerKind(m.Planner)
}
