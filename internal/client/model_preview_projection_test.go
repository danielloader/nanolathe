package client

import (
	"bytes"
	"math"
	"reflect"
	"testing"

	"github.com/nanolathe-gg/nanolathe/internal/drawlist"
	"github.com/nanolathe-gg/nanolathe/internal/frame"
	"github.com/nanolathe-gg/nanolathe/internal/palette"
	presentationrender "github.com/nanolathe-gg/nanolathe/internal/render"
	"github.com/nanolathe-gg/nanolathe/internal/sim/numeric"
)

func TestModelPreviewProjectionRoundsOnlyAtFinalScale(t *testing.T) {
	// These fractions all collapse at the ordinary model-pixel boundary.
	// Tool policy instead rounds the complete shear at each final resolution.
	for _, tt := range []struct {
		v, pivot [3]numeric.Fixed
		want     [4]int32
	}{
		{[3]numeric.Fixed{32768, 32768, -32768}, [3]numeric.Fixed{16384, 0, 0}, [4]int32{0, 0, 1, 1}},
		{[3]numeric.Fixed{-32768, 32768, 32768}, [3]numeric.Fixed{-16384, 0, 0}, [4]int32{-1, -3, -2, -5}},
	} {
		p := modelPreviewProjector{projection: ModelPreviewProjection{PixelsPerUnit: 3, Pivot: tt.pivot}}
		world := [3]numeric.Fixed{17*65536 + 3, 4*65536 + 7, -8*65536 - 11}
		v := tt.v
		for i := range v {
			v[i] += world[i]
		}
		x, y, x2, y2 := p.vertex(v, world)
		if got := [4]int32{x, y, x2, y2}; got != tt.want || p.err != nil {
			t.Fatalf("projected fractions = %v (%v), want %v", got, p.err, tt.want)
		}
	}
	p := modelPreviewProjector{projection: ModelPreviewProjection{PixelsPerUnit: math.MaxFloat64}}
	p.vertex([3]numeric.Fixed{65536, 0, 0}, [3]numeric.Fixed{})
	if p.err == nil {
		t.Fatal("unrepresentable coordinates were narrowed without an error")
	}
}

func TestModelPreviewProjectionPreservesSourceAttributes(t *testing.T) {
	c := testModelTextureClient()
	c.pal, c.antiAlias, c.recordModelGeometry = &palette.Tables{}, true, true
	c.effects = drawlist.AllEffects()
	draw := testPrimitiveDraw(presentationrender.PrimitiveDraw{
		TextureName: "tex", VertexIndices: []uint16{0, 1, 2, 3}, ShadeRow: 17, ShadeRows: []int{17, 18, 19, 20},
	}, [][3]numeric.Fixed{{32768, 32768, -32768}, {3 * 65536, 65536, 0}, {3 * 65536, 2 * 65536, 2 * 65536}, {0, 65536, 2 * 65536}})
	for _, keyPlane := range []bool{false, true} {
		draw.KeyPlane = keyPlane
		ordinary := c.prepareModelGeometry(draw, 0, teamColor{}, 0, modelCursorUnit, nil, 0)
		projected, err := c.projectedPreviewGeometry(draw, teamColor{}, ModelPreviewProjection{PixelsPerUnit: 7.25, Pivot: [3]numeric.Fixed{16384, 98304, -32768}}, 120, 90)
		if err != nil || ordinary == nil || projected == nil || projected.Supersample == nil {
			t.Fatalf("missing model geometry: %v", err)
		}
		if projected.KeyPlane != keyPlane || projected.AnchorX != 120 || projected.AnchorY != 90 || projected.Cache.Reusable() {
			t.Fatalf("projection changed depth class, pivot anchor or cache identity: %+v", projected)
		}
		for _, g := range []*drawlist.ModelGeometry{projected, projected.Supersample} {
			if len(g.Faces) != len(ordinary.Faces) {
				t.Fatal("projection changed material admission")
			}
			for i, face := range g.Faces {
				want := ordinary.Faces[i]
				if face.Texture != want.Texture || face.Shaded != want.Shaded || face.Normal != want.Normal || face.Material != want.Material || face.Glint != want.Glint {
					t.Fatal("projection changed source material or lighting")
				}
				for j, got := range face.Vertices {
					got.X, got.Y = want.Vertices[j].X, want.Vertices[j].Y
					if got != want.Vertices[j] {
						t.Fatalf("projection changed source vertex attributes: %+v, want %+v", got, want.Vertices[j])
					}
				}
			}
		}
	}
}

func projectedTestRenderer(t *testing.T) *ModelPreviewRenderer {
	t.Helper()
	c := newTestClient(t)
	p := &palette.Tables{}
	c.SetPalette(p)
	c.models["precision"] = syntheticModel([]pieceInfo{{name: "body", parent: -1}}, []syntheticTri{
		makeTriangle(0, "body", [3][3]float64{{-3.25, .5, -.5}, {4.25, 1.5, -.5}, {0, 2.5, 3.5}}, 77, 0),
	}, 0)
	return &ModelPreviewRenderer{client: c, palette: p}
}

func TestModelPreviewProjectedCallsLeaveOrdinaryOutputUnchanged(t *testing.T) {
	r := projectedTestRenderer(t)
	c := r.client
	opts := ModelPreviewOptions{Model: "precision", Width: 96, Height: 80, Heading: 8192, Structure: true, KeyPlane: true}
	classic, err := r.RecordModel(opts)
	if err != nil {
		t.Fatal(err)
	}
	geometry, err := r.RecordGeometry(opts)
	if err != nil {
		t.Fatal(err)
	}
	// A pre-existing ordinary orientation must neither quantize a tool angle
	// within its small-change threshold nor be replaced by the tool call.
	id := unitPresentationID(frame.UnitView{Slot: 1, InstanceID: 1})
	c.orientationCache(id).UpdateKey(c.models["precision"].compiled.Name, opts.Heading, opts.Pitch, opts.Bank)
	cache := *c.orientationCache(id)
	toolOpts := opts
	toolOpts.Heading++
	toolOpts.DisableAntiAlias = true
	tool, err := r.RecordProjectedGeometry(toolOpts, ModelPreviewProjection{PixelsPerUnit: 100})
	if err != nil || tool.Image != nil {
		t.Fatalf("tool geometry: %v", err)
	}
	if *c.orientationCache(id) != cache {
		t.Fatal("tool changed ordinary orientation cache")
	}
	// The same requested angle on a renderer without the prior cache must
	// produce the same packet, including one-unit camera-angle changes.
	saved := c.modelOrientation
	c.modelOrientation = nil
	fresh, err := r.RecordProjectedGeometry(toolOpts, ModelPreviewProjection{PixelsPerUnit: 100})
	c.modelOrientation = saved
	if err != nil || !reflect.DeepEqual(tool.List.ModelCommands(), fresh.List.ModelCommands()) {
		t.Fatal("tool projection inherited the ordinary small-angle threshold")
	}
	if _, err := r.RecordProjectedGeometry(ModelPreviewOptions{Model: "missing", Width: 96, Height: 80}, ModelPreviewProjection{PixelsPerUnit: 1}); err == nil {
		t.Fatal("missing model unexpectedly rendered")
	}
	again, err := r.RecordModel(opts)
	if err != nil || !bytes.Equal(classic.Image.Pix, again.Image.Pix) || !reflect.DeepEqual(classic.List.ModelCommands(), again.List.ModelCommands()) {
		t.Fatal("tool call changed later classic preview output")
	}
	again, err = r.RecordGeometry(opts)
	if err != nil || !reflect.DeepEqual(geometry.List.ModelCommands(), again.List.ModelCommands()) {
		t.Fatal("tool call changed later ordinary geometry")
	}
	if c.geometryOnlyModels || c.recordModelGeometry || c.enhanced || !c.antiAlias {
		t.Fatal("tool recording options leaked")
	}
}

func TestModelPreviewProjectionRejectsUnsupportedOptions(t *testing.T) {
	r := projectedTestRenderer(t)
	for _, scale := range []float64{0, -1, math.NaN(), math.Inf(1)} {
		if _, err := r.RecordProjectedGeometry(ModelPreviewOptions{Model: "precision", Width: 96, Height: 80}, ModelPreviewProjection{PixelsPerUnit: scale}); err == nil {
			t.Fatalf("scale %v unexpectedly accepted", scale)
		}
	}
	for _, opts := range []ModelPreviewOptions{
		{Scale: 1}, {Scale: 2}, {Children: []frame.UnitView{{}}}, {BuildRemaining: .5},
		{Cloaked: true}, {Digger: true}, {WorldHeight: 1}, {UnderwaterExempt: true},
	} {
		opts.Model, opts.Width, opts.Height = "precision", 96, 80
		if _, err := r.RecordProjectedGeometry(opts, ModelPreviewProjection{PixelsPerUnit: 1}); err == nil {
			t.Fatalf("unsupported options accepted: %+v", opts)
		}
	}
}
