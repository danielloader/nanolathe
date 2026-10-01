package camera

import "testing"

// The configurable stop replaces native rather than adding another detent.
// Its band remains proportional and catches large events (§16.6).
func TestPreferredZoomLockBandAndCrossings(t *testing.T) {
	const lock Zoom = 1229 // 1.20x rounded to 1/1024
	for _, tc := range []struct {
		from, want, got Zoom
	}{
		{lock, 1045, lock},
		{lock, 1413, lock},
		{lock, 1044, 1044},
		{lock, 1414, 1414},
		{800, 1900, lock},
		{1900, 800, lock},
		{800, ZoomUnit, ZoomUnit},
		{ZoomUnit, 980, 980},
	} {
		if got := SnapZoom(tc.from, tc.want, lock); got != tc.got {
			t.Fatalf("snap %d -> %d at %d = %d, want %d", tc.from, tc.want, lock, got, tc.got)
		}
	}
}

func TestPreferredZoomLockWheelHoldAndReset(t *testing.T) {
	for _, wheel := range modernWheels {
		t.Run(wheel.name, func(t *testing.T) {
			cam := feelCamera()
			cam.SetZoomAbout(500, 300, ZoomMax)
			const lock Zoom = 1229
			z := ZoomController{LockZoom: lock}
			wheel.call(&z, cam, 500, 300, -20, 10)
			if cam.EffectiveZoom() != lock || z.Target(cam) != lock {
				t.Fatalf("large event missed preferred stop: live=%d target=%d", cam.EffectiveZoom(), z.Target(cam))
			}
			wheel.call(&z, cam, 500, 300, -20, 100)
			wheel.call(&z, cam, 500, 300, -1, 200)
			if z.Target(cam) != lock {
				t.Fatal("continuous wheel burst skipped the preferred stop")
			}
			wheel.call(&z, cam, 500, 300, -1, 200+ZoomScrollCooldownMillis)
			if z.Target(cam) >= lock || z.Target(cam) == ZoomUnit {
				t.Fatal("quiet wheel did not leave the preferred stop")
			}
			z.Reset()
			if z.LockZoom != lock || z.Active(cam) {
				t.Fatal("reset lost the configured lock or retained its glide")
			}
		})
	}
}

func TestCustomZoomLockDoesNotCatchNative(t *testing.T) {
	cam := feelCamera()
	cam.SetZoomAbout(500, 300, 900)
	z := ZoomController{LockZoom: 1536}
	z.Wheel(cam, 500, 300, 1, 0)
	if z.Target(cam) != 1125 {
		t.Fatalf("native retained a detent: target=%d", z.Target(cam))
	}
	z.Wheel(cam, 500, 300, 1, 1)
	if z.Target(cam) != 1536 {
		t.Fatal("second notch did not reach the configured lock")
	}
}

func TestPreferredZoomStopsOrderAndCollapse(t *testing.T) {
	for _, tc := range []struct {
		floor, lock Zoom
		stops       []Zoom
	}{
		{64, 1229, []Zoom{64, 256, 1229, 2048}},
		{64, 205, []Zoom{64, 205, 256, 2048}},
		{64, 10, []Zoom{64, 256, 2048}},
		{256, 256, []Zoom{256, 2048}},
		{64, 2048, []Zoom{64, 256, 2048}},
	} {
		for i, current := range tc.stops {
			for _, in := range []bool{false, true} {
				j := i - 1
				if in {
					j = i + 1
				}
				got, ok := NextZoomStop(current, tc.floor, tc.lock, in)
				if j < 0 || j >= len(tc.stops) {
					if ok {
						t.Fatalf("stop %d passed its limit: %d", current, got)
					}
				} else if !ok || got != tc.stops[j] {
					t.Fatalf("floor=%d lock=%d current=%d in=%v: got %d,%v want %d", tc.floor, tc.lock, current, in, got, ok, tc.stops[j])
				}
			}
		}
	}
}
