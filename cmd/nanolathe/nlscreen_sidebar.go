package main

import (
	"strings"

	"github.com/nanolathe-gg/nanolathe/formats"
	"github.com/nanolathe-gg/nanolathe/internal/content"
	"github.com/nanolathe-gg/nanolathe/vfs"
)

// Resolve the demonstration on the picture worker, through the HUD's ordinary
// GUI/download/art path. CANBUILD is not a source of visible cells or page
// membership [07 R-HUD-03 §6] (HUD design §3.3 and §3.17).
func loadNLSidebar(fs vfs.FSOps, cat *content.Catalog, builder *content.UnitDef, side *content.SideDef, halted func() bool) *sidebarProductCatalog {
	if fs == nil || builder == nil || side == nil || builder.BuildPageCount < 2 {
		return nil
	}
	fs = nlSidebarFiles{FSOps: fs, halted: halted}
	ctx := newBattleWindowContext(&contentSet{fs: fs}, nil)
	ctx.completeTransition()
	h := &retailBattleHUD{fs: fs, cat: cat, side: side, windowContext: ctx}
	h.common, _ = formats.LoadGAFFile(fs, "anims/commongui.gaf")
	h.intGAF, _ = formats.LoadGAFFile(fs, "anims/"+strings.ToLower(side.IntGAF)+".gaf")
	h.oldMain, _ = formats.LoadGAFFile(fs, "anims/oldmain.gaf")
	h.share, _ = formats.LoadGAFFile(fs, "anims/share.gaf")
	c := h.sidebarProductCatalog(cat, builder, int(builder.BuildPageCount))
	if !c.safe || len(c.cells) == 0 {
		return nil
	}
	return c
}

// The picture worker stops between asset reads, including the HUD's nested
// GUI/art lookups. Cancellation discards this private catalog; it must never
// cache cancellation as a missing asset on a battle's actual HUD.
type nlSidebarFiles struct {
	vfs.FSOps
	halted func() bool
}

func (f nlSidebarFiles) ReadFileLimit(name string, limit int64) ([]byte, error) {
	if f.halted != nil && f.halted() {
		return nil, vfs.ErrNotFound
	}
	return f.FSOps.ReadFileLimit(name, limit)
}

func (f nlSidebarFiles) Stat(name string) (vfs.EntryInfo, error) {
	if f.halted != nil && f.halted() {
		return vfs.EntryInfo{}, vfs.ErrNotFound
	}
	return f.FSOps.Stat(name)
}

// Original keeps source pages; the lock also caps a page without pulling in
// the next source page. The demonstration uses the same command reservation,
// oversized-lock fallback and partitioning as the battle (HUD design §3.3).
func nlSidebarPageStarts(c *sidebarProductCatalog, mode, height, lock int) ([]int, int) {
	capacity := len(c.cells)
	preservePages := mode == 0
	authoredLayout := mode == 0
	if mode != 0 {
		spacing, fits := c.sidebarSpacing(int32(height))
		capacity = c.sidebarCapacity(int32(height), spacing)
		if !fits || capacity < 2 || lock > capacity {
			capacity, preservePages = len(c.cells), true
			authoredLayout = true
		} else if lock > 0 {
			capacity, preservePages = lock, true
		}
	}
	if authoredLayout {
		for _, source := range c.pages {
			if source != nil && source.window.Rect.Y+source.window.Rect.H > retailScreenH {
				// The fitted path paginates overlapping authored row groups,
				// not normalized cells. Describe that layout instead of giving
				// a false build-page count for the selected game resolution.
				return nil, capacity
			}
		}
	}
	return sidebarPageStarts(c.cells, capacity, preservePages), capacity
}
