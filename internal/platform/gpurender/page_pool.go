package gpurender

import (
	"image"
	"slices"

	"github.com/hajimehoshi/ebiten/v2"
)

// pagePool keeps the fixed-size source pages ResetSources retires — the scene
// atlas's shared pages, the model texture page and the pages of a terrain
// atlas filled on demand — and hands them back cleared, so the next battle or
// settings preview packs into device memory the renderer already holds
// instead of allocating fresh pages inside its first frame. On unified memory
// every fresh texture is wired before the frame that first uses it can start
// (DESIGN_GPU_RENDERER §2.3 "Source lifetime").
type pagePool struct {
	free  []*ebiten.Image
	sizes []image.Point
	bytes int
}

// recycledPageBytes bounds the pages a pool keeps: eight 2048² pages.
const recycledPageBytes = 128 << 20

// take returns a cleared page of exactly w×h, recycled when one is kept.
func (p *pagePool) take(w, h int) *ebiten.Image {
	for i, s := range p.sizes {
		if s.X == w && s.Y == h {
			img := p.free[i]
			p.free = slices.Delete(p.free, i, i+1)
			p.sizes = slices.Delete(p.sizes, i, i+1)
			p.bytes -= w * h * 4
			img.Clear()
			return img
		}
	}
	return newRendererImage(w, h)
}

// keep holds a retired w×h page for take, reporting false when the pool is
// full and the caller must release it instead.
func (p *pagePool) keep(img *ebiten.Image, w, h int) bool {
	if p.bytes+w*h*4 > recycledPageBytes {
		return false
	}
	p.free = append(p.free, img)
	p.sizes = append(p.sizes, image.Pt(w, h))
	p.bytes += w * h * 4
	return true
}
