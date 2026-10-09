package ai

import (
	"cmp"
	"slices"

	"github.com/nanolathe-gg/nanolathe/internal/content"
	"github.com/nanolathe-gg/nanolathe/internal/sim/checkpoint"
)

// Every Profile field is retained in source lexical order below. Presence on
// every map preserves fixtureWeights' nil-sensitive lazy application and
// difficulty switch. Neither defaults nor names are normalized during capture
// (DESIGN_MULTIPLAYER §16.3.5, §16.3.19; [08 R-AI-01 §12]).
func (p *Profile) writeCheckpoint(e *checkpoint.Encoder, c *CheckpointContext) error {
	const path = "ai.Manager.Profile"
	writeCheckpointIntMap(e, p.Limit, path+".Limit")
	e.Field(path + ".Plan")
	e.String(string(p.Plan))
	writeCheckpointIntMap(e, p.Weight, path+".Weight")
	writeCheckpointPlanMaps(e, p.allLimits, path+".allLimits")
	writeCheckpointPlanMaps(e, p.allWeights, path+".allWeights")
	e.Field(path + ".appliedCatalog")
	e.Bool(p.appliedCatalog != nil)
	e.Field(path + ".directives")
	e.Count(len(p.directives))
	for _, v := range p.directives {
		// AIDirective source lexical order is Args then Keyword, with authored
		// argument and directive order intact.
		e.Field(path + ".directives.Args")
		e.Count(len(v.Args))
		for _, arg := range v.Args {
			e.String(arg)
		}
		e.Field(path + ".directives.Keyword")
		e.String(v.Keyword)
	}
	writeCheckpointIntMap(e, p.fixtureLimits, path+".fixtureLimits")
	writeCheckpointIntMap(e, p.fixtureWeights, path+".fixtureWeights")
	writeCheckpointIDMap(e, p.limitsByID, path+".limitsByID")
	e.Field(path + ".name")
	e.String(p.name)
	e.Field(path + ".recordIDs")
	e.Bool(p.recordIDs != nil)
	if p.recordIDs != nil {
		rows, err := p.checkpointRecordIDs(c)
		if err != nil {
			e.Fail(err)
			return e.Err()
		}
		e.Count(len(rows))
		for _, row := range rows {
			e.Definition(row.key)
			e.U32(row.id)
		}
	}
	e.Field(path + ".textLoaded")
	e.Bool(p.textLoaded)
	writeCheckpointIDMap(e, p.weightsByID, path+".weightsByID")
	return e.Err()
}

// Map walks gather keys only. Values enter the stream after explicit sorting.
func checkpointSortedKeys[K ~string | ~uint32, V any](m map[K]V) []K {
	keys := make([]K, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	return keys
}

func writeCheckpointIntMap(e *checkpoint.Encoder, m map[string]int32, path string) {
	e.Field(path)
	e.Bool(m != nil)
	if m == nil {
		return
	}
	e.Count(len(m))
	for _, key := range checkpointSortedKeys(m) {
		e.String(key)
		e.I32(m[key])
	}
}

func writeCheckpointIDMap(e *checkpoint.Encoder, m map[uint32]int32, path string) {
	e.Field(path)
	e.Bool(m != nil)
	if m == nil {
		return
	}
	e.Count(len(m))
	for _, key := range checkpointSortedKeys(m) {
		e.U32(key)
		e.I32(m[key])
	}
}

func writeCheckpointPlanMaps(e *checkpoint.Encoder, m map[Difficulty]map[string]int32, path string) {
	e.Field(path)
	e.Bool(m != nil)
	if m == nil {
		return
	}
	e.Count(len(m))
	for _, key := range checkpointSortedKeys(m) {
		e.String(string(key))
		writeCheckpointIntMap(e, m[key], path+"["+string(key)+"]")
	}
}

type checkpointProfileRecord struct {
	key checkpoint.Definition
	id  uint32
}

// Definition identity is Family, Ordinal, Key. Equal names retain distinct
// admitted ordinals; neither pointer order nor the stored record ID sorts them.
func (p *Profile) checkpointRecordIDs(c *CheckpointContext) ([]checkpointProfileRecord, error) {
	defs := make([]*content.UnitDef, 0, len(p.recordIDs))
	for def := range p.recordIDs {
		defs = append(defs, def)
	}
	rows := make([]checkpointProfileRecord, 0, len(defs))
	for _, def := range defs {
		key, err := c.Units.Keys.Unit(def)
		if err != nil {
			return nil, aiCheckpointError("ai.Manager.Profile.recordIDs", "admitted definition identities")
		}
		rows = append(rows, checkpointProfileRecord{key: key, id: p.recordIDs[def]})
	}
	slices.SortFunc(rows, func(a, b checkpointProfileRecord) int {
		if d := cmp.Compare(a.key.Family, b.key.Family); d != 0 {
			return d
		}
		if d := cmp.Compare(a.key.Ordinal, b.key.Ordinal); d != 0 {
			return d
		}
		return cmp.Compare(a.key.Key, b.key.Key)
	})
	return rows, nil
}
