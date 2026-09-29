package gpurender

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/nanolathe-gg/nanolathe/internal/drawlist"
)

// These lock the approved artistic treatment, not retail material behavior.
func TestModelMaterialControlsAndPacking(t *testing.T) {
	r := &Renderer{}
	f := drawlist.ModelFace{Normal: [3]float32{-0.35, -0.15, 0.9246621}, Material: drawlist.ModelMaterialMetal}
	base := metalGlintColor(90, 0.8)
	if r.modelFinishColor(r.laneCtx(), &f, base) != base {
		t.Fatal("disabled materials changed glint")
	}
	r.setMaterials(true)
	for material := uint8(0); material <= 3; material++ {
		f.Material = material
		for low := 0; low < 65536; low++ {
			packed := int(r.modelFinishColor(r.laneCtx(), &f, float32(low)))
			if packed%65536 != low {
				t.Fatalf("material %d corrupted palette/glint %d", material, low)
			}
			want := 0
			if material == drawlist.ModelMaterialMetal || material == drawlist.ModelMaterialPaint {
				want = int(material) + 7*4
			}
			if packed/65536 != want {
				t.Fatalf("material %d packed = %d, want %d", material, packed/65536, want)
			}
		}
	}
	f.Material, f.Normal = drawlist.ModelMaterialMetal, [3]float32{0, 0, -1}
	if int(r.modelFinishColor(r.laneCtx(), &f, base))/65536 != int(f.Material) {
		t.Fatal("downward normal retained the overhead response")
	}
}

func TestModelMaterialShadersCompile(t *testing.T) {
	for _, source := range []string{modelDirectColourShaderSource(), scene2DShaderSource()} {
		shader, err := ebiten.NewShader([]byte(source))
		if err != nil {
			t.Fatal(err)
		}
		shader.Deallocate()
	}
}

// checkMaterialDevicePixels exercises the actual keyed color pass. Material
// changes must stay inside visible, unreplaced body texels with no extra pass.
func checkMaterialDevicePixels() error {
	pal := fixturePalette()
	pal.Base[90] = [4]byte{110, 110, 110, 255}
	pal.Base[91] = [4]byte{130, 65, 25, 255}
	const w, h = 190, 100
	r, err := NewChecked(&pal, w, h)
	if err != nil {
		return err
	}
	r.setMetalGlint(true)
	var list drawlist.List
	list.RecordClear()
	list.RecordFill(drawlist.Fill{Rect: drawlist.Rect{W: w, H: h}, Index: 25})
	for i := 0; i < 8; i++ {
		f := directFace(0, 0, 22, 22, 90, 10, 10)
		f.Normal = [3]float32{-0.35, -0.15, 0.9246621}
		f.Material = drawlist.ModelMaterialMetal
		if i == 1 {
			f.Color, f.Material = 91, drawlist.ModelMaterialPaint
		}
		if i == 2 {
			f.Material = drawlist.ModelMaterialDefault
		}
		if i == 3 {
			f.Color = 1
		}
		g := directSubject(10+int32(i%4)*44, 10+int32(i/4)*44, 22, 22, f)
		switch i {
		case 4, 5:
			cover := f
			cover.Color, cover.Material = 91, drawlist.ModelMaterialDefault
			if i == 4 { // Later equal-key face wins.
				g.Faces = append(g.Faces, cover)
			} else { // Higher existing key hides the later material face.
				cover.Vertices = append([]drawlist.ModelVertex(nil), cover.Vertices...)
				for j := range cover.Vertices {
					cover.Vertices[j].Key = 20
				}
				g.Faces = append([]drawlist.ModelFace{cover}, f)
			}
		case 6:
			g.Reveal = &drawlist.ModelReveal{Line: 20, Floor: 20, Below: 91, Band: 91, Above: 91}
		case 7:
			g.Waterline, g.WaterlineKey = drawlist.ModelWaterlineErase, 255
		}
		list.RecordModel(drawlist.Model{Geometry: g})
	}
	list.RecordExpand()
	read := func(on bool) ([]byte, ModelStats) {
		r.setMaterials(on)
		pixels := make([]byte, w*h*4)
		r.Execute(&list, w, h).ReadPixels(pixels)
		return pixels, r.ModelStats()
	}
	off, os := read(false)
	on, ns := read(true)
	again, _ := read(false)
	if !bytes.Equal(off, again) {
		return fmt.Errorf("materials off retained frame state")
	}
	if os.DeviceDraws != ns.DeviceDraws || os.Passes != ns.Passes || os.DirectPages != ns.DirectPages || os.SubmittedVertices != ns.SubmittedVertices || os.SubmittedIndices != ns.SubmittedIndices || ns.MaterialFaces == 0 {
		return fmt.Errorf("materials changed submissions or missed annotation: off=%+v on=%+v", os, ns)
	}
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			// Only the metal and paint subjects may change, including their
			// resolved edge coverage. Other subjects exercise rejection paths.
			if y >= 9 && y <= 33 && ((x >= 9 && x <= 33) || (x >= 53 && x <= 77)) {
				continue
			}
			at := (y*w + x) * 4
			if !bytes.Equal(off[at:at+4], on[at:at+4]) {
				return fmt.Errorf("material changed hidden/unannotated texel at %d,%d", x, y)
			}
		}
	}
	for i := 0; i < 2; i++ {
		at := (20*w + 20 + i*44) * 4
		if bytes.Equal(off[at:at+4], on[at:at+4]) {
			return fmt.Errorf("material %d unchanged", i)
		}
	}
	// The independent wreck screen blend must still brighten a material face
	// through the same body composite with identical submission counts.
	makeWreck := func(emission [3]float32) drawlist.List {
		var l drawlist.List
		l.RecordClear()
		l.RecordFill(drawlist.Fill{Rect: drawlist.Rect{W: w, H: h}, Index: 25})
		f := directFace(0, 0, 22, 22, 90, 10, 10)
		f.Material = drawlist.ModelMaterialMetal
		f.Normal = [3]float32{0, 0, 1}
		g := directSubject(10, 10, 22, 22, f)
		g.WreckEmission = emission
		l.RecordModel(drawlist.Model{Geometry: g})
		l.RecordExpand()
		return l
	}
	list = makeWreck([3]float32{})
	cold, cs := read(true)
	list = makeWreck([3]float32{.9, .75, .435})
	hot, hs := read(true)
	at := (20*w + 20) * 4
	if hot[at] <= cold[at] || hot[at+1] <= cold[at+1] || hs.DeviceDraws != cs.DeviceDraws || hs.Passes != cs.Passes {
		return fmt.Errorf("material lost independent wreck cooling or added submissions: %v -> %v", cold[at:at+4], hot[at:at+4])
	}
	return checkFinishAnchorDevicePixels()
}

// checkFinishAnchorDevicePixels locks the anchor of §23.7 and §29.1: the
// finish and the glint move light between orientations and never lift a face
// that points straight up, whose palette colour is what the authored art shows
// from above. An overhead saturated metal face, an overhead paint face and an
// overhead neutral face under the glint keep their exact pixels; a metal face
// turned away from the key darkens without a tint; a neutral metal face turned
// toward it brightens with a cool cast.
func checkFinishAnchorDevicePixels() error {
	pal := fixturePalette()
	pal.Base[90] = [4]byte{110, 110, 110, 255}
	pal.Base[91] = [4]byte{130, 65, 25, 255}
	const w, h = 120, 30
	r, err := NewChecked(&pal, w, h)
	if err != nil {
		return err
	}
	up := [3]float32{0, 0, 1}
	subjects := []struct {
		color    uint8
		material uint8
		normal   [3]float32
	}{
		{91, drawlist.ModelMaterialMetal, up},
		{91, drawlist.ModelMaterialPaint, up},
		{90, drawlist.ModelMaterialDefault, up},
		{90, drawlist.ModelMaterialMetal, [3]float32{1, 0, 0}},
		{90, drawlist.ModelMaterialMetal, [3]float32{-0.35, -0.15, 0.9246621}},
	}
	var list drawlist.List
	list.RecordClear()
	list.RecordFill(drawlist.Fill{Rect: drawlist.Rect{W: w, H: h}, Index: 25})
	for i, sub := range subjects {
		f := directFace(0, 0, 16, 16, sub.color, 10, 10)
		f.Normal, f.Material = sub.normal, sub.material
		list.RecordModel(drawlist.Model{Geometry: directSubject(6+int32(i)*22, 6, 16, 16, f)})
	}
	list.RecordExpand()
	read := func(on bool) []byte {
		r.setMetalGlint(on)
		r.setMaterials(on)
		pixels := make([]byte, w*h*4)
		r.Execute(&list, w, h).ReadPixels(pixels)
		return pixels
	}
	off, on := read(false), read(true)
	at := func(i int) int { return (14*w + 14 + i*22) * 4 }
	for i := 0; i < 3; i++ {
		if !bytes.Equal(off[at(i):at(i)+3], on[at(i):at(i)+3]) {
			return fmt.Errorf("overhead face %d lifted: %v -> %v", i, off[at(i):at(i)+4], on[at(i):at(i)+4])
		}
	}
	away, lit := on[at(3):at(3)+3], on[at(4):at(4)+3]
	if away[0]+20 > off[at(3)] || away[0] != away[1] || away[1] != away[2] {
		return fmt.Errorf("metal turned away from the key did not darken untinted: %v -> %v", off[at(3):at(3)+3], away)
	}
	if lit[0] <= off[at(4)] || lit[2] <= lit[1] || lit[1] <= lit[0] {
		return fmt.Errorf("metal turned toward the key missed its cool highlight: %v -> %v", off[at(4):at(4)+3], lit)
	}
	return nil
}
