package camera

import (
	"math"
	"testing"
)

func feelCamera() *Camera {
	return &Camera{X: 1200, Z: 900, ViewW: 1024, ViewH: 768, MapW: 16384, MapH: 16384}
}

func settleZoom(t *testing.T, z *ZoomController, cam *Camera) {
	t.Helper()
	for i := 0; i < 32 && z.Active(cam); i++ {
		z.Step(cam)
	}
	if z.Active(cam) || z.Step(cam) {
		t.Fatalf("ease did not settle: live=%s target=%s", cam.EffectiveZoom(), z.Target(cam))
	}
}

var modernWheels = []struct {
	name    string
	call    func(*ZoomController, *Camera, int32, int32, float64, uint32)
	in, out Zoom
}{
	{"smooth", (*ZoomController).Wheel, 1280, 819},
	{"stepped", (*ZoomController).WheelStepped, ZoomMax, ZoomUnit / 4},
}

// Smooth targets spend all whole notches proportionally rather than selecting
// a preset; one negative notch divides by 1.25 (DESIGN_GPU_RENDERER §16.6).
func TestSmoothWheelProportionalTargets(t *testing.T) {
	for _, tc := range []struct {
		from Zoom
		dy   float64
		want Zoom
	}{
		{ZoomUnit, 1, 1280},
		{ZoomUnit, 3, 2000},
		{ZoomUnit, 4, ZoomMax},
		{ZoomUnit, -1, 819},
		{ZoomUnit / 4, 1, 320},
		{ZoomUnit / 4, -2, 164},
	} {
		cam := feelCamera()
		cam.SetZoomAbout(500, 300, tc.from)
		var z ZoomController
		z.Wheel(cam, 500, 300, tc.dy, 0)
		if z.Target(cam) != tc.want || cam.EffectiveZoom() != tc.from {
			t.Fatalf("from %s scroll %v: target=%s live=%s, want target=%s live=%s", tc.from, tc.dy, z.Target(cam), cam.EffectiveZoom(), tc.want, tc.from)
		}
		settleZoom(t, &z, cam)
	}
	cam := feelCamera()
	var z ZoomController
	z.Wheel(cam, 500, 300, 1, 0)
	z.Wheel(cam, 500, 300, 1, 1)
	if z.Target(cam) != 1600 {
		t.Fatalf("successive notches did not compound their target: %s", z.Target(cam))
	}
}

// Strong native snapping catches both ends of the band and an event that
// crosses the entire band. A fresh departure from native is allowed (§16.6).
func TestSnapZoomCatchesDefaultBandAndCrossings(t *testing.T) {
	for _, tc := range []struct{ from, want, got Zoom }{
		{512, 871, ZoomUnit}, {1536, 1177, ZoomUnit},
		{512, ZoomMax, ZoomUnit}, {ZoomMax, 256, ZoomUnit},
		{1126, 512, ZoomUnit}, {922, ZoomMax, ZoomUnit},
		{ZoomUnit, 871, ZoomUnit}, {ZoomUnit, 1177, ZoomUnit},
		{ZoomUnit, 870, 870}, {ZoomUnit, 1178, 1178},
		{256, 512, 512}, {1536, ZoomMax, ZoomMax},
	} {
		if got := SnapZoom(tc.from, tc.want, ZoomUnit); got != tc.got {
			t.Errorf("from %s to %s: got %s, want %s", tc.from, tc.want, got, tc.got)
		}
	}
}

// Modern's native hold requires 180 ms without an event. Every ignored event
// restarts that interval, even a reversed fractional event. A large burst
// cannot skip native or queue its remainder, including across host clock wrap.
func TestModernWheelNativeArrivalAndQuietRelease(t *testing.T) {
	for _, wheel := range modernWheels {
		t.Run(wheel.name, func(t *testing.T) {
			for _, start := range []uint32{0, ^uint32(0) - 80} {
				for _, dy := range []float64{-20, 20} {
					cam := feelCamera()
					from, after := ZoomMax, wheel.out
					if dy > 0 {
						from, after = ZoomUnit/4, wheel.in
					}
					cam.SetZoomAbout(500, 300, from)
					var z ZoomController
					wheel.call(&z, cam, 500, 300, dy, start)
					if z.Target(cam) != ZoomUnit || cam.EffectiveZoom() != ZoomUnit || !cam.AtRestStep() {
						t.Fatalf("burst %v skipped immediate native: target=%s live=%s", dy, z.Target(cam), cam.EffectiveZoom())
					}
					for _, elapsed := range []uint32{1, 180, 359, 538} {
						wheel.call(&z, cam, 600, 400, -dy/100, start+elapsed)
						if z.Target(cam) != ZoomUnit || z.Step(cam) {
							t.Fatal("continuous events released native or queued motion")
						}
					}
					wheel.call(&z, cam, 500, 300, dy/40, start+718)
					if z.Target(cam) != ZoomUnit {
						t.Fatal("discarded burst travel leaked into the fresh half notch")
					}
					wheel.call(&z, cam, 500, 300, dy/40, start+719)
					if z.Target(cam) != after {
						t.Fatalf("quiet release gave %s, want %s", z.Target(cam), after)
					}
				}
			}
		})
	}
}

// Both Modern styles bank fractions, clear them on a pause, and discard the
// old direction's fractions on reversal (§16.6).
func TestModernWheelFractionalTravel(t *testing.T) {
	for _, wheel := range modernWheels {
		t.Run(wheel.name, func(t *testing.T) {
			cam := feelCamera()
			var z ZoomController
			wheel.call(&z, cam, 500, 300, .4, 0)
			wheel.call(&z, cam, 500, 300, .599, 10)
			if z.Target(cam) != ZoomUnit {
				t.Fatal("fraction below threshold changed target")
			}
			wheel.call(&z, cam, 500, 300, .001, 20)
			if z.Target(cam) != wheel.in {
				t.Fatalf("whole notch gave %s", z.Target(cam))
			}

			z.Reset()
			wheel.call(&z, cam, 500, 300, .4, 0)
			wheel.call(&z, cam, 500, 300, .6, 180)
			if z.Target(cam) != ZoomUnit {
				t.Fatal("pause retained old fractions")
			}
			wheel.call(&z, cam, 500, 300, .4, 181)
			if z.Target(cam) != wheel.in {
				t.Fatal("fresh fractions did not accumulate")
			}

			z.Reset()
			wheel.call(&z, cam, 500, 300, .4, 0)
			wheel.call(&z, cam, 500, 300, -.95, 1)
			if z.Target(cam) != ZoomUnit {
				t.Fatal("reversal below threshold changed target")
			}
			wheel.call(&z, cam, 500, 300, -.05, 2)
			if z.Target(cam) != wheel.out {
				t.Fatalf("reversed notch gave %s", z.Target(cam))
			}
		})
	}
}

// A reversal cancels a target immediately, even before its reversed fractions
// reach a notch. Further input starts from the live view, not the old target.
func TestModernWheelReversalCancelsGlide(t *testing.T) {
	for _, wheel := range modernWheels {
		t.Run(wheel.name, func(t *testing.T) {
			cam := feelCamera()
			var z ZoomController
			wheel.call(&z, cam, 500, 300, 4.3, 0)
			z.Step(cam)
			if cam.EffectiveZoom() != 1536 {
				t.Fatal("fixture did not reach 1.5x")
			}
			wheel.call(&z, cam, 500, 300, -.4, 1)
			if z.Target(cam) != 1536 || z.Active(cam) || z.Step(cam) {
				t.Fatal("fractional reversal did not cancel the old glide")
			}
			wheel.call(&z, cam, 500, 300, -.599, 2)
			if z.Target(cam) != 1536 {
				t.Fatal("old remainder finished a reversed notch")
			}
			wheel.call(&z, cam, 500, 300, -.001, 3)
			want := Zoom(1229)
			if wheel.name == "stepped" {
				want = ZoomUnit
			}
			if z.Target(cam) != want {
				t.Fatalf("reversal gave %s, want %s", z.Target(cam), want)
			}
		})
	}
}

// Cancelling a smooth glide inside the native band must settle at exactly 1x
// and acquire the same quiet hold as a crossed target (§16.6).
func TestSmoothWheelReversalInsideNativeBand(t *testing.T) {
	for _, dy := range []float64{-1, 1} {
		cam := feelCamera()
		var z ZoomController
		z.Wheel(cam, 500, 300, dy, 0)
		z.Step(cam)
		z.Wheel(cam, 500, 300, -dy*.01, 1)
		if cam.EffectiveZoom() != ZoomUnit || z.Target(cam) != ZoomUnit {
			t.Fatal("reversal stranded a near-native factor")
		}
		z.Wheel(cam, 500, 300, -dy*20, 180)
		if z.Target(cam) != ZoomUnit {
			t.Fatal("reversal arrival did not hold native")
		}
	}
}

// A four-stop list becomes three or two stops when its lower targets collapse
// onto the map floor. Unreachable targets are never selected (§16.7).
func TestNextZoomStopSkipsCollapsedAndUnreachableStops(t *testing.T) {
	for _, tc := range []struct {
		floor Zoom
		stops []Zoom
	}{
		{64, []Zoom{64, 256, 1024, 2048}},
		{256, []Zoom{256, 1024, 2048}},
		{448, []Zoom{448, 1024, 2048}},
		{1024, []Zoom{1024, 2048}},
		{1280, []Zoom{1280, 2048}},
		{2048, []Zoom{2048}},
		{2049, nil},
	} {
		current := Zoom(0)
		for _, want := range tc.stops {
			got, ok := NextZoomStop(current, tc.floor, ZoomUnit, true)
			if !ok || got != want {
				t.Fatalf("floor %s in from %s: %s,%v want %s", tc.floor, current, got, ok, want)
			}
			current = got
		}
		if _, ok := NextZoomStop(current, tc.floor, ZoomUnit, true); ok {
			t.Fatal("inward list did not end")
		}
		current = ZoomMax + 1
		for i := len(tc.stops) - 1; i >= 0; i-- {
			got, ok := NextZoomStop(current, tc.floor, ZoomUnit, false)
			if !ok || got != tc.stops[i] {
				t.Fatalf("floor %s out from %s: %s,%v want %s", tc.floor, current, got, ok, tc.stops[i])
			}
			current = got
		}
		if _, ok := NextZoomStop(current, tc.floor, ZoomUnit, false); ok {
			t.Fatal("outward list did not end")
		}
	}
	for _, tc := range []struct {
		current Zoom
		in      bool
		want    Zoom
	}{
		{768, true, ZoomUnit}, {768, false, ZoomUnit / 4},
		{1280, true, ZoomMax}, {1280, false, ZoomUnit},
	} {
		if got, ok := NextZoomStop(tc.current, 64, ZoomUnit, tc.in); !ok || got != tc.want {
			t.Fatalf("between stops gave %s,%v, want %s", got, ok, tc.want)
		}
	}
}

func TestSteppedWheelReachesFullMapAfterTactical(t *testing.T) {
	cam := feelCamera()
	cam.SetZoomAbout(500, 300, ZoomMax)
	var z ZoomController
	for _, tc := range []struct {
		dy   float64
		now  uint32
		want Zoom
	}{
		{-20, 0, ZoomUnit},
		{-1, 180, ZoomUnit / 4},
		{-1, 181, cam.MinZoom()},
		{-100.5, 182, cam.MinZoom()},
		{1, 183, ZoomUnit / 4},
		{20, 184, ZoomUnit},
		{20, 364, ZoomMax},
	} {
		z.WheelStepped(cam, 500, 300, tc.dy, tc.now)
		if got := z.Target(cam); got != tc.want {
			t.Fatalf("at %d target %s, want %s", tc.now, got, tc.want)
		}
		settleZoom(t, &z, cam)
	}
}

// A named full-map floor remains selectable even inside the smooth native
// snap band. Collapsed tactical stops do not add an extra notch (§16.7).
func TestSteppedWheelCollapsedMapFloor(t *testing.T) {
	for _, floor := range []Zoom{256, 448, 922, ZoomUnit} {
		cam := &Camera{ViewW: int32(floor) + OriginX, ViewH: int32(floor) + 2*OriginY, MapW: 1024, MapH: 1024}
		var z ZoomController
		z.WheelStepped(cam, 500, 300, -1, 0)
		if z.Target(cam) != floor {
			t.Fatalf("floor %s was skipped: %s", floor, z.Target(cam))
		}
		settleZoom(t, &z, cam)
		z.WheelStepped(cam, 500, 300, -1, 1)
		if z.Target(cam) != floor {
			t.Fatal("stepped wheel aimed below the floor")
		}
		z.WheelStepped(cam, 500, 300, 1, 2)
		want := ZoomUnit
		if floor == ZoomUnit {
			want = ZoomMax
		}
		if z.Target(cam) != want {
			t.Fatalf("collapsed floor returned %s, want %s", z.Target(cam), want)
		}
	}
}

// Excess at either limit must not delay reversal. An input larger than the
// travel bank can represent retains its direction and still stops at native.
func TestSmoothWheelLimitsDiscardExcess(t *testing.T) {
	for _, tc := range []struct {
		from Zoom
		dy   float64
		want Zoom
	}{
		{ZoomMax, 1e30, 1638}, {44, -1e30, 55},
	} {
		cam := feelCamera()
		cam.SetZoomAbout(500, 300, tc.from)
		var z ZoomController
		z.Wheel(cam, 500, 300, tc.dy, 0)
		z.Wheel(cam, 500, 300, -math.Copysign(1, tc.dy), 1)
		if z.Target(cam) != tc.want {
			t.Fatalf("limit reversal gave %s, want %s", z.Target(cam), tc.want)
		}
	}
	for _, dy := range []float64{-1e30, 1e30} {
		cam := feelCamera()
		from := ZoomMax
		if dy > 0 {
			from = ZoomUnit / 4
		}
		cam.SetZoomAbout(500, 300, from)
		var z ZoomController
		z.Wheel(cam, 500, 300, dy, 0)
		if cam.EffectiveZoom() != ZoomUnit {
			t.Fatal("very large event overflowed or skipped native")
		}
	}
	cam := feelCamera()
	cam.SetZoomAbout(500, 300, 64)
	var z ZoomController
	z.Wheel(cam, 500, 300, -20.75, 0)
	settleZoom(t, &z, cam)
	if cam.EffectiveZoom() != cam.MinZoom() {
		t.Fatal("smooth wheel did not clamp at full map")
	}
	z.Wheel(cam, 500, 300, .5, 1)
	if z.Target(cam) != cam.MinZoom() {
		t.Fatal("limit excess leaked into reversal")
	}
	z.Wheel(cam, 500, 300, .5, 2)
	if z.Target(cam) <= cam.MinZoom() {
		t.Fatal("fresh reversed notch did not leave floor")
	}
}

// Each accepted glide and immediate native arrival uses the pointer anchor
// throughout its precise presentation transform (§16.5).
func TestWheelKeepsPointerAnchored(t *testing.T) {
	for _, tc := range []struct {
		name string
		call func(*ZoomController, *Camera, int32, int32, float64, uint32)
		from Zoom
		dy   float64
	}{
		{"smooth ease", (*ZoomController).Wheel, ZoomUnit, 1},
		{"smooth native", (*ZoomController).Wheel, ZoomMax, -20},
		{"stepped ease", (*ZoomController).WheelStepped, ZoomUnit, -1},
		{"stepped native", (*ZoomController).WheelStepped, ZoomUnit / 4, 20},
		{"legacy ease", (*ZoomController).WheelLegacy, ZoomUnit, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cam := feelCamera()
			cam.X, cam.Z = 5000, 4000
			cam.SetZoomAbout(500, 300, tc.from)
			before := cam.PresentationView()
			wx := before.X + float64(500-OriginX)/before.Factor
			wz := before.Z + float64(300-OriginY)/before.Factor
			var z ZoomController
			tc.call(&z, cam, 500, 300, tc.dy, 0)
			for i := 0; i < 32; i++ {
				v := cam.PresentationView()
				if math.Abs((wx-v.X)*v.Factor-float64(500-OriginX)) > 1e-9 || math.Abs((wz-v.Z)*v.Factor-float64(300-OriginY)) > 1e-9 {
					t.Fatal("wheel moved the precise point under the pointer")
				}
				if !z.Step(cam) {
					break
				}
			}
			settleZoom(t, &z, cam)
		})
	}
}

// Legacy preserves the old three stops and slower ease, including requested
// tactical intent when the map floor clamps its geometry (§16.6, §16.7).
func TestLegacyWheelPreservesPresets(t *testing.T) {
	cam := feelCamera()
	var z ZoomController
	for i, tc := range []struct {
		dy   float64
		want Zoom
	}{
		{1, ZoomMax}, {1, ZoomMax}, {-1, ZoomUnit},
		{-1, ZoomUnit / 4}, {-1, ZoomUnit / 4}, {1, ZoomUnit},
	} {
		z.WheelLegacy(cam, 500, 300, tc.dy, uint32(i)*500)
		if z.Target(cam) != tc.want {
			t.Fatalf("legacy scroll %v gave %s, want %s", tc.dy, z.Target(cam), tc.want)
		}
	}
	for _, tc := range []struct {
		from Zoom
		in   bool
		want Zoom
	}{
		{1331, true, ZoomMax}, {1331, false, ZoomUnit},
		{614, false, ZoomUnit / 4}, {614, true, ZoomUnit},
	} {
		if got, ok := NextZoomStep(tc.from, tc.in); !ok || got != tc.want {
			t.Fatal("legacy preset selection changed")
		}
	}
}

func TestLegacyCooldownDoesNotExtendAndDiscardsBurst(t *testing.T) {
	for _, start := range []uint32{0, ^uint32(0) - 200} {
		for _, dy := range []float64{-20, 20} {
			cam := feelCamera()
			from, end := ZoomMax, ZoomUnit/4
			if dy > 0 {
				from, end = end, from
			}
			cam.SetZoomAbout(500, 300, from)
			var z ZoomController
			z.WheelLegacy(cam, 500, 300, dy, start)
			if z.Target(cam) != ZoomUnit || cam.EffectiveZoom() != from {
				t.Fatal("legacy wheel stopped easing its native arrival")
			}
			for _, elapsed := range []uint32{1, 200, 499} {
				z.WheelLegacy(cam, 500, 300, dy, start+elapsed)
				if z.Target(cam) != ZoomUnit {
					t.Fatal("legacy cooldown let a step through")
				}
			}
			settleZoom(t, &z, cam)
			z.WheelLegacy(cam, 500, 300, dy/40, start+500)
			if z.Target(cam) != ZoomUnit {
				t.Fatal("legacy excess travel was queued")
			}
			z.WheelLegacy(cam, 500, 300, dy/40, start+501)
			if z.Target(cam) != end {
				t.Fatal("ignored events extended the legacy deadline")
			}
		}
	}
}

func TestLegacyFractionsPreserveAcceptedGlide(t *testing.T) {
	cam := feelCamera()
	var z ZoomController
	z.WheelLegacy(cam, 500, 300, .4, 0)
	z.WheelLegacy(cam, 500, 300, .599, 10)
	if z.Target(cam) != ZoomUnit {
		t.Fatal("legacy fraction stepped too early")
	}
	z.WheelLegacy(cam, 500, 300, .001, 20)
	if z.Target(cam) != ZoomMax {
		t.Fatal("legacy fractions did not make a notch")
	}
	z.Step(cam)
	z.WheelLegacy(cam, 500, 300, .1, 520)
	z.WheelLegacy(cam, 500, 300, -.95, 530)
	if z.Target(cam) != ZoomMax || !z.Active(cam) {
		t.Fatal("legacy reversal cancelled its accepted glide")
	}
	z.WheelLegacy(cam, 500, 300, -.05, 540)
	if z.Target(cam) != ZoomUnit {
		t.Fatal("legacy reversed fractions did not make a notch")
	}
}

func TestLegacyClampedTacticalStopRetainsIntent(t *testing.T) {
	for _, floor := range []Zoom{576, 768, ZoomUnit} {
		cam := &Camera{ViewW: int32(floor) + OriginX, ViewH: int32(floor) + 2*OriginY, MapW: 1024, MapH: 1024}
		var z ZoomController
		z.WheelLegacy(cam, 500, 300, -1, 0)
		settleZoom(t, &z, cam)
		if cam.EffectiveZoom() != floor || cam.RequestedZoom() != ZoomUnit/4 || !cam.TacticalAtFloor() {
			t.Fatalf("lost legacy tactical intent: live=%s requested=%s", cam.EffectiveZoom(), cam.RequestedZoom())
		}
		z.WheelLegacy(cam, 500, 300, -1, 500)
		if cam.RequestedZoom() != ZoomUnit/4 {
			t.Fatal("extra legacy scroll changed its lowest stop")
		}
		z.WheelLegacy(cam, 500, 300, 1, 500)
		if cam.RequestedZoom() != ZoomUnit || cam.TacticalAtFloor() {
			t.Fatal("legacy scroll in did not restore native intent")
		}
	}
}

func TestExplicitTargetsSelectModernAndLegacyEase(t *testing.T) {
	for _, tc := range []struct {
		name string
		set  func(*ZoomController, *Camera, int32, int32, Zoom)
		want Zoom
	}{
		{"modern", (*ZoomController).SetTarget, 1536},
		{"legacy", (*ZoomController).SetTargetLegacy, 1331},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cam := feelCamera()
			var z ZoomController
			z.Wheel(cam, 500, 300, 4, 0)
			tc.set(&z, cam, 500, 300, ZoomMax)
			z.Step(cam)
			if cam.EffectiveZoom() != tc.want {
				t.Fatalf("first ease = %s, want %s", cam.EffectiveZoom(), tc.want)
			}
			settleZoom(t, &z, cam)
			if !cam.AtRestStep() {
				t.Fatal("settling left the camera off its record step")
			}
		})
	}
}

func TestExplicitTargetAndCancelWheelClearBurstState(t *testing.T) {
	cam := feelCamera()
	cam.SetZoomAbout(500, 300, ZoomMax)
	var z ZoomController
	z.Wheel(cam, 500, 300, -20, 0)
	z.SetTarget(cam, 500, 300, ZoomUnit/4)
	z.Wheel(cam, 500, 300, 20, 1)
	if z.Target(cam) != ZoomUnit {
		t.Fatal("explicit target retained wheel hold")
	}
	z.CancelWheel()
	z.Wheel(cam, 500, 300, 1, 2)
	if z.Target(cam) != 1280 {
		t.Fatal("cancel did not release native")
	}
	z.CancelWheel()
	if z.Target(cam) != 1280 || !z.Active(cam) {
		t.Fatal("cancel discarded an accepted target")
	}
	settleZoom(t, &z, cam)
	z.Reset()
	if z.Target(cam) != cam.EffectiveZoom() || z.Active(cam) {
		t.Fatal("reset retained its target")
	}
	z.Wheel(cam, 500, 300, .75, 3)
	z.CancelWheel()
	z.Wheel(cam, 500, 300, .25, 4)
	if z.Target(cam) != 1280 {
		t.Fatal("cancel retained fractional travel")
	}
	z.Wheel(cam, 500, 300, .75, 5)
	if z.Target(cam) != 1600 {
		t.Fatal("fresh fractional travel did not accumulate")
	}
}
