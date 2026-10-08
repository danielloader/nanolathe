package main

import (
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"

	"github.com/nanolathe-gg/nanolathe/vfs"
)

// colourKey is the palette index retail GAF art reserves as transparent; opaque
// pixels never take it.
const colourKey = 9

// LoadInstallPalette reads palettes/palette.pal (256 x {r, g, b, 0}) from a
// Total Annihilation install through the engine's VFS, so no retail data is
// copied out [fmt pal].
func LoadInstallPalette(root string) (color.Palette, error) {
	fs := vfs.New()
	defer fs.Close()
	if err := fs.MountGameDirectory(root); err != nil {
		return nil, err
	}
	b, err := fs.ReadFile("palettes/palette.pal")
	if err != nil {
		return nil, err
	}
	if len(b) != 1024 {
		return nil, fmt.Errorf("palettes/palette.pal is %d bytes, want 1024", len(b))
	}
	pal := make(color.Palette, 256)
	for i := range pal {
		pal[i] = color.NRGBA{b[i*4], b[i*4+1], b[i*4+2], 255}
	}
	return pal, nil
}

// LoadPalette reads a 256-entry palette from a 16x16 swatch image.
func LoadPalette(path string) (color.Palette, error) {
	l, err := LoadLayer(path)
	if err != nil {
		return nil, err
	}
	pal := make(color.Palette, 0, 256)
	for i := range 256 {
		c := l.At(i%16, i/16)
		pal = append(pal, color.NRGBA{uint8(math.Round(c.R * 255)), uint8(math.Round(c.G * 255)), uint8(math.Round(c.B * 255)), 255})
	}
	return pal, nil
}

// Dither reduces the layer to the palette with Floyd-Steinberg error
// diffusion. Pixels below half alpha become the colour key.
func Dither(l *Layer, pal color.Palette) *image.Paletted {
	out := image.NewPaletted(image.Rect(0, 0, l.W, l.H), pal)
	type rgb struct{ r, g, b float64 }
	cols := make([]rgb, len(pal))
	for i, c := range pal {
		r, g, b, _ := c.RGBA()
		cols[i] = rgb{float64(r) / 65535, float64(g) / 65535, float64(b) / 65535}
	}
	work := make([]rgb, l.W*l.H)
	for i := range work {
		work[i] = rgb{l.Pix[i*4], l.Pix[i*4+1], l.Pix[i*4+2]}
	}
	spread := func(x, y int, e rgb, k float64) {
		if x < 0 || y < 0 || x >= l.W || y >= l.H {
			return
		}
		w := &work[y*l.W+x]
		w.r += e.r * k
		w.g += e.g * k
		w.b += e.b * k
	}
	for y := 0; y < l.H; y++ {
		for x := 0; x < l.W; x++ {
			if l.At(x, y).A < 0.5 {
				out.SetColorIndex(x, y, colourKey)
				continue
			}
			c := work[y*l.W+x]
			best, bestD := 0, math.MaxFloat64
			for i, p := range cols {
				if i == colourKey {
					continue
				}
				d := (c.r-p.r)*(c.r-p.r) + (c.g-p.g)*(c.g-p.g) + (c.b-p.b)*(c.b-p.b)
				if d < bestD {
					best, bestD = i, d
				}
			}
			out.SetColorIndex(x, y, uint8(best))
			p := cols[best]
			e := rgb{c.r - p.r, c.g - p.g, c.b - p.b}
			spread(x+1, y, e, 7.0/16)
			spread(x-1, y+1, e, 3.0/16)
			spread(x, y+1, e, 5.0/16)
			spread(x+1, y+1, e, 1.0/16)
		}
	}
	return out
}

func savePaletted(path string, img *image.Paletted) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return png.Encode(f, img)
}
