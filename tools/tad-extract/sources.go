package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// demo is one mirrored recording ready to extract.
type demo struct {
	source string // "v4.8", "tada", ...
	id     string
	path   string
	tables []*unitTable
}

// Two mirror layouts are read:
//
//   - v<version>/<id>.json names its recording in "file" with its size in
//     "bytes"; v<version>/mod_units.json is the archive's unit table.
//   - tada/files/<party>.json carries the listing and detail records, with
//     the recording's size in detail.fileSize; the recording is
//     <party>.tad (original TA) or <party>.pro (ProTA).
//
// A recording is listed only when its size matches its metadata, so a
// download still in progress is never read.
var versionDir = regexp.MustCompile(`^v(\d+(?:\.\d+)*)$`)

const stockTablePath = "derived/unit_table.json"

// candidateTables lists unit tables that may number a source's recordings,
// in preference order. Original-TA sources also try the engine's stock
// catalog, which older recordings use.
func candidateTables(root, source string, ota bool) []*unitTable {
	var ts []*unitTable
	add := func(rel string) {
		if t, err := loadTable(filepath.Join(root, rel), rel); err == nil {
			ts = append(ts, t)
		}
	}
	if strings.HasPrefix(source, "v") {
		add(filepath.Join(source, "mod_units.json"))
	}
	if ota {
		if source != "v3.1" {
			add(filepath.Join("v3.1", "mod_units.json"))
		}
		add(stockTablePath)
	}
	if source == "tada" && !ota {
		// The predecessor archive's ProTA recordings carry no table of their
		// own. Every mirrored table is a candidate; chooseTable accepts one
		// only when the recording's enabled-key count and commander types
		// agree with it.
		for _, rel := range []string{"v3.1", "v4.4", "v4.5", "v4.6", "v4.7", "v4.8"} {
			add(filepath.Join(rel, "mod_units.json"))
		}
	}
	return ts
}

func listDemos(root string, sources []string) ([]demo, error) {
	want := map[string]bool{}
	for _, s := range sources {
		if s != "tada" && !strings.HasPrefix(s, "v") {
			s = "v" + s
		}
		want[s] = true
	}
	ents, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}
	tables := map[string][]*unitTable{}
	tablesFor := func(source string, ota bool) []*unitTable {
		k := source
		if ota {
			k += "+ota"
		}
		if _, ok := tables[k]; !ok {
			tables[k] = candidateTables(root, source, ota)
		}
		return tables[k]
	}
	var out []demo
	for _, e := range ents {
		if !e.IsDir() || (len(want) > 0 && !want[e.Name()]) {
			continue
		}
		switch {
		case versionDir.MatchString(e.Name()):
			source := e.Name()
			dir := filepath.Join(root, source)
			metas, _ := filepath.Glob(filepath.Join(dir, "*.json"))
			for _, mp := range metas {
				id := strings.TrimSuffix(filepath.Base(mp), ".json")
				if !allDigits(id) {
					continue
				}
				var meta struct {
					File  string `json:"file"`
					Bytes int64  `json:"bytes"`
				}
				if !readJSON(mp, &meta) || meta.File == "" || !sizeIs(filepath.Join(dir, meta.File), meta.Bytes) {
					continue
				}
				out = append(out, demo{source: source, id: id, path: filepath.Join(dir, meta.File),
					tables: tablesFor(source, source == "v3.1")})
			}
		case e.Name() == "tada":
			dir := filepath.Join(root, "tada", "files")
			metas, _ := filepath.Glob(filepath.Join(dir, "*.json"))
			for _, mp := range metas {
				id := strings.TrimSuffix(filepath.Base(mp), ".json")
				var meta struct {
					Detail struct {
						FileSize int64 `json:"fileSize"`
					} `json:"detail"`
				}
				if !readJSON(mp, &meta) {
					continue
				}
				for _, ext := range []string{".tad", ".pro"} {
					p := filepath.Join(dir, id+ext)
					if sizeIs(p, meta.Detail.FileSize) {
						out = append(out, demo{source: "tada", id: id, path: p, tables: tablesFor("tada", ext == ".tad")})
					}
				}
			}
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].source != out[j].source {
			return out[i].source < out[j].source
		}
		if len(out[i].id) != len(out[j].id) {
			return len(out[i].id) < len(out[j].id)
		}
		return out[i].id < out[j].id
	})
	return out, nil
}

// demoForPath describes a single recording named on the command line, with
// the candidate tables its location implies.
func demoForPath(root, path string) demo {
	abs, _ := filepath.Abs(path)
	dir := filepath.Base(filepath.Dir(abs))
	source := dir
	if dir == "files" && filepath.Base(filepath.Dir(filepath.Dir(abs))) == "tada" {
		source = "tada"
	}
	ota := source == "v3.1" || (source == "tada" && strings.EqualFold(filepath.Ext(abs), ".tad"))
	return demo{source: source, id: strings.TrimSuffix(filepath.Base(abs), filepath.Ext(abs)), path: abs,
		tables: candidateTables(root, source, ota)}
}

func extractPath(out string, d demo) string {
	return filepath.Join(out, d.source, d.id+".json.gz")
}

func allDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

func readJSON(path string, v any) bool {
	b, err := os.ReadFile(path)
	return err == nil && json.Unmarshal(b, v) == nil
}

func sizeIs(path string, want int64) bool {
	fi, err := os.Stat(path)
	return err == nil && fi.Mode().IsRegular() && fi.Size() == want && want > 0
}
