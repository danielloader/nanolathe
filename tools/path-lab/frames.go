package main

import (
	"flag"
	"math"
	"os"
)

// frame is every staged unit's state at one tick, in the shape the engine's
// replay runner writes: id, x, z, heading, blocked, moving.
type frame struct {
	Tick  int32      `json:"tick"`
	Units [][6]int32 `json:"units"`
}

// headingOf converts a travel direction to the engine's heading word: zero
// faces north (toward smaller z), a quarter turn faces west.
func headingOf(hx, hz float64) int32 {
	if hx == 0 && hz == 0 {
		return 32768
	}
	a := math.Atan2(-hx, -hz)
	if a < 0 {
		a += 2 * math.Pi
	}
	return int32(a/(2*math.Pi)*65536) & 0xffff
}

// cmdFrames writes the reconstructed positions of a snippet's units in a
// log, sampled every few ticks, so a recording can be drawn beside a replay.
func cmdFrames(args []string) error {
	fs := flag.NewFlagSet("frames", flag.ExitOnError)
	home, _ := os.UserHomeDir()
	snip := fs.String("snippet", "", "snippet file (required)")
	moves := fs.String("moves", "", "movement extract or replay log (required)")
	units := fs.String("units", home+"/nanolathe-bench/path-redesign/data/units-ota.json", "unit table for original TA")
	unitsProTA := fs.String("units-prota", home+"/nanolathe-bench/path-redesign/data/units-prota.json", "unit table for ProTA")
	every := fs.Int("every", 5, "ticks between frames")
	out := fs.String("out", "", "output file (required)")
	fs.Parse(args)
	var s snippet
	if err := readJSONMaybeGz(*snip, &s); err != nil {
		return err
	}
	var tabs tables
	var err error
	if tabs.ota, err = loadUnitTable(*units); err != nil {
		return err
	}
	if tabs.prota, err = loadUnitTable(*unitsProTA); err != nil {
		return err
	}
	r, err := loadRecording(*moves, tabs)
	if err != nil {
		return err
	}
	type member struct {
		id int
		u  *unitTL
	}
	var ms []member
	for _, su := range s.Units {
		if su.Structure {
			continue
		}
		for _, u := range r.Units {
			if u.Net == su.ID && u.Ground && u.alive(su.Spawn) {
				ms = append(ms, member{su.ID, u})
				break
			}
		}
	}
	var frames []frame
	for t := s.Start; t <= s.T1; t += int32(*every) {
		f := frame{Tick: t, Units: [][6]int32{}}
		for _, m := range ms {
			if !m.u.alive(t) {
				continue
			}
			x, z, moving, _ := m.u.pos(t)
			hx, hz, ok := m.u.heading(t)
			if !ok {
				hx, hz, _ = m.u.lastHeading(t)
			}
			b, mv := int32(0), int32(0)
			if moving {
				mv = 1
				if m.u.blockedAt(t) {
					b = 1
				}
			}
			f.Units = append(f.Units, [6]int32{int32(m.id), int32(math.Round(x)), int32(math.Round(z)), headingOf(hx, hz), b, mv})
		}
		frames = append(frames, f)
	}
	return writeJSONMaybeGz(*out, frames)
}
