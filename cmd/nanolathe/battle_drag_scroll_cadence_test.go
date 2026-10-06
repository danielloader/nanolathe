package main

import (
	"testing"

	"github.com/nanolathe-gg/nanolathe/internal/camera"
	"github.com/nanolathe-gg/nanolathe/internal/input"
	"github.com/nanolathe-gg/nanolathe/internal/session"
	"github.com/nanolathe-gg/nanolathe/internal/ui"
)

// Per-service signed truncation discards small motion even while paused. Extra
// presentation draws do not poll or spend another delta [07 R-CAM-01 §11].
func TestDragScrollViewerDiscardsRemainderIndependentlyOfDraws(t *testing.T) {
	for _, paused := range []bool{false, true} {
		b := newTestBattle(testCatalogON05(), testWorldON05(40, 40))
		b.millisSource = &fakeMillisSource{}
		b.cam = &camera.Camera{X: 128, Z: 128, ViewW: 640, ViewH: 480, MapW: 4000, MapH: 4000}
		cl := b.cl
		cl.SetFocused(true)
		b.applyBattleSchedule(ui.PauseIntent(paused))
		step := func(kind input.PointerEventKind, x, y int32, held bool) {
			e := input.PointerEvent{Kind: kind, X: x, Y: y, Buttons: input.MouseButtons{Right: held}, Modifiers: input.Modifiers{Ctrl: true}}
			cl.Input().UpdatePointerMotion(e)
			if kind != 0 {
				cl.Input().EnqueuePointer(e)
			}
			cl.Input().PublishPointer()
			b.viewerStep(0, cl)
		}
		step(input.RightDown, 300, 200, true)
		if !b.dragScrollActive {
			t.Fatal("viewer did not capture Ctrl-right")
		}
		for i := int32(1); i <= 8; i++ {
			step(0, 300+i*3, 200-i*3, true)
			for draw := 0; draw < 4; draw++ {
				cl.ComposeFrameSnapshot()
			}
			if b.cam.X != 128 || b.cam.Z != 128 {
				t.Fatalf("paused=%v sample=%d accumulated discarded motion: %d,%d", paused, i, b.cam.X, b.cam.Z)
			}
		}
		step(input.RightUp, 328, 172, false)
		if b.cam.X != 144 || b.cam.Z != 112 || b.dragScrollActive || cl.PointerCaptured() {
			t.Fatalf("paused=%v release camera=%d,%d capture=%v", paused, b.cam.X, b.cam.Z, b.dragScrollActive)
		}
	}
}

// Pointer entry runs before the sub-ticks, while its first displacement runs
// on the next host pass. The saved anchor survives any intervening glide
// movement [07 R-CAM-01 §§1, 11].
func TestDragEntryControllerPreservesGlideUntilFirstStep(t *testing.T) {
	for _, mode := range []string{"runnable", "zero-tick", "paused"} {
		t.Run(mode, func(t *testing.T) {
			b := newTestBattle(testCatalogON05(), testWorldON05(200, 200))
			b.sess.State = session.StateBattle
			millis := &fakeMillisSource{}
			b.millisSource = millis
			c := NewBattleController(b, millis)
			cl := b.cl
			cl.SetFocused(true)
			c.Step(BattleInputFrame{}, cl)
			b.cam.JumpTo(128, 128)
			b.cam.GlideTo(768, 768)
			if mode == "paused" {
				b.applyBattleSchedule(ui.PauseIntent(true))
			}
			if mode != "zero-tick" {
				millis.ms = 67
			}
			before := b.sess.Clock.GlobalTick
			pointer := input.PointerEvent{Kind: input.RightDown, X: 300, Y: 200,
				Buttons: input.MouseButtons{Right: true}, Modifiers: input.Modifiers{Ctrl: true}}
			c.Step(BattleInputFrame{PointerValid: true, Pointer: pointer}, cl)
			want := int32(128)
			if mode == "runnable" {
				if got := b.sess.Clock.GlobalTick - before; got != 2 {
					t.Fatalf("entry tick count=%d, want 2", got)
				}
				want = 608 // two ordinary follow steps toward 768
			} else if b.sess.Clock.GlobalTick != before {
				t.Fatal("zero-tick or paused entry advanced the simulation")
			}
			if b.cam.X != want || b.cam.Z != want || b.cam.Follow.Desired != (camera.Origin{X: 768, Z: 768}) || !b.cam.Follow.Gliding || !b.dragScrollActive {
				t.Fatalf("entry origin=%d,%d desired=%+v glide=%v drag=%v", b.cam.X, b.cam.Z, b.cam.Follow.Desired, b.cam.Follow.Gliding, b.dragScrollActive)
			}
			// This zero-displacement pass must use the anchor captured before
			// the entry frame's ticks, and overwrite the old desired origin
			// before any new sub-tick consumes it.
			millis.ms += 34
			pointer.Kind = 0
			c.Step(BattleInputFrame{PointerValid: true, Pointer: pointer}, cl)
			if b.cam.X != 128 || b.cam.Z != 128 || b.cam.Follow.Desired != (camera.Origin{X: 128, Z: 128}) {
				t.Fatalf("first step retained old glide or recaptured moved origin: %d,%d desired=%+v", b.cam.X, b.cam.Z, b.cam.Follow.Desired)
			}
			pointer.Kind, pointer.Buttons.Right = input.RightUp, false
			c.Step(BattleInputFrame{PointerValid: true, Pointer: pointer}, cl)
			if b.dragScrollActive || cl.PointerCaptured() || b.cam.X != 128 || b.cam.Z != 128 {
				t.Fatal("release restored the old glide or retained pointer capture")
			}
		})
	}
}
