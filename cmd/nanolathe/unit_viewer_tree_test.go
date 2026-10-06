package main

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/nanolathe-gg/nanolathe/internal/content"
	"github.com/nanolathe-gg/nanolathe/internal/platform/screenkit"
	"github.com/nanolathe-gg/nanolathe/internal/ui"
)

// An authored fixture: a lab whose retail list is cut short, a constructor
// with no retained authored list, a non-builder with a menu and a record
// hidden by a duplicate name.
func unitViewerTreeFixture() (*content.Catalog, map[string]*content.UnitDef) {
	d := map[string]*content.UnitDef{
		"con":    {UnitName: "CON", Name: "Constructor", Builder: true, WorkerTime: 90, BuildTime: 100},
		"lab":    {UnitName: "LAB", Name: "Lab", Builder: true, WorkerTime: 59, BuildTime: 300},
		"plane":  {UnitName: "PLANE", Name: "Plane", BuildTime: 64},
		"rock":   {UnitName: "ROCK", Name: "Rock"},
		"tank":   {UnitName: "TANK", Name: "Tank", BuildTime: 12},
		"hidden": {UnitName: "TANK", Name: "Tank copy", Builder: true},
	}
	cat := &content.Catalog{
		Units: map[string]*content.UnitDef{"con": d["con"], "lab": d["lab"], "plane": d["plane"], "rock": d["rock"], "tank": d["tank"], "tank copy": d["hidden"]},
		BuildMenus: map[string]*content.BuildMenuPage{
			// Names resolve case-insensitively; unknown and repeated names add nothing.
			"lab":  {Buttons: []string{"Tank", "MISSING"}, AuthoredButtons: []string{"Tank", "MISSING", "plane", "tank"}},
			"con":  {Buttons: []string{"lab"}},
			"rock": {Buttons: []string{"tank"}},
		},
	}
	return cat, d
}

func TestUnitViewerBuildTreeMarksModernAndFollowsLookup(t *testing.T) {
	cat, d := unitViewerTreeFixture()
	tree := unitViewerBuildTree(cat, unitViewerEntries(cat))
	if got, want := tree.builds[d["lab"]], []unitViewerLink{{d["plane"], true}, {d["tank"], false}}; !slices.Equal(got, want) {
		t.Fatalf("lab builds %+v, want %+v (library order, Modern-only plane)", got, want)
	}
	if got, want := tree.builtBy[d["plane"]], []unitViewerLink{{d["lab"], true}}; !slices.Equal(got, want) {
		t.Fatalf("plane built by %+v, want %+v", got, want)
	}
	// The non-builder's menu and the hidden duplicate contribute nothing.
	if got, want := tree.builtBy[d["tank"]], []unitViewerLink{{d["lab"], false}}; !slices.Equal(got, want) {
		t.Fatalf("tank built by %+v, want %+v", got, want)
	}
	if got, want := tree.builtBy[d["lab"]], []unitViewerLink{{d["con"], false}}; !slices.Equal(got, want) {
		t.Fatalf("lab built by %+v, want %+v", got, want)
	}
	if !tree.hidden[d["hidden"]] || tree.hidden[d["tank"]] || len(tree.builds[d["rock"]]) != 0 {
		t.Fatal("hidden or non-builder record entered the build tree")
	}
	s := &toolsScreen{tree: tree}
	rows := s.buildRows(d["lab"], 264)
	var modern, note bool
	for _, r := range rows {
		modern = modern || (r.Kind == unitViewerRowLink && r.Modern && r.Link == d["plane"])
		note = note || (r.Kind == unitViewerRowNote && strings.HasPrefix(r.Label, "MODERN"))
	}
	if !modern || !note {
		t.Fatal("Modern-only product lacks its tag or explanation")
	}
}

// Times are construction.WorkTicks at 30 ticks per second; unavailable
// estimates never become a number.
func TestUnitViewerWorkTimesUseTheConstructionStep(t *testing.T) {
	cat, d := unitViewerTreeFixture()
	s := &toolsScreen{tree: unitViewerBuildTree(cat, unitViewerEntries(cat))}
	for _, tc := range []struct {
		builder, product *content.UnitDef
		want             string
	}{
		{d["con"], d["tank"], "0.1 s"},  // quantum 3, buildtime 12: four exact quarter steps
		{d["lab"], d["plane"], "2.1 s"}, // quantum 1, buildtime 64: 64 steps
		{d["con"], d["lab"], "3.3 s"},   // quantum 3, buildtime 300: 100 steps
		{d["tank"], d["plane"], "no work"},
		{d["con"], d["rock"], "n/a"},
	} {
		if got := s.workText(tc.builder, tc.product); got != tc.want {
			t.Errorf("%s builds %s: %q, want %q", tc.builder.UnitName, tc.product.UnitName, got, tc.want)
		}
	}
	// Display truncates, like the minute form, so 2,999 ticks is not "100.0 s".
	for _, tc := range []struct {
		ticks int
		want  string
	}{{2999, "99.9 s"}, {3000, "1:40 min"}, {108000, "1:00:00 h"}} {
		if got := unitViewerDuration(tc.ticks); got != tc.want {
			t.Errorf("duration(%d) = %q, want %q", tc.ticks, got, tc.want)
		}
	}
}

func TestUnitViewerLinksAndHistoryKeepTheLibraryInStep(t *testing.T) {
	cat, d := unitViewerTreeFixture()
	s := &toolsScreen{open: true, viewer: true, action: "Idle", weapon: 1, infoTab: unitViewerTabBuild,
		shell: &gameShell{frontend: ui.NewFrontend(modeMenuSingle)}}
	s.entries = unitViewerEntries(cat)
	s.tree = unitViewerBuildTree(cat, s.entries)
	s.buildPanel()
	s.filter()
	s.visit(d["tank"])
	s.panel.SetText("SEARCH", "tank")
	s.query = "tank"
	s.filter()
	// Click the first Built-by entry, measured from the native list rows.
	row := slices.IndexFunc(s.infoRows, func(r unitViewerRow) bool { return r.Kind == unitViewerRowLink })
	if row < 0 || s.infoRows[row].Link != d["lab"] {
		t.Fatalf("tank's build tab has no Lab link: %+v", s.infoRows)
	}
	list := s.panel.Window.Gadgets[s.panel.Index("INFO")]
	x, y := float64(list.Rect.X+40), float64(list.Rect.Y+2)+float64(row)*float64(list.ItemHeight)+5
	s.updateInput(screenkit.Input{X: x, Y: y, Pressed: true, Down: true}, nil, false, 0)
	s.updateInput(screenkit.Input{X: x, Y: y, Released: true}, nil, false, 0)
	if s.selected != d["lab"] || s.query != "" || s.panel.TextOf("SEARCH") != "" || s.selectionIndex() < 0 {
		t.Fatalf("link did not select Lab and clear the excluding search: %v %q", s.selected, s.query)
	}
	if s.panel.Window.Gadgets[s.panel.Index("HISTBACK")].GrayedOut != 0 {
		t.Fatal("Back is unavailable after a navigation")
	}
	s.panel.SetFocus(s.panel.Index("UNITS"))
	s.updateInput(screenkit.Input{Keys: []ebiten.Key{ebiten.KeyBackspace}}, nil, false, 0)
	if s.selected != d["tank"] {
		t.Fatal("Backspace outside the search did not return to the previous unit")
	}
	s.altHeld = true
	yaw := s.yaw
	s.updateInput(screenkit.Input{Keys: []ebiten.Key{ebiten.KeyArrowRight}}, nil, false, 0)
	if s.selected != d["lab"] || s.yaw != yaw {
		t.Fatal("Alt+Right did not go forward, or also turned the model")
	}
	s.updateInput(screenkit.Input{Keys: []ebiten.Key{ebiten.KeyArrowLeft}}, nil, false, 0)
	if s.selected != d["tank"] {
		t.Fatal("Alt+Left did not go back")
	}
	// Backspace inside the search edits text instead of navigating.
	s.altHeld = false
	s.panel.SetText("SEARCH", "ta")
	s.panel.FocusEditor(s.panel.Index("SEARCH"))
	s.updateInput(screenkit.Input{Keys: []ebiten.Key{ebiten.KeyEnd}}, nil, false, 0)
	s.updateInput(screenkit.Input{Keys: []ebiten.Key{ebiten.KeyBackspace}}, nil, false, 0)
	if s.query != "t" || s.selected != d["tank"] {
		t.Fatalf("search Backspace navigated or failed to edit: %q %v", s.query, s.selected)
	}
}

// The picture worker reads the content's archives, so closing the viewer must
// wait for the file in hand before the host can unmount them.
func TestUnitViewerPicturesJoinTheLoaderOnRelease(t *testing.T) {
	fs := &nlPicFS{t: t, files: map[string][]byte{"unitpics/armpw.pcx": nlTestPCX(5, 3)}}
	var p unitViewerPictures
	p.bind(fs)
	pw, missing := &content.UnitDef{UnitName: "ARMPW"}, &content.UnitDef{UnitName: "NOPIC"}
	if p.image(pw) != nil || p.image(missing) != nil || p.endFrame() != 2 {
		t.Fatal("a picture appeared before its worker decoded it")
	}
	nlWaitFor(t, "the decoded picture", func() bool { return p.image(pw) != nil })
	nlWaitFor(t, "the missing picture", func() bool { p.image(missing); p.endFrame(); _, ok := p.images["nopic"]; return ok })
	if p.images["nopic"] != nil {
		t.Fatal("a missing picture was replaced by substitute art")
	}
	fs.hold.Lock()
	p.image(&content.UnitDef{UnitName: "LATER"})
	p.endFrame()
	nlWaitFor(t, "a read in flight", func() bool { return fs.readCount() >= 3 })
	released := make(chan struct{})
	go func() {
		p.release()
		close(released)
	}()
	select {
	case <-released:
		t.Fatal("release returned while the content was being read")
	case <-time.After(50 * time.Millisecond):
	}
	fs.hold.Unlock()
	<-released
	if p.loader != nil || p.images != nil {
		t.Fatal("release kept the loader or uploaded pictures")
	}
}
