package gpurender

import (
	"image"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/nanolathe-gg/nanolathe/formats"
)

// modelTextureAtlas packs every resolved 3DO texture frame into shared pages, so
// one batched body pass can carry faces of many subjects and many textures
// (C-G9). Frames are packed once per identity and reused until ResetSources
// retires the terrain generation. A frame's texels are written straight into
// its page region: a standalone texture per frame, copied into the page, cost
// a settings preview's first frame a hundred or more device allocations.
type modelTextureSlot struct {
	img        *ebiten.Image
	x, y, w, h int
}

// modelTexturePageSize is the side of one model texture page.
const modelTexturePageSize = 2048

type modelTextureAtlas struct {
	slots map[*formats.GAFFrame]modelTextureSlot
	page  *ebiten.Image
	// pages is every page filled, the open one last, so ResetSources can
	// return them all to the pool.
	pages     []*ebiten.Image
	x, y, row int
	scratch   []byte
}

func (r *Renderer) modelTextureFor(f *formats.GAFFrame) modelTextureSlot {
	if f == nil {
		return modelTextureSlot{}
	}
	a := &r.textureAtlas
	if s, ok := a.slots[f]; ok {
		return s
	}
	if a.slots == nil {
		a.slots = make(map[*formats.GAFFrame]modelTextureSlot)
	}
	w, h := int(f.Width), int(f.Height)
	if w <= 0 || h <= 0 {
		return modelTextureSlot{}
	}
	if w > modelTexturePageSize-2 || h > modelTexturePageSize-2 {
		s := modelTextureSlot{img: r.gafImageFor(f), w: w, h: h}
		a.slots[f] = s
		return s
	}
	if a.x+w+2 > modelTexturePageSize {
		a.x = 0
		a.y += a.row
		a.row = 0
	}
	if a.page == nil || a.y+h+2 > modelTexturePageSize {
		a.page = r.pool().take(modelTexturePageSize, modelTexturePageSize)
		a.pages = append(a.pages, a.page)
		a.x = 0
		a.y = 0
		a.row = 0
	}
	s := modelTextureSlot{a.page, a.x + 1, a.y + 1, w, h}
	// The page's border texels stay as the cleared page left them, as they did
	// when the frame was drawn in with a copy blend.
	a.scratch = gafFrameIndexPixels(a.scratch, f)
	sub := a.page.RecyclableSubImage(image.Rect(s.x, s.y, s.x+w, s.y+h))
	sub.WritePixels(a.scratch)
	sub.Recycle()
	a.x += w + 2
	if h+2 > a.row {
		a.row = h + 2
	}
	a.slots[f] = s
	return s
}
