package client

import (
	"bytes"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/nanolathe-gg/nanolathe/formats"
	"github.com/nanolathe-gg/nanolathe/internal/drawlist"
	"github.com/nanolathe-gg/nanolathe/internal/frame"
	presentationrender "github.com/nanolathe-gg/nanolathe/internal/render"
	"github.com/nanolathe-gg/nanolathe/internal/sim/numeric"
	"github.com/nanolathe-gg/nanolathe/vfs"
)

func skinEntry(name string, pixels ...byte) formats.GAFWriteEntry {
	e := formats.GAFWriteEntry{Name: name}
	for _, p := range pixels {
		e.Frames = append(e.Frames, formats.GAFWriteFrame{Width: 1, Height: 1, Pixels: []byte{p}})
	}
	return e
}

func skinTestFS(t *testing.T, entries ...formats.GAFWriteEntry) (*vfs.FS, string) {
	t.Helper()
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "skins"), 0700); err != nil {
		t.Fatal(err)
	}
	filename := filepath.Join(root, "skins", "infected.gaf")
	writeSkinTestBank(t, filename, entries...)
	fs := vfs.New()
	if err := fs.MountDirectory(root, 1); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { fs.Close() })
	return fs, filename
}
func writeSkinTestBank(t *testing.T, filename string, entries ...formats.GAFWriteEntry) {
	t.Helper()
	data, err := formats.EncodeGAF(entries)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filename, data, 0600); err != nil {
		t.Fatal(err)
	}
}
func requireSkinOK(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

// One immutable model is shared by normal and selected instances. Both retained
// products must track selection and bank reload independently of pose validity.
func TestModelSkinDynamicCachedProducts(t *testing.T) {
	for _, geometry := range []bool{false, true} {
		name := "classic"
		if geometry {
			name = "geometry"
		}
		t.Run(name, func(t *testing.T) {
			c, v, _ := cachedTeamColorRegressionSubject(t)
			fs, filename := skinTestFS(t, skinEntry("logo", 42))
			c.modelFS = fs
			original := &formats.GAFFrame{Width: 1, Height: 1, Pixels: []byte{17}}
			c.texIndex["logo"] = texRef{kind: texStatic, frame: original}
			c.texRefs = &sync.Map{}
			c.geometryOnlyModels = geometry
			v.Owner = 1
			second := v
			second.InstanceID++
			second.Slot++
			second.Owner = 2
			model := c.models[v.Model]
			var oldSkinFrame *formats.GAFFrame
			keys := map[uint64]drawlist.ModelCacheKey{}
			colors := map[uint64]byte{}
			check := func(unit frame.UnitView, want byte) {
				t.Helper()
				if geometry {
					g := recordKeyedSubject(t, c, unit)
					if previous, ok := colors[unit.InstanceID]; ok && previous != want && g.Cache == keys[unit.InstanceID] {
						t.Fatal("changed pixels reused the executor raster key")
					}
					colors[unit.InstanceID], keys[unit.InstanceID] = want, g.Cache
					if len(g.Faces) == 0 || g.Faces[0].Texture == nil || g.Faces[0].Texture.Pixels[0] != want {
						t.Fatalf("geometry texture does not select %d", want)
					}
					if want == 42 {
						oldSkinFrame = g.Faces[0].Texture
					}
				} else {
					cachedLiveReplay(t, c, unit)
					if bytes.Count(c.indexed, []byte{want}) == 0 {
						t.Fatalf("classic pixels do not select %d", want)
					}
				}
				if c.models[v.Model] != model || c.texIndex["logo"].frame != original {
					t.Fatal("skin changed shared model or base texture")
				}
			}
			check(v, 17) // warm base refs and retained body before selection
			check(second, 17)
			requireSkinOK(t, c.LoadModelSkin("infected", "skins/infected.gaf"))
			requireSkinOK(t, c.SetOwnerModelSkin(1, "infected"))
			check(v, 42)
			check(second, 17)
			check(v, 42)
			v.Owner = 2
			check(v, 17) // ownership change without model/cache revision change
			requireSkinOK(t, c.SetUnitModelSkin(v.InstanceID, "infected"))
			check(v, 42)
			check(second, 17)
			requireSkinOK(t, c.SetUnitModelSkin(v.InstanceID, ""))
			check(v, 17)
			v.Owner = 1
			check(v, 42)
			// Reload keeps mappings but replaces bank/frame identity. Old packets keep
			// their immutable pixels while subsequent draws use the new bank.
			writeSkinTestBank(t, filename, skinEntry("logo", 61))
			requireSkinOK(t, c.LoadModelSkin("infected", "skins/infected.gaf"))
			check(v, 61)
			if oldSkinFrame != nil && oldSkinFrame.Pixels[0] != 42 {
				t.Fatal("reload mutated a recorded texture")
			}
			if c.LoadModelSkin("infected", "skins/missing.gaf") == nil {
				t.Fatal("missing bank accepted")
			}
			check(v, 61)
			c.ClearModelSkins()
			check(v, 17)
			check(second, 17)
			if c.SetOwnerModelSkin(1, "infected") == nil {
				t.Fatal("clear retained installed name")
			}
		})
	}
}

func TestModelSkinValidationAndLifecycle(t *testing.T) {
	for _, tc := range []struct {
		name    string
		entries []formats.GAFWriteEntry
		kind    texKind
		bad     bool
	}{
		{"static", []formats.GAFWriteEntry{skinEntry("tex", 42)}, texStatic, false},
		{"animated bank", []formats.GAFWriteEntry{skinEntry("tex", 42, 43)}, texStatic, true},
		{"duplicate", []formats.GAFWriteEntry{skinEntry("tex", 42), skinEntry("TEX", 43)}, texStatic, true},
		{"team override", []formats.GAFWriteEntry{skinEntry("tex", 42)}, texTeam, true},
		{"animated override", []formats.GAFWriteEntry{skinEntry("tex", 42)}, texAnimated, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fs, _ := skinTestFS(t, tc.entries...)
			registry := newModelTextureRegistry(fs, false)
			registry.primary["tex"] = texRef{kind: tc.kind}
			skin, err := registry.PrepareModelSkin("infected", "skins/infected.gaf")
			if (err != nil) != tc.bad {
				t.Fatalf("PrepareModelSkin error=%v", err)
			}
			if tc.bad {
				return
			}
			c := testModelTextureClient()
			c.SetModelTextureRegistry(registry)
			c.InstallPreparedModelSkin(skin)
			requireSkinOK(t, c.SetOwnerModelSkin(1, "infected"))
			requireSkinOK(t, c.SetUnitModelSkin(71, "infected"))
			if c.SetUnitModelSkin(0, "infected") == nil || c.SetOwnerModelSkin(1, "missing") == nil {
				t.Fatal("invalid selection accepted")
			}
			if c.selectedModelSkin(71, 0) != skin {
				t.Fatal("failed selection lost prior mapping")
			}
			// A slot-only preview cannot acquire the mapping for an equal instance id.
			if c.selectedModelSkin(0, 0) != nil {
				t.Fatal("zero publication identity acquired an override")
			}
			c.SetModelTextureRegistry(registry)
			if c.selectedModelSkin(71, 0) != skin {
				t.Fatal("same registry cleared skins")
			}
			c.SetModelTextureRegistry(nil)
			if c.selectedModelSkin(71, 1) != nil || len(c.modelSkins) != 0 {
				t.Fatal("battle boundary retained skins")
			}
			c.InstallPreparedModelSkin(skin)
			requireSkinOK(t, c.SetOwnerModelSkin(1, "infected"))
			c.SetModelFS(fs)
			if c.selectedModelSkin(71, 1) != nil {
				t.Fatal("content boundary retained skins")
			}
			c.InstallPreparedModelSkin(skin)
			c.Close()
			if len(c.modelSkins) != 0 {
				t.Fatal("close retained skin bank")
			}
		})
	}
	c := testModelTextureClient()
	fs, _ := skinTestFS(t, skinEntry("tex", 42))
	c.modelFS = fs
	for _, p := range []string{"textures/infected.gaf", "../skins/infected.gaf", "/skins/infected.gaf", "skins/../skins/infected.gaf"} {
		if c.LoadModelSkin("infected", p) == nil {
			t.Fatalf("invalid path accepted: %s", p)
		}
	}
	if c.LoadModelSkin("", "skins/infected.gaf") == nil {
		t.Fatal("empty name accepted")
	}
}

// Even when a bank prepared for another content binding reaches a draw, it may
// never replace animated or team frames. Missing names keep their base pixels.
func TestModelSkinPreservesSpecialBindingsAndMissingNames(t *testing.T) {
	c := testModelTextureClient()
	skin := &ModelSkin{name: "infected", frames: map[string]*formats.GAFFrame{"tex": {Width: 1, Height: 1, Pixels: []byte{42}}}}
	draw := testPrimitiveDraw(presentationrender.PrimitiveDraw{TextureName: "tex", VertexIndices: []uint16{0, 1, 2, 3}}, [][3]numeric.Fixed{fixedVertex(0, 0, 0), fixedVertex(8, 0, 0), fixedVertex(8, 0, -8), fixedVertex(0, 0, -8)})
	original := c.texIndex["tex"].frame
	for _, kind := range []texKind{texStatic, texTeam, texAnimated} {
		c.texIndex["tex"] = texRef{kind: kind, frame: original, entry: &formats.GAFEntry{Frames: []formats.GAFFrameRef{{Frame: original}}}}
		got := c.collectDrawPolys(draw, teamColor{known: true}, 1, modelCursorUnit, skin)
		want := original
		if kind == texStatic {
			want = skin.frames["tex"]
		}
		if len(got) != 1 || got[0].frame != want {
			t.Fatalf("kind %d changed special binding", kind)
		}
	}
	c.texIndex["tex"] = texRef{kind: texStatic, frame: original}
	for _, kind := range []uint8{modelCursorFeature, modelCursorProjectile, modelCursorDebris} {
		got := c.collectDrawPolys(draw, teamColor{}, 1, kind, skin)
		if len(got) != 1 || got[0].frame != original {
			t.Fatal("non-unit model was skinned")
		}
	}
	empty := &ModelSkin{frames: map[string]*formats.GAFFrame{}}
	if got := c.collectDrawPolys(draw, teamColor{}, 1, modelCursorUnit, empty); len(got) != 1 || got[0].frame != original {
		t.Fatal("missing name lost base texture")
	}
}

func TestModelSkinLiveAndDirectUnitPaths(t *testing.T) {
	for _, geometry := range []bool{false, true} {
		for _, keyed := range []bool{false, true} {
			c, v, _ := cachedTeamColorRegressionSubject(t)
			c.texIndex["logo"] = texRef{kind: texStatic, frame: &formats.GAFFrame{Width: 1, Height: 1, Pixels: []byte{17}}}
			c.geometryOnlyModels = geometry
			v.ZBuffer = keyed
			c.InstallPreparedModelSkin(&ModelSkin{name: "infected", frames: map[string]*formats.GAFFrame{"logo": {Width: 1, Height: 1, Pixels: []byte{42}}}})
			requireSkinOK(t, c.SetUnitModelSkin(v.InstanceID, "infected"))
			// This fixture's only piece is live. Keyed geometry records LiveFaces;
			// keyless geometry emits a direct packet. Classic stages or draws direct.
			v.Pieces = []frame.PieceView{{Index: 0, DontCache: true}}
			if geometry {
				c.list.Reset()
				c.modelScratch.reset()
				c.modelScratch.active = true
				if !c.drawUnitModel(v, 0, 0) {
					t.Fatal("no geometry draw")
				}
				found := false
				for _, cmd := range c.list.ModelCommands() {
					if cmd.Geometry == nil {
						continue
					}
					for _, faces := range [][]drawlist.ModelFace{cmd.Geometry.Faces, cmd.Geometry.LiveFaces} {
						for _, face := range faces {
							if face.Texture != nil && face.Texture.Pixels[0] == 42 {
								found = true
							}
						}
					}
				}
				if !found {
					t.Fatalf("geometry=%v keyed=%v lost live skin", geometry, keyed)
				}
			} else {
				cachedLiveReplay(t, c, v)
				if bytes.Count(c.indexed, []byte{42}) == 0 {
					t.Fatalf("classic keyed=%v lost live skin", keyed)
				}
			}
		}
	}
}

func TestModelSkinParallelSelection(t *testing.T) {
	c, v, _ := cachedTeamColorRegressionSubject(t)
	c.texIndex["logo"] = texRef{kind: texStatic, frame: &formats.GAFFrame{Width: 1, Height: 1, Pixels: []byte{17}}}
	c.texRefs = &sync.Map{}
	c.InstallPreparedModelSkin(&ModelSkin{name: "infected", frames: map[string]*formats.GAFFrame{"logo": {Width: 1, Height: 1, Pixels: []byte{42}}}})
	requireSkinOK(t, c.SetOwnerModelSkin(1, "infected"))
	c.geometryOnlyModels = true
	c.recordPool = newRecordPool(4)
	c.recordPool.wakeAll = true
	defer c.Close()
	units := make([]frame.UnitView, parallelUnitFloor+8)
	jobs := make([]int32, len(units))
	for i := range units {
		units[i] = v
		units[i].InstanceID += uint64(i)
		units[i].Owner = uint8(i % 2)
		jobs[i] = int32(i)
	}
	for pass := range 2 {
		c.modelScratch.reset()
		c.modelScratch.active = true
		c.prepareUnitGeometry(units, jobs)
		for i, pair := range c.recordPool.pairs {
			want := byte(17)
			if units[i].Owner == 1 && pass == 0 {
				want = 42
			}
			if !pair.ready || pair.body == nil || len(pair.body.Faces) == 0 || pair.body.Faces[0].Texture.Pixels[0] != want {
				t.Fatalf("worker selection pass=%d instance=%d", pass, i)
			}
		}
		c.modelScratch.active = false
		requireSkinOK(t, c.SetOwnerModelSkin(1, ""))
	}
}
