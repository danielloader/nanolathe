package gpurender

import (
	"fmt"
	"math"

	"github.com/hajimehoshi/ebiten/v2"
)

// Ground illumination for the Enhanced battle lights
// (docs/DESIGN_GPU_RENDERER.md §31.8). Selected sources accumulate into a
// bounded field before one linear-light resolve brightens the painted terrain.
//
// It is presentation only, Enhanced only, and gated by the ground light switch
// alone (§30): with the switch off nothing is copied, nothing is batched and
// the composite is byte-identical to the executor without it, whatever the
// model light switch says.

// groundLightGain scales the optical density of the bounded light field.
// This is authored Enhanced presentation, not retail lighting (§31.8).
const groundLightGain = 2.0

// groundKindScale is each family's share of groundLightGain at the TERRAIN
// receiver; model and smoke receivers keep the full colour they always had
// (§31.6–§31.8). Terrain is the receiver that reads as a shape — a pool on
// open ground is a circle the eye finds — so the families that stand for a
// small, moving, short-lived source are held well below the standing ones.
//
// Every value is an authored presentation choice, not retail arithmetic. Fire
// and spark were tuned AFTER the receiver height of §31.7 landed: measured from
// the sea datum a pool on high ground carried a large standing attenuation, and
// a share picked against that reads bleached once the attenuation is gone.
var groundKindScale = [lightKindCount]float32{
	lightExplosion:  0.75 / groundLightGain, // §31.6's peak, before its envelope
	lightNano:       0.45,
	lightFire:       0.3, // a burning place still lights the ground it stands on
	lightProjectile: 0.65,
	lightWreck:      0.65,
	lightSpark:      0.15, // a burning fragment lights almost nothing
}

// explosionGroundScale multiplies only the terrain receiver's emission.
// Authored presentation tuning: 0.75 peak gain instead of 2, two ticks of
// peak followed by squared decay to zero at twelve ticks (GPU design §31.6).
func explosionGroundScale(age float32, known bool) float32 {
	peak := groundKindScale[lightExplosion]
	if !known {
		return peak
	}
	if age < 0 || age >= 12 {
		return 0
	}
	tail := 1 - max(age-2, 0)/10
	return peak * tail * tail
}

// groundScale is the terrain receiver's whole per-source multiplier: the
// family's share, the explosion's own envelope, and the source's own remaining
// emission where its producer carried one (§31.7).
func groundScale(light *battleLight) float32 {
	// add() rejects any kind at or past lightKindCount, so the table index is
	// always in range and needs no bound of its own.
	scale := groundKindScale[light.kind]
	if light.kind == lightExplosion {
		// The explosion's share is the table's entry as well, taken through the
		// envelope §31.6 wraps around it.
		scale = explosionGroundScale(light.age, light.ageKnown)
	}
	if light.fadeKnown {
		scale *= light.fade
	}
	return scale
}

type groundLighting struct {
	shader        *ebiten.Shader
	resolveShader *ebiten.Shader
	clearShader   *ebiten.Shader
	resolveOpts   ebiten.DrawTrianglesShaderOptions
	clearOpts     ebiten.DrawTrianglesShaderOptions
	// verts are the frame's discs in screen space; fieldVerts are the same
	// discs mapped into the field's region of the read surface (§31.8).
	verts      []ebiten.Vertex
	fieldVerts []ebiten.Vertex
	indices    []uint32
	opts       ebiten.DrawTrianglesShaderOptions
	// clearVerts and resolveVerts are the pass's two single-quad draws.
	clearVerts, resolveVerts [4]ebiten.Vertex
	// fieldRect is the field's x, y, width and height in the read surface,
	// set with the surfaces (ensureSize). fieldFits is false when neither
	// placement fits the device's largest texture; the pass then draws nothing.
	fieldRect [4]int
	fieldFits bool
	// read is the union of the discs' clipped quads. The resolve samples the
	// fragment's own pixel and the field under it and nothing else, so the
	// quads are exactly the region the copy has to carry: a couple of
	// explosions no longer cost a full-frame blit (readcopy.go).
	read readRect
}

// groundFieldDivisor is the light field's resolution divisor. Every pool ends
// in §31.7's smoothstep, so the field is smooth at the scale of its texels: a
// half-resolution field reconstructed bilinearly stays within a display byte
// or two of a full-resolution one for pools a few dozen pixels wide, at a
// quarter of the texels. The smallest pools, seen zoomed out, differ by a few
// bytes more (§31.8).
const groundFieldDivisor = 2

// groundFieldLayout sizes the read surface for a w×h frame: the read copy
// occupies its top-left w×h, and the field sits beside or below it. Of the
// placements within maxSide (the device's largest texture side; 0 means no
// limit), it takes the one whose texture is smaller, below on a tie.
// Ebitengine stores each image in a texture rounded up to powers of two, so at
// most frame sizes one placement fits in space that texture already has, and
// the field costs no memory of its own (§31.8). field is x, y, width, height.
// When neither placement fits, ok is false and the read surface is the frame.
func groundFieldLayout(w, h, maxSide int) (readW, readH int, field [4]int, ok bool) {
	fw, fh := (w+groundFieldDivisor-1)/groundFieldDivisor, (h+groundFieldDivisor-1)/groundFieldDivisor
	fits := func(a, b int) bool { return maxSide <= 0 || (a <= maxSide && b <= maxSide) }
	below, beside := fits(w, h+fh), fits(w+fw, h)
	if below && (!beside || texturePow2(w)*texturePow2(h+fh) <= texturePow2(w+fw)*texturePow2(h)) {
		return w, h + fh, [4]int{0, h, fw, fh}, true
	}
	if beside {
		return w + fw, h, [4]int{w, 0, fw, fh}, true
	}
	return w, h, [4]int{}, false
}

// texturePow2 is the power-of-two texture extent Ebitengine allocates for an
// image extent of n. It only ranks the two placements above; a different
// backend rounding would cost memory, not correctness.
func texturePow2(n int) int {
	p := 1
	for p < n {
		p <<= 1
	}
	return p
}

// drawGroundLighting runs at the end of the terrain pass, after the water
// surface and its reflections have resolved — so the pools brighten the water
// too, which is intended — and before objects, wakes and scorch, so units are
// drawn over it and the ordinary fog composite covers it (§26.3, §31).
//
// It costs two passes in a lit frame and nothing in a frame without a visible
// light (§31.8): one into the read surface carrying the composite copy, the
// field clear and the field batch, and the resolve back onto the composite.
func (r *Renderer) drawGroundLighting() {
	g := &r.ground
	if r.lighting.groundDisabled || 1+r.lighting.groundStrengthOffset <= 0 || len(r.lighting.lights) == 0 || g.shader == nil || g.resolveShader == nil || g.clearShader == nil || !g.fieldFits || r.surfaces[0] == nil || r.surfaces[1] == nil {
		return
	}
	r.appendGroundLights()
	if len(g.indices) == 0 {
		return
	}
	// The resolve rewrites the composite it reads, so everything under the
	// pools has to be on the composite before the copy is taken.
	r.submitSchedule()
	read, f := r.surfaces[1], g.fieldRect
	r.copyComposite(read, r.surfaces[0], g.read.x0, g.read.y0, g.read.x1, g.read.y1)

	// The field region keeps the previous frame's light, so zero every texel
	// the resolve's bilinear taps can reach, then accumulate: screen(1-exp(-E))
	// yields 1-exp(-sum(E)) without an HDR target or a per-light ceiling. The
	// copy, clear and batch share one destination, so they are one pass.
	d := groundFieldDivisor
	x0, y0 := max(g.read.x0/d-1, 0), max(g.read.y0/d-1, 0)
	x1, y1 := min((g.read.x1+d-1)/d+1, f[2]), min((g.read.y1+d-1)/d+1, f[3])
	setQuad(&g.clearVerts, float32(f[0]+x0), float32(f[1]+y0), float32(f[0]+x1), float32(f[1]+y1), [4]float32{})
	g.clearOpts.Blend = ebiten.BlendCopy
	r.recordSubmission(len(g.clearVerts), len(r.copyIdx))
	read.DrawTrianglesShader32(g.clearVerts[:], r.copyIdx[:], g.clearShader, &g.clearOpts)
	r.frameDraws++
	g.appendFieldVerts(f)
	g.opts.Blend = groundFieldBlend
	r.recordSubmission(len(g.fieldVerts), len(g.indices))
	read.DrawTrianglesShader32(g.fieldVerts, g.indices, g.shader, &g.opts)
	r.frameDraws++

	// Every terrain pixel under the pools is resolved once, regardless of the
	// number of lights. The quad's custom lanes carry the field's rectangle.
	setQuad(&g.resolveVerts, float32(g.read.x0), float32(g.read.y0), float32(g.read.x1), float32(g.read.y1),
		[4]float32{float32(f[0]), float32(f[1]), float32(f[2]), float32(f[3])})
	g.resolveOpts.Images = [4]*ebiten.Image{read}
	g.resolveOpts.Blend = ebiten.BlendCopy
	r.beginPass(r.surfaces[0])
	r.recordSubmission(len(g.resolveVerts), len(r.copyIdx))
	r.surfaces[0].DrawTrianglesShader32(g.resolveVerts[:], r.copyIdx[:], g.resolveShader, &g.resolveOpts)
	r.frameDraws++
}

// setQuad fills a corner-ordered quad matching r.copyIdx, with matching
// destination and source positions and the given custom lanes.
func setQuad(q *[4]ebiten.Vertex, x0, y0, x1, y1 float32, custom [4]float32) {
	for i, c := range [4][2]float32{{x0, y0}, {x1, y0}, {x0, y1}, {x1, y1}} {
		q[i] = ebiten.Vertex{DstX: c[0], DstY: c[1], SrcX: c[0], SrcY: c[1],
			Custom0: custom[0], Custom1: custom[1], Custom2: custom[2], Custom3: custom[3]}
	}
}

// appendFieldVerts maps the screen-space discs into the field: a 1/divisor
// scale about the frame origin, then the field's offset in the read surface.
// The disc test is a ratio of squared distances, so scaling the centre, height
// and radius together leaves every pool's shape unchanged. Each quad is
// rounded outward to whole field texels, so a disc clipped at the frame's
// right or bottom edge still writes the field's last texel there.
func (g *groundLighting) appendFieldVerts(f [4]int) {
	const inv = 1 / float32(groundFieldDivisor)
	fx, fy := float32(f[0]), float32(f[1])
	g.fieldVerts = append(g.fieldVerts[:0], g.verts...)
	for i := 0; i+3 < len(g.fieldVerts); i += 4 {
		q := g.fieldVerts[i : i+4 : i+4]
		x0, y0 := float32(math.Floor(float64(q[0].DstX*inv))), float32(math.Floor(float64(q[0].DstY*inv)))
		x1 := min(float32(math.Ceil(float64(q[3].DstX*inv))), float32(f[2]))
		y1 := min(float32(math.Ceil(float64(q[3].DstY*inv))), float32(f[3]))
		for j, c := range [4][2]float32{{x0, y0}, {x1, y0}, {x0, y1}, {x1, y1}} {
			v := &q[j]
			v.DstX, v.DstY = fx+c[0], fy+c[1]
			v.SrcX, v.SrcY = v.DstX, v.DstY
			v.Custom0, v.Custom1 = fx+v.Custom0*inv, fy+v.Custom1*inv
			v.Custom2, v.Custom3 = v.Custom2*inv, v.Custom3*inv
		}
	}
}

// appendGroundLights builds the frame's clipped discs. A light whose disc falls
// entirely outside the framebuffer contributes no geometry, so a battle beyond
// the viewport costs this pass nothing.
func (r *Renderer) appendGroundLights() {
	l := &r.lighting
	g := &r.ground
	g.verts, g.indices = g.verts[:0], g.indices[:0]
	g.read.reset()
	// The world transform of §16.3 applies exactly once, here, as it does for
	// the distortion batch: the lights are in record coordinates and the quads
	// this pass submits are device geometry.
	k := r.sched.txf(1)
	// The player's ground light strength (§30) multiplies every pool alike.
	strength := 1 + l.groundStrengthOffset
	for i := range l.lights {
		light := &l.lights[i]
		gain := groundScale(light) * strength
		if gain <= 0 {
			continue
		}
		// This pass overlays the painted terrain without a receiver height.
		// Centre its pool on the projected source, as the visible art is, rather
		// than treating the unsheared world row as a terrain pixel (SC20).
		// Physical model/smoke distances still use the unsheared source (§23.2).
		gx, gy := r.sched.txx(light.position[0]), r.sched.txy(light.position[1]-light.position[2]*0.5)
		// The source's height ABOVE THE GROUND under it. Measured from the sea
		// datum instead — as this pass had to before the producers carried a
		// receiver height — every pool narrower than the map's own elevation is
		// discarded outright by the reach test below, which is what suppressed
		// a spark's pool anywhere the ground rises (§31.5, §31.7).
		height := max(light.position[2]-light.ground, 0) * k
		radius := light.radius * k
		if radius <= 0 || height >= radius {
			// Every ground point is already past the radius; the disc is empty.
			continue
		}
		// The radius bounds the disc the light reaches on the ground plane: a
		// lifted light reaches slightly less than that, and the shader's own
		// distance test discards the difference, so the quad is a conservative
		// cover rather than an exact one and needs no square root [I2].
		reach := radius
		x0 := max(float32(math.Floor(float64(gx-reach))), 0)
		y0 := max(float32(math.Floor(float64(gy-reach))), 0)
		x1 := min(float32(math.Ceil(float64(gx+reach))), float32(r.w))
		y1 := min(float32(math.Ceil(float64(gy+reach))), float32(r.h))
		if x0 >= x1 || y0 >= y1 {
			continue
		}
		base := uint32(len(g.verts))
		for _, p := range [4][2]float32{{x0, y0}, {x1, y0}, {x0, y1}, {x1, y1}} {
			g.verts = append(g.verts, ebiten.Vertex{DstX: p[0], DstY: p[1], SrcX: p[0], SrcY: p[1],
				ColorR: light.color[0] * gain, ColorG: light.color[1] * gain, ColorB: light.color[2] * gain, ColorA: 0,
				Custom0: gx, Custom1: gy, Custom2: height, Custom3: radius})
		}
		g.indices = append(g.indices, base, base+1, base+2, base+1, base+2, base+3)
		g.read.add(x0, y0, x1, y1)
		r.modelStats.GroundLights++
	}
}

func newGroundLightShader() (*ebiten.Shader, error) {
	return compileShader(groundLightShaderSource)
}

// The field is colour data with opaque alpha wherever a disc writes. Keeping
// its alpha valid avoids relying on non-premultiplied image storage (§31.8).
var groundFieldBlend = ebiten.Blend{
	BlendFactorSourceRGB:        ebiten.BlendFactorOne,
	BlendFactorDestinationRGB:   ebiten.BlendFactorOneMinusSourceColor,
	BlendFactorSourceAlpha:      ebiten.BlendFactorOne,
	BlendFactorDestinationAlpha: ebiten.BlendFactorOneMinusSourceAlpha,
	BlendOperationRGB:           ebiten.BlendOperationAdd,
	BlendOperationAlpha:         ebiten.BlendOperationAdd,
}

// Shared transfer functions apply to palette display colours, not to already
// linear energy. The recorded light's peak retains its authored strength;
// converting its normalized hue cannot change the strength slider's meaning.
const groundTransferSource = `
func groundLinear(c vec3) vec3 {
 return mix(c/12.92, pow((c+vec3(0.055))/1.055, vec3(2.4)), step(vec3(0.04045),c))
}
func groundDisplay(c vec3) vec3 {
 return mix(c*12.92, 1.055*pow(c,vec3(1.0/2.4))-vec3(0.055), step(vec3(0.0031308),c))
}
`

var groundLightShaderSource = `//kage:unit pixels
package main
` + groundTransferSource + fmt.Sprintf(`
func Fragment(dst vec4, src vec2, color vec4, light vec4) vec4 {
 p := dst.xy-imageDstOrigin()
 d := p-light.xy
 r := light.w
 d2 := dot(d,d)+light.z*light.z
 if d2 >= r*r { discard() }
 falloff := 1.0-d2/(r*r)
 falloff = falloff*falloff*(3.0-2.0*falloff)
 peak := max(color.r,max(color.g,color.b))
 energy := groundLinear(color.rgb/max(peak,0.000001))*peak
 return vec4(vec3(1.0)-exp(-energy*(falloff*%v)), 1.0)
}
`, groundLightGain)

// Painted map colour is a reflectance proxy, not an unlit material. A bounded
// reflected contribution leaves its authored shadows and highlights intact:
// Lift the reflectance proxy R to R*(2-R), then resolve in linear light as
// out = base + (1-base)*liftedReflectance*field. Even a saturated
// field cannot turn a grey ramp into a flat white patch (§31.8).
const groundHueMix = 0.25

// The resolve reads both inputs from the read surface: the copy at the
// fragment's own pixel, and the field (rectangle x, y, width, height in the
// custom lanes) reconstructed bilinearly at 1/divisor scale. Each tap is
// clamped to the field's own rectangle, so the frame's edges never blend with
// the read copy beside it or with the texture beyond it.
var groundResolveShaderSource = `//kage:unit pixels
package main
` + groundTransferSource + fmt.Sprintf(`
func Fragment(dst vec4, src vec2, color vec4, region vec4) vec4 {
 p := dst.xy-imageDstOrigin()
 o := imageSrc0Origin()
 original := imageSrc0At(p+o)
 q := p/%d.0-vec2(0.5)
 i := floor(q)
 f := q-i
 hi := region.zw-vec2(0.5)
 a := o+region.xy+clamp(i+vec2(0.5),vec2(0.5),hi)
 b := o+region.xy+clamp(i+vec2(1.5),vec2(0.5),hi)
 field := mix(mix(imageSrc0At(a).rgb,imageSrc0At(vec2(b.x,a.y)).rgb,f.x),
  mix(imageSrc0At(vec2(a.x,b.y)).rgb,imageSrc0At(b).rgb,f.x),f.y)
 if max(field.r,max(field.g,field.b)) <= 0.0 { return original }
 base := groundLinear(original.rgb/max(original.a,0.000001))
 reflected := mix(base,vec3(dot(base,vec3(0.2126,0.7152,0.0722))),%v)
 reflected = reflected*(vec3(2.0)-reflected)
 lit := base+(vec3(1.0)-base)*reflected*field
 return vec4(groundDisplay(lit)*original.a,original.a)
}
`, groundFieldDivisor, groundHueMix)

func newGroundResolveShader() (*ebiten.Shader, error) {
	return compileShader(groundResolveShaderSource)
}

// groundClearShaderSource zeroes the field texels a frame's resolve can
// sample, inside the same pass as the copy and the field batch.
const groundClearShaderSource = `//kage:unit pixels
package main

func Fragment(dst vec4, src vec2, color vec4) vec4 { return vec4(0) }
`

func newGroundClearShader() (*ebiten.Shader, error) {
	return compileShader(groundClearShaderSource)
}
