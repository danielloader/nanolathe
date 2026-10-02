package main

import (
	"fmt"
	"image/color"
	"strconv"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/nanolathe-gg/nanolathe/internal/platform/ebitenapp"
	"github.com/nanolathe-gg/nanolathe/internal/platform/screenkit"
	"github.com/nanolathe-gg/nanolathe/internal/settings"
)

// Resolution is a host preference, shared across content and gameplay modes.
// The draft and window commit follow DESIGN_PRESENTATION_CLIENT §2.1.
func (s *nlScreen) resolutionCard() nlCard {
	return nlCard{
		key: "resolution", label: "Game resolution", pics: []string{"armrad", "armmark"}, kind: nlResolution,
		steps: []string{"Custom size"}, get: func(*nlDraft) int { return 0 }, set: func(*nlDraft, int) {},
		desc: func(*nlDraft, int) string {
			return "Choose a preset or enter your own width and height. Apply resizes the window and sets the next battle's resolution. Fullscreen fits it to your display."
		},
		scene: func(*nlDraft, int) string { return "armor" },
	}
}

func resolutionLabel(m retailDisplayMode) string { return fmt.Sprintf("%d × %d", m.W, m.H) }

func (s *nlScreen) readResolutionModes() {
	w, h := ebitenapp.DesktopSize()
	pw, ph := ebitenapp.DesktopPixelSize()
	s.resolutionNative = retailDisplayMode{pw, ph}
	s.resolutionModes = retailMonitorDisplayModes(retailDisplayMode{w, h}, s.resolutionNative, s.draft.resolution)
}

func (s *nlScreen) chooseResolution(m retailDisplayMode) {
	next := s.draft
	next.resolution = m
	s.setCardDraft(s.resolutionCard(), next)
}

func (s *nlScreen) stepResolution(delta int) {
	modes := s.resolutionModes
	if len(modes) == 0 {
		modes = retailMonitorDisplayModes(retailDisplayMode{}, retailDisplayMode{}, s.draft.resolution)
	}
	i := retailDisplayModeIndex(modes, s.draft.resolution.W, s.draft.resolution.H)
	// An entered custom size need not be a preset. Start at its sorted insertion
	// point so both arrows move towards the neighbouring size.
	if modes[i] != s.draft.resolution {
		i = len(modes)
		for j, m := range modes {
			if m.W > s.draft.resolution.W || m.W == s.draft.resolution.W && m.H > s.draft.resolution.H {
				i = j
				break
			}
		}
		if delta > 0 {
			i--
		}
	}
	s.chooseResolution(modes[max(0, min(len(modes)-1, i+delta))])
}

func (s *nlScreen) heroResolution(screen *ebiten.Image, x, y, a float64) float64 {
	u := s.u()
	s.arrow(screen, "resolution-prev", screenkit.Rect{X: x, Y: y, W: 48 * u, H: 48 * u}, true, true, func() { s.stepResolution(-1) })
	s.fonts.Display.Draw(screen, resolutionLabel(s.draft.resolution), x+72*u, y+35*u, screenkit.Style{Size: 32 * u, Top: alphaC(nlGreenText, a)})
	s.arrow(screen, "resolution-next", screenkit.Rect{X: x + 390*u, Y: y, W: 48 * u, H: 48 * u}, false, true, func() { s.stepResolution(1) })
	s.button(screen, "resolution-custom", screenkit.Rect{X: x + 456*u, Y: y, W: 166 * u, H: 48 * u}, "Custom size", false, false, s.openResolutionDialog)
	label := "Standard presets · monitor size unavailable"
	if s.resolutionNative.W > 0 {
		label = "Monitor: " + resolutionLabel(s.resolutionNative) + " · presets follow its proportions"
	}
	s.fonts.Body.Draw(screen, label, x, y+75*u, screenkit.Style{Size: 12 * u, Top: alphaC(nlBody, a)})
	if s.resolutionNative.W >= settings.MinDisplaymodeWidth && s.resolutionNative.H >= settings.MinDisplaymodeHeight {
		s.button(screen, "resolution-native", screenkit.Rect{X: x, Y: y + 92*u, W: 250 * u, H: 38 * u}, "Use monitor resolution", false, false, func() { s.chooseResolution(s.resolutionNative) })
	}
	return y + 136*u
}

func (s *nlScreen) openResolutionDialog() {
	s.resolutionText = [2]string{strconv.Itoa(s.draft.resolution.W), strconv.Itoa(s.draft.resolution.H)}
	s.resolutionField, s.resolutionReplace, s.resolutionError = 0, true, ""
	s.dialog = "resolution"
}

// Bound new custom entries before any renderer allocates surfaces. This is a
// Nanolathe UI limit, not a retail mode table or a settings-file migration.
func parseCustomResolution(fields [2]string) (retailDisplayMode, bool) {
	w, we := strconv.Atoi(fields[0])
	h, he := strconv.Atoi(fields[1])
	return retailDisplayMode{w, h}, we == nil && he == nil && w >= settings.MinDisplaymodeWidth && h >= settings.MinDisplaymodeHeight && w <= 8192 && h <= 8192
}

func (s *nlScreen) acceptResolution() {
	m, ok := parseCustomResolution(s.resolutionText)
	if !ok {
		s.resolutionError = "Enter a width of 640–8192 and height of 480–8192."
		return
	}
	s.chooseResolution(m)
	s.dialog = ""
}

func (s *nlScreen) updateResolution(in screenkit.Input) {
	if in.KeyPressed(ebiten.KeyEscape) {
		s.dialog = ""
		return
	}
	if in.KeyPressed(ebiten.KeyTab) {
		s.resolutionField = 1 - s.resolutionField
		s.resolutionReplace = true
	}
	if in.KeyPressed(ebiten.KeyEnter) && !ebiten.IsKeyPressed(ebiten.KeyAlt) {
		s.acceptResolution()
		return
	}
	field := &s.resolutionText[s.resolutionField]
	if in.KeyPressed(ebiten.KeyBackspace) || in.KeyPressed(ebiten.KeyDelete) {
		if s.resolutionReplace {
			*field = ""
		} else if len(*field) > 0 {
			*field = (*field)[:len(*field)-1]
		}
		s.resolutionReplace = false
	}
	for _, r := range ebiten.AppendInputChars(nil) {
		if r < '0' || r > '9' {
			continue
		}
		if s.resolutionReplace {
			*field = ""
			s.resolutionReplace = false
		}
		if len(*field) < 5 {
			*field += string(r)
		}
	}
}

func (s *nlScreen) drawResolutionDialog(screen *ebiten.Image) {
	u := s.u()
	full := screenkit.Rect{W: s.w(), H: s.h()}
	screenkit.Fill(screen, full, color.RGBA{0, 0, 0, 180})
	s.hits.Add(screenkit.Region{ID: "resolution-block", Rect: full})
	r := screenkit.Rect{X: s.w()/2 - 310*u, Y: s.h()/2 - 160*u, W: 620 * u, H: 320 * u}
	s.well(screen, r, 1)
	s.fonts.Display.Draw(screen, "Custom resolution", r.X+30*u, r.Y+50*u, screenkit.Style{Size: 28 * u, Top: nlGoldTop})
	for i, label := range []string{"Width", "Height"} {
		x := r.X + 30*u + float64(i)*280*u
		s.fonts.Body.Draw(screen, label, x, r.Y+92*u, screenkit.Style{Size: 14 * u, Top: nlBody})
		field := screenkit.Rect{X: x, Y: r.Y + 108*u, W: 260 * u, H: 54 * u}
		screenkit.Fill(screen, field, color.RGBA{4, 8, 4, 255})
		edge := nlDim
		if i == s.resolutionField {
			edge = nlGoldTop
		}
		screenkit.Outline(screen, field, 2*u, edge)
		if i == s.resolutionField && s.resolutionReplace {
			screenkit.Fill(screen, field.Inset(6*u), color.RGBA{40, 70, 44, 255})
		}
		value := s.resolutionText[i]
		if i == s.resolutionField {
			value += "_"
		}
		s.fonts.Display.Draw(screen, value, x+14*u, field.Y+36*u, screenkit.Style{Size: 26 * u, Top: nlCream})
		s.hits.Add(screenkit.Region{ID: fmt.Sprintf("resolution-field-%d", i), Rect: field, Click: func() { s.resolutionField = i; s.resolutionReplace = true }})
	}
	hint := "Pixels · Tab switches fields · Enter uses this size"
	if s.resolutionError != "" {
		hint = s.resolutionError
	}
	s.fonts.Body.Draw(screen, hint, r.X+30*u, r.Y+202*u, screenkit.Style{Size: 12 * u, Top: nlBody})
	s.button(screen, "resolution-cancel", screenkit.Rect{X: r.X + 30*u, Y: r.Y + 244*u, W: 160 * u, H: 44 * u}, "Cancel", false, false, func() { s.dialog = "" })
	s.button(screen, "resolution-use", screenkit.Rect{X: r.X + 400*u, Y: r.Y + 244*u, W: 190 * u, H: 44 * u}, "Use size", true, false, s.acceptResolution)
}
