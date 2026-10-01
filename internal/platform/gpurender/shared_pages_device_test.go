package gpurender

import (
	"bytes"
	"fmt"

	"github.com/nanolathe-gg/nanolathe/internal/drawlist"
)

// Invoked only by the existing opt-in device loop. Two renderers drawing
// through one SharedPages, interleaved frame by frame and across a reset,
// compose exactly what each composes alone: a page the other renderer drew
// last is never read before this one redraws it.
func checkSharedPagesDevicePixels() error {
	pal := fixturePalette()
	sceneA := func() drawlist.List { return fixtureModelList() }
	sceneB := func() drawlist.List {
		var list drawlist.List
		list.RecordClear()
		list.RecordFill(drawlist.Fill{Rect: drawlist.Rect{X: 0, Y: 0, W: 80, H: 48}, Index: 9, Style: drawlist.FillSolid})
		list.RecordModel(drawlist.Model{Geometry: fixtureGeometry(0, true, fixtureFace(30, 20, 9, 7, 40, 12), fixtureFace(33, 22, 5, 5, 60, 14))})
		list.RecordModel(drawlist.Model{Geometry: fixtureGeometry(0, true, fixtureFace(2, 30, 12, 12, 20, 21))})
		list.RecordExpand()
		return list
	}
	alone := func(scene func() drawlist.List) ([]byte, error) {
		r, err := NewChecked(&pal, 80, 48)
		if err != nil {
			return nil, err
		}
		defer r.ResetSources()
		list := scene()
		img := r.Execute(&list, 80, 48)
		if img == nil {
			return nil, fmt.Errorf("shared pages: reference frame missing")
		}
		px := make([]byte, 80*48*4)
		img.ReadPixels(px)
		return px, nil
	}
	wantA, err := alone(sceneA)
	if err != nil {
		return err
	}
	wantB, err := alone(sceneB)
	if err != nil {
		return err
	}
	if bytes.Equal(wantA, wantB) {
		return fmt.Errorf("shared pages: the two scenes compose alike, so sharing could not show")
	}
	set := NewSharedPages()
	a, err := NewChecked(&pal, 80, 48)
	if err != nil {
		return err
	}
	b, err := NewChecked(&pal, 80, 48)
	if err != nil {
		return err
	}
	defer a.ResetSources()
	defer b.ResetSources()
	a.SharePages(set)
	b.SharePages(set)
	got := make([]byte, 80*48*4)
	for pass := 0; pass < 3; pass++ {
		for _, step := range []struct {
			r     *Renderer
			scene func() drawlist.List
			want  []byte
			name  string
		}{{a, sceneA, wantA, "A"}, {b, sceneB, wantB, "B"}} {
			list := step.scene()
			img := step.r.Execute(&list, 80, 48)
			if img == nil {
				return fmt.Errorf("shared pages: renderer %s frame missing on pass %d", step.name, pass)
			}
			img.ReadPixels(got)
			if !bytes.Equal(got, step.want) {
				return fmt.Errorf("shared pages: renderer %s composed differently from its own pages on pass %d", step.name, pass)
			}
		}
		if a.modelDirect.pages[0].key == nil || a.modelDirect.pages[0].key != b.modelDirect.pages[0].key {
			return fmt.Errorf("shared pages: the renderers did not draw through one page")
		}
		// The renderers trade scenes' sources through the shared pool.
		a.ResetSources()
	}
	return nil
}
