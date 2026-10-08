package meshscene

import (
	"time"

	"github.com/nanolathe-gg/nanolathe/formats"
	"github.com/nanolathe-gg/nanolathe/internal/drawlist"
	"github.com/nanolathe-gg/nanolathe/internal/model"
	"github.com/nanolathe-gg/nanolathe/internal/palette"
	"github.com/nanolathe-gg/nanolathe/internal/session"
	"github.com/nanolathe-gg/nanolathe/vfs"
)

const battlePeriod = time.Second / 30

type battleSource struct {
	materialAtlas         *atlasResult
	materialSource        *ModelMaterialSource
	materialFramesMissing uint64
	effects               drawlist.Effects
	scene                 *Scene
	sess                  *session.Session // accessed only during preparation or by worker
	fs                    *vfs.FS
	models                []*model.Model
	modelIndices          map[string]int
	originY               float32
	viewport              [2]int
	sprites               battleSpriteAdapter
	palette               *palette.Tables
	fogGray, fogBlack     [4]*formats.GAFEntry
	poseScratch           battlePoseScratch
	timing                sectionTiming
	materialCache         map[modelMaterialCacheKey][]NativeModelMaterial
	materialArena         *[]NativeModelMaterial // the publication being built; uncached material walks use its storage
	materialPieces        []bool
	modelKeys             battleModelKeys
	cullSlack             float32 // widens model culling, world units (RetainedBattle.Frame)
	fogGeneration         uint64  // numbers built fog textures (FogFrame.Generation)
	lookup                publicationLookup
	spectator             bool
	missingModels         map[string]int
}

func newBattleSource(scene *Scene, sess *session.Session, fs *vfs.FS, models []*model.Model, indices map[string]int, originY float32, viewport [2]int) *battleSource {
	return &battleSource{effects: drawlist.AllEffects(), scene: scene, sess: sess, fs: fs, models: models, modelIndices: indices, originY: originY, viewport: viewport, missingModels: map[string]int{}}
}
func (s *battleSource) noteMissing(name string) { s.missingModels[name]++ }
