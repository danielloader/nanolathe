package main

import (
	"testing"

	"github.com/nanolathe-gg/nanolathe/internal/camera"
	"github.com/nanolathe-gg/nanolathe/internal/frame"
	"github.com/nanolathe-gg/nanolathe/internal/session"
)

// An idle, untracked camera returns to where it was once a shake ends. Retail
// applies the jolt to the current origin only and phase 10 steps the current
// origin back toward the unchanged desired origin every pass
// [07 R-CAM-01 §10][01 §4.4.1]. Before issue #34 this build panned by the
// cumulative offset with nothing pulling it back, so the jolts summed into a
// permanent walk away from the player's view.
func TestShakeReturnsIdleCameraToItsOrigin(t *testing.T) {
	cam := &camera.Camera{ViewW: 640, ViewH: 480, MapW: 1 << 20, MapH: 1 << 20}
	cam.JumpTo(50000, 50000)
	startX, startZ := cam.X, cam.Z
	buf := frame.NewBuffer()
	b := &battleSession{sess: &session.Session{Snapshot: buf}, cam: cam}

	// A one-sided walk is the worst case: every jolt pushes the same way, as
	// a run of large blasts could by chance. The published offset is the
	// session's cumulative sum, as phase 10 publishes it.
	var offX, offZ int32
	var tick uint32
	publish := func(dx, dz int32) {
		offX += dx
		offZ += dz
		tick++
		f := buf.BeginWrite()
		f.ShakeOffsetX, f.ShakeOffsetY = offX, offZ
		if err := buf.Publish(tick); err != nil {
			t.Fatalf("publish tick %d: %v", tick, err)
		}
		b.stepFollowCamera()
		b.applyPublishedCamera(buf.Current())
	}
	for i := 0; i < 60; i++ {
		publish(40, -30)
	}
	if cam.X == startX && cam.Z == startZ {
		t.Fatalf("camera did not move during the shake")
	}
	if offX < 2000 {
		t.Fatalf("test walk too small to matter: %d", offX)
	}
	for i := 0; i < 60; i++ {
		publish(0, 0)
	}
	if d := cam.X - startX; d < -1 || d > 1 {
		t.Fatalf("camera X settled at %d, want within the one-pixel stall of %d (walk %d)", cam.X, startX, offX)
	}
	if d := cam.Z - startZ; d < -1 || d > 1 {
		t.Fatalf("camera Z settled at %d, want within the one-pixel stall of %d (walk %d)", cam.Z, startZ, offZ)
	}
}
