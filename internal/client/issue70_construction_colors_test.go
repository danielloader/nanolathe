package client

import (
	"bytes"
	"testing"

	"github.com/nanolathe-gg/nanolathe/formats"
	"github.com/nanolathe-gg/nanolathe/internal/frame"
	presentationrender "github.com/nanolathe-gg/nanolathe/internal/render"
	"github.com/nanolathe-gg/nanolathe/internal/sim/numeric"
)

func constructionLampFixture(t *testing.T) (*Client, frame.UnitView, texRef) {
	t.Helper()
	c, v := cachedLiveRegressionSubject(t)
	v.Pieces[1].Tx = numeric.Fixed(24 << 16)
	entry := &formats.GAFEntry{Unknown1: 1, Frames: []formats.GAFFrameRef{
		{Value: 2, Frame: &formats.GAFFrame{Width: 1, Height: 1, Pixels: []byte{31}}},
		{Value: 3, Frame: &formats.GAFFrame{Width: 1, Height: 1, Pixels: []byte{99}}},
	}}
	ref := texRef{kind: texAnimated, key: "lamp", entry: entry, frame: entry.Frames[0].Frame}
	c.texIndex["lamp"] = ref
	m := c.models[v.Model]
	for i := range m.compiled.Pieces {
		pr := &m.compiled.Pieces[i].Primitives[0]
		pr.IsColored, pr.TextureName = 0, "lamp"
	}
	r := newModelTextureRegistry(nil, false)
	load := modelTextureLoadKey{kind: modelLoadUnit, id: v.Model}
	r.loads[load], r.byCompiled[m.compiled], r.primary = m, load, c.texIndex
	for i := range m.compiled.Pieces {
		key := modelTexturePrimitiveKey{load: load, piece: i, primitive: 0}
		r.bindings[key] = newModelTextureCursor(ref)
		r.players = append(r.players, r.bindings[key])
	}
	c.modelTextures = r
	return c, v, ref
}

// A progress-driven cache rebuild still selects frame zero; completion's live
// pass joins the already-running cursor [03 R-REN-03A §5]. This relationship
// is checked in both the indexed composer and the modern geometry producer.
func TestConstructionLampsSelectFirstFrameInBothModelPaths(t *testing.T) {
	for _, geometry := range []bool{false, true} {
		name := "classic"
		if geometry {
			name = "modern-recording"
		}
		t.Run(name, func(t *testing.T) {
			c, v, ref := constructionLampFixture(t)
			c.geometryOnlyModels = geometry
			v.BuildRemaining = .01
			var first []uint8
			if geometry {
				g := recordKeyedSubject(t, c, v)
				if len(g.Faces) != 2 || len(g.LiveFaces) != 0 {
					t.Fatalf("construction cached=%d live=%d, want both lamps in cached composition", len(g.Faces), len(g.LiveFaces))
				}
			} else {
				cachedLiveReplay(t, c, v)
				first = append([]uint8(nil), c.indexed...)
			}
			c.modelTextures.StepPhase7()
			c.modelTextures.StepPhase7()
			m := c.modelForUnit(v).compiled
			if got := c.modelTextures.animatedFrame(m, 1, 0, ref); got != ref.entry.Frames[1].Frame {
				t.Fatal("construction stopped the shared cursor or changed its strict countdown")
			}
			v.CacheValidityRevision++ // Same reveal: isolate selection on the next rebuild.
			if geometry {
				g := recordKeyedSubject(t, c, v)
				for _, face := range g.Faces {
					if face.Texture != ref.entry.Frames[0].Frame {
						t.Fatal("construction rebuild recorded the running lamp frame")
					}
				}
			} else {
				cachedLiveReplay(t, c, v)
				if !bytes.Equal(first, c.indexed) || bytes.Count(c.indexed, []byte{31}) == 0 {
					t.Fatal("construction rebuild animated the lamp or lost its first frame")
				}
			}
			v.BuildRemaining = 0
			if geometry {
				g := recordKeyedSubject(t, c, v)
				if len(g.Faces) != 1 || len(g.LiveFaces) != 1 || g.Faces[0].Texture != ref.entry.Frames[0].Frame || g.LiveFaces[0].Texture != ref.entry.Frames[1].Frame {
					t.Fatal("completed cached/live lanes did not select first/current frames")
				}
			} else {
				cachedLiveReplay(t, c, v)
				if bytes.Count(c.indexed, []byte{99}) == 0 || bytes.Count(c.indexed, []byte{31}) == 0 {
					t.Fatal("completion did not join the running live frame beside the cached first frame")
				}
			}
			if got := c.modelTextures.animatedFrame(m, 1, 0, ref); got != ref.entry.Frames[1].Frame {
				t.Fatal("completion or rendering reset the shared cursor")
			}
		})
	}
}

// The source selector belongs to the draw entry. A mobile live pass and the
// two standalone entries still read current playback [03 R-REN-03A §5]
// [03 R-COMP-02 §6], including while a mobile unit is unfinished.
func TestConstructionTextureSelectionPreservesDrawEntries(t *testing.T) {
	for _, tc := range []struct {
		name    string
		kind    uint8
		lane    presentationrender.PieceLane
		direct  bool
		first   bool
		preview bool
	}{
		{"cached-unit", modelCursorUnit, presentationrender.PieceLaneCached, false, true, false},
		{"unfinished-preview", modelCursorUnit, presentationrender.PieceLaneAll, false, true, true},
		{"live-mobile", modelCursorUnit, presentationrender.PieceLaneLive, false, false, false},
		{"direct-unit", modelCursorUnit, presentationrender.PieceLaneAll, true, false, false},
		{"feature-composition", modelCursorFeature, presentationrender.PieceLaneAll, false, true, false},
		{"debris", modelCursorDebris, presentationrender.PieceLaneAll, true, false, false},
		{"projectile", modelCursorProjectile, presentationrender.PieceLaneAll, false, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, v, ref := constructionLampFixture(t)
			v.BuildRemaining = .5
			v.BMCode = true
			if tc.preview {
				c.modelTextures = nil
			}
			draw, ok := c.unitDrawFor(v)
			if !ok {
				t.Fatal("fixture draw unavailable")
			}
			if tc.preview {
				c.collectDrawPolys(draw, teamColor{}, 1, tc.kind) // Bind the explicit preview cursors.
			}
			c.StepPhase7()
			c.StepPhase7()
			polys := c.collectDrawPolysLaneProjected(draw, teamColor{}, 1, tc.kind, tc.lane, tc.direct)
			if len(polys) == 0 {
				t.Fatal("fixture drew no material")
			}
			index := 1
			if tc.first {
				index = 0
			}
			for _, poly := range polys {
				if poly.frame != ref.entry.Frames[index].Frame {
					t.Fatalf("draw entry selected the wrong texture frame, want %d", index)
				}
			}
		})
	}
}

func TestConstructionKeepsTeamSelectorAndFlatColor(t *testing.T) {
	c, v, frames := cachedTeamColorRegressionSubject(t)
	v.BuildRemaining, v.OwnerColor = .5, 7
	draw, ok := c.unitDrawFor(v)
	if !ok {
		t.Fatal("fixture draw unavailable")
	}
	for _, lane := range []presentationrender.PieceLane{presentationrender.PieceLaneCached, presentationrender.PieceLaneAll, presentationrender.PieceLaneLive} {
		polys := c.collectDrawPolysLane(draw, unitTeamColor(v), 1, modelCursorUnit, lane)
		if len(polys) == 0 || polys[0].frame != frames[7] {
			t.Fatalf("lane %d replaced the construction owner selector", lane)
		}
	}
	draw.Pieces[0].Primitives[0].IsColored = 1
	draw.Pieces[0].Primitives[0].ColorIndex = 56
	if polys := c.collectDrawPolys(draw, unitTeamColor(v), 1, modelCursorUnit); len(polys) != 1 || polys[0].frame != nil || polys[0].color != 56 {
		t.Fatal("construction texture selection changed flat-color dispatch")
	}
	static := &formats.GAFFrame{Width: 1, Height: 1, Pixels: []byte{99}}
	c.texIndex["logo"] = texRef{kind: texStatic, key: "logo", frame: static}
	c.texGen++
	draw.Pieces[0].Primitives[0].IsColored = 0
	if polys := c.collectDrawPolys(draw, unitTeamColor(v), 1, modelCursorUnit); len(polys) != 1 || polys[0].frame != static {
		t.Fatal("construction texture selection changed single-frame dispatch")
	}
}

func TestConstructionFirstFrameDoesNotReviveEndedSequence(t *testing.T) {
	c, v, ref := constructionLampFixture(t)
	ref.entry.Unknown1 = 0
	c.modelTextures.players = nil
	load := modelTextureLoadKey{kind: modelLoadUnit, id: v.Model}
	for piece := 0; piece < 2; piece++ {
		key := modelTexturePrimitiveKey{load: load, piece: piece, primitive: 0}
		cursor := newModelTextureCursor(ref)
		c.modelTextures.bindings[key] = cursor
		c.modelTextures.players = append(c.modelTextures.players, cursor)
	}
	for tick := 0; tick < 5; tick++ {
		c.modelTextures.StepPhase7()
	}
	v.BuildRemaining = .5
	draw, ok := c.unitDrawFor(v)
	if !ok {
		t.Fatal("fixture draw unavailable")
	}
	for _, lane := range []presentationrender.PieceLane{presentationrender.PieceLaneCached, presentationrender.PieceLaneLive} {
		if got := c.collectDrawPolysLane(draw, teamColor{}, 1, modelCursorUnit, lane); len(got) != 0 {
			t.Fatalf("lane %d revived an inactive sequence", lane)
		}
	}
}
