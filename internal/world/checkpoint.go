package world

import (
	"fmt"
	"slices"

	"github.com/nanolathe-gg/nanolathe/internal/content"
	"github.com/nanolathe-gg/nanolathe/internal/sim/checkpoint"
)

// CheckpointContext binds frozen content and the terrain collected for section
// 5. It is capture-local, never simulation state (DESIGN_MULTIPLAYER §16.3.11).
type CheckpointContext struct {
	Keys    *content.CheckpointKeys
	Terrain *Terrain

	movementBinding *checkpointMovementReceipt // capture-local exact installation
}

// NewCheckpointContext binds frozen keys without reading the terrain.
func NewCheckpointContext(keys *content.CheckpointKeys) *CheckpointContext {
	return &CheckpointContext{Keys: keys}
}

// CollectCheckpointReferences validates this owner and records its composition
// identity. Terrain introduces no object tables.
func (t *Terrain) CollectCheckpointReferences(c *CheckpointContext) (int, error) {
	if err := t.validateCheckpoint(c); err != nil {
		return 0, err
	}
	c.Terrain = t
	return 0, nil
}

// WriteCheckpoint writes only the terrain payload. Fields in lexical order:
// ClassRestamp, FeatureDefs, FeatureNames, Movers, Plot, metalSeeded. Definition
// rows have presence, variant, base key, and (variant 2 only) FootprintX,
// FootprintZ, Damage, Metal, Energy. Plot rows are AnchorWord, Feature, Flags,
// Metal, OccupantA, OccupantB. U6 supplies map admission and section framing;
// immutable geometry/LOS, diagnostic revisions, and entry sweep undo data have
// no payload here (DESIGN_MULTIPLAYER §16.3.5–§16.3.6, §16.3.11).
func (t *Terrain) WriteCheckpoint(e *checkpoint.Encoder, c *CheckpointContext) error {
	e.Field("world.Terrain")
	if err := t.validateCheckpoint(c); err != nil {
		e.Fail(err)
		return e.Err()
	}
	e.Field("world.Terrain.ClassRestamp")
	e.Bool(t.classRestamp != nil)
	e.Field("world.Terrain.FeatureDefs")
	e.Count(len(t.FeatureDefs))
	for i, value := range t.FeatureDefs {
		e.Field(fmt.Sprintf("world.Terrain.FeatureDefs[%d]", i))
		e.Bool(value != nil)
		if value == nil {
			continue
		}
		ref, err := t.CheckpointFeature(c.Keys, value, nil)
		if err != nil {
			e.Fail(err)
			return e.Err()
		}
		e.U8(ref.Variant)
		e.Definition(ref.Base)
		if ref.Variant == 2 {
			e.I32(ref.FootprintX)
			e.I32(ref.FootprintZ)
			e.I32(ref.Damage)
			e.I32(ref.Metal)
			e.I32(ref.Energy)
		}
	}
	e.Field("world.Terrain.FeatureNames")
	e.Count(len(t.FeatureNames))
	for _, name := range t.FeatureNames {
		e.String(name)
	}
	e.Field("world.Terrain.Movers")
	e.Bool(t.movers != nil)
	e.Field("world.Terrain.Plot")
	e.Count(len(t.Plot))
	for i, cell := range t.Plot {
		e.FieldIndex("world.Terrain.Plot", i, "")
		e.U16(cell.AnchorWord())
		e.U16(cell.Feature())
		e.U8(cell.FlagByte() &^ (0x04 | 0x78))
		e.U8(cell.Metal())
		e.I16(cell.OccupantA())
		e.I16(cell.OccupantB())
	}
	e.Field("world.Terrain.metalSeeded")
	e.Bool(t.metalSeeded)
	return e.Err()
}

func (t *Terrain) validateCheckpoint(c *CheckpointContext) error {
	if t == nil {
		return worldCheckpointError("world.Terrain", "a present terrain")
	}
	if c == nil || c.Keys == nil {
		return worldCheckpointError("world.context", "admitted content keys")
	}
	if c.Terrain != nil && c.Terrain != t {
		return worldCheckpointError("world.context.Terrain", "the collected terrain identity")
	}
	if t.CellW <= 0 || t.CellH <= 0 || int64(t.CellW)*int64(t.CellH) != int64(len(t.Plot)) {
		return worldCheckpointError("world.Terrain.Plot", "the complete row-major plot dimensions")
	}
	if t.voidPassActive {
		return worldCheckpointError("world.Terrain.voidPassActive", "a completed entry feature pass")
	}
	if !t.voidSwept {
		return worldCheckpointError("world.Terrain.voidSwept", "the completed entry void sweep")
	}
	if err := t.validateCheckpointMovementBindings(c); err != nil {
		return err
	}
	for i, value := range t.FeatureDefs {
		if value == nil {
			continue
		}
		if _, err := t.CheckpointFeature(c.Keys, value, nil); err != nil {
			return fmt.Errorf("%w: %w", worldCheckpointError(fmt.Sprintf("world.Terrain.FeatureDefs[%d]", i), "an admitted feature definition or validated normalization"), err)
		}
	}
	return nil
}

// RecordCheckpointFeatureNormalization retains only an exact object already
// held by the terrain table. A failed, unretained placement cannot gain a
// diagnostic keepalive (DESIGN_MULTIPLAYER §16.3.11).
func (t *Terrain) RecordCheckpointFeatureNormalization(base, value *content.FeatureDef) {
	if t == nil || base == nil || value == nil || base == value || !slices.Contains(t.FeatureDefs, value) {
		return
	}
	if t.checkpointFeatureBases == nil {
		t.checkpointFeatureBases = make(map[*content.FeatureDef]*content.FeatureDef)
	}
	// A relation is recorded once at creation, never redirected by later calls.
	if _, known := t.checkpointFeatureBases[value]; !known {
		t.checkpointFeatureBases[value] = base
	}
}

// CheckpointFeature resolves admitted objects or an explicitly recorded base
// relation. Equal names never supply provenance; content verifies the transform.
func (t *Terrain) CheckpointFeature(keys *content.CheckpointKeys, value, base *content.FeatureDef) (content.CheckpointFeature, error) {
	if t != nil {
		if recorded := t.checkpointFeatureBases[value]; recorded != nil {
			if !slices.Contains(t.FeatureDefs, value) || (base != nil && base != recorded) {
				return content.CheckpointFeature{}, worldCheckpointError("world.feature", "the retained normalization relation")
			}
			base = recorded
		}
	}
	if base != nil {
		// TODO(M3-U6): a repeatedly normalized empty-key copy needs an admitted
		// original base; an intermediate copy is not an admitted identity.
		return keys.NormalizedFeature(base, value)
	}
	ref, err := keys.Feature(value)
	if err != nil {
		return content.CheckpointFeature{}, err
	}
	return content.CheckpointFeature{Variant: 1, Base: ref}, nil
}

func worldCheckpointError(path, expected string) error {
	return fmt.Errorf("nanolathe: checkpoint capture failed: logical path %s, providers searched [world], expected %s", path, expected)
}
