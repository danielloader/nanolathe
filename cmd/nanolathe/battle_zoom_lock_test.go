package main

import (
	"testing"

	"github.com/nanolathe-gg/nanolathe/internal/camera"
	"github.com/nanolathe-gg/nanolathe/internal/gameplay"
	"github.com/nanolathe-gg/nanolathe/internal/input"
	"github.com/nanolathe-gg/nanolathe/internal/session"
	"github.com/nanolathe-gg/nanolathe/internal/settings"
)

func preferredZoomTestBattle(percent int) *battleSession {
	b := zoomTestBattle()
	p := settings.DefaultPresentation()
	p.ZoomLockPercent = percent
	b.hostPresentation = &p
	b.syncCameraControls()
	return b
}

func TestPinchPreferredZoomLockAndDeparture(t *testing.T) {
	b := preferredZoomTestBattle(120)
	const lock camera.Zoom = 1229
	b.cam.SetZoomAbout(500, 300, camera.ZoomMax)
	apply := func(event input.PinchEvent) {
		b.applyTrackpadGestures(&input.MouseState{Pinches: []input.PinchEvent{event}}, true, 500, 300)
	}
	apply(input.PinchEvent{Began: true, Delta: -.6})
	if b.cam.EffectiveZoom() != lock || !b.gestures.pinchLatched {
		t.Fatal("large pinch missed the configured stop")
	}
	apply(input.PinchEvent{Delta: -.5, Ended: true})
	if b.cam.EffectiveZoom() != lock {
		t.Fatal("a held pinch passed the configured stop")
	}
	apply(input.PinchEvent{Began: true, Delta: -.01, Ended: true})
	if b.cam.EffectiveZoom() != lock {
		t.Fatal("a small new pinch lost the sticky resistance")
	}
	apply(input.PinchEvent{Began: true, Delta: -.4, Ended: true})
	if got := b.cam.EffectiveZoom(); got >= lock*85/100 || got <= b.cam.MinZoom() {
		t.Fatalf("a deliberate fresh pinch did not leave the lock: %d", got)
	}
	if b.zoom.LockZoom != lock {
		t.Fatal("direct pinch lost the wheel's configured stop")
	}
}

func TestPinchPreferredLockLeavesNativeFree(t *testing.T) {
	b := preferredZoomTestBattle(150)
	b.cam.SetZoomAbout(500, 300, 819)
	b.applyTrackpadGestures(&input.MouseState{Pinches: []input.PinchEvent{{Began: true, Delta: .15, Ended: true}}}, true, 500, 300)
	if got := b.cam.EffectiveZoom(); got <= camera.ZoomUnit || got >= b.zoomLock()*85/100 || b.gestures.pinchLatched {
		t.Fatalf("native retained its old pinch detent: %d", got)
	}
}

func TestSteppedPinchUsesPreferredStop(t *testing.T) {
	b := preferredZoomTestBattle(120)
	b.hostPresentation.ZoomStyle = settings.ZoomStepped
	b.syncCameraControls()
	b.cam.SetZoomAbout(500, 300, camera.ZoomMax)
	b.applyTrackpadGestures(&input.MouseState{Pinches: []input.PinchEvent{{Began: true, Delta: -.2, Ended: true}}}, true, 500, 300)
	if got := b.zoom.Target(b.cam); got != 1229 {
		t.Fatalf("stepped pinch chose %d instead of preferred stop", got)
	}
}

func TestPreferredLockChangeRetiresOldInput(t *testing.T) {
	b := preferredZoomTestBattle(120)
	b.gestures.pinchActive = true
	b.millisSource = &fakeMillisSource{}
	b.wheelZoom(400, 250, .7)
	b.hostPresentation.ZoomLockPercent = 150
	b.syncCameraControls()
	b.wheelZoom(400, 250, .3)
	if b.gestures.pinchActive || b.zoom.Target(b.cam) != camera.ZoomUnit || b.zoom.LockZoom != 1536 {
		t.Fatal("changing the lock retained an old pinch or wheel fraction")
	}
}

func TestPreferredLockOverviewAndLegacyBoundary(t *testing.T) {
	for _, from := range []camera.Zoom{921, 1126} {
		b := preferredZoomTestBattle(120)
		b.cam.SetZoomAbout(500, 300, from)
		b.toggleViewScale(true)
		b.toggleViewScale(true)
		want := from
		if from == 1126 {
			want = 1229
		}
		if b.cam.EffectiveZoom() != want {
			t.Fatalf("overview return from %d = %d, want %d", from, b.cam.EffectiveZoom(), want)
		}
	}
	for _, mode := range []gameplay.Mode{gameplay.Strict31, gameplay.Community39} {
		b := preferredZoomTestBattle(120)
		b.sess = &session.Session{Gameplay: mode}
		b.syncCameraControls()
		if b.zoomLock() != camera.ZoomUnit || b.zoom.LockZoom != camera.ZoomUnit {
			t.Fatalf("%s adopted the Modern lock", mode)
		}
	}
	b := preferredZoomTestBattle(1)
	if b.zoomLock() != b.cam.MinZoom() || b.hostPresentation.ZoomLockPercent != 1 {
		t.Fatal("map floor did not clamp the active lock independently of the preference")
	}
}
