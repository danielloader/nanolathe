package main

import (
	"github.com/nanolathe-gg/nanolathe/internal/camera"
	"github.com/nanolathe-gg/nanolathe/internal/client"
	"github.com/nanolathe-gg/nanolathe/internal/hud"
	"github.com/nanolathe-gg/nanolathe/internal/input"
	"github.com/nanolathe-gg/nanolathe/internal/ui"
)

// Modern sidebar scale (DESIGN_INTERFACE_HUD_INPUT "Modern sidebar scale").
// The rail is laid out on a virtual surface 1/k of the framebuffer's height
// and magnified k times; the minimap is drawn outside that region at its
// magnified size from a picture built at that size, so it stays sharp. The
// top and bottom strips, and the overlays anchored to the viewport's left
// edge, keep their scale and move right by the rail's extra width.

// chromeScale is this frame's sidebar magnification: always 1 under the
// classic executor, which cannot replay a magnified region.
func (b *battleSession) chromeScale() int32 {
	if b == nil || b.cl == nil || !b.cl.Enhanced() {
		return 1
	}
	_, h := b.cl.Size()
	return hud.ChromeScale(b.hostPreferences().SidebarScale, int32(h))
}

// railRegion magnifies the rail about the framebuffer origin.
func (b *battleSession) railRegion() client.ChromeRegion {
	return client.ChromeRegion{Scale: b.chromeScale()}
}

// stripRegion moves the horizontal strips and viewport-anchored overlays right
// by the width the magnified rail adds.
func (b *battleSession) stripRegion() client.ChromeRegion {
	return client.ChromeRegion{Scale: 1, OffsetX: camera.OriginX * (b.chromeScale() - 1)}
}

// railSize is the virtual surface the rail's windows are laid out on.
func (b *battleSession) railSize() (int, int) {
	if b == nil || b.cl == nil {
		return 0, 0
	}
	w, h := b.cl.Size()
	return b.railRegion().VirtualSize(w, h)
}

// railPointer maps a framebuffer pointer onto the rail's virtual surface.
func (b *battleSession) railPointer(x, y int32) (int32, int32) {
	return b.railRegion().ToVirtual(x, y)
}

// railPointerFrame maps a widget pass's pointer and events onto the rail.
func (b *battleSession) railPointerFrame(frame ui.WidgetFrame) ui.WidgetFrame {
	if b.railRegion().Identity() {
		return frame
	}
	frame.PointerX, frame.PointerY = b.railPointer(frame.PointerX, frame.PointerY)
	events := make([]input.PointerEvent, len(frame.PointerEvents))
	for i, event := range frame.PointerEvents {
		event.X, event.Y = b.railPointer(event.X, event.Y)
		events[i] = event
	}
	frame.PointerEvents = events
	return frame
}

// stripPointer maps a framebuffer pointer onto the strips' surface.
func (b *battleSession) stripPointer(x, y int32) (int32, int32) {
	return b.stripRegion().ToVirtual(x, y)
}

// syncChromeInsets widens the camera's left inset to the magnified rail, so
// the clamp keeps every playable column reachable beside it [03 §4.1].
func (b *battleSession) syncChromeInsets() {
	if b == nil || b.cam == nil {
		return
	}
	want := camera.ChromeInsets{}
	if k := b.chromeScale(); k > 1 {
		want.Left = camera.OriginX * k
	}
	if b.cam.Chrome != want {
		b.cam.Chrome = want
		b.cam.Clamp()
	}
}
