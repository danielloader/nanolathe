package main

import (
	"image"
	"strings"
	"sync"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/nanolathe-gg/nanolathe/formats"
	"github.com/nanolathe-gg/nanolathe/internal/content"
	"github.com/nanolathe-gg/nanolathe/vfs"
)

// The unit pictures the Nanolathe screen shows — card portraits, the mutator
// plates' examples and the sidebar demonstration's build products — are
// resolved against the running content's catalog and decoded from its
// archives by a worker goroutine (DESIGN_INTERFACE_HUD_INPUT §3.17). The game
// goroutine only turns decoded pixels into images. A page switch can show
// twenty pictures at once, and reading them inside Draw stalled that frame.
//
// A picture not decoded yet draws nothing for that frame, as before the
// content's catalog has compiled: the game goroutine never waits on the
// worker, and decoding there instead would bring back the stall. The worker
// starts as soon as the catalog has compiled — while the main menu idles,
// before the screen opens — and finishes within a fraction of a second.
//
// The worker reads the content's archives, which a content reload closes. It
// stops, and the game goroutine waits for the file in hand, wherever a reload
// can follow: a content switch from this screen, closing the screen, leaving
// the main menu and any window opened over it. It resumes when the menu idles
// or the screen draws again.

// nlPicNames is what the screen draws for the running content: each card's
// portrait names (first that exists wins), each mutator's example units, and
// the sidebar demonstration's products.
type nlPicNames struct {
	cards   map[string][]string
	stats   map[string][]string
	sidebar *sidebarProductCatalog
}

// nlPicRequest is one card's portrait preferences, copied from the catalogue.
type nlPicRequest struct {
	key  string
	pics []string
}

// resolveNLPicNames resolves every picture the screen can show through the
// same content roster the scenes use. halted is polled between names; a
// halted resolution returns nil.
func resolveNLPicNames(fs vfs.FSOps, cat *content.Catalog, cards []nlPicRequest, halted func() bool) *nlPicNames {
	st := &nlStats{base: cat}
	names := &nlPicNames{cards: map[string][]string{}, stats: map[string][]string{}}
	for _, info := range content.MutatorCatalog() {
		if stat, ok := nlMutatorStats[info.Key]; ok {
			if halted() {
				return nil
			}
			names.stats[info.Key] = st.statUnitNames(stat)
		}
	}
	r := st.contentRoster()
	for _, c := range cards {
		if halted() {
			return nil
		}
		// A mutator card shows the units its plate lists.
		if key, ok := strings.CutPrefix(c.key, "mut-"); ok {
			if list, ok := names.stats[key]; ok {
				names.cards[c.key] = list
				continue
			}
		}
		var list []string
		for _, preferred := range c.pics {
			if name := r.resolve(preferred); name != "" {
				list = append(list, name)
			}
		}
		names.cards[c.key] = list
	}
	if builder, ok := r.cat.Unit(r.resolve("armck")); ok {
		if halted() {
			return nil
		}
		// As in battle, the viewing player's side owns the HUD; a reachable
		// constructor can author a different unit-side tag [07 §6].
		for _, side := range cat.Sides {
			if side != nil && strings.EqualFold(side.Name, r.factions[0]) {
				names.sidebar = loadNLSidebar(fs, cat, builder, side, halted)
				break
			}
		}
		if halted() {
			return nil
		}
	}
	return names
}

// order lists every name once, cards first in catalogue order.
func (n *nlPicNames) order(cards []nlPicRequest) []string {
	var out []string
	seen := map[string]bool{}
	add := func(list []string) {
		for _, name := range list {
			if key := strings.ToLower(name); !seen[key] {
				seen[key] = true
				out = append(out, key)
			}
		}
	}
	for _, c := range cards {
		add(n.cards[c.key])
	}
	if n.sidebar != nil {
		for _, cell := range n.sidebar.cells {
			for _, product := range cell.products {
				add([]string{product.source.window.Gadgets[product.source.index].Name})
			}
		}
	}
	return out
}

// decodeNLPic reads one unit picture; nil when the content has none.
func decodeNLPic(fs vfs.FSOps, name string) *image.RGBA {
	p, err := formats.LoadPCXFile(fs, "unitpics/"+name+".pcx")
	if err != nil {
		return nil
	}
	return pcxRegion(p, image.Rect(0, 0, int(p.Width), int(p.Height)))
}

// nlPictures is one content's picture loader.
type nlPictures struct {
	fs    vfs.FSOps
	cat   *content.Catalog
	cards []nlPicRequest

	mu      sync.Mutex
	names   *nlPicNames
	decoded map[string]*image.RGBA // a nil value: the content has no such picture
	wanted  map[string]bool        // queued or decoded
	queue   []string
	running bool // a worker goroutine is live
	halt    bool // the worker returns before its next name or file
	wg      sync.WaitGroup
}

func newNLPictures(fs vfs.FSOps, cat *content.Catalog, cards []nlPicRequest) *nlPictures {
	return &nlPictures{fs: fs, cat: cat, cards: cards, decoded: map[string]*image.RGBA{}, wanted: map[string]bool{}}
}

// resume starts a worker when there is work and none is running.
func (p *nlPictures) resume() {
	if p == nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.running || (p.names != nil && len(p.queue) == 0) {
		return
	}
	p.running = true
	p.wg.Add(1)
	go p.run()
}

// stop halts the worker and waits for the name or file in hand; resume
// continues where it stopped.
func (p *nlPictures) stop() {
	if p == nil {
		return
	}
	p.mu.Lock()
	p.halt = true
	p.mu.Unlock()
	p.wg.Wait()
	p.mu.Lock()
	p.halt = false
	p.mu.Unlock()
}

func (p *nlPictures) halted() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.halt
}

func (p *nlPictures) run() {
	defer p.wg.Done()
	p.mu.Lock()
	resolved := p.names != nil
	p.mu.Unlock()
	if !resolved {
		// Only this goroutine resolves, and stop waits for it, so the names
		// are never resolved twice at once.
		names := resolveNLPicNames(p.fs, p.cat, p.cards, p.halted)
		p.mu.Lock()
		if names != nil {
			p.names = names
			var first []string
			for _, name := range names.order(p.cards) {
				if !p.wanted[name] {
					p.wanted[name] = true
					first = append(first, name)
				}
			}
			p.queue = append(first, p.queue...)
		}
		p.mu.Unlock()
	}
	for {
		p.mu.Lock()
		if p.halt || len(p.queue) == 0 {
			p.running = false
			p.mu.Unlock()
			return
		}
		name := p.queue[0]
		p.queue = p.queue[1:]
		p.mu.Unlock()
		img := decodeNLPic(p.fs, name)
		p.mu.Lock()
		p.decoded[name] = img
		p.mu.Unlock()
	}
}

// resolvedNames is the name table once the worker has resolved it.
func (p *nlPictures) resolvedNames() *nlPicNames {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.names
}

// take hands over a decoded picture: known reports whether the worker has
// read the name, and img is nil when the content has no such picture.
func (p *nlPictures) take(name string) (img *image.RGBA, known bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	img, known = p.decoded[name]
	if known {
		delete(p.decoded, name)
	}
	return img, known
}

// want queues a name the plan did not include; the next resume reads it.
func (p *nlPictures) want(name string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.wanted[name] {
		p.wanted[name] = true
		p.queue = append(p.queue, name)
	}
}

// bindContent points the pictures at a content set. A different content
// stops the old loader, waiting for its file in hand, and forgets every
// picture, since a mod switch replaces them; the base-install art stays.
func (a *nlArt) bindContent(cs *contentSet) {
	if a.cs == cs {
		return
	}
	a.loader.stop()
	a.cs, a.loader, a.names, a.pics = cs, nil, nil, map[string]*ebiten.Image{}
}

// pauseLoader stops the loader until the next resume.
func (a *nlArt) pauseLoader() {
	if a != nil {
		a.loader.stop()
	}
}

// pictureNames is the bound content's name table; nil until resolved.
func (a *nlArt) pictureNames() *nlPicNames {
	if a == nil || a.loader == nil {
		return nil
	}
	if a.names == nil {
		a.names = a.loader.resolvedNames()
	}
	return a.names
}

// pic is the first of names the content has a picture for, cached. A name
// not decoded yet ends the search for this frame: a later name may only stand
// in once the earlier ones are known to be missing.
func (a *nlArt) pic(names ...string) *ebiten.Image {
	for _, name := range names {
		key := strings.ToLower(name)
		if img, ok := a.pics[key]; ok {
			if img != nil {
				return img
			}
			continue
		}
		if a.loader == nil {
			return nil
		}
		px, known := a.loader.take(key)
		if !known {
			a.loader.want(key)
			return nil
		}
		var img *ebiten.Image
		if px != nil {
			img = ebiten.NewImageFromImage(px)
		}
		a.pics[key] = img
		if img != nil {
			return img
		}
	}
	return nil
}

// tendPictures binds the art and the catalog to g's content and keeps its
// picture loader going: it starts once the catalog has compiled, which
// nlStats does off the game goroutine.
func (s *nlScreen) tendPictures(g *gameShell) {
	if s.art == nil || g == nil || g.cs == nil {
		return
	}
	s.bindStats(g)
	s.art.bindContent(g.cs)
	if s.art.loader == nil {
		cat := s.stats.catalog(g.cs)
		if cat == nil {
			return
		}
		var cards []nlPicRequest
		for _, page := range s.pages() {
			for i := range page.cards {
				cards = append(cards, nlPicRequest{key: page.cards[i].key, pics: page.cards[i].pics})
			}
		}
		s.art.loader = newNLPictures(g.cs.fs, cat, cards)
	}
	s.art.loader.resume()
}

// tendMenuPictures is tendPictures while the main menu shows: a window over
// the menu (Mods & Mutators, a load dialog) can request a content reload,
// which closes the archives the loader reads, so the loader waits for the
// menu to idle again.
func (s *nlScreen) tendMenuPictures(g *gameShell) {
	if g.frontend != nil && g.frontend.Panels.Under() != nil {
		s.art.pauseLoader()
		return
	}
	s.tendPictures(g)
}

// bindStats points the catalog and mutator clones at g's content.
func (s *nlScreen) bindStats(g *gameShell) {
	if s.statsCS != g.cs {
		s.stats = nlStats{}
	}
	s.statsCS = g.cs
}
