package main

import (
	"fmt"
	"os"

	"github.com/nanolathe-gg/nanolathe/internal/camera"
	"github.com/nanolathe-gg/nanolathe/internal/client"
	"github.com/nanolathe-gg/nanolathe/internal/gameplay"
	"github.com/nanolathe-gg/nanolathe/internal/input"
	"github.com/nanolathe-gg/nanolathe/internal/session"
	"github.com/nanolathe-gg/nanolathe/internal/settings"
)

type battleCameraStyle uint8

const (
	battleZoomSmooth battleCameraStyle = iota
	battleZoomStepped
	battleZoomLegacy
	battleZoomDisabled
	battleZoomNone
)

// cameraControlStyle is the host's single mode boundary (§16.6). Registered
// rule sets inherit their base layer's presentation controls; this selects no
// gameplay rule and never changes an authoritative command or fingerprint.
func (b *battleSession) cameraControlStyle() battleCameraStyle {
	mode := gameplay.Modern
	if b != nil && b.sess != nil {
		mode = session.BaseModeOf(b.sess.Gameplay)
	}
	switch mode {
	case gameplay.Community39:
		return battleZoomDisabled
	case gameplay.Modern:
		switch b.hostPreferences().ZoomStyle {
		case settings.ZoomNone:
			return battleZoomNone
		case settings.ZoomStepped:
			return battleZoomStepped
		}
		return battleZoomSmooth
	default:
		return battleZoomLegacy
	}
}

func (s battleCameraStyle) modern() bool {
	return s == battleZoomSmooth || s == battleZoomStepped || s == battleZoomNone
}

func (s battleCameraStyle) disabled() bool {
	return s == battleZoomDisabled || s == battleZoomNone
}

// zoomLock resolves the Modern preference onto this battle's usable range.
// Strict and Community keep their original controls regardless of the host
// preference (DESIGN_GPU_RENDERER §16.6).
func (b *battleSession) zoomLock() camera.Zoom {
	style := b.cameraControlStyle()
	if !style.modern() || style.disabled() {
		return camera.ZoomUnit
	}
	percent := b.hostPreferences().ZoomLockPercent
	if percent < settings.ZoomLockMinPercent || percent > settings.ZoomLockMaxPercent {
		percent = settings.ZoomLockDefaultPercent
	}
	lock := camera.Zoom((int64(camera.ZoomUnit)*int64(percent) + 50) / 100)
	if b.cam != nil {
		lock = max(lock, b.cam.MinZoom())
	}
	return lock
}

// syncCameraControls cancels an input burst across a mode/settings change.
// Switching to Community returns to native before its separate megamap opens.
func (b *battleSession) syncCameraControls() {
	if b == nil || b.cam == nil {
		return
	}
	style := b.cameraControlStyle()
	b.cam.ViewportZoomFloor = !style.modern()
	lock := b.zoomLock()
	if b.cameraStyleSeen && b.cameraStyle == style && b.zoom.LockZoom == lock {
		return
	}
	changed := b.cameraStyleSeen
	b.cameraStyle, b.cameraStyleSeen = style, true
	b.zoom.Reset()
	b.zoom.LockZoom = lock
	b.gestures = battleGestures{}
	b.zoomReturn = battleZoomReturn{}
	b.zoomTabPending = false
	if style.disabled() || changed && !style.modern() {
		mx, my := beamAnchor(battleViewCentre(b.cam))
		if style.disabled() || !b.executorEnhanced {
			// Native is a scale, bypassing the legacy free-zoom floor even
			// when a large viewport is wider than this map (§16.8).
			b.cam.SetScaleAbout(mx, my, camera.ViewScaleNative)
		} else {
			b.cam.SetZoomAbout(mx, my, camera.ZoomUnit)
		}
	} else if changed && b.executorEnhanced && style == battleZoomStepped {
		current, floor := b.cam.EffectiveZoom(), b.cam.MinZoom()
		lower, hasLower := camera.NextZoomStop(current, floor, lock, false)
		upper, hasUpper := camera.NextZoomStop(current, floor, lock, true)
		// A factor already at a stop stays there. Otherwise choose the nearest
		// stop when turning steps on in the Enhanced executor. Initial sync
		// preserves entry/restart framing; Classic keeps its two exact scales
		// (DESIGN_GPU_RENDERER §16.8).
		atStop := current == floor || current == max(floor, camera.ZoomUnit/4) || current == lock || current == camera.ZoomMax
		if !atStop {
			want := lower
			if !hasLower || hasUpper && upper-current < current-lower {
				want = upper
			}
			mx, my := beamAnchor(battleViewCentre(b.cam))
			b.cam.SetZoomAbout(mx, my, want)
		}
	}
}

// Tab takes the same whole-map/return action as F9 in Modern, on release like
// the community megamap. F2 remains the battle menu (§16.8).
func (b *battleSession) serviceZoomOverviewTab(pressed bool, in *input.State, cl *client.Client) bool {
	if b == nil {
		return false
	}
	style := b.cameraControlStyle()
	if cl == nil || !cl.Enhanced() || !style.modern() || style.disabled() {
		b.zoomTabPending = false
		return false
	}
	if pressed {
		b.zoomTabPending = true
	}
	if b.zoomTabPending && (in == nil || in.Kbd == nil || !in.Kbd.KeyHeld(input.KeyTab)) {
		b.zoomTabPending = false
		b.toggleViewScale(true)
	}
	return pressed
}

// Strategic symbols are independently selectable in Modern. Community's
// separate megamap always keeps its authored icon bank (§18.7, interface §3.15).
func (b *battleSession) syncStrategicIcons(cl *client.Client) {
	if b == nil || cl == nil || b.cat == nil {
		return
	}
	p := b.hostPreferences()
	style := settings.StrategicIconsCommunity
	if b.cameraControlStyle().modern() {
		style = p.StrategicIconStyle
	}
	if b.iconStyleSeen && b.iconStyle == style && b.iconConfig == p.StrategicIconConfig {
		return
	}
	b.iconStyle, b.iconConfig, b.iconStyleSeen = style, p.StrategicIconConfig, true
	var icons *client.StrategicIconCatalog
	var err error
	if style == settings.StrategicIconsModern {
		icons, err = client.LoadStrategicIconCatalog(b.cat, "")
	} else {
		icons, err = battleStrategicIcons(b.cat, p.StrategicIconConfig, b.iconRoots)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
	}
	cl.SetStrategicIconCatalog(icons)
}
