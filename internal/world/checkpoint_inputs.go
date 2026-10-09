package world

import (
	"math"
	"slices"

	"github.com/nanolathe-gg/nanolathe/internal/content"
	"github.com/nanolathe-gg/nanolathe/internal/sim/numeric"
	"github.com/nanolathe-gg/nanolathe/vfs"
)

// This is load provenance, not world payload. Only the successful Load tail
// creates it; an arbitrary later snapshot would bless already changed inputs
// (DESIGN_MULTIPLAYER §16.3.76).
type checkpointTerrainInputs struct {
	terrain     *Terrain
	filesystem  vfs.FSOps
	catalog     *content.Catalog
	terrainKey  string
	logicalTNT  string
	values      checkpointTerrainValues
	plotPresent bool
	heights     [][3]uint8 // height, derived minimum, derived maximum in plot order
	losWords    []uint16
}

// Tidal is stored as bits so signed zero is not lost to numeric equality.
// The other fields preserve their source widths, including fixed-point gravity
// and the LOS build count whose zero value gates the actual word reader.
type checkpointTerrainValues struct {
	cellW, cellH                 int32
	version                      Version
	seaLevel                     uint8
	gravity                      numeric.Fixed
	authoredGravity, otaGravity  int32
	lavaWorld                    bool
	waterDoesDamage, waterDamage int32
	windMin, windMax             int32
	tidalBits                    uint32
	playRight, playBottom        int32
	losBuildCount                int
}

func (t *Terrain) checkpointInputValues() checkpointTerrainValues {
	return checkpointTerrainValues{
		cellW: t.CellW, cellH: t.CellH, version: t.Version, seaLevel: t.SeaLevel,
		gravity: t.Gravity, authoredGravity: t.AuthoredGravity, otaGravity: t.OTAGravity,
		lavaWorld: t.LavaWorld, waterDoesDamage: t.WaterDoesDamage, waterDamage: t.WaterDamage,
		windMin: t.WindMin, windMax: t.WindMax, tidalBits: math.Float32bits(t.Tidal),
		playRight: t.PlayRight, playBottom: t.PlayBottom, losBuildCount: t.losBuildCount,
	}
}

func (t *Terrain) snapshotCheckpointInputs(fs vfs.FSOps, cat *content.Catalog, key, logicalTNT string) {
	s := &checkpointTerrainInputs{
		terrain: t, filesystem: fs, catalog: cat, terrainKey: key, logicalTNT: logicalTNT,
		values: t.checkpointInputValues(), plotPresent: t.Plot != nil,
		heights: make([][3]uint8, len(t.Plot)), losWords: slices.Clone(t.losWords),
	}
	for i, cell := range t.Plot {
		s.heights[i] = [3]uint8{cell.Height(), cell.MinHeight(), cell.MaxHeight()}
	}
	t.checkpointInputs = s
}

// ValidateCheckpointInputs observes the actual load's immutable values and
// source ownership, never a reconstruction (DESIGN_MULTIPLAYER §16.3.76).
// expectedTerrainKey is the original mission terrain key, compared exactly,
// not a display map name. The composition root separately validates frozen
// content and the completed-entry boundary, and retains the original Terrain.
// Mutable plot fields and renderer-only tiles stay outside this proof.
func (t *Terrain) ValidateCheckpointInputs(inputs *content.SimulationInputs, expectedTerrainKey string) error {
	if t == nil || inputs == nil || t.checkpointInputs == nil {
		return worldCheckpointError("world.Terrain.inputs", "a loaded terrain and frozen simulation inputs")
	}
	s := t.checkpointInputs
	if s.terrain != t || s.catalog == nil || s.catalog != inputs.Catalog() || s.terrainKey != expectedTerrainKey ||
		!inputs.CheckpointFilesystemMatches(s.filesystem) {
		return worldCheckpointError("world.Terrain.inputs.source", "the original terrain, catalog, requested key and frozen filesystem")
	}
	_, tnt := inputs.MapFiles()
	if s.logicalTNT != tnt {
		return worldCheckpointError("world.Terrain.inputs.tnt", "the frozen selection's exact terrain logical path")
	}
	values := t.checkpointInputValues()
	if values != s.values || values.tidalBits&0x7fffffff > 0x7f800000 {
		return worldCheckpointError("world.Terrain.inputs.values", "unchanged load-time dimensions and environment without NaN")
	}
	if (t.Plot != nil) != s.plotPresent || len(t.Plot) != len(s.heights) {
		return worldCheckpointError("world.Terrain.inputs.Plot", "the original plot shape")
	}
	for i, cell := range t.Plot {
		if [3]uint8{cell.Height(), cell.MinHeight(), cell.MaxHeight()} != s.heights[i] {
			return worldCheckpointError("world.Terrain.inputs.heights", "unchanged load-time height and floor bounds")
		}
	}
	if (t.losWords == nil) != (s.losWords == nil) || !slices.Equal(t.losWords, s.losWords) {
		return worldCheckpointError("world.Terrain.inputs.losWords", "unchanged load-time LOS words")
	}
	return nil
}
