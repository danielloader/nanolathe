package main

import (
	"bufio"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// The shapes below mirror what tools/path-lab and `nanolathe-pathlab map`
// write. They are declared here rather than imported so this command stays
// standard-library only and builds without the engine.

type snippet struct {
	Schema    int         `json:"schema"`
	ID        string      `json:"id"`
	Map       string      `json:"map"`
	Content   string      `json:"content"`
	Class     string      `json:"class"`
	T0        int32       `json:"t0"`
	T1        int32       `json:"t1"`
	Region    [4]float64  `json:"region"` // x0, z0, x1, z1 in world units
	Units     []snipUnit  `json:"units"`
	Orders    []snipOrder `json:"orders"`
	Reclaimed [][2]int32  `json:"reclaimed"`
}

type snipUnit struct {
	ID        int     `json:"id"`
	Name      string  `json:"name"`
	X         float64 `json:"x"`
	Z         float64 `json:"z"`
	Structure bool    `json:"structure"`
	Spawn     int32   `json:"spawn"`
	Die       int32   `json:"die"`
	Subject   bool    `json:"subject"`
}

type snipOrder struct {
	Tick  int32   `json:"tick"`
	Unit  int     `json:"unit"`
	GoalX float64 `json:"goal_x"`
	GoalZ float64 `json:"goal_z"`
	Carry bool    `json:"carry"`
}

// frame is every mobile unit's state at one tick: id, x, z, heading,
// blocked, moving.
type frame struct {
	Tick  int32      `json:"tick"`
	Units [][6]int32 `json:"units"`
}

// gridFile is grid.json.gz of a map directory. The per-class passability
// layers are not needed here and are skipped by the decoder.
type gridFile struct {
	Map      string `json:"map"`
	CellW    int    `json:"cell_w"`
	CellH    int    `json:"cell_h"`
	Heights  []byte `json:"heights"`
	Blocking []byte `json:"blocking"`
	ImageW   int    `json:"image_w"`
	ImageH   int    `json:"image_h"`
}

type unitDef struct {
	Name  string `json:"unitname"`
	FootX int    `json:"foot_x"`
	FootZ int    `json:"foot_z"`
}

// scoreAgg is the part of a `path-lab score` entry the panels print.
type scoreAgg struct {
	Log      string  `json:"log"`
	Rules    string  `json:"rules"`
	N        int     `json:"n"`
	Reached  int     `json:"reached"`
	MeanNear float64 `json:"mean_near_ticks"`
	Blocked  int64   `json:"blocked_ticks"`
	Overlap  int64   `json:"overlap_unit_ticks"`
}

// readJSON decodes a JSON file, gzip-compressed or not.
func readJSON(path, product string, v any) error {
	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("nanolathe: %s is not readable: logical path %s, providers searched [file], expected %s: %w", product, path, product, err)
	}
	defer f.Close()
	br := bufio.NewReaderSize(f, 1<<16)
	var r io.Reader = br
	if magic, _ := br.Peek(2); len(magic) == 2 && magic[0] == 0x1f && magic[1] == 0x8b {
		zr, err := gzip.NewReader(br)
		if err != nil {
			return fmt.Errorf("nanolathe: %s is not readable: logical path %s, providers searched [file], expected %s: %w", product, path, product, err)
		}
		defer zr.Close()
		r = zr
	}
	if err := json.NewDecoder(r).Decode(v); err != nil {
		return fmt.Errorf("nanolathe: %s is not readable: logical path %s, providers searched [file], expected %s: %w", product, path, product, err)
	}
	return nil
}

func readSnippet(path string) (*snippet, error) {
	var s snippet
	if err := readJSON(path, "a path-lab snippet", &s); err != nil {
		return nil, err
	}
	if s.Schema != 1 {
		return nil, fmt.Errorf("nanolathe: snippet schema is not supported: logical path %s, providers searched [file], expected schema 1", path)
	}
	if s.T1 < s.T0 || s.Region[2] <= s.Region[0] || s.Region[3] <= s.Region[1] {
		return nil, fmt.Errorf("nanolathe: snippet window or region is empty: logical path %s, providers searched [file], expected t0 <= t1 and a non-empty region", path)
	}
	return &s, nil
}

func readGrid(dir string) (*gridFile, error) {
	path := filepath.Join(dir, "grid.json.gz")
	var g gridFile
	if err := readJSON(path, "a path-lab map grid", &g); err != nil {
		return nil, err
	}
	if g.CellW <= 0 || g.CellH <= 0 || len(g.Heights) != g.CellW*g.CellH || len(g.Blocking) != g.CellW*g.CellH {
		return nil, fmt.Errorf("nanolathe: map grid is inconsistent: logical path %s, providers searched [file], expected one height and one blocking byte per cell", path)
	}
	return &g, nil
}

// readFeet reads a unit table into each unit's footprint in cells.
func readFeet(path string) (map[string][2]int, error) {
	var defs []unitDef
	if err := readJSON(path, "a path-lab unit table", &defs); err != nil {
		return nil, err
	}
	feet := make(map[string][2]int, len(defs))
	for _, d := range defs {
		if d.FootX > 0 && d.FootZ > 0 {
			feet[strings.ToUpper(d.Name)] = [2]int{d.FootX, d.FootZ}
		}
	}
	return feet, nil
}

// readFrames reads a frames file and puts its frames in tick order.
func readFrames(path string) ([]frame, error) {
	var fr []frame
	if err := readJSON(path, "a path-lab frames file", &fr); err != nil {
		return nil, err
	}
	if len(fr) == 0 {
		return nil, fmt.Errorf("nanolathe: frames file is empty: logical path %s, providers searched [file], expected at least one frame", path)
	}
	sort.SliceStable(fr, func(i, j int) bool { return fr[i].Tick < fr[j].Tick })
	for i := 1; i < len(fr); i++ {
		if fr[i].Tick == fr[i-1].Tick {
			return nil, fmt.Errorf("nanolathe: frames file repeats tick %d: logical path %s, providers searched [file], expected one frame per tick", fr[i].Tick, path)
		}
	}
	return fr, nil
}

// scoreEntry is one log's scores: the aggregate for the panel line and the
// raw aggregate fields for the player bundle.
type scoreEntry struct {
	agg scoreAgg
	raw map[string]json.RawMessage
}

func readScores(path string) ([]scoreEntry, error) {
	var f struct {
		Scores []map[string]json.RawMessage `json:"scores"`
	}
	if err := readJSON(path, "a path-lab score file", &f); err != nil {
		return nil, err
	}
	out := make([]scoreEntry, 0, len(f.Scores))
	for _, raw := range f.Scores {
		delete(raw, "subjects") // per-subject detail stays in the score file
		b, err := json.Marshal(raw)
		if err != nil {
			return nil, err
		}
		var e scoreEntry
		if err := json.Unmarshal(b, &e.agg); err != nil {
			return nil, fmt.Errorf("nanolathe: score entry is not readable: logical path %s, providers searched [file], expected path-lab score aggregates: %w", path, err)
		}
		e.raw = raw
		out = append(out, e)
	}
	return out, nil
}

// matchScore finds the score entry for a log: the entry whose log or rule
// set name is the label, else the one whose name the frames file carries
// after the snippet ID (tools/path-lab names them <snippet>__<log>).
func matchScore(scores []scoreEntry, label, framesPath string) *scoreEntry {
	for i := range scores {
		if scores[i].agg.Log == label {
			return &scores[i]
		}
	}
	for i := range scores {
		if scores[i].agg.Rules != "" && scores[i].agg.Rules == label {
			return &scores[i]
		}
	}
	base := filepath.Base(framesPath)
	for i := range scores {
		if l := scores[i].agg.Log; l != "" && strings.Contains(base, "__"+l+".") {
			return &scores[i]
		}
	}
	return nil
}
