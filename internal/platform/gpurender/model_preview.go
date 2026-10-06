package gpurender

import (
	"fmt"
	"math"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/nanolathe-gg/nanolathe/internal/drawlist"
)

// The isolated viewer has no retail composition-key contract. Its geometric
// depth and subpixel coverage are deliberately separate from the battle lane
// (DESIGN_DEVELOPER_TOOLS §7, DESIGN_GPU_RENDERER §22.5).
const (
	modelPreviewDepthMax = 1<<24 - 1
	// Independently rounded 16.16 piece transforms can disagree about an
	// almost coincident plane. Within this world-space band, the first
	// recorded face wins consistently; genuinely separated surfaces keep
	// geometric depth order. This is viewer policy, not retail arithmetic.
	modelPreviewDepthTie = 1.0 / 1024
	// An attachment face without quad lanes carries its nanoframe key in the
	// quad lane's colour channel as -(key + modelPreviewKeyOffset), which can
	// never name a quad entry. Keys are whole numbers well inside the offset.
	modelPreviewKeyOffset = 32768
)

type modelPreviewRun struct {
	texture        *ebiten.Image
	vStart, iStart int
	vCount, iCount int
}

// modelPreviewBatch is one subject's packed vertices: the parent's faces, the
// attachment's faces, or the attachment's outline pixels.
type modelPreviewBatch struct {
	verts   []ebiten.Vertex
	indices []uint32
	runs    []modelPreviewRun
}

type modelPreviewSurface struct {
	shader, resolve *ebiten.Shader
	// compose and merge serve records with an attachment; they are compiled
	// for the first such record.
	compose, merge *ebiten.Shader
	depth          [2]*ebiten.Image
	colour         *ebiten.Image
	// The attachment's own depth and colour planes, allocated at the planes'
	// size for the first record with an attachment.
	attachedDepth, attachedColour *ebiten.Image
	w, h                          int
	params                        modelQuadParams
	body, attached, outline       modelPreviewBatch
	opts                          ebiten.DrawTrianglesShaderOptions
	composeOpts                   ebiten.DrawTrianglesShaderOptions
}

// DrawModelPreview draws one complete isolated tool model over dst. It uses
// geometric, non-wrapping depth even for assets without a retail key plane.
// It is an explicit tool entry, never selected by a battle draw command.
// dst is the finished RGB composite (call Expand first after Execute). A
// record with an attachment composes both models (drawModelPreviewComposed).
func (r *Renderer) DrawModelPreview(dst *ebiten.Image, g *drawlist.ModelPreviewGeometry) error {
	if dst == nil || r.tables.atlas == nil || g == nil {
		return fmt.Errorf("nanolathe: rendering model preview: logical path <projected geometry>, providers searched [GPU preview], expected destination, palette and geometry")
	}
	w, h := dst.Bounds().Dx(), dst.Bounds().Dy()
	if w <= 0 || h <= 0 || w > 2048 || h > 2048 {
		return fmt.Errorf("nanolathe: rendering model preview: logical path <preview surface>, providers searched [GPU preview], expected dimensions in 1..2048")
	}
	lo, hi, err := modelPreviewDepthBounds(g)
	if err != nil {
		return err
	}
	if r.preview == nil {
		p := &modelPreviewSurface{}
		p.shader, err = compileShader(modelPreviewShaderSource())
		if err != nil {
			return err
		}
		p.resolve, err = compileShader(modelPreviewResolveSource)
		if err != nil {
			p.shader.Deallocate()
			return err
		}
		r.preview = p
	}
	p := r.preview
	p.ensureSize(2*w, 2*h)
	// Normalization happens before the float32 device boundary. Heights can
	// be negative or span more than 256 units without wrapping.
	scale := float64(modelPreviewDepthMax) / max(hi-lo, 1)
	if g.Attachment != nil {
		return r.drawModelPreviewComposed(dst, g, lo, scale)
	}
	p.params.reset()
	p.body.pack(r, &p.params, g.Faces, lo, scale, false)
	if len(p.body.indices) == 0 {
		return nil
	}
	p.params.upload()
	params := p.params.img
	if params == nil {
		params = r.placeholderImage()
	}
	if p.opts.Uniforms == nil {
		p.opts.Uniforms = make(map[string]any)
	}
	// Two code points cover normalization/interpolation rounding as well as
	// the explicit world-space tie band. The same shader and vertex batch
	// evaluate depth in every pass, avoiding key/colour compiler disagreement.
	p.opts.Uniforms["DepthTie"] = float32(modelPreviewDepthTie*scale + 2)
	p.opts.Blend = modelPreviewMaxBlend
	previous := r.placeholderImage()
	for pass := range 3 {
		target := p.depth[pass&1]
		target.Clear()
		p.opts.Uniforms["Pass"] = float32(pass)
		p.body.draw(p.shader, &p.opts, target, previous, r.tables.atlas, params)
		previous = target
	}
	p.colour.Clear()
	p.opts.Uniforms["Pass"] = float32(3)
	p.opts.Blend = ebiten.BlendCopy
	p.body.draw(p.shader, &p.opts, p.colour, previous, r.tables.atlas, params)
	// Box-resolve all four covered samples, including transparent samples at
	// shared edges, onto the existing background with premultiplied alpha.
	var resolve ebiten.DrawRectShaderOptions
	resolve.Images[0] = p.colour
	resolve.GeoM.Scale(0.5, 0.5)
	dst.DrawRectShader(2*w, 2*h, p.resolve, &resolve)
	return nil
}

// modelPreviewMaxBlend keeps the larger byte in every channel: the depth
// passes' lexicographic selection.
var modelPreviewMaxBlend = ebiten.Blend{
	BlendFactorSourceRGB: ebiten.BlendFactorOne, BlendFactorSourceAlpha: ebiten.BlendFactorOne,
	BlendFactorDestinationRGB: ebiten.BlendFactorOne, BlendFactorDestinationAlpha: ebiten.BlendFactorOne,
	BlendOperationRGB: ebiten.BlendOperationMax, BlendOperationAlpha: ebiten.BlendOperationMax,
}

// drawModelPreviewComposed draws a parent and its attachment in one depth
// frame. The attachment first finishes its own image, as a battle carrier's
// child does [03 R-COMP-01 §3]: its own three depth passes select its top
// surface, its colour pass applies the nanoframe verdicts there, so an erased
// texel shows what lies behind the whole attachment rather than its far side,
// and its outline pixels are tested against that same surface. The parent is
// then drawn as an isolated record is, and one pass merges the two per sample
// by depth, the attachment winning ties as a carried child wins its carrier's
// key ties [03 R-REN-03A §4], and resolves the 2x coverage.
func (r *Renderer) drawModelPreviewComposed(dst *ebiten.Image, g *drawlist.ModelPreviewGeometry, lo, scale float64) error {
	p := r.preview
	if p.compose == nil {
		compose, err := compileShader(modelPreviewComposeShaderSource())
		if err != nil {
			return err
		}
		merge, err := compileShader(modelPreviewMergeSource)
		if err != nil {
			compose.Deallocate()
			return err
		}
		p.compose, p.merge = compose, merge
	}
	a := g.Attachment
	p.params.reset()
	p.body.pack(r, &p.params, g.Faces, lo, scale, false)
	p.attached.pack(r, &p.params, a.Faces, lo, scale, a.Reveal != nil)
	p.outline.packOutline(r, a.Outline, lo, scale)
	if len(p.body.indices)+len(p.attached.indices)+len(p.outline.indices) == 0 {
		return nil
	}
	if p.attachedDepth == nil {
		p.attachedDepth, p.attachedColour = newRendererImage(p.w, p.h), newRendererImage(p.w, p.h)
	}
	p.params.upload()
	params := p.params.img
	if params == nil {
		params = r.placeholderImage()
	}
	o := &p.composeOpts
	if o.Uniforms == nil {
		o.Uniforms = make(map[string]any)
	}
	tie := float32(modelPreviewDepthTie*scale + 2)
	o.Uniforms["DepthTie"] = tie
	o.Uniforms["Reveal"] = float32(0)
	if v := a.Reveal; v != nil {
		o.Uniforms["RevealLine"] = float32(v.Line)
		o.Uniforms["RevealFloor"] = float32(v.Floor)
		o.Uniforms["RevealBelow"] = float32(v.Below)
		o.Uniforms["RevealBand"] = float32(v.Band)
		o.Uniforms["RevealAbove"] = float32(v.Above)
	}
	atlas := r.tables.atlas
	// The attachment's own surface. Outline pixels join its depth passes: an
	// endpoint stores its depth beside its colour, so the merge tests the
	// outline as later key tests test it [03 R-COMP-01 §3].
	o.Blend = modelPreviewMaxBlend
	previous := r.placeholderImage()
	for pass, target := range [3]*ebiten.Image{p.attachedDepth, p.depth[1], p.attachedDepth} {
		target.Clear()
		o.Uniforms["Pass"] = float32(pass)
		p.attached.draw(p.compose, o, target, previous, atlas, params)
		p.outline.draw(p.compose, o, target, previous, atlas, params)
		previous = target
	}
	p.attachedColour.Clear()
	o.Uniforms["Pass"] = float32(3)
	o.Blend = ebiten.BlendCopy
	if a.Reveal != nil {
		o.Uniforms["Reveal"] = float32(1)
	}
	p.attached.draw(p.compose, o, p.attachedColour, p.attachedDepth, atlas, params)
	// The outline follows the reveal, which therefore never rewrites it.
	o.Uniforms["Reveal"] = float32(0)
	p.outline.draw(p.compose, o, p.attachedColour, p.attachedDepth, atlas, params)
	// The parent's own surface and colour.
	o.Blend = modelPreviewMaxBlend
	previous = r.placeholderImage()
	for pass := range 3 {
		target := p.depth[pass&1]
		target.Clear()
		o.Uniforms["Pass"] = float32(pass)
		p.body.draw(p.compose, o, target, previous, atlas, params)
		previous = target
	}
	p.colour.Clear()
	o.Uniforms["Pass"] = float32(3)
	o.Blend = ebiten.BlendCopy
	p.body.draw(p.compose, o, p.colour, previous, atlas, params)
	var merge ebiten.DrawRectShaderOptions
	merge.Images = [4]*ebiten.Image{p.colour, p.depth[0], p.attachedColour, p.attachedDepth}
	merge.Uniforms = map[string]any{"DepthTie": tie}
	merge.GeoM.Scale(0.5, 0.5)
	dst.DrawRectShader(p.w, p.h, p.merge, &merge)
	return nil
}

func modelPreviewDepthBounds(g *drawlist.ModelPreviewGeometry) (lo, hi float64, err error) {
	lo, hi = math.Inf(1), math.Inf(-1)
	faces := func(faces []drawlist.ModelPreviewFace) error {
		for _, f := range faces {
			if len(f.Positions) != len(f.Face.Vertices) {
				return fmt.Errorf("nanolathe: rendering model preview: logical path <projected face>, providers searched [GPU preview], expected matching corners and attributes")
			}
			for _, v := range f.Positions {
				if math.IsNaN(v.X) || math.IsNaN(v.Y) || math.IsNaN(v.Depth) || math.IsInf(v.X, 0) || math.IsInf(v.Y, 0) || math.IsInf(v.Depth, 0) || math.Abs(v.X) > 1<<20 || math.Abs(v.Y) > 1<<20 {
					return fmt.Errorf("nanolathe: rendering model preview: logical path <projected vertex>, providers searched [GPU preview], expected finite representable coordinates")
				}
				lo, hi = min(lo, v.Depth), max(hi, v.Depth)
			}
		}
		return nil
	}
	if err := faces(g.Faces); err != nil {
		return 0, 0, err
	}
	if a := g.Attachment; a != nil {
		if err := faces(a.Faces); err != nil {
			return 0, 0, err
		}
		for _, px := range a.Outline {
			if px.X < -1<<20 || px.X > 1<<20 || px.Y < -1<<20 || px.Y > 1<<20 {
				return 0, 0, fmt.Errorf("nanolathe: rendering model preview: logical path <outline pixel>, providers searched [GPU preview], expected representable coordinates")
			}
			for _, d := range px.Depth {
				if math.IsNaN(d) || math.IsInf(d, 0) {
					return 0, 0, fmt.Errorf("nanolathe: rendering model preview: logical path <outline pixel>, providers searched [GPU preview], expected finite depth")
				}
				lo, hi = min(lo, d), max(hi, d)
			}
		}
	}
	if math.IsInf(lo, 0) {
		return 0, 0, nil
	}
	if math.IsInf(hi-lo, 0) {
		return 0, 0, fmt.Errorf("nanolathe: rendering model preview: logical path <projected depth>, providers searched [GPU preview], expected finite depth span")
	}
	return lo, hi, nil
}

func (p *modelPreviewSurface) ensureSize(w, h int) {
	if p.w == w && p.h == h {
		return
	}
	for _, img := range []*ebiten.Image{p.depth[0], p.depth[1], p.colour, p.attachedDepth, p.attachedColour} {
		if img != nil {
			img.Deallocate()
		}
	}
	p.depth = [2]*ebiten.Image{newRendererImage(w, h), newRendererImage(w, h)}
	p.colour = newRendererImage(w, h)
	p.attachedDepth, p.attachedColour = nil, nil
	p.w, p.h = w, h
}

func (b *modelPreviewBatch) reset() {
	b.verts, b.indices = b.verts[:0], b.indices[:0]
	clear(b.runs)
	b.runs = b.runs[:0]
}

// pack replaces b's contents with faces. keyed carries each corner's
// nanoframe key: through the quad lanes where a quad has them, otherwise in
// the quad lane's colour channel (modelPreviewKeyOffset).
func (b *modelPreviewBatch) pack(r *Renderer, params *modelQuadParams, faces []drawlist.ModelPreviewFace, lo, scale float64, keyed bool) {
	b.reset()
	var corners [4]drawlist.ModelVertex
	var finishContext modelPlaceCtx
	// Reverse the immutable recorded order so the earliest admitted face
	// owns a near-coincident tie, independent of orbit or texture-page runs.
	for fi := len(faces) - 1; fi >= 0; fi-- {
		f := &faces[fi]
		n := len(f.Positions)
		if n < 3 {
			continue
		}
		var area float64
		a := f.Positions[n-1]
		for _, next := range f.Positions {
			area += a.X*next.Y - next.X*a.Y
			a = next
		}
		if area <= 0 {
			continue
		}
		slot := r.modelTextureFor(f.Face.Texture)
		texture := slot.img
		if texture == nil {
			texture = r.placeholderImage()
		}
		if len(b.runs) == 0 || b.runs[len(b.runs)-1].texture != texture {
			b.runs = append(b.runs, modelPreviewRun{texture: texture, vStart: len(b.verts), iStart: len(b.indices)})
		}
		run := &b.runs[len(b.runs)-1]
		quad := 0
		if n == 4 {
			for i, pos := range f.Positions {
				corners[i] = f.Face.Vertices[i]
				corners[i].X, corners[i].Y = int32(math.Floor(2*pos.X)), int32(math.Floor(2*pos.Y))
				// The shared mapper supplies UV/shade only; its integer depth
				// lane is intentionally unused by this renderer. An attachment
				// under construction reads it as the nanoframe key instead.
				corners[i].Key = 0
				if keyed {
					corners[i].Key = f.Face.Vertices[i].Key
				}
			}
			quad = params.add(corners[:], modelQuadLocalBias, modelQuadLocalBias, 0)
		}
		encoded := float32(f.Face.Color)
		if r.metalGlint {
			encoded = metalGlintColor(f.Face.Color, metalFaceGlint(f.Face.Normal)*f.Face.GlintScale())
		}
		encoded = r.modelFinishColor(&finishContext, &f.Face, encoded)
		mode := float32(0)
		if f.Face.Texture != nil {
			mode = 1
		}
		if f.Face.Shaded {
			mode += 2
		}
		base := uint32(run.vCount)
		for i, pos := range f.Positions {
			v := f.Face.Vertices[i]
			lane := float32(quad)
			if keyed && quad == 0 {
				lane = -float32(min(max(v.Key, 1-modelPreviewKeyOffset), modelPreviewKeyOffset-1) + modelPreviewKeyOffset)
			}
			b.verts = append(b.verts, ebiten.Vertex{
				DstX: float32(2 * pos.X), DstY: float32(2 * pos.Y),
				SrcX: float32(slot.x) + float32(v.U), SrcY: float32(slot.y) + float32(v.V),
				ColorR: float32(v.Shade), ColorG: encoded, ColorB: lane, ColorA: float32(slot.x*4096 + slot.y),
				Custom0: mode, Custom1: float32((pos.Depth - lo) * scale),
				Custom2: float32(max(slot.w-1, 0)), Custom3: float32(max(slot.h-1, 0)),
			})
		}
		for i := 1; i < n-1; i++ {
			b.indices = append(b.indices, base, base+uint32(i), base+uint32(i+1))
		}
		run.vCount += n
		run.iCount += 3 * (n - 2)
	}
}

// packOutline replaces b's contents with each outline pixel as a flat,
// unshaded square over its four 2x samples, its face's depth plane at the
// corners.
func (b *modelPreviewBatch) packOutline(r *Renderer, pixels []drawlist.ModelPreviewOutlinePixel, lo, scale float64) {
	b.reset()
	if len(pixels) == 0 {
		return
	}
	b.runs = append(b.runs, modelPreviewRun{texture: r.placeholderImage()})
	run := &b.runs[0]
	for _, px := range pixels {
		base := uint32(run.vCount)
		for k, corner := range [4][2]int32{{0, 0}, {1, 0}, {1, 1}, {0, 1}} {
			b.verts = append(b.verts, ebiten.Vertex{
				DstX: float32(2 * (px.X + corner[0])), DstY: float32(2 * (px.Y + corner[1])),
				ColorG: float32(px.Color), Custom1: float32((px.Depth[k] - lo) * scale),
			})
		}
		b.indices = append(b.indices, base, base+1, base+2, base, base+2, base+3)
		run.vCount += 4
		run.iCount += 6
	}
}

func (b *modelPreviewBatch) draw(shader *ebiten.Shader, opts *ebiten.DrawTrianglesShaderOptions, dst, depth, tables, params *ebiten.Image) {
	for _, run := range b.runs {
		opts.Images = [4]*ebiten.Image{run.texture, tables, depth, params}
		dst.DrawTrianglesShader32(b.verts[run.vStart:run.vStart+run.vCount], b.indices[run.iStart:run.iStart+run.iCount], shader, opts)
	}
}

func (p *modelPreviewSurface) resetSources(release func(*ebiten.Image)) {
	for _, img := range []*ebiten.Image{p.depth[0], p.depth[1], p.colour, p.attachedDepth, p.attachedColour, p.params.img} {
		if img != nil {
			release(img)
		}
	}
	*p = modelPreviewSurface{shader: p.shader, resolve: p.resolve, compose: p.compose, merge: p.merge}
}
