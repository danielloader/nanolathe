package main

import (
	"math"

	"github.com/nanolathe-gg/nanolathe/internal/camera"
	"github.com/nanolathe-gg/nanolathe/internal/input"
)

// Modern feel choices (DESIGN_GPU_RENDERER §16.6). Magnification moves in
// log space around the preferred lock; arriving there holds the rest of a
// gesture, and a fresh gesture must overcome extra travel to leave it.
const (
	pinchSensitivity = 2.0
	pinchStickiness  = 0.20
	pinchThreshold   = 0.12 // one stop per gesture in stepped and legacy controls
)

type battleGestures struct {
	panX, panY       float64 // fractional world pixels carried between direct deltas
	pinchActive      bool
	pinchLatched     bool
	pinchZoom        camera.Zoom // last requested factor, before the next delta
	pinchPosition    float64
	magnification    float64
	anchorX, anchorY int32
}

func (b *battleSession) applyTrackpadGestures(mouse *input.MouseState, allowed bool, x, y int32) {
	if b == nil {
		return
	}
	g := &b.gestures
	if !allowed || b.cam == nil || mouse == nil {
		*g = battleGestures{}
		return
	}
	if mouse.PanX != 0 || mouse.PanY != 0 {
		factor := float64(camera.ZoomUnit) / float64(b.cam.EffectiveZoom())
		g.panX += mouse.PanX * factor
		g.panY += mouse.PanY * factor
		dx, dy := int32(g.panX), int32(g.panY)
		g.panX -= float64(dx)
		g.panY -= float64(dy)
		oldX, oldZ := b.cam.X, b.cam.Z
		b.cam.Pan(-dx, -dy)
		// Do not bank motion into an edge; reversing should respond immediately.
		if b.cam.X != oldX-dx {
			g.panX = 0
		}
		if b.cam.Z != oldZ-dy {
			g.panY = 0
		}
		b.cam.ClearFollow()
		b.pendingFollowInput = nil
	}
	style := b.cameraControlStyle()
	lock := b.zoomLock()
	if style.disabled() {
		g.pinchActive = false
		return
	}
	for _, event := range mouse.Pinches {
		if event.Began {
			if style.modern() {
				b.zoom.CancelWheel()
			}
			g.pinchActive = true
			g.pinchLatched = false
			g.magnification = 0
			live := b.cam.EffectiveZoom()
			live = camera.SnapZoom(live, live, lock)
			g.pinchZoom = live
			g.pinchPosition = pinchPositionAt(live, lock)
			g.anchorX, g.anchorY = beamAnchor(x, y)
		}
		if event.Cancelled {
			g.pinchActive = false
		}
		if g.pinchActive && !g.pinchLatched {
			if style != battleZoomSmooth {
				g.magnification += event.Delta
				if math.Abs(g.magnification) >= pinchThreshold {
					// Each gesture spends one stop, including one against a limit.
					g.pinchLatched = true
					var next camera.Zoom
					var ok bool
					if style == battleZoomLegacy {
						next, ok = camera.NextZoomStep(b.cam.RequestedZoom(), g.magnification > 0)
					} else {
						next, ok = camera.NextZoomStop(b.zoom.Target(b.cam), b.cam.MinZoom(), lock, g.magnification > 0)
					}
					if ok {
						if style == battleZoomLegacy {
							b.zoom.SetTargetLegacy(b.cam, g.anchorX, g.anchorY, next)
						} else {
							b.zoom.SetTarget(b.cam, g.anchorX, g.anchorY, next)
						}
						b.cam.ClearFollow()
						b.pendingFollowInput = nil
					}
				}
			} else {
				g.pinchPosition += event.Delta * pinchSensitivity
				// Discard excess at either limit so reversing responds immediately.
				g.pinchPosition = min(max(g.pinchPosition, pinchPositionAt(b.cam.MinZoom(), lock)), pinchPositionAt(camera.ZoomMax, lock))
				position := g.pinchPosition
				switch {
				case position > pinchStickiness:
					position -= pinchStickiness
				case position < -pinchStickiness:
					position += pinchStickiness
				default:
					position = 0
				}
				want := camera.Zoom(math.Round(math.Exp(position) * float64(lock)))
				want = camera.SnapZoom(g.pinchZoom, want, lock)
				if want == lock && g.pinchZoom != lock {
					// Catch a crossing even when one large event skips the entire snap
					// band. Discard the rest of this gesture until the fingers lift.
					g.pinchLatched = true
				}
				g.pinchZoom = want
				// Follow the fingers directly; only wheel input uses the short ease.
				b.cam.SetZoomAbout(g.anchorX, g.anchorY, want)
				b.cam.ClearFollow()
				b.pendingFollowInput = nil
				b.zoom.Reset()
			}
		}
		if event.Ended {
			g.pinchActive = false
		}
	}
}

// Invert the sticky stretch when a new gesture starts at an arbitrary factor.
func pinchPositionAt(zoom, lock camera.Zoom) float64 {
	position := math.Log(float64(zoom) / float64(lock))
	if position > 0 {
		return position + pinchStickiness
	}
	if position < 0 {
		return position - pinchStickiness
	}
	return 0
}
