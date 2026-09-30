package main

import (
	"errors"
	"fmt"
	"image"
	"math"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// cellSize is the world units per map cell.
const cellSize = 16

// defaultFoot is the footprint, in cells, of a unit the unit table does not
// know.
const defaultFoot = 2

// heightAt is the height the terrain picture was drawn with at world (x, z):
// the height byte of the cell the point lies in.
func (g *gridFile) heightAt(x, z float64) float64 {
	cx := min(max(int(math.Floor(x/cellSize)), 0), g.CellW-1)
	cz := min(max(int(math.Floor(z/cellSize)), 0), g.CellH-1)
	return float64(g.Heights[cz*g.CellW+cx])
}

// project maps a point on the ground to the terrain picture. The picture is
// the pre-rendered ground in screen space, where ground of height h is drawn
// h/2 pixels above its z.
func (g *gridFile) project(x, z float64) (float64, float64) {
	return x, z - g.heightAt(x, z)/2
}

// pictureCrop is the part of the terrain picture that shows a world
// rectangle grown by margin: the rectangle's columns, and the rows from its
// highest-drawn to its lowest-drawn ground.
func (g *gridFile) pictureCrop(region [4]float64, margin float64) image.Rectangle {
	mapW, mapH := float64(g.CellW*cellSize), float64(g.CellH*cellSize)
	x0, z0 := max(0, region[0]-margin), max(0, region[1]-margin)
	x1, z1 := min(mapW, region[2]+margin), min(mapH, region[3]+margin)
	top, bottom := math.Inf(1), math.Inf(-1)
	for cz := int(z0 / cellSize); cz < g.CellH && float64(cz*cellSize) < z1; cz++ {
		zt, zb := max(z0, float64(cz*cellSize)), min(z1, float64((cz+1)*cellSize))
		for cx := int(x0 / cellSize); cx < g.CellW && float64(cx*cellSize) < x1; cx++ {
			h := float64(g.Heights[cz*g.CellW+cx]) / 2
			top, bottom = min(top, zt-h), max(bottom, zb-h)
		}
	}
	if !(top < bottom) || !(x0 < x1) {
		return image.Rectangle{}
	}
	r := image.Rect(int(math.Floor(x0)), int(math.Floor(top)), int(math.Ceil(x1)), int(math.Ceil(bottom)))
	return r.Intersect(image.Rect(0, 0, g.ImageW, g.ImageH))
}

// autoScale is the output scale that makes a panel about 480 pixels wide,
// kept within 0.25..2 pixels per world unit.
func autoScale(cropW int) float64 {
	return min(2, max(0.25, 480/float64(cropW)))
}

type goal struct {
	id   int
	x, z float64
}

type structure struct {
	x, z       float64 // centre, world units
	w, h       float64 // footprint, world units
	spawn, die int32
}

func (s structure) aliveAt(t float64) bool {
	return float64(s.spawn) <= t && (s.die < 0 || t < float64(s.die))
}

// standsDuring reports whether the structure stands at any time from t0 to
// t1.
func (s structure) standsDuring(t0, t1 int32) bool {
	return s.spawn <= t1 && (s.die < 0 || s.die > t0)
}

// scene is everything the pictures share: the snippet, the crop of the
// terrain it happened on, and the logs to draw.
type scene struct {
	snip     *snippet
	grid     *gridFile
	feet     map[string][2]int
	logs     []*logData
	crop     image.Rectangle // terrain picture pixels shown in a panel
	scale    float64         // output pixels per world unit
	w, h     int             // panel size in output pixels
	base     *image.RGBA     // terrain, region outline and blocking features
	names    map[int]string  // snippet unit names by ID
	subjects []int           // subject IDs, ascending
	colours  map[int]rgba    // subject colours
	goals    []goal          // subject goals, in subject order
	structs  []structure
}

// loadScene reads everything the options name and prepares the panel
// geometry and the layers every panel shares.
func loadScene(o *options) (*scene, error) {
	if o.snippet == "" || o.mapDir == "" || len(o.logs) == 0 {
		return nil, errors.New("-snippet, -map and at least one -log are required")
	}
	if o.scale < 0 || o.scale > 8 {
		return nil, fmt.Errorf("-scale %v: want 0 (automatic) or up to 8 pixels per world unit", o.scale)
	}
	s, err := readSnippet(o.snippet)
	if err != nil {
		return nil, err
	}
	g, err := readGrid(o.mapDir)
	if err != nil {
		return nil, err
	}
	feet := map[string][2]int{}
	if o.units != "" {
		if feet, err = readFeet(o.units); err != nil {
			return nil, err
		}
	}
	var scores []scoreEntry
	if o.score != "" {
		if scores, err = readScores(o.score); err != nil {
			return nil, err
		}
	}
	sc := &scene{snip: s, grid: g, feet: feet}
	for _, spec := range o.logs {
		fr, err := readFrames(spec.path)
		if err != nil {
			return nil, err
		}
		l := newLog(spec.label, spec.path, fr)
		l.score = matchScore(scores, spec.label, spec.path)
		sc.logs = append(sc.logs, l)
	}
	region := s.Region
	if o.focus {
		region = focusRegion(s, sc.logs)
	}
	sc.crop = g.pictureCrop(region, o.margin)
	if sc.crop.Empty() {
		return nil, fmt.Errorf("nanolathe: snippet region is off the map: logical path %s, providers searched [file], expected a region inside the %dx%d-cell map", o.snippet, g.CellW, g.CellH)
	}
	sc.scale = o.scale
	if sc.scale <= 0 {
		sc.scale = autoScale(sc.crop.Dx())
	}
	sc.w = max(1, int(math.Round(float64(sc.crop.Dx())*sc.scale)))
	sc.h = max(1, int(math.Round(float64(sc.crop.Dy())*sc.scale)))
	pic, err := readPNGCrop(filepath.Join(o.mapDir, "terrain.png"), sc.crop)
	if err != nil {
		return nil, err
	}
	if pic.Rect.Empty() {
		return nil, fmt.Errorf("nanolathe: terrain picture does not cover the snippet: logical path %s, providers searched [file], expected a %dx%d picture", filepath.Join(o.mapDir, "terrain.png"), g.ImageW, g.ImageH)
	}
	sc.index()
	sc.base = sc.renderBase(pic)
	return sc, nil
}

// focusRegion is the ground between where the snippet's subjects stood when
// the window opened and the goals they are judged by: the view for a snippet
// whose region is much larger than what is being judged. It is the snippet
// region when the snippet names no subject.
func focusRegion(s *snippet, logs []*logData) [4]float64 {
	subject := map[int]bool{}
	r := [4]float64{math.Inf(1), math.Inf(1), math.Inf(-1), math.Inf(-1)}
	add := func(x, z float64) {
		r[0], r[1] = math.Min(r[0], x), math.Min(r[1], z)
		r[2], r[3] = math.Max(r[2], x), math.Max(r[3], z)
	}
	for i := range s.Units {
		if u := &s.Units[i]; u.Subject {
			subject[u.ID] = true
			add(u.X, u.Z)
		}
	}
	// The goal that is judged is the one of a subject's first order in the
	// window that was not already under way.
	first := map[int]int32{}
	for _, o := range s.Orders {
		if !subject[o.Unit] || o.Carry {
			continue
		}
		if t, ok := first[o.Unit]; !ok || o.Tick < t {
			first[o.Unit] = o.Tick
		}
	}
	for _, o := range s.Orders {
		if t, ok := first[o.Unit]; ok && !o.Carry && o.Tick == t {
			add(o.GoalX, o.GoalZ)
		}
	}
	if len(subject) == 0 {
		return s.Region
	}
	return r
}

// index gathers the snippet's subjects, their goals and colours, and its
// structures.
func (sc *scene) index() {
	s := sc.snip
	sc.names = map[int]string{}
	for i := range s.Units {
		u := &s.Units[i]
		if _, dup := sc.names[u.ID]; !dup {
			sc.names[u.ID] = u.Name
		}
		if u.Structure {
			w, h := sc.footprint(u.Name)
			sc.structs = append(sc.structs, structure{x: u.X, z: u.Z, w: w, h: h, spawn: u.Spawn, die: u.Die})
		}
	}
	sc.subjects = subjectIDs(s)
	sc.colours = assignColours(sc.subjects)
	for _, id := range sc.subjects {
		if x, z, ok := goalOf(s, id); ok {
			sc.goals = append(sc.goals, goal{id: id, x: x, z: z})
		}
	}
}

// subjectIDs lists a snippet's mobile subjects, ascending.
func subjectIDs(s *snippet) []int {
	seen := map[int]bool{}
	var ids []int
	for _, u := range s.Units {
		if u.Subject && !u.Structure && !seen[u.ID] {
			seen[u.ID] = true
			ids = append(ids, u.ID)
		}
	}
	sort.Ints(ids)
	return ids
}

// goalOf is the goal of the unit's first order in the window that was not
// already under way when the window opened, the order its score measures.
func goalOf(s *snippet, id int) (x, z float64, ok bool) {
	best := int32(math.MaxInt32)
	for _, o := range s.Orders {
		if o.Unit == id && !o.Carry && o.Tick < best {
			best, x, z, ok = o.Tick, o.GoalX, o.GoalZ, true
		}
	}
	return x, z, ok
}

// footprint is a unit's drawn size in world units.
func (sc *scene) footprint(name string) (float64, float64) {
	if f, ok := sc.feet[strings.ToUpper(name)]; ok {
		return float64(f[0] * cellSize), float64(f[1] * cellSize)
	}
	return defaultFoot * cellSize, defaultFoot * cellSize
}

func (sc *scene) isSubject(id int) bool {
	_, ok := sc.colours[id]
	return ok
}

// toPanel maps a world point on the ground to panel pixels.
func (sc *scene) toPanel(x, z float64) pt {
	px, py := sc.grid.project(x, z)
	return pt{(px - float64(sc.crop.Min.X)) * sc.scale, (py - float64(sc.crop.Min.Y)) * sc.scale}
}

// renderBase draws the layers every panel shares: the terrain, dimmed so
// the overlays read, the snippet region, and the cells blocking features
// cover.
func (sc *scene) renderBase(pic *image.RGBA) *image.RGBA {
	img := resampleTerrain(pic, sc.crop, sc.scale, sc.w, sc.h)
	c := newCanvas(img)
	r := sc.snip.Region
	var outline []pt
	edge := func(x0, z0, x1, z1 float64) {
		n := max(1, int(math.Ceil(math.Max(math.Abs(x1-x0), math.Abs(z1-z0))/4)))
		for i := 0; i < n; i++ {
			f := float64(i) / float64(n)
			outline = append(outline, sc.toPanel(x0+(x1-x0)*f, z0+(z1-z0)*f))
		}
	}
	edge(r[0], r[1], r[2], r[1])
	edge(r[2], r[1], r[2], r[3])
	edge(r[2], r[3], r[0], r[3])
	edge(r[0], r[3], r[0], r[1])
	outline = append(outline, outline[0])
	c.dashed(outline, 1.2, 7, 5, regionLine)
	cell := cellSize * sc.scale
	side := max(2.5, cell*0.55)
	for _, p := range sc.blockers() {
		q := sc.toPanel((float64(p.X)+0.5)*cellSize, (float64(p.Y)+0.5)*cellSize)
		c.fillRect(q.x-side/2-0.75, q.y-side/2-0.75, q.x+side/2+0.75, q.y+side/2+0.75, featureEdge)
		c.fillRect(q.x-side/2, q.y-side/2, q.x+side/2, q.y+side/2, featureMark)
	}
	return img
}

// blockers lists the cells blocking features and voids cover near the
// crop. The grid marks a blocking feature's anchor cell 1 and every
// feature's other cells 2 (its fringe), and a feature's cells run right and
// down from its anchor, so a blocking feature is its anchor plus the fringe
// rectangle that follows it. A feature the recording shows reclaimed before
// the window opened is left out: the replay removes it, whichever of its
// cells the reclaim names.
func (sc *scene) blockers() []image.Point {
	g := sc.grid
	// The world rectangle the crop can show: its columns, and rows reaching
	// down by the most a height can lift ground into view.
	cx0, cx1 := sc.crop.Min.X/cellSize-8, sc.crop.Max.X/cellSize+1
	cz0, cz1 := sc.crop.Min.Y/cellSize-8, (sc.crop.Max.Y+128)/cellSize+1
	cx0, cz0 = max(cx0, 0), max(cz0, 0)
	cx1, cz1 = min(cx1, g.CellW-1), min(cz1, g.CellH-1)
	var out []image.Point
	for cz := cz0; cz <= cz1; cz++ {
		for cx := cx0; cx <= cx1; cx++ {
			if g.Blocking[cz*g.CellW+cx] != 1 {
				continue
			}
			fw, fh := 1, 1
			for cx+fw < g.CellW && g.Blocking[cz*g.CellW+cx+fw] == 2 {
				fw++
			}
			for cz+fh < g.CellH && g.Blocking[(cz+fh)*g.CellW+cx] == 2 {
				fh++
			}
			foot := image.Rect(cx, cz, cx+fw, cz+fh)
			gone := false
			for _, r := range sc.snip.Reclaimed {
				gone = gone || image.Pt(int(r[0]), int(r[1])).In(foot)
			}
			if gone {
				continue
			}
			for dz := 0; dz < fh; dz++ {
				for dx := 0; dx < fw; dx++ {
					if (dx == 0 && dz == 0) || g.Blocking[(cz+dz)*g.CellW+cx+dx] == 2 {
						out = append(out, image.Pt(cx+dx, cz+dz))
					}
				}
			}
		}
	}
	return out
}

// resampleTerrain scales the cropped picture to w x h, averaging the
// picture pixels under each output pixel, then desaturates and darkens it.
func resampleTerrain(pic *image.RGBA, crop image.Rectangle, scale float64, w, h int) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	n := min(8, max(1, int(math.Ceil(1/scale))))
	inv := 1 / float64(n*n)
	b := pic.Rect
	for oy := 0; oy < h; oy++ {
		for ox := 0; ox < w; ox++ {
			var r, g, bl float64
			for sy := 0; sy < n; sy++ {
				py := crop.Min.Y + int((float64(oy)+(float64(sy)+0.5)/float64(n))/scale)
				py = min(max(py, b.Min.Y), b.Max.Y-1)
				for sx := 0; sx < n; sx++ {
					px := crop.Min.X + int((float64(ox)+(float64(sx)+0.5)/float64(n))/scale)
					px = min(max(px, b.Min.X), b.Max.X-1)
					p := pic.Pix[pic.PixOffset(px, py):]
					r, g, bl = r+float64(p[0]), g+float64(p[1]), bl+float64(p[2])
				}
			}
			r, g, bl = r*inv, g*inv, bl*inv
			l := 0.299*r + 0.587*g + 0.114*bl
			o := img.Pix[img.PixOffset(ox, oy):]
			o[0] = uint8((r + (l-r)*terrainDesaturate) * terrainDim)
			o[1] = uint8((g + (l-g)*terrainDesaturate) * terrainDim)
			o[2] = uint8((bl + (l-bl)*terrainDesaturate) * terrainDim)
			o[3] = 255
		}
	}
	return img
}

// drawStructures draws the structures standing at tick t, or every
// structure that stands at any time in the window when all is set.
func (sc *scene) drawStructures(c *canvas, t float64, all bool) {
	for _, s := range sc.structs {
		if (all && !s.standsDuring(sc.snip.T0, sc.snip.T1)) || (!all && !s.aliveAt(t)) {
			continue
		}
		p := sc.toPanel(s.x, s.z)
		hw, hh := s.w*sc.scale/2, s.h*sc.scale/2
		c.fillRect(p.x-hw, p.y-hh, p.x+hw, p.y+hh, structFill)
		c.strokeRect(p.x-hw+0.5, p.y-hh+0.5, p.x+hw-0.5, p.y+hh-0.5, 1, structLine)
	}
}

// goalRadius is the radius of a goal ring, a little under half a unit.
func (sc *scene) goalRadius(id int) float64 {
	w, h := sc.footprint(sc.names[id])
	return max(5, 0.42*math.Min(w, h)*sc.scale)
}

// drawGoals draws each subject's goal as a ring in its colour, labelled
// with its unit ID so identity does not rest on colour alone.
func (sc *scene) drawGoals(c *canvas) {
	for _, g := range sc.goals {
		p := sc.toPanel(g.x, g.z)
		r := sc.goalRadius(g.id)
		c.ring(p.x, p.y, r, 4.2, halo.alpha(0.75))
		c.ring(p.x, p.y, r, 2.2, sc.colours[g.id])
	}
	for _, l := range sc.goalLabels() {
		c.textHalo(l.at.Min.X, l.at.Min.Y, l.text, textScale, sc.colours[l.id], halo)
	}
}

type goalLabel struct {
	id   int
	text string
	at   image.Rectangle // panel pixels
}

// goalLabels places each goal's label beside its ring: at the first of
// eight places around it that overlaps no ring and no label placed before,
// or where it overlaps least.
func (sc *scene) goalLabels() []goalLabel {
	rings := make([]image.Rectangle, len(sc.goals))
	for i, g := range sc.goals {
		p, r := sc.toPanel(g.x, g.z), sc.goalRadius(g.id)
		rings[i] = image.Rect(int(p.x-r-2), int(p.y-r-2), int(math.Ceil(p.x+r+2)), int(math.Ceil(p.y+r+2)))
	}
	area := func(r image.Rectangle) int { return r.Dx() * r.Dy() }
	var out []goalLabel
	for i, g := range sc.goals {
		text := strconv.Itoa(g.id)
		w, h := textWidth(text, textScale), capH
		p, r := sc.toPanel(g.x, g.z), sc.goalRadius(g.id)
		gap := r + 4
		cands := []pt{
			{p.x + gap, p.y - float64(h)/2}, {p.x - gap - float64(w), p.y - float64(h)/2},
			{p.x - float64(w)/2, p.y - gap - float64(h)}, {p.x - float64(w)/2, p.y + gap},
			{p.x + gap*0.7, p.y - gap*0.7 - float64(h)}, {p.x - gap*0.7 - float64(w), p.y - gap*0.7 - float64(h)},
			{p.x + gap*0.7, p.y + gap*0.7}, {p.x - gap*0.7 - float64(w), p.y + gap*0.7},
		}
		best, bestCost := image.Rectangle{}, -1
		for _, cp := range cands {
			box := image.Rect(int(math.Round(cp.x)), int(math.Round(cp.y)), int(math.Round(cp.x))+w, int(math.Round(cp.y))+h)
			cost := 0
			if !box.In(image.Rect(0, 0, sc.w, sc.h)) {
				cost += area(box) - area(box.Intersect(image.Rect(0, 0, sc.w, sc.h))) + 1
			}
			for j, rb := range rings {
				if j != i {
					cost += area(box.Intersect(rb))
				}
			}
			for _, l := range out {
				cost += 4 * area(box.Inset(-2).Intersect(l.at))
			}
			if bestCost < 0 || cost < bestCost {
				best, bestCost = box, cost
			}
			if cost == 0 {
				break
			}
		}
		out = append(out, goalLabel{id: g.id, text: text, at: best})
	}
	return out
}

// drawTracks draws every unit's path over the whole window: other units
// thin and grey, each subject in its colour with its blocked stretches
// thicker in the warning colour, its starting point and its goal.
func (sc *scene) drawTracks(c *canvas, l *logData) {
	path := func(run []sample) []pt {
		pts := make([]pt, len(run))
		for i, s := range run {
			pts[i] = sc.toPanel(s.x, s.z)
		}
		return pts
	}
	for _, id := range l.ids {
		if !sc.isSubject(id) {
			for _, run := range l.runs(id) {
				c.stroke(path(run), 1.3, otherTrack)
			}
		}
	}
	for _, id := range sc.subjects {
		for _, run := range l.runs(id) {
			pts := path(run)
			c.stroke(pts, 4.2, halo.alpha(0.6))
			c.stroke(pts, 2.2, sc.colours[id])
		}
	}
	// Blocked stretches go over every line so none is hidden.
	for _, id := range sc.subjects {
		for _, run := range l.runs(id) {
			for i := 0; i < len(run); {
				if !run[i].blocked {
					i++
					continue
				}
				j := i
				for j+1 < len(run) && run[j+1].blocked {
					j++
				}
				pts := path(run[i : j+1])
				c.stroke(pts, 7.5, halo.alpha(0.7))
				c.stroke(pts, 5, warn)
				i = j + 1
			}
		}
	}
	for _, id := range sc.subjects {
		if s := l.series[id]; len(s) > 0 {
			p := sc.toPanel(s[0].x, s[0].z)
			c.disc(p.x, p.y, 4.6, halo.alpha(0.85))
			c.disc(p.x, p.y, 3.3, sc.colours[id])
		}
	}
	sc.drawGoals(c)
}

// drawUnits draws every unit of the log at tick t as a square the size of
// its footprint, with a tick mark toward its heading: other units grey,
// subjects in their colours above them, and blocked units ringed in the
// warning colour.
func (sc *scene) drawUnits(c *canvas, l *logData, t float64) {
	for pass := 0; pass < 2; pass++ {
		for _, id := range l.ids {
			if sc.isSubject(id) != (pass == 1) {
				continue
			}
			if s, ok := l.at(id, t); ok {
				sc.drawUnit(c, id, s)
			}
		}
	}
}

func (sc *scene) drawUnit(c *canvas, id int, s state) {
	fw, fh := sc.footprint(sc.names[id])
	fill := otherUnit
	if col, ok := sc.colours[id]; ok {
		fill = col
	}
	unitShape(c, sc.toPanel(s.x, s.z), fw*sc.scale/2, fh*sc.scale/2, fill, s.blocked, s.heading)
}

// unitShape draws a unit's square of half-size hw x hh centred on p, with
// its heading mark.
func unitShape(c *canvas, p pt, hw, hh float64, fill rgba, blocked bool, heading uint16) {
	c.fillRect(p.x-hw, p.y-hh, p.x+hw, p.y+hh, fill)
	c.strokeRect(p.x-hw, p.y-hh, p.x+hw, p.y+hh, 1.2, halo.alpha(0.85))
	if blocked {
		c.strokeRect(p.x-hw-1.6, p.y-hh-1.6, p.x+hw+1.6, p.y+hh+1.6, 2.2, warn)
	}
	// Heading 0 faces north (smaller z) and a quarter turn faces west. The
	// mark is dark over the square and continues past its edge in the
	// square's colour, so it reads on both.
	a := float64(heading) / 65536 * 2 * math.Pi
	dx, dy := -math.Sin(a), -math.Cos(a)
	r := math.Min(hw, hh)
	c.stroke([]pt{p, {p.x + dx*r*0.95, p.y + dy*r*0.95}}, 1.7, halo.alpha(0.9))
	c.stroke([]pt{{p.x + dx*(r+1.5), p.y + dy*(r+1.5)}, {p.x + dx*(r+5), p.y + dy*(r+5)}}, 1.7, fill)
}
