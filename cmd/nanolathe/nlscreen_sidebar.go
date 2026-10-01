package main

import (
	"strings"

	"github.com/nanolathe-gg/nanolathe/formats"
	"github.com/nanolathe-gg/nanolathe/internal/content"
	"github.com/nanolathe-gg/nanolathe/internal/gui"
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

// Resolve the draft content's recommendation only while the player inherits.
// Explicit Free flow is zero and wins over every recommendation (HUD §3.3).
func (s *nlScreen) nlSidebarBuildLimit(d *nlDraft) int {
	if d.pres.BuildMenuPageSize >= 0 {
		return d.pres.BuildMenuPageSize
	}
	m := s.modAt(d.mod)
	if m != nil && m.BuildMenuPageSize > 0 {
		return m.BuildMenuPageSize
	}
	if g := s.shell(); g != nil && g.cs != nil && sameMod(m, g.cs.mod) {
		return max(0, g.cs.buildMenuPageSize())
	}
	return 0
}

func (s *nlScreen) nlSidebarCountChoice(d *nlDraft) int {
	if d.pres.ExpandedSidebar == 0 {
		return 0
	}
	limit := s.nlSidebarBuildLimit(d)
	if limit == 0 {
		return 3
	}
	// Keep unusual saved counts until a choice changes them. The description
	// and demonstration name the exact count; the selector uses the closest
	// supported fixed choice rather than changing the saved preference.
	if abs(limit-6) <= abs(limit-12) {
		return 1
	}
	return 2
}

// Original keeps source pages. Every adaptive choice partitions the flattened
// resolved cells, using the exact battle layout for capacity and orders (HUD
// design §3.3 and §3.17).
func nlSidebarPageStarts(c *sidebarProductCatalog, original bool, height, limit int, orders bool) ([]int, sidebarPageLayout) {
	p := sidebarPageLayout{capacity: len(c.cells)}
	if !original {
		p = c.sidebarLayout(height, limit, orders)
		if p.capacity > 0 {
			return sidebarPageStarts(c.cells, p.capacity, false), p
		}
	}
	if original || p.capacity == 0 {
		for _, source := range c.pages {
			if source != nil && source.window.Rect.Y+source.window.Rect.H > retailScreenH {
				// The fitted path paginates overlapping authored row groups,
				// not normalized cells. Describe that layout instead of giving
				// a false build-page count for the selected game resolution.
				return nil, p
			}
		}
	}
	// A scaffold the adaptive helper cannot fit uses the authored path.
	return sidebarPageStarts(c.cells, len(c.cells), true), p
}

// These are drawing placements only. Capacity and the retained command set
// always come from sidebarLayout, shared with the battle (HUD §3.3/§3.17).
func nlSidebarPreviewItems(c *sidebarProductCatalog, start, end, height int, p sidebarPageLayout, original bool) (products, controls []sidebarProduct) {
	if original {
		for _, cell := range c.cells[start:end] {
			for _, product := range cell.products {
				product.rect = product.source.window.PlacedRect(product.source.index)
				products = append(products, product)
			}
		}
		if start < end {
			if source := c.pages[c.cells[start].page]; source != nil {
				for _, i := range source.indices {
					controls = append(controls, sidebarProduct{source: sidebarGadgetSource{source.window, source.art, i}, rect: source.window.PlacedRect(i)})
				}
			}
		}
		return products, controls
	}
	gridGap, lowerHeight := int32(0), p.lowerHeight
	for _, gap := range p.spacing {
		if gap.at < 0 {
			gridGap += gap.pixels
		} else {
			lowerHeight += gap.pixels
		}
	}
	for _, tab := range c.tabs {
		tab.rect.Y += 128
		controls = append(controls, tab)
	}
	for _, item := range p.commands {
		r := item.rect
		for _, gap := range p.spacing {
			if gap.at >= 0 && gap.at <= item.rect.Y {
				r.Y += gap.pixels
			}
		}
		r.Y += int32(height) - lowerHeight
		controls = append(controls, sidebarProduct{source: item.source, rect: r})
	}
	for n, cell := range c.cells[start:end] {
		origin := gui.Rect{X: int32(n%2) * 64, Y: 128 + c.upperHeight + gridGap + int32(n/2)*64}
		for _, product := range cell.products {
			product.rect.X += origin.X
			product.rect.Y += origin.Y
			products = append(products, product)
		}
	}
	return products, controls
}
