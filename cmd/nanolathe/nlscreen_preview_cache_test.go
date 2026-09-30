package main

import (
	"testing"

	"github.com/nanolathe-gg/nanolathe/internal/gameplay"
	"github.com/nanolathe-gg/nanolathe/internal/world"
)

func cachedScene(key nlSceneKey) nlCachedPreview {
	return nlCachedPreview{inst: &nlPreviewInstance{key: key, preset: nlPreset{loop: 60}}}
}

// A revisit resumes the same paused client and its source identities, while
// rules, mutators, paired comparisons and surface geometry separate entries.
func TestNLPreviewCacheResumesOnlyMatchingScene(t *testing.T) {
	key := nlSceneKey{preset: "armor", gameplay: gameplay.Modern, w: 960, h: 540}
	p := newNLPreview(Options{}, nil)
	first := cachedScene(key)
	first.inst.ticks = 45
	p.activate(first)
	other := cachedScene(nlSceneKey{preset: "arrival", gameplay: gameplay.Modern, w: 960, h: 540})
	p.activate(other)
	for _, changed := range []nlSceneKey{
		{preset: "armor", gameplay: gameplay.Strict31, w: 960, h: 540},
		{preset: "armor", gameplay: gameplay.Modern, w: 640, h: 480},
		{preset: "armor", gameplay: gameplay.Modern, mutators: "health=2", w: 960, h: 540},
		{preset: "armor", gameplay: gameplay.Modern, paired: true, w: 960, h: 540},
	} {
		if _, ok := p.takeCached(changed); ok {
			t.Fatalf("reused a different composition: %+v", changed)
		}
	}
	p.request(key)
	if p.cur != first.inst || p.cur.ticks != 45 || p.loading || p.cacheHits != 1 {
		t.Fatal("revisit rebuilt or advanced the paused scene")
	}
	p.Close()
	if !first.inst.closed || !other.inst.closed || len(p.cache) != 0 {
		t.Fatal("closing left a cached preview alive")
	}
}

func TestNLPreviewCacheBoundsPairsAndRetiresExhaustedScenes(t *testing.T) {
	p := newNLPreview(Options{}, nil)
	a := cachedScene(nlSceneKey{preset: "armor"})
	b := cachedScene(nlSceneKey{preset: "arrival"})
	c := cachedScene(nlSceneKey{preset: "placement"})
	p.activate(a)
	p.activate(b)
	p.activate(c)
	p.request(a.inst.key) // A is now recent; B is the oldest.
	pair := cachedScene(nlSceneKey{preset: "sight", paired: true})
	pair.inst.twin = cachedScene(pair.inst.key).inst
	p.activate(pair)
	if !b.inst.closed || !c.inst.closed || a.inst.closed || len(p.cache) != 1 {
		t.Fatal("paired preview did not bound retained sessions by recency")
	}
	pair.inst.ticks = 60 * 30
	p.request(a.inst.key)
	if !pair.inst.closed || !pair.inst.twin.closed {
		t.Fatal("cached an exhausted preview pair")
	}
	p.Close()
	p.Close() // teardown remains safe after an already-retired cache.
}

// A moving placement example stays on the real footprint grid, and has one
// validated site for both the prospective tower and its weapon overlays.
func TestNLPlacementPreviewMovesAcrossValidSites(t *testing.T) {
	opts, cs := openNLTestContent(t)
	inst, err := buildNLPreviewScene(opts, cs, nlSceneKey{preset: "placement", gameplay: gameplay.Modern, w: 960, h: 540}, "")
	if err != nil {
		t.Fatal(err)
	}
	defer inst.close()
	if inst.placementDef == nil {
		t.Fatal("placement preview has no tower")
	}
	initial := inst.b.battleState().Input
	minX, maxX := initial.BuildCellX, initial.BuildCellX
	inst.update(1.0/30, 0)
	view := inst.b.cam.PresentationView()
	for i := 0; i < 240; i++ {
		inst.update(1.0/30, 0)
		state := inst.b.battleState().Input
		fx, fz := world.PlacementCenter(state.BuildCellX, state.BuildCellZ, state.BuildFootX, state.BuildFootZ)
		h, ok := nlPlacement(inst.sess, inst.placementDef, fx, fz)
		if !ok || !state.BuildOK || int32(h.Int()) != state.BuildSiteH || state.BuildDef != initial.BuildDef {
			t.Fatalf("moving ghost lost its validated site at frame %d", i)
		}
		minX, maxX = min(minX, state.BuildCellX), max(maxX, state.BuildCellX)
		if inst.b.cam.PresentationView() != view {
			t.Fatal("placement movement moved the camera")
		}
	}
	if maxX-minX < 6 {
		t.Fatal("placement preview did not visibly travel")
	}
}
