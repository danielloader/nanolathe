package client

import (
	"bytes"
	"image"
	"image/color"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/nanolathe-gg/nanolathe/formats"
	compiledmodel "github.com/nanolathe-gg/nanolathe/internal/model"
	"github.com/nanolathe-gg/nanolathe/internal/palette"
	presentationrender "github.com/nanolathe-gg/nanolathe/internal/render"
	"github.com/nanolathe-gg/nanolathe/internal/sim/numeric"
)

func infectionTestArt() (*palette.Tables, image.Image) {
	pal := &palette.Tables{}
	for i := range pal.Base {
		pal.Base[i] = [4]byte{byte(i), byte(i), byte(i), 255}
	}
	art := image.NewNRGBA(image.Rect(3, 5, 11, 13))
	for y := 5; y < 13; y++ {
		for x := 3; x < 11; x++ {
			art.SetNRGBA(x, y, color.NRGBA{R: 180, G: 30, B: 15, A: 220})
		}
	}
	return pal, art
}

func infectionTestRegistry(refs map[string]texRef) (*ModelTextureRegistry, *compiledmodel.Model) {
	r := emptyModelTextureRegistry(nil, false)
	r.primary = refs
	m := &compiledmodel.Model{Pieces: []compiledmodel.Piece{{Parent: -1}}}
	for name := range refs {
		m.Pieces[0].Primitives = append(m.Pieces[0].Primitives, compiledmodel.Primitive{TextureName: name})
	}
	load := modelTextureLoadKey{kind: modelLoadUnit, id: "mod-unit"}
	r.loads[load] = &unitModel{compiled: m}
	r.byCompiled[m] = load
	r.bindPiece(m, 0, load)
	return r, m
}

func infectionDraw(m *compiledmodel.Model, name string) *presentationrender.UnitDraw {
	draw := testPrimitiveDraw(presentationrender.PrimitiveDraw{TextureName: name, VertexIndices: []uint16{0, 1, 2, 3}, ShadeRow: presentationrender.NoShadeRow},
		[][3]numeric.Fixed{fixedVertex(0, 0, 0), fixedVertex(8, 0, 0), fixedVertex(8, 0, -8), fixedVertex(0, 0, -8)})
	draw.Model = m
	return draw
}

func TestModelSkinInfectionUsesActiveModPixelsAndPalette(t *testing.T) {
	pal, art := infectionTestArt()
	prepare := func(source byte, pal *palette.Tables) (*ModelSkin, *formats.GAFFrame) {
		t.Helper()
		f := &formats.GAFFrame{Width: 2, Height: 1, Pixels: []byte{source, 0}, Transparent: []bool{false, true}}
		r, _ := infectionTestRegistry(map[string]texRef{"unlisted_mod_plating": {kind: texStatic, frame: f}})
		skin, err := r.PrepareInfectedModelSkin("infected", pal, art, nil)
		requireSkinOK(t, err)
		got := skin.generated["unlisted_mod_plating"][f]
		if got == nil || got == f || got.Pixels[0] == source || got.Pixels[1] != 0 {
			t.Fatal("arbitrary mod texture was not composed with preserved recess/transparency")
		}
		if !bytes.Equal(f.Pixels, []byte{source, 0}) || !reflect.DeepEqual(got.Transparent, f.Transparent) {
			t.Fatal("source or transparency changed")
		}
		return skin, got
	}
	old, oldFrame := prepare(180, pal)
	oldPixels := append([]byte(nil), oldFrame.Pixels...)
	_, changed := prepare(100, pal)
	if bytes.Equal(oldFrame.Pixels, changed.Pixels) {
		t.Fatal("same-name mod replacement ignored active source pixels")
	}
	otherPalette := *pal
	for i := range otherPalette.Base {
		otherPalette.Base[i] = pal.Base[255-i]
	}
	_, recolored := prepare(180, &otherPalette)
	if bytes.Equal(oldFrame.Pixels, recolored.Pixels) {
		t.Fatal("composition ignored active palette")
	}
	if old == nil || !bytes.Equal(oldPixels, oldFrame.Pixels) {
		t.Fatal("preparing another skin changed retained frame pixels")
	}
}

func TestModelSkinInfectionAnimationAndTracePreserveSource(t *testing.T) {
	pal, art := infectionTestArt()
	first := &formats.GAFFrame{Width: 1, Height: 1, Pixels: []byte{160}}
	second := &formats.GAFFrame{Width: 1, Height: 1, Pixels: []byte{100}}
	entry := &formats.GAFEntry{FrameCount: 2, Unknown1: 1, Frames: []formats.GAFFrameRef{{Frame: first, Value: 2}, {Frame: second, Value: 3}}}
	r, m := infectionTestRegistry(map[string]texRef{"mod_anim": {kind: texAnimated, key: "mod_anim", frame: first, entry: entry}})
	players := len(r.players)
	skin, err := r.PrepareInfectedModelSkin("infected", pal, art, nil)
	requireSkinOK(t, err)
	c := testModelTextureClient()
	c.SetModelTextureRegistry(r)
	c.SetRendererTraceSink(func(RendererCandidate) {})
	draw := infectionDraw(m, "MOD_ANIM")
	draw.Pieces[0].DontCache = true
	for tick, want := range []*formats.GAFFrame{first, first, second, second, second, first} {
		before := c.collectDrawPolys(draw, teamColor{}, 12, modelCursorUnit)
		if len(before) != 1 || before[0].frame != want {
			t.Fatalf("tick %d: healthy cursor no longer follows authored durations", tick)
		}
		infected := c.collectDrawPolys(draw, teamColor{}, 11, modelCursorUnit, skin)
		if len(infected) != 1 || infected[0].frame != skin.generated["mod_anim"][want] || infected[0].frameIndex != before[0].frameIndex {
			t.Fatalf("tick %d: infected frame or source trace identity changed", tick)
		}
		cached := c.collectDrawPolysLane(draw, teamColor{}, 11, modelCursorUnit, presentationrender.PieceLaneCached, skin)
		if len(cached) != 0 {
			t.Fatal("cached lane admitted live piece")
		}
		draw.Pieces[0].DontCache = false
		cached = c.collectDrawPolys(draw, teamColor{}, 11, modelCursorUnit, skin)
		if cached[0].frame != skin.generated["mod_anim"][first] {
			t.Fatal("cached lane lost first-frame selection")
		}
		draw.Pieces[0].DontCache = true
		if len(r.players) != players || len(c.modelPlayers) != 0 {
			t.Fatal("preparation or draw changed cursor registration")
		}
		r.StepPhase7()
	}
	if first.Pixels[0] != 160 || second.Pixels[0] != 100 || entry.Frames[0].Value != 2 || entry.Frames[1].Value != 3 {
		t.Fatal("source animation was mutated")
	}
}

func TestModelSkinInfectionTeamFlatAndOverrides(t *testing.T) {
	pal, art := infectionTestArt()
	source := &formats.GAFFrame{Width: 1, Height: 1, Pixels: []byte{160}}
	for _, kind := range []texKind{texStatic, texTeam} {
		r, m := infectionTestRegistry(map[string]texRef{"mod": {kind: kind, frame: source, entry: &formats.GAFEntry{Frames: []formats.GAFFrameRef{{Frame: source}}}}})
		skin, err := r.PrepareInfectedModelSkin("infected", pal, art, nil)
		requireSkinOK(t, err)
		c := testModelTextureClient()
		c.SetModelTextureRegistry(r)
		draw := infectionDraw(m, "mod")
		got := c.collectDrawPolys(draw, teamColor{known: true}, 1, modelCursorUnit, skin)
		if kind == texTeam && (got[0].frame != source || len(skin.generated) != 0) {
			t.Fatal("team frame changed")
		}
		for _, family := range []uint8{modelCursorFeature, modelCursorProjectile, modelCursorDebris} {
			got = c.collectDrawPolys(draw, teamColor{known: true}, 1, family, skin)
			if got[0].frame != source {
				t.Fatal("skin escaped unit scope")
			}
		}
	}
	r, m := infectionTestRegistry(map[string]texRef{"mod": {kind: texStatic, frame: source}})
	explicit := &ModelSkin{frames: map[string]*formats.GAFFrame{"mod": {Width: 1, Height: 1, Pixels: []byte{77}}}}
	skin, err := r.PrepareInfectedModelSkin("infected", pal, art, explicit)
	requireSkinOK(t, err)
	c := testModelTextureClient()
	c.SetModelTextureRegistry(r)
	draw := infectionDraw(m, "mod")
	if got := c.collectDrawPolys(draw, teamColor{}, 1, modelCursorUnit, skin); got[0].frame != explicit.frames["mod"] {
		t.Fatal("explicit named override lost priority")
	}
	pr := &draw.Pieces[0].Primitives[0]
	pr.TextureName, pr.IsColored, pr.ColorIndex, pr.ShadeRow = "", 1, 160, 8
	got := c.collectDrawPolys(draw, teamColor{}, 1, modelCursorUnit, skin)
	if got[0].color == 160 || got[0].color != skin.flat[160] || !got[0].useSHD {
		t.Fatal("flat face lost infection remap or SHD")
	}
	pr.TextureName, pr.IsColored = "unresolved", 0
	got = c.collectDrawPolys(draw, teamColor{}, 1, modelCursorUnit, skin)
	if got[0].color != 0xd1 || !got[0].useSHD {
		t.Fatal("infection changed unresolved diagnostic or SHD")
	}
	flatOnly, err := emptyModelTextureRegistry(nil, false).PrepareInfectedModelSkin("flat-only", pal, art, nil)
	requireSkinOK(t, err)
	c.InstallPreparedModelSkin(flatOnly)
	requireSkinOK(t, c.SetOwnerModelSkin(1, "flat-only"))
}

func TestModelSkinInfectionCompositePreservesStructure(t *testing.T) {
	pal, art := infectionTestArt()
	data, err := formats.EncodeGAF([]formats.GAFWriteEntry{{Name: "composite", Frames: []formats.GAFWriteFrame{{Width: 4, Height: 2, XOffset: 1, Subframes: []formats.GAFWriteFrame{
		{Width: 2, Height: 1, XOffset: 0, Pixels: []byte{160, 9}, Transparent: []bool{false, true}},
		{Width: 1, Height: 1, XOffset: 1, AlternateBlitter: 1, Pixels: []byte{100}},
	}}}}})
	requireSkinOK(t, err)
	gaf, err := formats.LoadGAF(data)
	requireSkinOK(t, err)
	source := gaf.Entries[0].Frames[0].Frame
	original := append([]byte(nil), source.Pixels...)
	r, _ := infectionTestRegistry(map[string]texRef{"composite": {kind: texStatic, frame: source}})
	skin, err := r.PrepareInfectedModelSkin("infected", pal, art, nil)
	requireSkinOK(t, err)
	out := skin.generated["composite"][source]
	if out.SubframeCount != source.SubframeCount || out.XOffset != source.XOffset || out.ColorKey != source.ColorKey || !reflect.DeepEqual(out.Transparent, source.Transparent) {
		t.Fatal("composite header or transparency changed")
	}
	if out.Subframes[0] == source.Subframes[0] || out.Subframes[1].AlternateBlitter != 1 || out.Subframes[0].XOffset != 0 {
		t.Fatal("composite children lost independent storage or authored semantics")
	}
	if out.Pixels[1] != out.Subframes[0].Pixels[0] || out.PlainPixels[1] != out.Pixels[1] || !out.Transparent[0] {
		t.Fatal("plain raster disagrees with child placement or included alternate child")
	}
	if !bytes.Equal(source.Pixels, original) || source.Subframes[0].Pixels[0] != 160 {
		t.Fatal("composing composite mutated source")
	}
}

func TestModelSkinInfectionFailuresAndImmutableParallelReads(t *testing.T) {
	pal, art := infectionTestArt()
	source := &formats.GAFFrame{Width: 1, Height: 1, Pixels: []byte{160}}
	r, _ := infectionTestRegistry(map[string]texRef{"mod": {kind: texStatic, frame: source}})
	good, err := r.PrepareInfectedModelSkin("infected", pal, art, nil)
	requireSkinOK(t, err)
	c := testModelTextureClient()
	c.InstallPreparedModelSkin(good)
	requireSkinOK(t, c.SetOwnerModelSkin(1, "infected"))
	for _, tc := range []struct {
		name string
		pal  *palette.Tables
		art  image.Image
	}{
		{"", pal, art}, {"infected", nil, art}, {"infected", &palette.Tables{}, art}, {"infected", pal, nil},
		{"infected", pal, image.NewNRGBA(image.Rectangle{})}, {"infected", pal, image.NewNRGBA(image.Rect(0, 0, 1, 1))},
	} {
		failed, err := r.PrepareInfectedModelSkin(tc.name, tc.pal, tc.art, nil)
		if failed != nil || err == nil || !strings.Contains(err.Error(), "logical path skins/") || !strings.Contains(err.Error(), "providers searched [") || !strings.Contains(err.Error(), "expected ") {
			t.Fatalf("invalid preparation did not return shaped diagnostic: %v", err)
		}
		c.InstallPreparedModelSkin(failed)
		if c.selectedModelSkin(0, 1) != good {
			t.Fatal("failed preparation changed installed bank")
		}
	}
	source.SubframeCount = 1
	if bad, err := r.PrepareInfectedModelSkin("infected", pal, art, nil); bad != nil || err == nil || !strings.Contains(err.Error(), "mod frame 0") {
		t.Fatal("malformed composite did not fail transactionally with source context")
	}
	source.SubframeCount = 0
	var workers sync.WaitGroup
	for range 8 {
		workers.Go(func() {
			other, err := r.PrepareInfectedModelSkin("other", pal, art, nil)
			if err != nil || other.generated["mod"][source].Pixels[0] != good.generated["mod"][source].Pixels[0] {
				t.Error("independent concurrent preparation changed results")
			}
		})
	}
	workers.Wait()
}

func TestModelSkinInfectionPreparedDrawDoesNotAllocate(t *testing.T) {
	pal, art := infectionTestArt()
	source := &formats.GAFFrame{Width: 1, Height: 1, Pixels: []byte{160}}
	r, m := infectionTestRegistry(map[string]texRef{"mod": {kind: texStatic, frame: source}})
	skin, err := r.PrepareInfectedModelSkin("infected", pal, art, nil)
	requireSkinOK(t, err)
	c := testModelTextureClient()
	c.SetModelTextureRegistry(r)
	c.modelScratch.active = true
	draw := infectionDraw(m, "mod")
	if allocations := testing.AllocsPerRun(100, func() {
		c.modelScratch.reset()
		polys := c.collectDrawPolys(draw, teamColor{}, 1, modelCursorUnit, skin)
		if len(polys) != 1 || polys[0].frame != skin.generated["mod"][source] {
			t.Fatal("prepared draw lost replacement frame")
		}
	}); allocations != 0 {
		t.Fatalf("prepared draw allocated %g times", allocations)
	}
}
