package gpurender

import (
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/nanolathe-gg/nanolathe/formats"
	"github.com/nanolathe-gg/nanolathe/internal/world"
)

// Repeated battles have new immutable source identities, even on the same map.
// Reset must release those identities and shared device pages without losing
// the process's shaders or display surfaces (DESIGN_GPU_RENDERER §2.3).
func TestLifecycleResetSourcesReleasesBattleResources(t *testing.T) {
	shared, tile, table, output := &ebiten.Image{}, &ebiten.Image{}, &ebiten.Image{}, &ebiten.Image{}
	shader := &ebiten.Shader{}
	frame := &formats.GAFFrame{}
	r := &Renderer{
		tileAtlases:  map[tileAtlasKey]*tileAtlas{{terrain: &world.Terrain{}}: {pages: []*ebiten.Image{tile}}},
		gafImages:    map[*formats.GAFFrame]*ebiten.Image{frame: shared},
		scene:        sceneAtlas{pages: []*scenePage{{img: shared}}, frames: map[*formats.GAFFrame]sceneEntry{frame: {}}, pcx: map[*formats.PCX]sceneEntry{{}: {}}, fonts: map[*formats.FNT]*fntAtlas{{}: {}}},
		heat:         treeHeat{sources: []treeHeatSource{{width: 32}}, treeDisabled: true, wreckDisabled: true},
		textureAtlas: modelTextureAtlas{slots: map[*formats.GAFFrame]modelTextureSlot{frame: {img: shared}}, page: shared},
		fog:          fogPass{shader: shader, compiled: true, atlas: shared, atlasGray: [4]*formats.GAFEntry{{}}},
		scene2D:      shader, surfaces: [2]*ebiten.Image{output}, w: 640, h: 480,
	}
	groupKey, groupColour := &ebiten.Image{}, &ebiten.Image{}
	r.modelDirect.groups = modelGroupMergeLane{key: groupKey, colour: groupColour, shader: shader}
	pageKey, pageColour := &ebiten.Image{}, &ebiten.Image{}
	r.modelDirect.pages[0] = modelDirectPage{key: pageKey, colour: pageColour, usedRows: 12}
	flash := &ebiten.Image{}
	r.sched.flash.img = flash
	reflectionHeight := &ebiten.Image{}
	r.reflections.height = reflectionHeight
	r.tables.atlas = table
	r.sceneOpts.Images[0] = shared
	r.surfaceCache[0] = surfaceUpload{identity: 8, entry: sceneEntry{ok: true}}
	released := map[*ebiten.Image]int{}
	r.resetSources(func(img *ebiten.Image) { released[img]++ })
	if released[tile] != 1 || released[shared] != 1 || released[groupKey] != 1 || released[groupColour] != 1 || len(released) != 4 {
		t.Fatalf("shared source release counts=%v", released)
	}
	if r.sched.flash.img != flash || released[flash] != 0 || r.sched.flash.regions != nil {
		t.Fatal("source reset released the flash disc page or kept the old generation's placements")
	}
	if len(r.tileAtlases) != 0 || len(r.gafImages) != 0 || len(r.scene.pages) != 0 || len(r.scene.frames) != 0 || len(r.scene.pcx) != 0 || len(r.scene.fonts) != 0 || len(r.textureAtlas.slots) != 0 {
		t.Fatal("previous battle source identities retained")
	}
	if r.fog.atlas != nil || r.fog.atlasGray[0] != nil || r.sceneOpts.Images[0] != nil || r.surfaceCache[0].identity != 0 {
		t.Fatal("compiled or paused-source dependencies retained")
	}
	if r.heat.sources != nil || !r.heat.treeDisabled || !r.heat.wreckDisabled {
		t.Fatal("source reset retained heat sources or lost its comparison control")
	}
	if r.modelDirect.groups.key != nil || r.modelDirect.groups.colour != nil || r.modelDirect.groups.shader != shader {
		t.Fatal("source reset retained group scratch or lost its shader")
	}
	// The lane's fixed-size pages are frame scratch, cleared before each use:
	// they survive like the output surfaces, with nothing marked used.
	if pg := r.modelDirect.pages[0]; pg.key != pageKey || pg.colour != pageColour || pg.usedRows != 0 || released[pageKey] != 0 {
		t.Fatal("source reset released or kept stale use of the model lane's scratch pages")
	}
	if r.reflections.height != reflectionHeight || released[reflectionHeight] != 0 {
		t.Fatal("source reset released the reflection scratch plane")
	}
	if r.scene2D != shader || r.fog.shader != shader || !r.fog.compiled || r.tables.atlas != table || r.surfaces[0] != output || r.w != 640 || r.h != 480 {
		t.Fatal("source reset changed renderer configuration")
	}
	r.resetSources(func(img *ebiten.Image) { t.Fatal("empty reset released an already retired image") })
}

// The model texture pages and the water mask go back to the pool, not to the
// device: every page the texture atlas filled, although each in-page slot
// names its page, while an oversized frame's standalone texture is released.
func TestResetSourcesPoolsTexturePagesAndWaterMask(t *testing.T) {
	full, open, alone, mask := &ebiten.Image{}, &ebiten.Image{}, &ebiten.Image{}, &ebiten.Image{}
	a, b, c := &formats.GAFFrame{}, &formats.GAFFrame{}, &formats.GAFFrame{}
	r := &Renderer{
		gafImages: map[*formats.GAFFrame]*ebiten.Image{c: alone},
		textureAtlas: modelTextureAtlas{
			slots: map[*formats.GAFFrame]modelTextureSlot{a: {img: full}, b: {img: open}, c: {img: alone}},
			page:  open, pages: []*ebiten.Image{full, open},
		},
	}
	r.water.mask, r.water.w, r.water.h = mask, 64, 32
	released := map[*ebiten.Image]int{}
	r.resetSources(func(img *ebiten.Image) { released[img]++ })
	if released[alone] != 1 || len(released) != 1 {
		t.Fatalf("release counts=%v, want only the standalone texture", released)
	}
	kept := map[*ebiten.Image]bool{}
	for _, img := range r.pages.free {
		kept[img] = true
	}
	if !kept[full] || !kept[open] || !kept[mask] || len(r.pages.free) != 3 || r.water.mask != nil || r.textureAtlas.pages != nil {
		t.Fatal("texture pages or the water mask were not pooled for the next generation")
	}
}
