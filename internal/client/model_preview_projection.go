package client

import (
	"fmt"
	"math"
	"slices"

	"github.com/nanolathe-gg/nanolathe/internal/drawlist"
	presentationrender "github.com/nanolathe-gg/nanolathe/internal/render"
	"github.com/nanolathe-gg/nanolathe/internal/sim/numeric"
)

// ModelPreviewProjection is the isolated viewer's presentation policy
// (DESIGN_DEVELOPER_TOOLS §7). PixelsPerUnit measures final raster pixels per
// model unit. Pivot is an already-oriented model-relative point; it affects
// screen placement only, never source heights, shading or depth keys.
type ModelPreviewProjection struct {
	PixelsPerUnit float64
	Pivot         [3]numeric.Fixed
}

// RecordProjectedGeometry records a complete model with transformed vertex
// fractions retained through projection at the final raster scale. Its Image
// is nil and Projected supplies unrounded screen positions and depth. The
// legacy List keeps independently rounded native and doubled corners.
//
// Scale must be zero: PixelsPerUnit supplies the entire projection scale.
// Construction, attached children, cloak, digger and waterline options are
// unsupported for the model itself. RecordModel and RecordGeometry retain
// those ordinary contracts and the retail projection arithmetic
// [03 R-RAST-01 §2]. opts.Attachment alone composes a second model, which may
// be a nanoframe, into the same record (DESIGN_GPU_RENDERER §22.5).
func (r *ModelPreviewRenderer) RecordProjectedGeometry(opts ModelPreviewOptions, projection ModelPreviewProjection) (ModelPreviewRecord, error) {
	if err := validateProjectedPreview(opts, projection); err != nil {
		return ModelPreviewRecord{}, err
	}
	return r.recordModel(opts, true, &projection)
}

func validateProjectedPreview(opts ModelPreviewOptions, projection ModelPreviewProjection) error {
	if !isFinitePositive(projection.PixelsPerUnit) {
		return fmt.Errorf("nanolathe: rendering projected preview: logical path %s, providers searched [preview projection], expected finite positive pixels per unit", opts.Model)
	}
	if opts.Scale != 0 {
		return fmt.Errorf("nanolathe: rendering projected preview: logical path %s, providers searched [preview options], expected default Scale with PixelsPerUnit", opts.Model)
	}
	if len(opts.Children) != 0 || opts.BuildRemaining != 0 || opts.Cloaked || opts.Digger || opts.WorldHeight != 0 || opts.UnderwaterExempt {
		return fmt.Errorf("nanolathe: rendering projected preview: logical path %s, providers searched [preview options], expected complete isolated model without construction, children, cloak, digger or waterline options", opts.Model)
	}
	if a := opts.Attachment; a != nil {
		if previewRenderName(a.Model) == "" {
			return fmt.Errorf("nanolathe: rendering projected preview: logical path objects3d, providers searched [preview attachment], expected attachment model name")
		}
		// Zero is complete; a nanoframe's remaining fraction is (0, 1]
		// [03 R-P0-19-N]. NaN fails both comparisons.
		if !(a.BuildRemaining >= 0 && a.BuildRemaining <= 1) {
			return fmt.Errorf("nanolathe: rendering projected preview: logical path %s, providers searched [preview attachment], expected remaining construction fraction in 0..1", a.Model)
		}
	}
	return nil
}

func isFinitePositive(v float64) bool { return v > 0 && !math.IsInf(v, 0) }

type modelPreviewProjector struct {
	projection ModelPreviewProjection
	positions  []drawlist.ModelPreviewPosition
	err        error
}

func (p *modelPreviewProjector) vertex(v, world [3]numeric.Fixed) (x, y, x2, y2 int32) {
	position, ok := p.project(v, world)
	if !ok {
		return 0, 0, 0, 0
	}
	// This callback runs only after the shared walk admits the whole face.
	// Keep its precise corners in that same order without adding storage to
	// the ordinary polygon scratch or its per-vertex packet.
	p.positions = append(p.positions, position)
	sx, sy := position.X, position.Y
	return int32(math.Floor(sx)), int32(math.Floor(sy)), int32(math.Floor(2 * sx)), int32(math.Floor(2 * sy))
}

// project is the viewer's one projection of a model-space point relative to
// the subject origin: anchor-free output pixels and model-relative depth.
func (p *modelPreviewProjector) project(v, world [3]numeric.Fixed) (drawlist.ModelPreviewPosition, bool) {
	// Preserve the transformed fixed-point fractions and combine the height
	// shear before rounding. The model Z mirror remains the shared projection
	// convention [03 §2.5]; late rounding is tool-only host policy, §7.
	sx := (float64(v[0]-world[0]) - float64(p.projection.Pivot[0])) / 65536 * p.projection.PixelsPerUnit
	sy := (-(float64(v[2]-world[2]) - float64(p.projection.Pivot[2])) - (float64(v[1]-world[1])-float64(p.projection.Pivot[1]))/2) / 65536 * p.projection.PixelsPerUnit
	// Leave room for doubled coordinates, either bound's origin and margins.
	// A malformed scale must fail before an out-of-range float-to-int cast.
	const limit = float64(math.MaxInt32 / 8)
	if math.IsNaN(sx) || math.IsNaN(sy) || math.Abs(sx) > limit || math.Abs(sy) > limit {
		if p.err == nil {
			p.err = fmt.Errorf("nanolathe: rendering projected preview: logical path <model vertices>, providers searched [preview projection], expected representable raster coordinates")
		}
		return drawlist.ModelPreviewPosition{}, false
	}
	return drawlist.ModelPreviewPosition{X: sx, Y: sy, Depth: float64(v[1]-world[1]) / 65536}, true
}

func (p *modelPreviewProjector) geometry(faces []drawlist.ModelFace, anchorX, anchorY int32) *drawlist.ModelPreviewGeometry {
	positions := slices.Clone(p.positions)
	for i := range positions {
		positions[i].X += float64(anchorX)
		positions[i].Y += float64(anchorY)
	}
	out := &drawlist.ModelPreviewGeometry{Faces: make([]drawlist.ModelPreviewFace, len(faces))}
	offset := 0
	for i, face := range faces {
		end := offset + len(face.Vertices)
		face.Vertices = slices.Clone(face.Vertices)
		out.Faces[i] = drawlist.ModelPreviewFace{Face: face, Positions: positions[offset:end:end]}
		offset = end
	}
	return out
}

// ProjectedPieces returns the projected position of each named piece origin
// for the orientation, poses, projection and attachment a
// RecordProjectedGeometry call with the same arguments draws: absolute
// output-pixel X/Y and model-relative depth in the record's Projected frame.
// It records no geometry. A hidden piece still has an origin.
func (r *ModelPreviewRenderer) ProjectedPieces(opts ModelPreviewOptions, projection ModelPreviewProjection, pieces []ModelPreviewPiece) ([]drawlist.ModelPreviewPosition, error) {
	if r == nil || r.client == nil {
		return nil, fmt.Errorf("nanolathe: projecting preview pieces: renderer is not initialized")
	}
	if err := validateProjectedPreview(opts, projection); err != nil {
		return nil, err
	}
	c := r.client
	renderName := previewRenderName(opts.Model)
	if opts.Width <= 0 || opts.Height <= 0 || opts.Width > maxModelPreviewDimension || opts.Height > maxModelPreviewDimension {
		return nil, fmt.Errorf("nanolathe: projecting preview pieces: output size %dx%d outside 1..%d", opts.Width, opts.Height, maxModelPreviewDimension)
	}
	var x, z numeric.Fixed
	c.cam, x, z = previewCamera(opts)
	view := previewUnitView(opts, renderName, x, z)
	previousOrientation := c.modelOrientation
	c.modelOrientation = nil
	defer func() { c.modelOrientation = previousOrientation }()
	parentModel := c.modelForUnit(view)
	parent, ok := c.unitDrawFor(view)
	if !ok || parentModel == nil {
		return nil, fmt.Errorf("nanolathe: projecting preview pieces: logical path %s, providers searched %s, expected drawable 3DO model", renderName, previewProviders(c.modelFS))
	}
	var attached *presentationrender.UnitDraw
	var attachedModel *unitModel
	projector := modelPreviewProjector{projection: projection}
	anchorX, anchorY := float64(opts.Width/2), float64(opts.Height/2)
	out := make([]drawlist.ModelPreviewPosition, len(pieces))
	for i, ref := range pieces {
		d, m := parent, parentModel
		if ref.Attachment {
			if opts.Attachment == nil {
				return nil, fmt.Errorf("nanolathe: projecting preview pieces: logical path %s, providers searched [preview options], expected an attachment for piece %q", renderName, ref.Name)
			}
			if attached == nil {
				var err error
				if attached, _, attachedModel, err = c.previewAttachmentDraws(parent, view, opts.Attachment, false); err != nil {
					return nil, err
				}
			}
			d, m = attached, attachedModel
		}
		index, ok := previewPieceIndex(c, m, ref.Name)
		if !ok || index >= len(d.Pieces) {
			return nil, fmt.Errorf("nanolathe: projecting preview pieces: logical path %s, providers searched [model pieces], expected piece %q", d.Model.Name, ref.Name)
		}
		position, ok := projector.project(d.Pieces[index].WorldOrigin, d.WorldPos)
		if !ok {
			return nil, projector.err
		}
		position.X += anchorX
		position.Y += anchorY
		out[i] = position
	}
	return out, nil
}

// previewOutline walks every ring of every visible piece, last piece first and
// without the selection plate, exactly as the battle outline does
// [03 R-COMP-01 §3]. It includes rings the body's material dispatch omits.
func previewOutline(draw *presentationrender.UnitDraw, projector modelPreviewProjector, color uint8, anchorX, anchorY int32) ([]drawlist.ModelPreviewOutlinePixel, error) {
	var out []drawlist.ModelPreviewOutlinePixel
	var xs, ys, ds []float64
	for pi := len(draw.Pieces) - 1; pi >= 0; pi-- {
		if pi >= len(draw.Model.Pieces) {
			continue
		}
		piece := &draw.Pieces[pi]
	rings:
		for pri := range piece.Primitives {
			pr := &piece.Primitives[pri]
			if draw.Model.Pieces[pi].Selection && pri == 0 || len(pr.VertexIndices) < 3 {
				continue
			}
			xs, ys, ds = xs[:0], ys[:0], ds[:0]
			for _, vi := range pr.VertexIndices {
				if int(vi) >= len(piece.WorldVertices) {
					continue rings // a ring naming a missing corner is dropped
				}
				p, ok := projector.project(piece.WorldVertices[vi], draw.WorldPos)
				if !ok {
					return nil, projector.err
				}
				xs, ys, ds = append(xs, p.X+float64(anchorX)), append(ys, p.Y+float64(anchorY)), append(ds, p.Depth)
			}
			out = appendPreviewOutlineRing(out, xs, ys, ds, color)
		}
	}
	return out, nil
}

// appendPreviewOutlineRing translates the battle's outline edge walk to the
// viewer's fractional raster [03 R-COMP-01 §3][03 R-RAST-01 §1]. The ring's
// two chains run from its first top corner to its first bottom corner,
// decreasing indices on the left and increasing on the right, and a folded
// chain's later edge wins a row. Retail samples whole rows at integer corner
// positions and covers columns ceil(xL) up to, not including, ceil(xR). The
// viewer raster samples pixel centres, so the same rule samples each output
// row at its centre and covers columns ceil(xL-1/2) up to ceil(xR-1/2). Where
// xR-xL is strictly positive, the two pixels at those column bounds are
// written; back-facing and empty rows write nothing.
//
// Each pixel carries the depth plane of the face's fan triangle that holds
// its edge, the triangle the device rasterizes there, clamped to the ring's
// depth span. The device then tests the outline as the battle tests it,
// against the attachment's own surface, by its own face's depth.
func appendPreviewOutlineRing(out []drawlist.ModelPreviewOutlinePixel, xs, ys, ds []float64, color uint8) []drawlist.ModelPreviewOutlinePixel {
	n := len(xs)
	top, bottom := 0, 0
	lo, hi := ds[0], ds[0]
	for i := 1; i < n; i++ {
		if ys[i] < ys[top] {
			top = i
		}
		if ys[i] > ys[bottom] {
			bottom = i
		}
		lo, hi = min(lo, ds[i]), max(hi, ds[i])
	}
	first, end := int(math.Ceil(ys[top]-0.5)), int(math.Ceil(ys[bottom]-0.5))
	if end <= first {
		return out
	}
	type sample struct {
		x, depth float64
		edge     int
	}
	left, right := make([]sample, end-first), make([]sample, end-first)
	walk := func(step int, into []sample) {
		for cur := top; cur != bottom; {
			next := (cur + step + n) % n
			if ys[next] > ys[cur] {
				// The polygon edge joining e and e+1.
				edge := cur
				if step < 0 {
					edge = next
				}
				a, b := max(first, int(math.Ceil(ys[cur]-0.5))), min(end, int(math.Ceil(ys[next]-0.5)))
				for r := a; r < b; r++ {
					f := (float64(r) + 0.5 - ys[cur]) / (ys[next] - ys[cur])
					into[r-first] = sample{
						x:     xs[cur] + float64((xs[next]-xs[cur])*f),
						depth: ds[cur] + float64((ds[next]-ds[cur])*f),
						edge:  edge,
					}
				}
			}
			cur = next
		}
	}
	walk(-1, left)
	walk(1, right)
	for r := first; r < end; r++ {
		l, rt := left[r-first], right[r-first]
		if !(rt.x-l.x > 0) {
			continue
		}
		for _, s := range [2]sample{l, rt} {
			px := drawlist.ModelPreviewOutlinePixel{X: int32(math.Ceil(s.x - 0.5)), Y: int32(r), Color: color}
			gx, gy, ok := previewFanPlane(xs, ys, ds, s.edge)
			for k, corner := range [4][2]float64{{0, 0}, {1, 0}, {1, 1}, {0, 1}} {
				d := s.depth
				if ok {
					cx, cy := float64(px.X)+corner[0], float64(px.Y)+corner[1]
					d = ds[0] + float64(gx*(cx-xs[0])) + float64(gy*(cy-ys[0]))
				}
				px.Depth[k] = min(max(d, lo), hi)
			}
			out = append(out, px)
		}
	}
	return out
}

// previewFanPlane returns the screen-space depth gradient of the fan triangle
// holding edge e of a ring (the edge from corner e to corner e+1). The device
// draws every face as the fan (0, i, i+1); all its triangles share corner 0.
func previewFanPlane(xs, ys, ds []float64, e int) (gx, gy float64, ok bool) {
	n := len(xs)
	b := min(max(e, 1), n-2)
	c := b + 1
	x1, y1, d1 := xs[b]-xs[0], ys[b]-ys[0], ds[b]-ds[0]
	x2, y2, d2 := xs[c]-xs[0], ys[c]-ys[0], ds[c]-ds[0]
	det := float64(x1*y2) - float64(x2*y1)
	if det == 0 || math.IsNaN(det) {
		return 0, 0, false
	}
	return (float64(d1*y2) - float64(d2*y1)) / det, (float64(x1*d2) - float64(x2*d1)) / det, true
}
