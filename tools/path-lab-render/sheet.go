package main

import (
	"fmt"
	"image"
	"math"
	"sort"
	"strconv"
	"strings"
)

// sheetTicks resolves the moments of a contact sheet. A -ticks entry is a
// recording tick ("91500"), ticks after the window opens ("+300"), or
// seconds after it opens ("10s"); without -ticks, frames moments are spread
// evenly over the window, both ends included.
func sheetTicks(spec string, frames int, t0, t1 int32) ([]float64, error) {
	var out []float64
	if strings.TrimSpace(spec) != "" {
		for _, f := range strings.Split(spec, ",") {
			f = strings.TrimSpace(f)
			var t float64
			switch {
			case strings.HasSuffix(f, "s"):
				v, err := strconv.ParseFloat(strings.TrimSuffix(f, "s"), 64)
				if err != nil {
					return nil, fmt.Errorf("-ticks entry %q: %w", f, err)
				}
				t = float64(t0) + v*30
			case strings.HasPrefix(f, "+"):
				v, err := strconv.ParseFloat(f[1:], 64)
				if err != nil {
					return nil, fmt.Errorf("-ticks entry %q: %w", f, err)
				}
				t = float64(t0) + v
			default:
				v, err := strconv.ParseFloat(f, 64)
				if err != nil {
					return nil, fmt.Errorf("-ticks entry %q: %w", f, err)
				}
				t = v
			}
			if t < float64(t0) || t > float64(t1) {
				return nil, fmt.Errorf("-ticks entry %q is tick %.0f, outside the window %d..%d", f, t, t0, t1)
			}
			out = append(out, t)
		}
		return out, nil
	}
	if frames < 1 {
		return nil, fmt.Errorf("-frames %d: want at least 1", frames)
	}
	if frames == 1 {
		return []float64{float64(t1)}, nil
	}
	for i := 0; i < frames; i++ {
		out = append(out, float64(t0)+float64(t1-t0)*float64(i)/float64(frames-1))
	}
	return out, nil
}

// unitItems are the legend entries of the pictures that show units at a
// moment.
func (sc *scene) unitItems() []legendItem {
	return []legendItem{
		{label: "subject", width: 16, sym: func(c *canvas, x, y float64) {
			unitShape(c, pt{x + 8, y}, 6.5, 6.5, sc.firstColour(), false, 49152)
		}},
		{label: "other unit", width: 16, sym: func(c *canvas, x, y float64) {
			unitShape(c, pt{x + 8, y}, 6.5, 6.5, otherUnit, false, 49152)
		}},
		{label: "blocked", width: 20, sym: func(c *canvas, x, y float64) {
			unitShape(c, pt{x + 10, y}, 6.5, 6.5, otherUnit, true, 49152)
		}},
	}
}

// renderSheet draws a contact sheet: one row per log, one column per
// moment.
func renderSheet(sc *scene, ticks []float64) *image.RGBA {
	gutter := 0
	for _, l := range sc.logs {
		gutter = max(gutter, textWidth(l.label, textScale))
		for _, g := range scoreGroups(l) {
			gutter = max(gutter, textWidth(g, textScale))
		}
	}
	gutter += 16
	header := lineH + 6
	width := pad + gutter + pad + len(ticks)*(sc.w+pad)
	items := append(sc.unitItems(), sc.commonItems()...)
	items = append(items, sc.subjectItems()...)
	legendH := legendHeight(items, width)
	height := pad + header + len(sc.logs)*(sc.h+pad) + legendH
	img := newPicture(width, height, pageBG)
	c := newCanvas(img)
	x0 := pad + gutter + pad
	for j, t := range ticks {
		label := seconds(t - float64(sc.snip.T0))
		x := x0 + j*(sc.w+pad) + (sc.w-textWidth(label, textScale))/2
		c.text(x, pad, label, textScale, inkFG)
	}
	for i, l := range sc.logs {
		y := pad + header + i*(sc.h+pad)
		c.fillRectI(pad, y, gutter, sc.h, stripBG)
		c.text(pad+8, y+8, l.label, textScale, inkFG)
		for k, g := range scoreGroups(l) {
			c.text(pad+8, y+8+(k+1)*lineH, g, textScale, inkMuted)
		}
		for j, t := range ticks {
			x := x0 + j*(sc.w+pad)
			p := c.sub(image.Rect(x, y, x+sc.w, y+sc.h))
			sc.drawMoment(p, l, t, 0)
		}
	}
	drawLegend(c, 0, pad+header+len(sc.logs)*(sc.h+pad), width, items)
	return img
}

// drawMoment draws one log at tick t on a panel, with each subject's path
// over the last trail ticks fading behind it.
func (sc *scene) drawMoment(p *canvas, l *logData, t, trail float64) {
	p.paste(sc.base)
	sc.drawStructures(p, t, false)
	sc.drawGoals(p)
	if trail > 0 {
		sc.drawTrails(p, l, t, trail)
	}
	sc.drawUnits(p, l, t)
}

// drawTrails draws where each subject was over the last trail ticks, one
// segment per sample interval, more transparent the older it is.
func (sc *scene) drawTrails(c *canvas, l *logData, t, trail float64) {
	for _, id := range sc.subjects {
		cur, ok := l.at(id, t)
		if !ok {
			continue
		}
		col := sc.colours[id]
		prev := sc.toPanel(cur.x, cur.z)
		s := l.series[id]
		k := sort.Search(len(s), func(i int) bool { return float64(s[i].tick) >= t }) - 1
		newer := t
		for ; k >= 0 && t-float64(s[k].tick) <= trail; k-- {
			if !l.joined(s[k], sample{tick: int32(math.Ceil(newer))}) {
				break
			}
			q := sc.toPanel(s[k].x, s[k].z)
			age := (t - float64(s[k].tick)) / trail
			c.stroke([]pt{prev, q}, 2.4, col.alpha(0.85*(1-age)))
			prev, newer = q, float64(s[k].tick)
		}
	}
}
