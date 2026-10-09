package movement

import (
	"errors"
	"slices"

	"github.com/nanolathe-gg/nanolathe/internal/sim/checkpoint"
)

func validateMovementLayers(s *System, c *CheckpointContext, r *ClassLayers) error {
	if r.terrain != nil && r.terrain != s.Terrain || r.grid != nil && r.grid != s.Grid || r.world != nil && r.world != s.world {
		return errors.New("class registry owner alias differs")
	}
	if r.anchors != nil {
		if v, ok := r.anchors.(*System); !ok || v == nil || v != s {
			return errors.New("unsupported registry anchors")
		}
	}
	if r.movers != nil {
		if v, ok := r.movers.(*System); !ok || v == nil || v != s {
			return errors.New("unsupported registry movers")
		}
	}
	if r.ticks != nil {
		if v, ok := r.ticks.(*System); !ok || v == nil || v != s {
			return errors.New("unsupported registry ticks")
		}
	}
	return validateCheckpointMapping(c, r.mapping, r.checkpointMapping)
}

func checkpointLayerKeys(r *ClassLayers) []string {
	keys := make([]string, 0, len(r.byName))
	for key := range r.byName {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	return keys
}

// Registry: anchors, byName, grid, mapping, movers, names, terrain, ticks,
// world. Membership is key-sorted, while names keeps allocation order.
func writeMovementLayers(e *checkpoint.Encoder, c *CheckpointContext, s *System, r *ClassLayers, p string) {
	m := movementCheckpointRecord{e, c, s, p}
	if err := validateMovementLayers(s, c, r); err != nil {
		m.fail("", err)
		return
	}
	m.boolean("anchors", r.anchors != nil)
	keys := checkpointLayerKeys(r)
	m.count("byName", len(keys))
	memberPrefix := p + ".byName"
	for i, key := range keys {
		e.FieldIndex(memberPrefix, i, ".key")
		e.String(key)
		id, ok := c.Layers.Find(r.byName[key])
		e.FieldIndex(memberPrefix, i, ".value")
		writeMovementReference(e, 5, id, ok)
	}
	m.boolean("grid", r.grid != nil)
	m.boolean("mapping", r.mapping != nil)
	m.boolean("movers", r.movers != nil)
	m.count("names", len(r.names))
	namePrefix := p + ".names"
	for i, v := range r.names {
		e.FieldIndex(namePrefix, i, "")
		e.String(v)
	}
	m.boolean("terrain", r.terrain != nil)
	m.boolean("ticks", r.ticks != nil)
	m.boolean("world", r.world != nil)
}

// ClassLayer: Grid, H, Profile, Terrain, W, cells, commits, mapping, movers,
// watermark. Profile is nested lexically. All stamp buffers and fullStamps
// are scratch/diagnostics, never refreshed here (DESIGN_MULTIPLAYER §16.3.5).
func validateMovementLayer(s *System, c *CheckpointContext, v *ClassLayer) error {
	if v == nil {
		return errors.New("absent class-layer table record")
	}
	if v.Grid != nil && v.Grid != s.Grid || v.Terrain != nil && v.Terrain != s.Terrain {
		return errors.New("class layer owner alias differs")
	}
	if v.movers != nil {
		if source, ok := v.movers.(*System); !ok || source == nil || source != s {
			return errors.New("unsupported class mover source")
		}
	}
	return validateCheckpointMapping(c, v.mapping, v.checkpointMapping)
}

func writeMovementLayer(e *checkpoint.Encoder, c *CheckpointContext, s *System, v *ClassLayer, p string) {
	m := movementCheckpointRecord{e, c, s, p}
	if err := validateMovementLayer(s, c, v); err != nil {
		m.fail("", err)
		return
	}
	m.boolean("Grid", v.Grid != nil)
	m.i32("H", v.H)
	writeMovementProfile(e, c, s, &v.Profile, p+".Profile")
	m.boolean("Terrain", v.Terrain != nil)
	m.i32("W", v.W)
	m.count("cells", len(v.cells))
	for _, x := range v.cells {
		e.U32(x)
	}
	m.count("commits", len(v.commits))
	commitPrefix := p + ".commits"
	for i, x := range v.commits {
		e.FieldIndex(commitPrefix, i, ".set")
		e.Bool(x.set)
		e.FieldIndex(commitPrefix, i, ".tick")
		e.U32(x.tick)
	}
	m.boolean("mapping", v.mapping != nil)
	m.boolean("movers", v.movers != nil)
	m.u32("watermark", v.watermark)
}

func writeMovementLearned(e *checkpoint.Encoder, c *CheckpointContext, s *System, v *LearnedTerrain, p string) {
	m := movementCheckpointRecord{e, c, s, p}
	m.i32("h", v.h)
	m.i32("w", v.w)
	m.count("words", len(v.words))
	for _, x := range v.words {
		e.U16(x)
	}
}
