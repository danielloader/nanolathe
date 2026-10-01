package gpurender

import (
	"bytes"
	"slices"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/nanolathe-gg/nanolathe/formats"
	"github.com/nanolathe-gg/nanolathe/internal/drawlist"
	"github.com/nanolathe-gg/nanolathe/internal/world"
)

func TestPreparedWaterMaskMatchesLazyConstruction(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(*world.Terrain)
		red    byte
		blue   byte
	}{
		{name: "water", red: 255},
		{name: "acid", change: func(ter *world.Terrain) { ter.WaterDoesDamage, ter.WaterDamage = 1, 1 }, red: 255},
		{name: "lava", change: func(ter *world.Terrain) { ter.LavaWorld = true }, red: 255},
		{name: "zero-level lava", change: func(ter *world.Terrain) {
			ter.SeaLevel, ter.LavaWorld = 0, true
			ter.Plot = world.ExpandPlot(make([]formats.TNTAttribute, 16*16), 16, 16)
		}, red: 255},
		{name: "void liquid", change: func(ter *world.Terrain) {
			for i := range ter.Plot {
				ter.Plot[i].SetFeature(world.PlotFeatureVoid)
			}
		}, red: 255, blue: 128},
		{name: "dry", change: func(ter *world.Terrain) { ter.SeaLevel = 0 }, blue: 255},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ter := waterFixtureTerrain()
			if tc.change != nil {
				tc.change(ter)
			}
			before := slices.Clone(ter.Plot)
			prepared := BuildWaterMask(ter)
			pixels, w, h, step, blocks, bw, bh := waterMaskPixels(ter)
			if prepared.source != ter || prepared.w != w || prepared.h != h || prepared.step != step || prepared.blockW != bw || prepared.blockH != bh {
				t.Fatal("preparation changed source identity or mask geometry")
			}
			if !bytes.Equal(prepared.pixels, pixels) || !slices.Equal(prepared.blocks, blocks) {
				t.Fatal("preparation changed mask channels or its block index")
			}
			if !slices.Equal(ter.Plot, before) {
				t.Fatal("CPU mask construction changed terrain plots")
			}
			i := ((80/step)*w + 48/step) * 4
			if prepared.pixels[i] != tc.red || prepared.pixels[i+2] != tc.blue {
				t.Fatal("authored fixture did not exercise the intended receiving medium")
			}
		})
	}
}

func TestPreparedWaterMaskInvalidInputsNeedNoDevice(t *testing.T) {
	for _, ter := range []*world.Terrain{
		nil, {}, {CellW: 1, CellH: 16}, {CellW: 16, CellH: 1},
		{CellW: 16, CellH: 16, Plot: make([]world.PlotCell, 16*16-1)},
	} {
		prepared := BuildWaterMask(ter)
		if prepared.source != ter || prepared.w != 0 || prepared.h != 0 || prepared.step != 0 || len(prepared.pixels) != 0 || len(prepared.blocks) != 0 || prepared.blockW != 0 || prepared.blockH != 0 {
			t.Fatal("invalid terrain produced mask storage or lost its identity")
		}
		r := &Renderer{}
		r.PrepareWaterMask(prepared)
		if r.water.mask != nil || r.water.source != ter {
			t.Fatal("empty preparation uploaded an image or changed the terrain identity")
		}
		(*Renderer)(nil).PrepareWaterMask(prepared)
	}

	previous := &world.Terrain{}
	r := &Renderer{water: waterLayer{source: previous, w: 7, h: 5, step: 2, blocks: []bool{true}, blockW: 1, blockH: 1}}
	r.PrepareWaterMask(nil)
	r.PrepareWaterMask(&PreparedWaterMask{})
	r.PrepareWaterMask(BuildWaterMask(nil))
	if r.water.source != previous || r.water.w != 7 {
		t.Fatal("absent preparation changed the current source")
	}
	replacement := &world.Terrain{}
	r.PrepareWaterMask(BuildWaterMask(replacement))
	if r.water.source != replacement || r.water.mask != nil || r.water.w != 0 || r.water.h != 0 || r.water.step != 0 || len(r.water.blocks) != 0 || r.water.blockW != 0 || r.water.blockH != 0 {
		t.Fatal("invalid replacement reused geometry from another terrain")
	}
}

func TestPreparedWaterMaskReusesMatchingDrawsAndRetiresSources(t *testing.T) {
	skipAfterDeviceLoop(t)
	r := &Renderer{}
	defer r.ResetSources()
	first, second := waterFixtureTerrain(), waterFixtureTerrain()
	prepared := BuildWaterMask(first)
	r.PrepareWaterMask(prepared)
	firstMask := r.water.mask
	if firstMask == nil || r.water.source != first || r.water.w != prepared.w || r.water.h != prepared.h || r.water.step != prepared.step || !slices.Equal(r.water.blocks, prepared.blocks) || r.water.blockW != prepared.blockW || r.water.blockH != prepared.blockH {
		t.Fatal("prepared mask did not install its source, geometry and block index")
	}
	record := drawlist.Terrain{Terrain: first, OriginX: 19, OriginY: 23}
	r.prepareWater(record)
	r.PrepareWaterMask(prepared)
	if r.water.mask != firstMask || r.water.record.Terrain != first || r.water.record.OriginX != 19 || r.water.record.OriginY != 23 {
		t.Fatal("matching draw replaced the prepared mask or lost its current projection")
	}

	// A freshly loaded copy is a new source even when all of its bytes match.
	// Its mask is written wholesale into the pooled page of the same size, so
	// the source, not the image, is what changes.
	r.PrepareWaterMask(BuildWaterMask(second))
	if r.water.mask == nil || r.water.source != second || len(r.pages.free) != 0 {
		t.Fatal("different terrain identity kept the previous source or did not reuse its page")
	}
	r.prepareWater(record)
	if r.water.source != first || r.water.mask == nil {
		t.Fatal("a draw for another terrain kept the prepared source")
	}
	lazyMask := r.water.mask
	r.PrepareWaterMask(prepared)
	if r.water.mask != lazyMask {
		t.Fatal("preparation replaced a matching mask built by the lazy fallback")
	}

	released := make(map[*ebiten.Image]int)
	r.resetSources(func(img *ebiten.Image) {
		released[img]++
		img.Deallocate()
	})
	if released[lazyMask] != 0 || len(released) != 0 || len(r.pages.free) != 1 || r.pages.free[0] != lazyMask || r.water.mask != nil || r.water.source != nil || r.water.w != 0 || r.water.h != 0 || r.water.step != 0 || r.water.blocks != nil || r.water.blockW != 0 || r.water.blockH != 0 || r.water.record.Terrain != nil {
		t.Fatal("source reset retained the prepared generation or did not pool its image")
	}
}

func TestPreparedWaterMaskUploadDoesNotReadTerrain(t *testing.T) {
	skipAfterDeviceLoop(t)
	ter := waterFixtureTerrain()
	prepared := BuildWaterMask(ter)
	// Invalidate this authored fixture after construction to detect a second
	// traversal during upload. Actual callers freeze the source at transfer.
	ter.Plot = nil
	r := &Renderer{}
	defer r.ResetSources()
	r.PrepareWaterMask(prepared)
	if r.water.mask == nil || r.water.w != prepared.w || r.water.h != prepared.h || !slices.Equal(r.water.blocks, prepared.blocks) {
		t.Fatal("prepared upload recomputed the terrain instead of installing its snapshot")
	}
}

// The fingerprint covers every mask input and nothing the mask ignores: a
// second load of the same map, a moved tree or a changed metal byte keep it;
// a height, a void cell, the sea level or the lava flag move it.
func TestWaterMaskInputsCoverTheMaskAndNothingElse(t *testing.T) {
	base := waterFixtureTerrain()
	key := WaterMaskInputs(base)
	if WaterMaskInputs(waterFixtureTerrain()) != key {
		t.Fatal("two loads of the same terrain fingerprint differently")
	}
	for _, tc := range []struct {
		name   string
		change func(*world.Terrain)
		moves  bool
	}{
		{"height", func(ter *world.Terrain) { ter.Plot[5*16+3][4]++ }, true},
		{"void", func(ter *world.Terrain) { ter.Plot[7*16+7].SetFeature(world.PlotFeatureVoid) }, true},
		{"sea level", func(ter *world.Terrain) { ter.SeaLevel++ }, true},
		{"lava", func(ter *world.Terrain) { ter.LavaWorld = !ter.LavaWorld }, true},
		{"feature", func(ter *world.Terrain) { ter.Plot[7*16+7].SetFeature(3) }, false},
		{"metal", func(ter *world.Terrain) { ter.Plot[7*16+7][7] = 9 }, false},
	} {
		ter := waterFixtureTerrain()
		tc.change(ter)
		if moved := WaterMaskInputs(ter) != key; moved != tc.moves {
			t.Fatalf("%s: fingerprint moved %t, want %t", tc.name, moved, tc.moves)
		}
		if !tc.moves {
			// What the fingerprint ignores, the mask ignores.
			want, _, _, _, wantBlocks, _, _ := waterMaskPixels(base)
			got, _, _, _, gotBlocks, _, _ := waterMaskPixels(ter)
			if !bytes.Equal(got, want) || !slices.Equal(gotBlocks, wantBlocks) {
				t.Fatalf("%s: changed the mask without moving its fingerprint", tc.name)
			}
		}
	}
	mask := BuildWaterMask(base)
	other := waterFixtureTerrain()
	bound := mask.ForTerrain(other)
	if bound.source != other || mask.source != base || &bound.pixels[0] != &mask.pixels[0] || bound.w != mask.w || bound.step != mask.step {
		t.Fatal("ForTerrain did not share the pixels under the new terrain identity")
	}
}
