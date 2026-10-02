package main

import (
	"fmt"
	"strings"

	"github.com/nanolathe-gg/nanolathe/internal/gui"
	"github.com/nanolathe-gg/nanolathe/internal/input"
	"github.com/nanolathe-gg/nanolathe/internal/platform/screenkit"
	"github.com/nanolathe-gg/nanolathe/internal/ui"
)

// The native panel owns input in one logical coordinate system. Drawing maps
// these same rectangles and text metrics to the device-resolution settings
// style (DESIGN_DEVELOPER_TOOLS §7); no GPU upload occurs while building it.
func (s *toolsScreen) buildPanel() {
	if s.shell == nil {
		return
	}
	w := &gui.Window{Name: "nanolathe-unit-viewer", Rect: gui.Rect{W: unitViewerWidth, H: unitViewerHeight}, Gadgets: []gui.Gadget{{Kind: gui.KindPanel}}}
	add := func(kind gui.Kind, name, text string, x, y, width, height int32) int {
		w.Gadgets = append(w.Gadgets, gui.Gadget{Kind: kind, Name: name, Text: text, Active: 1, Rect: gui.Rect{X: x, Y: y, W: width, H: height}, Attribs: 1, ColorF: 9})
		return len(w.Gadgets) - 1
	}
	button := func(name, text string, x, y, width int32) {
		i := add(gui.KindButton, name, text, x, y, width, 36)
		w.Gadgets[i].Attribs = 0x20
	}
	if !s.viewer {
		button("VIEWER", "Unit viewer", 500, 392, 200)
		button("BACK", "Back", 1040, 26, 128)
	} else {
		i := add(gui.KindTextBox, "SEARCH", s.query, 32, 146, 234, 38)
		w.Gadgets[i].MaxChars = 127
		i = add(gui.KindListBox, "UNITS", "", 32, 208, 210, 500)
		w.Gadgets[i].ItemHeight = 52
		w.Gadgets[i].Assoc = 1
		i = add(gui.KindScrollBar, "UNITSCROLL", "", 248, 208, 18, 500)
		w.Gadgets[i].Assoc, w.Gadgets[i].Attribs = 1, 0
		i = add(gui.KindListBox, "INFO", "", 936, 144, 210, 564)
		w.Gadgets[i].ItemHeight = 24
		w.Gadgets[i].Assoc, w.Gadgets[i].Attribs = 2, 0x101
		i = add(gui.KindScrollBar, "INFOSCROLL", "", 1150, 144, 18, 564)
		w.Gadgets[i].Assoc, w.Gadgets[i].Attribs = 2, 0
		button("IDLE", "Idle", 312, 642, 92)
		button("WALK", "Walk", 414, 642, 92)
		button("AIM", "Aim", 516, 642, 92)
		button("FIRE", "Fire", 618, 642, 92)
		button("PAUSE", "Pause", 788, 642, 108)
		button("SPIN", "Rotate: on", 312, 688, 154)
		button("RESET", "Reset view", 478, 688, 142)
		button("WEAPON", "Weapon 1", 754, 688, 142)
		button("BACK", "Back", 1040, 26, 128)
		// The ordinary slider builder synthesizes associated arrow buttons.
		// Settings-sized controls keep their hit targets and painted extents
		// identical; the native panel still computes thumb size and travel.
		for i, g := range w.Gadgets {
			if g.Kind == gui.KindScrollBar {
				bar, arrows := gui.BuildSlider(g, &gui.SliderArt{BaseExtent: 18, KnobExtent: 18, ArrowExtent: 18, ArrowCrossExtent: 18})
				w.Gadgets[i] = bar
				w.Gadgets = append(w.Gadgets, arrows...)
			}
		}
		s.listRect = screenkit.Rect{X: 32, Y: 208, W: 234, H: 500}
		s.infoRect = screenkit.Rect{X: 936, Y: 144, W: 232, H: 564}
		s.viewRect = screenkit.Rect{X: 306, Y: 180, W: 596, H: 418}
	}
	s.panel = ui.NewPanel(w)
	s.shell.frontend.Open(modeMenuSingle, s.panel, false)
	if s.viewer {
		s.refreshLists()
		s.refreshControls()
		s.panel.FocusEditor(s.panel.Index("SEARCH"))
	} else {
		s.panel.SetFocus(s.panel.Index("VIEWER"))
	}
	s.tokens = nil
	s.widgetHeld = false
}

func (s *toolsScreen) refreshLists() {
	if s.panel == nil || !s.viewer {
		return
	}
	rows := make([]string, len(s.filtered))
	for i, e := range s.filtered {
		rows[i] = unitViewerName(e.Def) + "\r" + strings.ToUpper(e.Key)
	}
	s.panel.FillTextListAt(s.panel.Index("UNITS"), rows, nil, unitViewerTextMetric)
	list := s.panel.Window.Gadgets[s.panel.Index("UNITS")]
	s.visible = max(1, int(list.Rect.H)/int(list.ItemHeight))
	s.panel.SetListSelection("UNITS", s.selectionIndex(), s.visible)
	s.refreshInfo()
}

func (s *toolsScreen) refreshInfo() {
	if s.panel == nil || !s.viewer {
		return
	}
	var rows []string
	if s.selected != nil {
		measure := unitViewerTextWidth
		width := int(s.panel.Window.Gadgets[s.panel.Index("INFO")].Rect.W) - 2*unitViewerTextInset
		rows = append(rows, retailWrapLines(s.selected.Description, measure, width)...)
		rows = append(rows, "")
		for _, stat := range unitViewerStats(s.selected) {
			if measure(stat.Label)+measure(stat.Value)+12 <= width {
				rows = append(rows, stat.Label+"\t"+stat.Value)
			} else {
				rows = append(rows, stat.Label+":")
				rows = append(rows, retailWrapLines(stat.Value, measure, width)...)
			}
		}
	}
	s.panel.FillTextListAt(s.panel.Index("INFO"), rows, nil, unitViewerTextMetric)
}

func (s *toolsScreen) serviceWidgets(inX, inY float64, down, pressed, released bool, dt float64) {
	p := s.panel
	if p == nil {
		return
	}
	f := ui.WidgetFrame{PointerX: int32(inX), PointerY: int32(inY), TokenMode: true, KeyNavigation: true, Tokens: s.tokens, TimerAdvanced: dt > 0}
	inside := inX >= 0 && inX < unitViewerWidth && inY >= 0 && inY < unitViewerHeight
	if pressed && inside {
		s.widgetHeld = true
	}
	if down && s.widgetHeld {
		f.HeldButtons = 1
	}
	if pressed && inside {
		f.PointerEvents = append(f.PointerEvents, input.PointerEvent{Kind: input.LeftDown, X: int32(inX), Y: int32(inY)})
	}
	if released && s.widgetHeld {
		f.PointerEvents = append(f.PointerEvents, input.PointerEvent{Kind: input.LeftUp, X: int32(inX), Y: int32(inY)})
		s.widgetHeld = false
	}
	result := p.ServiceFrame(f, ui.WidgetHooks{
		Metric: func(int) int { return unitViewerTextMetric },
		Measure: func(index int, text string) int {
			width := unitViewerTextWidth(text)
			if p.Window.Gadgets[index].Kind == gui.KindTextBox {
				width += 2 * unitViewerTextInset
			}
			return width
		},
		ArtFrames: func(int) int { return 4 },
		Change: func(index int) {
			if index == p.Index("UNITS") {
				i := p.ListAt(index).Selected()
				if i >= 0 && i < len(s.filtered) {
					s.selectUnit(s.filtered[i].Def)
				}
			}
		},
	})
	s.tokens = s.tokens[min(len(s.tokens), result.ConsumedTokens):]
	if s.viewer && s.query != p.TextOf("SEARCH") {
		s.query = p.TextOf("SEARCH")
		s.filter()
	}
	s.searchFocus = p.EditorCaptured()
	if result.Fired {
		s.activateTool(p.Window.Gadgets[result.FiredIndex].Name)
	}
}

func (s *toolsScreen) activateTool(name string) {
	if s.shell != nil {
		s.shell.playMenuCue("SmallButton")
	}
	switch name {
	case "BACK":
		s.back()
	case "VIEWER":
		s.openViewer()
	case "SPIN":
		s.spinning = !s.spinning
	case "RESET":
		s.resetView()
	case "PAUSE":
		s.animationPaused = !s.animationPaused
	case "IDLE":
		s.chooseAnimation("Idle")
	case "WALK":
		s.chooseAnimation("Walk")
	case "AIM":
		s.chooseAnimation("Aim")
	case "FIRE":
		s.chooseAnimation("Fire")
	case "WEAPON":
		s.weapon = s.weapon%3 + 1
		s.model.setAnimation(unitViewerAction(s.action), s.weapon)
	}
	s.refreshControls()
}

func (s *toolsScreen) chooseAnimation(action string) {
	s.action, s.animationPaused = action, false
	s.model.setAnimation(unitViewerAction(action), s.weapon)
}

func (s *toolsScreen) refreshControls() {
	if s.panel == nil || !s.viewer {
		return
	}
	spin := "off"
	if s.spinning {
		spin = "on"
	}
	s.panel.SetText("SPIN", "Rotate: "+spin)
	pause := "Pause"
	if s.animationPaused {
		pause = "Play"
	}
	s.panel.SetText("PAUSE", pause)
	s.panel.SetText("WEAPON", fmt.Sprintf("Weapon %d", s.weapon))
	for _, a := range []struct{ name, action string }{{"IDLE", "Idle"}, {"WALK", "Walk"}, {"AIM", "Aim"}, {"FIRE", "Fire"}} {
		i := s.panel.Index(a.name)
		s.panel.Window.Gadgets[i].GrayedOut = 0
		if !unitViewerAnimationAvailable(s.selected, unitViewerAction(a.action), s.weapon) {
			s.panel.Window.Gadgets[i].GrayedOut = 1
		}
	}
}

func (s *toolsScreen) initializeRetail(g *gameShell) {
	assets := g.assets
	if assets == nil {
		assets = loadMenuAssets(s.cs)
	}
	s.shell = &gameShell{cs: s.cs, assets: assets, font: assets.font, frontend: ui.NewFrontend(modeMenuSingle)}
	s.buildPanel()
}
