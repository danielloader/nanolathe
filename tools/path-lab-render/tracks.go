package main

import "image"

// renderTracks draws one panel per log, side by side: every unit's path over
// the whole window, the subjects in their colours.
func renderTracks(sc *scene) *image.RGBA {
	n := len(sc.logs)
	titles := make([][]string, n)
	lines := 1
	for i, l := range sc.logs {
		titles[i] = wrapGroups(scoreGroups(l), sc.w-8)
		lines = max(lines, 1+len(titles[i]))
	}
	titleH := lines*lineH + 10
	width := pad + n*(sc.w+pad)
	items := append([]legendItem{
		{label: "subject path", width: 26, sym: func(c *canvas, x, y float64) {
			pts := []pt{{x, y + 4}, {x + 9, y - 4}, {x + 17, y + 3}, {x + 26, y - 3}}
			c.stroke(pts, 4.2, halo.alpha(0.6))
			c.stroke(pts, 2.2, sc.firstColour())
		}},
		{label: "blocked", width: 22, sym: func(c *canvas, x, y float64) {
			c.stroke([]pt{{x + 3, y}, {x + 19, y}}, 7.5, halo.alpha(0.7))
			c.stroke([]pt{{x + 3, y}, {x + 19, y}}, 5, warn)
		}},
		{label: "start", width: 10, sym: func(c *canvas, x, y float64) {
			c.disc(x+5, y, 4.6, halo.alpha(0.85))
			c.disc(x+5, y, 3.3, sc.firstColour())
		}},
		{label: "other units", width: 26, sym: func(c *canvas, x, y float64) {
			c.stroke([]pt{{x, y + 3}, {x + 13, y - 3}, {x + 26, y + 2}}, 1.3, otherTrack)
		}},
	}, sc.commonItems()...)
	items = append(items, sc.subjectItems()...)
	legendH := legendHeight(items, width)
	height := pad + titleH + sc.h + pad + legendH
	img := newPicture(width, height, pageBG)
	c := newCanvas(img)
	for i, l := range sc.logs {
		x := pad + i*(sc.w+pad)
		c.fillRectI(x, pad, sc.w, titleH, stripBG)
		c.text(x+8, pad+6, l.label, textScale, inkFG)
		for j, line := range titles[i] {
			c.text(x+8, pad+6+(j+1)*lineH, line, textScale, inkMuted)
		}
		p := c.sub(image.Rect(x, pad+titleH, x+sc.w, pad+titleH+sc.h))
		p.paste(sc.base)
		sc.drawStructures(p, 0, true)
		sc.drawTracks(p, l)
	}
	drawLegend(c, 0, pad+titleH+sc.h+pad, width, items)
	return img
}
