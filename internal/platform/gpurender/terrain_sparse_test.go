package gpurender

import (
	"bytes"
	"fmt"
	"image"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/nanolathe-gg/nanolathe/internal/camera"
	"github.com/nanolathe-gg/nanolathe/internal/drawlist"
	"github.com/nanolathe-gg/nanolathe/internal/world"
)

// sparseFixtureTerrain is an 8×6 tile map over a 40-tile set, laid out so
// tiles repeat and later tiles first appear further right and down: the
// camera walk below meets new tiles on most steps.
func sparseFixtureTerrain() (*world.Terrain, [][drawlist.DetailTilePixels]byte) {
	const cols, rows, tiles = 8, 6, 40
	t := &world.Terrain{CellW: 2 * cols, CellH: 2 * rows, TileIndices: make([]uint16, cols*rows), TileSet: make([][1024]byte, tiles)}
	for i := range t.TileIndices {
		t.TileIndices[i] = uint16((i*7 + i/cols) % tiles)
	}
	detail := make([][drawlist.DetailTilePixels]byte, tiles)
	for id := 0; id < tiles; id++ {
		for y := 0; y < terrainTileSize; y++ {
			for x := 0; x < terrainTileSize; x++ {
				t.TileSet[id][y*terrainTileSize+x] = byte(1 + (x+id)&15 + 16*((y+2*id)&7))
			}
		}
		for y := 0; y < terrainDetailSize; y++ {
			for x := 0; x < terrainDetailSize; x++ {
				detail[id][y*terrainDetailSize+x] = byte(129 + (x*3+id)&7 + 8*((y+id)&7))
			}
		}
	}
	return t, detail
}

// checkSparseTerrainDevicePixels walks a camera across a map at both scales and
// compares an atlas filled on demand with the complete atlas, frame by frame:
// the same pixels, while the sparse atlas places only tiles that came into view.
func checkSparseTerrainDevicePixels() error {
	pal := fixturePalette()
	dense, err := NewChecked(&pal, 80, 48)
	if err != nil {
		return err
	}
	defer dense.ResetSources()
	sparse, err := NewChecked(&pal, 80, 48)
	if err != nil {
		return err
	}
	defer sparse.ResetSources()
	sparse.SetSparseTerrain(true)
	terrain, detail := sparseFixtureTerrain()
	want, got := make([]byte, 80*48*4), make([]byte, 80*48*4)
	for _, scale := range []camera.ViewScale{camera.ViewScaleNative, camera.ViewScaleDetail} {
		for step, pos := range [][2]int32{{3, 5}, {37, 11}, {90, 40}, {150, 120}, {20, 150}, {3, 5}} {
			cam := &camera.Camera{X: pos[0], Z: pos[1], Scale: scale}
			list := terrainFixtureList(terrain, detail, cam, 80, 48, scale)
			dense.Execute(&list, 80, 48).ReadPixels(want)
			sparse.Execute(&list, 80, 48).ReadPixels(got)
			if !bytes.Equal(want, got) {
				return fmt.Errorf("sparse terrain atlas changed pixels at scale %v, camera step %d", scale, step)
			}
		}
		a := sparse.atlasFor(terrain, detail, scale)
		if a == nil || a.cells == nil || a.filled == 0 || a.filled >= a.count {
			return fmt.Errorf("sparse terrain atlas at scale %v did not fill on demand: %+v", scale, a)
		}
	}
	return nil
}

// An atlas filled on demand hands each tile one cell the first time it is in
// view, in order, and never again; tiles outside the range stay unplaced.
func TestSparseTerrainAtlasPlacesVisibleTilesOnce(t *testing.T) {
	skipAfterDeviceLoop(t)
	terrain, detail := sparseFixtureTerrain()
	r := &Renderer{tileAtlases: make(map[tileAtlasKey]*tileAtlas)}
	r.SetSparseTerrain(true)
	a := r.atlasFor(terrain, detail, camera.ViewScaleDetail)
	if a == nil || a.cells == nil || a.filled != 0 || len(a.pages) != 0 {
		t.Fatal("a sparse atlas did not start empty")
	}
	a.fill(&r.pages, terrain, detail, camera.ViewScaleDetail, 8, 0, 0, 1, 1)
	seen := map[int]bool{}
	for ty := 0; ty <= 1; ty++ {
		for tx := 0; tx <= 1; tx++ {
			id := int(terrain.TileIndices[ty*8+tx])
			if a.cellOf(id) < 0 || a.cellOf(id) >= a.filled {
				t.Fatalf("visible tile %d has no cell", id)
			}
			seen[id] = true
		}
	}
	if a.filled != len(seen) || len(a.pages) != 1 {
		t.Fatalf("filled %d cells on %d pages for %d distinct visible tiles", a.filled, len(a.pages), len(seen))
	}
	before := a.filled
	a.fill(&r.pages, terrain, detail, camera.ViewScaleDetail, 8, 0, 0, 1, 1)
	if a.filled != before {
		t.Fatal("a tile already placed was placed again")
	}
	if id := int(terrain.TileIndices[5*8+7]); !seen[id] && a.cellOf(id) != -1 {
		t.Fatal("a tile never in view was placed")
	}
	// A reset keeps the page for the next atlas rather than releasing it.
	page := a.pages[0]
	released := map[*ebiten.Image]int{}
	r.resetSources(func(img *ebiten.Image) { released[img]++ })
	if released[page] != 0 || len(r.tileAtlases) != 0 || len(r.pages.free) != 1 || r.pages.free[0] != page {
		t.Fatal("a sparse atlas page was not kept for reuse when its sources retired")
	}
	// The complete atlas stays the default.
	d := (&Renderer{tileAtlases: make(map[tileAtlasKey]*tileAtlas)}).atlasFor(terrain, detail, camera.ViewScaleNative)
	if d.cells != nil || d.cellOf(17) != 17 {
		t.Fatal("the complete atlas did not map tile i to cell i")
	}
}

// The pool keeps pages of any fixed size up to its byte bound; past it the
// caller releases them. Reuse itself clears on the device, which the source
// lifecycle device fixture covers across three reset generations.
func TestPagePoolKeepsPagesWithinItsBound(t *testing.T) {
	var p pagePool
	if !p.keep(&ebiten.Image{}, 1088, 1088) {
		t.Fatal("an empty pool refused a page")
	}
	kept := 1
	for p.keep(&ebiten.Image{}, 2048, 2048) {
		kept++
	}
	if p.bytes > recycledPageBytes || len(p.free) != kept || len(p.sizes) != kept || p.sizes[0] != image.Pt(1088, 1088) {
		t.Fatalf("pool kept %d pages, %d bytes, against a %d byte bound", len(p.free), p.bytes, recycledPageBytes)
	}
}
