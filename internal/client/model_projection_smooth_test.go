package client

import (
	"reflect"
	"testing"

	"github.com/nanolathe-gg/nanolathe/internal/camera"
	"github.com/nanolathe-gg/nanolathe/internal/drawlist"
	"github.com/nanolathe-gg/nanolathe/internal/palette"
	presentationrender "github.com/nanolathe-gg/nanolathe/internal/render"
	"github.com/nanolathe-gg/nanolathe/internal/sim/numeric"
)

// Enhanced presentation policy (DESIGN_GPU_RENDERER §13.5, §22): fractions
// survive the shear and the record scale, with each raster floored separately.
func TestEnhancedModelProjectionRetainsFractions(t *testing.T) {
	c := testModelTextureClient()
	c.enhanced, c.geometryOnlyModels, c.recordModelGeometry = true, true, true
	origin := fixedVertex(100, -20, 40)
	for _, tt := range []struct {
		name  string
		scale camera.ViewScale
		local [3]numeric.Fixed
		want  [4]int32
	}{
		{"native positive", camera.ViewScaleNative, [3]numeric.Fixed{3 << 14, 1 << 15, -(1 << 15)}, [4]int32{0, 0, 1, 0}},
		{"detail positive", camera.ViewScaleDetail, [3]numeric.Fixed{3 << 14, 1 << 15, -(1 << 15)}, [4]int32{1, 0, 3, 1}},
		{"native negative", camera.ViewScaleNative, [3]numeric.Fixed{-(1 << 14), -(1 << 15), 1 << 15}, [4]int32{-1, -1, -1, -1}},
		{"detail negative", camera.ViewScaleDetail, [3]numeric.Fixed{-(1 << 14), -(1 << 15), 1 << 15}, [4]int32{-1, -1, -1, -1}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			c.cam.Scale = tt.scale
			v := [3]numeric.Fixed{origin[0] + tt.local[0], origin[1] + tt.local[1], origin[2] + tt.local[2]}
			x, y, x2, y2 := c.smoothModelLocalVertex(v, origin)
			if got := [4]int32{x, y, x2, y2}; got != tt.want {
				t.Fatalf("projection = %v, want %v", got, tt.want)
			}
		})
	}
}

func smoothProjectionFixture() (*Client, *presentationrender.UnitDraw) {
	c := testModelTextureClient()
	c.enhanced, c.geometryOnlyModels, c.recordModelGeometry = true, true, true
	c.antiAlias, c.pal, c.effects = true, &palette.Tables{}, drawlist.AllEffects()
	draw := testPrimitiveDraw(presentationrender.PrimitiveDraw{IsColored: 1, ColorIndex: 7, VertexIndices: []uint16{0, 1, 2, 3}},
		[][3]numeric.Fixed{fixedVertex(-5, 1, 0), fixedVertex(8, 1, 0), fixedVertex(8, 1, -8), fixedVertex(-5, 1, -8)})
	draw.WorldPos = [3]numeric.Fixed{100<<16 + 1<<15, -2<<16 + 1<<15, 70<<16 + 1<<14}
	for i := range draw.Pieces[0].WorldVertices {
		v := &draw.Pieces[0].WorldVertices[i]
		v[0] += draw.WorldPos[0] + 3<<14
		v[1] += draw.WorldPos[1] + 1<<15
		v[2] += draw.WorldPos[2] + 1<<14
	}
	draw.KeyPlane, draw.CastsShadow = true, true
	return c, draw
}

func TestEnhancedModelPacketAlignsBoundsOutlineAndDirectLane(t *testing.T) {
	for _, scale := range []camera.ViewScale{camera.ViewScaleNative, camera.ViewScaleDetail} {
		for _, supersample := range []bool{false, true} {
			c, draw := smoothProjectionFixture()
			c.cam.Scale, c.effects.Supersample = scale, supersample
			reveal := presentationrender.BuildNanoframeReveal(.5, 20, 21)
			g := c.prepareModelGeometry(draw, 0, teamColor{}, 0, modelCursorUnit, &reveal, 21)
			direct := c.directUnitGeometry(draw, teamColor{}, 0, presentationrender.PieceLaneAll)
			if g == nil || direct == nil || len(g.Outline) != 1 {
				t.Fatal("missing model geometry")
			}
			for i, vertex := range g.Faces[0].Vertices {
				outline := g.Outline[0].Vertices[i]
				if outline.X != vertex.X || outline.Y != vertex.Y || outline.Key != vertex.Key {
					t.Fatal("outline differs from body projection")
				}
				if vertex.X < 0 || vertex.Y < 0 || vertex.X >= g.Width || vertex.Y >= g.Height {
					t.Fatal("native corner escaped extent")
				}
				dv := direct.Faces[0].Vertices[i]
				if vertex.X-g.OriginX+g.AnchorX != dv.X-direct.OriginX || vertex.Y-g.OriginY+g.AnchorY != dv.Y-direct.OriginY {
					t.Fatal("direct and local native placement differ")
				}
				if supersample {
					ss := g.Supersample
					sv, dsv := ss.Faces[0].Vertices[i], direct.Supersample.Faces[0].Vertices[i]
					if sv.X < 0 || sv.Y < 0 || sv.X >= ss.Width || sv.Y >= ss.Height {
						t.Fatal("doubled corner escaped extent")
					}
					if sv.X-ss.OriginX+2*g.AnchorX != dsv.X-direct.Supersample.OriginX || sv.Y-ss.OriginY+2*g.AnchorY != dsv.Y-direct.Supersample.OriginY {
						t.Fatal("direct and local doubled placement differ")
					}
					if sv.Key != vertex.Key {
						t.Fatal("doubled projection changed height key")
					}
				}
			}
			if g.Shadow == nil || !g.Shadow.Silhouette || len(g.Shadow.Faces) != 0 || g.Shadow.Width != g.Width || g.Shadow.Height != g.Height {
				t.Fatal("mobile shadow does not follow body silhouette")
			}
			// Only the geometry changes: the same keys drive submerged tint,
			// nanoframe bands and depth ordering on either projection path.
			c.enhanced = false
			classic := c.prepareModelGeometry(draw, 0, teamColor{}, 0, modelCursorUnit, &reveal, 21)
			if g.Waterline != classic.Waterline || g.WaterlineKey != classic.WaterlineKey || !reflect.DeepEqual(g.Reveal, classic.Reveal) {
				t.Fatal("late projection changed model verdicts")
			}
			for i, v := range g.Faces[0].Vertices {
				if v.Key != classic.Faces[0].Vertices[i].Key {
					t.Fatal("late projection changed depth key")
				}
			}
		}
	}
}

func TestEnhancedModelProjectionRequiresGeometryRecording(t *testing.T) {
	for _, flags := range [][3]bool{{false, true, true}, {true, false, true}, {true, true, false}, {false, false, false}} {
		c, draw := smoothProjectionFixture()
		c.enhanced, c.geometryOnlyModels, c.recordModelGeometry = flags[0], flags[1], flags[2]
		polys := c.collectDrawPolys(draw, teamColor{}, 0, modelCursorUnit)
		for i, v := range draw.Pieces[0].WorldVertices {
			x, y, h := modelLocalVertex(v, draw.WorldPos)
			if polys[0].x[i] != x || polys[0].y[i] != y || polys[0].oddHeight[i] != (h&1 != 0) {
				t.Fatalf("flags %v changed retail local projection", flags)
			}
		}
		if c.doubledPlacement(0, 0, 0, 0, false).exact {
			t.Fatalf("flags %v selected Enhanced doubled projection", flags)
		}
	}
}

func TestEnhancedRetainedBodyAndLiveFractionalMotion(t *testing.T) {
	c, v := cachedLiveRegressionSubject(t)
	c.enhanced, c.geometryOnlyModels, c.recordModelGeometry = true, true, true
	c.antiAlias, c.pal, c.effects = true, &palette.Tables{}, drawlist.AllEffects()
	v.Pieces[1].Tx = 1 << 14
	first := recordKeyedSubject(t, c, v).Clone()
	if len(first.Faces) == 0 || len(first.LiveFaces) == 0 || first.Supersample == nil || len(first.Supersample.LiveFaces) == 0 {
		t.Fatal("fixture did not record cached and live native/doubled lanes")
	}
	v.Pieces[1].Tx = 3 << 14
	second := recordKeyedSubject(t, c, v).Clone()
	if first.Cache != second.Cache || !reflect.DeepEqual(first.Faces, second.Faces) || !reflect.DeepEqual(first.Supersample.Faces, second.Supersample.Faces) {
		t.Fatal("live motion changed retained cached lane")
	}
	if !reflect.DeepEqual(first.LiveFaces, second.LiveFaces) {
		t.Fatal("half-pixel motion changed native corners")
	}
	for i, face := range first.Supersample.LiveFaces {
		for j, a := range face.Vertices {
			b := second.Supersample.LiveFaces[i].Vertices[j]
			if b.X != a.X+1 || b.Y != a.Y || b.Key != a.Key {
				t.Fatal("fractional live motion did not advance one doubled pixel")
			}
		}
	}
	v.X += 1 << 15
	third := recordKeyedSubject(t, c, v)
	if third.Cache.Revision != second.Cache.Revision {
		t.Fatal("placement rebuilt retained local geometry")
	}
	for i, a := range second.Supersample.Faces[0].Vertices {
		b := third.Supersample.Faces[0].Vertices[i]
		if b.X-third.Supersample.OriginX+2*third.AnchorX != a.X-second.Supersample.OriginX+2*second.AnchorX+1 {
			t.Fatal("retained rebase lost half-pixel placement")
		}
	}
}
