package gpurender

import (
	"testing"

	"github.com/nanolathe-gg/nanolathe/formats"
)

// Sprites placed at the loading boundary are the entries Execute finds: the
// first draw of a prepared frame places and uploads nothing, so a camera jump
// onto them costs the frame no atlas work (DESIGN_GPU_RENDERER §14.8).
func TestPreparedSpritesAreNotPlacedAgain(t *testing.T) {
	pal := fixturePalette()
	r, err := NewChecked(&pal, 64, 64)
	if err != nil {
		t.Fatalf("renderer: %v", err)
	}
	frames := []*formats.GAFFrame{
		{Width: 12, Height: 9, Pixels: make([]byte, 12*9)},
		{Width: 30, Height: 7, Pixels: make([]byte, 30*7)},
		{Width: 5, Height: 40, Pixels: make([]byte, 5*40)},
	}
	r.PrepareSprites(frames)
	prepared := make([]sceneEntry, len(frames))
	for i, f := range frames {
		prepared[i] = r.scene.frames[f]
		if !prepared[i].ok {
			t.Fatalf("frame %d was not placed", i)
		}
	}
	r.scene.beginFrameUploads()
	for i, f := range frames {
		if e := r.sceneFrameFor(f); e != prepared[i] {
			t.Errorf("frame %d: drawn at %+v, prepared at %+v", i, e, prepared[i])
		}
	}
	if uploads, _ := r.AtlasUploads(); uploads != 0 {
		t.Errorf("drawing prepared frames uploaded %d sprites", uploads)
	}
	late := &formats.GAFFrame{Width: 8, Height: 8, Pixels: make([]byte, 64)}
	r.sceneFrameFor(late)
	if uploads, unionKB := r.AtlasUploads(); uploads != 1 || unionKB != 0 {
		t.Errorf("one unprepared 8x8 frame: %d uploads spanning %d KiB", uploads, unionKB)
	}
}
