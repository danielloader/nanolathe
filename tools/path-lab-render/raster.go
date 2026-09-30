package main

import (
	"fmt"
	"image"
	"image/draw"
	"math"
	"strconv"
)

// rgba is a colour with straight opacity a in 0..1; the channels are 0..255.
type rgba struct{ r, g, b, a float64 }

// hex parses "#rrggbb".
func hex(s string) rgba {
	if len(s) != 7 || s[0] != '#' {
		panic(fmt.Sprintf("hex colour %q", s))
	}
	v, err := strconv.ParseUint(s[1:], 16, 32)
	if err != nil {
		panic(fmt.Sprintf("hex colour %q", s))
	}
	return rgba{float64(v >> 16 & 0xff), float64(v >> 8 & 0xff), float64(v & 0xff), 1}
}

func (c rgba) alpha(a float64) rgba {
	c.a = a
	return c
}

func (c rgba) hex() string {
	return fmt.Sprintf("#%02x%02x%02x", uint8(c.r+0.5), uint8(c.g+0.5), uint8(c.b+0.5))
}

type pt struct{ x, y float64 }

// canvas draws antialiased shapes onto an opaque RGBA picture. Coordinates
// are relative to the canvas origin and clipped to its rectangle, so a panel
// is drawn into its place in a larger picture without spilling into its
// neighbours.
type canvas struct {
	img    *image.RGBA
	clip   image.Rectangle
	ox, oy float64
	m      *mask
}

func newCanvas(img *image.RGBA) *canvas {
	return &canvas{img: img, clip: img.Rect, m: &mask{r: img.Rect, a: make([]float32, img.Rect.Dx()*img.Rect.Dy())}}
}

// newPicture returns an opaque picture filled with bg.
func newPicture(w, h int, bg rgba) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	p := [4]uint8{uint8(bg.r), uint8(bg.g), uint8(bg.b), 255}
	for i := 0; i < len(img.Pix); i += 4 {
		copy(img.Pix[i:i+4], p[:])
	}
	return img
}

// sub is the canvas for rectangle r of c's picture, with its origin at r's
// corner.
func (c *canvas) sub(r image.Rectangle) *canvas {
	return &canvas{img: c.img, clip: r.Intersect(c.clip), ox: float64(r.Min.X), oy: float64(r.Min.Y), m: c.m}
}

// paste copies src into c with src's corner at the canvas origin.
func (c *canvas) paste(src *image.RGBA) {
	r := src.Rect.Add(image.Pt(int(c.ox), int(c.oy)))
	draw.Draw(c.img, r.Intersect(c.clip), src, src.Rect.Min, draw.Src)
}

func (c *canvas) blend(x, y int, col rgba, cover float64) {
	a := col.a * cover
	if a <= 0 {
		return
	}
	i := c.img.PixOffset(x, y)
	p := c.img.Pix[i : i+3 : i+3]
	if a >= 1 {
		p[0], p[1], p[2] = uint8(col.r+0.5), uint8(col.g+0.5), uint8(col.b+0.5)
		return
	}
	p[0] = uint8(float64(p[0]) + (col.r-float64(p[0]))*a + 0.5)
	p[1] = uint8(float64(p[1]) + (col.g-float64(p[1]))*a + 0.5)
	p[2] = uint8(float64(p[2]) + (col.b-float64(p[2]))*a + 0.5)
}

// bounds is the clipped pixel box that covers x0..x1, y0..y1 in picture
// coordinates, grown by pad.
func (c *canvas) bounds(x0, y0, x1, y1, pad float64) image.Rectangle {
	return image.Rect(int(math.Floor(x0-pad)), int(math.Floor(y0-pad)), int(math.Ceil(x1+pad)), int(math.Ceil(y1+pad))).Intersect(c.clip)
}

// overlap is how much of the pixel starting at p lies between a0 and a1.
func overlap(a0, a1, p float64) float64 {
	return max(0, min(a1, p+1)-max(a0, p))
}

func clamp01(v float64) float64 {
	return min(1, max(0, v))
}

// fillRectI fills whole pixels; text and layout strips use it.
func (c *canvas) fillRectI(x, y, w, h int, col rgba) {
	r := image.Rect(x, y, x+w, y+h).Add(image.Pt(int(c.ox), int(c.oy))).Intersect(c.clip)
	for py := r.Min.Y; py < r.Max.Y; py++ {
		for px := r.Min.X; px < r.Max.X; px++ {
			c.blend(px, py, col, 1)
		}
	}
}

// fillRect fills a rectangle with fractional corners, so a unit's square
// moves smoothly between pixels.
func (c *canvas) fillRect(x0, y0, x1, y1 float64, col rgba) {
	x0, y0, x1, y1 = x0+c.ox, y0+c.oy, x1+c.ox, y1+c.oy
	b := c.bounds(x0, y0, x1, y1, 0)
	for y := b.Min.Y; y < b.Max.Y; y++ {
		cy := overlap(y0, y1, float64(y))
		for x := b.Min.X; x < b.Max.X; x++ {
			c.blend(x, y, col, cy*overlap(x0, x1, float64(x)))
		}
	}
}

// strokeRect draws the outline of a rectangle, the line of width w centred
// on its edges.
func (c *canvas) strokeRect(x0, y0, x1, y1, w float64, col rgba) {
	x0, y0, x1, y1 = x0+c.ox, y0+c.oy, x1+c.ox, y1+c.oy
	h := w / 2
	b := c.bounds(x0, y0, x1, y1, h)
	inner := x1-x0 > w && y1-y0 > w
	for y := b.Min.Y; y < b.Max.Y; y++ {
		fy, oy := float64(y), overlap(y0-h, y1+h, float64(y))
		iy := 0.0
		if inner {
			iy = overlap(y0+h, y1-h, fy)
		}
		for x := b.Min.X; x < b.Max.X; x++ {
			fx := float64(x)
			cov := oy * overlap(x0-h, x1+h, fx)
			if inner {
				cov -= iy * overlap(x0+h, x1-h, fx)
			}
			c.blend(x, y, col, cov)
		}
	}
}

// disc fills a circle of radius r.
func (c *canvas) disc(cx, cy, r float64, col rgba) {
	cx, cy = cx+c.ox, cy+c.oy
	b := c.bounds(cx-r, cy-r, cx+r, cy+r, 1)
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			d := math.Hypot(float64(x)+0.5-cx, float64(y)+0.5-cy)
			c.blend(x, y, col, clamp01(r+0.5-d))
		}
	}
}

// ring draws a circle of radius r with a line of width w.
func (c *canvas) ring(cx, cy, r, w float64, col rgba) {
	cx, cy = cx+c.ox, cy+c.oy
	b := c.bounds(cx-r, cy-r, cx+r, cy+r, w/2+1)
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			d := math.Hypot(float64(x)+0.5-cx, float64(y)+0.5-cy)
			c.blend(x, y, col, clamp01(w/2+0.5-math.Abs(d-r)))
		}
	}
}

// stroke draws the polyline through pts as one line of width w with round
// ends and joins. It gathers coverage first and paints once, so a
// translucent line does not darken where its segments overlap.
func (c *canvas) stroke(pts []pt, w float64, col rgba) {
	for i := range pts {
		j := min(i+1, len(pts)-1)
		if i > 0 && i == j {
			break
		}
		c.m.capsule(pts[i].x+c.ox, pts[i].y+c.oy, pts[j].x+c.ox, pts[j].y+c.oy, w/2, c.clip)
	}
	c.m.paint(c, col)
}

// dashed draws the polyline through pts as dashes of length on separated
// by gaps of length off.
func (c *canvas) dashed(pts []pt, w, on, off float64, col rgba) {
	phase := 0.0 // distance into the current dash-gap period
	for i := 0; i+1 < len(pts); i++ {
		a, b := pts[i], pts[i+1]
		l := math.Hypot(b.x-a.x, b.y-a.y)
		for s := 0.0; s < l; {
			var step float64
			if phase < on {
				step = min(on-phase, l-s)
				t0, t1 := s/l, (s+step)/l
				c.m.capsule(a.x+(b.x-a.x)*t0+c.ox, a.y+(b.y-a.y)*t0+c.oy, a.x+(b.x-a.x)*t1+c.ox, a.y+(b.y-a.y)*t1+c.oy, w/2, c.clip)
			} else {
				step = min(on+off-phase, l-s)
			}
			s += step
			phase = math.Mod(phase+step, on+off)
		}
	}
	c.m.paint(c, col)
}

// mask gathers stroke coverage for one picture.
type mask struct {
	r     image.Rectangle
	a     []float32
	dirty image.Rectangle
}

// capsule adds the coverage of a segment of radius rad, in picture
// coordinates.
func (m *mask) capsule(ax, ay, bx, by, rad float64, clip image.Rectangle) {
	b := image.Rect(int(math.Floor(min(ax, bx)-rad-1)), int(math.Floor(min(ay, by)-rad-1)),
		int(math.Ceil(max(ax, bx)+rad+1)), int(math.Ceil(max(ay, by)+rad+1))).Intersect(clip).Intersect(m.r)
	if b.Empty() {
		return
	}
	m.dirty = m.dirty.Union(b)
	dx, dy := bx-ax, by-ay
	l2 := dx*dx + dy*dy
	for y := b.Min.Y; y < b.Max.Y; y++ {
		py := float64(y) + 0.5
		row := (y - m.r.Min.Y) * m.r.Dx()
		for x := b.Min.X; x < b.Max.X; x++ {
			px := float64(x) + 0.5
			t := 0.0
			if l2 > 0 {
				t = clamp01(((px-ax)*dx + (py-ay)*dy) / l2)
			}
			cov := rad + 0.5 - math.Hypot(ax+t*dx-px, ay+t*dy-py)
			if cov <= 0 {
				continue
			}
			i := row + x - m.r.Min.X
			if v := float32(min(cov, 1)); v > m.a[i] {
				m.a[i] = v
			}
		}
	}
}

// paint composites the gathered coverage in col and clears it.
func (m *mask) paint(c *canvas, col rgba) {
	b := m.dirty
	for y := b.Min.Y; y < b.Max.Y; y++ {
		row := (y - m.r.Min.Y) * m.r.Dx()
		for x := b.Min.X; x < b.Max.X; x++ {
			i := row + x - m.r.Min.X
			if v := m.a[i]; v > 0 {
				c.blend(x, y, col, float64(v))
				m.a[i] = 0
			}
		}
	}
	m.dirty = image.Rectangle{}
}
