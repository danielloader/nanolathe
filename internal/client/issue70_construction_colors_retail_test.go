package client

import (
	"bytes"
	"strings"
	"testing"

	"github.com/nanolathe-gg/nanolathe/formats"
	"github.com/nanolathe-gg/nanolathe/internal/cob"
	"github.com/nanolathe-gg/nanolathe/internal/frame"
)

// The named stock lamps are ordinary animated textures on pieces whose Create
// script clears the cache bit [03 R-REN-03A §5]. Keep the pose and reveal pulse
// fixed while phase 7 advances: only texture selection may change these images.
func TestStockConstructionLampsWaitForCompletion(t *testing.T) {
	for _, name := range []string{"CORLAB", "CORALAB", "ARMESTOR", "CORESTOR"} {
		t.Run(name, func(t *testing.T) {
			c, fs := captureClient(t, 320, 240)
			r := newModelTextureRegistry(nil, false)
			r.fs, r.primary, r.logos = fs, c.texIndex, c.logoIndex
			load := modelTextureLoadKey{kind: modelLoadUnit, id: ckey(name)}
			m, err := r.bindLoad(load, name)
			if err != nil {
				t.Fatal(err)
			}
			c.modelTextures = r
			prog, _, err := cob.LoadFromFS(fs, name)
			if err != nil || prog == nil {
				t.Fatalf("%s script: %v", name, err)
			}
			vm := cob.NewPresentationVM(prog, 4096)
			flags := make([]uint8, len(prog.Pieces))
			for i := range flags {
				flags[i] = 7
			}
			vm.BindRenderFlags(flags)
			if !vm.StartByName("Create", nil) {
				t.Fatalf("%s Create unavailable", name)
			}
			for tick := 0; tick < 60; tick++ {
				vm.Drain(1)
			}
			v := frame.UnitView{InstanceID: 1, Slot: 1, Model: name, ZBuffer: true, NoShadow: true, OwnerColorKnown: true,
				CacheRevision: 1, CacheValidityRevision: 1, Pieces: make([]frame.PieceView, len(m.compiled.Pieces))}
			firstFrames := map[*formats.GAFFrame]bool{}
			currentFrames := map[*formats.GAFFrame]bool{}
			for pi, piece := range m.compiled.Pieces {
				v.Pieces[pi].Index = pi
				for si, scriptPiece := range prog.Pieces {
					if strings.EqualFold(piece.Name, scriptPiece) {
						state := vm.Pieces[si]
						v.Pieces[pi] = frame.PieceView{Index: pi, RotX: state.RotX, RotY: state.RotY, RotZ: state.RotZ,
							Tx: state.Trans[0], Ty: state.Trans[1], Tz: state.Trans[2],
							DontCache: flags[si]&2 == 0, DontShade: flags[si]&4 == 0, Hidden: flags[si]&1 == 0}
						break
					}
				}
				for pri, pr := range piece.Primitives {
					ref, ok := r.resolve(pr.TextureName)
					if !ok || ref.kind != texAnimated || pr.IsColored&1 != 0 || len(pr.VertexIndices) != 4 {
						continue
					}
					firstFrames[ref.entry.Frames[0].Frame] = true
					if v.Pieces[pi].DontCache {
						p := r.bindings[modelTexturePrimitiveKey{load: load, piece: pi, primitive: pri}]
						if p == nil {
							t.Fatal("stock lamp has no loaded cursor")
						}
						currentFrames[ref.entry.Frames[3].Frame] = true
					}
				}
			}
			if len(currentFrames) == 0 {
				t.Fatal("stock Create did not admit an animated live lamp")
			}
			v.BuildRemaining = .01
			cachedLiveReplay(t, c, v)
			writeCapture(t, c, name+"-construction-phase0")
			unfinished := append([]uint8(nil), c.indexed...)
			v.BuildRemaining = 0
			cachedLiveReplay(t, c, v)
			writeCapture(t, c, name+"-complete-phase0")
			complete := append([]uint8(nil), c.indexed...)
			for tick := 0; tick < 30; tick++ {
				r.StepPhase7()
			}
			v.BuildRemaining = .01
			v.CacheValidityRevision++
			cachedLiveReplay(t, c, v)
			writeCapture(t, c, name+"-construction-phase30")
			if !bytes.Equal(unfinished, c.indexed) {
				t.Error("construction cache rebuild changed the stock lamp pixels")
			}
			v.BuildRemaining = 0
			cachedLiveReplay(t, c, v)
			writeCapture(t, c, name+"-complete-phase30")
			if bytes.Equal(complete, c.indexed) {
				t.Error("completion did not expose the stock lamp's advancing cursor")
			}
			c.geometryOnlyModels = true
			v.BuildRemaining = .01
			g := recordKeyedSubject(t, c, v)
			seenFirst := false
			for _, face := range g.Faces {
				seenFirst = seenFirst || firstFrames[face.Texture]
				if currentFrames[face.Texture] {
					t.Error("modern construction geometry selected the running lamp frame")
				}
			}
			if !seenFirst || len(g.LiveFaces) != 0 {
				t.Error("modern construction geometry lost the first-frame cached lamps")
			}
			v.BuildRemaining = 0
			g = recordKeyedSubject(t, c, v)
			seenCurrent := false
			for _, face := range g.LiveFaces {
				seenCurrent = seenCurrent || currentFrames[face.Texture]
			}
			if !seenCurrent {
				t.Error("modern completion geometry did not join the current lamp frame")
			}
		})
	}
}
