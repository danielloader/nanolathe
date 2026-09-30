package main

import (
	"errors"
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/nanolathe-gg/nanolathe/internal/settings"
)

// runNLScreenShot renders the Nanolathe screen to PNGs with no visible
// window, one file per card plus a few interaction states, each captured once
// its live background has staged and played for a moment: the review route
// for the screen (docs/DESIGN_INTERFACE_HUD_INPUT.md §3.17). The shell starts
// from default preferences, as every capture does.
func runNLScreenShot(opts Options, cs *contentSet) error {
	w, h, err := parseNLShotSize(opts.NLShotSize)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(opts.NLShot, 0o755); err != nil {
		return fmt.Errorf("nanolathe: nl-shot: create %s: %w", opts.NLShot, err)
	}
	shell, err := newGameShell(opts, cs)
	if err != nil {
		return err
	}
	// A fresh player's settings, layered for the running content as a
	// window start would (modsettings.go).
	shell.applySettings(settings.Defaults())
	s := newNLScreen(func() *gameShell { return shell })
	s.canvasScale = 1
	s.show(shell)
	defer s.finishHide()
	game := &nlShotGame{s: s, out: opts.NLShot, w: w, h: h}
	game.steps = nlShotSteps(s, opts.NLShotOnly)
	if len(game.steps) == 0 {
		return fmt.Errorf("nanolathe: nl-shot: no card matches %q", opts.NLShotOnly)
	}
	fmt.Fprintf(os.Stderr, "nanolathe: nl-shot: %d captures at %dx%d\n", len(game.steps), w, h)
	ebiten.SetWindowVisible(false)
	ebiten.SetWindowSize(640, 360)
	if err := ebiten.RunGame(game); err != nil && !errors.Is(err, ebiten.Termination) {
		return fmt.Errorf("nanolathe: nl-shot: capture loop: %w", err)
	}
	if game.err != nil {
		return game.err
	}
	fmt.Fprintf(os.Stderr, "nanolathe: nl-shot: wrote %d captures to %s\n", game.written, opts.NLShot)
	return nil
}

// nlShotSeconds is the point of a scene's clock a capture is taken at: the
// preset's own, or three seconds in.
func nlShotSeconds(preset nlPreset) float64 {
	if preset.shot > 0 {
		return preset.shot
	}
	return 3
}

func parseNLShotSize(text string) (int, int, error) {
	ws, hs, ok := strings.Cut(strings.ToLower(text), "x")
	w, werr := strconv.Atoi(ws)
	h, herr := strconv.Atoi(hs)
	if !ok || werr != nil || herr != nil || w < 320 || h < 240 {
		return 0, 0, fmt.Errorf("nanolathe: nl-shot: size %q: expected WxH of at least 320x240", text)
	}
	return w, h, nil
}

// nlShotStep is one capture: which card, and the state around it.
type nlShotStep struct {
	name    string
	page    int
	card    int
	compare bool
	dialog  string
	draft   func(d *nlDraft)
	// The Controls page: which tab, and a key cap waiting for a press.
	ctlGroup  int
	capturing bool
	profile   int
	// part is the grouped card's part to select, for a part with a scene of
	// its own.
	part int
}

func nlShotSteps(s *nlScreen, only string) []nlShotStep {
	want := map[string]bool{}
	for _, key := range strings.Split(only, ",") {
		if key = strings.TrimSpace(key); key != "" {
			want[key] = true
		}
	}
	var steps []nlShotStep
	for pi, page := range s.pages() {
		if page.key == "controls" {
			if len(want) > 0 && !want["controls"] {
				continue
			}
			for gi, name := range nlControlGroups {
				steps = append(steps, nlShotStep{name: fmt.Sprintf("%d-controls-%d-%s", pi+1, gi+1, strings.ToLower(name)), page: pi, ctlGroup: gi})
			}
			steps = append(steps,
				nlShotStep{name: fmt.Sprintf("%d-controls-9-capture", pi+1), page: pi, capturing: true},
				nlShotStep{name: fmt.Sprintf("%d-controls-9-zero", pi+1), page: pi, profile: 3})
			continue
		}
		for ci, card := range page.cards {
			if len(want) > 0 && !want[card.key] && !want[page.key+":"+card.key] && !want[page.key] {
				continue
			}
			base := fmt.Sprintf("%d-%s-%02d-%s", pi+1, page.key, ci+1, card.key)
			steps = append(steps, nlShotStep{name: base, page: pi, card: ci})
			// A grouped card shows its selected part's scene, so each part
			// that brings a scene of its own is captured too, named after the
			// card so it sorts beside it.
			if len(card.parts) > 0 {
				seen := []string{card.parts[0].scene}
				for i, part := range card.parts {
					if part.scene == "" || slices.Contains(seen, part.scene) {
						continue
					}
					seen = append(seen, part.scene)
					steps = append(steps, nlShotStep{name: base + "-" + strings.ToLower(part.key), page: pi, card: ci, part: i})
				}
			}
			if card.compare != nil && !card.usesMutators {
				steps = append(steps, nlShotStep{name: base + "-compare", page: pi, card: ci, compare: true})
				// Asked for by name, a grouped card compares every part, so
				// each part's measured change is reported.
				for i := 1; i < len(card.parts) && len(want) > 0; i++ {
					steps = append(steps, nlShotStep{name: base + "-compare-" + strings.ToLower(card.parts[i].key), page: pi, card: ci, compare: true, part: i})
				}
			}
			if card.demo != "" {
				// The demonstration's other value too.
				steps = append(steps, nlShotStep{name: base + "-alt", page: pi, card: ci, draft: func(d *nlDraft) {
					card.set(d, (card.get(d)+1)%len(card.steps))
				}})
			}
			if card.usesMutators {
				// Raised two steps and compared against ×1, as twin scenes.
				steps = append(steps, nlShotStep{name: base + "-raised", page: pi, card: ci, compare: true, draft: func(d *nlDraft) {
					card.set(d, min(len(card.steps)-1, card.get(d)+2))
				}})
			}
			if card.key == "profile" {
				steps = append(steps, nlShotStep{name: base + "-community", page: pi, card: ci, draft: func(d *nlDraft) { d.controls = 2 }})
			}
			if card.key == "content" {
				steps = append(steps, nlShotStep{name: base + "-presets", page: pi, card: ci, dialog: "presets"})
			}
			if card.key == "rules" {
				steps = append(steps, nlShotStep{name: base + "-override", page: pi, card: ci, dialog: "override"})
			}
			if card.key == "glow" && len(want) == 0 {
				steps = append(steps, nlShotStep{name: base + "-classic", page: pi, card: ci, draft: func(d *nlDraft) { d.pres.Renderer = "classic" }})
			}
		}
	}
	return steps
}

type nlShotGame struct {
	diff      nlDiffStats
	s         *nlScreen
	steps     []nlShotStep
	index     int
	out       string
	w, h      int
	target    *ebiten.Image
	frames    int
	settle    int
	started   bool
	demoArmed bool
	written   int
	err       error
	done      bool
}

func (g *nlShotGame) Update() error {
	if g.err != nil || g.done {
		return ebiten.Termination
	}
	return nil
}

func (g *nlShotGame) Layout(int, int) (int, int) { return 640, 360 }

func (g *nlShotGame) Draw(screen *ebiten.Image) {
	if g.done || g.err != nil {
		return
	}
	if g.target == nil {
		g.target = ebiten.NewImage(g.w, g.h)
	}
	s := g.s
	step := g.steps[g.index]
	if !g.started {
		g.started = true
		g.frames, g.settle, g.demoArmed = 0, 0, false
		s.draft = s.snapshot(s.shell())
		if step.draft != nil {
			step.draft(&s.draft)
		}
		s.selectPage(step.page)
		s.focusCard(step.card)
		s.compare = step.compare
		s.dialog, s.pendingV = step.dialog, 0
		s.presetScopes = [3]bool{true, true, true}
		s.ctlGroup, s.ctlRow, s.ctlScroll, s.capture = step.ctlGroup, 1, 0, nlCapture{}
		if step.profile != 0 {
			s.chooseProfile(step.profile)
		}
		if card := s.pages()[step.page].cards[step.card]; len(card.parts) > 0 {
			s.partSel[card.key] = step.part
		}
		if step.capturing {
			acts := s.controlActions()
			s.capture = nlCapture{id: acts[1].ID}
		}
	}
	g.target.Clear()
	s.Draw(g.target)
	g.frames++
	p := s.preview
	// A capture is taken at a fixed point of its scene's own clock, so a
	// scene with a rhythm (a shell landing, a wreck appearing) is caught at
	// the same moment every run whatever the host's frame rate. A scene
	// already past that point when the step begins — the step before it
	// showed the same one — is staged afresh.
	if g.frames == 1 && p != nil && p.cur != nil && p.cur.key == p.want && !p.loading && p.cur.seconds() >= nlShotSeconds(p.cur.preset) {
		p.startLoad(p.want)
	}
	// Over the last second before the capture, a split compare's two
	// pictures are measured against each other, so a review can say how
	// much of the frame an effect changes rather than guess from one still.
	if g.frames == 1 {
		g.diff = nlDiffStats{}
	}
	if step.compare && p != nil && p.cur != nil && p.cur.key == p.want && !p.loading && p.fadeLeft == 0 && p.cur.twin == nil &&
		p.frame != nil && p.alt != nil && p.cur.seconds() >= nlShotSeconds(p.cur.preset)-1 {
		g.diff.add(p.frame, p.alt)
	}
	if p != nil && p.cur != nil && p.cur.key == p.want && !p.loading && p.fadeLeft == 0 && (!step.compare || p.alt != nil) &&
		p.cur.seconds() >= nlShotSeconds(p.cur.preset) {
		g.settle++
	}
	// A scene that never stages is captured as it stands after a long wait.
	if g.settle < 1 && g.frames < 4000 {
		return
	}
	// A demonstration is caught at the same point of its loop every time:
	// restarted once the scene has settled, captured 2.6 s in.
	if card := s.pages()[step.page].cards[step.card]; card.demo != "" && g.frames < 4000 {
		if !g.demoArmed {
			g.demoArmed = true
			s.demoT = 0
			return
		}
		if s.demoT < 2.6 {
			return
		}
	}
	if g.frames >= 4000 {
		fmt.Fprintf(os.Stderr, "nanolathe: nl-shot: %s: the preview never staged (%s)\n", step.name, p.lastErr)
	}
	pix := make([]byte, 4*g.w*g.h)
	g.target.ReadPixels(pix)
	img := &image.RGBA{Pix: pix, Stride: 4 * g.w, Rect: image.Rect(0, 0, g.w, g.h)}
	for i := 3; i < len(pix); i += 4 {
		pix[i] = 255
	}
	path := filepath.Join(g.out, step.name+".png")
	f, err := os.Create(path)
	if err != nil {
		g.err = fmt.Errorf("nanolathe: nl-shot: create %s: %w", path, err)
		return
	}
	if err := png.Encode(f, img); err != nil {
		_ = f.Close()
		g.err = fmt.Errorf("nanolathe: nl-shot: write %s: %w", path, err)
		return
	}
	if err := f.Close(); err != nil {
		g.err = err
		return
	}
	g.written++
	fmt.Fprintf(os.Stderr, "nanolathe: nl-shot: %s (%d frames)\n", path, g.frames)
	if g.diff.frames > 0 {
		fmt.Fprintf(os.Stderr, "nanolathe: nl-shot: %s: %s\n", step.name, g.diff)
	}
	g.index++
	g.started = false
	if g.index >= len(g.steps) {
		g.done = true
	}
}

// nlDiffStats measures how much a compare's value changes the picture: over
// some frames, the share of pixels whose colour differs between the two
// renders at all and by eight levels or more on some channel, and the mean
// largest-channel difference of those that differ at all, for the whole
// frame and for the stage (right of the hero text, above the cards).
type nlDiffStats struct {
	frames int
	all    [2]nlDiffArea // whole frame, stage
}

type nlDiffArea struct {
	pixels, changed, visible, levels int64
}

func (d *nlDiffStats) add(a, b *ebiten.Image) {
	w, h := a.Bounds().Dx(), a.Bounds().Dy()
	if b.Bounds().Dx() != w || b.Bounds().Dy() != h {
		return
	}
	pa, pb := make([]byte, 4*w*h), make([]byte, 4*w*h)
	a.ReadPixels(pa)
	b.ReadPixels(pb)
	x0, x1, y0, y1 := w*46/100, w, h/10, h*72/100
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			i := 4 * (y*w + x)
			m := 0
			for c := 0; c < 3; c++ {
				m = max(m, abs(int(pa[i+c])-int(pb[i+c])))
			}
			areas := d.all[:1]
			if x >= x0 && x < x1 && y >= y0 && y < y1 {
				areas = d.all[:]
			}
			for k := range areas {
				ar := &areas[k]
				ar.pixels++
				if m > 0 {
					ar.changed++
					ar.levels += int64(m)
				}
				if m >= 8 {
					ar.visible++
				}
			}
		}
	}
	d.frames++
}

func (d nlDiffStats) String() string {
	part := func(a nlDiffArea) string {
		if a.pixels == 0 {
			return "nothing"
		}
		mean := 0.0
		if a.changed > 0 {
			mean = float64(a.levels) / float64(a.changed)
		}
		return fmt.Sprintf("%.2f%% changed, %.2f%% by 8+ levels, mean %.1f levels", 100*float64(a.changed)/float64(a.pixels), 100*float64(a.visible)/float64(a.pixels), mean)
	}
	return fmt.Sprintf("compare over %d frames: frame %s; stage %s", d.frames, part(d.all[0]), part(d.all[1]))
}
