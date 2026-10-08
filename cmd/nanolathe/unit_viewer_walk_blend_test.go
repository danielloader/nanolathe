package main

import (
	"reflect"
	"slices"
	"testing"

	"github.com/nanolathe-gg/nanolathe/internal/frame"
	"github.com/nanolathe-gg/nanolathe/internal/model"
	"github.com/nanolathe-gg/nanolathe/internal/testsupport"
	"github.com/nanolathe-gg/nanolathe/vfs"
)

func TestWalkPreviewDoesNotAlterPlaybackOrOtherActions(t *testing.T) {
	def, mdl := unitViewerAnimationFixture(viewerScriptFixture{"StartMoving", []uint32{
		viewerPush, 0, viewerMoveNow, 1, 0, viewerPush, 100, viewerSleep,
		viewerPush, 9 << 16, viewerMoveNow, 1, 0, viewerPush, 100, viewerSleep,
		0x10064000, 0,
	}})
	def.BMCode, def.MaxVelocity, def.Acceleration, def.MoveRate1, def.MoveRate2 = 1, 1<<16, 1<<16, 2<<16, 2<<16
	for _, fps := range []int{30, 60, 120} {
		plain := unitViewerModel{}
		smooth := unitViewerModel{walkEnabled: true, walkSmooth: true}
		for _, m := range []*unitViewerModel{&plain, &smooth} {
			m.loadAnimation(def, mdl)
			m.setAnimation(unitViewerMoving, 1)
		}
		var prior []frame.PieceView
		changed := 0
		for step := range fps * 3 {
			plain.updateAnimation(1.0 / float64(fps))
			smooth.updateAnimation(1.0 / float64(fps))
			if !reflect.DeepEqual(plain.anim.script.vm.Pieces, smooth.anim.script.vm.Pieces) ||
				plain.anim.script.vm.Threads != smooth.anim.script.vm.Threads || plain.anim.ticks != smooth.anim.ticks {
				t.Fatalf("%d fps: smoothing changed script state at frame %d", fps, step)
			}
			if step >= fps && !slices.Equal(prior, smooth.poses) {
				changed++
			}
			prior = append(prior[:0], smooth.poses...)
		}
		if changed < fps*2-1 {
			t.Fatalf("%d fps: held ticks remained: %d/ %d frames moved", fps, changed, fps*2)
		}
		ticks := smooth.anim.ticks
		smooth.walkSmooth = false
		smooth.refreshPose()
		if !slices.Equal(smooth.poses, plain.poses) || smooth.anim.ticks != ticks {
			t.Fatal("comparison switch reset playback or did not restore raw pose")
		}
		smooth.walkSmooth = true
		smooth.setAnimation(unitViewerIdle, 1)
		smooth.updateAnimation(0.1)
		if smooth.walkPreviewActive() || !slices.Equal(smooth.poses, smooth.anim.poses()) {
			t.Fatal("walk prototype affected Idle")
		}
	}
}

func TestWalkPreviewFidoRetail(t *testing.T) {
	cs, err := openContent(Options{Root: testsupport.RetailRoot(t), Mod: "none", ModSet: true})
	if err != nil {
		t.Fatal(err)
	}
	defer cs.Close()
	cat, err := cs.nlPreviewCatalog()
	if err != nil {
		t.Fatal(err)
	}
	def, ok := cat.Unit("armfido")
	if !ok {
		t.Fatal("missing Fido")
	}
	mdl, err := model.Load(cs.unmappedMount, vfs.ResourcePath("objects3d", def.ObjectName, "3do"))
	if err != nil {
		t.Fatal(err)
	}
	for _, fps := range []int{30, 60, 120} {
		m := unitViewerModel{walkEnabled: true, walkSmooth: true}
		m.loadAnimation(def, mdl)
		m.setAnimation(unitViewerMoving, 1)
		var previous, rawPrevious []frame.PieceView
		smoothChanges, rawChanges := 0, 0
		for step := range fps * 6 {
			m.updateAnimation(1.0 / float64(fps))
			raw := m.anim.poses()
			if step >= fps*2 {
				if !slices.Equal(previous, m.poses) {
					smoothChanges++
				}
				if !slices.Equal(rawPrevious, raw) {
					rawChanges++
				}
			}
			previous = append(previous[:0], m.poses...)
			rawPrevious = append(rawPrevious[:0], raw...)
		}
		if smoothChanges < fps*4-1 || rawChanges >= smoothChanges/2 {
			t.Fatalf("%d fps: smooth %d raw %d changes in 4 seconds", fps, smoothChanges, rawChanges)
		}
		t.Logf("%d fps: %d smooth vs %d original changes in 4 seconds", fps, smoothChanges, rawChanges)
	}
}
