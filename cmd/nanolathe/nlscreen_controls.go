package main

import (
	"fmt"
	"image/color"
	"math"
	"slices"
	"strings"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/nanolathe-gg/nanolathe/internal/input"
	"github.com/nanolathe-gg/nanolathe/internal/platform/screenkit"
)

// The Nanolathe screen's Controls page: a keyboard and mouse mapping view
// rather than a card deck (docs/DESIGN_INTERFACE_HUD_INPUT.md §3.17). A
// profile bar applies Retail, Community or TA Zero; a table lists every
// battle shortcut by group, each key a cap the player clicks to rebind; a
// keyboard diagram shows where the group's keys sit; the Mouse tab holds the
// button and behaviour switches. Every edit goes to the draft key map, which
// Apply hands to the shell.

// nlControlGroups are the table's tabs: the key map's groups, then Mouse.
var nlControlGroups = []string{input.GroupOrders, input.GroupSelection, input.GroupCamera, input.GroupGame, "Mouse"}

// nlCapture is a key cap waiting for a key press.
type nlCapture struct {
	id   string // action being rebound; "" when idle
	slot int    // which of its keys; len(keys) adds one
}

// nlKeyCap is one key on the keyboard diagram: its label, the portable key it
// stands for (KeyNone for a key the battle never reads) and its width in key
// units.
type nlKeyCap struct {
	label string
	key   input.Key
	w     float64
}

// nlKeyboard is a compact US layout, row by row; a zero-width cap is a gap.
var nlKeyboard = [][]nlKeyCap{
	{{"Esc", input.KeyEscape, 1}, {"", 0, 0.5}, {"F1", input.KeyF1, 1}, {"F2", input.KeyF2, 1}, {"F3", input.KeyF3, 1}, {"F4", input.KeyF4, 1}, {"", 0, 0.3},
		{"F5", input.KeyF5, 1}, {"F6", input.KeyF6, 1}, {"F7", input.KeyF7, 1}, {"F8", input.KeyF8, 1}, {"", 0, 0.3},
		{"F9", input.KeyF9, 1}, {"F10", input.KeyF10, 1}, {"F11", input.KeyF11, 1}, {"F12", input.KeyF12, 1}, {"", 0, 0.3}, {"Pause", input.KeyPause, 1.2}},
	{{"`", input.KeyBackquote, 1}, {"1", input.Key1, 1}, {"2", input.Key2, 1}, {"3", input.Key3, 1}, {"4", input.Key4, 1}, {"5", input.Key5, 1},
		{"6", input.Key6, 1}, {"7", input.Key7, 1}, {"8", input.Key8, 1}, {"9", input.Key9, 1}, {"0", input.Key0, 1}, {"-", input.KeyMinus, 1},
		{"=", input.KeyEqual, 1}, {"Back", input.KeyBackspace, 1.9}, {"", 0, 0.3}, {"Ins", input.KeyInsert, 1}, {"Home", input.KeyHome, 1}, {"PgUp", input.KeyPrior, 1}},
	{{"Tab", input.KeyTab, 1.5}, {"Q", input.KeyQ, 1}, {"W", input.KeyW, 1}, {"E", input.KeyE, 1}, {"R", input.KeyR, 1}, {"T", input.KeyT, 1},
		{"Y", input.KeyY, 1}, {"U", input.KeyU, 1}, {"I", input.KeyI, 1}, {"O", input.KeyO, 1}, {"P", input.KeyP, 1}, {"[", 0, 1}, {"]", 0, 1},
		{"\\", 0, 1.4}, {"", 0, 0.3}, {"Del", input.KeyDelete, 1}, {"End", input.KeyEnd, 1}, {"PgDn", input.KeyNext, 1}},
	{{"Caps", 0, 1.8}, {"A", input.KeyA, 1}, {"S", input.KeyS, 1}, {"D", input.KeyD, 1}, {"F", input.KeyF, 1}, {"G", input.KeyG, 1},
		{"H", input.KeyH, 1}, {"J", input.KeyJ, 1}, {"K", input.KeyK, 1}, {"L", input.KeyL, 1}, {";", 0, 1}, {"'", 0, 1}, {"Enter", input.KeyEnter, 2.1}},
	{{"Shift", input.KeyShift, 2.3}, {"Z", input.KeyZ, 1}, {"X", input.KeyX, 1}, {"C", input.KeyC, 1}, {"V", input.KeyV, 1}, {"B", input.KeyB, 1},
		{"N", input.KeyN, 1}, {"M", input.KeyM, 1}, {",", input.KeyComma, 1}, {".", input.KeyPeriod, 1}, {"/", 0, 1}, {"Shift", input.KeyShift, 2.6},
		{"", 0, 1.3}, {"Up", input.KeyUp, 1}},
	{{"Ctrl", input.KeyCtrl, 1.5}, {"", 0, 1}, {"Alt", input.KeyAlt, 1.3}, {"Space", input.KeySpace, 6.3}, {"Alt", input.KeyAlt, 1.3}, {"", 0, 1},
		{"Ctrl", input.KeyCtrl, 1.5}, {"", 0, 0.3}, {"Left", input.KeyLeft, 1}, {"Down", input.KeyDown, 1}, {"Right", input.KeyRight, 1}},
}

// nlGroupTint colours a group's keys on the diagram.
var nlGroupTint = map[string]color.RGBA{
	input.GroupOrders:    {86, 196, 96, 255},
	input.GroupSelection: {90, 160, 230, 255},
	input.GroupCamera:    {220, 180, 80, 255},
	input.GroupGame:      {200, 110, 200, 255},
}

func (s *nlScreen) controlsGroup() string { return nlControlGroups[s.ctlGroup] }

// controlActions lists the actions of the selected group.
func (s *nlScreen) controlActions() []input.Action {
	var out []input.Action
	for _, a := range input.Actions() {
		if a.Group == s.controlsGroup() {
			out = append(out, a)
		}
	}
	return out
}

// controlRows are the Mouse tab's rows: the Controls page's cards bar the
// profile, which the profile bar owns.
func (s *nlScreen) controlRows() []nlCard {
	var out []nlCard
	for _, c := range s.controlCards() {
		if c.key != "profile" {
			out = append(out, c)
		}
	}
	return out
}

// keysDiffer counts the actions whose draft keys differ from the shell's.
func (s *nlScreen) keysDiffer() int {
	g := s.shell()
	if g == nil || s.draft.keys == nil {
		return 0
	}
	live := g.liveKeyMap()
	n := 0
	if live.Profile() != s.draft.keys.Profile() {
		n++
	}
	for _, a := range input.Actions() {
		if !slices.Equal(live.Keys(a.ID), s.draft.keys.Keys(a.ID)) {
			n++
		}
	}
	return n
}

// updateControls handles the page's keyboard: a pending capture takes the
// next key; otherwise Up/Down choose a row and Space starts a capture. It
// reports whether it used the input.
func (s *nlScreen) updateControls(in screenkit.Input) bool {
	if s.capture.id != "" {
		for _, k := range in.Keys {
			switch k {
			case ebiten.KeyEscape:
				s.capture = nlCapture{}
				return true
			case ebiten.KeyBackspace:
				keys := s.draft.keys.Keys(s.capture.id)
				if s.capture.slot < len(keys) {
					keys = slices.Delete(slices.Clone(keys), s.capture.slot, s.capture.slot+1)
					s.draft.keys.Rebind(s.capture.id, keys)
					s.touched["keys"] = true
				}
				s.capture = nlCapture{}
				return true
			}
			ctrl := ebiten.IsKeyPressed(ebiten.KeyControl)
			shift := ebiten.IsKeyPressed(ebiten.KeyShift)
			chord, ok := keyCaptureChord(k, ctrl, shift)
			if !ok {
				continue
			}
			s.bindCaptured(chord)
			return true
		}
		// Any key or click that is not a chord waits; a click elsewhere
		// cancels (the table's regions handle clicks on caps).
		return len(in.Keys) > 0 || in.Pressed
	}
	if s.controlsGroup() == "Mouse" {
		return false
	}
	actions := s.controlActions()
	switch {
	case in.KeyPressed(ebiten.KeyArrowDown):
		s.ctlRow = min(s.ctlRow+1, len(actions)-1)
		return true
	case in.KeyPressed(ebiten.KeyArrowUp):
		s.ctlRow = max(s.ctlRow-1, 0)
		return true
	case in.KeyPressed(ebiten.KeySpace):
		if s.ctlRow < len(actions) && !actions[s.ctlRow].Fixed {
			s.capture = nlCapture{id: actions[s.ctlRow].ID, slot: len(s.draft.keys.Keys(actions[s.ctlRow].ID))}
		}
		return true
	}
	return false
}

// guardKeys runs a keyboard change now, or asks to override the mod's lock
// on the keyboard first.
func (s *nlScreen) guardKeys(change func()) {
	if !s.keysLocked() {
		change()
		return
	}
	s.capture = nlCapture{}
	s.pendingAction, s.pendingWhat, s.dialog = change, "the keyboard", "override"
}

// bindCaptured puts the captured chord in the capture's slot and says what,
// if anything, lost it.
func (s *nlScreen) bindCaptured(chord input.Chord) {
	if s.keysLocked() {
		s.guardKeys(func() { s.bindCaptured(chord) })
		return
	}
	id := s.capture.id
	keys := slices.Clone(s.draft.keys.Keys(id))
	if s.capture.slot < len(keys) {
		keys[s.capture.slot] = chord
	} else {
		keys = append(keys, chord)
	}
	displaced := s.draft.keys.Rebind(id, keys)
	s.touched["keys"] = true
	s.capture = nlCapture{}
	if g := s.shell(); g != nil {
		g.playMenuCue("SmallButton")
	}
	var names []string
	for _, d := range displaced {
		if d == id {
			continue
		}
		if a, ok := input.LookupAction(d); ok {
			names = append(names, a.Label)
		}
	}
	if len(names) > 0 {
		s.toast, s.toastLeft = fmt.Sprintf("%s taken from %s", chord.Label(), strings.Join(names, ", ")), 3
	}
	if !slices.Contains(s.draft.keys.Keys(id), chord) {
		s.toast, s.toastLeft = fmt.Sprintf("%s cannot be bound to that action", chord.Label()), 3
	}
}

// drawControls draws the whole page in place of the hero and the cards.
func (s *nlScreen) drawControls(screen *ebiten.Image) {
	u := s.u()
	// The battle behind stays, dimmed well down, so the table reads.
	screenkit.Fill(screen, screenkit.Rect{W: s.w(), H: s.h()}, color.RGBA{0, 0, 0, 170})
	x0 := 72 * u
	y := 116 * u
	y = s.drawProfileBar(screen, x0, y)
	y += 18 * u
	// Group tabs.
	tx := x0
	for i, name := range nlControlGroups {
		st := screenkit.Style{Size: 15 * u, Tracking: 0.14, Upper: true, Shadow: 0.1, Top: color.RGBA{183, 174, 140, 255}}
		id := "ctl-group-" + name
		if i == s.ctlGroup {
			st.Top = nlCream
		} else {
			st.Top = lerpRGBA(st.Top, color.RGBA{255, 255, 255, 255}, s.hits.HoverAmount(id))
		}
		tw := s.fonts.Display.Draw(screen, name, tx, y+16*u, st)
		if tint, ok := nlGroupTint[name]; ok {
			screenkit.Disc(screen, tx+tw+9*u, y+10*u, 3.5*u, tint)
			tw += 16 * u
		}
		if i == s.ctlGroup {
			screenkit.VGradient(screen, screenkit.Rect{X: tx, Y: y + 24*u, W: tw, H: 3 * u}, color.RGBA{184, 255, 194, 255}, color.RGBA{21, 168, 43, 255})
		}
		s.hits.Add(screenkit.Region{ID: id, Rect: screenkit.Rect{X: tx - 8*u, Y: y - 6*u, W: tw + 16*u, H: 36 * u}, Click: func() {
			s.ctlGroup, s.ctlRow, s.ctlScroll, s.capture = i, 0, 0, nlCapture{}
		}})
		tx += tw + 30*u
	}
	y += 44 * u
	table := screenkit.Rect{X: x0, Y: y, W: 700 * u, H: s.h() - y - 52*u}
	right := screenkit.Rect{X: table.X + table.W + 36*u, Y: y, W: s.w() - (table.X + table.W + 36*u) - 56*u, H: table.H}
	if s.controlsGroup() == "Mouse" {
		s.drawMouseRows(screen, table)
		s.drawMouse(screen, right)
	} else {
		s.drawKeyTable(screen, table)
		s.drawKeyboard(screen, right)
	}
}

// drawProfileBar is the row of profiles; the chosen one applies with Apply,
// and its keys show in the table at once.
func (s *nlScreen) drawProfileBar(screen *ebiten.Image, x, y float64) float64 {
	u := s.u()
	df := s.fonts.Display
	df.Draw(screen, "Controls", x, y+30*u, screenkit.Style{Size: 40 * u, Tracking: 0.02, Top: nlGoldTop, Bottom: nlGoldBottom, Shadow: 0.05})
	bx := x + 250*u
	recommend := s.recommendedProfile()
	df.Draw(screen, "Profile", bx, y+4*u, screenkit.Style{Size: 11 * u, Tracking: 0.26, Top: nlKicker, Upper: true})
	for i, p := range nlControlsPresets {
		label := p.label
		st := screenkit.Style{Size: 14 * u, Tracking: 0.08, Upper: true, Align: 1}
		w := df.Measure(label, st) + 34*u
		r := screenkit.Rect{X: bx, Y: y + 12*u, W: w, H: 34 * u}
		id := fmt.Sprintf("ctl-profile-%d", i)
		on := s.draft.controls == i
		hover := s.hits.HoverAmount(id)
		screenkit.Fill(screen, r, color.RGBA{14, 16, 12, 220})
		if on {
			screenkit.VGradient(screen, r, color.RGBA{50, 110, 50, 230}, color.RGBA{20, 50, 22, 230})
		}
		border := lerpRGBA(color.RGBA{70, 70, 60, 255}, color.RGBA{160, 160, 140, 255}, hover)
		if on {
			border = color.RGBA{255, 227, 138, 255}
		}
		screenkit.Outline(screen, r, 1.5*u, border)
		st.Top = map[bool]color.RGBA{false: {200, 192, 160, 255}, true: nlCream}[on]
		df.Draw(screen, label, r.X+r.W/2, r.Y+23*u, st)
		if recommend == i {
			s.fonts.Body.Draw(screen, "Recommended by "+s.recommendedBy(), r.X+r.W/2, r.Y+r.H+14*u, screenkit.Style{Size: 10 * u, Top: nlGreenText, Align: 1})
		}
		s.hits.Add(screenkit.Region{ID: id, Rect: r, Click: func() { s.chooseProfile(i) }})
		bx += w + 8*u
	}
	// Reset keys.
	rw := 190 * u
	s.button(screen, "ctl-reset", screenkit.Rect{X: s.w() - 56*u - rw, Y: y + 12*u, W: rw, H: 34 * u}, "Reset keys", false, false, func() {
		for _, a := range input.Actions() {
			s.draft.keys.Reset(a.ID)
		}
		s.touched["keys"] = true
		s.capture = nlCapture{}
	})
	return y + 60*u
}

// chooseProfile selects a profile: its keyboard keys show at once, and every
// setting it assigns is written by Apply, as the Mods screen's offer does.
func (s *nlScreen) chooseProfile(i int) {
	if s.keysLocked() {
		s.guardKeys(func() { s.chooseProfile(i) })
		return
	}
	s.draft.controls = i
	s.touched["profile"] = true
	preset := nlControlsPresets[i].preset
	profile := input.ProfileRetail
	g := s.shell()
	switch {
	case preset == "" && g != nil:
		profile = g.liveKeyMap().Profile()
	case preset == controlsPresetCommunity:
		profile = input.ProfileCommunity
	case preset == controlsPresetZero:
		profile = input.ProfileZero
	}
	s.draft.keys = input.NewKeyMap(profile, s.draft.keys.Overrides())
	s.touched["keys"] = true
	if g != nil {
		g.playMenuCue("SmallButton")
	}
}

// recommendedProfile is the profile the draft content recommends, -1 for
// none.
func (s *nlScreen) recommendedProfile() int {
	m := s.modAt(s.draft.mod)
	if m == nil || m.Controls == "" {
		return -1
	}
	for i, p := range nlControlsPresets {
		if p.preset == m.Controls {
			return i
		}
	}
	return -1
}

func (s *nlScreen) recommendedBy() string {
	if m := s.modAt(s.draft.mod); m != nil {
		return m.Name
	}
	return ""
}

// drawKeyTable lists the group's actions with their keys.
func (s *nlScreen) drawKeyTable(screen *ebiten.Image, r screenkit.Rect) {
	u := s.u()
	actions := s.controlActions()
	rowH := 36 * u
	visible := max(1, int(r.H/rowH))
	s.ctlRow = min(s.ctlRow, len(actions)-1)
	if s.ctlRow < s.ctlScroll {
		s.ctlScroll = s.ctlRow
	}
	if s.ctlRow >= s.ctlScroll+visible {
		s.ctlScroll = s.ctlRow - visible + 1
	}
	s.ctlScroll = max(0, min(s.ctlScroll, len(actions)-visible))
	s.ctlTable = r
	screenkit.Fill(screen, r, color.RGBA{8, 12, 8, 200})
	screenkit.Outline(screen, r, 1*u, color.RGBA{50, 60, 46, 255})
	df, bf := s.fonts.Display, s.fonts.Body
	g := s.shell()
	for row := 0; row < visible && s.ctlScroll+row < len(actions); row++ {
		i := s.ctlScroll + row
		a := actions[i]
		rr := screenkit.Rect{X: r.X + 6*u, Y: r.Y + 6*u + float64(row)*rowH, W: r.W - 12*u, H: rowH - 4*u}
		id := "ctl-row-" + a.ID
		sel := i == s.ctlRow
		if sel {
			screenkit.HGradient(screen, rr, color.RGBA{40, 80, 40, 200}, color.RGBA{14, 24, 14, 60})
		} else if s.hits.HoverAmount(id) > 0 {
			screenkit.Fill(screen, rr, color.RGBA{50, 60, 46, uint8(80 * s.hits.HoverAmount(id))})
		}
		tc := color.RGBA{214, 206, 176, 255}
		if a.Fixed {
			tc = nlDim
		}
		bf.Draw(screen, a.Label, rr.X+12*u, rr.Y+rr.H/2+5*u, screenkit.Style{Size: 13 * u, Top: tc, Shadow: 0.1})
		if a.Fixed {
			s.padlock(screen, rr.X+rr.W-24*u, rr.Y+rr.H/2-8*u, 14*u, nlDim, false)
		}
		keys := s.draft.keys.Keys(a.ID)
		if a.Fixed {
			keys = a.Default
		}
		kx := rr.X + 290*u
		for slot := 0; slot <= len(keys) && slot < 3; slot++ {
			if a.Fixed && slot == len(keys) {
				break
			}
			capturing := s.capture.id == a.ID && s.capture.slot == slot
			label := "+"
			if slot < len(keys) {
				label = keys[slot].Label()
			}
			if capturing {
				label = "Press a key"
			}
			cid := fmt.Sprintf("ctl-cap-%s-%d", a.ID, slot)
			w := s.keyCapButton(screen, cid, kx, rr.Y+4*u, rr.H-8*u, label, slot == len(keys), capturing, a.Fixed)
			if !a.Fixed {
				s.hits.Add(screenkit.Region{ID: cid, Rect: screenkit.Rect{X: kx, Y: rr.Y + 4*u, W: w, H: rr.H - 8*u},
					Click: func() {
						s.ctlRow = i
						s.capture = nlCapture{id: a.ID, slot: slot}
					},
					Right: func() {
						// Right-click clears a key.
						if slot < len(keys) {
							s.draft.keys.Rebind(a.ID, slices.Delete(slices.Clone(keys), slot, slot+1))
							s.touched["keys"] = true
						}
					}})
			}
			kx += w + 8*u
		}
		// A row that differs from the profile shows its lamp and a reset.
		if !a.Fixed && g != nil && !slices.Equal(keys, input.ProfileDefaults(s.draft.keys.Profile())[a.ID]) {
			s.lamp(screen, rr.X+rr.W-60*u, rr.Y+rr.H/2, 4.5*u, nlAmber, true)
			rid := "ctl-undo-" + a.ID
			st := screenkit.Style{Size: 10 * u, Tracking: 0.12, Top: lerpRGBA(nlDim, nlCream, s.hits.HoverAmount(rid)), Upper: true, Align: 2}
			df.Draw(screen, "Reset", rr.X+rr.W-10*u, rr.Y+rr.H/2+4*u, st)
			s.hits.Add(screenkit.Region{ID: rid, Rect: screenkit.Rect{X: rr.X + rr.W - 48*u, Y: rr.Y, W: 46 * u, H: rr.H}, Click: func() {
				s.draft.keys.Reset(a.ID)
				s.touched["keys"] = true
			}})
		}
		s.hits.Add(screenkit.Region{ID: id, Rect: screenkit.Rect{X: rr.X, Y: rr.Y, W: 280 * u, H: rr.H}, Click: func() { s.ctlRow = i }})
	}
	if len(actions) > visible {
		track := screenkit.Rect{X: r.X + r.W + 8*u, Y: r.Y, W: 5 * u, H: r.H}
		screenkit.Fill(screen, track, color.RGBA{20, 24, 20, 200})
		th := track.H * float64(visible) / float64(len(actions))
		ty := track.Y + (track.H-th)*float64(s.ctlScroll)/float64(len(actions)-visible)
		screenkit.Fill(screen, screenkit.Rect{X: track.X, Y: ty, W: track.W, H: th}, color.RGBA{120, 200, 120, 220})
	}
	hint := "Click a key to change it, or + to add one. Right-click a key to clear it."
	if s.capture.id != "" {
		hint = "Press the new key, with Ctrl or Shift if you want them. Esc cancels, Backspace clears."
	}
	bf.Draw(screen, hint, r.X, r.Y+r.H+22*u, screenkit.Style{Size: 11.5 * u, Top: nlKicker})
}

// keyCapButton draws a key cap button and returns its width.
func (s *nlScreen) keyCapButton(screen *ebiten.Image, id string, x, y, h float64, label string, add, capturing, fixed bool) float64 {
	u := s.u()
	st := screenkit.Style{Size: 13 * u, Tracking: 0.06, Top: nlCream, Align: 1}
	w := max(44*u, s.fonts.Display.Measure(label, st)+22*u)
	r := screenkit.Rect{X: x, Y: y, W: w, H: h}
	hover := s.hits.HoverAmount(id)
	switch {
	case capturing:
		a := 0.55 + 0.45*math.Sin(s.clock*7)
		screenkit.Glow(screen, screenkit.Rect{X: r.X - 16*u, Y: r.Y - 12*u, W: r.W + 32*u, H: r.H + 24*u}, alphaC(color.RGBA{255, 210, 90, 255}, 0.5*a))
		screenkit.Fill(screen, r, color.RGBA{60, 50, 18, 255})
		screenkit.Outline(screen, r, 1.5*u, alphaC(color.RGBA{255, 227, 138, 255}, a))
		st.Top = nlAmber
	case add:
		screenkit.Outline(screen, r, 1*u, lerpRGBA(color.RGBA{60, 70, 56, 255}, color.RGBA{140, 160, 130, 255}, hover))
		st.Top = lerpRGBA(nlDim, nlCream, hover)
	default:
		screenkit.Fill(screen, r.Inset(-1*u), color.RGBA{0, 0, 0, 255})
		top, bottom := color.RGBA{96, 96, 90, 255}, color.RGBA{50, 50, 46, 255}
		if fixed {
			top, bottom = color.RGBA{56, 56, 52, 255}, color.RGBA{36, 36, 32, 255}
			st.Top = nlDim
		}
		screenkit.VGradient(screen, r, lerpRGBA(top, color.RGBA{255, 255, 255, 255}, hover*0.15), bottom)
		screenkit.Bevel(screen, r, 1.5*u, color.RGBA{190, 190, 182, 255}, color.RGBA{20, 20, 18, 255}, false)
	}
	s.fonts.Display.Draw(screen, label, r.X+w/2, r.Y+h/2+5*u, st)
	return w
}

// drawKeyboard is the diagram: every key the battle reads, coloured by the
// group holding it. The selected row's keys glow gold, with the modifiers
// they need; hovering a key names what it does.
func (s *nlScreen) drawKeyboard(screen *ebiten.Image, r screenkit.Rect) {
	u := s.u()
	m := s.draft.keys
	// Who holds each key, plain or with modifiers.
	owners := map[input.Key][]string{}
	groupOf := map[string]string{}
	labelOf := map[string]string{}
	for _, a := range input.Actions() {
		groupOf[a.ID], labelOf[a.ID] = a.Group, a.Label
		keys := m.Keys(a.ID)
		if a.Fixed {
			keys = a.Default
		}
		for _, c := range keys {
			owners[c.Key] = append(owners[c.Key], a.ID)
		}
	}
	var selected []input.Chord
	if acts := s.controlActions(); s.ctlRow < len(acts) {
		selected = m.Keys(acts[s.ctlRow].ID)
		if acts[s.ctlRow].Fixed {
			selected = acts[s.ctlRow].Default
		}
	}
	lit := func(k input.Key) bool {
		for _, c := range selected {
			if c.Key == k || (k == input.KeyCtrl && c.Ctrl) || (k == input.KeyShift && c.Shift) {
				return true
			}
		}
		return false
	}
	unit := r.W / 19.4
	gap := unit * 0.1
	df := s.fonts.Display
	hoverKey := input.KeyNone
	hoverRect := screenkit.Rect{}
	y := r.Y
	for ri, row := range nlKeyboard {
		x := r.X
		h := unit
		if ri == 0 {
			h = unit * 0.8
		}
		for ci, k := range row {
			w := k.w*unit - gap
			if k.label == "" {
				x += k.w * unit
				continue
			}
			kr := screenkit.Rect{X: x, Y: y, W: w, H: h - gap}
			base := color.RGBA{34, 36, 32, 255}
			text := color.RGBA{110, 106, 90, 255}
			if k.key != input.KeyNone {
				base, text = color.RGBA{56, 58, 52, 255}, color.RGBA{180, 174, 150, 255}
			}
			if ids := owners[k.key]; len(ids) > 0 && k.key != input.KeyNone {
				tint := nlGroupTint[groupOf[ids[0]]]
				inGroup := false
				for _, id := range ids {
					if groupOf[id] == s.controlsGroup() {
						tint, inGroup = nlGroupTint[groupOf[id]], true
						break
					}
				}
				amount := 0.25
				if inGroup {
					amount = 0.6
				}
				base = lerpRGBA(base, tint, amount)
				text = color.RGBA{240, 236, 220, 255}
			}
			if lit(k.key) && k.key != input.KeyNone {
				screenkit.Glow(screen, screenkit.Rect{X: kr.X - unit*0.4, Y: kr.Y - unit*0.4, W: kr.W + unit*0.8, H: kr.H + unit*0.8}, color.RGBA{255, 214, 92, 120})
				base, text = color.RGBA{200, 160, 60, 255}, color.RGBA{30, 20, 0, 255}
			}
			id := fmt.Sprintf("kb-%d-%d", ri, ci)
			screenkit.Fill(screen, kr.Inset(-1*u), color.RGBA{0, 0, 0, 255})
			screenkit.VGradient(screen, kr, lerpRGBA(base, color.RGBA{255, 255, 255, 255}, 0.12), base)
			size := min(12*u, unit*0.34)
			for size > 6*u && df.Measure(k.label, screenkit.Style{Size: size}) > kr.W-6*u {
				size -= 0.5 * u
			}
			df.Draw(screen, k.label, kr.X+kr.W/2, kr.Y+kr.H/2+size/2, screenkit.Style{Size: size, Top: text, Align: 1})
			if k.key != input.KeyNone {
				s.hits.Add(screenkit.Region{ID: id, Rect: kr})
				if s.hits.Hot() == id {
					hoverKey, hoverRect = k.key, kr
				}
			}
			x += k.w * unit
		}
		y += h
	}
	// Legend.
	ly := y + 18*u
	lx := r.X
	for _, name := range nlControlGroups[:4] {
		screenkit.Fill(screen, screenkit.Rect{X: lx, Y: ly - 9*u, W: 12 * u, H: 12 * u}, nlGroupTint[name])
		lx += s.fonts.Body.Draw(screen, name, lx+18*u, ly+2*u, screenkit.Style{Size: 11 * u, Top: nlBody}) + 36*u
	}
	// The selected action's detail, or the hovered key's actions.
	info := screenkit.Rect{X: r.X, Y: ly + 24*u, W: r.W, H: r.Y + r.H - ly - 24*u}
	if hoverKey != input.KeyNone {
		s.drawKeyInfo(screen, info, hoverKey, labelOf)
		screenkit.Outline(screen, hoverRect.Inset(-2*u), 2*u, color.RGBA{255, 227, 138, 255})
		return
	}
	s.drawActionInfo(screen, info)
}

// drawKeyInfo lists what a key does alone, with Shift and with Ctrl.
func (s *nlScreen) drawKeyInfo(screen *ebiten.Image, r screenkit.Rect, k input.Key, labelOf map[string]string) {
	u := s.u()
	bf := s.fonts.Body
	y := r.Y + 16*u
	for _, mod := range []struct {
		name        string
		ctrl, shift bool
	}{{"", false, false}, {"Shift+", false, true}, {"Ctrl+", true, false}, {"Ctrl+Shift+", true, true}} {
		c := input.Chord{Key: k, Ctrl: mod.ctrl, Shift: mod.shift}
		id, ok := s.draft.keys.Owner(c)
		if !ok {
			continue
		}
		bf.Draw(screen, c.Label(), r.X, y, screenkit.Style{Size: 12.5 * u, Top: nlCream})
		bf.Draw(screen, labelOf[id], r.X+120*u, y, screenkit.Style{Size: 12.5 * u, Top: nlBody})
		y += 22 * u
	}
	if y == r.Y+16*u {
		bf.Draw(screen, "Nothing in battle uses this key.", r.X, y, screenkit.Style{Size: 12 * u, Top: nlDim})
	}
}

// drawActionInfo describes the selected action and, when a profile is
// chosen, what applying it changes beyond the keys.
func (s *nlScreen) drawActionInfo(screen *ebiten.Image, r screenkit.Rect) {
	u := s.u()
	bf := s.fonts.Body
	acts := s.controlActions()
	y := r.Y
	if s.ctlRow < len(acts) {
		a := acts[s.ctlRow]
		var retail []string
		for _, c := range a.Default {
			retail = append(retail, c.Label())
		}
		text := a.Label + ". The original key: " + strings.Join(retail, ", ") + "."
		if a.Fixed {
			text += " Held keys and group digits are read directly and are not rebound."
		} else if a.AnyShift {
			text += " An order key also answers with Shift held."
		}
		y += bf.DrawWrapped(screen, text, r.X, y, r.W, 2.0, screenkit.Style{Size: 12 * u, Top: nlBody})
	}
	if s.draft.controls != 0 {
		s.heroProfileChanges(screen, s.draft.controls, r.X, y+10*u, 1)
	}
}

// drawMouseRows is the Mouse tab: the mouse buttons and the behaviour
// switches, one row each with its choices inline.
func (s *nlScreen) drawMouseRows(screen *ebiten.Image, r screenkit.Rect) {
	u := s.u()
	screenkit.Fill(screen, r, color.RGBA{8, 12, 8, 200})
	screenkit.Outline(screen, r, 1*u, color.RGBA{50, 60, 46, 255})
	bf, df := s.fonts.Body, s.fonts.Display
	rowH := 58 * u
	y := r.Y + 8*u
	for _, c := range s.controlRows() {
		v := c.get(&s.draft)
		bf.Draw(screen, c.label, r.X+16*u, y+22*u, screenkit.Style{Size: 13.5 * u, Top: nlCream, Shadow: 0.1})
		if len(c.subs) > v {
			bf.Draw(screen, c.subs[v], r.X+16*u, y+42*u, screenkit.Style{Size: 10.5 * u, Top: nlDim})
		}
		bx := r.X + 250*u
		for i, step := range c.steps {
			st := screenkit.Style{Size: 12 * u, Tracking: 0.06, Upper: true, Align: 1}
			w := max(70*u, df.Measure(step, st)+24*u)
			br := screenkit.Rect{X: bx, Y: y + 10*u, W: w, H: 32 * u}
			id := fmt.Sprintf("ctl-%s-%d", c.key, i)
			on := i == v
			screenkit.Fill(screen, br, color.RGBA{14, 16, 12, 220})
			if on {
				screenkit.VGradient(screen, br, color.RGBA{50, 110, 50, 230}, color.RGBA{20, 50, 22, 230})
			}
			border := lerpRGBA(color.RGBA{70, 70, 60, 255}, color.RGBA{160, 160, 140, 255}, s.hits.HoverAmount(id))
			if on {
				border = color.RGBA{255, 227, 138, 255}
			}
			screenkit.Outline(screen, br, 1.5*u, border)
			st.Top = map[bool]color.RGBA{false: {190, 182, 150, 255}, true: nlCream}[on]
			df.Draw(screen, step, br.X+w/2, br.Y+21*u, st)
			s.hits.Add(screenkit.Region{ID: id, Rect: br, Click: func() { s.setCard(c, i) }})
			bx += w + 6*u
		}
		y += rowH
		if y+rowH > r.Y+r.H {
			break
		}
	}
}

// drawMouse is a mouse with what each button does under the chosen
// Interface Type (DESIGN_INTERFACE_HUD_INPUT §3.5).
func (s *nlScreen) drawMouse(screen *ebiten.Image, r screenkit.Rect) {
	u := s.u()
	cx := r.X + r.W*0.28
	top := r.Y + 30*u
	w, h := 150*u, 240*u
	body := screenkit.Rect{X: cx - w/2, Y: top, W: w, H: h}
	// Body: a rounded shell from a rectangle and two discs.
	shell := color.RGBA{70, 72, 66, 255}
	screenkit.Disc(screen, cx, body.Y+w/2, w/2, shell)
	screenkit.Fill(screen, screenkit.Rect{X: body.X, Y: body.Y + w/2, W: w, H: h - w}, shell)
	screenkit.Disc(screen, cx, body.Y+h-w/2, w/2, shell)
	right := s.draft.interfaceType == 1
	btn := func(x0 float64, lit bool) {
		c := color.RGBA{96, 98, 90, 255}
		if lit {
			c = color.RGBA{86, 196, 96, 255}
		}
		screenkit.Fill(screen, screenkit.Rect{X: x0, Y: body.Y + 18*u, W: w/2 - 10*u, H: 70 * u}, c)
	}
	btn(body.X+6*u, true)
	btn(cx+4*u, right)
	screenkit.Fill(screen, screenkit.Rect{X: cx - 5*u, Y: body.Y + 30*u, W: 10 * u, H: 40 * u}, color.RGBA{30, 30, 28, 255})
	bf, df := s.fonts.Body, s.fonts.Display
	tx := cx + w/2 + 40*u
	row := func(y float64, title, text string) {
		df.Draw(screen, title, tx, y, screenkit.Style{Size: 13 * u, Tracking: 0.14, Top: nlKicker, Upper: true})
		bf.DrawWrapped(screen, text, tx, y+8*u, r.X+r.W-tx, 1.9, screenkit.Style{Size: 12 * u, Top: nlBody})
	}
	leftText := "Select, and give the armed order."
	rightText := "Deselect and cancel."
	mini := "Right-drag moves the camera; left gives the order there."
	if right {
		rightText = "Give the contextual order; cancel an armed order or placement."
		mini = "Left-drag moves the camera; right gives the order there."
	}
	row(top+10*u, "Left button", leftText)
	row(top+90*u, "Right button", rightText)
	row(top+170*u, "Wheel", "Zoom the Enhanced view in steps.")
	row(top+250*u, "On the minimap", mini)
}
