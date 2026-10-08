package main

import (
	"github.com/nanolathe-gg/nanolathe/internal/camera"
	"github.com/nanolathe-gg/nanolathe/internal/client"
	"github.com/nanolathe-gg/nanolathe/internal/hud"
	"github.com/nanolathe-gg/nanolathe/internal/settings"
	"github.com/nanolathe-gg/nanolathe/internal/ui"
)

// Modern sidebar scale (DESIGN_INTERFACE_HUD_INPUT "Modern sidebar scale").
// The rail is laid out on a virtual surface 1/k of the framebuffer's height
// and magnified k times; the minimap is drawn outside that region at its
// magnified size from a picture built at that size, so it stays sharp. The
// top and bottom strips, and the overlays anchored to the viewport's left
// edge, keep their scale and move right by the rail's extra width.

// resolveChromeScale fixes the sidebar magnification for the frame being
// drawn; input until the next draw maps the pointer through the same value,
// which is what the player sees. It is 1 wherever the classic executor may
// replay the recording, which ignores the region markers, and for captures
// that crop the chrome at retail's fixed insets (films, `--shot-renderer
// both`, Nanolathe screen previews).
func (b *battleSession) resolveChromeScale() {
	if b == nil {
		return
	}
	b.chromeK = 1
	if b.cl == nil || !b.cl.Enhanced() || b.chromeFixed || b.preview {
		return
	}
	_, h := b.cl.Size()
	b.chromeK = hud.ChromeScale(b.hostPreferences().SidebarScale, int32(h), settings.MaxChromeScale)
}

// chromeScale is the sidebar magnification resolveChromeScale last fixed.
func (b *battleSession) chromeScale() int32 {
	if b == nil {
		return 1
	}
	return max(b.chromeK, 1)
}

// railRegion magnifies the rail about the framebuffer origin.
func (b *battleSession) railRegion() client.ChromeRegion {
	return client.ChromeRegion{Scale: b.chromeScale()}
}

// stripRegion moves the horizontal strips and viewport-anchored overlays right
// by the width the magnified rail adds.
func (b *battleSession) stripRegion() client.ChromeRegion {
	return client.ChromeRegion{Scale: 1, OffsetX: railInset(b.chromeScale()) - camera.OriginX}
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

// railPointerFrame maps a widget pass's pointer and events onto the rail. The
// mapped events reuse the session's buffer; the frame is consumed within the
// pass.
func (b *battleSession) railPointerFrame(frame ui.WidgetFrame) ui.WidgetFrame {
	if b == nil || b.railRegion().Identity() {
		return frame
	}
	frame.PointerX, frame.PointerY = b.railPointer(frame.PointerX, frame.PointerY)
	events := b.railEvents[:0]
	for _, event := range frame.PointerEvents {
		event.X, event.Y = b.railPointer(event.X, event.Y)
		events = append(events, event)
	}
	b.railEvents = events
	frame.PointerEvents = events
	return frame
}

// stripPointer maps a framebuffer pointer onto the strips' surface.
func (b *battleSession) stripPointer(x, y int32) (int32, int32) {
	return b.stripRegion().ToVirtual(x, y)
}

// railInset is the camera's left inset beside a rail magnified k times: the
// magnified PANELSIDE covers 129k columns, and retail's 128 is that less one
// [03 §4.1][07 §6].
func railInset(k int32) int32 { return hud.ChromeRailX*k - 1 }

// syncChromeInsets widens the camera's left inset to the magnified rail, so
// the clamp keeps every playable column reachable beside it [03 §4.1]. The
// point at the viewport's centre stays there, which also re-centres the
// battle-start placement made before the first draw knew the scale.
func (b *battleSession) syncChromeInsets() {
	if b == nil || b.cam == nil {
		return
	}
	want := camera.ChromeInsets{}
	if k := b.chromeScale(); k > 1 {
		want.Left = railInset(k)
	}
	if b.cam.Chrome == want {
		return
	}
	ox, oz := b.cam.BattleViewOrigin()
	w, h := b.cam.BattleView()
	b.cam.Chrome = want
	b.cam.JumpToBattleViewCenter(ox+w/2, oz+h/2)
}
