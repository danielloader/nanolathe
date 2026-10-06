package camera

import "testing"

func TestDragScrollQuantizesRetailOriginAndSignedDelta(t *testing.T) {
	c := &Camera{X: 1, Z: 1, ViewW: 640, ViewH: 480, MapW: 3000, MapH: 3000}
	c.SetTracked(7)
	var d DragScroll
	d.Begin(c)
	if c.Tracked() != 0 {
		t.Fatal("drag entry retained tracked unit")
	}
	// The framebuffer origin (1,1) corresponds to beam origin (129,33).
	// -3 truncates to zero; +7 contributes one quantized step [07 R-CAM-01 §11].
	d.Step(c, -3, 7)
	if x, z := c.BattleViewOrigin(); x != 128 || z != 48 {
		t.Fatalf("beam origin = %d,%d, want 128,48", x, z)
	}
	c.SetTracked(9)
	d.Step(c, -3, -7)
	if x, z := c.BattleViewOrigin(); x != 128 || z != 32 {
		t.Fatalf("second beam origin = %d,%d, want 128,32", x, z)
	}
	if c.Tracked() != 9 || c.Follow.Desired != (Origin{c.X, c.Z}) {
		t.Fatal("step cleared follow or failed to synchronize desired origin")
	}
}

func TestDragScrollReanchorsAfterClamp(t *testing.T) {
	c := &Camera{ViewW: 640, ViewH: 480, MapW: 1001, MapH: 1003}
	var d DragScroll
	d.Begin(c)
	d.Step(c, 10000, 10000)
	if x, z := c.BattleViewOrigin(); x != 489 || z != 587 {
		t.Fatalf("clamped beam origin = %d,%d, want 489,587", x, z)
	}
	// Quantize the clamped origin, not the attempted displacement. A zero
	// displacement on the next (including release) frame still snaps it down.
	d.Step(c, 0, 0)
	if x, z := c.BattleViewOrigin(); x != 480 || z != 576 {
		t.Fatalf("reanchored beam origin = %d,%d, want 480,576", x, z)
	}
}

// Entry clears tracking but does not replace the previous desired origin.
// Runnable sub-ticks can finish that glide before the first later drag step
// replaces it from the saved entry anchor [07 R-CAM-01 §11].
func TestDragEntryPreservesPendingGlide(t *testing.T) {
	c := testCamera()
	c.JumpTo(128, 128)
	c.SetTracked(7)
	c.GlideTo(768, 768)
	c.LatchTracked()
	var drag DragScroll
	drag.Begin(c)
	if c.Tracked() != 0 || c.LatchedTracked() != 0 || c.Follow.Desired != (Origin{768, 768}) || !c.Follow.Gliding {
		t.Fatalf("entry tracking/glide state: %+v", c.Follow)
	}
	c.StepLatchedGlide()
	if c.X != 448 || c.Z != 448 {
		t.Fatalf("entry tick did not continue glide: %d,%d", c.X, c.Z)
	}
	// The next host pass latched the old glide before pointer dispatch. The
	// zero-displacement drag still replaces it using its entry-time anchor.
	c.LatchTracked()
	drag.Step(c, 0, 0)
	c.StepLatchedGlide()
	if c.X != 128 || c.Z != 128 || c.Follow.Desired != (Origin{128, 128}) {
		t.Fatalf("first step followed the intervening origin/glide: %d,%d desired=%+v", c.X, c.Z, c.Follow.Desired)
	}
}
