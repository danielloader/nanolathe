package visibility

import "github.com/nanolathe-gg/nanolathe/internal/sim/checkpoint"

// AppendCheckpointSummary reads only the selected visibility words, without
// rebuilding a raster or consulting rules/readers. Footprints, dimensions and
// sensor caches are deliberate blind spots (DESIGN_MULTIPLAYER §16.3.41).
func (s *Service) AppendCheckpointSummary(out *checkpoint.Summary) error {
	if s == nil || out == nil {
		return visibilityCheckpointError("visibility.summary", "a present service and summary")
	}
	out.Word(uint64(s.mode &^ ModeFogCacheValid))
	out.Word(uint64(s.local))
	for _, bits := range s.team {
		out.Word(uint64(bits))
	}
	if s.viewerDefeated {
		out.Word(1)
	} else {
		out.Word(0)
	}
	out.Word(uint64(len(s.wordMask)))
	for _, bits := range s.wordMask {
		out.Word(uint64(bits))
	}
	for _, grid := range s.byteGrids {
		out.Word(uint64(len(grid)))
		for _, bits := range grid {
			out.Word(uint64(bits))
		}
	}
	return nil
}
