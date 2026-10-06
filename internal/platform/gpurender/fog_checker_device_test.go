package gpurender

import (
	"fmt"

	"github.com/nanolathe-gg/nanolathe/formats"
	"github.com/nanolathe-gg/nanolathe/internal/drawlist"
	"github.com/nanolathe-gg/nanolathe/internal/render"
)

// Called by the existing opt-in device fixture. All three GPU paths must use
// even destination parity, including clipped source columns and asymmetric key
// holes [03 §3.3]. The renderer's approved colour policy is unchanged.
func checkFogCheckerPhaseDevicePixels() error {
	const w, h = 20, 12
	for _, parity := range []int32{0, 1} {
		for _, left := range []int32{-3, 3} {
			for _, mode := range []string{"fill", "atlas", "ordered"} {
				pal := fixturePalette()
				pal.Base[100] = [4]byte{30, 60, 90, 255}
				r, err := NewChecked(&pal, w, h)
				if err != nil {
					return err
				}
				// Native camera projection gives this left edge and a top of
				// one or two while supplying the requested camera parity.
				camX := int32(16) - left
				camZ := int32(15) - parity
				op := fogOpAt(0, 0, camX, camZ, render.FogKindPatterned)
				fg := drawlist.Fog{Ops: []render.FogOp{op}}
				f := &formats.GAFFrame{Width: 9, Height: 7, ColorKey: 9, Pixels: make([]byte, 63), Transparent: make([]bool, 63)}
				for _, i := range []int{1, 2*9 + 4, 5*9 + 6} {
					f.Transparent[i], f.Pixels[i] = true, 9
				}
				if mode != "fill" {
					root := f
					if mode == "ordered" {
						root = fogTestComposite(f)
					}
					fg.Gray[0] = &formats.GAFEntry{FrameCount: 1, Frames: []formats.GAFFrameRef{{Frame: root}}}
					fg.Ops[0].Kind = render.FogKindGAFCh1
					fg.Ops[0].Variant, fg.Ops[0].Frame, fg.Ops[0].Patterned = 0, 0, true
				}
				var list drawlist.List
				list.RecordClear()
				list.RecordFill(drawlist.Fill{Rect: drawlist.Rect{W: w, H: h}, Index: 100})
				list.RecordFog(fg)
				list.RecordExpand()
				pixels := make([]byte, w*h*4)
				r.Execute(&list, w, h).ReadPixels(pixels)
				for y := 0; y < h; y++ {
					for x := 0; x < w; x++ {
						want := [4]byte{30, 60, 90, 255}
						sx, sy := x-int(left), y-int(1+parity)
						covered := sx >= 0 && sx < 32 && sy >= 0 && sy < 32
						if mode != "fill" {
							covered = sx >= 0 && sx < 9 && sy >= 0 && sy < 7 && !f.Transparent[sy*9+sx]
						}
						if covered && (int32(x+y)+parity)&1 == 0 {
							want = pal.Base[render.FogDarkPaletteIndex]
						}
						at := (y*w + x) * 4
						if got := [4]byte(pixels[at : at+4]); got != want {
							return fmt.Errorf("fog %s parity=%d left=%d pixel(%d,%d)=%v want %v", mode, parity, left, x, y, got, want)
						}
					}
				}
			}
		}
	}
	return nil
}
