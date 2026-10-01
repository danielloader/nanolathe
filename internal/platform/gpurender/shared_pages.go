package gpurender

import "github.com/hajimehoshi/ebiten/v2"

// SharedPages is device memory renderers that execute one after another on
// one goroutine may draw through instead of each holding their own: the model
// lane's page planes, which every Execute clears where it draws and finishes
// reading before it returns, and the pool of recycled source pages, which
// hands a page to one owner at a time. Ebitengine keeps the device commands
// in submission order, so the next renderer's clears follow the last one's
// reads. The settings screen's preview renderers share one, so a renderer
// made for a new card size or the first compare does not allocate (and on
// unified memory wire) two 64 MiB planes inside its first frame, and the
// compare twin packs into pages the primary retired (DESIGN_GPU_RENDERER §2.3
// "Source lifetime"). Renderers that may execute concurrently must not share
// one.
type SharedPages struct {
	pool  pagePool
	model [modelDirectMaxPages]modelPagePlanes
}

// modelPagePlanes is one shared model page's two planes, allocated by the
// first renderer to need the page.
type modelPagePlanes struct {
	key, colour *ebiten.Image
}

// NewSharedPages returns an empty set; nothing is allocated until a renderer
// draws through it.
func NewSharedPages() *SharedPages { return &SharedPages{} }

// SharePages makes r draw its model pages and recycle its source pages
// through s. It is called before r's first Execute: a page r already holds
// stays its own.
func (r *Renderer) SharePages(s *SharedPages) {
	if r == nil || s == nil {
		return
	}
	r.pages = &s.pool
	r.scene.pool = r.pages
	r.modelDirect.shared = s
}

// pool is the renderer's page pool: its own, or the set it shares.
func (r *Renderer) pool() *pagePool {
	if r.pages == nil {
		r.pages = &pagePool{}
	}
	return r.pages
}
