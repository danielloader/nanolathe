package client

import (
	"github.com/nanolathe-gg/nanolathe/internal/camera"
	"testing"

	"github.com/nanolathe-gg/nanolathe/internal/drawlist"
	"github.com/nanolathe-gg/nanolathe/internal/frame"
	"github.com/nanolathe-gg/nanolathe/internal/palette"
	presentationrender "github.com/nanolathe-gg/nanolathe/internal/render"
	"github.com/nanolathe-gg/nanolathe/internal/sim/numeric"
)

func TestModelGeometryPacketPreservesFaceOrderAndArity(t *testing.T) {
	first := newScreenPoly(3)
	first.x = []int32{2, 3, 4}
	first.y = []int32{5, 6, 7}
	first.attr[spanKey] = []int32{-10, 20, 300}
	first.attr[spanU] = []int32{0, 4, 8}
	first.attr[spanV] = []int32{1, 5, 9}
	first.attr[spanRow] = []int32{2, 3, 4}
	second := newScreenPoly(4)
	second.x = []int32{10, 11, 12, 13}
	second.y = []int32{14, 15, 16, 17}
	packet := modelGeometryPacketAt([]screenPoly{first, second}, 30, 31, 7, 8, 20, 21, 1, true, drawlist.ModelFallbackNone)
	if !packet.Eligible || packet.Fallback != drawlist.ModelFallbackNone {
		t.Fatalf("packet eligibility = %v/%v, want eligible/no fallback", packet.Eligible, packet.Fallback)
	}
	if got := len(packet.Faces); got != 2 {
		t.Fatalf("face count = %d, want 2", got)
	}
	if got := len(packet.Faces[0].Vertices); got != 3 {
		t.Fatalf("first face arity = %d, want 3", got)
	}
	if got := len(packet.Faces[1].Vertices); got != 4 {
		t.Fatalf("second face arity = %d, want 4", got)
	}
	vertex := packet.Faces[0].Vertices[2]
	if vertex.X != 4 || vertex.Y != 7 || vertex.Key != 300 || vertex.U != 8 || vertex.V != 9 || vertex.Shade != 4 {
		t.Fatalf("third first-face vertex = %+v, want original projected lanes", vertex)
	}
	first.x[0] = 99
	if got := packet.Faces[0].Vertices[0].X; got != 2 {
		t.Fatalf("packet aliases composition polygon: X=%d, want 2", got)
	}
}

func TestModelGeometryTraceOnlyOmitsBody(t *testing.T) {
	pending := pendingModelCommit{m: composedModel{geometry: &drawlist.ModelGeometry{Eligible: true}}}
	packet := geometryForCommit(pending)
	if packet.Eligible || packet.Fallback != drawlist.ModelFallbackNoBodyCommit {
		t.Fatalf("trace/shadow-only packet = eligible=%v fallback=%v, want false/no-body", packet.Eligible, packet.Fallback)
	}
}

func TestGeometryOnlyModelRecordsStructureResolveWithoutCPUCommit(t *testing.T) {
	c := testModelTextureClient()
	c.geometryOnlyModels = true
	c.antiAlias = true
	c.pal = &palette.Tables{}
	// The doubled lane also needs the player's supersampling on (§17.5).
	c.effects = drawlist.AllEffects()
	draw := testPrimitiveDraw(presentationrender.PrimitiveDraw{
		IsColored:     1,
		ColorIndex:    7,
		VertexIndices: []uint16{0, 1, 2, 3},
	}, [][3]numeric.Fixed{
		fixedVertex(0, 1, 0), fixedVertex(8, 1, 0), fixedVertex(8, 1, -8), fixedVertex(0, 1, -8),
	})
	draw.Structure, draw.KeyPlane, draw.CastsShadow = true, true, true
	if !c.drawModel(draw, 0, teamColor{}, 1, modelCursorUnit, nil, 0) {
		t.Fatal("geometry-only structure was not recorded")
	}
	models := c.list.ModelCommands()
	for _, model := range models {
		if model.Classic != nil {
			t.Fatal("geometry-only recording retained CPU image planes")
		}
	}
	if len(models) != 1 || models[0].Geometry == nil || !models[0].Geometry.Eligible || models[0].Geometry.Scale != 1 {
		t.Fatalf("geometry-only record = %#v, want eligible native geometry", models)
	}
	g := models[0].Geometry
	if g.Supersample == nil || g.Supersample.Scale != 2 {
		t.Fatal("structure did not record GPU resolve geometry")
	}
	native, doubled := g.Faces[0].Vertices[0], g.Supersample.Faces[0].Vertices[0]
	if doubled.X != 2*native.X || doubled.Y != 2*native.Y-1 || doubled.Key != native.Key {
		t.Fatalf("doubled odd-height corner=%+v, native=%+v", doubled, native)
	}
	if g.Shadow == nil || !g.Shadow.Eligible || len(g.Shadow.Faces) == 0 || models[0].ShadowOmissions != 0 {
		t.Fatal("eligible shadow was not recorded as geometry")
	}
	clone := g.Clone()
	clone.Shadow.Faces[0].Vertices[0].X++
	if clone.Shadow.Faces[0].Vertices[0].X == g.Shadow.Faces[0].Vertices[0].X {
		t.Fatal("cloned shadow aliases source vertices")
	}
}

// The player's supersampling off (GPU design §17.5, a Nanolathe presentation
// choice) records the subject as the Anti-Alias option off does: no doubled
// lane for the body or its shadow, retail's anchor with no half-pixel offset,
// and a cache identity that no longer claims the doubled lane, so the retained
// lane rebuilds when the switch moves.
func TestSupersampleOffRecordsNoDoubledLane(t *testing.T) {
	c := testModelTextureClient()
	c.geometryOnlyModels = true
	c.antiAlias = true
	c.pal = &palette.Tables{}
	c.effects = drawlist.AllEffects()
	c.effects.Supersample = false
	draw := testPrimitiveDraw(presentationrender.PrimitiveDraw{
		IsColored:     1,
		ColorIndex:    7,
		VertexIndices: []uint16{0, 1, 2, 3},
	}, [][3]numeric.Fixed{
		fixedVertex(0, 1, 0), fixedVertex(8, 1, 0), fixedVertex(8, 1, -8), fixedVertex(0, 1, -8),
	})
	draw.Structure, draw.KeyPlane, draw.CastsShadow = true, true, true
	if c.cachedBodyInputs(draw).geometrySupersampled || c.cachedShadowInputs(draw).doubled {
		t.Fatal("supersampling off still claims the doubled lane in the cache identity")
	}
	if !c.drawModel(draw, 0, teamColor{}, 1, modelCursorUnit, nil, 0) {
		t.Fatal("geometry-only structure was not recorded")
	}
	models := c.list.ModelCommands()
	if len(models) != 1 || models[0].Geometry == nil || !models[0].Geometry.Eligible || len(models[0].Geometry.Faces) == 0 {
		t.Fatalf("supersampling off record = %#v, want eligible native geometry", models)
	}
	g := models[0].Geometry
	if g.Supersample != nil || g.Shadow == nil || g.Shadow.Supersample != nil {
		t.Fatal("supersampling off recorded a doubled lane")
	}
	ax, ay, hx, hy := c.modelPlacement(draw)
	rx, ry := c.modelAnchor(draw)
	if ax != rx || ay != ry || hx != 0 || hy != 0 {
		t.Fatalf("supersampling off placed the body at %d,%d+%d,%d, want retail's %d,%d", ax, ay, hx, hy, rx, ry)
	}
}

func TestGeometryOnlyDiggerRecordsClipping(t *testing.T) {
	c := testModelTextureClient()
	c.geometryOnlyModels = true
	draw := testPrimitiveDraw(presentationrender.PrimitiveDraw{
		IsColored:     1,
		ColorIndex:    7,
		VertexIndices: []uint16{0, 1, 2, 3},
	}, [][3]numeric.Fixed{
		fixedVertex(0, 0, 0), fixedVertex(8, 0, 0), fixedVertex(8, 0, -8), fixedVertex(0, 0, -8),
	})
	draw.DiggerClip, draw.KeyPlane = true, true
	if !c.drawModel(draw, 0, teamColor{}, 1, modelCursorUnit, nil, 0) {
		t.Fatal("valid omitted geometry did not retain selection-chrome eligibility")
	}
	models := c.list.ModelCommands()
	for _, model := range models {
		if model.Classic != nil {
			t.Fatal("geometry-only recording retained CPU image planes")
		}
	}
	if len(models) != 1 || models[0].Geometry == nil || !models[0].Geometry.Eligible || !models[0].Geometry.Digger || models[0].Geometry.DiggerKey != uint8(diggerEraseThreshold) {
		t.Fatalf("geometry-only omission = %#v, want eligible digger clipping", models)
	}
}

// TestDirectDebrisProjectionGateAndClassicScale locks the detached model path:
// projected coordinates use the direct combined-world truncation, origin
// admission is inclusive, and an Original-2x replay does not scale an already
// framebuffer-positioned image again [03 R-COMP-02 §6][03 R-RAST-01 §2].
func TestDirectDebrisProjectionGateAndClassicScale(t *testing.T) {
	c := testModelTextureClient()
	c.width, c.height = 64, 64
	c.indexed = make([]byte, c.width*c.height)
	c.cam.Scale = camera.ViewScaleDetail
	draw := testPrimitiveDraw(presentationrender.PrimitiveDraw{
		IsColored: 1, ColorIndex: 77, VertexIndices: []uint16{0, 1, 2},
	}, [][3]numeric.Fixed{
		fixedVertex(10, 0, 10), fixedVertex(20, 0, 10), fixedVertex(10, 0, 0),
	})
	draw.WorldPos = [3]numeric.Fixed{numeric.FixedFromInt(10).Add(1), 0, numeric.FixedFromInt(10)}
	geometry := c.directDebrisGeometry(draw, teamColor{}, 3)
	if geometry == nil || len(geometry.Faces) != 1 {
		t.Fatal("in-viewport direct debris did not record geometry")
	}
	wantX, wantY := c.modelDirectVertex(draw.Pieces[0].WorldVertices[0], draw.WorldPos)
	got := geometry.Faces[0].Vertices[0]
	// The packet's local space is screen space offset by its own box origin,
	// with the anchor at screen (0,0).
	if geometry.AnchorX != 0 || geometry.AnchorY != 0 || got.X-geometry.OriginX != wantX || got.Y-geometry.OriginY != wantY {
		t.Fatalf("direct debris vertex = (%d,%d) at origin (%d,%d), want direct projection (%d,%d)", got.X, got.Y, geometry.OriginX, geometry.OriginY, wantX, wantY)
	}
	draw.WorldPos[0] = numeric.FixedFromInt(32) // projects to the inclusive right edge at scale 2
	if g := c.directDebrisGeometry(draw, teamColor{}, 3); g == nil {
		t.Fatal("right-edge debris origin was rejected")
	}

	// Geometry overlap is irrelevant once the detached call's own origin has
	// left the inclusive viewport.
	draw.WorldPos[0] = numeric.FixedFromInt(65)
	if g := c.directDebrisGeometry(draw, teamColor{}, 3); g != nil {
		t.Fatal("off-origin debris recorded overlapping geometry")
	}
	draw.WorldPos[0] = numeric.FixedFromInt(10).Add(1)

	classic, ok := c.composeDirectDebrisModel(draw, teamColor{}, 3)
	if !ok || !classic.raster.framebuffer || classic.raster.height != nil {
		t.Fatal("classic direct debris lost its keyless framebuffer mapper")
	}
	c.finishModel(classic, nil)
	models := c.list.ModelCommands()
	if len(models) != 1 || models[0].Classic == nil || models[0].Classic.Body == nil || !models[0].Classic.Body.Blit.Native() {
		t.Fatalf("direct classic packet = %#v, want replay scale one", models)
	}
	c.list.Replay(c.classicSink())
	if c.indexed[22*c.width+22] != 77 {
		t.Fatalf("direct 2x replay pixel = %d, want native-screen debris colour", c.indexed[22*c.width+22])
	}
	if c.indexed[44*c.width+44] == 77 {
		t.Fatal("direct 2x replay scaled framebuffer coordinates twice")
	}
}

// A wrapped zero must retain its erase/tint mode in the device packet, rather
// than becoming an absent pass [03 R-REN-03A §8].
func TestModelGeometryRecordsActiveZeroWaterline(t *testing.T) {
	for _, tc := range []struct {
		name        string
		owner, kind uint8
		want        drawlist.ModelWaterline
	}{
		{"own body", 0, modelCursorUnit, drawlist.ModelWaterlineBlue},
		{"unseen enemy body", 1, modelCursorUnit, drawlist.ModelWaterlineErase},
		{"feature", 1, modelCursorFeature, drawlist.ModelWaterlineBlue},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := testModelTextureClient()
			c.buffer = frame.NewBuffer()
			publishSeaLevel(t, c, 1, 206, 0)
			c.geometryOnlyModels = true
			draw := testPrimitiveDraw(presentationrender.PrimitiveDraw{
				IsColored: 1, ColorIndex: 40, VertexIndices: []uint16{0, 1, 2, 3},
			}, [][3]numeric.Fixed{fixedVertex(0, 0, 0), fixedVertex(8, 0, 0), fixedVertex(8, 0, -8), fixedVertex(0, 0, -8)})
			draw.KeyPlane = true
			if !c.drawModel(draw, tc.owner, teamColor{}, 1, tc.kind, nil, 0) {
				t.Fatal("model was not recorded")
			}
			models := c.list.ModelCommands()
			if len(models) != 1 || models[0].Geometry == nil {
				t.Fatal("geometry packet absent")
			}
			g := models[0].Geometry
			if g.Waterline != tc.want || g.WaterlineKey != 0 {
				t.Fatalf("waterline mode/key=%v/%d, want %v/0", g.Waterline, g.WaterlineKey, tc.want)
			}
		})
	}
}
