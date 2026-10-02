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
)

type modelPreviewRun struct {
	texture        *ebiten.Image
	vStart, iStart int
	vCount, iCount int
}

type modelPreviewSurface struct {
	shader, resolve *ebiten.Shader
	depth           [2]*ebiten.Image
	colour          *ebiten.Image
	w, h            int
	params          modelQuadParams
	verts           []ebiten.Vertex
	indices         []uint32
	runs            []modelPreviewRun
	opts            ebiten.DrawTrianglesShaderOptions
}

// DrawModelPreview draws one complete isolated tool model over dst. It uses
// geometric, non-wrapping depth even for assets without a retail key plane.
// It is an explicit tool entry, never selected by a battle draw command.
// dst is the finished RGB composite (call Expand first after Execute).
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
	p.pack(r, g, lo, scale)
	if len(p.indices) == 0 {
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
	p.opts.Blend = ebiten.Blend{
		BlendFactorSourceRGB: ebiten.BlendFactorOne, BlendFactorSourceAlpha: ebiten.BlendFactorOne,
		BlendFactorDestinationRGB: ebiten.BlendFactorOne, BlendFactorDestinationAlpha: ebiten.BlendFactorOne,
		BlendOperationRGB: ebiten.BlendOperationMax, BlendOperationAlpha: ebiten.BlendOperationMax,
	}
	previous := r.placeholderImage()
	for pass := range 3 {
		target := p.depth[pass&1]
		target.Clear()
		p.opts.Uniforms["Pass"] = float32(pass)
		p.draw(target, previous, r.tables.atlas, params)
		previous = target
	}
	p.colour.Clear()
	p.opts.Uniforms["Pass"] = float32(3)
	p.opts.Blend = ebiten.BlendCopy
	p.draw(p.colour, previous, r.tables.atlas, params)
	// Box-resolve all four covered samples, including transparent samples at
	// shared edges, onto the existing background with premultiplied alpha.
	var resolve ebiten.DrawRectShaderOptions
	resolve.Images[0] = p.colour
	resolve.GeoM.Scale(0.5, 0.5)
	dst.DrawRectShader(2*w, 2*h, p.resolve, &resolve)
	return nil
}

func modelPreviewDepthBounds(g *drawlist.ModelPreviewGeometry) (lo, hi float64, err error) {
	lo, hi = math.Inf(1), math.Inf(-1)
	for _, f := range g.Faces {
		if len(f.Positions) != len(f.Face.Vertices) {
			return 0, 0, fmt.Errorf("nanolathe: rendering model preview: logical path <projected face>, providers searched [GPU preview], expected matching corners and attributes")
		}
		for _, v := range f.Positions {
			if math.IsNaN(v.X) || math.IsNaN(v.Y) || math.IsNaN(v.Depth) || math.IsInf(v.X, 0) || math.IsInf(v.Y, 0) || math.IsInf(v.Depth, 0) || math.Abs(v.X) > 1<<20 || math.Abs(v.Y) > 1<<20 {
				return 0, 0, fmt.Errorf("nanolathe: rendering model preview: logical path <projected vertex>, providers searched [GPU preview], expected finite representable coordinates")
			}
			lo, hi = min(lo, v.Depth), max(hi, v.Depth)
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
	for _, img := range []*ebiten.Image{p.depth[0], p.depth[1], p.colour} {
		if img != nil {
			img.Deallocate()
		}
	}
	p.depth = [2]*ebiten.Image{newRendererImage(w, h), newRendererImage(w, h)}
	p.colour = newRendererImage(w, h)
	p.w, p.h = w, h
}

func (p *modelPreviewSurface) pack(r *Renderer, g *drawlist.ModelPreviewGeometry, lo, scale float64) {
	p.verts, p.indices = p.verts[:0], p.indices[:0]
	clear(p.runs)
	p.runs = p.runs[:0]
	p.params.reset()
	var corners [4]drawlist.ModelVertex
	var finishContext modelPlaceCtx
	// Reverse the immutable recorded order so the earliest admitted face
	// owns a near-coincident tie, independent of orbit or texture-page runs.
	for fi := len(g.Faces) - 1; fi >= 0; fi-- {
		f := &g.Faces[fi]
		n := len(f.Positions)
		if n < 3 {
			continue
		}
		var area float64
		a := f.Positions[n-1]
		for _, b := range f.Positions {
			area += a.X*b.Y - b.X*a.Y
			a = b
		}
		if area <= 0 {
			continue
		}
		slot := r.modelTextureFor(f.Face.Texture)
		texture := slot.img
		if texture == nil {
			texture = r.placeholderImage()
		}
		if len(p.runs) == 0 || p.runs[len(p.runs)-1].texture != texture {
			p.runs = append(p.runs, modelPreviewRun{texture: texture, vStart: len(p.verts), iStart: len(p.indices)})
		}
		run := &p.runs[len(p.runs)-1]
		quad := 0
		if n == 4 {
			for i, pos := range f.Positions {
				corners[i] = f.Face.Vertices[i]
				corners[i].X, corners[i].Y = int32(math.Floor(2*pos.X)), int32(math.Floor(2*pos.Y))
				// The shared mapper supplies UV/shade only; its integer depth
				// lane is intentionally unused by this renderer.
				corners[i].Key = 0
			}
			quad = p.params.add(corners[:], modelQuadLocalBias, modelQuadLocalBias, 0)
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
			p.verts = append(p.verts, ebiten.Vertex{
				DstX: float32(2 * pos.X), DstY: float32(2 * pos.Y),
				SrcX: float32(slot.x) + float32(v.U), SrcY: float32(slot.y) + float32(v.V),
				ColorR: float32(v.Shade), ColorG: encoded, ColorB: float32(quad), ColorA: float32(slot.x*4096 + slot.y),
				Custom0: mode, Custom1: float32((pos.Depth - lo) * scale),
				Custom2: float32(max(slot.w-1, 0)), Custom3: float32(max(slot.h-1, 0)),
			})
		}
		for i := 1; i < n-1; i++ {
			p.indices = append(p.indices, base, base+uint32(i), base+uint32(i+1))
		}
		run.vCount += n
		run.iCount += 3 * (n - 2)
	}
}

func (p *modelPreviewSurface) draw(dst, depth, tables, params *ebiten.Image) {
	for _, run := range p.runs {
		p.opts.Images = [4]*ebiten.Image{run.texture, tables, depth, params}
		dst.DrawTrianglesShader32(p.verts[run.vStart:run.vStart+run.vCount], p.indices[run.iStart:run.iStart+run.iCount], p.shader, &p.opts)
	}
}

func (p *modelPreviewSurface) resetSources(release func(*ebiten.Image)) {
	for _, img := range []*ebiten.Image{p.depth[0], p.depth[1], p.colour, p.params.img} {
		if img != nil {
			release(img)
		}
	}
	*p = modelPreviewSurface{shader: p.shader, resolve: p.resolve}
}
