package main

import (
	"fmt"
	"image/color"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/nanolathe-gg/nanolathe/internal/platform/screenkit"
)

// Small demonstrations drawn over the live preview for settings a battle
// alone cannot show: the side panel's build pages and the window on a display
// (docs/DESIGN_INTERFACE_HUD_INPUT.md §3.17).

// demoStage is the part of the screen a demonstration may use: right of the
// hero and above the cards.
func (s *nlScreen) demoStage() screenkit.Rect {
	u := s.u()
	x := 800 * u
	return screenkit.Rect{X: x, Y: 120 * u, W: s.w() - x - 40*u, H: float64(s.carouselTop()) - 150*u}
}

// caption is a small tag under a demonstration.
func (s *nlScreen) caption(screen *ebiten.Image, text string, x, y, a float64) {
	u := s.u()
	st := screenkit.Style{Size: 12 * u, Tracking: 0.16, Top: alphaC(nlGreenText, a), Upper: true}
	tw := s.fonts.Display.Measure(text, st) + 24*u
	r := screenkit.Rect{X: x - tw/2, Y: y, W: tw, H: 30 * u}
	screenkit.Fill(screen, r, color.RGBA{6, 12, 6, uint8(215 * a)})
	screenkit.Outline(screen, r, 1*u, alphaC(color.RGBA{60, 100, 60, 255}, a))
	s.fonts.Display.Draw(screen, text, r.X+12*u, r.Y+20*u, st)
}

// drawDemo runs the focused card's demonstration, if it has one.
func (s *nlScreen) drawDemo(screen *ebiten.Image, card nlCard, v int) {
	if card.demo == "" || s.compare || s.preview.frame == nil {
		return
	}
	switch card.demo {
	case "sidebar":
		s.demoSidebar(screen, v)
	case "fullscreen":
		s.demoFullscreen(screen, v == 1)
	}
}

// demoSidebar shows the running content's resolved build cells and source
// page breaks at the selected game resolution, with automatic page turns.
func (s *nlScreen) demoSidebar(screen *ebiten.Image, mode int) {
	u := s.u()
	stage := s.demoStage()
	var c *sidebarProductCatalog
	if names := s.art.pictureNames(); names != nil {
		c = names.sidebar
	}
	if c == nil {
		s.fonts.Body.Draw(screen, "No resolved build pages in this content.", stage.X, stage.Y+28*u, screenkit.Style{Size: 11 * u, Top: nlDim})
		return
	}
	g := s.shell()
	lock := (&battleSession{shell: &gameShell{presentation: s.draft.pres}, modBuildPageSize: g.cs.buildMenuPageSize()}).buildPageLock()
	starts, _ := nlSidebarPageStarts(c, mode, g.display.Height, lock)
	if len(starts) == 0 {
		s.fonts.Body.Draw(screen, "Uses the authored layout; oversized pages fit rows to the available height.", stage.X, stage.Y+28*u, screenkit.Style{Size: 11 * u, Top: nlDim})
		return
	}
	page := int(s.demoT/2.2) % len(starts)
	start, end := starts[page], len(c.cells)
	if page+1 < len(starts) {
		end = starts[page+1]
	}
	// Keep the panel stationary on short final pages without implying that
	// a six-cell authored page will be padded with the next page's products.
	most := 0
	for i, first := range starts {
		last := len(c.cells)
		if i+1 < len(starts) {
			last = starts[i+1]
		}
		most = max(most, last-first)
	}
	rows := s.ease("sidebar-rows", float64((most+1)/2), 8)
	cell := min(58*u, (stage.H-110*u)/float64(max(1, (most+1)/2)))
	panel := screenkit.Rect{X: stage.X + stage.W*0.12, Y: stage.Y + 10*u, W: 2*cell + 30*u, H: rows*cell + 74*u}
	screenkit.Shade(screen, screenkit.Rect{X: panel.X - 20*u, Y: panel.Y - 10*u, W: panel.W + 40*u, H: panel.H + 30*u}, 0.6)
	screenkit.Fill(screen, panel, color.RGBA{10, 14, 10, 235})
	s.well(screen, panel, 1)
	pages := len(starts)
	heading := s.ui.text(nlTextKey{kind: "build pages", i: page + 1, j: pages}, func() string { return fmt.Sprintf("Build  %d / %d", page+1, pages) })
	s.fonts.Display.Draw(screen, heading, panel.X+14*u, panel.Y+28*u, screenkit.Style{Size: 13 * u, Tracking: 0.2, Top: nlKicker, Upper: true})
	for i, buildCell := range c.cells[start:end] {
		r := screenkit.Rect{X: panel.X + 12*u + float64(i%2)*(cell+6*u), Y: panel.Y + 40*u + float64(i/2)*cell, W: cell, H: cell - 4*u}
		if r.Y+r.H > panel.Y+panel.H-30*u {
			break
		}
		for _, product := range buildCell.products {
			name := product.source.window.Gadgets[product.source.index].Name
			child := screenkit.Rect{X: r.X + float64(product.rect.X)*r.W/64, Y: r.Y + float64(product.rect.Y)*r.H/64, W: float64(product.rect.W) * r.W / 64, H: float64(product.rect.H) * r.H / 64}
			if pic := s.art.pic(name); pic != nil {
				screenkit.Image(screen, pic, child, 1, false)
			}
		}
		screenkit.Bevel(screen, r, 1.5*u, color.RGBA{190, 190, 182, 255}, color.RGBA{20, 20, 18, 255}, false)
	}
	label := [...]string{"Authored pages", "Authored, at most twelve", "Rows above orders"}[mode]
	s.caption(screen, label, panel.X+panel.W/2, panel.Y+panel.H+14*u, 1)
	resolution := s.ui.text(nlTextKey{kind: "sidebar resolution", i: g.display.Width, j: g.display.Height}, func() string {
		return fmt.Sprintf("Game resolution: %d x %d", g.display.Width, g.display.Height)
	})
	s.fonts.Body.Draw(screen, resolution, panel.X+panel.W/2, panel.Y+panel.H+62*u, screenkit.Style{Size: 11 * u, Top: nlDim, Align: 1})
}

// demoFullscreen is a display with the game's window on it.
func (s *nlScreen) demoFullscreen(screen *ebiten.Image, full bool) {
	u := s.u()
	stage := s.demoStage()
	mw := min(stage.W*0.7, 520*u)
	mh := mw * 10 / 16
	mon := screenkit.Rect{X: stage.X + (stage.W-mw)/2, Y: stage.Y + 20*u, W: mw, H: mh}
	screenkit.Shade(screen, screenkit.Rect{X: mon.X - 30*u, Y: mon.Y - 20*u, W: mon.W + 60*u, H: mon.H + 90*u}, 0.6)
	screenkit.Fill(screen, mon.Inset(-10*u), color.RGBA{24, 24, 22, 255})
	screenkit.Bevel(screen, mon.Inset(-10*u), 2*u, color.RGBA{120, 120, 112, 255}, color.RGBA{10, 10, 8, 255}, false)
	screenkit.Fill(screen, mon, color.RGBA{16, 30, 44, 255})
	screenkit.Fill(screen, screenkit.Rect{X: mon.X + mon.W/2 - 30*u, Y: mon.Y + mon.H + 10*u, W: 60 * u, H: 26 * u}, color.RGBA{40, 40, 36, 255})
	screenkit.Fill(screen, screenkit.Rect{X: mon.X + mon.W/2 - 70*u, Y: mon.Y + mon.H + 34*u, W: 140 * u, H: 8 * u}, color.RGBA{50, 50, 46, 255})
	target := 0.0
	if full {
		target = 1
	}
	k := s.ease("fullscreen", target, 6)
	ww, wh := mon.W*(0.62+0.38*k), mon.H*(0.62+0.38*k)
	win := screenkit.Rect{X: mon.X + (mon.W-ww)/2, Y: mon.Y + (mon.H-wh)/2, W: ww, H: wh}
	bar := 18 * u * (1 - k)
	if bar > 0.5 {
		screenkit.Fill(screen, screenkit.Rect{X: win.X, Y: win.Y, W: win.W, H: bar}, color.RGBA{200, 200, 196, 255})
		dots := []color.RGBA{{230, 90, 80, 255}, {230, 190, 70, 255}, {90, 200, 90, 255}}
		for i, c := range dots {
			screenkit.Disc(screen, win.X+10*u+float64(i)*12*u, win.Y+bar/2, 3.5*u*(1-k), c)
		}
	}
	body := screenkit.Rect{X: win.X, Y: win.Y + bar, W: win.W, H: win.H - bar}
	if s.preview.frame != nil {
		screenkit.Cover(screen, s.preview.frame, body, 1)
	}
	screenkit.Outline(screen, win, 1*u, color.RGBA{0, 0, 0, 255})
	label := "A window you can move"
	if full {
		label = "The whole display"
	}
	s.caption(screen, label, mon.X+mon.W/2, mon.Y+mon.H+54*u, 1)
}
