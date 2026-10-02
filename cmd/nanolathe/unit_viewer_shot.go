package main

import (
	"fmt"
	"image"
	"strings"

	"github.com/hajimehoshi/ebiten/v2"
)

// This uses the shipped screen and geometry path, including clipping at the
// requested window size. It creates no session and writes no preferences.
func runUnitViewerShot(opts Options, cs *contentSet) error {
	size := opts.ShotSize
	if size == "" {
		size = "1440x900"
	}
	w, h, err := parseNLShotSize(size)
	if err != nil || w > 4096 || h > 4096 {
		return fmt.Errorf("nanolathe: unit viewer capture size: logical path <command line>, providers searched [--shot-size], expected WxH within 320x240..4096x4096")
	}
	s := &toolsScreen{}
	s.show(&gameShell{cs: cs})
	defer s.release()
	if opts.ShotUnitViewer != "@tools" {
		cat, err := cs.nlPreviewCatalog()
		if err != nil {
			return err
		}
		s.viewer, s.entries = true, unitViewerEntries(cat)
		s.buildPanel()
		s.filter()
		found := false
		for _, entry := range s.entries {
			if strings.EqualFold(entry.Key, opts.ShotUnitViewer) || strings.EqualFold(entry.Def.UnitName, opts.ShotUnitViewer) {
				s.selectUnit(entry.Def)
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("nanolathe: unit viewer capture: logical path units/%s, providers searched [compiled catalog], expected unit ID", opts.ShotUnitViewer)
		}
	}
	g := &unitViewerShotGame{s: s, w: w, h: h, out: opts.Shot}
	ebiten.SetWindowVisible(false)
	ebiten.SetWindowSize(640, 480)
	if err := ebiten.RunGame(g); err != nil {
		return err
	}
	return g.err
}

type unitViewerShotGame struct {
	s    *toolsScreen
	w, h int
	out  string
	done bool
	err  error
}

func (g *unitViewerShotGame) Update() error {
	if g.done {
		return ebiten.Termination
	}
	return nil
}
func (g *unitViewerShotGame) Layout(int, int) (int, int) { return g.w, g.h }
func (g *unitViewerShotGame) Draw(dst *ebiten.Image) {
	if g.done {
		return
	}
	g.s.Draw(dst)
	// The first draw measures how many rows fit. Reveal the subject using
	// those measured rows, exactly as a keyboard selection does.
	g.s.revealSelection()
	g.s.Draw(dst)
	img := image.NewRGBA(image.Rect(0, 0, g.w, g.h))
	dst.ReadPixels(img.Pix)
	g.err = encodeShotPNG(g.out, img)
	if g.err == nil {
		g.err = g.s.model.err
	}
	g.done = true
}
