package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"image/jpeg"
	"math"
	"os"
	"sort"
)

// A bundle is everything the HTML player needs for one snippet, already
// projected into the pixels of its background picture and rounded to
// integers so it stays small: the background as an embedded JPEG, the
// structures, the subjects' goals, and per log the sampled frames.

type bundle struct {
	ID     string  `json:"id"`
	Map    string  `json:"map"`
	Class  string  `json:"class,omitempty"`
	T0     int32   `json:"t0"`
	T1     int32   `json:"t1"`
	Scale  float64 `json:"scale"`
	Width  int     `json:"width"`
	Height int     `json:"height"`
	// Background is a data URL of the terrain, region outline and blocking
	// features.
	Background string `json:"background"`
	// Structures are [x, y, w, h] with x, y the top-left corner; a
	// structure that does not stand for the whole window adds its first
	// tick and the tick it is gone (-1 for never).
	Structures [][]int `json:"structures"`
	// Goals are [unit index, x, y].
	Goals [][3]int     `json:"goals"`
	Units []bundleUnit `json:"units"`
	Logs  []bundleLog  `json:"logs"`
}

type bundleUnit struct {
	ID      int    `json:"id"`
	Name    string `json:"name"`
	Subject bool   `json:"subject"`
	W       int    `json:"w"`
	H       int    `json:"h"`
	// Color is a subject's colour, the one the pictures use.
	Color string `json:"color,omitempty"`
}

type bundleLog struct {
	Label string                     `json:"label"`
	Score map[string]json.RawMessage `json:"score,omitempty"`
	Ticks []int32                    `json:"ticks"`
	// Frames holds per tick [unit index, x, y, heading, flags] for every
	// unit in view, flattened: heading is the 16-bit heading over 256,
	// flags bit 0 blocked and bit 1 moving.
	Frames [][]int `json:"frames"`
}

func buildBundle(sc *scene, quality int) (*bundle, error) {
	var jpg bytes.Buffer
	if err := jpeg.Encode(&jpg, sc.base, &jpeg.Options{Quality: quality}); err != nil {
		return nil, err
	}
	b := &bundle{ID: sc.snip.ID, Map: sc.snip.Map, Class: sc.snip.Class, T0: sc.snip.T0, T1: sc.snip.T1,
		Scale: math.Round(sc.scale*1e4) / 1e4, Width: sc.w, Height: sc.h,
		Background: "data:image/jpeg;base64," + base64.StdEncoding.EncodeToString(jpg.Bytes()),
		Structures: [][]int{}, Goals: [][3]int{}}
	for _, s := range sc.structs {
		if !s.standsDuring(sc.snip.T0, sc.snip.T1) {
			continue
		}
		p := sc.toPanel(s.x, s.z)
		w, h := s.w*sc.scale, s.h*sc.scale
		if p.x+w/2 < 0 || p.y+h/2 < 0 || p.x-w/2 > float64(sc.w) || p.y-h/2 > float64(sc.h) {
			continue // wholly outside the picture
		}
		e := []int{round(p.x - w/2), round(p.y - h/2), round(w), round(h)}
		if s.spawn > sc.snip.T0 || s.die >= 0 {
			e = append(e, int(s.spawn), int(s.die))
		}
		b.Structures = append(b.Structures, e)
	}
	// Units: the subjects in colour order, then every other unit a log
	// shows, by ID.
	index := map[int]int{}
	add := func(id int) {
		if _, ok := index[id]; ok {
			return
		}
		index[id] = len(b.Units)
		w, h := sc.footprint(sc.names[id])
		u := bundleUnit{ID: id, Name: sc.names[id], Subject: sc.isSubject(id), W: round(w * sc.scale), H: round(h * sc.scale)}
		if u.Subject {
			u.Color = sc.colours[id].hex()
		}
		b.Units = append(b.Units, u)
	}
	for _, id := range sc.subjects {
		add(id)
	}
	var others []int
	for _, l := range sc.logs {
		others = append(others, l.ids...)
	}
	sort.Ints(others)
	for _, id := range others {
		add(id)
	}
	for _, g := range sc.goals {
		p := sc.toPanel(g.x, g.z)
		b.Goals = append(b.Goals, [3]int{index[g.id], round(p.x), round(p.y)})
	}
	for _, l := range sc.logs {
		bl := bundleLog{Label: l.label, Ticks: l.ticks, Frames: make([][]int, len(l.ticks))}
		if l.score != nil {
			bl.Score = l.score.raw
		}
		at := make(map[int32]int, len(l.ticks))
		for i, t := range l.ticks {
			at[t] = i
			bl.Frames[i] = []int{}
		}
		for _, u := range b.Units {
			// A unit well outside the picture is left out of that frame;
			// the player draws nothing there anyway.
			m := 24 + float64(max(u.W, u.H))
			for _, s := range l.series[u.ID] {
				p := sc.toPanel(s.x, s.z)
				if p.x < -m || p.y < -m || p.x > float64(sc.w)+m || p.y > float64(sc.h)+m {
					continue
				}
				flags := 0
				if s.blocked {
					flags |= 1
				}
				if s.moving {
					flags |= 2
				}
				i := at[s.tick]
				bl.Frames[i] = append(bl.Frames[i], index[u.ID], round(p.x), round(p.y), int(math.Round(float64(s.heading)/256))&255, flags)
			}
		}
		b.Logs = append(b.Logs, bl)
	}
	return b, nil
}

func round(v float64) int {
	return int(math.Round(v))
}

func writeBundleJSON(path string, b *bundle) error {
	data, err := json.Marshal(b)
	if err != nil {
		return err
	}
	return writeFile(path, append(data, '\n'))
}

// writeBundleJS writes the bundle as a script, for pages opened from disk
// where a fetch of a local file is refused.
func writeBundleJS(path string, b *bundle) error {
	data, err := json.Marshal(b)
	if err != nil {
		return err
	}
	var out bytes.Buffer
	out.WriteString("window.PATHLAB_BUNDLES = window.PATHLAB_BUNDLES || [];\nwindow.PATHLAB_BUNDLES.push(")
	out.Write(data)
	out.WriteString(");\n")
	return writeFile(path, out.Bytes())
}

func writeFile(path string, data []byte) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
