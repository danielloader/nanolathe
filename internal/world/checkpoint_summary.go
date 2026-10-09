package world

import "github.com/nanolathe-gg/nanolathe/internal/sim/checkpoint"

// AppendCheckpointSummary retains selected plot words in row-major order.
// The full writer's fog/placer exclusions apply; geometry and owner bindings
// are not consulted (DESIGN_MULTIPLAYER §16.3.77).
func (t *Terrain) AppendCheckpointSummary(out *checkpoint.Summary) error {
	if t == nil || out == nil {
		return worldCheckpointError("world.Terrain.summary", "a present terrain and summary")
	}
	next := *out
	next.Word(uint64(len(t.Plot)))
	for _, cell := range t.Plot {
		next.Word(uint64(int64(cell.OccupantA())))
		next.Word(uint64(int64(cell.OccupantB())))
		next.Word(uint64(cell.Metal()))
		next.Word(uint64(cell.Feature()))
		next.Word(uint64(cell.AnchorWord()))
		next.Word(uint64(cell.FlagByte() &^ (0x04 | 0x78)))
	}
	*out = next
	return nil
}
