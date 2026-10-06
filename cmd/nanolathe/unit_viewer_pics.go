package main

import (
	"strings"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/nanolathe-gg/nanolathe/internal/content"
	"github.com/nanolathe-gg/nanolathe/vfs"
)

// unitViewerPictures borrows the settings screen's asynchronous picture
// loader: a worker decodes `unitpics/<unit name>.pcx` from the running content
// (decodeNLPic) and the game goroutine only uploads decoded pixels during Draw
// (DESIGN_INTERFACE_HUD_INPUT §3.17). Only visible rows request pictures. The
// worker reads the content's archives, so release halts and joins it before
// the screen lets the host unmount that content.
type unitViewerPictures struct {
	loader  *nlPictures
	images  map[string]*ebiten.Image // nil value: the content has no picture
	pending int                      // requests this frame that are still decoding
}

func (p *unitViewerPictures) bind(fs vfs.FSOps) {
	if p.loader != nil || fs == nil {
		return
	}
	p.loader = newNLPictures(fs, nil, nil)
	// The viewer asks for unit names directly. An empty, already-resolved
	// plan skips the settings screen's card and sidebar resolution.
	p.loader.names = &nlPicNames{}
	p.images = map[string]*ebiten.Image{}
}

// image returns the definition's picture, or nil while it decodes or when the
// content has none. The caller draws a neutral placeholder for nil.
func (p *unitViewerPictures) image(def *content.UnitDef) *ebiten.Image {
	if p.loader == nil || def == nil || strings.TrimSpace(def.UnitName) == "" {
		return nil
	}
	key := strings.ToLower(strings.TrimSpace(def.UnitName))
	if img, ok := p.images[key]; ok {
		return img
	}
	px, known := p.loader.take(key)
	if !known {
		p.loader.want(key)
		p.pending++
		return nil
	}
	var img *ebiten.Image
	if px != nil {
		img = ebiten.NewImageFromImage(px)
	}
	p.images[key] = img
	return img
}

// endFrame starts the worker for this frame's requests.
func (p *unitViewerPictures) endFrame() int {
	pending := p.pending
	p.pending = 0
	if pending > 0 {
		p.loader.resume()
	}
	return pending
}

func (p *unitViewerPictures) release() {
	if p.loader != nil {
		p.loader.stop()
	}
	for _, img := range p.images {
		if img != nil {
			img.Deallocate()
		}
	}
	*p = unitViewerPictures{}
}
