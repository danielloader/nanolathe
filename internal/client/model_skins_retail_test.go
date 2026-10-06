package client

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"reflect"
	"sync"
	"testing"

	"github.com/nanolathe-gg/nanolathe/formats"
	"github.com/nanolathe-gg/nanolathe/internal/cob"
	"github.com/nanolathe-gg/nanolathe/internal/content"
	"github.com/nanolathe-gg/nanolathe/internal/drawlist"
	"github.com/nanolathe-gg/nanolathe/internal/frame"
	"github.com/nanolathe-gg/nanolathe/internal/sim/numeric"
)

// TestModelSkinRetailProbe is opt-in evidence for DESIGN_GPU_RENDERER §38.
// NANOLATHE_INFECTED_SKIN names a local indexed skin GAF; alternatively
// NANOLATHE_INFECTION_OVERLAY names original RGBA art to compose at load time.
// Retail assets and
// NANOLATHE_SHOT_DIR supply the reference models and optional capture directory.
// Local art is copied into a temporary skins/ mount, never the global textures/
// namespace. The probe uses one client and two instances of each shared model.
func TestModelSkinRetailProbe(t *testing.T) {
	source := os.Getenv("NANOLATHE_INFECTED_SKIN")
	overlayPath := os.Getenv("NANOLATHE_INFECTION_OVERLAY")
	if source == "" && overlayPath == "" {
		t.Skip("set NANOLATHE_INFECTED_SKIN or NANOLATHE_INFECTION_OVERLAY to local art")
	}
	var overlay image.Image
	if overlayPath != "" {
		f, err := os.Open(overlayPath)
		requireSkinOK(t, err)
		overlay, err = png.Decode(f)
		f.Close()
		requireSkinOK(t, err)
	}
	root := t.TempDir()
	if source != "" {
		data, err := os.ReadFile(source)
		requireSkinOK(t, err)
		requireSkinOK(t, os.Mkdir(filepath.Join(root, "skins"), 0700))
		requireSkinOK(t, os.WriteFile(filepath.Join(root, "skins", "infected.gaf"), data, 0600))
	}
	if dir := os.Getenv("NANOLATHE_SHOT_DIR"); dir != "" {
		requireSkinOK(t, os.MkdirAll(dir, 0755))
	}
	names := []string{"armpw", "armstump", "corgol"}
	if overlay != nil {
		names = append(names, "armham", "corak", "corraid", "armhawk", "corvamp", "armcrus", "corcrus")
	}
	loader, sourceFS := captureClient(t, 1, 1)
	t.Cleanup(loader.Close)
	defs, err := content.CompileUnits(sourceFS)
	requireSkinOK(t, err)
	for _, name := range names {
		t.Run(name, func(t *testing.T) {
			width, height, separation := 256, 128, 48
			if overlay != nil {
				width, height, separation = 512, 192, 120
			}
			c, fs := captureClient(t, width, height)
			t.Cleanup(c.Close)
			def := defs[name]
			if def == nil {
				t.Fatal("probe unit definition is absent")
			}
			requireSkinOK(t, fs.MountDirectory(root, 100))
			r := newModelTextureRegistry(fs, false)
			load := modelTextureLoadKey{kind: modelLoadUnit, id: name}
			m, err := r.bindLoad(load, def.ObjectName)
			requireSkinOK(t, err)
			r.unitByName[name] = load
			c.SetModelTextureRegistry(r)
			c.texRefs = &sync.Map{}
			var skin *ModelSkin
			if source != "" {
				skin, err = r.PrepareModelSkin("infected", "skins/infected.gaf")
				requireSkinOK(t, err)
			}
			if overlay != nil {
				skin, err = r.PrepareInfectedModelSkin("infected", c.pal, overlay, skin)
				requireSkinOK(t, err)
			}
			c.InstallPreparedModelSkin(skin)
			// Use the authored initialization pose, including hidden muzzle flash
			// pieces, through the existing isolated presentation VM.
			prog, _, err := cob.LoadFromFS(fs, name)
			requireSkinOK(t, err)
			if prog == nil {
				t.Fatal("stock model has no presentation script")
			}
			vm := cob.NewPresentationVM(prog, 4096)
			flags := make([]uint8, len(prog.Pieces))
			for i := range flags {
				flags[i] = 7
			}
			vm.BindRenderFlags(flags)
			if !vm.StartByName("Create", nil) {
				t.Fatal("stock Create script is absent")
			}
			for range 60 {
				vm.Drain(1)
			}
			pose := make([]frame.PieceView, len(m.compiled.Pieces))
			modelPieces := make([]string, len(m.compiled.Pieces))
			for pi, piece := range m.compiled.Pieces {
				modelPieces[pi] = piece.Name
				pose[pi].Index = pi
			}
			for si, pi := range cob.LinkPieces(prog.Pieces, modelPieces) {
				if pi < 0 {
					continue
				}
				state := vm.Pieces[si]
				pose[pi] = frame.PieceView{Index: pi, RotX: state.RotX, RotY: state.RotY, RotZ: state.RotZ,
					Tx: state.Trans[0], Ty: state.Trans[1], Tz: state.Trans[2],
					DontCache: flags[si]&2 == 0, DontShade: flags[si]&4 == 0, Hidden: flags[si]&1 == 0}
			}
			healthy := frame.UnitView{Slot: 1, InstanceID: 101, DefName: name, Model: def.ObjectName, Pieces: pose, OwnerColorKnown: true, OwnerColor: 2, Heading: 5000,
				X: numeric.Fixed(-separation << 16), BMCode: def.BMCode != 0, ZBuffer: def.ZBuffer, NoShadow: def.NoShadow,
				CanHover: def.CanHover, Floater: def.Floater, CacheRevision: 1, CacheValidityRevision: 1}
			subject := healthy
			subject.Slot = 2
			subject.InstanceID = 102
			subject.X = numeric.Fixed(separation << 16)
			if c.modelForUnit(healthy) != m || c.modelForUnit(subject) != m {
				t.Fatal("instances do not share loaded model")
			}
			players := len(r.players)

			renderPair := func(label string) []byte {
				t.Helper()
				c.geometryOnlyModels = false
				clear(c.indexed)
				c.resetListForTest()
				c.modelScratch.reset()
				c.modelScratch.active = true
				for _, v := range []frame.UnitView{healthy, subject} {
					sx, sy := c.cam.WorldToScreen(v.X, v.Y, v.Z)
					if !c.drawUnitModel(v, sx, sy) {
						t.Fatal("real model did not record")
					}
				}
				c.replayForTest()
				c.modelScratch.active = false
				writeCapture(t, c, name+"-"+label+"-native")
				writeModelSkinProbeZoom(t, c, name+"-"+label+"-4x")
				return append([]byte(nil), c.indexed...)
			}
			normal := renderPair("normal")
			requireSkinOK(t, c.SetUnitModelSkin(subject.InstanceID, "infected"))
			infected := renderPair("infected")
			changed := false
			for y := range c.height {
				row := y * c.width
				if !bytes.Equal(normal[row:row+c.width/2], infected[row:row+c.width/2]) {
					t.Fatal("healthy peer changed with infected subject")
				}
				changed = changed || !bytes.Equal(normal[row+c.width/2:row+c.width], infected[row+c.width/2:row+c.width])
			}
			if !changed {
				t.Fatal("infected subject pixels did not change")
			}
			requireSkinOK(t, c.SetUnitModelSkin(subject.InstanceID, ""))
			if cleared := renderPair("cleared"); !bytes.Equal(normal, cleared) {
				t.Fatal("cleared subject did not restore original pixels")
			}

			// Native packets feed the modern renderer directly. Assert their texture
			// pixels, topology and durable raster keys without introducing a second
			// rasterizer or requiring a graphics device for this material-only probe.
			c.geometryOnlyModels = true
			c.InvalidateModelImages()
			beforePeer := recordKeyedSubject(t, c, healthy).Clone()
			before := recordKeyedSubject(t, c, subject).Clone()
			teamPixels := map[*formats.GAFFrame][]byte{}
			for _, piece := range m.compiled.Pieces {
				for _, pr := range piece.Primitives {
					ref, ok := c.resolveModelTexture(pr.TextureName)
					if ok && ref.kind == texTeam {
						f := teamTextureFrame(ref, unitTeamColor(subject))
						if f != nil {
							teamPixels[f] = append([]byte(nil), f.Pixels...)
						}
					}
				}
			}
			requireSkinOK(t, c.SetUnitModelSkin(subject.InstanceID, "infected"))
			after := recordKeyedSubject(t, c, subject).Clone()
			afterPeer := recordKeyedSubject(t, c, healthy).Clone()
			if !reflect.DeepEqual(beforePeer.Faces, afterPeer.Faces) || !reflect.DeepEqual(beforePeer.LiveFaces, afterPeer.LiveFaces) || beforePeer.Cache != afterPeer.Cache {
				t.Fatal("healthy peer packet or raster key changed")
			}
			if before.Cache == after.Cache {
				t.Fatal("infected packet retained original raster key")
			}
			changedFaces, teamFaces := checkModelSkinProbeFaces(t, before.Faces, after.Faces, teamPixels, overlay != nil)
			changedLive, teamLive := checkModelSkinProbeFaces(t, before.LiveFaces, after.LiveFaces, teamPixels, overlay != nil)
			changedFaces, teamFaces = changedFaces+changedLive, teamFaces+teamLive
			if changedFaces == 0 {
				t.Fatal("native geometry did not select replacement pixels")
			}
			if teamFaces == 0 {
				t.Fatal("probe encountered no team-colour faces")
			}
			requireSkinOK(t, c.SetUnitModelSkin(subject.InstanceID, ""))
			cleared := recordKeyedSubject(t, c, subject).Clone()
			if !reflect.DeepEqual(before.Faces, cleared.Faces) || !reflect.DeepEqual(before.LiveFaces, cleared.LiveFaces) || cleared.Cache == after.Cache {
				t.Fatal("cleared geometry did not restore original frames with a new raster key")
			}
			if c.modelForUnit(subject) != m || c.modelForUnit(healthy) != m || len(r.players) != players {
				t.Fatal("skin selection changed model or animation binding identity")
			}
			t.Logf("%s: paired classic pixels restored; modern replaced %d faces and preserved %d team faces", name, changedFaces, teamFaces)
		})
	}
}

func checkModelSkinProbeFaces(t *testing.T, before, after []drawlist.ModelFace, teamPixels map[*formats.GAFFrame][]byte, allowFlat bool) (changed, team int) {
	t.Helper()
	if len(before) != len(after) {
		t.Fatal("skin changed face count")
	}
	for i, a := range before {
		b := after[i]
		if original, ok := teamPixels[a.Texture]; ok {
			team++
			if a.Texture != b.Texture || !bytes.Equal(original, b.Texture.Pixels) {
				t.Fatal("skin changed team pixels")
			}
		}
		if a.Texture != b.Texture && a.Texture != nil && b.Texture != nil && !bytes.Equal(a.Texture.Pixels, b.Texture.Pixels) {
			changed++
		}
		a.Texture = b.Texture
		if allowFlat && a.Texture == nil {
			a.Color = b.Color
		}
		if !reflect.DeepEqual(a, b) {
			t.Fatal("skin changed face geometry or material annotation")
		}
	}
	return changed, team
}

func TestModelSkinInfectionRetailFlatPalette(t *testing.T) {
	c, _ := captureClient(t, 8, 8)
	t.Cleanup(c.Close)
	_, art := infectionTestArt()
	skin, err := emptyModelTextureRegistry(nil, false).PrepareInfectedModelSkin("flat", c.pal, art, nil)
	requireSkinOK(t, err)
	for _, light := range []int{64, 128, 192} {
		source := nearestPaletteIndex(c.pal, light, light, light)
		mapped := skin.flat[source]
		before, after := c.pal.Base[source], c.pal.Base[mapped]
		if mapped == source || int(after[0])-int(after[1]) <= int(before[0])-int(before[1])+10 {
			t.Fatalf("neutral %d did not acquire a visible warm stain: %v -> %v", light, before, after)
		}
	}
}

// The enlarged capture is nearest-neighbour display of the native indexed
// replay, with no filtering or new model rasterization. Left stays healthy;
// right is the same publication identity before, during and after selection.
func writeModelSkinProbeZoom(t *testing.T, c *Client, name string) {
	t.Helper()
	dir := os.Getenv("NANOLATHE_SHOT_DIR")
	if dir == "" {
		return
	}
	const scale = 4
	img := image.NewRGBA(image.Rect(0, 0, c.width*scale, c.height*scale))
	for y := range img.Bounds().Dy() {
		for x := range img.Bounds().Dx() {
			r, g, b, a := c.pal.RGBA(c.indexed[(y/scale)*c.width+x/scale])
			img.SetRGBA(x, y, color.RGBA{R: r, G: g, B: b, A: a})
		}
	}
	f, err := os.Create(filepath.Join(dir, name+".png"))
	requireSkinOK(t, err)
	defer f.Close()
	requireSkinOK(t, png.Encode(f, img))
}
