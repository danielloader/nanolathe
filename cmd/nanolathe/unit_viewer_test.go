package main

import (
	"math"
	"strings"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/nanolathe-gg/nanolathe/internal/cob"
	"github.com/nanolathe-gg/nanolathe/internal/content"
	"github.com/nanolathe-gg/nanolathe/internal/gui"
	"github.com/nanolathe-gg/nanolathe/internal/input"
	"github.com/nanolathe-gg/nanolathe/internal/model"
	"github.com/nanolathe-gg/nanolathe/internal/platform/screenkit"
	"github.com/nanolathe-gg/nanolathe/internal/sim/numeric"
	"github.com/nanolathe-gg/nanolathe/internal/ui"
)

func TestUnitViewerFilteringClearsStaleModelAndKeepsMatchingSelection(t *testing.T) {
	a := &content.UnitDef{Name: "Scout", UnitName: "ARMSCOUT"}
	b := &content.UnitDef{Name: "Heavy scout", UnitName: "CORSCOUT"}
	s := &toolsScreen{entries: []unitViewerEntry{{Key: "armscout", Def: a}, {Key: "corscout", Def: b}}, visible: 1}
	s.filter()
	s.moveSelection(1)
	if s.selected != b || s.top != 1 {
		t.Fatal("keyboard selection did not scroll into view")
	}
	s.query = "scout"
	s.filter()
	if s.selected != b {
		t.Fatal("search discarded a matching selected record")
	}
	s.query = "missing"
	s.filter()
	if s.selected != nil || len(s.filtered) != 0 || s.top != 0 || s.model.key.def != nil {
		t.Fatal("empty search retained stale selection or model")
	}
	s.query = "arm"
	s.filter()
	if s.selected != a {
		t.Fatal("clearing a no-match query did not recover a selected unit")
	}
}

func TestUnitViewerInputScopesDragZoomAndSearch(t *testing.T) {
	s := &toolsScreen{open: true, viewer: true, spinning: true, searchFocus: true, scale: 1,
		viewRect: screenkit.Rect{X: 300, Y: 100, W: 400, H: 400},
		listRect: screenkit.Rect{X: 0, Y: 100, W: 250, H: 400}}
	s.shell = &gameShell{frontend: ui.NewFrontend(modeMenuSingle)}
	s.buildPanel()
	s.resetView()
	s.updateInput(screenkit.Input{Keys: []ebiten.Key{ebiten.KeySpace}}, []rune("Scout tank"), false, 0)
	if s.query != "Scout tank" || !s.spinning {
		t.Fatal("typing spaces changed rotation or lost search text")
	}
	s.updateInput(screenkit.Input{Keys: []ebiten.Key{ebiten.KeyA}}, nil, true, 0)
	s.updateInput(screenkit.Input{}, []rune("x"), false, 0)
	s.updateInput(screenkit.Input{Keys: []ebiten.Key{ebiten.KeyBackspace}}, nil, false, 0)
	if s.query != "" {
		t.Fatal("native editor select-all replacement or backspace failed")
	}
	s.query, s.selectAll = strings.Repeat("x", 127), true
	s.panel.SetText("SEARCH", s.query)
	s.panel.FocusEditor(s.panel.Index("SEARCH"))
	s.updateInput(screenkit.Input{}, []rune("a"), false, 0)
	if s.query != "a" {
		t.Fatal("select-all could not replace a full search field")
	}
	s.updateInput(screenkit.Input{X: 400, Y: 200, Pressed: true, Down: true}, nil, false, 0)
	yaw, pitch := s.yaw, s.pitch
	s.updateInput(screenkit.Input{X: 430, Y: 220, Down: true}, nil, false, 0.1)
	if s.yaw != yaw+2700 || s.pitch != math.Mod(pitch+1800, 65536) || !s.dragging || s.searchFocus {
		t.Fatalf("drag included auto-spin or wrong input owner: yaw=%v pitch=%v", s.yaw, s.pitch)
	}
	s.updateInput(screenkit.Input{X: 430, Y: 220, Released: true}, nil, false, 0.1)
	if s.dragging || s.yaw <= yaw+2700 {
		t.Fatal("rotation did not resume after drag release")
	}
	s.panel.SetFocus(s.panel.Index("UNITS"))
	s.updateInput(screenkit.Input{Keys: []ebiten.Key{ebiten.KeyArrowLeft}}, nil, false, 0)
	if s.panel.Focused() != s.panel.Index("UNITS") {
		t.Fatal("orbit shortcut also moved widget focus")
	}
	s.panel.SetFocus(s.panel.Index("SPIN"))
	s.updateInput(screenkit.Input{Keys: []ebiten.Key{ebiten.KeySpace}}, []rune{' '}, false, 0)
	yaw = s.yaw
	s.updateInput(screenkit.Input{}, nil, false, 0.1)
	if s.yaw != yaw {
		t.Fatal("paused view rotated")
	}
	s.updateInput(screenkit.Input{X: 50, Y: 200, WheelY: 100}, nil, false, 0)
	if s.zoom != 1 {
		t.Fatal("list wheel changed the camera")
	}
	for range 5 {
		s.updateInput(screenkit.Input{X: 400, Y: 200, WheelY: 100}, nil, false, 0)
	}
	if s.zoom != 3 {
		t.Fatalf("upper zoom bound = %v", s.zoom)
	}
	for range 5 {
		s.updateInput(screenkit.Input{X: 400, Y: 200, WheelY: -100}, nil, false, 0)
	}
	if s.zoom != 0.35 {
		t.Fatalf("lower zoom bound = %v", s.zoom)
	}
}

func TestUnitViewerExitWaitsForGestureAndLoader(t *testing.T) {
	s := &toolsScreen{open: true, viewer: true}
	s.updateInput(screenkit.Input{Keys: []ebiten.Key{ebiten.KeyEscape}, Held: true}, nil, false, 0)
	if !s.Active() || !s.closing || !s.viewer {
		t.Fatal("viewer Escape must close directly after consuming the held gesture")
	}
	s.loading = make(chan unitViewerLoad, 1)
	s.updateInput(screenkit.Input{}, nil, false, 0)
	if !s.Active() {
		t.Fatal("content could unmount while its loader still reads it")
	}
	s.loading <- unitViewerLoad{}
	s.pollLoad()
	s.updateInput(screenkit.Input{Held: true}, nil, false, 0)
	if !s.Active() {
		t.Fatal("loader completion bypassed the gesture release")
	}
	s.updateInput(screenkit.Input{}, nil, false, 0)
	if s.Active() {
		t.Fatal("screen remained active after both barriers completed")
	}
}

func TestUnitViewerToolsKeyboardNavigation(t *testing.T) {
	s := &toolsScreen{open: true, entries: []unitViewerEntry{}, shell: &gameShell{frontend: ui.NewFrontend(modeMenuSingle)}}
	s.buildPanel()
	if s.panel.Focused() != s.panel.Index("VIEWER") {
		t.Fatal("Tools must initially focus Unit viewer")
	}
	s.updateInput(screenkit.Input{Keys: []ebiten.Key{ebiten.KeyTab}}, nil, false, 0)
	if s.panel.Focused() != s.panel.Index("BACK") {
		t.Fatal("Tab did not reach Previous")
	}
	s.updateInput(screenkit.Input{Keys: []ebiten.Key{ebiten.KeyEnter}}, nil, false, 0)
	if !s.closing || s.viewer {
		t.Fatal("Enter did not activate the focused Previous button")
	}
	s.closing = false
	s.buildPanel()
	s.updateInput(screenkit.Input{Keys: []ebiten.Key{ebiten.KeyEnter}}, nil, false, 0)
	if !s.viewer {
		t.Fatal("Enter did not open the initially focused Unit viewer")
	}
}

func TestUnitViewerOrbitKeepsTiltInDisplaySpace(t *testing.T) {
	// Nanolathe viewer policy: a turntable's vertical axis must not wobble
	// as yaw changes, including at and across either vertical orbit pole.
	angles := []uint16{0, 1, 8192, 16384, 24576, 32768, 40960, 49152, 57344, 65535}
	for _, yaw := range angles {
		for _, pitch := range angles {
			want := [][3]numeric.Fixed{{1 << 16, 0, 0}, {0, 1 << 16, 0}, {0, 0, 1 << 16}}
			got := append([][3]numeric.Fixed(nil), want...)
			model.RotatePoints(want, yaw, 0, 0)
			model.RotatePoints(want, 0, pitch, 0)
			h, p, b := unitViewerOrientation(yaw, pitch)
			model.RotatePoints(got, h, p, b)
			for i := range want {
				for axis := range want[i] {
					// Converting three angles back to uint16 and per-axis
					// vertex rounding can differ by a few fixed-point bits.
					if numeric.Abs(int64(got[i][axis]-want[i][axis])) > 32 {
						t.Fatalf("orbit %d/%d: basis %d axis %d = %d, want %d", yaw, pitch, i, axis, got[i][axis], want[i][axis])
					}
				}
			}
		}
	}
}

func TestUnitViewerFitIncludesParentsButExcludesSelectionAndLocators(t *testing.T) {
	f := func(n int) numeric.Fixed { return numeric.Fixed(n) << 16 }
	m := &model.Model{Root: 0, Pieces: []model.Piece{
		{Parent: -1, Selection: true, Vertices: [][3]numeric.Fixed{{f(1000), 0, 0}, {0, 0, f(6)}}, Primitives: []model.Primitive{{IsColored: 1, VertexIndices: []uint16{0, 0, 0}}, {IsColored: 1, VertexIndices: []uint16{1, 1, 1}}}},
		{Parent: 0, Translate: [3]numeric.Fixed{f(3), 0, 0}, Vertices: [][3]numeric.Fixed{{f(4), 0, 0}}, Primitives: []model.Primitive{{IsColored: 1, VertexIndices: []uint16{0, 0, 0}}}},
		{Parent: 0, Vertices: [][3]numeric.Fixed{{f(2000), 0, 0}}, Primitives: []model.Primitive{{IsColored: 1, VertexIndices: []uint16{0}}, {VertexIndices: []uint16{0, 0, 0}}}},
	}}
	if got := unitViewerRadius(m, nil); got != math.Sqrt(85)/2 {
		t.Fatalf("fit radius = %v, want centered bounds containing parent offset and drawable vertices", got)
	}
}

func TestUnitViewerCreationPoseIsBoundedAndDoesNotAdvanceSleep(t *testing.T) {
	// Independently authored COB instructions [fmt cob]: hide first piece,
	// sleep, then hide second. Only the creation-time barrier is allowed.
	const hide, push, sleep, ret, jump = 0x10006000, 0x10021000, 0x10013000, 0x10065000, 0x10064000
	def := &content.UnitDef{Script: &cob.Program{Pieces: []string{"flash", "body"}, Scripts: map[string]int{"Create": 0}, ScriptsByID: []int{0},
		Code: []uint32{hide, 0, push, 1000, sleep, hide, 1, push, 0, ret}}}
	mdl := &model.Model{Root: 0, Pieces: []model.Piece{
		{Name: "body", Parent: -1, Vertices: make([][3]numeric.Fixed, 3)},
		{Name: "flash", Parent: 0, Vertices: make([][3]numeric.Fixed, 3)},
	}}
	poses, note := unitViewerCreationPose(def, mdl)
	if len(poses) != 2 || !poses[1].Hidden || poses[0].Hidden || note != "Preview pose / rotation only" {
		t.Fatalf("creation snapshot lost piece mapping or advanced time: %+v, %s", poses, note)
	}
	def.Script.Code = []uint32{jump, 0}
	if poses, note := unitViewerCreationPose(def, mdl); poses != nil || note != "Authored pose / script unavailable" {
		t.Fatalf("unbounded Create did not fall back: %+v, %s", poses, note)
	}
}

func TestUnitViewerPreviewEntryIsConfinedToMainMenu(t *testing.T) {
	previous := toolsScreenInst
	t.Cleanup(func() { toolsScreenInst = previous })
	for _, tc := range []struct {
		name                                               string
		mode                                               shellMode
		ctrl, shift, alt, held, editor, child, modal, want bool
	}{
		{name: "main menu", ctrl: true, want: true},
		{name: "U alone"},
		{name: "repeat", ctrl: true, held: true},
		{name: "shift chord", ctrl: true, shift: true},
		{name: "alt chord", ctrl: true, alt: true},
		{name: "single player", mode: modeMenuSingle, ctrl: true},
		{name: "skirmish", mode: modeMenuSkirmish, ctrl: true},
		{name: "editor", ctrl: true, editor: true},
		{name: "child window", ctrl: true, child: true},
		{name: "modal", ctrl: true, modal: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			g, p, cl := editorMenuShell(t)
			p.SetFocus(-1)
			g.frontend.SetMode(tc.mode)
			g.assets = &menuAssets{}
			g.cs = &contentSet{}
			g.cs.preview.catalogOnce.Do(func() { g.cs.preview.catalog = &content.Catalog{} })
			s := &toolsScreen{}
			toolsScreenInst = s
			t.Cleanup(s.release)
			if tc.editor {
				p.FocusEditor(1)
			}
			if tc.child {
				child := ui.NewPanel(p.Window)
				child.SetFocus(-1)
				g.frontend.Panels.Push(child)
			}
			if tc.modal {
				g.frontend.Panels.PushModal(ui.NewPanel(p.Window))
			}
			in := cl.Input()
			in.Kbd.SetKey(input.KeyCtrl, tc.ctrl)
			in.Kbd.SetKey(input.KeyShift, tc.shift)
			in.Kbd.SetKey(input.KeyAlt, tc.alt)
			in.Kbd.SetKey(input.KeyU, true)
			if tc.held {
				in.Kbd.ResetEdges()
			}
			in.EnqueueToken(input.Token{Kind: input.TokenText, Rune: 'u'})
			g.menuInput(cl)
			if s.Active() != tc.want || s.viewer != tc.want {
				t.Fatalf("preview active/viewer = %t/%t, want %t", s.Active(), s.viewer, tc.want)
			}
			if tc.want && (in.PendingTokens() != 0 || s.query != "" || g.activePanel() != p) {
				t.Fatal("preview entry leaked shortcut text or replaced its main-menu parent")
			}
		})
	}
}

func TestUnitViewerPreviewHasNoMainMenuButton(t *testing.T) {
	w := &gui.Window{Rect: gui.Rect{W: retailScreenW, H: retailScreenH}, Gadgets: []gui.Gadget{
		{Kind: gui.KindPanel},
		{Kind: gui.KindButton, Name: "SINGLE", Active: 1, Rect: gui.Rect{W: 120, H: 30}},
	}}
	g := &gameShell{frontend: ui.NewFrontend(modeMenuMain), assets: &menuAssets{panel: map[shellMode]*retailPanelAssets{modeMenuMain: {window: w}}}}
	g.openMenu(modeMenuMain)
	p := g.activePanel()
	if p.Index("TOOLS") >= 0 || p.Index("MODS") < 0 {
		t.Fatal("main menu exposes the preview or lost the Nanolathe button")
	}
}

func TestUnitViewerNativeControlsExcludeLetterboxPresses(t *testing.T) {
	a, b := &content.UnitDef{Name: "Alpha"}, &content.UnitDef{Name: "Bravo"}
	s := &toolsScreen{viewer: true, entries: []unitViewerEntry{{Key: "a", Def: a}, {Key: "b", Def: b}}, shell: &gameShell{frontend: ui.NewFrontend(modeMenuSingle)}}
	s.buildPanel()
	s.filter()
	for _, name := range []string{"UNITSCROLL", "INFOSCROLL"} {
		bar := s.panel.Window.Gadgets[s.panel.Index(name)]
		if bar.Attribs&1 != 0 || bar.Rect.W >= bar.Rect.H {
			t.Fatal("vertical list has horizontal scrollbar behavior")
		}
	}
	list := s.panel.Window.Gadgets[s.panel.Index("UNITS")]
	y := float64(list.Rect.Y + int32(list.ItemHeight) + 5)
	s.serviceWidgets(float64(list.Rect.X+8), y, false, false, false, 0)
	s.serviceWidgets(-20, y, true, true, false, 0)
	s.serviceWidgets(-20, y, false, false, true, 0)
	if s.selected != a {
		t.Fatal("letterbox click acted on the last in-window pointer")
	}
	s.serviceWidgets(float64(list.Rect.X+8), y, true, true, false, 0)
	s.serviceWidgets(float64(list.Rect.X+8), y, false, false, true, 0)
	if s.selected != b {
		t.Fatal("native list did not select its clicked row")
	}
}
