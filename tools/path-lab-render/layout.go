package main

import (
	"bufio"
	"fmt"
	"image"
	"image/png"
	"math"
	"os"
	"strconv"
)

const (
	textScale = 2                    // labels are drawn at twice the font's size
	lineH     = lineAdv * textScale  // one line of text
	pad       = 12                   // space around and between panels
	capH      = 7 * textScale        // height of a capital
	legendRow = glyphH*textScale + 8 // one row of the legend
)

// scoreGroups is a log's score as short fields, e.g. "near 12/13".
func scoreGroups(l *logData) []string {
	if l.score == nil {
		return nil
	}
	a := l.score.agg
	return []string{
		fmt.Sprintf("near %d/%d", a.Reached, a.N),
		fmt.Sprintf("mean %.0f t", a.MeanNear),
		fmt.Sprintf("blocked %d t", a.Blocked),
		fmt.Sprintf("overlap %d", a.Overlap),
	}
}

// wrapGroups joins fields into lines no wider than width pixels.
func wrapGroups(groups []string, width int) []string {
	var lines []string
	cur := ""
	for _, g := range groups {
		next := g
		if cur != "" {
			next = cur + "  " + g
		}
		if cur != "" && textWidth(next, textScale) > width {
			lines = append(lines, cur)
			next = g
		}
		cur = next
	}
	if cur != "" {
		lines = append(lines, cur)
	}
	return lines
}

// seconds formats a tick offset from the window's start as "t+12.5 s".
func seconds(ticks float64) string {
	s := ticks / 30
	if math.Abs(s-math.Round(s)) < 0.05 {
		return "t+" + strconv.Itoa(int(math.Round(s))) + " s"
	}
	return fmt.Sprintf("t+%.1f s", s)
}

// legendItem is a symbol and its label.
type legendItem struct {
	label string
	width int                           // symbol width, 0 for a text-only item
	sym   func(c *canvas, x, y float64) // draws the symbol with its left-middle at x, y
}

func (it legendItem) span() int {
	w := textWidth(it.label, textScale)
	if it.width > 0 {
		w += it.width + 7
	}
	return w
}

// legendHeight is the height of the strip drawLegend draws for items.
func legendHeight(items []legendItem, width int) int {
	return len(flowLegend(items, width-2*pad))*legendRow + 12
}

func flowLegend(items []legendItem, width int) [][]legendItem {
	var rows [][]legendItem
	var row []legendItem
	x := 0
	for _, it := range items {
		w := it.span()
		if len(row) > 0 && x+w > width {
			rows = append(rows, row)
			row, x = nil, 0
		}
		row = append(row, it)
		x += w + 20
	}
	if len(row) > 0 {
		rows = append(rows, row)
	}
	return rows
}

// drawLegend draws items on a strip of the given width at (x, y).
func drawLegend(c *canvas, x, y, width int, items []legendItem) {
	rows := flowLegend(items, width-2*pad)
	c.fillRectI(x, y, width, len(rows)*legendRow+12, stripBG)
	for ri, row := range rows {
		cx := x + pad
		cy := y + 6 + ri*legendRow + legendRow/2
		for _, it := range row {
			if it.width > 0 {
				it.sym(c, float64(cx), float64(cy))
				cx += it.width + 7
			}
			c.text(cx, cy-capH/2, it.label, textScale, inkMuted)
			cx += textWidth(it.label, textScale) + 20
		}
	}
}

// subjectItems lists the subjects' colours by unit ID.
func (sc *scene) subjectItems() []legendItem {
	items := []legendItem{{label: "subjects:"}}
	for _, id := range sc.subjects {
		col := sc.colours[id]
		items = append(items, legendItem{label: strconv.Itoa(id), width: 14, sym: func(c *canvas, x, y float64) {
			c.fillRect(x-1, y-8, x+15, y+8, halo.alpha(0.8))
			c.fillRect(x+1, y-6, x+13, y+6, col)
		}})
	}
	return items
}

func (sc *scene) firstColour() rgba {
	if len(sc.subjects) > 0 {
		return sc.colours[sc.subjects[0]]
	}
	return subjectPalette[0]
}

// commonItems are the legend entries of the map layers every picture has.
func (sc *scene) commonItems() []legendItem {
	return []legendItem{
		{label: "goal", width: 18, sym: func(c *canvas, x, y float64) {
			c.ring(x+9, y, 6.5, 4.2, halo.alpha(0.75))
			c.ring(x+9, y, 6.5, 2.2, sc.firstColour())
		}},
		{label: "structure", width: 22, sym: func(c *canvas, x, y float64) {
			c.fillRect(x, y-8, x+22, y+8, hex("#6b5a45"))
			c.fillRect(x, y-8, x+22, y+8, structFill)
			c.strokeRect(x+0.5, y-7.5, x+21.5, y+7.5, 1, structLine)
		}},
		{label: "blocking feature", width: 10, sym: func(c *canvas, x, y float64) {
			c.fillRect(x+0.25, y-4.75, x+9.75, y+4.75, featureEdge)
			c.fillRect(x+1, y-4, x+9, y+4, featureMark)
		}},
		{label: "snippet region", width: 26, sym: func(c *canvas, x, y float64) {
			c.dashed([]pt{{x, y}, {x + 26, y}}, 1.2, 7, 5, regionLine)
		}},
	}
}

func writePNG(path string, img image.Image) error {
	tmp := path + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	w := bufio.NewWriterSize(f, 1<<20)
	if err := png.Encode(w, img); err != nil {
		f.Close()
		return err
	}
	if err := w.Flush(); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
