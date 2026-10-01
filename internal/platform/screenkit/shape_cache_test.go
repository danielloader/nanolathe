package screenkit

import (
	"image"
	"image/color"
	"math"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

func testShape(x float32) shapeSpec {
	return shapeSpec{kind: shapeDisc, geometry: [5]float32{x, 20.375, 3}, colour: color.RGBA{61, 255, 92, 255}}
}

func TestShapeKeyRetainsCoverageAndColour(t *testing.T) {
	s := testShape(20.125)
	for _, change := range []func(*shapeSpec){
		func(s *shapeSpec) { s.geometry[0] = math.Nextafter32(s.geometry[0], 30) },
		func(s *shapeSpec) { s.geometry[1] += 0.25 },
		func(s *shapeSpec) { s.geometry[2] = math.Nextafter32(s.geometry[2], 4) },
		func(s *shapeSpec) { s.colour.A-- },
		func(s *shapeSpec) { s.colour.G-- },
		func(s *shapeSpec) { s.kind = shapeRing; s.geometry[3] = 1 },
	} {
		changed := s
		change(&changed)
		if changed.key() == s.key() {
			t.Fatal("changed float32 coverage or colour reused the same stamp")
		}
	}
	b, ok := s.bounds(shapeCacheSide)
	if !ok || b != image.Rect(15, 15, 26, 26) {
		t.Fatalf("fractional shape margin: %v, admitted %t", b, ok)
	}
	for _, bad := range []shapeSpec{
		{kind: shapeDisc, geometry: [5]float32{0, 0, 0}},
		{kind: shapeRing, geometry: [5]float32{0, 0, 2, -1}},
		{kind: shapeLine, geometry: [5]float32{0, 0, 1, 1, 0}},
		{kind: shapeDisc, geometry: [5]float32{0, 0, shapeCacheSide}},
		{kind: shapeDisc, geometry: [5]float32{float32(math.Inf(1)), 0, 2}},
		{kind: shapeDisc, geometry: [5]float32{0, float32(math.NaN()), 2}},
	} {
		if _, ok := bad.bounds(shapeCacheSide); ok {
			t.Fatalf("unsupported shape admitted: %+v", bad)
		}
	}
}

func TestShapeStampsAdmitRepeatsAndBoundHistory(t *testing.T) {
	builds, releases := 0, 0
	c := newShapeStampCache(3, 1<<20, shapeCacheSide,
		func(shapeSpec, image.Rectangle) *ebiten.Image { builds++; return &ebiten.Image{} },
		func(*ebiten.Image) { releases++ })
	for i := 0; i < 100; i++ {
		if img, _ := c.stamp(testShape(float32(i))); img != nil {
			t.Fatal("one-use animation geometry allocated a stamp")
		}
		if len(c.entries) > 3 || c.bytes != 0 {
			t.Fatal("one-use key history exceeded its entry or byte budget")
		}
	}
	a := testShape(99)
	img, _ := c.stamp(a)
	if img == nil || builds != 1 {
		t.Fatal("repeated geometry did not acquire a stamp")
	}
	if again, _ := c.stamp(a); again != img || builds != 1 {
		t.Fatal("retained stamp was rebuilt")
	}
	for _, x := range []float32{100, 101, 102} {
		c.stamp(testShape(x))
	}
	if releases != 1 || c.bytes != 0 || len(c.entries) != 3 {
		t.Fatalf("retirement did not release the least recent stamp: released %d, bytes %d, entries %d", releases, c.bytes, len(c.entries))
	}
}

func TestShapeStampsEvictForBytesAndRespectRecentUse(t *testing.T) {
	a, b, d := testShape(20.125), testShape(40.125), testShape(60.125)
	bounds, _ := a.bounds(shapeCacheSide)
	cost := bounds.Dx() * bounds.Dy() * 4
	var retired []*ebiten.Image
	c := newShapeStampCache(8, 2*cost, shapeCacheSide,
		func(shapeSpec, image.Rectangle) *ebiten.Image { return &ebiten.Image{} },
		func(img *ebiten.Image) { retired = append(retired, img) })
	c.stamp(a)
	aImage, _ := c.stamp(a)
	c.stamp(b)
	bImage, _ := c.stamp(b)
	c.stamp(a) // a must survive the next byte-budget eviction.
	c.stamp(d)
	dImage, _ := c.stamp(d)
	if dImage == nil || c.bytes != 2*cost || len(retired) != 1 || retired[0] != bImage {
		t.Fatalf("byte-budget eviction: bytes %d, retired %v", c.bytes, retired)
	}
	if retained, _ := c.stamp(a); retained != aImage {
		t.Fatal("recently used stamp was evicted instead of the older one")
	}
	tooSmall := newShapeStampCache(8, cost-1, shapeCacheSide,
		func(shapeSpec, image.Rectangle) *ebiten.Image { t.Fatal("over-budget stamp built"); return nil },
		func(*ebiten.Image) { t.Fatal("over-budget stamp retired") })
	tooSmall.stamp(a)
	tooSmall.stamp(a)
	if len(tooSmall.entries) != 0 || tooSmall.bytes != 0 {
		t.Fatal("an individually over-budget stamp entered the history")
	}
}
