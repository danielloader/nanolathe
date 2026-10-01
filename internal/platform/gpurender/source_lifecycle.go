package gpurender

import (
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/nanolathe-gg/nanolathe/formats"
)

// ResetSources retires one terrain generation after the host has joined the
// recorder and finished submitting the previous frame. A restart loads fresh
// source identities, so keeping their maps for the process lifetime retains
// every earlier battle. Shaders, palette tables, output surfaces, frame scratch
// (the model lane's pages, the reflection planes), recycled fixed-size source
// pages and options survive (DESIGN_GPU_RENDERER §2.3 "Source lifetime").
func (r *Renderer) ResetSources() {
	r.resetSources(func(img *ebiten.Image) { img.Deallocate() })
}

// release is injectable so the ownership walk can be verified without opening
// a graphics device. Several source caches can share the same image.
func (r *Renderer) resetSources(release func(*ebiten.Image)) {
	if r == nil {
		return
	}
	r.joinModelPages()
	seen := make(map[*ebiten.Image]struct{})
	retire := func(img *ebiten.Image) {
		if img == nil {
			return
		}
		if _, ok := seen[img]; ok {
			return
		}
		seen[img] = struct{}{}
		release(img)
	}
	// Fixed-size pages go back to the pool for the next generation's packers.
	recycle := func(img *ebiten.Image, w, h int) {
		if img == nil {
			return
		}
		if _, ok := seen[img]; ok {
			return
		}
		seen[img] = struct{}{}
		if !r.pages.keep(img, w, h) {
			release(img)
		}
	}
	// These maps are resource owners only. Retirement order cannot change any
	// submitted pixels or simulation state; all previous work is enqueued [I6].
	for _, a := range r.tileAtlases {
		if a == nil {
			continue
		}
		for _, img := range a.pages {
			if a.cells != nil {
				recycle(img, a.cols*a.stride, a.rowsPer*a.stride)
			} else {
				retire(img)
			}
		}
	}
	for _, img := range r.gafImages {
		retire(img)
	}
	for _, slot := range r.scene.transient.slots {
		retire(slot.image)
	}
	for _, p := range r.scene.pages {
		if p != nil && p.shared {
			recycle(p.img, p.w, p.h)
		} else if p != nil {
			retire(p.img)
		}
	}
	for _, slot := range r.textureAtlas.slots {
		retire(slot.img)
	}
	recycle(r.textureAtlas.page, modelTexturePageSize, modelTexturePageSize)
	// The model lane's 4096² pages are frame scratch, not sources: each frame
	// clears the rows it uses before rasterizing into them. They survive like
	// the output surfaces, so a new battle or settings preview does not
	// allocate (and on unified memory wire) two fresh 64 MiB planes per page
	// inside its first frame.
	pages := r.modelDirect.pages
	retire(r.modelDirect.params.img)
	retire(r.modelDirect.groups.key)
	retire(r.modelDirect.groups.colour)
	retire(r.fog.atlas)
	retire(r.fog.grid)
	retire(r.water.mask)
	retire(r.pointPlane.img)
	retire(r.sched.flash.img)
	for _, atlas := range r.markerAtlases {
		retire(atlas.image)
	}
	r.markerAtlases = [4]markerAtlasUpload{}

	r.tileAtlases = make(map[tileAtlasKey]*tileAtlas)
	r.gafImages = make(map[*formats.GAFFrame]*ebiten.Image)
	r.scene = sceneAtlas{
		pool:   &r.pages,
		frames: make(map[*formats.GAFFrame]sceneEntry),
		pcx:    make(map[*formats.PCX]sceneEntry),
		fonts:  make(map[*formats.FNT]*fntAtlas),
	}
	r.textureAtlas = modelTextureAtlas{}
	r.modelPrep = modelPrepScratch{}
	r.lighting = battleLighting{modelDisabled: r.lighting.modelDisabled, groundDisabled: r.lighting.groundDisabled, groundStrengthOffset: r.lighting.groundStrengthOffset}
	r.heat = treeHeat{treeDisabled: r.heat.treeDisabled, wreckDisabled: r.heat.wreckDisabled}
	r.water = waterLayer{surfaceDisabled: r.water.surfaceDisabled, motionDisabled: r.water.motionDisabled, foamDisabled: r.water.foamDisabled,
		shader: r.water.shader, stillShader: r.water.stillShader, wakeShader: r.water.wakeShader}
	// The reflection planes are surface-sized scratch, cleared before every
	// use, and survive like the output surfaces.
	r.reflections = waterReflections{disabled: r.reflections.disabled, sourceShader: r.reflections.sourceShader, resolveShader: r.reflections.resolveShader, softResolveShader: r.reflections.softResolveShader,
		source: r.reflections.source, height: r.reflections.height}
	r.modelDirect = modelDirectLane{keyShader: r.modelDirect.keyShader, colourShader: r.modelDirect.colourShader, shaderErr: r.modelDirect.shaderErr, groups: modelGroupMergeLane{shader: r.modelDirect.groups.shader}}
	for i, pg := range pages {
		r.modelDirect.pages[i] = modelDirectPage{key: pg.key, colour: pg.colour}
	}
	r.fog = fogPass{shader: r.fog.shader, shaderErr: r.fog.shaderErr, compiled: r.fog.compiled}
	r.arrival = arrivalLayer{shader: r.arrival.shader}
	// Retained compiled runs and options also reference source images. Drop
	// those and frame scratch together; nothing from the old frame is replayable.
	r.sched = scheduler{}
	r.sceneOpts = ebiten.DrawTrianglesShaderOptions{}
	r.surfaceDynamic = surfaceUpload{}
	r.surfaceCache = [4]surfaceUpload{}
	r.pointPlane = pointPlane{}
	r.pointRows = nil
	r.pointRuns = nil
	r.pointGroups = nil
	r.pointGroupIdx = nil
	r.lastDest = nil
}
