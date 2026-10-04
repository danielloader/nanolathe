package gpurender

import (
	"fmt"
	"math"

	"github.com/nanolathe-gg/nanolathe/formats"
	"github.com/nanolathe-gg/nanolathe/internal/camera"
	"github.com/nanolathe-gg/nanolathe/internal/drawlist"
)

// Overview margins stay void even when a feature's sprite reaches past the map
// and beyond the fog's border cells (DESIGN_GPU_RENDERER §16.7, issue #90).
func checkWorldMarginDevicePixels() error {
	const w, h = 256, 160
	pal := fixturePalette()
	r, err := NewChecked(&pal, w, h)
	if err != nil {
		return err
	}
	terrain := terrainFixtureTerrain() // 64×32 map pixels
	art := &formats.GAFFrame{Width: 256, Height: 256, Pixels: make([]byte, 256*256)}
	for i := range art.Pixels {
		art.Pixels[i] = 100
	}
	for _, factor := range []float32{.25, .375, .7, 1, 1.5, 2} {
		step := camera.Zoom(factor * float32(camera.ZoomUnit)).Step()
		k := factor / float32(step.Float())
		const ox, oy = float32(.375), float32(-.25)
		var list drawlist.List
		list.RecordClear()
		list.RecordWorld(drawlist.WorldSpace{Begin: true, Factor: factor, Step: step, OffsetX: ox, OffsetY: oy, RecordW: 512, RecordH: 384})
		list.RecordTerrain(drawlist.Terrain{Terrain: terrain, OriginX: -48, OriginY: -32, Scale: step, DstW: 512, DstH: 384})
		// The art covers the terrain and spills into all four margins.
		list.RecordSprite(drawlist.Sprite{Frame: art, X: step.Px(16), Y: 0, Kind: drawlist.BlitKeyed})
		list.RecordWorld(drawlist.WorldSpace{})
		// A second region carries world-positioned UI with no terrain command;
		// it must not reuse the first region's margin cleanup and erase the HUD.
		list.RecordFill(drawlist.Fill{Rect: drawlist.Rect{X: 2, Y: 2, W: 4, H: 4}, Index: 211})
		list.RecordWorld(drawlist.WorldSpace{Begin: true, Factor: factor, Step: step, RecordW: 512, RecordH: 384})
		list.RecordWorld(drawlist.WorldSpace{})
		list.RecordExpand()
		pixels := make([]byte, w*h*4)
		r.Execute(&list, w, h).ReadPixels(pixels)
		left := int(math.Ceil(float64(step.Px(48))*float64(k) + float64(ox) - .5))
		top := int(math.Ceil(float64(step.Px(32))*float64(k) + float64(oy) - .5))
		right := int(math.Ceil(float64(step.Px(112))*float64(k) + float64(ox) - .5))
		bottom := int(math.Ceil(float64(step.Px(64))*float64(k) + float64(oy) - .5))
		for y := 0; y < h; y++ {
			for x := 0; x < w; x++ {
				want := byte(0)
				if x >= left && x < right && y >= top && y < bottom {
					want = 100
				}
				if x >= 2 && x < 6 && y >= 2 && y < 6 {
					want = 211
				}
				if err := checkExactIndex(fmt.Sprintf("world margin factor %v pixel %d,%d", factor, x, y), pixels, (y*w+x)*4, &pal, want); err != nil {
					return err
				}
			}
		}
	}
	return nil
}
