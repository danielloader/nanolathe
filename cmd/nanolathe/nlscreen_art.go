package main

import (
	"image"
	"image/color"
	"strings"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/nanolathe-gg/nanolathe/formats"
	"github.com/nanolathe-gg/nanolathe/vfs"
)

// nlArt is the retail art the Nanolathe screen borrows: the NANOLATHE
// wordmark spliced from the stone title lettering the options, map-select and
// skirmish backdrops already carry, the map-select window's riveted frame,
// the backdrop texture, and unit pictures for the cards. Everything is read
// from the base install so a mod that repaints the front end cannot move the
// letters (docs/DESIGN_MODS_MUTATORS.md §8.2).
type nlArt struct {
	wordmark *ebiten.Image // nil when the title backdrops are missing
	frame    *ebiten.Image // riveted well, nine-sliced at edge nlFrameEdge
	texture  *ebiten.Image
	pics     map[string]*ebiten.Image
	fs       vfs.FSOps
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
	art := &nlArt{pics: map[string]*ebiten.Image{}, fs: content}
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
