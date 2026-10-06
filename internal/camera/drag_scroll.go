package camera

// DragScroll retains the quantized retail beam origin between cursor recenter
// steps [07 R-CAM-01 §11]. Pointer capture and release belong to the host caller.
type DragScroll struct {
	anchorX, anchorZ int32
}

// Begin captures the origin and clears the tracked unit, preserving the
// desired origin and any unfinished glide. The caller owns projectile-hold
// cancellation and pointer capture [07 R-CAM-01 §11]. Camera X/Z use the
// framebuffer origin, so quantization must follow conversion to the retail
// battle-view origin [03 §4.1].
func (d *DragScroll) Begin(c *Camera) {
	if d == nil || c == nil {
		return
	}
	x, z := c.BattleViewOrigin()
	d.anchorX, d.anchorZ = x/16, z/16
	c.Follow.Tracked = 0
	// Pointer dispatch precedes phase 10, unlike keyboard hotkeys. The host
	// already sampled tracking and glide state before dispatch; refresh both
	// after cancellation: this frame must not track the old unit, but its
	// pending glide still runs [07 R-CAM-01 §1][07 R-CAM-01 §11].
	c.LatchTracked()
}

// Step spends this frame's displacement from the recentered cursor. Signed
// division discards sub-four-pixel movement each frame; it is not accumulated
// or rounded downward. The release frame still takes this step before the
// caller ends capture. Follow state is preserved during steps [07 R-CAM-01 §11].
func (d *DragScroll) Step(c *Camera, dx, dy int32) {
	if d == nil || c == nil {
		return
	}
	leadX, _, leadZ, _ := c.clampInsets()
	c.JumpTo((d.anchorX+dx/4)*16-leadX, (d.anchorZ+dy/4)*16-leadZ)
	x, z := c.BattleViewOrigin()
	d.anchorX, d.anchorZ = x/16, z/16
	// Pointer work precedes the pending sub-ticks: they must consume the
	// desired origin this step just replaced, not an earlier glide.
	c.LatchTracked()
}
