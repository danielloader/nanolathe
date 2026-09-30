package main

import (
	"image"
	"image/color"
	"math"
	"strings"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/nanolathe-gg/nanolathe/formats"
	"github.com/nanolathe-gg/nanolathe/internal/gui"
	"github.com/nanolathe-gg/nanolathe/internal/palette"
	"github.com/nanolathe-gg/nanolathe/internal/platform/screenkit"
	"github.com/nanolathe-gg/nanolathe/vfs"
)

// nlArt is the retail art the Nanolathe screen borrows: the NANOLATHE
// wordmark spliced from the stone title lettering the options, map-select and
// skirmish backdrops already carry, the map-select window's riveted frame,
// the backdrop texture, and unit pictures for the cards. Everything is read
// from the base install so a mod that repaints the front end cannot move the
// letters (docs/DESIGN_MODS_MUTATORS.md §8.2).
type nlArt struct {
	wordmark     *ebiten.Image // nil when the title backdrops are missing
	frame        *ebiten.Image // riveted well, nine-sliced at edge nlFrameEdge
	texture      *ebiten.Image
	pics         map[string]*ebiten.Image
	fs           vfs.FSOps
	buttons      *formats.GAFEntry
	buttonPal    *palette.Tables
	buttonImages map[*formats.GAFFrame]*ebiten.Image
}

const nlFrameEdge = 12

// nlTitleGlyph is one baked title letter: which backdrop holds it, its column
// span and the top row of its 25-row box.
type nlTitleGlyph struct {
	backdrop string
	x0, x1   int
	top      int
}

// The letters of NANOLATHE as they sit in the retail title bitmaps. Every
// title glyph is 25 rows tall.
var nlTitleGlyphs = map[rune]nlTitleGlyph{
	'N': {"options4x", 143, 156, 27},
	'A': {"optvisual4x", 355, 368, 27},
	'O': {"options4x", 63, 74, 27},
	'L': {"optvisual4x", 375, 384, 27},
	'T': {"options4x", 98, 108, 27},
	'H': {"skirmsetup4x", 164, 176, 18},
	'E': {"optinterface4x", 316, 326, 27},
}

func loadNLArt(base, content vfs.FSOps) *nlArt {
	art := &nlArt{pics: map[string]*ebiten.Image{}, fs: content, buttonImages: map[*formats.GAFFrame]*ebiten.Image{}}
	if base != nil {
		if g, err := formats.LoadGAFFile(base, "anims/commongui.gaf"); err == nil {
			art.buttons, _ = g.Find("BUTTONS0")
			art.buttonPal, _ = palette.Load(base)
		}
	}
	pcx := map[string]*formats.PCX{}
	load := func(name string) *formats.PCX {
		if p, ok := pcx[name]; ok {
			return p
		}
		p, err := formats.LoadPCXFile(base, "bitmaps/"+name+".pcx")
		if err != nil {
			p = nil
		}
		pcx[name] = p
		return p
	}
	if word := spliceTitle("NANOLATHE", load); word != nil {
		art.wordmark = ebiten.NewImageFromImage(word)
	}
	if p := load("dselectmap2"); p != nil {
		art.frame = ebiten.NewImageFromImage(pcxRegion(p, image.Rect(46, 75, 306, 280)))
	}
	if p := load("options4x"); p != nil {
		art.texture = ebiten.NewImageFromImage(mirrorTile(pcxRegion(p, image.Rect(260, 70, 420, 230))))
	}
	return art
}

// pic is a unit picture by name, cached; nil when the content has none.
func (a *nlArt) pic(names ...string) *ebiten.Image {
	for _, name := range names {
		key := strings.ToLower(name)
		if img, ok := a.pics[key]; ok {
			if img != nil {
				return img
			}
			continue
		}
		p, err := formats.LoadPCXFile(a.fs, "unitpics/"+key+".pcx")
		if err != nil {
			a.pics[key] = nil
			continue
		}
		img := ebiten.NewImageFromImage(pcxRegion(p, image.Rect(0, 0, int(p.Width), int(p.Height))))
		a.pics[key] = img
		return img
	}
	return nil
}

func pcxRegion(p *formats.PCX, r image.Rectangle) *image.RGBA {
	r = r.Intersect(image.Rect(0, 0, int(p.Width), int(p.Height)))
	out := image.NewRGBA(image.Rect(0, 0, r.Dx(), r.Dy()))
	for y := 0; y < r.Dy(); y++ {
		for x := 0; x < r.Dx(); x++ {
			c := p.Palette[p.Pixels[(r.Min.Y+y)*int(p.Width)+r.Min.X+x]]
			c.A = 255
			out.SetRGBA(x, y, c)
		}
	}
	return out
}

// mirrorTile makes a seamless 2×2 tile from one crop by mirroring it.
func mirrorTile(src *image.RGBA) *image.RGBA {
	w, h := src.Bounds().Dx(), src.Bounds().Dy()
	out := image.NewRGBA(image.Rect(0, 0, 2*w, 2*h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			c := src.RGBAAt(x, y)
			out.SetRGBA(x, y, c)
			out.SetRGBA(2*w-1-x, y, c)
			out.SetRGBA(x, 2*h-1-y, c)
			out.SetRGBA(2*w-1-x, 2*h-1-y, c)
		}
	}
	return out
}

// titleInk is the bright stone of the baked lettering.
func titleInk(c color.RGBA) bool {
	s := int(c.R) + int(c.G) + int(c.B)
	return (s > 330 && int(c.R)-int(c.B) > 15) || s > 560
}

// spliceTitle assembles a word from baked title glyphs on a transparent
// canvas: each letter's ink, grown by two pixels so its dark outline comes
// with it. A missing backdrop or letter returns nil.
func spliceTitle(word string, load func(string) *formats.PCX) *image.RGBA {
	const height, pad, gap = 25, 3, 6
	width := 0
	for _, r := range word {
		g, ok := nlTitleGlyphs[r]
		if !ok || load(g.backdrop) == nil {
			return nil
		}
		width += g.x1 - g.x0 + 1 + gap
	}
	out := image.NewRGBA(image.Rect(0, 0, width-gap+2*pad, height+2*pad))
	pen := pad
	for _, r := range word {
		g := nlTitleGlyphs[r]
		p := load(g.backdrop)
		at := func(x, y int) color.RGBA {
			if x < 0 || y < 0 || x >= int(p.Width) || y >= int(p.Height) {
				return color.RGBA{}
			}
			c := p.Palette[p.Pixels[y*int(p.Width)+x]]
			c.A = 255
			return c
		}
		for y := g.top - pad; y < g.top+height+pad; y++ {
			for x := g.x0 - pad; x <= g.x1+pad; x++ {
				ink := false
				for dy := -2; dy <= 2 && !ink; dy++ {
					for dx := -2; dx <= 2; dx++ {
						xx, yy := x+dx, y+dy
						if xx >= g.x0 && xx <= g.x1 && yy >= g.top && yy < g.top+height && titleInk(at(xx, yy)) {
							ink = true
							break
						}
					}
				}
				if ink {
					out.SetRGBA(pen+x-g.x0, pad+y-g.top, at(x, y))
				}
			}
		}
		pen += g.x1 - g.x0 + 1 + gap
	}
	return out
}

// nlGAFImage resolves indexed art through its physical palette, retaining the
// decoder's raw key and RLE transparency [fmt gaf][fmt pal].
func nlGAFImage(f *formats.GAFFrame, pal *palette.Tables) *image.RGBA {
	if f == nil || pal == nil || f.Width == 0 || f.Height == 0 {
		return nil
	}
	out := image.NewRGBA(image.Rect(0, 0, int(f.Width), int(f.Height)))
	for y := 0; y < int(f.Height); y++ {
		for x := 0; x < int(f.Width); x++ {
			if index, opaque := f.At(x, y); opaque {
				c := pal.Base[index]
				out.SetRGBA(x, y, color.RGBA{c[0], c[1], c[2], 255})
			}
		}
	}
	return out
}

const nlButtonEdge = 3

// nlButtonImage keeps the authored texture and press bevel, with a darker
// face for this screen's gunmetal palette. This is host styling, not retail
// palette conversion (docs/DESIGN_INTERFACE_HUD_INPUT.md §3.17). Cursors
// continue to use nlGAFImage without this adjustment.
func nlButtonImage(f *formats.GAFFrame, pal *palette.Tables) *image.RGBA {
	out := nlGAFImage(f, pal)
	if out == nil {
		return nil
	}
	b := out.Bounds()
	for y := 0; y < b.Dy(); y++ {
		for x := 0; x < b.Dx(); x++ {
			amount := uint16(65)
			if x < nlButtonEdge || x >= b.Dx()-nlButtonEdge || y < nlButtonEdge || y >= b.Dy()-nlButtonEdge {
				amount = 85
			}
			c := out.RGBAAt(x, y)
			c.R = uint8(uint16(c.R) * amount / 100)
			c.G = uint8(uint16(c.G) * amount / 100)
			c.B = uint8(uint16(c.B) * amount / 100)
			out.SetRGBA(x, y, c)
		}
	}
	return out
}

// buttonFrame uses the existing stock family resolver and up/down/grey
// frames [07 R-WGT-01 §3]. Geometry is a host adaptation: a uniform scale
// fits the native height, then tiled strips retain the original border.
func (a *nlArt) buttonFrame(r screenkit.Rect, down, disabled bool) *formats.GAFFrame {
	if a == nil || a.buttons == nil || a.buttonPal == nil || r.H <= 0 {
		return nil
	}
	// Only rectangular metal faces belong to these action/cap controls;
	// the small square stock family carries checkbox lamps. Read the native
	// face height and shortest width from the bank rather than stretching it.
	height, width := int32(0), int32(0)
	for i := 0; i < len(a.buttons.Frames); i += 4 {
		f := a.buttons.Frames[i].Frame
		if f == nil || f.Width <= f.Height {
			continue
		}
		if height == 0 || int32(f.Height) < height {
			height, width = int32(f.Height), int32(f.Width)
		}
		if int32(f.Height) == height {
			width = min(width, int32(f.Width))
		}
	}
	if height == 0 {
		return nil
	}
	gad := gui.Gadget{Rect: gui.Rect{W: max(width, int32(r.W*float64(height)/r.H)), H: height}}
	base := stockButtonBase(a.buttons, gad)
	f, _ := retailButtonFrameFromEntry(a.buttons, gad, base, boolInt(down), 0, disabled)
	return f
}

func (a *nlArt) drawButton(dst *ebiten.Image, r screenkit.Rect, down, disabled bool) bool {
	f := a.buttonFrame(r, down, disabled)
	if f == nil {
		return false
	}
	img := a.buttonImages[f]
	if img == nil {
		rgba := nlButtonImage(f, a.buttonPal)
		if rgba == nil {
			return false
		}
		img = ebiten.NewImageFromImage(rgba)
		a.buttonImages[f] = img
	}
	// Three native pixels include the two bevel runs and their inner seam.
	// Tile each face/edge in its long direction instead of stretching noise.
	const edge = nlButtonEdge
	b := img.Bounds()
	k := r.H / float64(b.Dy())
	e := min(float64(edge)*k, r.W/2)
	xs := [4]float64{r.X, r.X + e, r.X + r.W - e, r.X + r.W}
	ys := [4]float64{r.Y, r.Y + float64(edge)*k, r.Y + r.H - float64(edge)*k, r.Y + r.H}
	sx := [4]int{b.Min.X, b.Min.X + edge, b.Max.X - edge, b.Max.X}
	sy := [4]int{b.Min.Y, b.Min.Y + edge, b.Max.Y - edge, b.Max.Y}
	for y := 0; y < 3; y++ {
		for x := 0; x < 3; x++ {
			src := img.SubImage(image.Rect(sx[x], sy[y], sx[x+1], sy[y+1])).(*ebiten.Image)
			nlTileArt(dst, src, screenkit.Rect{X: xs[x], Y: ys[y], W: xs[x+1] - xs[x], H: ys[y+1] - ys[y]}, k)
		}
	}
	return true
}

// nlTileArt clips the last repeat to its destination, preserving pixel scale.
func nlTileArt(dst, src *ebiten.Image, r screenkit.Rect, k float64) {
	if r.W <= 0 || r.H <= 0 || k <= 0 {
		return
	}
	clip := image.Rect(int(math.Round(r.X)), int(math.Round(r.Y)), int(math.Round(r.X+r.W)), int(math.Round(r.Y+r.H))).Intersect(dst.Bounds())
	if clip.Empty() {
		return
	}
	target := dst.SubImage(clip).(*ebiten.Image)
	w, h := float64(src.Bounds().Dx())*k, float64(src.Bounds().Dy())*k
	for y := r.Y; y < r.Y+r.H-0.01; y += h {
		for x := r.X; x < r.X+r.W-0.01; x += w {
			screenkit.Image(target, src, screenkit.Rect{X: x, Y: y, W: w, H: h}, 1, true)
		}
	}
}
