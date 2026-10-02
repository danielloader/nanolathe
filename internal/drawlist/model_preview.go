package drawlist

// ModelPreviewPosition is one isolated viewer corner before raster rounding
// (DESIGN_DEVELOPER_TOOLS §7). X and Y are absolute output-pixel coordinates.
// Depth is oriented model-relative height in world units, retaining fractions;
// it has neither the retail height-key bias nor the preview's fit/pivot offset.
type ModelPreviewPosition struct {
	X, Y, Depth float64
}

// ModelPreviewFace pairs shared model materials and per-corner UV/shade lanes
// with precise viewer positions in the same order as Face.Vertices. The
// integer positions and keys in Face remain the legacy packet's values;
// a precise viewer renderer uses Positions for geometry and surface ordering.
type ModelPreviewFace struct {
	Face      ModelFace
	Positions []ModelPreviewPosition
}

// ModelPreviewGeometry owns one isolated viewer's admitted faces in recorded
// order. Face vertices and positions are durable copies; texture frames remain
// immutable shared assets, as on ModelFace. This payload is separate from the
// ordinary model packet and never participates in battle recording or replay.
type ModelPreviewGeometry struct {
	Faces []ModelPreviewFace
}
