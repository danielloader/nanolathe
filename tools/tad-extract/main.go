// Command tad-extract reads TA Demo Recorder recordings (.tad, and the
// ProTA-renamed .pro) and writes a compact per-recording extract of where and
// when each player started building each unit, with completion and death
// times. It exists so a local recording mirror can be pruned without losing
// the position data only the raw recordings hold. The format is implemented
// from research/formats/tad.md [fmt tad]; README.md covers usage, what is
// decoded, validation results and known gaps.
//
// Recordings and extracts are local research data and never enter the
// repository. Chat subpackets and sectors are skipped without decoding, and
// lobby-address sectors are never read.
package main

import (
	"compress/gzip"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

func main() {
	home, _ := os.UserHomeDir()
	root := flag.String("root", filepath.Join(home, "ta-demos"), "mirror root holding v<version>/ and tada/files/")
	out := flag.String("out", "", "extract directory (default <root>/extract)")
	workers := flag.Int("workers", 2, "parallel recordings (at most 4)")
	force := flag.Bool("force", false, "re-extract recordings whose extract is current")
	dump := flag.String("dump", "", "decode one recording and print its setup, statistics and first builds")
	validate := flag.Bool("validate", false, "compare extracts with the archive's build orders and write validation.json")
	moves := flag.Bool("moves", false, "write movement extracts (builds joined with unit-sync movement) for the listed recordings or sources")
	movesOut := flag.String("moves-out", "", "movement extract directory (default <root>/extract-moves)")
	flag.StringVar(&grammarTable, "grammar-table", "", "unit summary giving canfly by unit name for -moves (default <root>/derived/unit_table.json)")
	pathEvidence := flag.Bool("path-evidence", false, "write nameless, conservatively admitted path evidence for explicit recording paths")
	evidenceOut := flag.String("evidence-out", "", "required output directory for -path-evidence")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "usage: tad-extract [flags] [source ...]\n\nsources are version directories (4.8 or v4.8) and tada; default all.\n\n")
		flag.PrintDefaults()
	}
	flag.Parse()
	if *out == "" {
		*out = filepath.Join(*root, "extract")
	}
	*workers = max(1, min(4, *workers))
	var err error
	switch {
	case *pathEvidence:
		if *dump != "" || *moves || *validate || *evidenceOut == "" || len(flag.Args()) == 0 {
			err = fmt.Errorf("path evidence requires -evidence-out and explicit recording paths, without other modes")
		} else {
			err = runPathEvidence(*root, *evidenceOut, flag.Args())
		}
	case *evidenceOut != "":
		err = fmt.Errorf("-evidence-out requires -path-evidence")
	case *dump != "":
		err = dumpOne(*root, *dump)
	case *moves:
		if *movesOut == "" {
			*movesOut = filepath.Join(*root, "extract-moves")
		}
		err = runMoves(*root, *movesOut, flag.Args())
	case *validate:
		err = runValidate(*root, *out, flag.Args())
	default:
		err = runExtract(*root, *out, flag.Args(), *workers, *force)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "nanolathe: tad-extract:", err)
		os.Exit(1)
	}
}

// extractPlayer is one participant with its build events. Name is kept
// only so the local extract can be joined to archive listings that carry no
// transport identity; DPID joins the archive's decoded player views.
type extractPlayer struct {
	Number int           `json:"number"`
	Side   int           `json:"side"`
	Color  int           `json:"color"`
	Name   string        `json:"name"`
	DPID   uint32        `json:"dpid"`
	Block  *int          `json:"block,omitempty"`
	Builds []*buildEvent `json:"builds"`
}

// extract is the per-recording output file.
type extract struct {
	Source      string          `json:"source"`
	ID          string          `json:"id"`
	File        string          `json:"file"`
	SourceBytes int64           `json:"source_bytes"`
	SourceMtime int64           `json:"source_mtime"`
	TablesSig   string          `json:"tables_sig"`
	Status      string          `json:"status"`
	Error       string          `json:"error,omitempty"`
	Naming      naming          `json:"naming"`
	Setup       *setup          `json:"setup,omitempty"`
	Players     []extractPlayer `json:"players"`
	Decode      *decodeStats    `json:"decode,omitempty"`
	Extract     *extractStats   `json:"extract,omitempty"`
	Unknown09   map[string]int  `json:"unknown_09_distinct,omitempty"`
	Seconds     float64         `json:"seconds"`
}

func tablesSig(ts []*unitTable) string {
	var s []string
	for _, t := range ts {
		s = append(s, t.Sig)
	}
	return strings.Join(s, ";")
}

func extractFile(path string, tables []*unitTable) (*extract, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	fi, err := f.Stat()
	if err != nil {
		return nil, err
	}
	t0 := time.Now()
	ex := &extract{File: filepath.Base(path), SourceBytes: fi.Size(), SourceMtime: fi.ModTime().Unix(), TablesSig: tablesSig(tables)}
	var x *extractor
	su, st, derr := decode(f, func(ctx subContext, sp []byte) {
		if x != nil {
			x.onSub(ctx, sp)
		}
	}, func(su *setup) { x = newExtractor(su) })
	ex.Setup, ex.Decode = su, st
	switch {
	case derr != nil:
		ex.Status, ex.Error = "failed", derr.Error()
	case st.End != "eof":
		ex.Status = "truncated"
	default:
		ex.Status = "ok"
	}
	ex.Players = []extractPlayer{}
	if x == nil {
		ex.Naming = naming{Reason: "setup not decoded"}
		ex.Seconds = time.Since(t0).Seconds()
		return ex, nil
	}
	ex.Extract = &x.st
	ex.Unknown09 = map[string]int{"offset5_u16": len(x.unk5), "offset19_u32": len(x.unk19)}
	byNum := map[int]int{}
	for _, p := range su.Players {
		ep := extractPlayer{Number: p.Number, Side: p.Side, Color: p.Color, Name: p.Name, Builds: []*buildEvent{}}
		for _, s := range su.Status {
			if s.Number == p.Number && s.Error == "" {
				ep.DPID = s.DPID
			}
		}
		if r, ok := x.blockByNum[p.Number]; ok {
			ep.Block = &r
		}
		byNum[p.Number] = len(ex.Players)
		ex.Players = append(ex.Players, ep)
	}
	for _, ev := range x.events {
		if i, ok := byNum[ev.sender]; ok {
			ex.Players[i].Builds = append(ex.Players[i].Builds, ev)
		}
	}
	t, nm := chooseTable(su, ex.Players, tables)
	ex.Naming = nm
	if t != nil {
		for _, ev := range x.events {
			if ev.Type >= 1 && ev.Type <= len(t.Names) {
				ev.Unit = t.Names[ev.Type-1]
			}
		}
	}
	ex.Seconds = time.Since(t0).Seconds()
	return ex, nil
}

func dumpOne(root, path string) error {
	d := demoForPath(root, path)
	ex, err := extractFile(path, d.tables)
	if err != nil {
		return err
	}
	// Names stay out of the terminal dump; the extract file keeps them.
	for i := range ex.Players {
		ex.Players[i].Name = ""
		if len(ex.Players[i].Builds) > 12 {
			ex.Players[i].Builds = ex.Players[i].Builds[:12]
		}
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", " ")
	return enc.Encode(ex)
}

// current reports whether an extract exists for this source size, mtime and
// candidate unit tables.
func current(path string, d demo) bool {
	src, err := os.Stat(d.path)
	if err != nil {
		return false
	}
	ex, err := readExtract(path)
	if err != nil {
		return false
	}
	return ex.SourceBytes == src.Size() && ex.SourceMtime == src.ModTime().Unix() && ex.TablesSig == tablesSig(d.tables)
}

func readExtract(path string) (*extract, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	zr, err := gzip.NewReader(f)
	if err != nil {
		return nil, err
	}
	var ex extract
	if err := json.NewDecoder(zr).Decode(&ex); err != nil {
		return nil, err
	}
	return &ex, nil
}

func writeExtract(path string, ex *extract) error { return writeJSONGz(path, ex) }

// writeJSONGz writes v as gzip-compressed JSON, atomically by rename.
func writeJSONGz(path string, v any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	zw, _ := gzip.NewWriterLevel(f, gzip.BestCompression)
	if err := json.NewEncoder(zw).Encode(v); err != nil {
		f.Close()
		return err
	}
	if err := zw.Close(); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func runExtract(root, out string, sources []string, workers int, force bool) error {
	demos, err := listDemos(root, sources)
	if err != nil {
		return err
	}
	type result struct {
		d      demo
		bytes  int64
		status string
		err    error
	}
	jobs := make(chan demo)
	results := make(chan result)
	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for d := range jobs {
				p := extractPath(out, d)
				if !force && current(p, d) {
					results <- result{d: d, status: "current"}
					continue
				}
				ex, err := extractFile(d.path, d.tables)
				if err != nil {
					results <- result{d: d, err: err}
					continue
				}
				ex.Source, ex.ID = d.source, d.id
				if err := writeExtract(p, ex); err != nil {
					results <- result{d: d, err: err}
					continue
				}
				results <- result{d: d, bytes: ex.SourceBytes, status: ex.Status}
			}
		}()
	}
	go func() {
		for _, d := range demos {
			jobs <- d
		}
		close(jobs)
		wg.Wait()
		close(results)
	}()
	t0 := time.Now()
	var done, fresh int
	var freshBytes int64
	counts := map[string]int{}
	for r := range results {
		done++
		if r.err != nil {
			counts["error"]++
			fmt.Fprintf(os.Stderr, "%s/%s: %v\n", r.d.source, r.d.id, r.err)
			continue
		}
		counts[r.status]++
		if r.status != "current" {
			fresh++
			freshBytes += r.bytes
		}
		if done%250 == 0 {
			fmt.Fprintf(os.Stderr, "%d/%d %v\n", done, len(demos), counts)
		}
	}
	el := time.Since(t0).Seconds()
	run := runInfo{
		Workers: workers, Recordings: len(demos), Extracted: fresh, Seconds: el,
		MBRead: float64(freshBytes) / 1e6, Results: counts, Finished: time.Now().UTC().Format(time.RFC3339),
	}
	if el > 0 {
		run.MBPerSecond = run.MBRead / el
	}
	fmt.Fprintf(os.Stderr, "%d recordings %v: %.0f MB decoded in %.1fs (%.0f MB/s, %d workers)\n",
		len(demos), counts, run.MBRead, el, run.MBPerSecond, workers)
	return writeSummary(out, demos, run)
}
