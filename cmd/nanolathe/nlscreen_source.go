package main

import (
	"encoding/json"
	"image/color"
	"strings"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/nanolathe-gg/nanolathe/internal/platform/screenkit"
	"github.com/nanolathe-gg/nanolathe/internal/settings"
)

// Where a setting's value comes from (docs/DESIGN_MODS_MUTATORS.md §4.6): the
// base settings, the running mod's recommendation, or the player's own
// change for that mod; and whether the mod locks it. The screen marks each
// card and offers to go back to the mod's value.

// nlSource is the running content's layers as drafts, for comparing cards.
type nlSource struct {
	mod        string  // the running mod's name, "" without a config
	base, rec  nlDraft // the base settings, and the base with the mod's recommendation
	locks      []string
	overridden bool
	paths      map[string][]string // settings paths each card writes
}

// bindSource computes the running content's layers.
func (s *nlScreen) bindSource(g *gameShell) {
	s.src = nlSource{paths: map[string][]string{}}
	m := g.contentMod()
	if m == nil || m.Config == nil {
		return
	}
	base := g.baseSettings
	rec, err := settings.Layer(base, modRecommendations(m))
	if err != nil {
		return
	}
	s.src.mod = m.Name
	s.src.base = s.draftOf(base)
	s.src.rec = s.draftOf(rec)
	s.src.locks = modLocks(m)
	s.src.overridden = g.lockOverridden(m)
	for _, page := range s.pages() {
		for _, c := range page.cards {
			s.src.paths[c.key] = s.cardPaths(c, rec)
		}
	}
}

// draftOf reads a settings block into a draft, the way snapshot reads the
// shell.
func (s *nlScreen) draftOf(st settings.Settings) nlDraft {
	st.Normalize()
	d := nlDraft{
		gameplay: st.Gameplay.Normalize(), pres: st.Presentation,
		glow: st.Display.Glow, glowStrength: st.Display.GlowStrength,
		unitLimit: st.UnitLimit, switchAlt: st.SwitchAltEnabled(), interfaceType: st.InterfaceType,
		keys: keyMapFromSettings(st.KeyBindings),
	}
	if d.glowStrength <= 0 {
		d.glowStrength = settings.DefaultGlowStrength
	}
	return d
}

// cardPaths finds the settings paths a card writes by changing it and
// diffing: every part of a grouped card, every other card to its next value.
func (s *nlScreen) cardPaths(c nlCard, base settings.Settings) []string {
	if c.key == "content" {
		return nil
	}
	from := s.draftOf(base)
	var probes []nlDraft
	if c.kind == nlGroup {
		for _, p := range c.parts {
			d := from
			p.set(&d, (p.get(&d)+1)%len(p.steps))
			probes = append(probes, d)
		}
	} else if len(c.steps) > 1 {
		d := from
		c.set(&d, (c.get(&d)+1)%len(c.steps))
		probes = append(probes, d)
	}
	seen := map[string]bool{}
	var out []string
	for _, d := range probes {
		diff, err := settings.Diff(settingsOf(base, d), settingsOf(base, from), settings.ModScoped)
		if err != nil || len(diff) == 0 {
			continue
		}
		var doc map[string]any
		if json.Unmarshal(diff, &doc) != nil {
			continue
		}
		collectPaths(doc, "", func(p string) {
			if !seen[p] {
				seen[p] = true
				out = append(out, p)
			}
		})
	}
	return out
}

func collectPaths(doc map[string]any, prefix string, add func(string)) {
	for k, v := range doc {
		p := k
		if prefix != "" {
			p = prefix + "." + k
		}
		if m, ok := v.(map[string]any); ok && len(m) > 0 && p != "keyBindings" && p != "gameplayFeatures" {
			collectPaths(m, p, add)
			continue
		}
		add(p)
	}
}

// sourceActive reports whether the markers apply: the draft content is the
// running mod, which has a config.
func (s *nlScreen) sourceActive() bool {
	g := s.shell()
	return s.src.mod != "" && g != nil && sameMod(s.modAt(s.draft.mod), g.cs.mod)
}

// cardLocked reports whether the running mod locks something the card
// writes and the player has not overridden it.
func (s *nlScreen) cardLocked(c nlCard) bool {
	if !s.sourceActive() || s.src.overridden || s.draft.override || len(s.src.locks) == 0 {
		return false
	}
	return pathsLocked(s.src.paths[c.key], s.src.locks)
}

// keysLocked reports whether the keyboard is locked.
func (s *nlScreen) keysLocked() bool {
	if !s.sourceActive() || s.src.overridden || s.draft.override {
		return false
	}
	return pathsLocked([]string{"keyBindings"}, s.src.locks)
}

func pathsLocked(paths, locks []string) bool {
	for _, p := range paths {
		for _, l := range locks {
			if p == l || strings.HasPrefix(p, l+".") || strings.HasPrefix(l, p+".") {
				return true
			}
		}
	}
	return false
}

// guardLocked runs change now, or, when the card is locked, asks to
// override the mod's locks first and runs it after.
func (s *nlScreen) guardLocked(c nlCard, change func()) {
	if !s.cardLocked(c) {
		change()
		return
	}
	s.pendingAction, s.pendingWhat, s.dialog = change, c.label, "override"
}

// cardSource is the card's marker: "set" when the mod's recommendation sets
// it, "changed" when the player changed it for the mod, "" otherwise.
func (s *nlScreen) cardSource(c nlCard) string {
	if !s.sourceActive() || c.key == "content" {
		return ""
	}
	v, rec, base := c.get(&s.draft), c.get(&s.src.rec), c.get(&s.src.base)
	switch {
	case v != rec:
		return "changed"
	case rec != base:
		return "set"
	}
	return ""
}

// drawSource is the hero's marker beside the page counter: who set the
// value, and a way back to the mod's. y is the marker's top.
func (s *nlScreen) drawSource(screen *ebiten.Image, c nlCard, x, y, a float64) {
	u := s.u()
	df := s.fonts.Display
	chip := func(text string, col color.RGBA, lock bool) float64 {
		st := screenkit.Style{Size: 10.5 * u, Tracking: 0.14, Top: alphaC(col, a), Upper: true}
		tw := df.Measure(text, st) + 20*u
		if lock {
			tw += 16 * u
		}
		r := screenkit.Rect{X: x, Y: y, W: tw, H: 22 * u}
		screenkit.Fill(screen, r, color.RGBA{6, 12, 6, uint8(210 * a)})
		screenkit.Outline(screen, r, 1*u, alphaC(col, 0.7*a))
		tx := r.X + 10*u
		if lock {
			s.padlock(screen, tx, r.Y+4*u, 12*u, col, false)
			tx += 16 * u
		}
		df.Draw(screen, text, tx, r.Y+15.5*u, st)
		return tw
	}
	if s.cardLocked(c) {
		chip("Locked by "+s.src.mod, nlAmber, true)
		return
	}
	switch s.cardSource(c) {
	case "set":
		chip("Set by "+s.src.mod, nlGreenText, false)
	case "changed":
		w := chip("Changed for "+s.src.mod, nlAmber, false)
		id := "use-mod-" + c.key
		label := "Use " + s.src.mod + "'s"
		if c.kind != nlGroup {
			if rec := c.get(&s.src.rec); rec >= 0 && rec < len(c.steps) && c.steps[rec] != "" {
				label = "Use " + s.src.mod + "'s " + c.steps[rec]
			}
		}
		st := screenkit.Style{Size: 10.5 * u, Tracking: 0.12, Top: alphaC(lerpRGBA(nlKicker, nlCream, s.hits.HoverAmount(id)), a), Upper: true}
		lx := x + w + 12*u
		lw := df.Draw(screen, label, lx, y+15.5*u, st)
		screenkit.Fill(screen, screenkit.Rect{X: lx, Y: y + 19*u, W: lw, H: 1 * u}, alphaC(nlKicker, a))
		s.hits.Add(screenkit.Region{ID: id, Rect: screenkit.Rect{X: lx - 4*u, Y: y, W: lw + 8*u, H: 22 * u}, Click: func() {
			s.setCard(c, c.get(&s.src.rec))
		}})
	}
}

// drawCardSource tags a carousel card with its marker.
func (s *nlScreen) drawCardSource(screen *ebiten.Image, c nlCard, r screenkit.Rect, a float64) {
	u := s.u()
	if s.cardLocked(c) {
		s.padlock(screen, r.X+r.W-30*u, r.Y+14*u, 14*u, nlAmber, false)
		return
	}
	tag := ""
	col := nlGreenText
	switch s.cardSource(c) {
	case "set":
		tag = s.src.mod
	case "changed":
		tag, col = "Yours", nlAmber
	}
	if tag == "" {
		return
	}
	st := screenkit.Style{Size: 9.5 * u, Tracking: 0.12, Top: alphaC(col, a), Upper: true}
	tw := s.fonts.Display.Measure(tag, st) + 12*u
	tr := screenkit.Rect{X: r.X + 14*u, Y: r.Y + 16*u, W: tw, H: 18 * u}
	screenkit.Fill(screen, tr, color.RGBA{6, 12, 6, uint8(210 * a)})
	screenkit.Outline(screen, tr, 1*u, alphaC(col, 0.6*a))
	s.fonts.Display.Draw(screen, tag, tr.X+6*u, tr.Y+13*u, st)
}
