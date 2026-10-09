package features

import "github.com/nanolathe-gg/nanolathe/internal/sim/checkpoint"

// AppendCheckpointSummary reads only stored counts and the reproduction cursor.
// It does not walk instances, resolve content or refresh lookup/sequence caches
// (DESIGN_MULTIPLAYER §16.3.77).
func (s *Service) AppendCheckpointSummary(out *checkpoint.Summary) error {
	if s == nil || out == nil {
		return featureCheckpointError("features.Service.summary", "a present service and summary")
	}
	next := *out
	next.Word(uint64(len(s.instances)))
	next.Word(uint64(int64(s.cursor)))
	next.Word(uint64(int64(s.arenaHeld)))
	*out = next
	return nil
}
