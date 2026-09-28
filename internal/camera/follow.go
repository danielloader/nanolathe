package camera

import "github.com/nanolathe-gg/nanolathe/internal/pool"

// TargetPoint is a map-pixel point used by the phase-10 follow writer. Y is
// the vertical/shear component, not a screen coordinate [01 §4.4][07 §10].
type TargetPoint struct {
	X, Y, Z int32
}

// Origin is a camera origin in map pixels.
type Origin struct {
	X, Z int32
}

// DesiredOrigin converts a target point to the clamped camera origin that
// centers it in the battle viewport. The desired origin is clamped here;
// the current origin is deliberately not clamped by this helper [01 §4.4].
//
// Both halves of the conversion are the battle viewport's, not the
// framebuffer's: the recentre is BattleViewCenterOrigin's (retail's
// `point - viewportSpan/2` plus this build's leading-inset term, [07 R-CAM-01
// §12][03 §4.1]) and the clamp is the shared per-axis clamp of [07 §10]. Before
// 2026-08-31 this halved the framebuffer and clamped to framebuffer bounds, so a
// followed unit settled 64 map pixels right of the viewport's centre and could
// not be centred at all within 128 pixels of the map's west edge (defect
// PT5-01, the same frame-of-reference error the battle-start jump had).
//
// The height shear stays `target.Y/2` — a truncation toward zero — which is not
// the arithmetic shift [07 R-CRD-006 §1] specifies; that is a separate
// divergence and is deliberately not changed here.
func (c *Camera) DesiredOrigin(target TargetPoint) Origin {
	if c == nil {
		return Origin{}
	}
	spanW, spanH := c.BattleView()
	leadX, _, leadZ, _ := c.clampInsets()
	x, z := c.BattleViewCenterOrigin(target.X, target.Z-target.Y/2)
	return Origin{
		X: clampAxis(x, c.MapW, spanW, leadX),
		Z: clampAxis(z, c.MapH, spanH, leadZ),
	}
}

// LatchTracked retains the pre-hotkey target and glide for every sub-tick in
// the pending batch. BigBrother can replace this latch after its phase-2
// selection cycle. No movement occurs until a completed frame is consumed
// [01 §4.4][07 R-CAM-01 §12][I6].
func (c *Camera) LatchTracked() pool.Handle {
	if c == nil {
		return 0
	}
	c.Follow.latched = c.Follow.Tracked
	c.Follow.latchedDesired = c.Follow.Desired
	c.Follow.latchedGliding = c.Follow.Gliding
	return c.Follow.latched
}

// LatchedTracked returns the object most recently captured by LatchTracked.
func (c *Camera) LatchedTracked() pool.Handle {
	if c == nil {
		return 0
	}
	return c.Follow.latched
}

// FollowTo steps the current camera origin toward target and returns the
// clamped desired origin. It does not clamp the resulting current origin;
// callers apply Camera.Clamp after any later follow-adjacent effects such as
// shake [01 §4.4][03 §5.6].
func (c *Camera) FollowTo(target TargetPoint) Origin {
	if c == nil {
		return Origin{}
	}
	desired := c.DesiredOrigin(target)
	c.X = stepAxis(c.X, desired.X)
	c.Z = stepAxis(c.Z, desired.Z)
	return desired
}

// stepAxis applies the phase-10 bounded signed half-step. The wider delta
// keeps absolute-value handling correct for all int32 origins [01 §4.4].
func stepAxis(current, desired int32) int32 {
	delta := int64(desired) - int64(current)
	switch {
	case delta > 320:
		return int32(int64(current) + 320)
	case delta < -320:
		return int32(int64(current) - 320)
	default:
		return int32(int64(current) + delta/2)
	}
}

// StepLatchedGlide advances the pre-hotkey untracked glide once per committed
// sub-tick without consuming a newly issued glide in the same host batch.
// The existing half-step and stop-on-no-movement rule apply [07 R-CAM-01 §12].
func (c *Camera) StepLatchedGlide() {
	if c == nil || !c.Follow.latchedGliding {
		return
	}
	x, z := c.X, c.Z
	c.X = stepAxis(c.X, c.Follow.latchedDesired.X)
	c.Z = stepAxis(c.Z, c.Follow.latchedDesired.Z)
	if x == c.X && z == c.Z {
		c.Follow.latchedGliding = false
		if c.Follow.Desired == c.Follow.latchedDesired {
			c.Follow.Gliding = false
		}
	}
}

// Shake applies one phase-10 shake displacement to the current origin, after
// the follow step, and clamps the result [07 R-CAM-01 §10][01 §4.4.1].
//
// Retail's shake moves only the current origin and never the desired one, and
// phase 10 steps the current origin toward the desired origin on every pass,
// so each jolt is pulled back at the bounded half-step rate and the view ends
// where it started. This build keeps no live desired origin for an idle,
// untracked camera: the scroll pass writes the current origin alone and a
// glide is armed only when a writer asks for one. Without an anchor the jolts
// would be a permanent random walk, and a large or repeated shake (a
// commander's death blast) could leave the view screens away (issue #34).
// So when nothing is tracked and no glide is in flight, the pre-shake origin
// becomes the desired origin — the value retail's desired origin holds for a
// settled camera — and the glide step returns the view to it. A tracked
// follow and a glide already in flight keep their own desired origin, which
// the shake does not change.
//
// The half-step stalls one pixel short of its target, so a return glide can
// end one pixel away from the rest origin while the shake is still running.
// A desired origin within that pixel is kept rather than replaced, so repeated
// jolts cannot walk the rest origin away a pixel at a time.
func (c *Camera) Shake(dx, dz int32) {
	if c == nil || (dx == 0 && dz == 0) {
		return
	}
	if c.Follow.latched == 0 {
		rest := Origin{X: c.X, Z: c.Z}
		if withinOnePixel(c.Follow.Desired, rest) {
			rest = c.Follow.Desired
		}
		if !c.Follow.latchedGliding {
			c.Follow.latchedDesired = rest
			c.Follow.latchedGliding = true
			if !c.Follow.Gliding {
				c.Follow.Desired = rest
				c.Follow.Gliding = true
			}
		}
	}
	c.X += dx
	c.Z += dz
	c.Clamp()
}

// withinOnePixel reports whether two origins differ by at most the one-pixel
// stall of the phase-10 half-step on each axis.
func withinOnePixel(a, b Origin) bool {
	dx, dz := int64(a.X)-int64(b.X), int64(a.Z)-int64(b.Z)
	return dx >= -1 && dx <= 1 && dz >= -1 && dz <= 1
}
