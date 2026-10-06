package client

import (
	"bytes"
	"github.com/nanolathe-gg/nanolathe/internal/camera"
	"testing"

	"github.com/nanolathe-gg/nanolathe/formats"
	"github.com/nanolathe-gg/nanolathe/internal/drawlist"
	"github.com/nanolathe-gg/nanolathe/internal/frame"
	"github.com/nanolathe-gg/nanolathe/internal/palette"
	presentationrender "github.com/nanolathe-gg/nanolathe/internal/render"
	"github.com/nanolathe-gg/nanolathe/internal/sim/numeric"
)

func cachedLiveRegressionSubject(t *testing.T) (*Client, frame.UnitView) {
	t.Helper()
	c := newPieceFixtureClient(t)
	c.models["split"] = syntheticModel([]pieceInfo{{name: "base", parent: -1}, {name: "live", parent: 0}}, []syntheticTri{
		makeTriangle(0, "base", [3][3]float64{{0, 0, 0}, {16, 0, 0}, {0, 0, 16}}, 31, 0),
		makeTriangle(1, "live", [3][3]float64{{0, 0, 0}, {16, 0, 0}, {0, 0, 16}}, 99, 0),
	}, 0)
	v := frame.UnitView{InstanceID: 71, Slot: 1, Model: "split", X: numeric.Fixed(100 << 16), Z: numeric.Fixed(100 << 16), ZBuffer: true, CacheRevision: 1, CacheValidityRevision: 1,
		Pieces: []frame.PieceView{{Index: 0}, {Index: 1, DontCache: true}}}
	return c, v
}

func cachedTeamColorRegressionSubject(t *testing.T) (*Client, frame.UnitView, [10]*formats.GAFFrame) {
	t.Helper()
	c := newPieceFixtureClient(t)
	var authored [10]*formats.GAFFrame
	frames := make([]formats.GAFFrameRef, len(authored))
	for i := range frames {
		authored[i] = &formats.GAFFrame{Width: 1, Height: 1, Pixels: []byte{byte(100 + i)}}
		frames[i].Frame = authored[i]
	}
	c.texIndex["logo"] = texRef{kind: texTeam, key: "logo", entry: &formats.GAFEntry{FrameCount: uint16(len(frames)), Frames: frames}}
	c.models = map[string]*unitModel{"team": teamLogoTestModel()}
	v := frame.UnitView{
		InstanceID: 72, Slot: 1, Model: "team", X: numeric.Fixed(100 << 16), Z: numeric.Fixed(100 << 16),
		ZBuffer: true, CacheRevision: 1, CacheValidityRevision: 1, OwnerColorKnown: true,
	}
	return c, v, authored
}

func TestCachedBodyMemoizationTracksPublishedTeamColor(t *testing.T) {
	c, v, _ := cachedTeamColorRegressionSubject(t)
	cachedLiveReplay(t, c, v)
	first := c.cachedModelBodies[v.InstanceID]
	if first == nil || first.teamColor != (teamColor{index: 0, known: true}) || bytes.Count(c.indexed, []byte{100}) == 0 {
		t.Fatalf("initial team-colour body=%+v pixels=%d", first, bytes.Count(c.indexed, []byte{100}))
	}
	v.OwnerColor = 7
	cachedLiveReplay(t, c, v)
	second := c.cachedModelBodies[v.InstanceID]
	if second == first || second.teamColor != (teamColor{index: 7, known: true}) || bytes.Count(c.indexed, []byte{107}) == 0 {
		t.Fatalf("changed team-colour body=%+v pixels=%d", second, bytes.Count(c.indexed, []byte{107}))
	}

	native, nv, authored := cachedTeamColorRegressionSubject(t)
	native.geometryOnlyModels = true
	record := func() *cachedModelBody {
		native.list.Reset()
		native.modelScratch.reset()
		native.modelScratch.active = true
		defer func() { native.modelScratch.active = false }()
		if !native.drawUnitModel(nv, 0, 0) {
			t.Fatal("team-colour native model was not recorded")
		}
		return native.cachedModelBodies[nv.InstanceID]
	}
	body := record()
	if body == nil || body.geometry == nil || len(body.geometry.Faces) == 0 || body.geometry.Faces[0].Texture != authored[0] {
		t.Fatal("initial native body did not retain team frame zero")
	}
	nv.OwnerColor = 7
	body = record()
	if body == nil || body.teamColor != (teamColor{index: 7, known: true}) || body.geometry == nil || len(body.geometry.Faces) == 0 || body.geometry.Faces[0].Texture != authored[7] {
		t.Fatal("changed native body did not rebuild with team frame seven")
	}
}

func TestCachedLiveMotionExpandsRetainedBounds(t *testing.T) {
	c, v := cachedLiveRegressionSubject(t)
	cachedLiveReplay(t, c, v)
	before := bytes.Count(c.indexed, []byte{99})
	v.Pieces[1].Tx = numeric.Fixed(48 << 16)
	cachedLiveReplay(t, c, v)
	if after := bytes.Count(c.indexed, []byte{99}); before == 0 || after != before {
		t.Fatalf("translated live pixels=%d, want preserved %d", after, before)
	}
}

func TestGeometryOnlyCachedLaneRetainsBodyAndRecordsLiveFaces(t *testing.T) {
	c, v := cachedLiveRegressionSubject(t)
	c.antiAlias, c.pal = true, &palette.Tables{}
	v.Pieces[1].Tx = numeric.Fixed(48 << 16)
	c.geometryOnlyModels = true
	c.modelScratch.reset()
	c.modelScratch.active = true
	defer func() { c.modelScratch.active = false }()
	if !c.drawUnitModel(v, 0, 0) {
		t.Fatal("geometry-only unit was not recorded")
	}
	models := c.list.ModelCommands()
	if len(models) != 1 || models[0].Geometry == nil || len(models[0].Geometry.Faces) == 0 || len(models[0].Geometry.LiveFaces) == 0 {
		t.Fatalf("first native lanes = %#v, want cached and live geometry", models)
	}
	if got := len(c.cachedModelBodies[v.InstanceID].geometry.LiveFaces); got != 0 {
		t.Fatalf("retained cached geometry holds %d live faces", got)
	}
	if models[0].Geometry.Supersample == nil {
		t.Fatal("structure cached lane lost its doubled geometry")
	}
	native := models[0].Geometry.Faces[0].Vertices[0]
	doubled := models[0].Geometry.Supersample.Faces[0].Vertices[0]
	if doubled.X != 2*native.X || doubled.Y != 2*native.Y {
		t.Fatalf("doubled cached corner=%+v, native=%+v", doubled, native)
	}
	cachedVertex := models[0].Geometry.Faces[0].Vertices[0]
	cachedWidth, cachedHeight := models[0].Geometry.Width, models[0].Geometry.Height
	c.list.Reset()
	c.modelScratch.reset()
	v.Pieces[1].Tx = 0
	if !c.drawUnitModel(v, 0, 0) {
		t.Fatal("second geometry-only unit was not recorded")
	}
	second := c.list.ModelCommands()[0].Geometry
	if len(second.LiveFaces) == 0 || second.Faces[0].Vertices[0] != cachedVertex {
		t.Fatal("current live pose changed retained cached lane")
	}
	if second.Width < cachedWidth || second.Height < cachedHeight {
		t.Fatalf("contracted live pose cropped retained box: %dx%d, want at least %dx%d", second.Width, second.Height, cachedWidth, cachedHeight)
	}
}

func TestGeometryOnlyKeylessLiveIsLaterDirectModel(t *testing.T) {
	c, v := cachedLiveRegressionSubject(t)
	v.BMCode, v.ZBuffer = true, false
	c.geometryOnlyModels = true
	c.modelScratch.reset()
	c.modelScratch.active = true
	defer func() { c.modelScratch.active = false }()
	if !c.drawUnitModel(v, 0, 0) {
		t.Fatal("keyless geometry-only unit was not recorded")
	}
	models := c.list.ModelCommands()
	if len(models) != 2 || models[0].Geometry.KeyPlane || models[1].Geometry.KeyPlane || len(models[0].Geometry.LiveFaces) != 0 {
		t.Fatalf("keyless record = %#v, want cached body then direct live", models)
	}
	// A direct packet's local space is screen space: its anchor is screen
	// (0,0) and its box is the corners' own extent, not the framebuffer's.
	live := models[1].Geometry
	if live.AnchorX != 0 || live.AnchorY != 0 || live.Width <= 0 || live.Width >= int32(c.width) {
		t.Fatalf("later live command did not use direct framebuffer projection: anchor (%d,%d), box %dx%d", live.AnchorX, live.AnchorY, live.Width, live.Height)
	}
}

func TestCachedLiveChildPassWinsEqualCachedKeys(t *testing.T) {
	c, v := cachedLiveRegressionSubject(t)
	c.resetListForTest()
	c.modelScratch.reset()
	c.modelScratch.active = true
	defer func() { c.modelScratch.active = false }()
	m, ok := c.composeChildModel(v)
	if !ok || m.image == nil || bytes.Count(m.image.color, []byte{99}) == 0 {
		t.Fatal("child live face lost the later equal-key pass")
	}
	if bytes.Count(c.cachedModelBodies[v.InstanceID].image.color, []byte{99}) != 0 {
		t.Fatal("live face leaked into retained child body")
	}
}

func TestCachedLiveStructureLaneIsNeverSupersampled(t *testing.T) {
	c, v := cachedLiveRegressionSubject(t)
	c.antiAlias = true
	c.pal = &palette.Tables{}
	draw, ok := c.unitDrawFor(v)
	if !ok {
		t.Fatal("draw missing")
	}
	live, ok := c.composeModelLane(draw, v.Owner, unitTeamColor(v), v.InstanceID, modelCursorUnit, nil, 0, presentationrender.PieceLaneLive, false)
	if !ok || live.raster.scale != 1 {
		t.Fatal("live structure lane was supersampled")
	}
	cached, ok := c.composeModelLane(draw, v.Owner, unitTeamColor(v), v.InstanceID, modelCursorUnit, nil, 0, presentationrender.PieceLaneCached, false)
	if !ok || cached.raster.scale != 2 {
		t.Fatal("cached structure lane lost its authored anti-alias pass")
	}
}

func TestCachedLiveMissingImageCarrierUsesDirectRoute(t *testing.T) {
	c, v := cachedLiveRegressionSubject(t)
	v.BMCode = true
	v.ZBuffer = false
	cachedLiveReplay(t, c, v)
	v.CacheRevision++ // image discard without validity clear [03 R-COMP-01 §4]
	child := v
	child.InstanceID++
	child.X += numeric.Fixed(32 << 16)
	clearIndexed(c)
	c.resetListForTest()
	c.modelScratch.reset()
	c.modelScratch.active = true
	defer func() { c.modelScratch.active = false }()
	if !c.composeCarrier(v, 0, 0, []frame.UnitView{child}) {
		t.Fatal("image-less carrier did not use direct present")
	}
	c.replayForTest()
	if bytes.Count(c.indexed, []byte{99}) == 0 {
		t.Fatal("image-less carrier and child lost live geometry")
	}
}

func TestCachedLiveRevealDoesNotMutateRetainedBody(t *testing.T) {
	c, v := cachedLiveRegressionSubject(t)
	v.BuildRemaining = 1
	cachedLiveReplay(t, c, v)
	if bytes.Count(c.indexed, []byte{31}) != 0 || bytes.Count(c.indexed, []byte{99}) != 0 {
		t.Fatal("new nanoframe retained unrevealed face colors")
	}
	body := c.cachedModelBodies[v.InstanceID].image
	if bytes.Count(body.color, []byte{31}) == 0 {
		t.Fatal("reveal destroyed the retained raw composition")
	}
}

func TestCachedLiveCarrierDiggerEraseIncludesAttachedChild(t *testing.T) {
	c, v := cachedLiveRegressionSubject(t)
	v.Digger = true
	v.Y = numeric.Fixed(20 << 16)
	child := v
	child.InstanceID++
	child.Digger = false
	child.X += numeric.Fixed(40 << 16)
	// The child is outside the carrier body but below its final key-125 erase.
	// Its raw image survives until the carrier's group finalization.
	clearIndexed(c)
	c.resetListForTest()
	c.modelScratch.reset()
	c.modelScratch.active = true
	defer func() { c.modelScratch.active = false }()
	if !c.composeCarrier(v, 0, 0, []frame.UnitView{child}) {
		t.Fatal("carrier missing")
	}
	c.replayForTest()
	if bytes.Count(c.indexed, []byte{99}) != 0 {
		t.Fatal("attached child escaped the carrier's final Digger erase")
	}
}

func TestCachedLiveTransparentIndexStillErasesCachedColor(t *testing.T) {
	c, v := cachedLiveRegressionSubject(t)
	c.models["split"].compiled.Pieces[1].Primitives[0].ColorIndex = uint32(transparentModelIndex)
	cachedLiveReplay(t, c, v)
	if bytes.Count(c.indexed, []byte{31}) != 0 {
		t.Fatal("live key-colored face failed to erase cached pixels")
	}
}

// The live renderer is an unshaded second invocation even for a shaded
// BMcode=0 body. Give every SHD row a deliberately different result so the
// production lane cannot accidentally pass by choosing row zero.
func TestCachedLiveStructurePieceBypassesSHD(t *testing.T) {
	c, v := cachedLiveRegressionSubject(t)
	previousShading := presentationrender.Shading
	presentationrender.Shading = true
	t.Cleanup(func() { presentationrender.Shading = previousShading })
	c.pal = &palette.Tables{}
	c.antiAlias = false
	for row := range c.pal.Shade {
		c.pal.Shade[row][99] = 7
		c.pal.Shade[row][31] = 7
	}
	draw, ok := c.unitDrawFor(v)
	if !ok {
		t.Fatal("draw missing")
	}
	live, ok := c.composeModelLane(draw, v.Owner, unitTeamColor(v), v.InstanceID, modelCursorUnit, nil, 0, presentationrender.PieceLaneLive, false)
	if !ok || bytes.Count(live.image.color, []byte{99}) == 0 || bytes.Count(live.image.color, []byte{7}) != 0 {
		t.Fatal("structure live lane did not retain its raw authored colour")
	}
	cached, ok := c.composeModelLane(draw, v.Owner, unitTeamColor(v), v.InstanceID, modelCursorUnit, nil, 0, presentationrender.PieceLaneCached, false)
	if !ok || bytes.Count(cached.image.color, []byte{7}) == 0 {
		t.Fatalf("structure cached lane did not retain its shaded renderer: colors 31=%d 7=%d rows=%#v", bytes.Count(cached.image.color, []byte{31}), bytes.Count(cached.image.color, []byte{7}), draw.Pieces[0].Primitives[0].ShadeRows)
	}
}

func TestModelCommitAndChildCompositeKeyOnTransparentIndex(t *testing.T) {
	// A live raster write may mark coverage while placing the image background
	// over a cached colour. The final blit still keys on color 1, leaving the
	// earlier framebuffer pixel alone [03 R-REN-03A §1].
	final := newModelImage(1, 1, 0, 0, 0, 0, true, 1)
	final.color[0], final.covered[0] = transparentModelIndex, true
	framebuffer := []byte{73}
	final.commit(framebuffer, 1, 1)
	if framebuffer[0] != 73 {
		t.Fatalf("final transparent write painted %d, want prior framebuffer pixel 73", framebuffer[0])
	}

	// Attached-child composition has the same boundary: its image background
	// cannot erase the carrier before the final keyed blit [03 R-REN-03A §4].
	carrier := newModelImage(1, 1, 0, 0, 0, 0, true, 1)
	carrier.color[0], carrier.covered[0] = 55, true
	child := newModelImage(1, 1, 0, 0, 0, 0, true, 1)
	child.color[0], child.covered[0] = transparentModelIndex, true
	carrier.compositeChild(child, 0)
	framebuffer[0] = 73
	carrier.commit(framebuffer, 1, 1)
	if framebuffer[0] != 55 {
		t.Fatalf("transparent child replaced carrier colour with %d, want 55", framebuffer[0])
	}
}

func TestCachedBodyMemoizationTracksPresentationInputs(t *testing.T) {
	c, v := cachedLiveRegressionSubject(t)
	previousShading := presentationrender.Shading
	t.Cleanup(func() { presentationrender.Shading = previousShading })
	p1, p2 := &palette.Tables{}, &palette.Tables{}
	c.SetPalette(p1)
	c.SetAntiAlias(false)
	c.SetShadowOptions(false, false, true)
	cachedLiveReplay(t, c, v)
	first := c.cachedModelBodies[v.InstanceID]
	if first == nil || first.supersampled || first.scale != camera.ViewScaleNative || first.palette != p1 || !first.shaded {
		t.Fatalf("initial body memo key=%+v", first)
	}

	c.SetShadowOptions(false, false, false)
	cachedLiveReplay(t, c, v)
	shadingOff := c.cachedModelBodies[v.InstanceID]
	if shadingOff == first || shadingOff.shaded {
		t.Fatal("shading change reused the shaded cached body")
	}

	c.SetAntiAlias(true)
	cachedLiveReplay(t, c, v)
	aa := c.cachedModelBodies[v.InstanceID]
	if aa == shadingOff || !aa.supersampled {
		t.Fatal("effective anti-alias change reused the native cached body")
	}

	c.cam.Scale = camera.ViewScaleDetail
	cachedLiveReplay(t, c, v)
	zoom := c.cachedModelBodies[v.InstanceID]
	if zoom == aa || zoom.scale != camera.ViewScaleDetail {
		t.Fatal("effective camera scale change reused the prior cached body")
	}

	c.SetPalette(p2)
	cachedLiveReplay(t, c, v)
	paletteChanged := c.cachedModelBodies[v.InstanceID]
	if paletteChanged == zoom || paletteChanged.palette != p2 {
		t.Fatal("palette installation reused the prior cached body")
	}

	// Switching to the native recorder rebuilds retained geometry under the
	// new palette identity. It must not relabel this classic plane and let a
	// later classic frame reuse it.
	oldImage := paletteChanged.image
	c.SetPalette(p1)
	c.geometryOnlyModels = true
	c.modelScratch.reset()
	c.modelScratch.active = true
	if !c.drawUnitModel(v, 0, 0) {
		t.Fatal("geometry-only cached lane was not recorded")
	}
	c.modelScratch.active = false
	c.geometryOnlyModels = false
	if body := c.cachedModelBodies[v.InstanceID]; body.image != nil {
		t.Fatal("geometry replacement retained a classic image under new inputs")
	}
	cachedLiveReplay(t, c, v)
	if body := c.cachedModelBodies[v.InstanceID]; body.image == nil || body.image == oldImage || body.palette != p1 {
		t.Fatal("classic replay reused the pre-geometry cached image")
	}
}

// Rebuilding the image for a separate input must not consume sub-threshold
// orientation deltas: the reference describes the rasterized pose [03 §5.2].
func TestCachedBodyMemoRebuildPreservesOrientationReference(t *testing.T) {
	c, v := cachedLiveRegressionSubject(t)
	cachedLiveReplay(t, c, v)
	v.Heading = 7
	c.cam.Scale = camera.ViewScaleDetail
	cachedLiveReplay(t, c, v)
	if got := c.orientationCache(v.InstanceID).Heading; got != 0 {
		t.Fatalf("scale rebuild advanced retained heading to %d", got)
	}
	v.Heading = 14
	cachedLiveReplay(t, c, v)
	if got := c.orientationCache(v.InstanceID).Heading; got != 14 {
		t.Fatalf("accumulated turn did not refresh heading: %d", got)
	}
	draw, ok := c.unitDrawFor(v)
	if !ok || draw.PieceStates[0].RotY != 14 {
		t.Fatalf("live transform disagrees with refreshed cached pose")
	}
}

// A key-colored live face erases the body but must leave its ground shadow.
// Coverage inside staging is not opacity at the shadow punch [R-REN-03D §5].
func TestCachedLiveTransparentBodyPreservesGroundShadow(t *testing.T) {
	c, v := cachedLiveRegressionSubject(t)
	oldShading := presentationrender.Shading
	t.Cleanup(func() { presentationrender.Shading = oldShading })
	c.pal = &palette.Tables{}
	for i := range c.pal.Alpha {
		c.pal.Alpha[i] = 42
	}
	c.SetShadowOptions(true, true, false)
	cachedLiveReplay(t, c, v)
	before := append([]byte(nil), c.indexed...)
	c.models["split"].compiled.Pieces[1].Primitives[0].ColorIndex = uint32(transparentModelIndex)
	cachedLiveReplay(t, c, v)
	exposed := 0
	for i, color := range before {
		if color == 99 && c.indexed[i] == 42 {
			exposed++
		}
	}
	if exposed == 0 {
		t.Fatal("transparent live face incorrectly removed the shadow below the body")
	}
}

// A cache or shade script setter discards the composition image on every call
// and leaves the cached-body validity state alone [03 R-COMP-01 §4]
// [04 R-MOV-03 §4]. The retained native lane is that discarded reference, so a
// piece whose cache bit is restored has to return to the cached lane; keeping
// the stale lane draws it in neither lane and the body disappears behind the
// pieces that are still live, which is what a Kbot Lab shows after its door
// animation [03 R-REN-03A §4].
func TestGeometryOnlyImageDiscardRebuildsCachedLaneMembership(t *testing.T) {
	c, v := cachedLiveRegressionSubject(t)
	c.geometryOnlyModels = true
	c.modelScratch.reset()
	c.modelScratch.active = true
	defer func() { c.modelScratch.active = false }()
	// Both pieces live: the cached lane is empty, exactly as it is while a
	// script holds the whole model in its animated half.
	v.Pieces[0].DontCache = true
	if !c.drawUnitModel(v, 0, 0) {
		t.Fatal("all-live unit was not recorded")
	}
	if got := len(c.list.ModelCommands()[0].Geometry.Faces); got != 0 {
		t.Fatalf("all-live cached lane holds %d faces, want none", got)
	}
	// The script caches the body again: the flag write discards the image and
	// advances no validity state.
	c.list.Reset()
	c.modelScratch.reset()
	v.Pieces[0].DontCache = false
	v.CacheRevision++
	if !c.drawUnitModel(v, 0, 0) {
		t.Fatal("recached unit was not recorded")
	}
	g := c.list.ModelCommands()[0].Geometry
	if !modelLaneHasColor(g.Faces, 31) {
		t.Fatal("recached body piece returned to neither lane")
	}
	if modelLaneHasColor(g.LiveFaces, 31) {
		t.Fatal("recached body piece stayed in the live lane")
	}
	if !modelLaneHasColor(g.LiveFaces, 99) {
		t.Fatal("still-live piece left the live lane")
	}
}

// The Kbot Lab shape: the retained lane was built while the animated half was
// dont-cached, so it holds the base alone. Caching those pieces again empties
// the live lane, and without reading the discard the retained lane draws the
// base and nothing else [03 R-COMP-01 §4][03 R-REN-03A §4].
func TestGeometryOnlyRecachedAnimationReturnsToCachedLane(t *testing.T) {
	c, v := cachedLiveRegressionSubject(t)
	c.geometryOnlyModels = true
	c.modelScratch.reset()
	c.modelScratch.active = true
	defer func() { c.modelScratch.active = false }()
	if !c.drawUnitModel(v, 0, 0) {
		t.Fatal("animating unit was not recorded")
	}
	if got := c.list.ModelCommands()[0].Geometry; !modelLaneHasColor(got.Faces, 31) || !modelLaneHasColor(got.LiveFaces, 99) {
		t.Fatal("animating record did not split base and live lanes")
	}
	c.list.Reset()
	c.modelScratch.reset()
	v.Pieces[1].DontCache = false
	v.CacheRevision++
	if !c.drawUnitModel(v, 0, 0) {
		t.Fatal("recached unit was not recorded")
	}
	g := c.list.ModelCommands()[0].Geometry
	if len(g.LiveFaces) != 0 {
		t.Fatalf("recached piece left %d faces in the live lane", len(g.LiveFaces))
	}
	if !modelLaneHasColor(g.Faces, 99) || !modelLaneHasColor(g.Faces, 31) {
		t.Fatal("recached animated piece is drawn by neither lane")
	}
}

func modelLaneHasColor(faces []drawlist.ModelFace, color uint8) bool {
	for i := range faces {
		if faces[i].Color == color {
			return true
		}
	}
	return false
}

func TestModelImageInvalidationPreservesRetainedOrientation(t *testing.T) {
	c, v := cachedLiveRegressionSubject(t)
	cachedLiveReplay(t, c, v)
	before := *c.orientationCache(v.InstanceID)
	c.InvalidateModelImages()
	if c.cachedBody(v.InstanceID) != nil {
		t.Fatal("cached model survived invalidation")
	}
	v.Heading = 7
	cachedLiveReplay(t, c, v)
	if got := *c.orientationCache(v.InstanceID); got != before {
		t.Fatalf("invalidation advanced orientation: %v, want %v", got, before)
	}
}

// The final ZBuffer=0 image stays keyless, but anti-aliasing always rasterizes
// into the shared keyed scratch [03 R-REN-03A §6]. This also selects the keyed
// texture mapper rather than its raw keyless fallback.
func TestKeylessStructureAntialiasUsesKeyedScratch(t *testing.T) {
	c, v := cachedLiveRegressionSubject(t)
	c.pal = &palette.Tables{}
	draw, ok := c.unitDrawFor(v)
	if !ok {
		t.Fatal("draw missing")
	}
	draw.KeyPlane = false
	for _, aa := range []bool{false, true} {
		c.antiAlias = aa
		m, ok := c.composeModelLane(draw, v.Owner, unitTeamColor(v), v.InstanceID, modelCursorUnit, nil, 0, presentationrender.PieceLaneCached, false)
		if !ok || m.image.height != nil || (m.raster.height != nil) != aa || m.raster.framebuffer {
			t.Fatalf("aa=%v: final/scratch key-plane contract lost", aa)
		}
	}
	live, ok := c.composeDirectLiveModel(draw, unitTeamColor(v), v.InstanceID, modelCursorUnit, presentationrender.PieceLaneLive)
	if !ok || !live.raster.framebuffer || live.raster.height != nil {
		t.Fatal("direct live lane lost framebuffer mapper")
	}
}

func cachedStagingKeyOneSubject(t *testing.T) (*Client, frame.UnitView, frame.UnitView) {
	t.Helper()
	c := newPieceFixtureClient(t)
	c.models["seed-parent"] = syntheticModel([]pieceInfo{{name: "base", parent: -1}, {name: "live", parent: 0}}, []syntheticTri{
		makeTriangle(0, "base", [3][3]float64{{0, -50, 0}, {16, -50, 0}, {0, -50, 16}}, 31, 0),
		makeTriangle(1, "live", [3][3]float64{{0, -49, 0}, {16, -49, 0}, {0, -49, 16}}, 99, 0),
	}, 0)
	c.models["seed-child"] = syntheticModel([]pieceInfo{{name: "base", parent: -1}}, []syntheticTri{
		makeTriangle(0, "base", [3][3]float64{{0, -50, 0}, {32, -50, 0}, {0, -50, 32}}, 77, 0),
	}, 0)
	parent := frame.UnitView{InstanceID: 801, Slot: 1, Model: "seed-parent", X: numeric.FixedFromInt(100), Z: numeric.FixedFromInt(100), ZBuffer: true, BMCode: true, CacheRevision: 1, CacheValidityRevision: 1,
		Pieces: []frame.PieceView{{Index: 0}, {Index: 1, DontCache: true}}}
	child := frame.UnitView{InstanceID: 802, Slot: 2, Model: "seed-child", X: parent.X, Z: parent.Z, ZBuffer: true, BMCode: true, NoShadow: true, CacheRevision: 1, CacheValidityRevision: 1}
	return c, parent, child
}

// The parent live face has key1 and is above the child key0 at identical
// projected pixels. Enlarging for cargo must seed first, then draw that live
// face: filtering a completed parent a second time would let cargo win
// [03 R-REN-03A §4].
func TestCachedStagingSeedsBeforeLiveAndCargo(t *testing.T) {
	c, parent, child := cachedStagingKeyOneSubject(t)
	c.shadows, c.vehicleShadows = true, true
	c.pal = &palette.Tables{}
	cachedLiveReplay(t, c, parent)
	wantLive := bytes.Count(c.indexed, []byte{99})
	if wantLive == 0 {
		t.Fatal("authored live face was not drawn")
	}
	cached := c.cachedModelBodies[parent.InstanceID].image
	colors, keys := bytes.Clone(cached.color), bytes.Clone(cached.height)
	wantShadow := 0
	for _, cmd := range c.list.ModelCommands() {
		if cmd.Classic != nil && cmd.Classic.Shadow != nil {
			for _, covered := range cmd.Classic.Shadow.Coverage {
				if covered {
					wantShadow++
				}
			}
		}
	}
	if wantShadow == 0 {
		t.Fatal("authored mobile parent did not cast a shadow")
	}

	clearIndexed(c)
	c.resetListForTest()
	c.modelScratch.reset()
	c.modelScratch.active = true
	defer func() { c.modelScratch.active = false }()
	if !c.composeCarrier(parent, 0, 0, []frame.UnitView{child}) {
		t.Fatal("carrier was not recorded")
	}
	c.replayForTest()
	if got := bytes.Count(c.indexed, []byte{99}); got != wantLive {
		t.Fatalf("live pixels after cargo=%d, want unchanged%d", got, wantLive)
	}
	if bytes.Count(c.indexed, []byte{77}) == 0 {
		t.Fatal("enlarged cargo was not drawn")
	}
	if !bytes.Equal(cached.color, colors) || !bytes.Equal(cached.height, keys) {
		t.Fatal("frame staging changed immutable retained planes")
	}
	gotShadow := 0
	for _, cmd := range c.list.ModelCommands() {
		if cmd.Classic != nil && cmd.Classic.Shadow != nil {
			for _, covered := range cmd.Classic.Shadow.Coverage {
				if covered {
					gotShadow++
				}
			}
		}
	}
	if gotShadow != wantShadow {
		t.Fatalf("parent-only shadow pixels=%d, want%d", gotShadow, wantShadow)
	}
}

// The packet retains source dimensions while live motion and cargo enlarge the
// current union; neither changes the immutable cached lane's identity.
func TestGeometryCachedSeedRetainsSourceBeforeRebase(t *testing.T) {
	c, v := cachedLiveRegressionSubject(t)
	c.geometryOnlyModels = true
	record := func(children []frame.UnitView) *drawlist.ModelGeometry {
		c.resetListForTest()
		c.modelScratch.reset()
		c.modelScratch.active = true
		defer func() { c.modelScratch.active = false }()
		if len(children) == 0 {
			if !c.drawUnitModel(v, 0, 0) {
				t.Fatal("missing unit")
			}
		} else if !c.composeCarrier(v, 0, 0, children) {
			t.Fatal("missing carrier")
		}
		cmds := c.list.ModelCommands()
		return cmds[len(cmds)-1].Geometry.Clone()
	}
	first := record(nil)
	if first.CachedSeed.Width <= 0 || first.CachedSeed.Height <= 0 {
		t.Fatalf("cached packet omitted seed: key=%v cache=%+v size=%dx%d commands=%d body=%v", first.KeyPlane, first.Cache, first.Width, first.Height, len(c.list.ModelCommands()), c.cachedBody(v.InstanceID) != nil)
	}
	source := c.cachedBody(v.InstanceID).geometry
	want := drawlist.ModelCachedSeed{Width: source.Width, Height: source.Height, OriginX: source.OriginX, OriginY: source.OriginY}
	if first.CachedSeed != want {
		t.Fatalf("seed=%+v source=%+v", first.CachedSeed, want)
	}
	v.Pieces[1].Tx = numeric.Fixed(48 << 16)
	moved := record(nil)
	if moved.Width <= first.Width || moved.CachedSeed != want || moved.Cache != first.Cache {
		t.Fatalf("live rebase changed seed/cache: first=%+v moved=%+v", first.CachedSeed, moved.CachedSeed)
	}
	child := v
	child.InstanceID++
	child.Slot++
	child.X += numeric.Fixed(80 << 16)
	group := record([]frame.UnitView{child})
	if group.CachedSeed != want || group.Cache != first.Cache || len(group.Children) != 1 || group.Children[0].Geometry.CachedSeed.Width <= 0 {
		t.Fatal("cargo recording lost parent/child seed metadata")
	}
	if source.CachedSeed != (drawlist.ModelCachedSeed{}) || source.Width != want.Width || source.Height != want.Height {
		t.Fatal("frame seed mutated retained source")
	}
	v.InstanceID = 0
	v.Slot = 0
	if got := record(nil).CachedSeed; got != (drawlist.ModelCachedSeed{}) {
		t.Fatal("standalone adapter opted into cached seed")
	}
}
