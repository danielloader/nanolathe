package gpurender

import (
	"github.com/nanolathe-gg/nanolathe/formats"
	"github.com/nanolathe-gg/nanolathe/internal/camera"
	"github.com/nanolathe-gg/nanolathe/internal/drawlist"
)

// PrepareTerrain uploads the two terrain atlas scales at the loading boundary.
// It uses precisely the keys Execute will request: native omits detail art,
// detail includes the installed provider or nearest-doubles the native tiles.
// Repeated calls reuse the same pages; ResetSources owns their disposal.
// This moves one-time work and memory to loading, not a retail behavior change
// (DESIGN_GPU_RENDERER §14.8). Call only on the graphics-device owner.
func (r *Renderer) PrepareTerrain(sources drawlist.Terrain) {
	if r == nil {
		return
	}
	r.atlasFor(sources.Terrain, nil, camera.ViewScaleNative)
	r.atlasFor(sources.Terrain, sources.Detail, camera.ViewScaleDetail)
}

// PrepareSprites places sprite frames in the scene atlas at the loading
// boundary, by the same identity keys Execute uses, so a frame first seen
// mid-battle is already on its page. Placed on first use instead, the first
// sight of a stretch of map packs and uploads dozens of sprites inside one
// frame: in a traced game a camera jump onto unseen ground spent 6 ms in
// Replay, and the render thread then blocked 43 ms in one texture upload
// (Renderer.AtlasUploads now records how much atlas such a frame writes).
// Transient frames keep their own
// short-lived images. Like PrepareTerrain it moves one-time work to loading and
// changes no pixel (DESIGN_GPU_RENDERER §14.8). Call only on the
// graphics-device owner.
func (r *Renderer) PrepareSprites(frames []*formats.GAFFrame) {
	if r == nil {
		return
	}
	for _, f := range frames {
		if f != nil && !f.Transient {
			r.sceneFrameFor(f)
		}
	}
}
