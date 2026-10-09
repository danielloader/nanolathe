package content

import (
	"slices"

	"github.com/nanolathe-gg/nanolathe/internal/cob"
	"github.com/nanolathe-gg/nanolathe/internal/model"
)

type checkpointInputUnit struct {
	pointer *UnitDef
	value   UnitDef
	unknown checkpointInputMap[string, string]
	program checkpointInputProgram
}

func snapshotCheckpointInputUnit(u *UnitDef) checkpointInputUnit {
	v := *u
	v.Unknown = nil
	v.Script = nil
	v.VeterancyThresholds = slices.Clone(u.VeterancyThresholds)
	v.UnitMask.words = slices.Clone(u.UnitMask.words)
	v.BadTargetCategoryWPRIMask.words = slices.Clone(u.BadTargetCategoryWPRIMask.words)
	v.BadTargetCategoryWSECMask.words = slices.Clone(u.BadTargetCategoryWSECMask.words)
	v.BadTargetCategoryWSPEMask.words = slices.Clone(u.BadTargetCategoryWSPEMask.words)
	v.NoChaseCategoryMask.words = slices.Clone(u.NoChaseCategoryMask.words)
	return checkpointInputUnit{u, v, snapshotCheckpointInputMap(u.Unknown), snapshotCheckpointInputProgram(u.Script)}
}
func (s checkpointInputUnit) matches() bool {
	a, b := s.pointer, &s.value
	return sameCheckpointInputUnit(a, b) && s.unknown.matches(a.Unknown) && s.program.matches(a.Script) &&
		checkpointInputSlice(a.VeterancyThresholds, b.VeterancyThresholds) && checkpointInputSlice(a.UnitMask.words, b.UnitMask.words) &&
		checkpointInputSlice(a.BadTargetCategoryWPRIMask.words, b.BadTargetCategoryWPRIMask.words) &&
		checkpointInputSlice(a.BadTargetCategoryWSECMask.words, b.BadTargetCategoryWSECMask.words) &&
		checkpointInputSlice(a.BadTargetCategoryWSPEMask.words, b.BadTargetCategoryWSPEMask.words) && checkpointInputSlice(a.NoChaseCategoryMask.words, b.NoChaseCategoryMask.words)
}

type checkpointInputWeapon struct {
	pointer *WeaponDef
	value   WeaponDef
	damage  checkpointInputMap[string, int32]
	unknown checkpointInputMap[string, string]
}

func snapshotCheckpointInputWeapon(w *WeaponDef) checkpointInputWeapon {
	v := *w
	v.Damage = nil
	v.Unknown = nil
	v.damageOrder = slices.Clone(w.damageOrder)
	return checkpointInputWeapon{w, v, snapshotCheckpointInputMap(w.Damage), snapshotCheckpointInputMap(w.Unknown)}
}
func (s checkpointInputWeapon) matches() bool {
	return sameCheckpointInputWeapon(s.pointer, &s.value) && s.damage.matches(s.pointer.Damage) && s.unknown.matches(s.pointer.Unknown) && checkpointInputSlice(s.pointer.damageOrder, s.value.damageOrder)
}

type checkpointInputFeature struct {
	pointer *FeatureDef
	value   FeatureDef
	unknown checkpointInputMap[string, string]
}

func snapshotCheckpointInputFeature(f *FeatureDef) checkpointInputFeature {
	v := *f
	v.Unknown = nil
	return checkpointInputFeature{f, v, snapshotCheckpointInputMap(f.Unknown)}
}
func (s checkpointInputFeature) matches() bool {
	return sameCheckpointInputFeature(s.pointer, &s.value) && s.unknown.matches(s.pointer.Unknown)
}

type checkpointInputMovement struct {
	key     string
	pointer *MovementClass
	value   MovementClass
}
type checkpointInputSide struct {
	pointer *SideDef
	value   SideDef
	anchors checkpointInputMap[string, Rect]
}
type checkpointInputBuildMenu struct {
	key     string
	pointer *BuildMenuPage
	value   BuildMenuPage
}
type checkpointInputSequence struct {
	key     string
	pointer *simArtSequence
	visits  int32
	frames  []simArtFrame
}

// COB identity remains semantic: another Program with the same words and
// entry points is valid. Nil/empty slices have the same existing digest;
// original Program pointer addresses are not its identity (§16.3.6, §16.3.56).
type checkpointInputProgram struct {
	present  bool
	code     []uint32
	scripts  checkpointInputMap[string, int]
	pieces   []string
	statics  int
	ids      []int
	checksum uint32
}

func snapshotCheckpointInputProgram(p *cob.Program) checkpointInputProgram {
	if p == nil {
		return checkpointInputProgram{}
	}
	return checkpointInputProgram{true, slices.Clone(p.Code), snapshotCheckpointInputMap(p.Scripts), slices.Clone(p.Pieces), p.Statics, slices.Clone(p.ScriptsByID), p.SourceChecksum}
}
func (s checkpointInputProgram) matches(p *cob.Program) bool {
	if p == nil {
		return !s.present
	}
	return s.present && slices.Equal(p.Code, s.code) && s.scripts.entriesMatch(p.Scripts) && slices.Equal(p.Pieces, s.pieces) && p.Statics == s.statics && slices.Equal(p.ScriptsByID, s.ids) && p.SourceChecksum == s.checksum
}

func snapshotCheckpointInputLOS(a *LOSTables) *LOSTables {
	b := *a
	b.Tables = slices.Clone(a.Tables)
	for i := range b.Tables {
		b.Tables[i].Lines = slices.Clone(a.Tables[i].Lines)
		for j := range b.Tables[i].Lines {
			b.Tables[i].Lines[j] = slices.Clone(a.Tables[i].Lines[j])
		}
	}
	return &b
}
func sameCheckpointInputLOS(a, b *LOSTables) bool {
	if !sameCheckpointInputHeader(a.DefinitionHeader, b.DefinitionHeader) || a.NumTables != b.NumTables || (a.Tables == nil) != (b.Tables == nil) || len(a.Tables) != len(b.Tables) {
		return false
	}
	for i, t := range a.Tables {
		v := b.Tables[i]
		if t.TableNum != v.TableNum || t.NumLines != v.NumLines || (t.Lines == nil) != (v.Lines == nil) || len(t.Lines) != len(v.Lines) {
			return false
		}
		for j := range t.Lines {
			if !checkpointInputSlice(t.Lines[j], v.Lines[j]) {
				return false
			}
		}
	}
	return true
}

type checkpointInputModel struct {
	key     string
	pointer *model.Model
	value   model.Model
}

// Model has only Name, Root, Hash and Pieces; there is no mutable model cache
// to exclude. Copy every nested geometry slice, including authored primitives.
func snapshotCheckpointInputModel(a *model.Model) model.Model {
	b := *a
	b.Pieces = slices.Clone(a.Pieces)
	for i := range b.Pieces {
		p := &b.Pieces[i]
		p.Children = slices.Clone(p.Children)
		p.Vertices = slices.Clone(p.Vertices)
		p.Primitives = slices.Clone(p.Primitives)
		for j := range p.Primitives {
			p.Primitives[j].VertexIndices = slices.Clone(p.Primitives[j].VertexIndices)
		}
	}
	return b
}
func sameCheckpointInputModel(a, b *model.Model) bool {
	if a.Name != b.Name || a.Root != b.Root || a.Hash != b.Hash || (a.Pieces == nil) != (b.Pieces == nil) || len(a.Pieces) != len(b.Pieces) {
		return false
	}
	for i, p := range a.Pieces {
		v := b.Pieces[i]
		if p.Name != v.Name || p.Parent != v.Parent || p.Translate != v.Translate || p.Selection != v.Selection || !checkpointInputSlice(p.Children, v.Children) || !checkpointInputSlice(p.Vertices, v.Vertices) || (p.Primitives == nil) != (v.Primitives == nil) || len(p.Primitives) != len(v.Primitives) {
			return false
		}
		for j, x := range p.Primitives {
			y := v.Primitives[j]
			if x.ColorIndex != y.ColorIndex || x.TextureName != y.TextureName || x.IsColored != y.IsColored || x.SourceIndex != y.SourceIndex || !checkpointInputSlice(x.VertexIndices, y.VertexIndices) {
				return false
			}
		}
	}
	return true
}
