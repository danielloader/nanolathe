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
	// Attachment is a second model composed into the same output-pixel and
	// depth frame, such as a factory's product on its pad, or nil for an
	// isolated model (DESIGN_GPU_RENDERER §22.5).
	Attachment *ModelPreviewAttachment
}

// ModelPreviewAttachment is the attached model's admitted faces in recorded
// order. Positions share the parent faces' frame, so the two models occlude
// each other by depth. On these faces Face.Vertices[i].Key is not the
// parent-relative legacy key: it is the attachment's own nanoframe height key,
// the whole height above the attachment origin plus the key bias, unwrapped,
// as a carried child's own packet keys it [03 R-P0-19-N].
//
// Reveal is the attachment's nanoframe reveal and Outline its outline
// endpoint pixels; both are absent when the attachment is complete.
type ModelPreviewAttachment struct {
	Faces   []ModelPreviewFace
	Reveal  *ModelReveal
	Outline []ModelPreviewOutlinePixel
}

// ModelPreviewOutlinePixel is one nanoframe outline endpoint: a whole output
// pixel at column X, row Y in the palette index Color [03 R-COMP-01 §3].
// Depth holds the outlined face's depth plane at the pixel's corners, in the
// order (X,Y), (X+1,Y), (X+1,Y+1), (X,Y+1), in the frame of
// ModelPreviewPosition.Depth.
type ModelPreviewOutlinePixel struct {
	X, Y  int32
	Color uint8
	Depth [4]float64
}
