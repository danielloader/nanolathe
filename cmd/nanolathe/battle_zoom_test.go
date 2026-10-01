package main

import (
	"testing"

	"github.com/nanolathe-gg/nanolathe/internal/camera"
	"github.com/nanolathe-gg/nanolathe/internal/gameplay"
	"github.com/nanolathe-gg/nanolathe/internal/input"
	"github.com/nanolathe-gg/nanolathe/internal/session"
	"github.com/nanolathe-gg/nanolathe/internal/settings"
	"github.com/nanolathe-gg/nanolathe/internal/ui"
)

func TestBattleCameraModeBoundary(t *testing.T) {
	for _, mode := range []gameplay.Mode{gameplay.Modern, gameplay.Community39, gameplay.Strict31} {
		for _, zoom := range []int{settings.ZoomSmooth, settings.ZoomStepped, settings.ZoomNone} {
			for _, overview := range []int{settings.OverviewZoom, settings.OverviewMegamap} {
				b := zoomTestBattle()
				b.sess = &session.Session{Gameplay: mode}
				p := settings.DefaultPresentation()
				p.ZoomStyle, p.Overview = zoom, overview
				b.hostPresentation = &p
				b.syncCameraControls()
				want := battleZoomLegacy
				mega := overview == settings.OverviewMegamap
				if mode == gameplay.Community39 {
					want, mega = battleZoomDisabled, true
				} else if mode == gameplay.Modern {
					want, mega = battleZoomSmooth, false
					if zoom == settings.ZoomStepped {
						want = battleZoomStepped
					} else if zoom == settings.ZoomNone {
						want = battleZoomNone
					}
				}
				if b.cameraControlStyle() != want || b.megamapMode() != mega || b.cam.ViewportZoomFloor == want.modern() {
					t.Fatalf("mode=%s zoom=%d overview=%d: style=%d megamap=%v viewport floor=%v", mode, zoom, overview, b.cameraControlStyle(), b.megamapMode(), b.cam.ViewportZoomFloor)
				}
			}
		}
	}
}

func TestCommunityCameraZoomIsDisabled(t *testing.T) {
	for _, size := range [][4]int32{{640, 480, 8192, 8192}, {1920, 1080, 1600, 3968}} {
		b := zoomTestBattle()
		b.cam.ViewW, b.cam.ViewH, b.cam.MapW, b.cam.MapH = size[0], size[1], size[2], size[3]
		b.sess = &session.Session{Gameplay: gameplay.Community39}
		applyEntryZoom(Options{Renderer: "modern", Zoom: camera.ZoomMax}, b)
		b.wheelZoom(400, 250, -8)
		b.applyTrackpadGestures(&input.MouseState{Pinches: []input.PinchEvent{{Began: true, Delta: 2, Ended: true}}}, true, 400, 250)
		b.toggleViewScale(true)
		b.toggleViewScale(false)
		if b.cam.EffectiveZoom() != camera.ZoomUnit || b.zoom.Active(b.cam) || !b.megamapMode() {
			t.Fatal("Community controls admitted camera zoom")
		}
	}
}

func TestModernNoZoomDisablesOnlyCameraZoom(t *testing.T) {
	for _, size := range [][4]int32{{640, 480, 8192, 8192}, {1920, 1080, 1600, 3968}} {
		b := zoomTestBattle()
		b.cam.ViewW, b.cam.ViewH, b.cam.MapW, b.cam.MapH = size[0], size[1], size[2], size[3]
		p := settings.DefaultPresentation()
		p.ZoomStyle, p.StrategicIconStyle, p.Overview = settings.ZoomNone, settings.StrategicIconsCommunity, settings.OverviewMegamap
		b.hostPresentation = &p
		b.cam.SetZoomAbout(400, 250, camera.ZoomMax)
		b.zoom.SetTarget(b.cam, 400, 250, camera.ZoomUnit/4)
		b.gestures.pinchActive = true
		b.zoomReturn.factor = camera.ZoomMax
		applyEntryZoom(Options{Renderer: "modern", Zoom: camera.ZoomMax}, b)
		b.wheelZoom(400, 250, -8)
		b.applyTrackpadGestures(&input.MouseState{Pinches: []input.PinchEvent{{Began: true, Delta: 2, Ended: true}}}, true, 400, 250)
		b.toggleViewScale(true)
		b.toggleViewScale(false)
		if b.cam.EffectiveZoom() != camera.ZoomUnit || b.zoom.Active(b.cam) || b.gestures.pinchActive || b.zoomReturn.factor != 0 || b.megamapMode() {
			t.Fatal("No zoom retained camera zoom or enabled the community megamap")
		}
		if !b.cameraControlStyle().modern() || b.hostPreferences().StrategicIconStyle != settings.StrategicIconsCommunity {
			t.Fatal("No zoom lost Modern presentation policy or the independent icon preference")
		}
	}
	b := zoomTestBattle()
	p := settings.DefaultPresentation()
	p.ZoomStyle = settings.ZoomNone
	b.hostPresentation = &p
	b.syncCameraControls()
	b.cam.X, b.cam.Z = 1000, 1000
	b.applyTrackpadGestures(&input.MouseState{PanX: 12, PanY: 8}, true, 400, 250)
	if b.cam.X != 988 || b.cam.Z != 992 {
		t.Fatal("No zoom disabled ordinary trackpad panning")
	}
	p.ZoomStyle = settings.ZoomSmooth
	b.wheelZoom(400, 250, 1)
	if b.zoom.Target(b.cam) != camera.ZoomUnit*5/4 {
		t.Fatal("enabling zoom retained the disabled input state")
	}
}

func TestModernNoZoomTabKeepsOptions(t *testing.T) {
	b, cl := megamapTestBattle(t, settings.OverviewMegamap)
	b.sess.Gameplay = gameplay.Modern
	p := settings.DefaultPresentation()
	p.ZoomStyle = settings.ZoomNone
	b.hostPresentation = &p
	cl.SetEnhanced(true)
	cl.Input().Mouse.SetPosition(300, 200)
	megamapStep(b, cl, func(in *input.State) { in.Kbd.SetKey(input.KeyTab, true) })
	if b.battleState().Modal() != ui.BattleModalOptions || b.zoomReturn.factor != 0 || b.megamapShown() {
		t.Fatal("No zoom took Tab away from Options")
	}
}

func TestModernOverviewReturnsToCombatView(t *testing.T) {
	b := zoomTestBattle()
	b.cam.SetZoomAbout(600, 400, camera.ZoomUnit*3/2)
	view, factor := b.cam.PresentationView(), b.cam.EffectiveZoom()
	b.toggleViewScale(true)
	if b.cam.EffectiveZoom() != b.cam.MinZoom() || !b.cam.TacticalAtFloor() || b.megamapShown() {
		t.Fatal("Modern overview did not fit the map")
	}
	b.toggleViewScale(true)
	if b.cam.EffectiveZoom() != factor || b.cam.PresentationView() != view || b.zoomReturn.factor != 0 {
		t.Fatalf("return lost the previous factor/origin: %v (%d,%d)", b.cam.EffectiveZoom(), b.cam.X, b.cam.Z)
	}
}

func TestSteppedPinchRetiresWheelFractionsAtBegin(t *testing.T) {
	b := zoomTestBattle()
	p := settings.DefaultPresentation()
	p.ZoomStyle = settings.ZoomStepped
	b.hostPresentation = &p
	b.millisSource = &fakeMillisSource{}
	b.wheelZoom(400, 250, 0.7)
	b.applyTrackpadGestures(&input.MouseState{Pinches: []input.PinchEvent{{Began: true, Delta: 0.05, Ended: true}}}, true, 400, 250)
	b.wheelZoom(400, 250, 0.3)
	if b.zoom.Target(b.cam) != camera.ZoomUnit {
		t.Fatal("a new pinch retained the previous wheel remainder")
	}
}

func TestModernTabUsesZoomAndF2KeepsOptions(t *testing.T) {
	b, cl := megamapTestBattle(t, settings.OverviewMegamap)
	b.sess.Gameplay = gameplay.Modern
	cl.SetEnhanced(true)
	cl.Input().Mouse.SetPosition(300, 200)
	megamapStep(b, cl, func(in *input.State) { in.Kbd.SetKey(input.KeyTab, true) })
	if b.zoomReturn.factor != 0 || b.battleState().Modal() != ui.BattleModalClosed {
		t.Fatal("Tab acted before its release")
	}
	megamapStep(b, cl, func(in *input.State) { in.Kbd.SetKey(input.KeyTab, false) })
	if b.cam.EffectiveZoom() != b.cam.MinZoom() || b.zoomReturn.factor == 0 || b.megamapShown() {
		t.Fatal("Tab release did not enter Modern overview")
	}
	megamapStep(b, cl, func(in *input.State) { in.Kbd.SetKey(input.KeyTab, true) })
	megamapStep(b, cl, func(in *input.State) { in.Kbd.SetKey(input.KeyTab, false) })
	if b.cam.EffectiveZoom() != camera.ZoomUnit || b.zoomReturn.factor != 0 {
		t.Fatal("second Tab did not return")
	}
	megamapStep(b, cl, func(in *input.State) { in.Kbd.SetKey(input.KeyF2, true) })
	if b.battleState().Modal() != ui.BattleModalOptions {
		t.Fatal("F2 no longer opens Options")
	}
}

func TestChangingZoomPolicyCancelsOldInput(t *testing.T) {
	b := zoomTestBattle()
	b.sess = &session.Session{Gameplay: gameplay.Modern}
	b.wheelZoom(400, 250, 2)
	b.gestures.pinchActive = true
	b.toggleViewScale(true)
	b.sess.Gameplay = gameplay.Community39
	b.syncCameraControls()
	if b.cam.EffectiveZoom() != camera.ZoomUnit || b.zoom.Active(b.cam) || b.gestures.pinchActive || b.zoomReturn.factor != 0 {
		t.Fatal("Community inherited Modern's gesture or overview return")
	}
	b.sess.Gameplay = gameplay.Modern
	b.syncCameraControls()
	b.wheelZoom(400, 250, 1)
	if b.zoom.Target(b.cam) != camera.ZoomUnit*5/4 {
		t.Fatal("switching back left the Modern wheel blocked")
	}
}

func TestZoomAndIconControlsEditIndependentPreferences(t *testing.T) {
	s := &nlScreen{}
	d := nlDraft{pres: settings.DefaultPresentation()}
	found := 0
	for _, c := range s.controlRows() {
		if c.key == "zoomstyle" || c.key == "iconstyle" {
			c.set(&d, 1)
			if c.get(&d) != 1 {
				t.Fatalf("%s did not retain its choice", c.key)
			}
			found++
		}
	}
	if found != 2 || d.pres.ZoomStyle != settings.ZoomStepped || d.pres.StrategicIconStyle != settings.StrategicIconsCommunity {
		t.Fatal("missing Modern zoom/icon controls")
	}
}
