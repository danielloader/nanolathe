// Command path-lab-render draws what a pathfinding-laboratory snippet's units
// did, so a reviewer can judge a replay's movement against the recorded game
// by eye: the recorded log beside replays of the same snippet under named
// rule sets, all on the ground they moved over.
//
//	path-lab-render tracks|sheet|video|pack [flags]
//
// It reads a snippet (tools/path-lab snippet), one frames file per log
// (tools/path-lab frames), a map directory (nanolathe-pathlab map) and,
// optionally, the unit table and a score file (tools/path-lab score -out).
// tracks draws every path over the window, sheet a grid of moments, video
// the moments animated, and pack a compact bundle for the HTML player in
// player/. Everything it reads and writes is local research data that never
// enters the repository; README.md describes the outputs.
package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func main() {
	if len(os.Args) < 2 {
		usage()
	}
	var err error
	switch os.Args[1] {
	case "tracks":
		err = cmdTracks(os.Args[2:])
	case "sheet":
		err = cmdSheet(os.Args[2:])
	case "video":
		err = cmdVideo(os.Args[2:])
	case "pack":
		err = cmdPack(os.Args[2:])
	default:
		usage()
	}
	if err != nil {
		msg := err.Error()
		if !strings.HasPrefix(msg, "nanolathe: ") {
			msg = "nanolathe: path-lab-render: " + msg
		}
		fmt.Fprintln(os.Stderr, msg)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, `usage: path-lab-render tracks|sheet|video|pack -snippet FILE -map DIR -log LABEL=FRAMES [-log ...] -out FILE [flags]
run a subcommand with -h for its flags`)
	os.Exit(2)
}

// options are the flags every subcommand shares.
type options struct {
	snippet, mapDir, units, score, out string
	logs                               []logSpec
	margin, scale                      float64
	focus                              bool
}

type logSpec struct{ label, path string }

// logList collects repeated -log LABEL=FILE flags in the order given.
type logList []logSpec

func (l *logList) String() string {
	parts := make([]string, len(*l))
	for i, s := range *l {
		parts[i] = s.label + "=" + s.path
	}
	return strings.Join(parts, " ")
}

func (l *logList) Set(v string) error {
	label, path, ok := strings.Cut(v, "=")
	if !ok || label == "" || path == "" {
		return fmt.Errorf("want LABEL=FRAMESFILE, got %q", v)
	}
	*l = append(*l, logSpec{label: label, path: path})
	return nil
}

func newFlags(name string, o *options) *flag.FlagSet {
	fs := flag.NewFlagSet("path-lab-render "+name, flag.ExitOnError)
	fs.StringVar(&o.snippet, "snippet", "", "snippet file written by path-lab snippet (required)")
	fs.StringVar(&o.mapDir, "map", "", "map directory with terrain.png and grid.json.gz (required)")
	fs.StringVar(&o.units, "units", "", "unit table giving footprints; without it every unit is drawn 2x2 cells")
	fs.Var((*logList)(&o.logs), "log", "LABEL=FRAMESFILE, repeated; panels follow the order given (at least one)")
	fs.StringVar(&o.score, "score", "", "score file written by path-lab score -out, for the panel titles")
	fs.Float64Var(&o.margin, "margin", 96, "world units shown around the snippet region")
	fs.BoolVar(&o.focus, "focus", false, "show the ground between the subjects and their goals in place of the snippet region")
	fs.Float64Var(&o.scale, "scale", 0, "output pixels per world unit; 0 makes a panel about 480 pixels wide, within 0.25..2")
	fs.StringVar(&o.out, "out", "", "output file (required)")
	return fs
}

func cmdTracks(args []string) error {
	var o options
	newFlags("tracks", &o).Parse(args)
	if o.out == "" {
		return errors.New("-out is required")
	}
	sc, err := loadScene(&o)
	if err != nil {
		return err
	}
	return writePNG(o.out, renderTracks(sc))
}

func cmdSheet(args []string) error {
	var o options
	fs := newFlags("sheet", &o)
	ticks := fs.String("ticks", "", "comma-separated moments: recording ticks, +ticks after t0, or seconds after t0 like 10s")
	frames := fs.Int("frames", 4, "moments spread evenly over the window when -ticks is not given")
	fs.Parse(args)
	if o.out == "" {
		return errors.New("-out is required")
	}
	sc, err := loadScene(&o)
	if err != nil {
		return err
	}
	ts, err := sheetTicks(*ticks, *frames, sc.snip.T0, sc.snip.T1)
	if err != nil {
		return err
	}
	return writePNG(o.out, renderSheet(sc, ts))
}

func cmdVideo(args []string) error {
	var o options
	fs := newFlags("video", &o)
	fps := fs.Float64("fps", 30, "video frames per second (15 for a .gif when not given)")
	speed := fs.Float64("speed", 4, "game seconds per video second")
	fs.Parse(args)
	ext := strings.ToLower(filepath.Ext(o.out))
	if ext != ".mp4" && ext != ".gif" {
		return errors.New("-out must name a .mp4 or a .gif file")
	}
	gif := ext == ".gif"
	if gif && !flagSet(fs, "fps") {
		*fps = 15
	}
	if *fps <= 0 || *speed <= 0 {
		return fmt.Errorf("-fps and -speed must be positive")
	}
	sc, err := loadScene(&o)
	if err != nil {
		return err
	}
	v := newVideo(sc, *fps, *speed)
	if gif {
		return v.writeGIF(o.out)
	}
	return v.writeMP4(o.out)
}

func cmdPack(args []string) error {
	var o options
	fs := newFlags("pack", &o)
	js := fs.String("js", "", "write the bundle as a script that pushes it onto window.PATHLAB_BUNDLES, for pages opened from disk")
	quality := fs.Int("quality", 70, "JPEG quality of the background, 1..100")
	fs.Parse(args)
	if o.out == "" && *js == "" {
		return errors.New("-out or -js is required")
	}
	if *quality < 1 || *quality > 100 {
		return fmt.Errorf("-quality %d: want 1..100", *quality)
	}
	sc, err := loadScene(&o)
	if err != nil {
		return err
	}
	b, err := buildBundle(sc, *quality)
	if err != nil {
		return err
	}
	if o.out != "" {
		if err := writeBundleJSON(o.out, b); err != nil {
			return err
		}
	}
	if *js != "" {
		return writeBundleJS(*js, b)
	}
	return nil
}

// flagSet reports whether the flag was given on the command line.
func flagSet(fs *flag.FlagSet, name string) bool {
	set := false
	fs.Visit(func(f *flag.Flag) {
		if f.Name == name {
			set = true
		}
	})
	return set
}
