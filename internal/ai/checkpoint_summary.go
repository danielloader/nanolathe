package ai

import "github.com/nanolathe-gg/nanolathe/internal/sim/checkpoint"

// AppendCheckpointSummary is only the existing manager fragment (§16.3.19).
// Presence and the application/controller fragment belong to composition.
// The group lengths follow task slots 1..9, deliberately not lexical order.
// No binding, profile, map, allocation or worker access is needed.
func (m *Manager) AppendCheckpointSummary(s *checkpoint.Summary) error {
	if m == nil {
		return aiCheckpointError("ai.Manager", "a present manager")
	}
	if s == nil {
		return aiCheckpointError("ai.summary", "a summary accumulator")
	}
	next := *s
	next.Word(uint64(m.Controller))
	for _, v := range m.Deadlines {
		next.Word(uint64(v))
	}
	next.Word(uint64(m.countdown))
	for _, n := range [...]int{len(m.GroupResource), len(m.GroupWaveA), len(m.GroupRegroupA), len(m.GroupConstruction), len(m.GroupNull), len(m.GroupWaveB), len(m.GroupRegroupB), len(m.GroupExplore), len(m.GroupRally)} {
		next.Word(uint64(n))
	}
	*s = next
	return nil
}
