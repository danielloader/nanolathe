package screenkit

import (
	"fmt"
	"image/color"
	"math"

	"github.com/hajimehoshi/ebiten/v2"
)

// Control discs, rings, strokes and convex polygons draw through one Kage
// program that computes each pixel's antialiased coverage analytically
// (DESIGN_INTERFACE_HUD_INPUT §3.17). Shape-local coordinates travel in
// SrcX/SrcY with no source image, and the kind and its parameters in
// Custom0..3, so no uniform changes between shapes and Ebitengine merges a
// run of them into one draw command however they animate.
//
// Coverage is the fraction of the pixel square [x, x+1)×[y, y+1) inside the
// shape, so totals keep the shape's area at any size:
//   - a disc of radius at most 8 takes the exact area of the disc inside the
//     pixel; a larger one treats its edge as straight across the pixel at the
//     radius sqrt(r²−1/12), which removes the bias of a straight edge on a
//     curve and avoids the large-radius cancellation of the exact form;
//   - a ring is the outer disc less the inner;
//   - a stroke is the product of two exact slab coverages, along and across
//     the segment, exact everywhere except within a pixel of its corners;
//   - a polygon is the product of its edges' exact half-plane coverages.
//
// All of it runs on the game goroutine: the program compiles on first use
// and the vertex scratch below is reused by every call. Ebitengine copies
// vertices and indices during each draw call.

// Shape kinds, carried in Custom0.
const (
	shapeDisc = 0
	shapeRing = 1
	shapeLine = 2
	shapePoly = 3
)

// shapeReach pads every quad past the shape's edge: a pixel centre farther
// than half the pixel's diagonal outside an edge has no coverage.
const shapeReach = 1

// shapeLimit bounds coordinates; past float32's integer range a pixel
// position can no longer be represented.
const shapeLimit = 1 << 24

const shapeShaderSource = `//kage:unit pixels

package main

// edge is the fraction of a pixel square inside a straight edge, for the
// pixel centre t inside it (negative outside) and the absolute components a
// and b of the edge normal: the exact box-filtered coverage of a half-plane.
func edge(t, a, b float) float {
	hi := max(a, b)
	lo := min(a, b)
	h := (hi + lo) * 0.5
	if t >= h {
		return 1
	}
	if t <= -h {
		return 0
	}
	m := (hi - lo) * 0.5
	if t < -m {
		u := t + h
		return u * u / (2 * hi * lo)
	}
	if t > m {
		u := h - t
		return 1 - u*u/(2*hi*lo)
	}
	return 0.5 + t/hi
}

// slab is the fraction of the pixel within w of a centre line the pixel
// centre lies x from.
func slab(x, w, a, b float) float {
	return max(edge(w-x, a, b)+edge(w+x, a, b)-1, 0)
}

// sweep is the area under a circle of radius r from 0 to t, 0 <= t <= r.
func sweep(t, r float) float {
	return 0.5 * (t*sqrt(max((r-t)*(r+t), 0)) + r*r*asin(min(t/r, 1)))
}

// quarter is the area of the disc of radius r inside the rectangle from the
// disc centre to (x, y), signed by the rectangle's quadrant.
func quarter(x, y, r float) float {
	s := sign(x) * sign(y)
	ax := min(abs(x), r)
	ay := min(abs(y), r)
	m := min(ax, sqrt(max((r-ay)*(r+ay), 0)))
	return s * (ay*m + sweep(ax, r) - sweep(m, r))
}

// disc is the fraction of the pixel centred p from a disc's centre that lies
// inside the disc of radius r.
func disc(p vec2, r float) float {
	if r <= 0 {
		return 0
	}
	rho := length(p)
	if rho >= r+0.7072 {
		return 0
	}
	if rho <= r-0.7072 {
		return 1
	}
	if r <= 8 {
		v := quarter(p.x+0.5, p.y+0.5, r) - quarter(p.x-0.5, p.y+0.5, r) - quarter(p.x+0.5, p.y-0.5, r) + quarter(p.x-0.5, p.y-0.5, r)
		return clamp(v, 0, 1)
	}
	n := abs(p) / max(rho, 1e-6)
	return edge(sqrt(r*r-1.0/12.0)-rho, n.x, n.y)
}

func Fragment(dstPos vec4, srcPos vec2, color vec4, custom vec4) vec4 {
	// A polygon's edge normals come from its distances' screen gradients.
	// Every pixel takes them before any branch: each primitive is a single
	// kind, but derivatives belong in uniform control flow.
	gx := abs(dfdx(srcPos))
	gy := abs(dfdy(srcPos))
	hx := abs(dfdx(custom.yzw))
	hy := abs(dfdy(custom.yzw))
	k := custom.x
	if k < 0.5 {
		return color * disc(srcPos, custom.y)
	}
	if k < 1.5 {
		return color * max(disc(srcPos, custom.z)-disc(srcPos, custom.y), 0)
	}
	if k < 2.5 {
		b := sqrt(max(1-custom.w*custom.w, 0))
		return color * slab(srcPos.x, custom.y, custom.w, b) * slab(srcPos.y, custom.z, custom.w, b)
	}
	c := edge(-srcPos.x, gx.x, gy.x) * edge(-srcPos.y, gx.y, gy.y)
	c *= edge(-custom.y, hx.x, hy.x) * edge(-custom.z, hx.y, hy.y) * edge(-custom.w, hx.z, hy.z)
	return color * c
}
`

var (
	shapeShader  *ebiten.Shader
	shapeOptions ebiten.DrawTrianglesShaderOptions
	shapeQuad    [4]ebiten.Vertex
	shapeQuadIdx = [6]uint32{0, 1, 2, 1, 3, 2}

	polyPoints []polyPoint
	polyEdges  []polyEdge
	polyVerts  []ebiten.Vertex
	polyIdx    []uint32
)

func shapeProgram() *ebiten.Shader {
	if shapeShader == nil {
		s, err := ebiten.NewShader([]byte(shapeShaderSource))
		if err != nil {
			panic(fmt.Sprintf("screenkit: compile shape shader: %v", err))
		}
		shapeShader = s
	}
	return shapeShader
}

// shapeColour is a vertex colour, already alpha-premultiplied.
type shapeColour [4]float32

// premultipliedColour reads c as image/color defines color.RGBA: already
// alpha-premultiplied. Disc, Ring and Line keep the vector painter's
// reading of their colour this way.
func premultipliedColour(c color.RGBA) shapeColour {
	return shapeColour{float32(c.R) / 255, float32(c.G) / 255, float32(c.B) / 255, float32(c.A) / 255}
}

func usable(vs ...float64) bool {
	for _, v := range vs {
		if !(math.Abs(v) <= shapeLimit) {
			return false
		}
	}
	return true
}

// setShapeQuad fills q with the rectangle centred (cx, cy) whose local x
// axis is the unit vector (ux, uy), with half-extents ex and ey. Each vertex
// carries its local coordinates, so they interpolate exactly across it.
func setShapeQuad(q *[4]ebiten.Vertex, cx, cy, ux, uy, ex, ey float64, c shapeColour, kind float32, p1, p2, p3 float64) {
	for i := range q {
		lx, ly := ex, ey
		if i&1 == 0 {
			lx = -ex
		}
		if i < 2 {
			ly = -ey
		}
		q[i] = ebiten.Vertex{
			DstX: float32(cx + lx*ux - ly*uy), DstY: float32(cy + lx*uy + ly*ux),
			SrcX: float32(lx), SrcY: float32(ly),
			ColorR: c[0], ColorG: c[1], ColorB: c[2], ColorA: c[3],
			Custom0: kind, Custom1: float32(p1), Custom2: float32(p2), Custom3: float32(p3),
		}
	}
}

// discQuad prepares a filled disc; it reports false when nothing would draw.
func discQuad(q *[4]ebiten.Vertex, cx, cy, radius float64, c color.RGBA) bool {
	if !(radius > 0) || !usable(cx, cy, radius) || c == (color.RGBA{}) {
		return false
	}
	e := radius + shapeReach
	setShapeQuad(q, cx, cy, 1, 0, e, e, premultipliedColour(c), shapeDisc, radius, 0, 0)
	return true
}

// ringQuad prepares a stroke of width centred on the radius. A stroke wider
// than the diameter is the whole outer disc.
func ringQuad(q *[4]ebiten.Vertex, cx, cy, radius, width float64, c color.RGBA) bool {
	if !(radius > 0) || !(width > 0) || !usable(cx, cy, radius, width) || c == (color.RGBA{}) {
		return false
	}
	inner, outer := max(radius-width/2, 0), radius+width/2
	e := outer + shapeReach
	setShapeQuad(q, cx, cy, 1, 0, e, e, premultipliedColour(c), shapeRing, inner, outer, 0)
	return true
}

// lineQuad prepares a butt-capped stroke: the rectangle of the segment's
// length and the width, centred on the segment.
func lineQuad(q *[4]ebiten.Vertex, x0, y0, x1, y1, width float64, c color.RGBA) bool {
	if !(width > 0) || !usable(x0, y0, x1, y1, width) || c == (color.RGBA{}) {
		return false
	}
	dx, dy := x1-x0, y1-y0
	length := span(dx, dy)
	if !(length > 0) {
		return false
	}
	ux, uy := dx/length, dy/length
	a, b := length/2, width/2
	setShapeQuad(q, (x0+x1)/2, (y0+y1)/2, ux, uy, a+shapeReach, b+shapeReach, premultipliedColour(c), shapeLine, a, b, max(math.Abs(ux), math.Abs(uy)))
	return true
}

func drawShapeQuad(dst *ebiten.Image) {
	dst.DrawTrianglesShader32(shapeQuad[:], shapeQuadIdx[:], shapeProgram(), &shapeOptions)
}

type polyPoint struct{ x, y float64 }

// polyEdge is an edge's outward unit normal and offset: a point's distance
// outside the edge is nx·x + ny·y − c.
type polyEdge struct{ nx, ny, c float64 }

func (e polyEdge) distance(p polyPoint) float64 { return e.nx*p.x + e.ny*p.y - e.c }

// polyFan appends the triangles that draw the convex polygon pts in colour
// c (premultiplied) to vs. The polygon is pushed out by shapeReach so its
// edge coverage has room, and fanned from its first vertex. Each fan
// triangle carries its distance to the five edges that can lie within a
// pixel of it: the two at the fan's apex and the three around its far
// side. Distances are affine, so they interpolate exactly; the shader reads
// each edge's normal from their screen gradients.
func polyFan(vs []ebiten.Vertex, pts []float64, c shapeColour) []ebiten.Vertex {
	vs = vs[:0]
	if c == (shapeColour{}) {
		return vs
	}
	ps := polyPoints[:0]
	for i := 0; i+1 < len(pts); i += 2 {
		p := polyPoint{pts[i], pts[i+1]}
		if !usable(p.x, p.y) {
			polyPoints = ps
			return vs
		}
		if len(ps) == 0 || p != ps[len(ps)-1] {
			ps = append(ps, p)
		}
	}
	for len(ps) > 1 && ps[0] == ps[len(ps)-1] {
		ps = ps[:len(ps)-1]
	}
	polyPoints = ps
	n := len(ps)
	if n < 3 {
		return vs
	}
	area := 0.0
	for i, p := range ps {
		q := ps[(i+1)%n]
		area += p.x*q.y - q.x*p.y
	}
	if !(area != 0) {
		return vs
	}
	orient := 1.0
	if area < 0 {
		orient = -1
	}
	es := polyEdges[:0]
	for i, p := range ps {
		q := ps[(i+1)%n]
		dx, dy := q.x-p.x, q.y-p.y
		l := span(dx, dy)
		nx, ny := orient*dy/l, -orient*dx/l
		es = append(es, polyEdge{nx, ny, nx*p.x + ny*p.y})
	}
	polyEdges = es
	// Each outset vertex lies shapeReach outside both of its edges. A very
	// acute corner is capped rather than reaching far past the polygon.
	outset := func(i int) polyPoint {
		a, b := es[(i+n-1)%n], es[i]
		k := shapeReach / max(1+a.nx*b.nx+a.ny*b.ny, 0.02)
		return polyPoint{ps[i].x + k*(a.nx+b.nx), ps[i].y + k*(a.ny+b.ny)}
	}
	apex := outset(0)
	next := outset(1)
	for i := 1; i+1 < n; i++ {
		far := outset(i + 1)
		var slots [5]int
		used := 0
		for _, e := range [5]int{n - 1, 0, i - 1, i, i + 1} {
			seen := false
			for _, s := range slots[:used] {
				seen = seen || s == e
			}
			if !seen {
				slots[used] = e
				used++
			}
		}
		for _, p := range [3]polyPoint{apex, next, far} {
			var d [5]float32
			for k := range d {
				d[k] = -1
				if k < used {
					d[k] = float32(es[slots[k]].distance(p))
				}
			}
			vs = append(vs, ebiten.Vertex{
				DstX: float32(p.x), DstY: float32(p.y),
				SrcX: d[0], SrcY: d[1],
				ColorR: c[0], ColorG: c[1], ColorB: c[2], ColorA: c[3],
				Custom0: shapePoly, Custom1: d[2], Custom2: d[3], Custom3: d[4],
			})
		}
		next = far
	}
	return vs
}

func drawPoly(dst *ebiten.Image, pts []float64, c shapeColour) {
	polyVerts = polyFan(polyVerts, pts, c)
	if len(polyVerts) == 0 {
		return
	}
	for len(polyIdx) < len(polyVerts) {
		polyIdx = append(polyIdx, uint32(len(polyIdx)))
	}
	dst.DrawTrianglesShader32(polyVerts, polyIdx[:len(polyVerts)], shapeProgram(), &shapeOptions)
}
