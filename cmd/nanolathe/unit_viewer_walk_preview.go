package main

import (
	"fmt"
	"image"
	"path/filepath"
	"strings"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
	"github.com/nanolathe-gg/nanolathe/internal/settings"
)

// This standalone preview uses the shipped viewer and its detached script
// machine. Its presentation clock and comparison choice never enter a battle.
func runUnitViewerWalkPreview(opts Options, cs *contentSet) error {
	size := opts.ShotSize
	if size == "" {
		size = "1440x900"
	}
	w, h, err := parseNLShotSize(size)
	if err != nil || w > 4096 || h > 4096 {
		return fmt.Errorf("nanolathe: walk preview size: logical path <command line>, providers searched [--shot-size], expected WxH within 320x240..4096x4096")
	}
	if opts.WalkPreviewFrames < 1 || opts.WalkPreviewFrames > 3600 || opts.ShotTicks < 0 || opts.ShotTicks > 3600 {
		return fmt.Errorf("nanolathe: walk preview capture: logical path <command line>, providers searched [--walk-preview-frames, --shot-ticks], expected 1..3600 frames and 0..3600 warmup ticks")
	}
	s := &toolsScreen{}
	s.show(&gameShell{cs: cs, opts: opts})
	defer s.release()
	// Even the restriction editor's Apply remains detached from settings.
	s.restrict.host = nil
	s.fieldRender = unitViewerFieldRender(settings.DefaultPresentation(), settings.DefaultDisplay())
	s.layout(float64(w), float64(h))
	cat, err := cs.nlPreviewCatalog()
	if err != nil {
		return err
	}
	s.viewer, s.entries = true, unitViewerEntries(cat)
	s.tree = unitViewerBuildTree(cat, s.entries)
	s.features = cat.Features
	s.restrict.names, s.restrict.keys = unitViewerRestrictNames(cat, s.entries)
	s.model.walkEnabled, s.model.walkSmooth = true, !opts.WalkPreviewOriginal
	s.buildPanel()
	s.filter()
	found := false
	for _, entry := range s.entries {
		if strings.EqualFold(entry.Key, opts.WalkPreview) || strings.EqualFold(entry.Def.UnitName, opts.WalkPreview) {
			s.selectUnit(entry.Def)
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("nanolathe: walk preview: logical path units/%s, providers searched [compiled catalog], expected unit ID", opts.WalkPreview)
	}
	if !s.model.ensureLoaded(cs, s.selected) {
		return s.model.err
	}
	s.activateTool("MOVE")
	s.spinning, s.searchFocus = false, false
	s.panel.SetFocus(-1)
	g := &unitViewerWalkPreviewGame{s: s, w: w, h: h, out: opts.Shot, count: opts.WalkPreviewFrames, warmup: opts.ShotTicks * 4}
	g.updateTitle()
	ebiten.SetTPS(120)
	ebiten.SetWindowSize(w, h)
	ebiten.SetWindowResizingMode(ebiten.WindowResizingModeEnabled)
	if opts.Shot != "" {
		ebiten.SetWindowVisible(false)
	}
	if err := ebiten.RunGame(g); err != nil {
		return err
	}
	return g.err
}

type unitViewerWalkPreviewGame struct {
	s                       *toolsScreen
	w, h                    int
	out                     string
	count, warmup           int
	pictureFrames, captured int
	done                    bool
	err                     error
}

func (g *unitViewerWalkPreviewGame) updateTitle() {
	mode := "Original"
	if g.s.model.walkSmooth {
		mode = "Smooth"
	}
	g.s.contentName = "Walk preview: " + mode + "  |  I: compare  |  Smooth: 100 ms delay"
	ebiten.SetWindowTitle("Nanolathe — " + g.s.contentName)
}

func (g *unitViewerWalkPreviewGame) Update() error {
	if g.done || !g.s.Active() {
		return ebiten.Termination
	}
	if g.out != "" {
		return nil
	}
	search := g.s.panel != nil && g.s.panel.EditorCaptured()
	if !search && inpututil.IsKeyJustPressed(ebiten.KeyI) {
		g.s.model.walkSmooth = !g.s.model.walkSmooth
		g.s.model.refreshPose()
		g.updateTitle()
	}
	g.s.Update()
	return nil
}

func (g *unitViewerWalkPreviewGame) Layout(w, h int) (int, int) {
	if g.out == "" {
		g.w, g.h = max(320, w), max(240, h)
	}
	return g.w, g.h
}

func (g *unitViewerWalkPreviewGame) Draw(dst *ebiten.Image) {
	if g.done || !g.s.Active() {
		return
	}
	if g.out == "" {
		g.s.Draw(dst)
		return
	}
	// Hold the clock while asynchronous pictures load, so their completion
	// cannot change the captured pose. Then each Draw is exactly 1/120 s.
	g.s.Draw(dst)
	g.s.revealSelection()
	if g.pictureFrames++; g.s.picsPending > 0 && g.pictureFrames < unitViewerShotPictureFrames {
		return
	}
	if g.warmup > 0 {
		g.s.model.updateAnimation(1.0 / 120)
		g.warmup--
		return
	}
	g.s.Draw(dst)
	img := image.NewRGBA(image.Rect(0, 0, g.w, g.h))
	dst.ReadPixels(img.Pix)
	out := g.out
	if g.count > 1 {
		ext := filepath.Ext(out)
		out = fmt.Sprintf("%s-%04d%s", strings.TrimSuffix(out, ext), g.captured, ext)
	}
	g.err = encodeShotPNG(out, img)
	if g.err == nil {
		g.err = g.s.model.err
	}
	g.captured++
	g.done = g.err != nil || g.captured == g.count
	if !g.done {
		g.s.model.updateAnimation(1.0 / 120)
	}
}
