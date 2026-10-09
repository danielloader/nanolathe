package combat

// CheckpointRulesKind identifies the closed stateless rules without invoking
// gameplay. Nil retains tag zero; typed nil and custom implementations refuse.
// Session must separately attest the selected set and its owner projections
// (DESIGN_MULTIPLAYER §16.3.42).
func CheckpointRulesKind(r Rules) (uint8, error) {
	switch v := r.(type) {
	case nil:
		return 0, nil
	case StrictRules:
		return 1, nil
	case *StrictRules:
		if v != nil {
			return 1, nil
		}
	case CommunityRules:
		return 2, nil
	case *CommunityRules:
		if v != nil {
			return 2, nil
		}
	case *ModernRules:
		if v != nil {
			return 3, nil
		}
	}
	return 0, combatCheckpointError("combat.Service.Rules", "nil or a nonnil reviewed stateless rule implementation")
}
