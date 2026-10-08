// Command uigen generates gunmetal battle UI elements from a seamless material
// and two fonts, at any integer scale. Layout is in retail 1x pixels.
//
//	uigen -material gunmetal-tile.png -caption-font SairaCondensed-750.ttf -scale 2 -out out/
//	uigen ... -ta ~/TotalAnnihilation   # palette-indexed, dithered output
//	uigen tile gunmetal.jpg gunmetal-tile.png   # make a material seamless
package main

import (
	"flag"
	"fmt"
	"image/color"
	"math"
	"os"
	"path/filepath"
	"strings"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "tile" {
		if len(os.Args) != 4 {
			fail("usage: uigen tile <in> <out>")
		}
		src, err := LoadLayer(os.Args[2])
		check(err)
		check(MakeSeamless(src).Save(os.Args[3]))
		return
	}
	material := flag.String("material", "gunmetal-tile.png", "seamless material texture")
	captionFont := flag.String("caption-font", "SairaCondensed-750.ttf", "button caption font")
	labelFont := flag.String("label-font", "", "top bar label font (default: the caption font)")
	scale := flag.Float64("scale", 2, "output scale over retail 1x")
	light := flag.String("light", "bottom-left", "light direction: bottom-left, top-left, bottom-right or top-right")
	cool := flag.Float64("cool", 0.05, "shift the material towards blue by this fraction")
	flag.Float64Var(&recessSoft, "recess-soft", recessSoft, "blur on the light recesses, in 1x pixels")
	flag.Float64Var(&pressedLift, "pressed-lift", pressedLift, "pressed face brightness over normal")
	out := flag.String("out", "uigen-out", "output directory")
	palette := flag.String("palette", "", "16x16 palette swatch image; when set, elements are written palette-indexed with Floyd-Steinberg dithering")
	install := flag.String("ta", "", "Total Annihilation install to read the palette from, instead of -palette")
	flag.Parse()

	mat, err := LoadLayer(*material)
	check(err)
	s := &Style{Scale: *scale, Cool: *cool, Material: mat,
		CaptionFont: loadFont(*captionFont)}
	s.LabelFont = s.CaptionFont
	if *labelFont != "" {
		s.LabelFont = loadFont(*labelFont)
	}
	switch *light {
	case "bottom-left":
		s.Light = Light{-1, 1}
	case "top-left":
		s.Light = Light{-1, -1}
	case "bottom-right":
		s.Light = Light{1, 1}
	case "top-right":
		s.Light = Light{1, -1}
	default:
		fail("unknown -light " + *light)
	}
	check(os.MkdirAll(*out, 0o755))
	var pal color.Palette
	switch {
	case *install != "":
		pal, err = LoadInstallPalette(*install)
		check(err)
	case *palette != "":
		pal, err = LoadPalette(*palette)
		check(err)
	}
	save := func(name string, l *Layer) *Layer {
		path := filepath.Join(*out, name+".png")
		if pal != nil {
			check(savePaletted(path, Dither(l, pal)))
		} else {
			check(l.Save(path))
		}
		fmt.Println(name, l.W, "x", l.H)
		return l
	}
	rows := [][]*Layer{{save("topbar", s.TopBar())}}
	for _, st := range []string{"normal", "pressed", "greyed"} {
		var row []*Layer
		for _, b := range commandButtons {
			row = append(row, save(strings.ToLower(strings.ReplaceAll(b, "-", ""))+"-"+st, s.ActionButton(b, st)))
		}
		rows = append(rows, row)
	}
	var arrows []*Layer
	for _, dir := range []string{"prev", "next"} {
		for _, st := range []string{"normal", "pressed", "greyed"} {
			arrows = append(arrows, save(dir+"-"+st, s.PageArrow(dir == "next", st)))
		}
	}
	rows = append(rows, arrows)
	for _, t := range selectors {
		var row []*Layer
		for i, text := range t.states {
			row = append(row, save(fmt.Sprintf("%s-%d", t.name, i), s.Toggle(t.name, text, i)))
		}
		rows = append(rows, row)
	}
	sheet := Grid(rows, s.atLeast1(4))
	if pal != nil {
		check(savePaletted(filepath.Join(*out, "preview.png"), Dither(sheet, pal)))
	} else {
		check(sheet.Save(filepath.Join(*out, "preview.png")))
	}
}

// commandButtons are the order buttons generated in each state.
var commandButtons = []string{"MOVE", "STOP", "ATTACK", "PATROL", "GUARD", "REPAIR", "RECLAIM", "CAPTURE", "LOAD", "UNLOAD", "D-GUN"}

// selectors are the three-light order buttons; each state lights its light.
var selectors = []struct {
	name   string
	states []string
}{
	{"fireorders", []string{"HOLD FIRE", "RETURN FIRE", "FIRE AT WILL"}},
	{"moveorders", []string{"HOLD POSITION", "MANEUVER", "ROAM"}},
}

// TopBar is the 513x32 resource strip: plate, centre divider, METAL and ENERGY
// labels, +/- signs and the two bar grooves. The game draws the bar fill and
// numbers over it.
func (s *Style) TopBar() *Layer {
	w, h := s.px(513), s.px(32)
	l := s.Face(w, h, 0.7287)
	e := s.atLeast1(1)
	litC, darkC := white.WithA(0.25), black.WithA(0.65)
	side := func(isLit bool) RGBA {
		if isLit {
			return litC
		}
		return darkC
	}
	bottomLit, leftLit := s.Light.Y > 0, s.Light.X < 0
	l.Rect(0, 0, w-1, e-1, side(!bottomLit))
	l.Rect(0, h-e, w-1, h-1, side(bottomLit))
	l.Rect(0, 0, e-1, h-1, side(leftLit))
	// Divider: a dark line with a light line on the side away from the light.
	dx := s.px(255.5)
	l.Rect(dx, e, dx+e-1, h-1-e, black.WithA(0.6))
	lx := dx + e
	if !leftLit {
		lx = dx - 1
	}
	l.Rect(lx, e, lx, h-1-e, white.WithA(0.18))
	// Wear in the open plate either side of each label; the labels, signs and
	// grooves are drawn over it.
	s.Wear(l, "topbar-metal", s.px(38))
	s.Wear(l, "topbar-energy", s.px(287))
	s.Wear(l, "topbar-mid", s.px(250))

	for _, lab := range []struct {
		text        string
		x, w        float64
		top, bottom string
	}{{"METAL", 40, 40, "#b4d0ff", "#2a44d8"}, {"ENERGY", 289, 46, "#ffe880", "#e84a10"}} {
		g := s.GradientLabel(lab.text, s.px(lab.w), s.px(11), hex(lab.top), hex(lab.bottom))
		s.Place(l, g, 0, s.px(lab.x), s.px(8), 0.9)
	}
	for _, off := range []float64{0, 252} {
		green, red := hex("#30e040"), hex("#ff3020")
		l.Rect(s.px(221+off), s.px(9.5), s.px(226.5+off)-1, s.px(11)-1, green)
		l.Rect(s.px(223+off), s.px(7.5), s.px(224.5+off)-1, s.px(13)-1, green)
		l.Rect(s.px(221+off), s.px(19.5), s.px(226.5+off)-1, s.px(21)-1, red)
	}
	// Bar anchors from the ARM side data, relative to the strip's left edge.
	g := s.atLeast1(1)
	for _, b := range [][4]float64{{89, 12, 216, 14}, {342, 12, 469, 14}} {
		x0, y0 := s.px(b[0]), s.px(b[1])
		x1, y1 := s.px(b[2]+1)-1, s.px(b[3]+1)-1
		s.Recess(l, x0-g, y0-g, x1+g, y1+g, 0.55, 0.85, 0.22)
	}
	return l
}

var captionNormal = CaptionStyle{Fill: hex("#dfdfb7"), NearLight: hex("#b9b996"), FarLight: hex("#f6f6e2"),
	Outline: hex("#070f00"), OutlineR: 0.75, SX: 0.85, SY: 1.15}

var captionGreyed = CaptionStyle{Fill: hex("#5a5a52"), NearLight: hex("#4a4a44"), FarLight: hex("#6c6c64"),
	Outline: hex("#070f00"), OutlineR: 0.75, SX: 0.85, SY: 1.15}

const faceBright = 2.0

// pressedLift is the pressed face's brightness over the normal one. Retail's
// art doubles it (98 against 50 at 1x); from this brighter base 1.75 reads
// the same.
var pressedLift = 1.75

// recessSoft is the Gaussian blur on the light recesses, in 1x pixels.
var recessSoft = 0.35

// ActionButton is a 54x30 command button in its normal, pressed or greyed state.
func (s *Style) ActionButton(text, state string) *Layer {
	w, h := s.px(54), s.px(30)
	mult, sunken, cs, shift, capShadow := faceBright, false, captionNormal, 0, 0.8
	switch state {
	case "pressed":
		// Retail's cues: the face lights up to nearly twice its brightness, the
		// lip inverts and the caption moves one pixel down-right.
		mult, sunken, shift = faceBright*pressedLift, true, s.px(1)
	case "greyed":
		mult, cs = faceBright*0.6, captionGreyed
	}
	l := s.Face(w, h, mult)
	s.Bevel(l, s.px(2), 0.40, 0.65, 0.12, sunken)
	s.Wear(l, text, w)
	cw := s.captionWidth(text, 46)
	s.placeCaption(l, text, (54-cw)/2, 10, cw, cs, shift, capShadow)
	return l
}

// Toggle is a 113x21 three-state order button: a caption and three pill
// lights, the given one lit. seed keeps the wear the same across states.
func (s *Style) Toggle(seed, text string, lit int) *Layer {
	w, h := s.px(113), s.px(21)
	l := s.Face(w, h, faceBright)
	s.Bevel(l, s.px(2), 0.40, 0.65, 0.12, false)
	s.Wear(l, seed, s.px(80))
	s.placeCaption(l, text, 7, 6, s.captionWidth(text, 74), captionNormal, 0, 0.8)
	well, wm := s.Well(s.px(7), s.px(15), 1.5*s.Scale, recessSoft*s.Scale)
	on := s.LED(s.px(4), s.px(11), hex("#0c3800"), hex("#62e01c"), RGBA{200.0 / 255, 1, 140.0 / 255, 1}, 0.85)
	off := s.LED(s.px(4), s.px(11), hex("#050600"), hex("#2c3320"), RGBA{235.0 / 255, 1, 210.0 / 255, 1}, 0.30)
	for i := range 3 {
		x := float64(9 * i)
		l.Over(well, s.px(85+x)-wm, s.px(3)-wm)
		led := off
		if i == lit {
			led = on
		}
		l.Over(led, s.px(86.5+x), s.px(5))
	}
	return l
}

// captionWidth is a caption's box width in 1x pixels, from the word's own
// proportions, calibrated so RECLAIM fills the 42 pixels retail gives it, and
// capped at limit.
func (s *Style) captionWidth(text string, limit float64) float64 {
	ratio := func(t string) float64 { m := TextMask(s.CaptionFont, t, 0); return float64(m.W) / float64(m.H) }
	return math.Min(limit, math.Round(42*ratio(text)/ratio("RECLAIM")))
}

// PageArrow is the build menu's 45x17 previous or next page button: a plate
// with a 45-degree point at one end, in its normal, pressed or greyed state.
func (s *Style) PageArrow(next bool, state string) *Layer {
	w, h := s.px(45), s.px(17)
	mult, sunken := faceBright, false
	switch state {
	case "pressed":
		mult, sunken = faceBright*pressedLift, true
	case "greyed":
		mult = faceBright * 0.6
	}
	l := s.Face(w, h, mult)
	seed := "PREV"
	if next {
		seed = "NEXT"
	}
	s.Wear(l, seed, w-s.px(10))
	// The point: inside where the distance from the pointed end exceeds the
	// distance from the vertical centre line, so it closes at 45 degrees.
	mask, edge := ArrowMask(w, h, next), float64(s.px(2))
	s.BevelShape(l, mask, edge, 0.40, 0.65, 0.12, sunken)
	return l
}

// placeCaption centres a taller caption on its 8px authored box.
func (s *Style) placeCaption(l *Layer, text string, x, y, boxW float64, cs CaptionStyle, shift int, shadow float64) {
	c, pad := s.Caption(text, s.px(boxW), s.px(8), cs)
	ch := c.H - 2*pad
	top := s.px(y) - (ch-s.px(8))/2
	s.Place(l, c, pad, s.px(x)+shift, top+shift, shadow)
}

// Grid lays rows of layers out on a dark background for review.
func Grid(rows [][]*Layer, gap int) *Layer {
	w, h := 0, gap
	for _, row := range rows {
		rw, rh := gap, 0
		for _, l := range row {
			rw += l.W + gap
			rh = max(rh, l.H)
		}
		w, h = max(w, rw), h+rh+gap
	}
	out := NewLayer(w, h)
	out.Rect(0, 0, w-1, h-1, hex("#1e1e1e"))
	y := gap
	for _, row := range rows {
		x, rh := gap, 0
		for _, l := range row {
			out.Over(l, x, y)
			x += l.W + gap
			rh = max(rh, l.H)
		}
		y += rh + gap
	}
	return out
}

// MakeSeamless removes large-scale lighting with a wrapped box blur and
// cross-fades the image with its half-rolled copy so opposite edges meet.
func MakeSeamless(src *Layer) *Layer {
	w, h := src.W, src.H
	rad := w / 8
	out := NewLayer(w, h)
	var chans [3][]float64
	for c := range 3 {
		v := make([]float64, w*h)
		mean := 0.0
		for i := range v {
			v[i] = src.Pix[i*4+c]
			mean += v[i]
		}
		mean /= float64(w * h)
		bl := v
		for range 3 {
			bl = boxWrap(bl, w, h, rad)
		}
		for i := range v {
			v[i] = v[i] - bl[i] + mean
		}
		chans[c] = v
	}
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			d := math.Min(math.Min(float64(x), float64(w-1-x)), math.Min(float64(y), float64(h-1-y))) / (float64(w) / 2)
			m := math.Min(1, d*4)
			rx, ry := (x+w/2)%w, (y+h/2)%h
			var px [3]float64
			for c := range 3 {
				px[c] = m*chans[c][y*w+x] + (1-m)*chans[c][ry*w+rx]
			}
			out.Set(x, y, RGBA{px[0], px[1], px[2], 1})
		}
	}
	return out
}

func boxWrap(in []float64, w, h, rad int) []float64 {
	tmp, out := make([]float64, w*h), make([]float64, w*h)
	n := float64(2*rad + 1)
	for y := 0; y < h; y++ {
		s := 0.0
		for k := -rad; k <= rad; k++ {
			s += in[y*w+((k%w)+w)%w]
		}
		for x := 0; x < w; x++ {
			tmp[y*w+x] = s / n
			s += in[y*w+(x+rad+1)%w] - in[y*w+((x-rad)%w+w)%w]
		}
	}
	for x := 0; x < w; x++ {
		s := 0.0
		for k := -rad; k <= rad; k++ {
			s += tmp[((k%h)+h)%h*w+x]
		}
		for y := 0; y < h; y++ {
			out[y*w+x] = s / n
			s += tmp[((y+rad+1)%h)*w+x] - tmp[((y-rad)%h+h)%h*w+x]
		}
	}
	return out
}

func check(err error) {
	if err != nil {
		fail(err.Error())
	}
}

func fail(msg string) {
	fmt.Fprintln(os.Stderr, "uigen:", msg)
	os.Exit(1)
}
