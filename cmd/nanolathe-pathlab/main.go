// Command nanolathe-pathlab is the engine half of the pathfinding
// laboratory: it writes the unit and map tables the offline miner reads and
// replays recorded snippets under a gameplay rule set
// (docs/PATHFINDING_LAB.md). It is a developer tool; nothing it writes is
// read by the game.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/nanolathe-gg/nanolathe/internal/pathlab"
	_ "github.com/nanolathe-gg/nanolathe/mods"
)

type rootList []string

func (r *rootList) String() string     { return fmt.Sprint(*r) }
func (r *rootList) Set(v string) error { *r = append(*r, v); return nil }

func main() {
	if len(os.Args) < 2 {
		usage()
	}
	var err error
	switch os.Args[1] {
	case "units":
		err = cmdUnits(os.Args[2:])
	case "maps":
		err = cmdMaps(os.Args[2:])
	case "replay":
		err = cmdReplay(os.Args[2:])
	case "map":
		err = cmdMap(os.Args[2:])
	default:
		usage()
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: nanolathe-pathlab units|maps|map|replay [flags]")
	os.Exit(2)
}

func contentFlags(fs *flag.FlagSet) (*rootList, *string) {
	var roots rootList
	fs.Var(&roots, "root", "content root; repeat in load order; omitted uses $NANOLATHE_TA_ROOT or installation discovery")
	mod := fs.String("mod", "", "installed mod to mount as the last content root, <id> or <id>@<version>")
	return &roots, mod
}

func writeJSON(path string, v any) error {
	b, err := json.MarshalIndent(v, "", " ")
	if err != nil {
		return err
	}
	if path == "" || path == "-" {
		_, err = os.Stdout.Write(append(b, '\n'))
		return err
	}
	return os.WriteFile(path, append(b, '\n'), 0o644)
}

func cmdUnits(args []string) error {
	fs := flag.NewFlagSet("units", flag.ExitOnError)
	roots, mod := contentFlags(fs)
	out := fs.String("out", "-", "output file")
	fs.Parse(args)
	c, err := pathlab.LoadContent(*roots, *mod)
	if err != nil {
		return err
	}
	defer c.Close()
	return writeJSON(*out, pathlab.UnitRows(c.Catalog))
}

func cmdMaps(args []string) error {
	fs := flag.NewFlagSet("maps", flag.ExitOnError)
	roots, mod := contentFlags(fs)
	out := fs.String("out", "-", "output file")
	fs.Parse(args)
	c, err := pathlab.LoadContent(*roots, *mod)
	if err != nil {
		return err
	}
	defer c.Close()
	return writeJSON(*out, pathlab.MapNames(c.Catalog))
}

// cmdReplay replays snippets under one or more rule sets and writes, for
// each pair, the movement log and optionally the frames.
func cmdReplay(args []string) error {
	fs := flag.NewFlagSet("replay", flag.ExitOnError)
	roots, mod := contentFlags(fs)
	rules := fs.String("rules", "strict-3.1,modern", "comma-separated gameplay rule sets")
	out := fs.String("out", "", "output directory (required)")
	frames := fs.Int("frames", 0, "record unit positions every N ticks, 0 for none")
	timing := fs.Bool("timing", true, "record per-tick wall time")
	extra := fs.Int("extra-ticks", 0, "ticks to run past each snippet's window")
	seed := fs.Uint("seed", 7, "seed for both deterministic streams")
	jitters := fs.Int("jitters", 1, "replay each pair this many times, under jitters 0..N-1")
	fs.Parse(args)
	if *out == "" || fs.NArg() == 0 {
		return fmt.Errorf("nanolathe: replay needs -out and snippet files: logical path <command line>, providers searched [], expected -out DIR and one or more snippet files")
	}
	if err := os.MkdirAll(*out, 0o755); err != nil {
		return err
	}
	loaded := map[string]*pathlab.Content{}
	defer func() {
		for _, c := range loaded {
			c.Close()
		}
	}()
	for _, path := range fs.Args() {
		s, err := pathlab.ReadSnippet(path)
		if err != nil {
			return err
		}
		m := *mod
		if m == "" && s.Content == "prota" {
			m = "prota"
		}
		c := loaded[m]
		if c == nil {
			if c, err = pathlab.LoadContent(*roots, m); err != nil {
				return err
			}
			loaded[m] = c
		}
		for _, rule := range strings.Split(*rules, ",") {
			for j := 0; j < max(1, *jitters); j++ {
				rule = strings.TrimSpace(rule)
				res, err := pathlab.Replay(c, s, pathlab.ReplayOptions{Rules: rule, Seed: uint32(*seed), Timing: *timing, Frames: *frames, ExtraTicks: *extra, Jitter: j})
				if err != nil {
					return fmt.Errorf("%s under %s: %w", s.ID, rule, err)
				}
				base := filepath.Join(*out, s.ID+"__"+rule)
				if j > 0 {
					base += fmt.Sprintf("__j%d", j)
				}
				if err := pathlab.WriteJSON(base+".moves.json.gz", res.Log); err != nil {
					return err
				}
				if *frames > 0 {
					if err := pathlab.WriteJSON(base+".frames.json.gz", res.Frames); err != nil {
						return err
					}
				}
				e := res.Log.Engine
				fmt.Fprintf(os.Stderr, "%s %s j%d: last tick %d, overlap unit-ticks %d, nudged %d, dropped %d\n", s.ID, rule, j, res.Log.LastTick, e.OverlapTicks, e.Nudged, e.Dropped)
			}
		}
	}
	return nil
}

// cmdMap writes a map's terrain picture and static passability grid.
func cmdMap(args []string) error {
	fs := flag.NewFlagSet("map", flag.ExitOnError)
	roots, mod := contentFlags(fs)
	out := fs.String("out", "", "output directory; each map gets a subdirectory (required)")
	gridOnly := fs.Bool("grid-only", false, "write the passability grid without the terrain picture")
	all := fs.Bool("all", false, "export every installed map")
	fs.Parse(args)
	if *out == "" || (fs.NArg() == 0 && !*all) {
		return fmt.Errorf("nanolathe: map needs -out and map names: logical path <command line>, providers searched [], expected -out DIR and one or more map names")
	}
	c, err := pathlab.LoadContent(*roots, *mod)
	if err != nil {
		return err
	}
	defer c.Close()
	names := fs.Args()
	if *all {
		names = pathlab.MapNames(c.Catalog)
	}
	for _, name := range names {
		dir := filepath.Join(*out, strings.ReplaceAll(strings.ToLower(name), " ", "_"))
		g, err := pathlab.ExportMap(c, name, dir, !*gridOnly)
		if err != nil {
			if *all {
				fmt.Fprintf(os.Stderr, "%s: %v\n", name, err)
				continue
			}
			return err
		}
		fmt.Fprintf(os.Stderr, "%s: %dx%d cells, picture %dx%d, %d classes\n", g.Map, g.CellW, g.CellH, g.ImageW, g.ImageH, len(g.Classes))
	}
	return nil
}
