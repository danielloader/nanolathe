package client

import (
	"github.com/nanolathe-gg/nanolathe/internal/frame"
	"github.com/nanolathe-gg/nanolathe/internal/sim/numeric"
)

// retainNoticeViewport rebuilds the on-screen identities used by the next
// presentation's under-attack admission [07 R-REV-01 §5][07 R-HUD-03 §14.1].
// It uses committed definition bounds, not selected units or painted pixels.
// Camera and membership remain presentation-owned [I6].
func (c *Client) retainNoticeViewport(f *frame.Frame) {
	c.noticeOnScreen = c.noticeOnScreen[:0]
	if f == nil || c.cam == nil {
		return
	}
	viewport := c.battleViewportRect()
	if viewport.W <= 0 || viewport.H <= 0 {
		return
	}
	z := c.cam.EffectiveZoom()
	whole := func(v numeric.Fixed) int32 { return int32(int16(v >> 16)) }
	for _, u := range f.Units { // committed pool-slot order [I1]
		if u.Slot == 0 || u.Model == "" {
			continue
		}
		// Shift each coordinate and extent independently before adding: the
		// producer reads signed whole words, so fractional carries are absent
		// [07 R-REV-01 §5]. The published hull holds min X/Z, max Y and spans.
		x, y, wz := whole(u.X), whole(u.Y), whole(u.Z)
		minX, minZ := whole(u.HullOffsetX), whole(u.HullOffsetZ)
		maxX := whole(u.HullOffsetX + u.HullXExtent)
		maxZ := whole(u.HullOffsetZ + u.HullZExtent)
		topY := y + whole(u.HullOffsetY)
		bottomY := y + whole(u.HullOffsetY-u.HullYExtent)
		if u.MoverMode != 1 && c.terrain != nil {
			if cell := c.terrain.PlotAt(int32(u.X>>20), int32(u.Z>>20)); cell != nil && int32(cell.Height()) < bottomY {
				bottomY = int32(cell.Height())
			}
		}
		// This camera names the framebuffer origin; retail names the viewport
		// origin. The viewport bias is absorbed by that conversion, just as in
		// the drawn world (DESIGN_INTERFACE_HUD_INPUT §2.3). Live zoom keeps the
		// bounds in the actual presented framebuffer space.
		left, right := z.Project(x+minX-c.cam.X), z.Project(x+maxX-c.cam.X)
		top := z.Project(wz + minZ - c.cam.Z - (topY >> 1))
		bottom := z.Project(wz + maxZ - c.cam.Z - (bottomY >> 1))
		if left > viewport.X+viewport.W-1 || right < viewport.X || top > viewport.Y+viewport.H-1 || bottom < viewport.Y {
			continue // exact edge contact is retained [07 R-REV-01 §5]
		}
		if !unitVisibleForFrame(f, u, f.ViewingPlayer) {
			continue
		}
		c.noticeOnScreen = append(c.noticeOnScreen, u.Slot)
	}
}
