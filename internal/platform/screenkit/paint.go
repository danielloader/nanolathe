package screenkit

import (
	"image"
	"image/color"
	"math"

	"github.com/hajimehoshi/ebiten/v2"
)

// Rect is a screen rectangle in pixels.
type Rect struct{ X, Y, W, H float64 }

// Contains reports whether a point lies inside r.
func (r Rect) Contains(x, y float64) bool {
	return x >= r.X && y >= r.Y && x < r.X+r.W && y < r.Y+r.H
}

// Inset shrinks r by d on every side.
func (r Rect) Inset(d float64) Rect { return Rect{r.X + d, r.Y + d, r.W - 2*d, r.H - 2*d} }

var (
	white    = newWhite()
	softDisc = newSoftDisc(64)
)

func newWhite() *ebiten.Image {
	img := ebiten.NewImage(3, 3)
	img.Fill(color.White)
	return img
}

// newSoftDisc is a radial falloff used for glows and lamp halos.
func newSoftDisc(n int) *ebiten.Image {
	rgba := image.NewRGBA(image.Rect(0, 0, n, n))
	c := float64(n-1) / 2
	for y := 0; y < n; y++ {
		for x := 0; x < n; x++ {
			d := math.Hypot(float64(x)-c, float64(y)-c) / c
			a := clamp01(1 - d)
			a = a * a * (3 - 2*a)
			v := uint8(a * 255)
			i := rgba.PixOffset(x, y)
			rgba.Pix[i], rgba.Pix[i+1], rgba.Pix[i+2], rgba.Pix[i+3] = v, v, v, v
		}
	}
	return ebiten.NewImageFromImage(rgba)
}

func premul(c color.RGBA) (r, g, b, a float32) {
	a = float32(c.A) / 255
	return float32(c.R) / 255 * a, float32(c.G) / 255 * a, float32(c.B) / 255 * a, a
}

// drawQuad draws src stretched over the rectangle with per-corner colours
// (top-left, top-right, bottom-left, bottom-right), multiplied into the image.
func drawQuad(dst, src *ebiten.Image, x, y, w, h float64, tl, tr, bl, br color.RGBA) {
	b := src.Bounds()
	sx0, sy0, sx1, sy1 := float32(b.Min.X), float32(b.Min.Y), float32(b.Max.X), float32(b.Max.Y)
	v := make([]ebiten.Vertex, 4)
	pts := [4][2]float64{{x, y}, {x + w, y}, {x, y + h}, {x + w, y + h}}
	uvs := [4][2]float32{{sx0, sy0}, {sx1, sy0}, {sx0, sy1}, {sx1, sy1}}
	cols := [4]color.RGBA{tl, tr, bl, br}
	for i := range v {
		r, g, bb, a := premul(cols[i])
		v[i] = ebiten.Vertex{DstX: float32(pts[i][0]), DstY: float32(pts[i][1]), SrcX: uvs[i][0], SrcY: uvs[i][1], ColorR: r, ColorG: g, ColorB: bb, ColorA: a}
	}
	op := &ebiten.DrawTrianglesOptions{Filter: ebiten.FilterLinear}
	op.ColorScaleMode = ebiten.ColorScaleModePremultipliedAlpha
	dst.DrawTriangles(v, []uint16{0, 1, 2, 1, 3, 2}, src, op)
}

// Fill paints a solid rectangle.
func Fill(dst *ebiten.Image, r Rect, c color.RGBA) {
	drawQuad(dst, white.SubImage(image.Rect(1, 1, 2, 2)).(*ebiten.Image), r.X, r.Y, r.W, r.H, c, c, c, c)
}

// VGradient paints a rectangle from top colour to bottom colour.
func VGradient(dst *ebiten.Image, r Rect, top, bottom color.RGBA) {
	drawQuad(dst, white.SubImage(image.Rect(1, 1, 2, 2)).(*ebiten.Image), r.X, r.Y, r.W, r.H, top, top, bottom, bottom)
}

// HGradient paints a rectangle from left colour to right colour.
func HGradient(dst *ebiten.Image, r Rect, left, right color.RGBA) {
	drawQuad(dst, white.SubImage(image.Rect(1, 1, 2, 2)).(*ebiten.Image), r.X, r.Y, r.W, r.H, left, right, left, right)
}

// Glow paints a soft radial halo of colour c centred in r.
func Glow(dst *ebiten.Image, r Rect, c color.RGBA) {
	op := &ebiten.DrawImageOptions{Filter: ebiten.FilterLinear, Blend: ebiten.BlendLighter}
	b := softDisc.Bounds()
	op.GeoM.Scale(r.W/float64(b.Dx()), r.H/float64(b.Dy()))
	op.GeoM.Translate(r.X, r.Y)
	cr, cg, cb, ca := premul(c)
	op.ColorScale.Scale(cr, cg, cb, ca)
	dst.DrawImage(softDisc, op)
}

// Shade darkens under a soft disc, used for drop shadows behind panels.
func Shade(dst *ebiten.Image, r Rect, alpha float64) {
	op := &ebiten.DrawImageOptions{Filter: ebiten.FilterLinear}
	b := softDisc.Bounds()
	op.GeoM.Scale(r.W/float64(b.Dx()), r.H/float64(b.Dy()))
	op.GeoM.Translate(r.X, r.Y)
	op.ColorScale.Scale(0, 0, 0, float32(alpha))
	dst.DrawImage(softDisc, op)
}

// Line strokes a segment.
func Line(dst *ebiten.Image, x0, y0, x1, y1, width float64, c color.RGBA) {
	drawSmallShape(dst, shapeSpec{kind: shapeLine, geometry: [5]float32{float32(x0), float32(y0), float32(x1), float32(y1), float32(width)}, colour: c})
}

// Outline strokes a rectangle's border inside r.
func Outline(dst *ebiten.Image, r Rect, width float64, c color.RGBA) {
	Fill(dst, Rect{r.X, r.Y, r.W, width}, c)
	Fill(dst, Rect{r.X, r.Y + r.H - width, r.W, width}, c)
	Fill(dst, Rect{r.X, r.Y, width, r.H}, c)
	Fill(dst, Rect{r.X + r.W - width, r.Y, width, r.H}, c)
}

// Bevel draws the TA two-tone raised (or, sunken=true, recessed) edge.
func Bevel(dst *ebiten.Image, r Rect, width float64, light, dark color.RGBA, sunken bool) {
	if sunken {
		light, dark = dark, light
	}
	Fill(dst, Rect{r.X, r.Y, r.W, width}, light)
	Fill(dst, Rect{r.X, r.Y, width, r.H}, light)
	Fill(dst, Rect{r.X, r.Y + r.H - width, r.W, width}, dark)
	Fill(dst, Rect{r.X + r.W - width, r.Y, width, r.H}, dark)
}

// Disc paints a filled circle.
func Disc(dst *ebiten.Image, cx, cy, radius float64, c color.RGBA) {
	drawSmallShape(dst, shapeSpec{kind: shapeDisc, geometry: [5]float32{float32(cx), float32(cy), float32(radius)}, colour: c})
}

// Ring strokes a circle.
func Ring(dst *ebiten.Image, cx, cy, radius, width float64, c color.RGBA) {
	drawSmallShape(dst, shapeSpec{kind: shapeRing, geometry: [5]float32{float32(cx), float32(cy), float32(radius), float32(width)}, colour: c})
}

// Image draws src scaled into r. Nearest keeps pixel art crisp.
func Image(dst, src *ebiten.Image, r Rect, alpha float64, nearest bool) {
	b := src.Bounds()
	op := &ebiten.DrawImageOptions{Filter: ebiten.FilterLinear}
	if nearest {
		op.Filter = ebiten.FilterNearest
	}
	op.GeoM.Scale(r.W/float64(b.Dx()), r.H/float64(b.Dy()))
	op.GeoM.Translate(r.X, r.Y)
	op.ColorScale.ScaleAlpha(float32(alpha))
	dst.DrawImage(src, op)
}

// Cover draws src scaled to cover r, cropping the overflow, clipped to r.
// A whole-number scale samples the nearest pixel (Crisp).
func Cover(dst, src *ebiten.Image, r Rect, alpha float64) {
	b := src.Bounds()
	sw, sh := float64(b.Dx()), float64(b.Dy())
	scale := max(r.W/sw, r.H/sh)
	cw, ch := r.W/scale, r.H/scale
	ox, oy := float64(b.Min.X)+(sw-cw)/2, float64(b.Min.Y)+(sh-ch)/2
	sub := src.SubImage(image.Rect(int(ox), int(oy), int(ox+cw), int(oy+ch))).(*ebiten.Image)
	Image(dst, sub, r, alpha, Crisp(scale))
}

// Crisp reports whether an image drawn at scale should sample the nearest
// pixel: a whole-number enlargement of pixel art keeps every pixel square,
// where filtering would blur it.
func Crisp(scale float64) bool {
	return scale >= 1 && math.Abs(scale-math.Round(scale)) < 1e-3
}

// NineSlice stretches src into r keeping corners of size edge (source
// pixels) at scale k.
func NineSlice(dst, src *ebiten.Image, r Rect, edge int, k float64, fillCentre bool) {
	b := src.Bounds()
	e := float64(edge) * k
	xs := [4]float64{r.X, r.X + e, r.X + r.W - e, r.X + r.W}
	ys := [4]float64{r.Y, r.Y + e, r.Y + r.H - e, r.Y + r.H}
	sx := [4]int{b.Min.X, b.Min.X + edge, b.Max.X - edge, b.Max.X}
	sy := [4]int{b.Min.Y, b.Min.Y + edge, b.Max.Y - edge, b.Max.Y}
	for j := 0; j < 3; j++ {
		for i := 0; i < 3; i++ {
			if i == 1 && j == 1 && !fillCentre {
				continue
			}
			sub := src.SubImage(image.Rect(sx[i], sy[j], sx[i+1], sy[j+1])).(*ebiten.Image)
			Image(dst, sub, Rect{xs[i], ys[j], xs[i+1] - xs[i], ys[j+1] - ys[j]}, 1, true)
		}
	}
}

// Poly fills a convex polygon given as x, y pairs, as one triangle fan.
func Poly(dst *ebiten.Image, pts []float64, c color.RGBA) {
	n := len(pts) / 2
	if n < 3 {
		return
	}
	r, g, b, a := premul(c)
	v := make([]ebiten.Vertex, n)
	for i := range v {
		v[i] = ebiten.Vertex{DstX: float32(pts[2*i]), DstY: float32(pts[2*i+1]), SrcX: 1, SrcY: 1, ColorR: r, ColorG: g, ColorB: b, ColorA: a}
	}
	idx := make([]uint16, 0, 3*(n-2))
	for i := 1; i < n-1; i++ {
		idx = append(idx, 0, uint16(i), uint16(i+1))
	}
	//lint:ignore SA1019 Retain the triangle-fan antialiasing path during the compiler upgrade.
	op := &ebiten.DrawTrianglesOptions{AntiAlias: true}
	op.ColorScaleMode = ebiten.ColorScaleModePremultipliedAlpha
	dst.DrawTriangles(v, idx, white, op)
}
