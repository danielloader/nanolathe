package screenkit

import (
	"container/list"
	"image"
	"image/color"
	"math"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"
)

// The settings screen repeats small lamps at the host cadence. Retain their
// original vector rasterization rather than rebuilding its antialiased stencil
// passes each draw (DESIGN_INTERFACE_HUD_INPUT §3.17). All cache access and image
// construction belongs to the game goroutine, like the screen's font uploads.
// The RGBA intermediate adds blend rounding: the native coverage fixture allows
// at most one 8-bit channel level while retaining geometry and opacity.
const (
	shapeCacheEntries = 256
	shapeCacheBytes   = 4 << 20
	shapeCacheSide    = 128
)

type shapeKind uint8

const (
	shapeDisc shapeKind = iota
	shapeRing
	shapeLine
)

type shapeSpec struct {
	kind     shapeKind
	geometry [5]float32
	colour   color.RGBA
}

type shapeKey struct {
	kind     shapeKind
	geometry [5]uint32
	colour   color.RGBA
}

func (s shapeSpec) key() shapeKey {
	k := shapeKey{kind: s.kind, colour: s.colour}
	for i, v := range s.geometry {
		k.geometry[i] = math.Float32bits(v)
	}
	return k
}

// Keep the absolute float32 geometry in both the key and the stamp's bounds.
// Rebasing a path or rounding its fractional position changes edge coverage.
// Two pixels of transparent margin contain the vector antialiasing samples.
func (s shapeSpec) bounds(maxSide int) (image.Rectangle, bool) {
	for _, v := range s.geometry {
		if math.IsNaN(float64(v)) || math.IsInf(float64(v), 0) || math.Abs(float64(v)) > 1<<20 {
			return image.Rectangle{}, false
		}
	}
	v := s.geometry
	x0, y0, x1, y1 := float64(v[0]), float64(v[1]), float64(v[0]), float64(v[1])
	var reach float64
	switch s.kind {
	case shapeDisc:
		if v[2] <= 0 {
			return image.Rectangle{}, false
		}
		reach = float64(v[2])
	case shapeRing:
		if v[2] <= 0 || v[3] <= 0 {
			return image.Rectangle{}, false
		}
		reach = float64(v[2]) + float64(v[3])/2
	case shapeLine:
		if v[4] <= 0 {
			return image.Rectangle{}, false
		}
		x0, x1 = min(float64(v[0]), float64(v[2])), max(float64(v[0]), float64(v[2]))
		y0, y1 = min(float64(v[1]), float64(v[3])), max(float64(v[1]), float64(v[3]))
		reach = float64(v[4]) / 2
	default:
		return image.Rectangle{}, false
	}
	b := image.Rect(int(math.Floor(x0-reach))-2, int(math.Floor(y0-reach))-2,
		int(math.Ceil(x1+reach))+2, int(math.Ceil(y1+reach))+2)
	return b, !b.Empty() && b.Dx() <= maxSide && b.Dy() <= maxSide
}

type shapeStamp struct {
	key   shapeKey
	image *ebiten.Image
	bytes int
}

type shapeStampCache struct {
	entries                       map[shapeKey]*list.Element
	recent                        list.List // most recently used first, including one-use candidates
	bytes                         int
	maxEntries, maxBytes, maxSide int
	build                         func(shapeSpec, image.Rectangle) *ebiten.Image
	release                       func(*ebiten.Image)
}

func newShapeStampCache(entries, bytes, side int, build func(shapeSpec, image.Rectangle) *ebiten.Image, release func(*ebiten.Image)) *shapeStampCache {
	return &shapeStampCache{entries: make(map[shapeKey]*list.Element), maxEntries: entries, maxBytes: bytes, maxSide: side, build: build, release: release}
}

// A shape must recur before it gets an image. Opening animations and dragging a
// window therefore leave only bounded key history instead of cold GPU stamps.
func (c *shapeStampCache) stamp(s shapeSpec) (*ebiten.Image, image.Rectangle) {
	b, ok := s.bounds(c.maxSide)
	cost := b.Dx() * b.Dy() * 4
	if !ok || c.maxEntries <= 0 || cost > c.maxBytes {
		return nil, b
	}
	k := s.key()
	if elem := c.entries[k]; elem != nil {
		c.recent.MoveToFront(elem)
		e := elem.Value.(*shapeStamp)
		if e.image == nil {
			for c.bytes+cost > c.maxBytes {
				c.evict()
			}
			e.image = c.build(s, b)
			if e.image != nil {
				e.bytes = cost
				c.bytes += cost
			}
		}
		return e.image, b
	}
	for len(c.entries) >= c.maxEntries {
		c.evict()
	}
	c.entries[k] = c.recent.PushFront(&shapeStamp{key: k})
	return nil, b
}

func (c *shapeStampCache) evict() {
	elem := c.recent.Back()
	e := elem.Value.(*shapeStamp)
	delete(c.entries, e.key)
	c.recent.Remove(elem)
	c.bytes -= e.bytes
	if e.image != nil {
		c.release(e.image)
	}
}

var smallShapes = newShapeStampCache(shapeCacheEntries, shapeCacheBytes, shapeCacheSide,
	func(s shapeSpec, b image.Rectangle) *ebiten.Image {
		img := ebiten.NewImageWithOptions(b, nil)
		drawShapeDirect(img, s)
		return img
	}, func(img *ebiten.Image) { img.Deallocate() })

func drawSmallShape(dst *ebiten.Image, s shapeSpec) {
	img, b := smallShapes.stamp(s)
	if img == nil {
		drawShapeDirect(dst, s)
		return
	}
	// Integer placement keeps the cached samples on their original pixel grid.
	// DrawImage retains the destination's own (possibly sub-image) clip.
	op := &ebiten.DrawImageOptions{Filter: ebiten.FilterNearest}
	op.GeoM.Translate(float64(b.Min.X), float64(b.Min.Y))
	dst.DrawImage(img, op)
}

func drawShapeDirect(dst *ebiten.Image, s shapeSpec) {
	v := s.geometry
	switch s.kind {
	case shapeDisc:
		vector.FillCircle(dst, v[0], v[1], v[2], s.colour, true)
	case shapeRing:
		vector.StrokeCircle(dst, v[0], v[1], v[2], v[3], s.colour, true)
	case shapeLine:
		vector.StrokeLine(dst, v[0], v[1], v[2], v[3], v[4], s.colour, true)
	}
}
