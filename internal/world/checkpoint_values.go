package world

import "github.com/nanolathe-gg/nanolathe/internal/sim/checkpoint"

// WriteCheckpoint retains the rectangle's stored values in lexical field order:
// anchor (cellX, cellZ), extent (depth, initialized, width), maxX, maxZ. In
// particular an authored empty extent differs from an uninitialized one; using
// public dimensions alone would lose that future validation input
// (DESIGN_MULTIPLAYER §16.3.15). Capture does not reconstruct the endpoints.
func (r FootprintRect) WriteCheckpoint(e *checkpoint.Encoder) error {
	e.Field("world.FootprintRect.anchor.cellX")
	e.I32(r.anchor.cellX)
	e.Field("world.FootprintRect.anchor.cellZ")
	e.I32(r.anchor.cellZ)
	e.Field("world.FootprintRect.extent.depth")
	e.I32(r.extent.depth)
	e.Field("world.FootprintRect.extent.initialized")
	e.Bool(r.extent.initialized)
	e.Field("world.FootprintRect.extent.width")
	e.I32(r.extent.width)
	e.Field("world.FootprintRect.maxX")
	e.I32(r.maxX)
	e.Field("world.FootprintRect.maxZ")
	e.I32(r.maxZ)
	return e.Err()
}
