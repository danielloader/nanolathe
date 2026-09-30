package main

import (
	"bufio"
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/gif"
	"math"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"
)

// videoTrail is how far back, in ticks, a subject's fading trail reaches in
// the video: two game seconds, enough to show which way a unit came
// without hiding the others.
const videoTrail = 60

// video animates the sheet's picture: every log side by side, the units
// moving between their samples, and a clock.
type video struct {
	sc              *scene
	fps, speed      float64
	width, height   int
	headerH, titleH int
	frames          int         // animated frames, the first at t0 and the last at t1
	layout          *image.RGBA // everything that does not move
	img             *image.RGBA
	c               *canvas
}

func newVideo(sc *scene, fps, speed float64) *video {
	v := &video{sc: sc, fps: fps, speed: speed}
	n := len(sc.logs)
	titles := make([][]string, n)
	lines := 1
	for i, l := range sc.logs {
		titles[i] = wrapGroups(scoreGroups(l), sc.w-8)
		lines = max(lines, 1+len(titles[i]))
	}
	v.headerH = lineH + 10
	v.titleH = lines*lineH + 10
	width := pad + n*(sc.w+pad)
	items := append(sc.unitItems(), sc.commonItems()...)
	items = append(items, sc.subjectItems()...)
	legendH := legendHeight(items, width)
	height := pad + v.headerH + v.titleH + sc.h + pad + legendH
	// H.264 in yuv420p wants even dimensions.
	v.width, v.height = width+width%2, height+height%2
	span := float64(sc.snip.T1 - sc.snip.T0)
	v.frames = int(math.Ceil(span/v.ticksPerFrame())) + 1

	v.layout = newPicture(v.width, v.height, pageBG)
	c := newCanvas(v.layout)
	head := sc.snip.ID + " · " + sc.snip.Map
	if sc.snip.Class != "" {
		head += " · " + sc.snip.Class
	}
	head += " · " + strconv.FormatFloat(speed, 'f', -1, 64) + "x"
	c.text(pad, pad+4, head, textScale, inkMuted)
	for i, l := range sc.logs {
		x := pad + i*(sc.w+pad)
		y := pad + v.headerH
		c.fillRectI(x, y, sc.w, v.titleH, stripBG)
		c.text(x+8, y+6, l.label, textScale, inkFG)
		for j, line := range titles[i] {
			c.text(x+8, y+6+(j+1)*lineH, line, textScale, inkMuted)
		}
	}
	drawLegend(c, 0, pad+v.headerH+v.titleH+sc.h+pad, width, items)
	v.img = image.NewRGBA(v.layout.Rect)
	v.c = newCanvas(v.img)
	return v
}

func (v *video) ticksPerFrame() float64 {
	return 30 * v.speed / v.fps
}

// render draws frame i into v.img.
func (v *video) render(i int) *image.RGBA {
	sc := v.sc
	t := math.Min(float64(sc.snip.T0)+float64(i)*v.ticksPerFrame(), float64(sc.snip.T1))
	copy(v.img.Pix, v.layout.Pix)
	clock := fmt.Sprintf("t+%.1f s  tick %d", (t-float64(sc.snip.T0))/30, int(t))
	v.c.text(v.width-pad-textWidth(clock, textScale), pad+4, clock, textScale, inkFG)
	for i, l := range sc.logs {
		x := pad + i*(sc.w+pad)
		y := pad + v.headerH + v.titleH
		sc.drawMoment(v.c.sub(image.Rect(x, y, x+sc.w, y+sc.h)), l, t, videoTrail)
	}
	return v.img
}

// holdFrames is how long the last moment stays on screen: one second.
func (v *video) holdFrames() int {
	return max(1, int(math.Round(v.fps)))
}

// writeMP4 pipes raw frames to ffmpeg for H.264.
func (v *video) writeMP4(path string) error {
	ff, err := exec.LookPath("ffmpeg")
	if err != nil {
		return fmt.Errorf("nanolathe: ffmpeg is not installed: logical path ffmpeg, providers searched [PATH], expected an ffmpeg binary to encode %s; write a .gif instead, which needs none", path)
	}
	tmp := strings.TrimSuffix(path, ".mp4") + ".tmp.mp4"
	var stderr bytes.Buffer
	cmd := exec.Command(ff, "-hide_banner", "-loglevel", "error", "-y",
		"-f", "rawvideo", "-pix_fmt", "rgba", "-s", fmt.Sprintf("%dx%d", v.width, v.height),
		"-framerate", strconv.FormatFloat(v.fps, 'f', -1, 64), "-i", "-",
		"-an", "-c:v", "libx264", "-preset", "medium", "-crf", "20", "-pix_fmt", "yuv420p",
		"-movflags", "+faststart", "-map_metadata", "-1", "-fflags", "+bitexact", "-flags:v", "+bitexact", tmp)
	cmd.Stderr = &stderr
	in, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("ffmpeg: %w", err)
	}
	w := bufio.NewWriterSize(in, 1<<22)
	var werr error
	for i := 0; i < v.frames && werr == nil; i++ {
		_, werr = w.Write(v.render(i).Pix)
	}
	for i := 0; i < v.holdFrames() && werr == nil; i++ {
		_, werr = w.Write(v.img.Pix)
	}
	if werr == nil {
		werr = w.Flush()
	}
	in.Close()
	if err := cmd.Wait(); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("ffmpeg: %v: %s", err, strings.TrimSpace(stderr.String()))
	}
	if werr != nil {
		os.Remove(tmp)
		return fmt.Errorf("ffmpeg: %w", werr)
	}
	return os.Rename(tmp, path)
}

// writeGIF writes the animation as a GIF. One palette serves every frame:
// the colours the overlays use plus a median cut of the first frame, which
// holds all the terrain. After the first frame, pixels that did not change
// are transparent and each frame is cropped to what changed, which keeps
// the file small.
func (v *video) writeGIF(path string) error {
	first := v.render(0)
	pal, fixed := gifPalette(first)
	lut := nearestLUT(pal[:len(pal)-1])
	// The page, text and overlay colours keep their exact entries even
	// where a terrain colour shares their five-bit cell.
	for i, c := range pal[:fixed] {
		rc := c.(color.RGBA)
		lut[int(rc.R>>3)<<10|int(rc.G>>3)<<5|int(rc.B>>3)] = uint8(i)
	}
	transparent := uint8(len(pal) - 1)
	delay := max(2, int(math.Round(100/v.fps)))
	g := &gif.GIF{Config: image.Config{ColorModel: pal, Width: v.width, Height: v.height}}
	prev := quantize(first, pal, lut)
	g.Image = append(g.Image, prev)
	g.Delay = append(g.Delay, delay)
	g.Disposal = append(g.Disposal, gif.DisposalNone)
	for i := 1; i < v.frames; i++ {
		cur := quantize(v.render(i), pal, lut)
		g.Image = append(g.Image, changedPart(prev, cur, transparent))
		g.Delay = append(g.Delay, delay)
		g.Disposal = append(g.Disposal, gif.DisposalNone)
		prev = cur
	}
	g.Delay[len(g.Delay)-1] += 100 // hold the last moment for a second, as the MP4 does
	tmp := path + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	bw := bufio.NewWriterSize(f, 1<<20)
	if err := gif.EncodeAll(bw, g); err != nil {
		f.Close()
		return err
	}
	if err := bw.Flush(); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// gifPalette is 255 opaque colours and a transparent last entry: first the
// page, text and overlay colours (fixed of them), then a median cut of the
// first frame.
func gifPalette(img *image.RGBA) (pal color.Palette, fixed int) {
	own := []rgba{pageBG, stripBG, inkFG, inkMuted, halo, warn, otherUnit, featureMark}
	own = append(own, subjectPalette...)
	seen := map[[3]uint8]bool{}
	for _, c := range own {
		k := [3]uint8{uint8(c.r), uint8(c.g), uint8(c.b)}
		if !seen[k] {
			seen[k] = true
			pal = append(pal, color.RGBA{k[0], k[1], k[2], 255})
		}
	}
	fixed = len(pal)
	for _, c := range medianCut(img, 255-len(pal)) {
		pal = append(pal, c)
	}
	return append(pal, color.RGBA{}), fixed
}

// medianCut reduces the picture's colours, counted at five bits a channel,
// to k by repeatedly splitting the most populous box along its widest
// channel.
func medianCut(img *image.RGBA, k int) []color.RGBA {
	var counts [1 << 15]int
	for i := 0; i < len(img.Pix); i += 4 {
		counts[int(img.Pix[i]>>3)<<10|int(img.Pix[i+1]>>3)<<5|int(img.Pix[i+2]>>3)]++
	}
	type bin struct {
		c [3]int
		n int
	}
	var bins []bin
	for key, n := range counts {
		if n > 0 {
			bins = append(bins, bin{[3]int{key >> 10 & 31, key >> 5 & 31, key & 31}, n})
		}
	}
	boxes := [][]bin{bins}
	for len(boxes) < k {
		best, bestScore, axis := -1, 0, 0
		for i, b := range boxes {
			if len(b) < 2 {
				continue
			}
			var lo, hi [3]int
			lo, hi = b[0].c, b[0].c
			total := 0
			for _, e := range b {
				for ch := 0; ch < 3; ch++ {
					lo[ch], hi[ch] = min(lo[ch], e.c[ch]), max(hi[ch], e.c[ch])
				}
				total += e.n
			}
			for ch := 0; ch < 3; ch++ {
				if s := (hi[ch] - lo[ch]) * total; s > bestScore {
					best, bestScore, axis = i, s, ch
				}
			}
		}
		if best < 0 {
			break
		}
		b := boxes[best]
		sort.Slice(b, func(i, j int) bool {
			for ch := 0; ch < 3; ch++ {
				c := (axis + ch) % 3
				if b[i].c[c] != b[j].c[c] {
					return b[i].c[c] < b[j].c[c]
				}
			}
			return false
		})
		total := 0
		for _, e := range b {
			total += e.n
		}
		m, acc := 1, b[0].n
		for m < len(b)-1 && acc*2 < total {
			acc += b[m].n
			m++
		}
		boxes[best] = b[:m]
		boxes = append(boxes, b[m:])
	}
	out := make([]color.RGBA, 0, len(boxes))
	for _, b := range boxes {
		var s [3]int
		total := 0
		for _, e := range b {
			for ch := 0; ch < 3; ch++ {
				s[ch] += (e.c[ch]<<3 | 4) * e.n
			}
			total += e.n
		}
		out = append(out, color.RGBA{uint8(s[0] / total), uint8(s[1] / total), uint8(s[2] / total), 255})
	}
	return out
}

// nearestLUT maps every five-bit colour to its nearest palette entry.
func nearestLUT(pal color.Palette) []uint8 {
	lut := make([]uint8, 1<<15)
	for key := range lut {
		r, g, b := key>>10&31<<3|4, key>>5&31<<3|4, key&31<<3|4
		best, bestD := 0, math.MaxInt
		for i, c := range pal {
			pc := c.(color.RGBA)
			dr, dg, db := r-int(pc.R), g-int(pc.G), b-int(pc.B)
			if d := 3*dr*dr + 4*dg*dg + 2*db*db; d < bestD {
				best, bestD = i, d
			}
		}
		lut[key] = uint8(best)
	}
	return lut
}

func quantize(img *image.RGBA, pal color.Palette, lut []uint8) *image.Paletted {
	out := image.NewPaletted(img.Rect, pal)
	for i, j := 0, 0; i < len(img.Pix); i, j = i+4, j+1 {
		out.Pix[j] = lut[int(img.Pix[i]>>3)<<10|int(img.Pix[i+1]>>3)<<5|int(img.Pix[i+2]>>3)]
	}
	return out
}

// changedPart is cur cropped to the pixels that differ from prev, with the
// unchanged ones inside the crop transparent.
func changedPart(prev, cur *image.Paletted, transparent uint8) *image.Paletted {
	b := image.Rectangle{}
	w := cur.Rect.Dx()
	for y := 0; y < cur.Rect.Dy(); y++ {
		for x := 0; x < w; x++ {
			if prev.Pix[y*w+x] != cur.Pix[y*w+x] {
				b = b.Union(image.Rect(x, y, x+1, y+1))
			}
		}
	}
	if b.Empty() {
		b = image.Rect(0, 0, 1, 1)
	}
	out := image.NewPaletted(b, cur.Palette)
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			i := y*w + x
			v := cur.Pix[i]
			if prev.Pix[i] == v {
				v = transparent
			}
			out.Pix[out.PixOffset(x, y)] = v
		}
	}
	return out
}
