package main

import (
	"image/color"
	"testing"
)

// Opaque pixels never take the colour key, which retail GAF art treats as
// transparent; a fully transparent pixel always does.
func TestDitherAvoidsColourKey(t *testing.T) {
	pal := make(color.Palette, 256)
	for i := range pal {
		pal[i] = color.NRGBA{uint8(i), uint8(i), uint8(i), 255}
	}
	l := NewLayer(16, 4)
	for y := 0; y < 3; y++ {
		for x := 0; x < 16; x++ {
			l.Set(x, y, RGBA{9.0 / 255, 9.0 / 255, 9.0 / 255, 1})
		}
	}
	img := Dither(l, pal)
	for y := 0; y < 3; y++ {
		for x := 0; x < 16; x++ {
			if img.ColorIndexAt(x, y) == colourKey {
				t.Fatalf("opaque pixel (%d,%d) took the colour key", x, y)
			}
		}
	}
	if img.ColorIndexAt(0, 3) != colourKey {
		t.Fatalf("transparent pixel took index %d, want the colour key", img.ColorIndexAt(0, 3))
	}
}

// Wear is seeded by the label: the same label wears identically, different
// labels differently.
func TestWearIsSeededByLabel(t *testing.T) {
	s := &Style{Scale: 2, Light: Light{-1, 1}}
	draw := func(seed string) *Layer {
		l := NewLayer(108, 60)
		l.Rect(0, 0, 107, 59, RGBA{0.5, 0.5, 0.5, 1})
		s.Wear(l, seed, l.W)
		return l
	}
	same := func(a, b *Layer) bool {
		for i := range a.Pix {
			if a.Pix[i] != b.Pix[i] {
				return false
			}
		}
		return true
	}
	if !same(draw("MOVE"), draw("MOVE")) {
		t.Fatal("the same label wore differently")
	}
	if same(draw("MOVE"), draw("STOP")) {
		t.Fatal("different labels wore identically")
	}
}
