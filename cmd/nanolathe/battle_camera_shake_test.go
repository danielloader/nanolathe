package main

import (
	"testing"

	"github.com/nanolathe-gg/nanolathe/internal/camera"
	"github.com/nanolathe-gg/nanolathe/internal/frame"
	"github.com/nanolathe-gg/nanolathe/internal/pool"
	"github.com/nanolathe-gg/nanolathe/internal/session"
	"github.com/nanolathe-gg/nanolathe/internal/sim/numeric"
)

// shakeRig drives the battle camera in the controller's order: once per host
// frame the pre-input latch, then the host's own camera writers (scroll, drag,
// zoom, a minimap jump, a hotkey), then each completed sub-tick's publication
// carrying the session's cumulative shake offset as phase 10 publishes it,
// then any follow request the controller re-runs after the batch
// [07 R-CAM-01 §12][01 §4.4.1]. The asynchronous simulation keeps the same
// order: a batch launched after input is applied at the next join, before the
// next latch.
type shakeRig struct {
	t          *testing.T
	b          *battleSession
	cam        *camera.Camera
	buf        *frame.Buffer
	offX, offZ int32
	tick       uint32
	unit       *frame.UnitView
	after      func()
}

func newShakeRig(t *testing.T) *shakeRig {
	cam := &camera.Camera{ViewW: 640, ViewH: 480, MapW: 1 << 20, MapH: 1 << 20}
	cam.JumpTo(20000, 20000)
	buf := frame.NewBuffer()
	return &shakeRig{t: t, b: &battleSession{sess: &session.Session{Snapshot: buf}, cam: cam}, cam: cam, buf: buf}
}

func (r *shakeRig) origin() camera.Origin { return camera.Origin{X: r.cam.X, Z: r.cam.Z} }

// frame runs one host frame: the latch, the host writer (nil for none), one
// completed sub-tick per jolt, then the after-batch request, if any.
func (r *shakeRig) frame(host func(), jolts ...[2]int32) {
	r.t.Helper()
	r.b.stepFollowCamera()
	if host != nil {
		host()
	}
	for _, j := range jolts {
		r.offX += j[0]
		r.offZ += j[1]
		r.tick++
		f := r.buf.BeginWrite()
		f.ShakeOffsetX, f.ShakeOffsetY = r.offX, r.offZ
		f.Units = f.Units[:0]
		if r.unit != nil {
			f.Units = append(f.Units, *r.unit)
		}
		if err := r.buf.Publish(r.tick); err != nil {
			r.t.Fatalf("publish tick %d: %v", r.tick, err)
		}
		r.b.applyPublishedCamera(r.buf.Current())
	}
	if after := r.after; after != nil {
		r.after = nil
		after()
	}
}

// shake runs n host frames of one sub-tick each, every sub-tick carrying the
// same jolt. A one-sided walk is the worst case: every jolt pushes the same
// way, as a run of large blasts could by chance.
func (r *shakeRig) shake(n int, dx, dz int32) {
	r.t.Helper()
	for i := 0; i < n; i++ {
		r.frame(nil, [2]int32{dx, dz})
	}
}

// settle runs n host frames whose one sub-tick carries no jolt.
func (r *shakeRig) settle(n int) {
	r.t.Helper()
	for i := 0; i < n; i++ {
		r.frame(nil, [2]int32{})
	}
}

// near fails unless the camera rests within the phase-10 half-step's one-pixel
// stall of want on each axis [07 §10].
func (r *shakeRig) near(what string, want camera.Origin) {
	r.t.Helper()
	dx, dz := r.cam.X-want.X, r.cam.Z-want.Z
	if dx < -1 || dx > 1 || dz < -1 || dz > 1 {
		r.t.Fatalf("%s: camera settled at (%d,%d), want within one pixel of (%d,%d)", what, r.cam.X, r.cam.Z, want.X, want.Z)
	}
}

// An idle, untracked camera returns to where it was once a shake ends. Retail
// applies the jolt to the current origin only, and every phase-10 pass steps
// the current origin toward the desired origin, which the shake never writes,
// whether or not a follow target is selected [07 R-CAM-01 §10][01 §4.4.1]
// [03 §5.6]. Before issue #34 this build panned by the cumulative offset with
// nothing pulling it back, so the jolts summed into a permanent walk away from
// the player's view.
func TestShakeReturnsIdleCameraToItsOrigin(t *testing.T) {
	r := newShakeRig(t)
	start := r.origin()
	r.shake(60, 40, -30)
	if r.origin() == start {
		t.Fatal("camera did not move during the shake")
	}
	if r.offX < 2000 {
		t.Fatalf("test walk too small to matter: %d", r.offX)
	}
	r.settle(60)
	r.near("after the shake", start)

	// Catch-up batches carry several sub-ticks in one host frame; the return
	// is a per-sub-tick step, and nothing between those sub-ticks is a host
	// write.
	for i := 0; i < 20; i++ {
		r.frame(nil, [2]int32{-25, 35}, [2]int32{-25, 35}, [2]int32{-25, 35})
	}
	r.settle(30)
	r.near("after catch-up batches", start)
}

// A second shake arriving mid-return, and any number of shakes that each end
// in the one-pixel stall, return to the same origin: the stall never becomes
// the next shake's rest, so no pixel accumulates per jolt [07 §10].
func TestRepeatedShakesDoNotCreep(t *testing.T) {
	r := newShakeRig(t)
	start := r.origin()
	r.shake(10, 90, 70)
	r.settle(2) // part-way back
	r.shake(10, 90, 70)
	r.settle(40)
	r.near("after a shake mid-return", start)
	for i := 0; i < 200; i++ {
		// Every burst pushes the same way, so every return stalls one pixel
		// short on the same side.
		r.shake(3, 7, 5)
		r.settle(12)
	}
	r.near("after 200 separate shakes", start)
}

// A host writer that moves the view during the return owns the new position.
// Retail's scroll pass, minimap jump, drag steps and bookmark recall copy the
// current origin into the desired origin [07 R-CAM-01 §10][07 R-CAM-01 §11]
// [07 R-CAM-01 §12]; the presentation-only middle drag and zoom move the view
// the same way. The return must never pull the view back toward where it was
// before the player moved it.
func TestShakeReturnYieldsToHostCameraWriters(t *testing.T) {
	var drag camera.DragScroll
	cases := []struct {
		name  string
		setup func(*shakeRig)
		write func(*shakeRig)
	}{
		{name: "keyboard or edge scroll", write: func(r *shakeRig) {
			// The scroll pass's own shape: the move, then the follow cancel.
			r.cam.Scroll(32, 1, camera.DirRight)
			r.cam.ClearFollow()
		}},
		{name: "middle drag", write: func(r *shakeRig) { r.cam.Drag(-60, 45) }},
		{name: "minimap jump", write: func(r *shakeRig) {
			r.cam.ClearFollow()
			r.cam.JumpToBattleViewCenter(30000, 9000)
		}},
		{name: "Ctrl-right drag step", setup: func(r *shakeRig) { drag.Begin(r.cam) },
			write: func(r *shakeRig) { drag.Step(r.cam, 64, -32) }},
		{name: "bookmark recall", write: func(r *shakeRig) {
			// The controller re-runs a recall after the batch.
			r.cam.RecallBookmark(0)
			r.after = func() { r.cam.RecallBookmark(0) }
		}},
		{name: "zoom about the pointer", write: func(r *shakeRig) {
			r.cam.SetZoomAbout(600, 400, camera.ZoomMax)
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := newShakeRig(t)
			r.cam.JumpTo(31000, 12000)
			r.cam.StoreBookmark(0)
			r.cam.JumpTo(20000, 20000)
			start := r.origin()
			if tc.setup != nil {
				r.frame(func() { tc.setup(r) })
			}
			r.shake(8, 40, -30)
			var moved camera.Origin
			r.frame(func() {
				tc.write(r)
				moved = r.origin()
			}, [2]int32{})
			if moved == start {
				t.Fatal("the writer did not move the view")
			}
			if r.origin() != moved {
				t.Fatalf("the sub-tick after the writer pulled the view from (%d,%d) to (%d,%d)", moved.X, moved.Z, r.cam.X, r.cam.Z)
			}
			r.settle(30)
			r.near("after the writer, with no further jolt", moved)

			// A later shake returns to the writer's position, not the old one.
			r.shake(8, -40, 30)
			r.settle(30)
			r.near("after a later shake", moved)
		})
	}
}

// Scrolling through a shake moves the view by the scroll, as in retail, where
// every scroll pass rewrites the desired origin: the return never eats the
// player's scroll [07 R-CAM-01 §10].
func TestShakeReturnDoesNotDampScroll(t *testing.T) {
	r := newShakeRig(t)
	start := r.origin()
	const frames = 40
	for i := 0; i < frames; i++ {
		sign := int32(1 - 2*(i%2)) // alternating jolts, so the jitter nets out
		r.frame(func() {
			r.cam.Scroll(32, 1, camera.DirRight)
			r.cam.ClearFollow()
		}, [2]int32{sign * 40, sign * 30})
	}
	r.settle(30)
	if got, want := r.cam.X-start.X, int32(frames*32); got < want-80 || got > want+80 {
		t.Fatalf("scrolled %d map pixels through the shake, want %d within two jolts", got, want)
	}
}

// A tracked follow and a glide already in flight keep their own desired
// origin; the shake is damped toward it and the view ends there
// [07 R-CAM-01 §12].
func TestShakeKeepsFollowAndGlideTargets(t *testing.T) {
	t.Run("tracked unit", func(t *testing.T) {
		r := newShakeRig(t)
		const h = pool.Handle(5)
		r.unit = &frame.UnitView{Slot: h, X: numeric.Fixed(int64(22000) << 16), Z: numeric.Fixed(int64(21000) << 16)}
		r.shake(4, 40, -30) // a return is in flight when the follow starts
		r.frame(func() { r.cam.SetTracked(h) }, [2]int32{40, -30})
		r.shake(20, 40, -30)
		r.settle(30)
		r.near("tracked", r.cam.DesiredOrigin(camera.TargetPoint{X: 22000, Z: 21000}))

		// A scroll stops the follow and the view stays where the scroll put
		// it; a later shake returns there. The scroll's frame runs no
		// sub-tick, so the follow latched before it cannot step.
		var moved camera.Origin
		r.frame(func() {
			r.cam.Scroll(32, 1, camera.DirDown)
			r.cam.ClearFollow()
			moved = r.origin()
		})
		r.shake(10, -40, 30)
		r.settle(30)
		r.near("after the follow stopped", moved)
	})
	t.Run("glide in flight", func(t *testing.T) {
		r := newShakeRig(t)
		r.frame(func() { r.cam.GlideTo(25000, 16000) }, [2]int32{})
		r.shake(20, 40, -30)
		r.settle(40)
		r.near("glide", camera.Origin{X: 25000, Z: 16000})
	})
}

// Every phase-10 pass ends with the camera clamp, so the return can never
// carry the view past a bound that a zoom moved under it [07 R-CAM-01 §10].
func TestShakeReturnStaysInsideTheClamp(t *testing.T) {
	r := newShakeRig(t)
	r.cam.MapW, r.cam.MapH = 4000, 4000
	r.cam.JumpTo(-1000, 1000) // clamps to the west bound at 1x
	r.shake(1, 70, 0)
	// Zooming in about the viewport's own corner keeps the origin where it is
	// but moves the west bound above the pre-shake origin.
	r.frame(func() { r.cam.SetZoomAbout(camera.OriginX, camera.OriginY, camera.ZoomMax) })
	r.settle(20)
	x := r.cam.X
	r.cam.Clamp()
	if r.cam.X != x {
		t.Fatalf("the return left the camera at x=%d, outside the clamp (clamped to %d)", x, r.cam.X)
	}
}
